package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
)

// Note: CommandRunner and RealCommandRunner are defined in services.go

// APConfig holds parsed hostapd.conf parameters.
type APConfig struct {
	SSID          string
	WPAPassphrase string
	WPA           string // "2" when WPA2 is enabled, "" when open
	WPAKeyMgmt   string
}

// APManager manages hostapd configuration.
type APManager struct {
	confPath string
	runner   CommandRunner
	// onSecuredChange is called when ap_secured status changes.
	// This allows decoupling from the storage layer.
	onSecuredChange func(secured bool) error
}

// NewAPManager creates a new APManager.
func NewAPManager(confPath string, runner CommandRunner, onSecuredChange func(bool) error) *APManager {
	return &APManager{
		confPath:        confPath,
		runner:          runner,
		onSecuredChange: onSecuredChange,
	}
}

// ReadConfig reads and parses hostapd.conf.
func (m *APManager) ReadConfig() (*APConfig, error) {
	f, err := os.Open(m.confPath)
	if err != nil {
		return nil, fmt.Errorf("open hostapd.conf: %w", err)
	}
	defer f.Close()

	cfg := &APConfig{}
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		switch k {
		case "ssid":
			cfg.SSID = v
		case "wpa_passphrase":
			cfg.WPAPassphrase = v
		case "wpa":
			cfg.WPA = v
		case "wpa_key_mgmt":
			cfg.WPAKeyMgmt = v
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read hostapd.conf: %w", err)
	}
	return cfg, nil
}

// SetPassphrase sets the WPA2 passphrase in hostapd.conf, restarts hostapd,
// and updates ap_secured in config.json.
func (m *APManager) SetPassphrase(passphrase string) error {
	lines, err := readLines(m.confPath)
	if err != nil {
		return err
	}

	lines = setOrAddKey(lines, "wpa", "2")
	lines = setOrAddKey(lines, "wpa_passphrase", passphrase)
	lines = setOrAddKey(lines, "wpa_key_mgmt", "WPA-PSK")

	if err := writeLines(m.confPath, lines); err != nil {
		return err
	}

	if err := m.restartHostapd(); err != nil {
		return err
	}

	if m.onSecuredChange != nil {
		return m.onSecuredChange(true)
	}
	return nil
}

// RemovePassphrase removes WPA2 settings from hostapd.conf, restarts hostapd,
// and updates ap_secured in config.json.
func (m *APManager) RemovePassphrase() error {
	lines, err := readLines(m.confPath)
	if err != nil {
		return err
	}

	lines = removeKey(lines, "wpa")
	lines = removeKey(lines, "wpa_passphrase")
	lines = removeKey(lines, "wpa_key_mgmt")

	if err := writeLines(m.confPath, lines); err != nil {
		return err
	}

	if err := m.restartHostapd(); err != nil {
		return err
	}

	if m.onSecuredChange != nil {
		return m.onSecuredChange(false)
	}
	return nil
}

// restartHostapd restarts the hostapd service.
func (m *APManager) restartHostapd() error {
	_, err := m.runner.Run("sudo", "systemctl", "restart", "hostapd")
	return err
}

// readLines reads all lines from a file.
func readLines(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	var lines []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return lines, nil
}

// writeLines writes lines to a file, preserving newlines.
func writeLines(path string, lines []string) error {
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	for _, line := range lines {
		fmt.Fprintln(w, line)
	}
	return w.Flush()
}

// setOrAddKey sets an existing key=value or appends it if not found.
func setOrAddKey(lines []string, key, value string) []string {
	prefix := key + "="
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			lines[i] = key + "=" + value
			return lines
		}
	}
	return append(lines, key+"="+value)
}

// removeKey removes all lines matching key=... from the config.
func removeKey(lines []string, key string) []string {
	prefix := key + "="
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, prefix) {
			result = append(result, line)
		}
	}
	return result
}
