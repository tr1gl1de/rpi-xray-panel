package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func setupLoginHandler(t *testing.T, password string) (*LoginHandler, *Storage) {
	t.Helper()
	store := newTestStorage(t)
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("bcrypt: %v", err)
	}
	store.SaveConfig(Config{
		SetupDone:    true,
		PasswordHash: string(hash),
		Port:         8080,
	})
	sessions := NewSessionStore()
	return &LoginHandler{Store: store, Sessions: sessions}, store
}

func TestHandleLogin_CorrectPassword(t *testing.T) {
	h, _ := setupLoginHandler(t, "secret123")

	form := url.Values{"password": {"secret123"}}
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	h.HandleLogin(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/" {
		t.Fatalf("expected redirect to /, got %s", loc)
	}
	cookies := w.Result().Cookies()
	found := false
	for _, c := range cookies {
		if c.Name == sessionCookieName {
			found = true
			if !c.HttpOnly {
				t.Error("cookie should be HttpOnly")
			}
			if c.SameSite != http.SameSiteStrictMode {
				t.Error("cookie should be SameSite=Strict")
			}
		}
	}
	if !found {
		t.Error("session cookie not set")
	}
}

func TestHandleLogin_WrongPassword(t *testing.T) {
	h, _ := setupLoginHandler(t, "secret123")

	form := url.Values{"password": {"wrong"}}
	req := httptest.NewRequest("POST", "/auth/login", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()

	start := time.Now()
	h.HandleLogin(w, req)
	elapsed := time.Since(start)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
	if elapsed < 2*time.Second {
		t.Errorf("expected ≥2s delay, got %v", elapsed)
	}
}

func TestHandleLogout(t *testing.T) {
	h, _ := setupLoginHandler(t, "secret123")
	token, _ := h.Sessions.Create()

	req := httptest.NewRequest("POST", "/auth/logout", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w := httptest.NewRecorder()
	h.HandleLogout(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}

	if h.Sessions.Valid(token) {
		t.Error("session should be invalidated after logout")
	}

	cookies := w.Result().Cookies()
	for _, c := range cookies {
		if c.Name == sessionCookieName && c.MaxAge != -1 {
			t.Error("cookie should be expired")
		}
	}
}
