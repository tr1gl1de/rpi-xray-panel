package main

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func newTestStorage(t *testing.T) *Storage {
	t.Helper()
	s, err := NewStorage(t.TempDir())
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	return s
}

func TestLoadConfig_Default(t *testing.T) {
	s := newTestStorage(t)
	cfg, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.SetupDone {
		t.Error("expected setup_done=false by default")
	}
	if cfg.Port != 8080 {
		t.Errorf("expected port=8080, got %d", cfg.Port)
	}
}

func TestSaveLoadConfig(t *testing.T) {
	s := newTestStorage(t)
	cfg := Config{
		SetupDone:    true,
		APSecured:    true,
		PasswordHash: "$2a$12$test",
		Port:         9090,
	}
	if err := s.SaveConfig(cfg); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	loaded, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if loaded != cfg {
		t.Errorf("config mismatch: got %+v, want %+v", loaded, cfg)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	s := newTestStorage(t)
	os.WriteFile(s.configPath(), []byte("{invalid"), 0644)
	_, err := s.LoadConfig()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadServers_Default(t *testing.T) {
	s := newTestStorage(t)
	sl, err := s.LoadServers()
	if err != nil {
		t.Fatalf("LoadServers: %v", err)
	}
	if len(sl.Servers) != 0 {
		t.Error("expected empty servers list")
	}
	if sl.ActiveIdx != -1 {
		t.Errorf("expected active_idx=-1, got %d", sl.ActiveIdx)
	}
}

func TestSaveLoadServers(t *testing.T) {
	s := newTestStorage(t)
	sl := ServerList{
		Servers: []VLESSServer{
			{ID: 1, Name: "Test", Address: "1.2.3.4", Port: 443, UUID: "abc"},
			{ID: 2, Name: "Test2", Address: "5.6.7.8", Port: 443, UUID: "def"},
		},
		ActiveIdx: 0,
	}
	if err := s.SaveServers(sl); err != nil {
		t.Fatalf("SaveServers: %v", err)
	}
	loaded, err := s.LoadServers()
	if err != nil {
		t.Fatalf("LoadServers: %v", err)
	}
	if len(loaded.Servers) != 2 {
		t.Errorf("expected 2 servers, got %d", len(loaded.Servers))
	}
	if loaded.ActiveIdx != 0 {
		t.Errorf("expected active_idx=0, got %d", loaded.ActiveIdx)
	}
}

func TestDeleteServer(t *testing.T) {
	s := newTestStorage(t)
	sl := ServerList{
		Servers:   []VLESSServer{{ID: 1, Name: "A"}, {ID: 2, Name: "B"}},
		ActiveIdx: 0,
	}
	s.SaveServers(sl)

	// Remove server at index 0
	sl.Servers = sl.Servers[1:]
	sl.ActiveIdx = -1
	s.SaveServers(sl)

	loaded, _ := s.LoadServers()
	if len(loaded.Servers) != 1 {
		t.Errorf("expected 1 server after delete, got %d", len(loaded.Servers))
	}
	if loaded.Servers[0].Name != "B" {
		t.Errorf("expected server B, got %s", loaded.Servers[0].Name)
	}
}

func TestLoadServers_InvalidJSON(t *testing.T) {
	s := newTestStorage(t)
	os.WriteFile(s.serversPath(), []byte("not json"), 0644)
	_, err := s.LoadServers()
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestSaveLoadSubs(t *testing.T) {
	s := newTestStorage(t)
	now := time.Now().Truncate(time.Second)
	sl := SubList{
		Subscriptions: []Subscription{
			{URL: "https://example.com/sub", UpdatedAt: now},
		},
	}
	if err := s.SaveSubs(sl); err != nil {
		t.Fatalf("SaveSubs: %v", err)
	}
	loaded, err := s.LoadSubs()
	if err != nil {
		t.Fatalf("LoadSubs: %v", err)
	}
	if len(loaded.Subscriptions) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(loaded.Subscriptions))
	}
	if loaded.Subscriptions[0].URL != "https://example.com/sub" {
		t.Errorf("URL mismatch")
	}
}

func TestLoadSubs_Default(t *testing.T) {
	s := newTestStorage(t)
	sl, err := s.LoadSubs()
	if err != nil {
		t.Fatalf("LoadSubs: %v", err)
	}
	if len(sl.Subscriptions) != 0 {
		t.Error("expected empty subscriptions")
	}
}

func TestStorage_ConcurrentAccess(t *testing.T) {
	s := newTestStorage(t)
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			cfg := Config{Port: 8080 + i, SetupDone: i%2 == 0}
			s.SaveConfig(cfg)
			s.LoadConfig()
		}(i)
	}
	wg.Wait()

	// Verify we can still read valid config
	_, err := s.LoadConfig()
	if err != nil {
		t.Fatalf("LoadConfig after concurrent writes: %v", err)
	}
}

func TestNewStorage_CreatesDir(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "dir")
	s, err := NewStorage(dir)
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}
	info, err := os.Stat(s.dataDir)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if !info.IsDir() {
		t.Error("expected directory")
	}
}
