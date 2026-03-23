package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGenerateHWIDFromMAC(t *testing.T) {
	// Known value: SHA256("aa:bb:cc:dd:ee:ff") first 16 hex chars
	hwid := GenerateHWIDFromMAC("aa:bb:cc:dd:ee:ff")
	if len(hwid) != 16 {
		t.Errorf("HWID length=%d, want 16", len(hwid))
	}
	// Deterministic
	hwid2 := GenerateHWIDFromMAC("aa:bb:cc:dd:ee:ff")
	if hwid != hwid2 {
		t.Error("HWID should be deterministic")
	}
	// Different MAC → different HWID
	hwid3 := GenerateHWIDFromMAC("11:22:33:44:55:66")
	if hwid == hwid3 {
		t.Error("different MACs should produce different HWIDs")
	}
}

func TestFetchSubscription_WithHeaders(t *testing.T) {
	var gotHWID, gotOS, gotModel string

	vlessURL := "vless://uuid1@server1.com:443?security=tls&sni=server1.com#Server1\nvless://uuid2@server2.com:443?security=reality&sni=yahoo.com&pbk=pk&sid=s&fp=ch#Server2"
	encoded := base64.StdEncoding.EncodeToString([]byte(vlessURL))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotHWID = r.Header.Get("x-hwid")
		gotOS = r.Header.Get("x-device-os")
		gotModel = r.Header.Get("x-device-model")
		w.Write([]byte(encoded))
	}))
	defer srv.Close()

	servers, err := FetchSubscriptionWithClient(srv.Client(), srv.URL, "testhwid123")
	if err != nil {
		t.Fatalf("FetchSubscription: %v", err)
	}

	if gotHWID != "testhwid123" {
		t.Errorf("x-hwid=%s, want testhwid123", gotHWID)
	}
	if gotOS != "Linux" {
		t.Errorf("x-device-os=%s, want Linux", gotOS)
	}
	if gotModel != "RPi Zero W" {
		t.Errorf("x-device-model=%s, want RPi Zero W", gotModel)
	}

	if len(servers) != 2 {
		t.Fatalf("expected 2 servers, got %d", len(servers))
	}
	if servers[0].Name != "Server1" {
		t.Errorf("server[0].Name=%s, want Server1", servers[0].Name)
	}
	if servers[1].Security != "reality" {
		t.Errorf("server[1].Security=%s, want reality", servers[1].Security)
	}
}

func TestFetchSubscription_RawContent(t *testing.T) {
	// Some providers return non-base64 content
	raw := "vless://uuid1@server1.com:443?security=tls&sni=server1.com#Raw"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(raw))
	}))
	defer srv.Close()

	servers, err := FetchSubscriptionWithClient(srv.Client(), srv.URL, "hwid")
	if err != nil {
		t.Fatalf("FetchSubscription: %v", err)
	}
	if len(servers) != 1 {
		t.Fatalf("expected 1 server, got %d", len(servers))
	}
	if servers[0].Name != "Raw" {
		t.Errorf("Name=%s, want Raw", servers[0].Name)
	}
}

func TestFetchSubscription_EmptyResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(""))
	}))
	defer srv.Close()

	servers, err := FetchSubscriptionWithClient(srv.Client(), srv.URL, "hwid")
	if err != nil {
		t.Fatalf("FetchSubscription: %v", err)
	}
	if len(servers) != 0 {
		t.Errorf("expected 0 servers for empty response, got %d", len(servers))
	}
}
