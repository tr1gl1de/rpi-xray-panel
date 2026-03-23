package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleHostapdConf = `interface=uap0
driver=nl80211
ssid=MyPiNetwork
hw_mode=g
channel=6
wmm_enabled=0
macaddr_acl=0
auth_algs=1
ignore_broadcast_ssid=0
`

const sampleHostapdConfWPA = `interface=uap0
driver=nl80211
ssid=MyPiNetwork
hw_mode=g
channel=6
wmm_enabled=0
macaddr_acl=0
auth_algs=1
ignore_broadcast_ssid=0
wpa=2
wpa_passphrase=supersecret
wpa_key_mgmt=WPA-PSK
`

func writeHostapdConf(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "hostapd.conf")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write hostapd.conf: %v", err)
	}
	return path
}

func newMockRunner() *MockCommandRunner {
	return &MockCommandRunner{
		Responses: map[string]MockResponse{
			"sudo systemctl restart hostapd": {Output: []byte(""), Err: nil},
		},
	}
}

// --- Task #24: Tests ---

func TestReadConfig_OpenAP(t *testing.T) {
	dir := t.TempDir()
	path := writeHostapdConf(t, dir, sampleHostapdConf)

	mgr := NewAPManager(path, newMockRunner(), nil)
	cfg, err := mgr.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if cfg.SSID != "MyPiNetwork" {
		t.Errorf("expected SSID=MyPiNetwork, got %q", cfg.SSID)
	}
	if cfg.WPAPassphrase != "" {
		t.Errorf("expected empty wpa_passphrase, got %q", cfg.WPAPassphrase)
	}
	if cfg.WPA != "" {
		t.Errorf("expected empty wpa, got %q", cfg.WPA)
	}
}

func TestReadConfig_SecuredAP(t *testing.T) {
	dir := t.TempDir()
	path := writeHostapdConf(t, dir, sampleHostapdConfWPA)

	mgr := NewAPManager(path, newMockRunner(), nil)
	cfg, err := mgr.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if cfg.SSID != "MyPiNetwork" {
		t.Errorf("expected SSID=MyPiNetwork, got %q", cfg.SSID)
	}
	if cfg.WPAPassphrase != "supersecret" {
		t.Errorf("expected wpa_passphrase=supersecret, got %q", cfg.WPAPassphrase)
	}
	if cfg.WPA != "2" {
		t.Errorf("expected wpa=2, got %q", cfg.WPA)
	}
	if cfg.WPAKeyMgmt != "WPA-PSK" {
		t.Errorf("expected wpa_key_mgmt=WPA-PSK, got %q", cfg.WPAKeyMgmt)
	}
}

func TestReadConfig_Comments(t *testing.T) {
	dir := t.TempDir()
	content := "# This is a comment\nssid=TestNet\n# wpa=2\n"
	path := writeHostapdConf(t, dir, content)

	mgr := NewAPManager(path, newMockRunner(), nil)
	cfg, err := mgr.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if cfg.SSID != "TestNet" {
		t.Errorf("expected SSID=TestNet, got %q", cfg.SSID)
	}
	if cfg.WPA != "" {
		t.Errorf("commented wpa should not be parsed, got %q", cfg.WPA)
	}
}

func TestReadConfig_FileNotFound(t *testing.T) {
	mgr := NewAPManager("/nonexistent/hostapd.conf", newMockRunner(), nil)
	_, err := mgr.ReadConfig()
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestSetPassphrase(t *testing.T) {
	dir := t.TempDir()
	path := writeHostapdConf(t, dir, sampleHostapdConf)

	var securedState *bool
	onSecured := func(secured bool) error {
		securedState = &secured
		return nil
	}

	mgr := NewAPManager(path, newMockRunner(), onSecured)
	if err := mgr.SetPassphrase("newpassword123"); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}

	// Verify file was updated
	cfg, err := mgr.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig after SetPassphrase: %v", err)
	}

	if cfg.WPA != "2" {
		t.Errorf("expected wpa=2, got %q", cfg.WPA)
	}
	if cfg.WPAPassphrase != "newpassword123" {
		t.Errorf("expected wpa_passphrase=newpassword123, got %q", cfg.WPAPassphrase)
	}
	if cfg.WPAKeyMgmt != "WPA-PSK" {
		t.Errorf("expected wpa_key_mgmt=WPA-PSK, got %q", cfg.WPAKeyMgmt)
	}

	// Verify callback was called with secured=true
	if securedState == nil || !*securedState {
		t.Error("expected onSecuredChange called with true")
	}

	// Verify other lines are preserved
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "ssid=MyPiNetwork") {
		t.Error("original SSID should be preserved")
	}
	if !strings.Contains(string(data), "channel=6") {
		t.Error("original channel should be preserved")
	}
}

func TestSetPassphrase_UpdateExisting(t *testing.T) {
	dir := t.TempDir()
	path := writeHostapdConf(t, dir, sampleHostapdConfWPA)

	mgr := NewAPManager(path, newMockRunner(), nil)
	if err := mgr.SetPassphrase("updatedpass"); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}

	cfg, err := mgr.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}

	if cfg.WPAPassphrase != "updatedpass" {
		t.Errorf("expected wpa_passphrase=updatedpass, got %q", cfg.WPAPassphrase)
	}
	if cfg.WPA != "2" {
		t.Errorf("expected wpa=2, got %q", cfg.WPA)
	}
}

func TestRemovePassphrase(t *testing.T) {
	dir := t.TempDir()
	path := writeHostapdConf(t, dir, sampleHostapdConfWPA)

	var securedState *bool
	onSecured := func(secured bool) error {
		securedState = &secured
		return nil
	}

	mgr := NewAPManager(path, newMockRunner(), onSecured)
	if err := mgr.RemovePassphrase(); err != nil {
		t.Fatalf("RemovePassphrase: %v", err)
	}

	// Verify file was updated
	cfg, err := mgr.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig after RemovePassphrase: %v", err)
	}

	if cfg.WPA != "" {
		t.Errorf("expected empty wpa, got %q", cfg.WPA)
	}
	if cfg.WPAPassphrase != "" {
		t.Errorf("expected empty wpa_passphrase, got %q", cfg.WPAPassphrase)
	}
	if cfg.WPAKeyMgmt != "" {
		t.Errorf("expected empty wpa_key_mgmt, got %q", cfg.WPAKeyMgmt)
	}

	// Verify callback was called with secured=false
	if securedState == nil || *securedState {
		t.Error("expected onSecuredChange called with false")
	}

	// Verify other lines are preserved
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "ssid=MyPiNetwork") {
		t.Error("original SSID should be preserved")
	}
}

func TestRemovePassphrase_AlreadyOpen(t *testing.T) {
	dir := t.TempDir()
	path := writeHostapdConf(t, dir, sampleHostapdConf)

	mgr := NewAPManager(path, newMockRunner(), nil)
	if err := mgr.RemovePassphrase(); err != nil {
		t.Fatalf("RemovePassphrase on open AP: %v", err)
	}

	cfg, err := mgr.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig: %v", err)
	}
	if cfg.WPA != "" {
		t.Errorf("expected empty wpa, got %q", cfg.WPA)
	}
}

func TestSetPassphrase_RestartsHostapd(t *testing.T) {
	dir := t.TempDir()
	path := writeHostapdConf(t, dir, sampleHostapdConf)

	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{},
	}

	mgr := NewAPManager(path, runner, nil)
	err := mgr.SetPassphrase("testpass")
	// Should fail because mock has no response for restart command
	if err == nil {
		t.Error("expected error when hostapd restart fails")
	}

	// Now add the response and retry
	runner.Responses["sudo systemctl restart hostapd"] = MockResponse{Output: []byte(""), Err: nil}
	// Rewrite the config since it was modified
	writeHostapdConf(t, dir, sampleHostapdConf)
	err = mgr.SetPassphrase("testpass")
	if err != nil {
		t.Fatalf("SetPassphrase should succeed with mock: %v", err)
	}
}

func TestRemovePassphrase_RestartsHostapd(t *testing.T) {
	dir := t.TempDir()
	path := writeHostapdConf(t, dir, sampleHostapdConfWPA)

	runner := &MockCommandRunner{
		Responses: map[string]MockResponse{},
	}

	mgr := NewAPManager(path, runner, nil)
	err := mgr.RemovePassphrase()
	if err == nil {
		t.Error("expected error when hostapd restart fails")
	}
}

func TestAPManager_WithStorage(t *testing.T) {
	// Integration-style test: APManager updates config.json via callback
	dir := t.TempDir()
	confPath := writeHostapdConf(t, dir, sampleHostapdConf)

	storage, err := NewStorage(filepath.Join(dir, "data"))
	if err != nil {
		t.Fatalf("NewStorage: %v", err)
	}

	onSecured := func(secured bool) error {
		cfg, err := storage.LoadConfig()
		if err != nil {
			return err
		}
		cfg.APSecured = secured
		return storage.SaveConfig(cfg)
	}

	mgr := NewAPManager(confPath, newMockRunner(), onSecured)

	// Set passphrase -> ap_secured should be true
	if err := mgr.SetPassphrase("mypassword"); err != nil {
		t.Fatalf("SetPassphrase: %v", err)
	}
	cfg, _ := storage.LoadConfig()
	if !cfg.APSecured {
		t.Error("expected ap_secured=true after SetPassphrase")
	}

	// Remove passphrase -> ap_secured should be false
	if err := mgr.RemovePassphrase(); err != nil {
		t.Fatalf("RemovePassphrase: %v", err)
	}
	cfg, _ = storage.LoadConfig()
	if cfg.APSecured {
		t.Error("expected ap_secured=false after RemovePassphrase")
	}
}
