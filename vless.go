package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ParseVLESSURL parses a vless:// URL into a VLESSServer.
func ParseVLESSURL(raw string) (VLESSServer, error) {
	if !strings.HasPrefix(raw, "vless://") {
		return VLESSServer{}, fmt.Errorf("not a vless:// URL")
	}

	u, err := url.Parse(raw)
	if err != nil {
		return VLESSServer{}, fmt.Errorf("invalid URL: %w", err)
	}

	port := 443
	if u.Port() != "" {
		p, err := strconv.Atoi(u.Port())
		if err != nil {
			return VLESSServer{}, fmt.Errorf("invalid port: %w", err)
		}
		port = p
	}

	uuid := u.User.Username()
	if uuid == "" {
		return VLESSServer{}, fmt.Errorf("missing UUID")
	}

	name := u.Fragment
	if name == "" {
		name = u.Hostname()
	}

	q := u.Query()

	return VLESSServer{
		Name:     name,
		Address:  u.Hostname(),
		Port:     port,
		UUID:     uuid,
		Flow:     q.Get("flow"),
		Security: q.Get("security"),
		SNI:      q.Get("sni"),
		PBK:      q.Get("pbk"),
		SID:      q.Get("sid"),
		FP:       q.Get("fp"),
	}, nil
}

// GenerateXrayConfig generates a minimal xray config.json for a given server.
func GenerateXrayConfig(server VLESSServer) ([]byte, error) {
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
		"inbounds": []map[string]interface{}{
			{
				"port":     1080,
				"protocol": "socks",
				"settings": map[string]interface{}{"auth": "noauth", "udp": true},
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

// VLESSHandler manages VLESS server API endpoints.
type VLESSHandler struct {
	Store      *Storage
	Runner     CommandRunner
	XrayConfig string // path to xray config.json
}

// HandleList returns the server list (GET /api/vless).
func (h *VLESSHandler) HandleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sl, err := h.Store.LoadServers()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(sl)
}

// HandleAdd adds a server from a vless:// URL (POST /api/vless).
func (h *VLESSHandler) HandleAdd(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	vlessURL := r.FormValue("url")
	server, err := ParseVLESSURL(vlessURL)
	if err != nil {
		http.Error(w, "Invalid VLESS URL: "+err.Error(), http.StatusBadRequest)
		return
	}

	sl, err := h.Store.LoadServers()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	server.ID = nextServerID(sl.Servers)
	sl.Servers = append(sl.Servers, server)

	if err := h.Store.SaveServers(sl); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(server)
}

// HandleActivate activates a server (POST /api/vless/{id}/activate).
func (h *VLESSHandler) HandleActivate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/vless/")
	idStr = strings.TrimSuffix(idStr, "/activate")
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

	idx := -1
	for i, s := range sl.Servers {
		if s.ID == id {
			idx = i
			break
		}
	}
	if idx == -1 {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	// Generate and write xray config
	configData, err := GenerateXrayConfig(sl.Servers[idx])
	if err != nil {
		http.Error(w, "Failed to generate config", http.StatusInternalServerError)
		return
	}

	if h.XrayConfig != "" {
		if err := writeFile(h.XrayConfig, configData); err != nil {
			http.Error(w, "Failed to write xray config", http.StatusInternalServerError)
			return
		}
	}

	sl.ActiveIdx = idx
	if err := h.Store.SaveServers(sl); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// Restart xray
	if h.Runner != nil {
		h.Runner.Run("sudo", "systemctl", "restart", "xray")
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "activated"})
}

// HandleDelete removes a server (DELETE /api/vless/{id}).
func (h *VLESSHandler) HandleDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	idStr := strings.TrimPrefix(r.URL.Path, "/api/vless/")
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

	found := false
	newServers := make([]VLESSServer, 0, len(sl.Servers))
	for i, s := range sl.Servers {
		if s.ID == id {
			found = true
			if sl.ActiveIdx == i {
				sl.ActiveIdx = -1
			} else if sl.ActiveIdx > i {
				sl.ActiveIdx--
			}
			continue
		}
		newServers = append(newServers, s)
	}

	if !found {
		http.Error(w, "Server not found", http.StatusNotFound)
		return
	}

	sl.Servers = newServers
	if err := h.Store.SaveServers(sl); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

func nextServerID(servers []VLESSServer) int {
	max := 0
	for _, s := range servers {
		if s.ID > max {
			max = s.ID
		}
	}
	return max + 1
}

// WriteFileFunc is replaceable for testing.
var WriteFileFunc = func(path string, data []byte) error {
	return os.WriteFile(path, data, 0644)
}

func writeFile(path string, data []byte) error {
	return WriteFileFunc(path, data)
}
