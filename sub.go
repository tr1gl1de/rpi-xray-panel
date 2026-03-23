package main

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

// GenerateHWID creates a hardware ID from the MAC address of the given interface.
// Returns the first 16 hex characters of SHA256(MAC).
func GenerateHWID(ifaceName string) (string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return "", fmt.Errorf("interface %s not found: %w", ifaceName, err)
	}
	mac := iface.HardwareAddr.String()
	if mac == "" {
		return "", fmt.Errorf("no MAC address for %s", ifaceName)
	}
	hash := sha256.Sum256([]byte(mac))
	return fmt.Sprintf("%x", hash)[:16], nil
}

// GenerateHWIDFromMAC creates HWID directly from a MAC string (for testing).
func GenerateHWIDFromMAC(mac string) string {
	hash := sha256.Sum256([]byte(mac))
	return fmt.Sprintf("%x", hash)[:16]
}

// FetchSubscription downloads a subscription URL and returns parsed VLESS servers.
func FetchSubscription(subURL, hwid string) ([]VLESSServer, error) {
	return FetchSubscriptionWithClient(http.DefaultClient, subURL, hwid)
}

// FetchSubscriptionWithClient is like FetchSubscription but with a custom HTTP client.
func FetchSubscriptionWithClient(client *http.Client, subURL, hwid string) ([]VLESSServer, error) {
	req, err := http.NewRequest("GET", subURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("x-hwid", hwid)
	req.Header.Set("x-device-os", "Linux")
	req.Header.Set("x-device-model", "RPi Zero W")

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	// Subscription content is base64-encoded list of vless:// URLs separated by newlines
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(body)))
	if err != nil {
		// Try raw content (some providers don't base64 encode)
		decoded = body
	}

	lines := strings.Split(strings.TrimSpace(string(decoded)), "\n")
	var servers []VLESSServer
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || !strings.HasPrefix(line, "vless://") {
			continue
		}
		s, err := ParseVLESSURL(line)
		if err != nil {
			continue // skip unparseable servers
		}
		servers = append(servers, s)
	}

	return servers, nil
}

// SubHandler manages subscription API endpoints.
type SubHandler struct {
	Store     *Storage
	Interface string // network interface for HWID, default "wlan0"
}

// HandleHWID returns the device HWID (GET /api/hwid).
func (h *SubHandler) HandleHWID(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	ifaceName := h.Interface
	if ifaceName == "" {
		ifaceName = "wlan0"
	}
	hwid, err := GenerateHWID(ifaceName)
	if err != nil {
		http.Error(w, "Failed to generate HWID: "+err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"hwid": hwid})
}

// HandleSub processes subscription import (POST /api/sub).
func (h *SubHandler) HandleSub(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	subURL := r.FormValue("url")
	if subURL == "" {
		http.Error(w, "URL is required", http.StatusBadRequest)
		return
	}

	ifaceName := h.Interface
	if ifaceName == "" {
		ifaceName = "wlan0"
	}
	hwid, err := GenerateHWID(ifaceName)
	if err != nil {
		hwid = "unknown"
	}

	servers, err := FetchSubscription(subURL, hwid)
	if err != nil {
		http.Error(w, "Failed to fetch subscription: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Add servers to store
	sl, err := h.Store.LoadServers()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}
	for _, s := range servers {
		s.ID = nextServerID(sl.Servers)
		sl.Servers = append(sl.Servers, s)
	}
	h.Store.SaveServers(sl)

	// Save subscription
	subs, _ := h.Store.LoadSubs()
	subs.Subscriptions = append(subs.Subscriptions, Subscription{
		URL:       subURL,
		UpdatedAt: time.Now(),
	})
	h.Store.SaveSubs(subs)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"added":   len(servers),
		"servers": servers,
	})
}
