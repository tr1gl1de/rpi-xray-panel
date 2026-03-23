package main

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"
)

const sessionCookieName = "rpi_session"
const sessionTTL = 24 * time.Hour

// SessionStore manages in-memory sessions.
type SessionStore struct {
	mu       sync.Mutex
	sessions map[string]time.Time // token -> expiry
}

func NewSessionStore() *SessionStore {
	return &SessionStore{sessions: make(map[string]time.Time)}
}

func (ss *SessionStore) Create() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	token := hex.EncodeToString(b)
	ss.mu.Lock()
	ss.sessions[token] = time.Now().Add(sessionTTL)
	ss.mu.Unlock()
	return token, nil
}

func (ss *SessionStore) Valid(token string) bool {
	ss.mu.Lock()
	defer ss.mu.Unlock()
	expiry, ok := ss.sessions[token]
	if !ok {
		return false
	}
	if time.Now().After(expiry) {
		delete(ss.sessions, token)
		return false
	}
	return true
}

func (ss *SessionStore) Delete(token string) {
	ss.mu.Lock()
	delete(ss.sessions, token)
	ss.mu.Unlock()
}

// Middleware creates the onboarding + auth middleware.
func Middleware(store *Storage, sessions *SessionStore, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		// Static assets pass through (if any in future)
		if path == "/favicon.ico" {
			next.ServeHTTP(w, r)
			return
		}

		cfg, err := store.LoadConfig()
		if err != nil {
			http.Error(w, "Internal error", http.StatusInternalServerError)
			return
		}

		// Onboarding: not set up yet
		if !cfg.SetupDone {
			if path == "/setup" || hasPrefix(path, "/setup/") {
				next.ServeHTTP(w, r)
				return
			}
			http.Redirect(w, r, "/setup", http.StatusFound)
			return
		}

		// Login page is public
		if path == "/login" {
			next.ServeHTTP(w, r)
			return
		}

		// Auth endpoints are public
		if path == "/auth/login" || path == "/auth/logout" {
			next.ServeHTTP(w, r)
			return
		}

		// Check session
		cookie, err := r.Cookie(sessionCookieName)
		if err != nil || !sessions.Valid(cookie.Value) {
			if hasPrefix(path, "/api/") {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			http.Redirect(w, r, "/login", http.StatusFound)
			return
		}

		next.ServeHTTP(w, r)
	})
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
