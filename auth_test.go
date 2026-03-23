package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func dummyHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
}

func TestMiddleware_SetupNotDone_RedirectToSetup(t *testing.T) {
	s := newTestStorage(t)
	sessions := NewSessionStore()
	handler := Middleware(s, sessions, dummyHandler())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/setup" {
		t.Fatalf("expected redirect to /setup, got %s", loc)
	}
}

func TestMiddleware_SetupNotDone_SetupPageAllowed(t *testing.T) {
	s := newTestStorage(t)
	sessions := NewSessionStore()
	handler := Middleware(s, sessions, dummyHandler())

	req := httptest.NewRequest("GET", "/setup", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /setup, got %d", w.Code)
	}
}

func TestMiddleware_SetupNotDone_SetupPostAllowed(t *testing.T) {
	s := newTestStorage(t)
	sessions := NewSessionStore()
	handler := Middleware(s, sessions, dummyHandler())

	req := httptest.NewRequest("POST", "/setup/password", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /setup/password, got %d", w.Code)
	}
}

func TestMiddleware_SetupDone_NoSession_RedirectToLogin(t *testing.T) {
	s := newTestStorage(t)
	s.SaveConfig(Config{SetupDone: true, Port: 8080})
	sessions := NewSessionStore()
	handler := Middleware(s, sessions, dummyHandler())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", w.Code)
	}
	if loc := w.Header().Get("Location"); loc != "/login" {
		t.Fatalf("expected redirect to /login, got %s", loc)
	}
}

func TestMiddleware_SetupDone_LoginPageAllowed(t *testing.T) {
	s := newTestStorage(t)
	s.SaveConfig(Config{SetupDone: true, Port: 8080})
	sessions := NewSessionStore()
	handler := Middleware(s, sessions, dummyHandler())

	req := httptest.NewRequest("GET", "/login", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for /login, got %d", w.Code)
	}
}

func TestMiddleware_SetupDone_ValidSession(t *testing.T) {
	s := newTestStorage(t)
	s.SaveConfig(Config{SetupDone: true, Port: 8080})
	sessions := NewSessionStore()
	token, _ := sessions.Create()
	handler := Middleware(s, sessions, dummyHandler())

	req := httptest.NewRequest("GET", "/", nil)
	req.AddCookie(&http.Cookie{Name: sessionCookieName, Value: token})
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestMiddleware_API_NoSession_Unauthorized(t *testing.T) {
	s := newTestStorage(t)
	s.SaveConfig(Config{SetupDone: true, Port: 8080})
	sessions := NewSessionStore()
	handler := Middleware(s, sessions, dummyHandler())

	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", w.Code)
	}
}

func TestSessionStore_Expiry(t *testing.T) {
	ss := &SessionStore{sessions: make(map[string]time.Time)}
	ss.sessions["expired"] = time.Now().Add(-time.Hour)
	if ss.Valid("expired") {
		t.Error("expected expired session to be invalid")
	}
}

func TestSessionStore_Delete(t *testing.T) {
	ss := NewSessionStore()
	token, _ := ss.Create()
	if !ss.Valid(token) {
		t.Fatal("token should be valid")
	}
	ss.Delete(token)
	if ss.Valid(token) {
		t.Error("token should be invalid after delete")
	}
}
