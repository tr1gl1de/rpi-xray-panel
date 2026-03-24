package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptrace"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/net/proxy"
)

const (
	pingTestURL    = "http://www.gstatic.com/generate_204"
	pingTimeout    = 10 * time.Second
	xrayStartWait = 2 * time.Second
)

// PingResult holds the latency test result for a server.
type PingResult struct {
	ServerID  int    `json:"server_id"`
	Name      string `json:"name"`
	LatencyMs int    `json:"latency_ms"` // -1 = timeout/error
	Error     string `json:"error,omitempty"`
}

// PingHandler manages latency test endpoints.
type PingHandler struct {
	Store   *Storage
	Runner  CommandRunner
	DataDir string // for temp xray configs
}

// measureLatencyViaSocks tests latency through a SOCKS5 proxy.
func measureLatencyViaSocks(socksAddr string, timeout time.Duration) (int, error) {
	dialer, err := proxy.SOCKS5("tcp", socksAddr, nil, proxy.Direct)
	if err != nil {
		return -1, fmt.Errorf("socks5 dialer: %w", err)
	}

	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			return dialer.Dial(network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}
	client := &http.Client{
		Transport: transport,
		Timeout:   timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	var ttfb time.Duration
	req, err := http.NewRequest("GET", pingTestURL, nil)
	if err != nil {
		return -1, err
	}

	var start time.Time
	trace := &httptrace.ClientTrace{
		GotFirstResponseByte: func() {
			ttfb = time.Since(start)
		},
	}
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), trace))

	start = time.Now()
	resp, err := client.Do(req)
	if err != nil {
		return -1, err
	}
	resp.Body.Close()

	if ttfb == 0 {
		ttfb = time.Since(start)
	}
	return int(ttfb.Milliseconds()), nil
}

// pingActiveServer tests the currently active xray instance (SOCKS5 on :1080).
func pingActiveServer() (int, error) {
	return measureLatencyViaSocks("127.0.0.1:1080", pingTimeout)
}

// pingServerWithTempXray spawns a temporary xray on a free port, tests, then kills it.
func (h *PingHandler) pingServerWithTempXray(server VLESSServer) (int, error) {
	// Find a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return -1, fmt.Errorf("free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Generate temp config
	configData, err := generatePingXrayConfig(server, port)
	if err != nil {
		return -1, fmt.Errorf("gen config: %w", err)
	}

	tmpDir := filepath.Join(h.DataDir, "tmp")
	os.MkdirAll(tmpDir, 0755)
	configPath := filepath.Join(tmpDir, fmt.Sprintf("ping_%d_%d.json", server.ID, port))
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		return -1, fmt.Errorf("write config: %w", err)
	}
	defer os.Remove(configPath)

	// Start temporary xray
	ctx, cancel := context.WithTimeout(context.Background(), pingTimeout+xrayStartWait)
	defer cancel()

	cmd := exec.CommandContext(ctx, "xray", "run", "-config", configPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("start xray: %w", err)
	}
	defer func() {
		cmd.Process.Kill()
		cmd.Wait()
	}()

	// Wait for xray to start listening
	deadline := time.Now().Add(xrayStartWait)
	socksAddr := fmt.Sprintf("127.0.0.1:%d", port)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", socksAddr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(200 * time.Millisecond)
	}

	return measureLatencyViaSocks(socksAddr, pingTimeout)
}

// generatePingXrayConfig creates a minimal xray config for latency testing.
func generatePingXrayConfig(server VLESSServer, socksPort int) ([]byte, error) {
	outboundSettings := map[string]interface{}{
		"vnext": []map[string]interface{}{
			{
				"address": server.Address,
				"port":    server.Port,
				"users": []map[string]interface{}{
					{
						"id":         server.UUID,
						"flow":       server.Flow,
						"encryption": "none",
					},
				},
			},
		},
	}

	streamSettings := map[string]interface{}{
		"network":  "tcp",
		"security": server.Security,
	}

	if server.Security == "reality" {
		streamSettings["realitySettings"] = map[string]interface{}{
			"serverName":  server.SNI,
			"publicKey":   server.PBK,
			"shortId":     server.SID,
			"fingerprint": server.FP,
		}
	} else if server.Security == "tls" {
		streamSettings["tlsSettings"] = map[string]interface{}{
			"serverName": server.SNI,
		}
	}

	config := map[string]interface{}{
		"log": map[string]interface{}{"loglevel": "none"},
		"inbounds": []map[string]interface{}{
			{
				"port":     socksPort,
				"listen":   "127.0.0.1",
				"protocol": "socks",
				"settings": map[string]interface{}{"auth": "noauth", "udp": false},
			},
		},
		"outbounds": []map[string]interface{}{
			{
				"protocol":       "vless",
				"settings":       outboundSettings,
				"streamSettings": streamSettings,
			},
		},
	}

	return json.MarshalIndent(config, "", "  ")
}

// HandlePing tests latency for a single server (GET /api/vless/{id}/ping).
func (h *PingHandler) HandlePing(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse /api/vless/{id}/ping
	path := strings.TrimPrefix(r.URL.Path, "/api/vless/")
	idStr := strings.TrimSuffix(path, "/ping")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "Invalid ID", http.StatusBadRequest)
		return
	}

	sl, err := h.Store.LoadServers()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	var server *VLESSServer
	var serverIdx int
	for i, s := range sl.Servers {
		if s.ID == id {
			server = &sl.Servers[i]
			serverIdx = i
			break
		}
	}
	if server == nil {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	result := PingResult{ServerID: server.ID, Name: server.Name}

	// If this is the active server, test through the running xray
	if serverIdx == sl.ActiveIdx {
		latency, err := pingActiveServer()
		if err != nil {
			result.LatencyMs = -1
			result.Error = err.Error()
		} else {
			result.LatencyMs = latency
		}
	} else {
		latency, err := h.pingServerWithTempXray(*server)
		if err != nil {
			result.LatencyMs = -1
			result.Error = err.Error()
		} else {
			result.LatencyMs = latency
		}
	}

	if isHTMX(r) {
		renderFragment(w, "ping-result", result)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// HandlePingAll tests all servers (GET /api/vless/ping-all).
func (h *PingHandler) HandlePingAll(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sl, err := h.Store.LoadServers()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if len(sl.Servers) == 0 {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode([]PingResult{})
		return
	}

	results := make([]PingResult, len(sl.Servers))
	var wg sync.WaitGroup

	// Ping active server first (no temp xray needed)
	// Ping non-active servers sequentially (avoid spawning many xray instances on RPi Zero)
	for i, s := range sl.Servers {
		results[i] = PingResult{ServerID: s.ID, Name: s.Name}

		if i == sl.ActiveIdx {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				latency, err := pingActiveServer()
				if err != nil {
					results[idx].LatencyMs = -1
					results[idx].Error = err.Error()
				} else {
					results[idx].LatencyMs = latency
				}
			}(i)
		}
	}
	wg.Wait()

	// Non-active servers sequentially
	for i, s := range sl.Servers {
		if i == sl.ActiveIdx {
			continue
		}
		latency, err := h.pingServerWithTempXray(s)
		if err != nil {
			results[i].LatencyMs = -1
			results[i].Error = err.Error()
		} else {
			results[i].LatencyMs = latency
		}
	}

	if isHTMX(r) {
		renderFragment(w, "ping-all-results", map[string]interface{}{
			"Results":   results,
			"Servers":   sl.Servers,
			"ActiveIdx": sl.ActiveIdx,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(results)
}
