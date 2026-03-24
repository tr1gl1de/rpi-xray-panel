package main

import (
	"encoding/json"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

// SettingsHandler manages settings endpoints.
type SettingsHandler struct {
	Store *Storage
	AP    *APManager
}

// HandlePanelPassword processes POST /api/settings/panel-password.
func (h *SettingsHandler) HandlePanelPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")

	cfg, err := h.Store.LoadConfig()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(currentPassword)); err != nil {
		http.Error(w, "Неверный текущий пароль", http.StatusUnauthorized)
		return
	}

	if len(newPassword) < 6 {
		http.Error(w, "Новый пароль должен быть не менее 6 символов", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(newPassword), bcryptCost)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	cfg.PasswordHash = string(hash)
	if err := h.Store.SaveConfig(cfg); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if isHTMX(r) {
		renderFragment(w, "settings-result", map[string]string{"Message": "Пароль панели изменён"})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok", "message": "Пароль панели изменён"})
}

// HandleAPPassword processes POST /api/settings/ap-password.
func (h *SettingsHandler) HandleAPPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	currentPassword := r.FormValue("current_password")
	apPassword := r.FormValue("ap_password")

	cfg, err := h.Store.LoadConfig()
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(cfg.PasswordHash), []byte(currentPassword)); err != nil {
		http.Error(w, "Неверный текущий пароль", http.StatusUnauthorized)
		return
	}

	if apPassword == "" {
		// Remove AP password — make open
		if h.AP != nil {
			if err := h.AP.RemovePassphrase(); err != nil {
				http.Error(w, "Ошибка AP: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		cfg.APSecured = false
	} else {
		if len(apPassword) < 8 {
			http.Error(w, "Пароль AP должен быть не менее 8 символов", http.StatusBadRequest)
			return
		}
		if h.AP != nil {
			if err := h.AP.SetPassphrase(apPassword); err != nil {
				http.Error(w, "Ошибка AP: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		cfg.APSecured = true
	}

	h.Store.SaveConfig(cfg)

	if isHTMX(r) {
		msg := "Пароль AP обновлён"
		if apPassword == "" {
			msg = "AP открыта (пароль удалён)"
		}
		renderFragment(w, "settings-result", map[string]string{"Message": msg})
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}
