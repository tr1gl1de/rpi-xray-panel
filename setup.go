package main

import (
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

const bcryptCost = 12

// SetupHandler manages the onboarding wizard.
type SetupHandler struct {
	Store *Storage
	AP    *APManager
}

// HandleSetupPage renders the setup wizard (GET /setup).
func (h *SetupHandler) HandleSetupPage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, _ := h.Store.LoadConfig()
	if cfg.SetupDone {
		http.Redirect(w, r, "/login", http.StatusFound)
		return
	}
	templates.ExecuteTemplate(w, "setup.html", nil)
}

// HandleSetupPassword processes step 1 — panel password (POST /setup/password).
func (h *SetupHandler) HandleSetupPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, _ := h.Store.LoadConfig()
	if cfg.SetupDone {
		http.Error(w, "Setup already completed", http.StatusForbidden)
		return
	}

	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if len(password) < 6 {
		http.Error(w, "Пароль должен быть не менее 6 символов", http.StatusBadRequest)
		return
	}
	if password != confirm {
		http.Error(w, "Пароли не совпадают", http.StatusBadRequest)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	cfg.PasswordHash = string(hash)
	if err := h.Store.SaveConfig(cfg); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<div id="wizard-step2">
		<h2 class="text-lg font-semibold mb-4">Шаг 2 — Пароль WiFi точки доступа</h2>
		<form hx-post="/setup/ap-password" hx-target="#wizard-steps" hx-swap="innerHTML">
			<div class="mb-4">
				<label class="block text-sm font-medium mb-1">Пароль AP (мин. 8 символов, WPA2)</label>
				<input type="password" name="ap_password" class="w-full border rounded px-3 py-2" minlength="8">
			</div>
			<div class="flex gap-2">
				<button type="submit" class="flex-1 bg-blue-600 text-white rounded py-2 hover:bg-blue-700">Установить</button>
				<button type="button" hx-post="/setup/ap-password" hx-target="#wizard-steps" hx-swap="innerHTML" hx-vals='{"skip":"true"}' class="flex-1 bg-gray-300 text-gray-700 rounded py-2 hover:bg-gray-400">Пропустить</button>
			</div>
		</form>
	</div>`))
}

// HandleSetupAPPassword processes step 2 — AP password (POST /setup/ap-password).
func (h *SetupHandler) HandleSetupAPPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, _ := h.Store.LoadConfig()
	if cfg.SetupDone {
		http.Error(w, "Setup already completed", http.StatusForbidden)
		return
	}

	apSecured := false
	skip := r.FormValue("skip")
	if skip != "true" {
		apPassword := r.FormValue("ap_password")
		if len(apPassword) < 8 {
			http.Error(w, "Пароль AP должен быть не менее 8 символов", http.StatusBadRequest)
			return
		}
		if h.AP != nil {
			if err := h.AP.SetPassphrase(apPassword); err != nil {
				http.Error(w, "Ошибка настройки AP: "+err.Error(), http.StatusInternalServerError)
				return
			}
		}
		apSecured = true
	}

	cfg.APSecured = apSecured
	h.Store.SaveConfig(cfg)

	status := "открытая"
	if apSecured {
		status = "защищена паролем"
	}

	w.Header().Set("Content-Type", "text/html")
	w.Write([]byte(`<div id="wizard-step3">
		<h2 class="text-lg font-semibold mb-4">Шаг 3 — Готово!</h2>
		<div class="mb-4 space-y-2">
			<p class="text-green-600">✓ Пароль панели установлен</p>
			<p>Точка доступа: <strong>` + status + `</strong></p>
		</div>
		<form hx-post="/setup/complete" hx-target="#wizard-steps" hx-swap="innerHTML">
			<button type="submit" class="w-full bg-green-600 text-white rounded py-2 hover:bg-green-700">Войти в панель</button>
		</form>
	</div>`))
}

// HandleSetupComplete processes step 3 — finish (POST /setup/complete).
func (h *SetupHandler) HandleSetupComplete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	cfg, _ := h.Store.LoadConfig()
	if cfg.SetupDone {
		http.Error(w, "Setup already completed", http.StatusForbidden)
		return
	}

	cfg.SetupDone = true
	if err := h.Store.SaveConfig(cfg); err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	http.Redirect(w, r, "/login", http.StatusFound)
}
