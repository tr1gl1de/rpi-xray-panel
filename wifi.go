package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
)

// WiFiNetwork represents a scanned WiFi network.
type WiFiNetwork struct {
	SSID     string `json:"ssid"`
	Signal   int    `json:"signal"`   // signal level in dBm
	Security string `json:"security"` // "open", "WPA", "WPA2", "WEP"
}

// WiFiStatus represents the current WiFi connection status.
type WiFiStatus struct {
	Connected bool   `json:"connected"`
	SSID      string `json:"ssid,omitempty"`
}

// WiFiConnectRequest represents a request to connect to a WiFi network.
type WiFiConnectRequest struct {
	SSID     string `json:"ssid"`
	Password string `json:"password"`
}

// WiFiHandler holds dependencies for WiFi HTTP handlers.
type WiFiHandler struct {
	Runner            CommandRunner
	WpaSupplicantPath string
}

// NewWiFiHandler creates a new WiFiHandler with default settings.
func NewWiFiHandler(runner CommandRunner) *WiFiHandler {
	return &WiFiHandler{
		Runner:            runner,
		WpaSupplicantPath: "/etc/wpa_supplicant/wpa_supplicant.conf",
	}
}

// RegisterRoutes registers WiFi endpoints on the given mux.
func (h *WiFiHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/wifi/scan", h.HandleScan)
	mux.HandleFunc("/api/wifi/status", h.HandleStatus)
	mux.HandleFunc("/api/wifi/connect", h.HandleConnect)
}

// HandleScan handles GET /api/wifi/scan.
func (h *WiFiHandler) HandleScan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	output, err := h.Runner.Run("sudo", "iwlist", "wlan0", "scan")
	if err != nil {
		http.Error(w, fmt.Sprintf("scan failed: %v", err), http.StatusInternalServerError)
		return
	}

	networks := ParseIwlistScan(string(output))

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(networks)
}

// HandleStatus handles GET /api/wifi/status.
func (h *WiFiHandler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	output, err := h.Runner.Run("iwgetid", "-r")
	if err != nil {
		// iwgetid returns error when not connected
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(WiFiStatus{Connected: false})
		return
	}

	ssid := ParseIwgetid(string(output))
	status := WiFiStatus{
		Connected: ssid != "",
		SSID:      ssid,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(status)
}

// HandleConnect handles POST /api/wifi/connect.
func (h *WiFiHandler) HandleConnect(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req WiFiConnectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.SSID == "" {
		http.Error(w, "ssid is required", http.StatusBadRequest)
		return
	}

	conf := GenerateWpaSupplicantConf(req.SSID, req.Password)

	if err := os.WriteFile(h.WpaSupplicantPath, []byte(conf), 0600); err != nil {
		http.Error(w, fmt.Sprintf("failed to write wpa_supplicant.conf: %v", err), http.StatusInternalServerError)
		return
	}

	if _, err := h.Runner.Run("sudo", "wpa_cli", "-i", "wlan0", "reconfigure"); err != nil {
		http.Error(w, fmt.Sprintf("reconfigure failed: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// ParseIwlistScan parses the output of `iwlist wlan0 scan` into a list of WiFiNetwork.
func ParseIwlistScan(output string) []WiFiNetwork {
	var networks []WiFiNetwork

	// Split output into cells (one per network)
	cells := strings.Split(output, "Cell ")
	if len(cells) < 2 {
		return networks
	}

	ssidRe := regexp.MustCompile(`ESSID:"([^"]*)"`)
	signalRe := regexp.MustCompile(`Signal level[=:](-?\d+)\s*dBm`)
	signalQualityRe := regexp.MustCompile(`Signal level[=:](\d+)/100`)

	for _, cell := range cells[1:] {
		var net WiFiNetwork

		// Parse SSID
		if m := ssidRe.FindStringSubmatch(cell); len(m) > 1 {
			net.SSID = m[1]
		}
		if net.SSID == "" {
			continue // skip hidden networks
		}

		// Parse signal level
		if m := signalRe.FindStringSubmatch(cell); len(m) > 1 {
			net.Signal, _ = strconv.Atoi(m[1])
		} else if m := signalQualityRe.FindStringSubmatch(cell); len(m) > 1 {
			q, _ := strconv.Atoi(m[1])
			// Convert quality 0-100 to approximate dBm
			net.Signal = q/2 - 100
		}

		// Parse security
		net.Security = parseSecurityType(cell)

		networks = append(networks, net)
	}

	return networks
}

// parseSecurityType determines the security type from an iwlist scan cell.
func parseSecurityType(cell string) string {
	if strings.Contains(cell, "WPA2") {
		return "WPA2"
	}
	if strings.Contains(cell, "WPA") {
		return "WPA"
	}
	if strings.Contains(cell, "WEP") {
		return "WEP"
	}
	if strings.Contains(cell, "Encryption key:on") {
		return "WEP"
	}
	return "open"
}

// ParseIwgetid parses the output of `iwgetid -r` and returns the current SSID.
func ParseIwgetid(output string) string {
	return strings.TrimSpace(output)
}

// GenerateWpaSupplicantConf generates a wpa_supplicant.conf file content.
func GenerateWpaSupplicantConf(ssid, password string) string {
	var sb strings.Builder
	sb.WriteString("ctrl_interface=DIR=/var/run/wpa_supplicant GROUP=netdev\n")
	sb.WriteString("update_config=1\n")
	sb.WriteString("country=US\n")
	sb.WriteString("\n")
	sb.WriteString("network={\n")
	sb.WriteString(fmt.Sprintf("    ssid=\"%s\"\n", ssid))
	if password != "" {
		sb.WriteString(fmt.Sprintf("    psk=\"%s\"\n", password))
		sb.WriteString("    key_mgmt=WPA-PSK\n")
	} else {
		sb.WriteString("    key_mgmt=NONE\n")
	}
	sb.WriteString("}\n")
	return sb.String()
}
