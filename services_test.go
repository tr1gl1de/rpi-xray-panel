package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// MockCommandRunner is a test mock for CommandRunner.
type MockCommandRunner struct {
	// Responses maps "name arg1 arg2 ..." to (output, error).
	Responses map[string]MockResponse
}

type MockResponse struct {
	Output []byte
	Err    error
}

func (m *MockCommandRunner) Run(name string, args ...string) ([]byte, error) {
	key := name
	for _, a := range args {
		key += " " + a
	}
	if resp, ok := m.Responses[key]; ok {
		return resp.Output, resp.Err
	}
	return []byte(""), fmt.Errorf("mock: no response for %q", key)
}

// MockExternalIPFetcher returns a fixed IP.
type MockExternalIPFetcher struct {
	IP  string
	Err error
}

func (m *MockExternalIPFetcher) Fetch() (string, error) {
	return m.IP, m.Err
}

func newTestHandler() (*ServiceHandler, *MockCommandRunner) {
	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{
			"systemctl is-active xray":     {Output: []byte("active\n"), Err: nil},
			"systemctl is-active redsocks": {Output: []byte("inactive\n"), Err: fmt.Errorf("exit 3")},
			"systemctl is-active hostapd":  {Output: []byte("active\n"), Err: nil},
			"systemctl is-active dnsmasq":  {Output: []byte("active\n"), Err: nil},
			"systemctl is-active uap0":     {Output: []byte("failed\n"), Err: fmt.Errorf("exit 3")},
		},
	}
	fetcher := &MockExternalIPFetcher{IP: "1.2.3.4", Err: nil}
	h := &ServiceHandler{Runner: runner, IPFetcher: fetcher}
	return h, runner
}

func TestHandleStatus_JSONStructure(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()

	h.HandleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Fatalf("expected Content-Type application/json, got %q", ct)
	}

	var resp StatusResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(resp.Services) != len(AllowedServices) {
		t.Fatalf("expected %d services, got %d", len(AllowedServices), len(resp.Services))
	}

	if resp.ExternalIP != "1.2.3.4" {
		t.Errorf("expected external IP 1.2.3.4, got %q", resp.ExternalIP)
	}

	// Check that xray is active
	found := false
	for _, svc := range resp.Services {
		if svc.Name == "xray" {
			found = true
			if !svc.Active {
				t.Errorf("expected xray to be active")
			}
			if svc.Status != "active" {
				t.Errorf("expected xray status 'active', got %q", svc.Status)
			}
		}
		if svc.Name == "redsocks" {
			if svc.Active {
				t.Errorf("expected redsocks to be inactive")
			}
		}
	}
	if !found {
		t.Errorf("xray not found in services")
	}
}

func TestHandleStatus_MethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	w := httptest.NewRecorder()

	h.HandleStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleRestart_AllowedService(t *testing.T) {
	h, runner := newTestHandler()
	runner.Responses["sudo systemctl restart xray"] = MockResponse{Output: []byte(""), Err: nil}

	req := httptest.NewRequest(http.MethodPost, "/api/restart/xray", nil)
	w := httptest.NewRecorder()

	h.HandleRestart(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ok" {
		t.Errorf("expected status ok, got %q", resp["status"])
	}
	if resp["service"] != "xray" {
		t.Errorf("expected service xray, got %q", resp["service"])
	}
}

func TestHandleRestart_ForbiddenService(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/restart/ssh", nil)
	w := httptest.NewRecorder()

	h.HandleRestart(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestHandleRestart_EmptyService(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/restart/", nil)
	w := httptest.NewRecorder()

	h.HandleRestart(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleRestart_MethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/restart/xray", nil)
	w := httptest.NewRecorder()

	h.HandleRestart(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestWhitelist_AllServices(t *testing.T) {
	expected := []string{"xray", "redsocks", "hostapd", "dnsmasq"}
	for _, svc := range expected {
		if !AllowedServices[svc] {
			t.Errorf("expected %q to be in AllowedServices", svc)
		}
	}
}

func TestWhitelist_RejectUnknown(t *testing.T) {
	rejected := []string{"ssh", "nginx", "mysql", "sshd", "cron"}
	for _, svc := range rejected {
		if AllowedServices[svc] {
			t.Errorf("expected %q to NOT be in AllowedServices", svc)
		}
	}
}

func TestHandleRestart_AllWhitelistedServices(t *testing.T) {
	for svc := range AllowedServices {
		h, runner := newTestHandler()
		runner.Responses[fmt.Sprintf("sudo systemctl restart %s", svc)] = MockResponse{Output: []byte(""), Err: nil}

		req := httptest.NewRequest(http.MethodPost, "/api/restart/"+svc, nil)
		w := httptest.NewRecorder()

		h.HandleRestart(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("restart %s: expected 200, got %d", svc, w.Code)
		}
	}
}

// --- Logs tests (Task #11) ---

func TestHandleLogs_AllowedService(t *testing.T) {
	h, runner := newTestHandler()
	runner.Responses["journalctl -u xray -n 100 --no-pager"] = MockResponse{
		Output: []byte("line1\nline2\nline3\n"),
		Err:    nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/xray", nil)
	w := httptest.NewRecorder()

	h.HandleLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if resp["service"] != "xray" {
		t.Errorf("expected service xray, got %v", resp["service"])
	}
	lines, ok := resp["lines"].([]interface{})
	if !ok {
		t.Fatalf("expected lines to be array, got %T", resp["lines"])
	}
	if len(lines) != 3 {
		t.Errorf("expected 3 lines, got %d", len(lines))
	}
}

func TestHandleLogs_ForbiddenService(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/logs/ssh", nil)
	w := httptest.NewRecorder()

	h.HandleLogs(w, req)

	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestHandleLogs_EmptyService(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodGet, "/api/logs/", nil)
	w := httptest.NewRecorder()

	h.HandleLogs(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleLogs_MethodNotAllowed(t *testing.T) {
	h, _ := newTestHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/logs/xray", nil)
	w := httptest.NewRecorder()

	h.HandleLogs(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", w.Code)
	}
}

func TestHandleLogs_EmptyOutput(t *testing.T) {
	h, runner := newTestHandler()
	runner.Responses["journalctl -u dnsmasq -n 100 --no-pager"] = MockResponse{
		Output: []byte(""),
		Err:    nil,
	}

	req := httptest.NewRequest(http.MethodGet, "/api/logs/dnsmasq", nil)
	w := httptest.NewRecorder()

	h.HandleLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	lines, ok := resp["lines"].([]interface{})
	if !ok {
		t.Fatalf("expected lines to be array, got %T", resp["lines"])
	}
	if len(lines) != 0 {
		t.Errorf("expected 0 lines, got %d", len(lines))
	}
}

func TestHandleLogs_WhitelistOnly(t *testing.T) {
	forbidden := []string{"ssh", "nginx", "mysql", "sshd", "cron", "systemd"}
	for _, svc := range forbidden {
		h, _ := newTestHandler()

		req := httptest.NewRequest(http.MethodGet, "/api/logs/"+svc, nil)
		w := httptest.NewRecorder()

		h.HandleLogs(w, req)

		if w.Code != http.StatusForbidden {
			t.Errorf("logs %s: expected 403, got %d", svc, w.Code)
		}
	}
}
