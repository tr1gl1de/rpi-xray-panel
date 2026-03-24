package main

import (
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"
)

// CommandRunner is an interface for executing system commands.
// Use RealCommandRunner in production; mock it in tests.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

// RealCommandRunner executes commands via exec.Command.
type RealCommandRunner struct{}

func (r *RealCommandRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}

// ExternalIPFetcher fetches the external IP address.
// Abstracted for testability.
type ExternalIPFetcher interface {
	Fetch() (string, error)
}

// HTTPExternalIPFetcher fetches external IP via HTTP (ifconfig.me).
type HTTPExternalIPFetcher struct {
	Client *http.Client
}

func (f *HTTPExternalIPFetcher) Fetch() (string, error) {
	client := f.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	resp, err := client.Get("https://ifconfig.me/ip")
	if err != nil {
		return "", fmt.Errorf("failed to fetch external IP: %w", err)
	}
	defer resp.Body.Close()
	buf := make([]byte, 256)
	n, _ := resp.Body.Read(buf)
	return strings.TrimSpace(string(buf[:n])), nil
}

// AllowedServices is the whitelist of services that can be managed.
var AllowedServices = map[string]bool{
	"xray":     true,
	"redsocks": true,
	"hostapd":  true,
	"dnsmasq":  true,
}

// ServiceStatus represents the status of a single service.
type ServiceStatus struct {
	Name   string `json:"name"`
	Active bool   `json:"active"`
	Status string `json:"status"`
}

// StatusResponse is the JSON response for GET /api/status.
type StatusResponse struct {
	Services   []ServiceStatus `json:"services"`
	ExternalIP string          `json:"external_ip"`
	LocalIP    string          `json:"local_ip"`
}

// ServiceHandler holds dependencies for service-related HTTP handlers.
type ServiceHandler struct {
	Runner    CommandRunner
	IPFetcher ExternalIPFetcher
}

// getServiceStatus checks the status of a single service via systemctl.
func (h *ServiceHandler) getServiceStatus(service string) ServiceStatus {
	out, err := h.Runner.Run("systemctl", "is-active", service)
	status := strings.TrimSpace(string(out))
	if err != nil && status == "" {
		status = "unknown"
	}
	return ServiceStatus{
		Name:   service,
		Active: status == "active",
		Status: status,
	}
}

// getLocalIP returns the IP address of the wlan0 interface.
func (h *ServiceHandler) getLocalIP() string {
	iface, err := net.InterfaceByName("wlan0")
	if err != nil {
		return ""
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return ""
	}
	for _, addr := range addrs {
		if ipNet, ok := addr.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			return ipNet.IP.String()
		}
	}
	return ""
}

// HandleStatus handles GET /api/status.
func (h *ServiceHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var services []ServiceStatus
	for svc := range AllowedServices {
		services = append(services, h.getServiceStatus(svc))
	}

	externalIP, _ := h.IPFetcher.Fetch()
	localIP := h.getLocalIP()

	resp := StatusResponse{
		Services:   services,
		ExternalIP: externalIP,
		LocalIP:    localIP,
	}

	if isHTMX(r) {
		vpnActive := false
		for _, s := range services {
			if s.Name == "xray" && s.Active {
				vpnActive = true
			}
		}
		renderFragment(w, "status", map[string]interface{}{
			"VPNActive":  vpnActive,
			"ExternalIP": externalIP,
			"LocalIP":    localIP,
			"Services":   services,
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// HandleRestart handles POST /api/restart/{service}.
func (h *ServiceHandler) HandleRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service name from path: /api/restart/{service}
	service := strings.TrimPrefix(r.URL.Path, "/api/restart/")
	if service == "" {
		http.Error(w, "service name required", http.StatusBadRequest)
		return
	}

	if !AllowedServices[service] {
		http.Error(w, fmt.Sprintf("service %q is not allowed", service), http.StatusForbidden)
		return
	}

	_, err := h.Runner.Run("sudo", "systemctl", "restart", service)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to restart %s: %v", service, err), http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		renderFragment(w, "restart-result", map[string]string{"Service": service})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"service": service,
	})
}

// HandleLogs handles GET /api/logs/{service}.
// Returns the last 100 lines of journalctl output for the given service.
func (h *ServiceHandler) HandleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract service name from path: /api/logs/{service}
	service := strings.TrimPrefix(r.URL.Path, "/api/logs/")
	if service == "" {
		http.Error(w, "service name required", http.StatusBadRequest)
		return
	}

	if !AllowedServices[service] {
		http.Error(w, fmt.Sprintf("service %q is not allowed", service), http.StatusForbidden)
		return
	}

	out, err := h.Runner.Run("journalctl", "-u", service, "-n", "100", "--no-pager")
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get logs for %s: %v", service, err), http.StatusInternalServerError)
		return
	}

	raw := strings.TrimSpace(string(out))
	var lines []string
	if raw != "" {
		lines = strings.Split(raw, "\n")
	} else {
		lines = []string{}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"service": service,
		"lines":   lines,
	})
}
