package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- ParseIwlistScan tests ---

const iwlistScanOutput = `wlan0     Scan completed :
          Cell 01 - Address: AA:BB:CC:DD:EE:01
                    Channel:6
                    Frequency:2.437 GHz (Channel 6)
                    Quality=70/70  Signal level=-33 dBm
                    Encryption key:on
                    ESSID:"MyHomeWiFi"
                    IE: IEEE 802.11i/WPA2 Version 1
                        Group Cipher : CCMP
                        Pairwise Ciphers (1) : CCMP
                        Authentication Suites (1) : PSK
          Cell 02 - Address: AA:BB:CC:DD:EE:02
                    Channel:1
                    Frequency:2.412 GHz (Channel 1)
                    Quality=50/70  Signal level=-60 dBm
                    Encryption key:on
                    ESSID:"NeighborNet"
                    IE: WPA Version 1
                        Group Cipher : TKIP
                        Pairwise Ciphers (1) : TKIP
                        Authentication Suites (1) : PSK
          Cell 03 - Address: AA:BB:CC:DD:EE:03
                    Channel:11
                    Frequency:2.462 GHz (Channel 11)
                    Quality=30/70  Signal level=-80 dBm
                    Encryption key:off
                    ESSID:"OpenCafe"
`

func TestParseIwlistScan_Basic(t *testing.T) {
	networks := ParseIwlistScan(iwlistScanOutput)

	if len(networks) != 3 {
		t.Fatalf("expected 3 networks, got %d", len(networks))
	}

	tests := []struct {
		ssid     string
		signal   int
		security string
	}{
		{"MyHomeWiFi", -33, "WPA2"},
		{"NeighborNet", -60, "WPA"},
		{"OpenCafe", -80, "open"},
	}

	for i, tt := range tests {
		if networks[i].SSID != tt.ssid {
			t.Errorf("network[%d] SSID: expected %q, got %q", i, tt.ssid, networks[i].SSID)
		}
		if networks[i].Signal != tt.signal {
			t.Errorf("network[%d] Signal: expected %d, got %d", i, tt.signal, networks[i].Signal)
		}
		if networks[i].Security != tt.security {
			t.Errorf("network[%d] Security: expected %q, got %q", i, tt.security, networks[i].Security)
		}
	}
}

func TestParseIwlistScan_Empty(t *testing.T) {
	networks := ParseIwlistScan("")
	if len(networks) != 0 {
		t.Fatalf("expected 0 networks for empty output, got %d", len(networks))
	}
}

func TestParseIwlistScan_NoResults(t *testing.T) {
	output := "wlan0     No scan results\n"
	networks := ParseIwlistScan(output)
	if len(networks) != 0 {
		t.Fatalf("expected 0 networks, got %d", len(networks))
	}
}

func TestParseIwlistScan_HiddenSSID(t *testing.T) {
	output := `wlan0     Scan completed :
          Cell 01 - Address: AA:BB:CC:DD:EE:01
                    Quality=70/70  Signal level=-33 dBm
                    Encryption key:on
                    ESSID:""
                    IE: IEEE 802.11i/WPA2 Version 1
          Cell 02 - Address: AA:BB:CC:DD:EE:02
                    Quality=50/70  Signal level=-60 dBm
                    Encryption key:on
                    ESSID:"VisibleNet"
                    IE: IEEE 802.11i/WPA2 Version 1
`
	networks := ParseIwlistScan(output)
	if len(networks) != 1 {
		t.Fatalf("expected 1 network (hidden skipped), got %d", len(networks))
	}
	if networks[0].SSID != "VisibleNet" {
		t.Errorf("expected SSID VisibleNet, got %q", networks[0].SSID)
	}
}

func TestParseIwlistScan_WEP(t *testing.T) {
	output := `wlan0     Scan completed :
          Cell 01 - Address: AA:BB:CC:DD:EE:01
                    Quality=40/70  Signal level=-55 dBm
                    Encryption key:on
                    ESSID:"OldRouter"
`
	networks := ParseIwlistScan(output)
	if len(networks) != 1 {
		t.Fatalf("expected 1 network, got %d", len(networks))
	}
	// Encryption key:on without WPA/WPA2 → WEP
	if networks[0].Security != "WEP" {
		t.Errorf("expected security WEP, got %q", networks[0].Security)
	}
}

// --- ParseIwgetid tests ---

func TestParseIwgetid_Connected(t *testing.T) {
	ssid := ParseIwgetid("MyHomeWiFi\n")
	if ssid != "MyHomeWiFi" {
		t.Errorf("expected MyHomeWiFi, got %q", ssid)
	}
}

func TestParseIwgetid_WithSpaces(t *testing.T) {
	ssid := ParseIwgetid("  My WiFi Network  \n")
	if ssid != "My WiFi Network" {
		t.Errorf("expected 'My WiFi Network', got %q", ssid)
	}
}

func TestParseIwgetid_Empty(t *testing.T) {
	ssid := ParseIwgetid("")
	if ssid != "" {
		t.Errorf("expected empty string, got %q", ssid)
	}
}

func TestParseIwgetid_WhitespaceOnly(t *testing.T) {
	ssid := ParseIwgetid("  \n  ")
	if ssid != "" {
		t.Errorf("expected empty string, got %q", ssid)
	}
}

// --- GenerateWpaSupplicantConf tests ---

func TestGenerateWpaSupplicantConf_WithPassword(t *testing.T) {
	conf := GenerateWpaSupplicantConf("TestNetwork", "secret123")

	if !strings.Contains(conf, `ssid="TestNetwork"`) {
		t.Error("conf missing ssid")
	}
	if !strings.Contains(conf, `psk="secret123"`) {
		t.Error("conf missing psk")
	}
	if !strings.Contains(conf, "key_mgmt=WPA-PSK") {
		t.Error("conf missing key_mgmt=WPA-PSK")
	}
	if !strings.Contains(conf, "ctrl_interface=") {
		t.Error("conf missing ctrl_interface")
	}
	if !strings.Contains(conf, "network={") {
		t.Error("conf missing network block")
	}
}

func TestGenerateWpaSupplicantConf_OpenNetwork(t *testing.T) {
	conf := GenerateWpaSupplicantConf("OpenCafe", "")

	if !strings.Contains(conf, `ssid="OpenCafe"`) {
		t.Error("conf missing ssid")
	}
	if strings.Contains(conf, "psk=") {
		t.Error("conf should not contain psk for open network")
	}
	if !strings.Contains(conf, "key_mgmt=NONE") {
		t.Error("conf missing key_mgmt=NONE for open network")
	}
}

// --- HTTP handler tests ---

func TestHandleScan_Success(t *testing.T) {
	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{
			"sudo iwlist wlan0 scan": {Output: []byte(iwlistScanOutput), Err: nil},
		},
	}
	h := NewWiFiHandler(runner)

	req := httptest.NewRequest(http.MethodGet, "/api/wifi/scan", nil)
	w := httptest.NewRecorder()

	h.HandleScan(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var networks []WiFiNetwork
	if err := json.NewDecoder(w.Body).Decode(&networks); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if len(networks) != 3 {
		t.Fatalf("expected 3 networks, got %d", len(networks))
	}
}

func TestHandleScan_MethodNotAllowed(t *testing.T) {
	h := NewWiFiHandler(&MockCommandRunner{Responses: map[string]MockResponse{}})

	req := httptest.NewRequest(http.MethodPost, "/api/wifi/scan", nil)
	w := httptest.NewRecorder()

	h.HandleScan(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleStatus_Connected(t *testing.T) {
	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{
			"iwgetid -r": {Output: []byte("MyHomeWiFi\n"), Err: nil},
		},
	}
	h := NewWiFiHandler(runner)

	req := httptest.NewRequest(http.MethodGet, "/api/wifi/status", nil)
	w := httptest.NewRecorder()

	h.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status WiFiStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if !status.Connected {
		t.Error("expected connected=true")
	}
	if status.SSID != "MyHomeWiFi" {
		t.Errorf("expected SSID MyHomeWiFi, got %q", status.SSID)
	}
}

func TestHandleStatus_Disconnected(t *testing.T) {
	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{
			// iwgetid returns error when not connected
		},
	}
	h := NewWiFiHandler(runner)

	req := httptest.NewRequest(http.MethodGet, "/api/wifi/status", nil)
	w := httptest.NewRecorder()

	h.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status WiFiStatus
	if err := json.NewDecoder(w.Body).Decode(&status); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if status.Connected {
		t.Error("expected connected=false")
	}
	if status.SSID != "" {
		t.Errorf("expected empty SSID, got %q", status.SSID)
	}
}

func TestHandleConnect_Success(t *testing.T) {
	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{
			"sudo wpa_cli -i wlan0 reconfigure": {Output: []byte("OK\n"), Err: nil},
		},
	}
	h := NewWiFiHandler(runner)

	// Use a temp file for wpa_supplicant.conf
	tmpDir := t.TempDir()
	h.WpaSupplicantPath = filepath.Join(tmpDir, "wpa_supplicant.conf")

	body := `{"ssid":"TestNet","password":"pass1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/wifi/connect", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.HandleConnect(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	// Verify wpa_supplicant.conf was written
	confBytes, err := os.ReadFile(h.WpaSupplicantPath)
	if err != nil {
		t.Fatalf("failed to read wpa_supplicant.conf: %v", err)
	}
	conf := string(confBytes)
	if !strings.Contains(conf, `ssid="TestNet"`) {
		t.Error("conf missing ssid")
	}
	if !strings.Contains(conf, `psk="pass1234"`) {
		t.Error("conf missing psk")
	}
}

func TestHandleConnect_MissingSSID(t *testing.T) {
	h := NewWiFiHandler(&MockCommandRunner{Responses: map[string]MockResponse{}})

	body := `{"ssid":"","password":"pass1234"}`
	req := httptest.NewRequest(http.MethodPost, "/api/wifi/connect", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.HandleConnect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleConnect_InvalidJSON(t *testing.T) {
	h := NewWiFiHandler(&MockCommandRunner{Responses: map[string]MockResponse{}})

	req := httptest.NewRequest(http.MethodPost, "/api/wifi/connect", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	h.HandleConnect(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleConnect_MethodNotAllowed(t *testing.T) {
	h := NewWiFiHandler(&MockCommandRunner{Responses: map[string]MockResponse{}})

	req := httptest.NewRequest(http.MethodGet, "/api/wifi/connect", nil)
	w := httptest.NewRecorder()

	h.HandleConnect(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleConnect_OpenNetwork(t *testing.T) {
	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{
			"sudo wpa_cli -i wlan0 reconfigure": {Output: []byte("OK\n"), Err: nil},
		},
	}
	h := NewWiFiHandler(runner)

	tmpDir := t.TempDir()
	h.WpaSupplicantPath = filepath.Join(tmpDir, "wpa_supplicant.conf")

	body := `{"ssid":"OpenCafe","password":""}`
	req := httptest.NewRequest(http.MethodPost, "/api/wifi/connect", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	h.HandleConnect(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d; body: %s", w.Code, w.Body.String())
	}

	confBytes, err := os.ReadFile(h.WpaSupplicantPath)
	if err != nil {
		t.Fatalf("failed to read wpa_supplicant.conf: %v", err)
	}
	conf := string(confBytes)
	if !strings.Contains(conf, "key_mgmt=NONE") {
		t.Error("open network should have key_mgmt=NONE")
	}
	if strings.Contains(conf, "psk=") {
		t.Error("open network should not have psk")
	}
}
