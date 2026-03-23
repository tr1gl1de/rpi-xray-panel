package main

import (
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func init() {
	if templates == nil {
		templates = template.Must(template.ParseFS(templateFS, "templates/*.html"))
	}
}

func newSetupHandler(t *testing.T) *SetupHandler {
	t.Helper()
	store := newTestStorage(t)
	return &SetupHandler{Store: store, AP: nil}
}

func TestSetupPage_ShowsWizard(t *testing.T) {
	h := newSetupHandler(t)
	req := httptest.NewRequest("GET", "/setup", nil)
	w := httptest.NewRecorder()
	h.HandleSetupPage(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestSetupPage_RedirectsWhenDone(t *testing.T) {
	h := newSetupHandler(t)
	h.Store.SaveConfig(Config{SetupDone: true, Port: 8080})

	req := httptest.NewRequest("GET", "/setup", nil)
	w := httptest.NewRecorder()
	h.HandleSetupPage(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
}

func TestSetupPassword_TooShort(t *testing.T) {
	h := newSetupHandler(t)
	form := url.Values{"password": {"12345"}, "confirm": {"12345"}}
	req := httptest.NewRequest("POST", "/setup/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetupPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetupPassword_Mismatch(t *testing.T) {
	h := newSetupHandler(t)
	form := url.Values{"password": {"123456"}, "confirm": {"654321"}}
	req := httptest.NewRequest("POST", "/setup/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetupPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetupPassword_Valid(t *testing.T) {
	h := newSetupHandler(t)
	form := url.Values{"password": {"secretpass"}, "confirm": {"secretpass"}}
	req := httptest.NewRequest("POST", "/setup/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetupPassword(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d, body: %s", w.Code, w.Body.String())
	}

	cfg, _ := h.Store.LoadConfig()
	if cfg.PasswordHash == "" {
		t.Error("password hash not saved")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte("secretpass")); err != nil {
		t.Error("password hash doesn't match")
	}
}

func TestSetupPassword_AlreadyDone(t *testing.T) {
	h := newSetupHandler(t)
	h.Store.SaveConfig(Config{SetupDone: true, Port: 8080})

	form := url.Values{"password": {"secretpass"}, "confirm": {"secretpass"}}
	req := httptest.NewRequest("POST", "/setup/password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetupPassword(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestSetupAPPassword_TooShort(t *testing.T) {
	h := newSetupHandler(t)
	form := url.Values{"ap_password": {"1234567"}}
	req := httptest.NewRequest("POST", "/setup/ap-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetupAPPassword(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSetupAPPassword_Skip(t *testing.T) {
	h := newSetupHandler(t)
	form := url.Values{"skip": {"true"}}
	req := httptest.NewRequest("POST", "/setup/ap-password", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleSetupAPPassword(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	cfg, _ := h.Store.LoadConfig()
	if cfg.APSecured {
		t.Error("AP should not be secured when skipped")
	}
}

func TestSetupComplete(t *testing.T) {
	h := newSetupHandler(t)
	req := httptest.NewRequest("POST", "/setup/complete", nil)
	w := httptest.NewRecorder()
	h.HandleSetupComplete(w, req)
	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	cfg, _ := h.Store.LoadConfig()
	if !cfg.SetupDone {
		t.Error("setup_done should be true")
	}
}

func TestSetupComplete_AlreadyDone(t *testing.T) {
	h := newSetupHandler(t)
	h.Store.SaveConfig(Config{SetupDone: true, Port: 8080})

	req := httptest.NewRequest("POST", "/setup/complete", nil)
	w := httptest.NewRecorder()
	h.HandleSetupComplete(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}
