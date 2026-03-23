package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Config represents the main application configuration.
type Config struct {
	SetupDone    bool   `json:"setup_done"`
	APSecured    bool   `json:"ap_secured"`
	PasswordHash string `json:"password_hash"`
	Port         int    `json:"port"`
}

// VLESSServer represents a single VLESS server entry.
type VLESSServer struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	UUID     string `json:"uuid"`
	Flow     string `json:"flow,omitempty"`
	Security string `json:"security,omitempty"`
	SNI      string `json:"sni,omitempty"`
	PBK      string `json:"pbk,omitempty"`
	SID      string `json:"sid,omitempty"`
	FP       string `json:"fp,omitempty"`
}

// ServerList holds all VLESS servers and the active index.
type ServerList struct {
	Servers   []VLESSServer `json:"servers"`
	ActiveIdx int           `json:"active_idx"`
}

// Subscription represents a single subscription URL entry.
type Subscription struct {
	URL       string    `json:"url"`
	UpdatedAt time.Time `json:"updated_at"`
}

// SubList holds all subscriptions.
type SubList struct {
	Subscriptions []Subscription `json:"subscriptions"`
}

// Storage provides thread-safe access to JSON config files.
type Storage struct {
	dataDir string
	mu      sync.Mutex
}

// NewStorage creates a Storage with the given data directory.
// Creates the directory if it doesn't exist.
func NewStorage(dataDir string) (*Storage, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, err
	}
	return &Storage{dataDir: dataDir}, nil
}

func (s *Storage) configPath() string  { return filepath.Join(s.dataDir, "config.json") }
func (s *Storage) serversPath() string { return filepath.Join(s.dataDir, "servers.json") }
func (s *Storage) subsPath() string    { return filepath.Join(s.dataDir, "subs.json") }

// LoadConfig reads config.json. Returns default config if file doesn't exist.
func (s *Storage) LoadConfig() (Config, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	cfg := Config{Port: 8080}
	data, err := os.ReadFile(s.configPath())
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return Config{Port: 8080}, err
	}
	return cfg, nil
}

// SaveConfig writes config.json.
func (s *Storage) SaveConfig(cfg Config) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.configPath(), data, 0644)
}

// LoadServers reads servers.json. Returns empty list if file doesn't exist.
func (s *Storage) LoadServers() (ServerList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sl := ServerList{ActiveIdx: -1}
	data, err := os.ReadFile(s.serversPath())
	if err != nil {
		if os.IsNotExist(err) {
			return sl, nil
		}
		return sl, err
	}
	if err := json.Unmarshal(data, &sl); err != nil {
		return ServerList{ActiveIdx: -1}, err
	}
	return sl, nil
}

// SaveServers writes servers.json.
func (s *Storage) SaveServers(sl ServerList) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(sl, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.serversPath(), data, 0644)
}

// LoadSubs reads subs.json. Returns empty list if file doesn't exist.
func (s *Storage) LoadSubs() (SubList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sl := SubList{}
	data, err := os.ReadFile(s.subsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return sl, nil
		}
		return sl, err
	}
	if err := json.Unmarshal(data, &sl); err != nil {
		return SubList{}, err
	}
	return sl, nil
}

// SaveSubs writes subs.json.
func (s *Storage) SaveSubs(sl SubList) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	data, err := json.MarshalIndent(sl, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(s.subsPath(), data, 0644)
}
