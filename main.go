package main

import (
	"embed"
	"flag"
	"fmt"
	"html/template"
	"log"
	"net/http"
)

//go:embed templates/*
var templateFS embed.FS

var templates *template.Template

func main() {
	port := flag.Int("port", 8080, "HTTP server port")
	dataDir := flag.String("data-dir", "./data", "Path to data directory")
	hostapdConf := flag.String("hostapd-conf", "/etc/hostapd/hostapd.conf", "Path to hostapd.conf")
	xrayConfig := flag.String("xray-config", "/usr/local/etc/xray/config.json", "Path to xray config.json")
	flag.Parse()

	var err error
	templates, err = template.ParseFS(templateFS, "templates/*.html")
	if err != nil {
		log.Fatalf("Failed to parse templates: %v", err)
	}

	store, err := NewStorage(*dataDir)
	if err != nil {
		log.Fatalf("Failed to init storage: %v", err)
	}

	cmdRunner := &RealCommandRunner{}
	ipFetcher := &HTTPExternalIPFetcher{}
	sessions := NewSessionStore()

	apManager := NewAPManager(*hostapdConf, cmdRunner, func(secured bool) error {
		cfg, err := store.LoadConfig()
		if err != nil {
			return err
		}
		cfg.APSecured = secured
		return store.SaveConfig(cfg)
	})

	svcHandler := &ServiceHandler{Runner: cmdRunner, IPFetcher: ipFetcher}
	loginHandler := &LoginHandler{Store: store, Sessions: sessions}
	setupHandler := &SetupHandler{Store: store, AP: apManager}
	vlessHandler := &VLESSHandler{Store: store, Runner: cmdRunner, XrayConfig: *xrayConfig}
	subHandler := &SubHandler{Store: store}
	wifiHandler := NewWiFiHandler(cmdRunner)
	settingsHandler := &SettingsHandler{Store: store, AP: apManager}

	mux := http.NewServeMux()

	// Setup wizard
	mux.HandleFunc("/setup", setupHandler.HandleSetupPage)
	mux.HandleFunc("/setup/password", setupHandler.HandleSetupPassword)
	mux.HandleFunc("/setup/ap-password", setupHandler.HandleSetupAPPassword)
	mux.HandleFunc("/setup/complete", setupHandler.HandleSetupComplete)

	// Auth
	mux.HandleFunc("/login", loginHandler.HandleLoginPage)
	mux.HandleFunc("/auth/login", loginHandler.HandleLogin)
	mux.HandleFunc("/auth/logout", loginHandler.HandleLogout)

	// Main page
	mux.HandleFunc("/", handleIndex)

	// API: services
	mux.HandleFunc("/api/status", svcHandler.HandleStatus)
	mux.HandleFunc("/api/restart/", svcHandler.HandleRestart)
	mux.HandleFunc("/api/logs/", svcHandler.HandleLogs)

	// API: VLESS
	mux.HandleFunc("/api/vless", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			vlessHandler.HandleList(w, r)
		case http.MethodPost:
			vlessHandler.HandleAdd(w, r)
		default:
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})
	mux.HandleFunc("/api/vless/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodDelete {
			vlessHandler.HandleDelete(w, r)
		} else if r.Method == http.MethodPost {
			vlessHandler.HandleActivate(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	})

	// API: subscriptions
	mux.HandleFunc("/api/hwid", subHandler.HandleHWID)
	mux.HandleFunc("/api/sub", subHandler.HandleSub)

	// API: settings
	mux.HandleFunc("/api/settings/panel-password", settingsHandler.HandlePanelPassword)
	mux.HandleFunc("/api/settings/ap-password", settingsHandler.HandleAPPassword)

	// API: WiFi
	mux.HandleFunc("/api/wifi/scan", wifiHandler.HandleScan)
	mux.HandleFunc("/api/wifi/status", wifiHandler.HandleStatus)
	mux.HandleFunc("/api/wifi/connect", wifiHandler.HandleConnect)

	// Wrap with middleware
	handler := Middleware(store, sessions, mux)

	addr := fmt.Sprintf(":%d", *port)
	log.Printf("RPi Panel starting on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}

func handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	templates.ExecuteTemplate(w, "index.html", nil)
}
