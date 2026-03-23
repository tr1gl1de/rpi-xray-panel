package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func setupSettingsHandler(t *testing.T) *SettingsHandler {
	t.Helper()
	store := newTestStorage(t)
	hash, _ := bcrypt.GenerateFromPassword([]byte("oldpass"), bcrypt.DefaultCost)
	store.SaveConfig(Config{
		SetupDone:    true,
		PasswordHash: string(hash),
		Port:         8080,
	})

	// Create a temp hostapd.conf
	confPath := filepath.Join(t.TempDir(), "hostapd.conf")
	os.WriteFile(confPath, []byte("ssid=RPi-Panel\nchannel=6\n"), 0644)
	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{
			"sudo systemctl restart hostapd": {Output: []byte(""), Err: nil},
		},
	}
	ap := NewAPManager(confPath, runner, func(secured bool) error {
		cfg, _ := store.LoadConfig()
		cfg.APSecured = secured
		return store.SaveConfig(cfg)
	})

	return &SettingsHandler{Store: store, AP: ap}
}

func TestHandlePanelPassword_Success(t *testing.T) {
	h := setupSettingsHandler(t)
	form := url.Values{"current_password": {"oldpass"}, "new_password": {"newpass123"}}
	req := httptest.NewRequest("POST", "/api/settings/panel-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandlePanelPassword(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	cfg, _ := h.Store.LoadConfig()
	if err := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte("newpass123")); err != nil {
		t.Error("new password hash doesn't match")
	}
}

func TestHandlePanelPassword_WrongCurrent(t *testing.T) {
	h := setupSettingsHandler(t)
	form := url.Values{"current_password": {"wrong"}, "new_password": {"newpass123"}}
	req := httptest.NewRequest("POST", "/api/settings/panel-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandlePanelPassword(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestHandlePanelPassword_TooShort(t *testing.T) {
	h := setupSettingsHandler(t)
	form := url.Values{"current_password": {"oldpass"}, "new_password": {"12345"}}
	req := httptest.NewRequest("POST", "/api/settings/panel-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandlePanelPassword(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHandleAPPassword_SetPassword(t *testing.T) {
	h := setupSettingsHandler(t)
	form := url.Values{"current_password": {"oldpass"}, "ap_password": {"wifipass1"}}
	req := httptest.NewRequest("POST", "/api/settings/ap-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleAPPassword(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	cfg, _ := h.Store.LoadConfig()
	if !cfg.APSecured {
		t.Error("ap_secured should be true")
	}
}

func TestHandleAPPassword_RemovePassword(t *testing.T) {
	h := setupSettingsHandler(t)
	// First set it
	h.Store.SaveConfig(Config{SetupDone: true, APSecured: true, PasswordHash: func() string {
		hash, _ := bcrypt.GenerateFromPassword([]byte("oldpass"), bcrypt.DefaultCost)
		return string(hash)
	}()})

	form := url.Values{"current_password": {"oldpass"}, "ap_password": {""}}
	req := httptest.NewRequest("POST", "/api/settings/ap-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleAPPassword(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	cfg, _ := h.Store.LoadConfig()
	if cfg.APSecured {
		t.Error("ap_secured should be false after removing password")
	}
}
