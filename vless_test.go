package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseVLESSURL_Basic(t *testing.T) {
	raw := "vless://uuid-1234@example.com:443?security=tls&sni=example.com#MyServer"
	s, err := ParseVLESSURL(raw)
	if err != nil {
		t.Fatalf("ParseVLESSURL: %v", err)
	}
	if s.UUID != "uuid-1234" {
		t.Errorf("UUID=%s, want uuid-1234", s.UUID)
	}
	if s.Address != "example.com" {
		t.Errorf("Address=%s, want example.com", s.Address)
	}
	if s.Port != 443 {
		t.Errorf("Port=%d, want 443", s.Port)
	}
	if s.Security != "tls" {
		t.Errorf("Security=%s, want tls", s.Security)
	}
	if s.SNI != "example.com" {
		t.Errorf("SNI=%s, want example.com", s.SNI)
	}
	if s.Name != "MyServer" {
		t.Errorf("Name=%s, want MyServer", s.Name)
	}
}

func TestParseVLESSURL_Reality(t *testing.T) {
	raw := "vless://uuid-5678@1.2.3.4:443?security=reality&sni=yahoo.com&pbk=publickey123&sid=short1&fp=chrome&flow=xtls-rprx-vision#RealityServer"
	s, err := ParseVLESSURL(raw)
	if err != nil {
		t.Fatalf("ParseVLESSURL: %v", err)
	}
	if s.Security != "reality" {
		t.Errorf("Security=%s, want reality", s.Security)
	}
	if s.PBK != "publickey123" {
		t.Errorf("PBK=%s, want publickey123", s.PBK)
	}
	if s.SID != "short1" {
		t.Errorf("SID=%s, want short1", s.SID)
	}
	if s.FP != "chrome" {
		t.Errorf("FP=%s, want chrome", s.FP)
	}
	if s.Flow != "xtls-rprx-vision" {
		t.Errorf("Flow=%s, want xtls-rprx-vision", s.Flow)
	}
}

func TestParseVLESSURL_Invalid(t *testing.T) {
	cases := []string{
		"",
		"http://example.com",
		"vless://",
		"vless://@example.com:443",
	}
	for _, c := range cases {
		_, err := ParseVLESSURL(c)
		if err == nil {
			t.Errorf("expected error for %q", c)
		}
	}
}

func TestParseVLESSURL_DefaultPort(t *testing.T) {
	raw := "vless://uuid@example.com?security=none#Test"
	s, err := ParseVLESSURL(raw)
	if err != nil {
		t.Fatalf("ParseVLESSURL: %v", err)
	}
	if s.Port != 443 {
		t.Errorf("Port=%d, want 443 (default)", s.Port)
	}
}

func TestParseVLESSURL_NoFragment(t *testing.T) {
	raw := "vless://uuid@example.com:8443"
	s, err := ParseVLESSURL(raw)
	if err != nil {
		t.Fatalf("ParseVLESSURL: %v", err)
	}
	if s.Name != "example.com" {
		t.Errorf("Name=%s, want example.com (fallback)", s.Name)
	}
}

func TestGenerateXrayConfig_Reality(t *testing.T) {
	s := VLESSServer{
		Address:  "1.2.3.4",
		Port:     443,
		UUID:     "abc",
		Flow:     "xtls-rprx-vision",
		Security: "reality",
		SNI:      "yahoo.com",
		PBK:      "pk",
		SID:      "sid",
		FP:       "chrome",
	}
	data, err := GenerateXrayConfig(s)
	if err != nil {
		t.Fatalf("GenerateXrayConfig: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	outbounds := config["outbounds"].([]interface{})
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(outbounds))
	}
	ob := outbounds[0].(map[string]interface{})
	if ob["protocol"] != "vless" {
		t.Errorf("protocol=%s, want vless", ob["protocol"])
	}
	ss := ob["streamSettings"].(map[string]interface{})
	if ss["security"] != "reality" {
		t.Errorf("security=%s, want reality", ss["security"])
	}
	rs := ss["realitySettings"].(map[string]interface{})
	if rs["publicKey"] != "pk" {
		t.Errorf("publicKey=%s, want pk", rs["publicKey"])
	}
}

func TestGenerateXrayConfig_TLS(t *testing.T) {
	s := VLESSServer{
		Address:  "example.com",
		Port:     443,
		UUID:     "abc",
		Security: "tls",
		SNI:      "example.com",
	}
	data, err := GenerateXrayConfig(s)
	if err != nil {
		t.Fatalf("GenerateXrayConfig: %v", err)
	}
	var config map[string]interface{}
	json.Unmarshal(data, &config)
	ob := config["outbounds"].([]interface{})[0].(map[string]interface{})
	ss := ob["streamSettings"].(map[string]interface{})
	if _, ok := ss["tlsSettings"]; !ok {
		t.Error("expected tlsSettings for tls security")
	}
}

func TestVLESSHandler_AddAndList(t *testing.T) {
	store := newTestStorage(t)
	h := &VLESSHandler{Store: store}

	// Add server
	form := url.Values{"url": {"vless://uuid-1@example.com:443?security=tls&sni=example.com#Test"}}
	req := httptest.NewRequest("POST", "/api/vless", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleAdd(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("Add: expected 201, got %d, body: %s", w.Code, w.Body.String())
	}

	// List
	req = httptest.NewRequest("GET", "/api/vless", nil)
	w = httptest.NewRecorder()
	h.HandleList(w, req)

	var sl ServerList
	json.NewDecoder(w.Body).Decode(&sl)
	if len(sl.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(sl.Servers))
	}
	if sl.Servers[0].Name != "Test" {
		t.Errorf("Name=%s, want Test", sl.Servers[0].Name)
	}
}

func TestVLESSHandler_Activate(t *testing.T) {
	store := newTestStorage(t)
	xrayPath := filepath.Join(t.TempDir(), "config.json")
	mockRunner := &MockCommandRunner{}
	h := &VLESSHandler{Store: store, Runner: mockRunner, XrayConfig: xrayPath}

	// Add server
	sl := ServerList{
		Servers:   []VLESSServer{{ID: 1, Name: "S1", Address: "1.2.3.4", Port: 443, UUID: "u1", Security: "reality", SNI: "sni", PBK: "pk", SID: "s", FP: "ch"}},
		ActiveIdx: -1,
	}
	store.SaveServers(sl)

	req := httptest.NewRequest("POST", "/api/vless/1/activate", nil)
	w := httptest.NewRecorder()
	h.HandleActivate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Activate: expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	// Check xray config was written
	data, err := os.ReadFile(xrayPath)
	if err != nil {
		t.Fatalf("xray config not written: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatalf("invalid xray config JSON: %v", err)
	}

	// Check active index updated
	loaded, _ := store.LoadServers()
	if loaded.ActiveIdx != 0 {
		t.Errorf("ActiveIdx=%d, want 0", loaded.ActiveIdx)
	}
}

func TestVLESSHandler_Delete(t *testing.T) {
	store := newTestStorage(t)
	h := &VLESSHandler{Store: store}

	sl := ServerList{
		Servers:   []VLESSServer{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}},
		ActiveIdx: 0,
	}
	store.SaveServers(sl)

	req := httptest.NewRequest("DELETE", "/api/vless/1", nil)
	w := httptest.NewRecorder()
	h.HandleDelete(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("Delete: expected 200, got %d", w.Code)
	}

	loaded, _ := store.LoadServers()
	if len(loaded.Servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(loaded.Servers))
	}
	if loaded.Servers[0].Name != "B" {
		t.Errorf("remaining server Name=%s, want B", loaded.Servers[0].Name)
	}
	if loaded.ActiveIdx != -1 {
		t.Errorf("ActiveIdx=%d, want -1 (deleted active)", loaded.ActiveIdx)
	}
}

func TestVLESSHandler_DeleteNotFound(t *testing.T) {
	store := newTestStorage(t)
	h := &VLESSHandler{Store: store}
	store.SaveServers(ServerList{ActiveIdx: -1})

	req := httptest.NewRequest("DELETE", "/api/vless/999", nil)
	w := httptest.NewRecorder()
	h.HandleDelete(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
