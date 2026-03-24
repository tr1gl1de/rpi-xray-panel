#!/bin/bash
set -e

# ============================================================
#  RPi Panel — Install Script
#  Поддерживаемые устройства: Raspberry Pi Zero W / Zero 2W
#  ОС: Raspberry Pi OS (Debian Bookworm/Trixie)
# ============================================================

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
BOLD='\033[1m'
NC='\033[0m'

log()     { echo -e "${GREEN}[✓]${NC} $1"; }
info()    { echo -e "${BLUE}[i]${NC} $1"; }
warn()    { echo -e "${YELLOW}[!]${NC} $1"; }
error()   { echo -e "${RED}[✗]${NC} $1"; exit 1; }
section() { echo -e "\n${BOLD}${BLUE}▶ $1${NC}"; }

# ============================================================
# Конфигурация — можно менять перед запуском
# ============================================================
AP_SSID="RPi-Panel"
AP_CHANNEL=6
AP_INTERFACE="uap0"
AP_IP="192.168.4.1"
AP_DHCP_START="192.168.4.2"
AP_DHCP_END="192.168.4.20"
WIFI_INTERFACE="wlan0"
PANEL_PORT=8080
PANEL_USER="max"
PANEL_DIR="/home/${PANEL_USER}"
PANEL_CONFIG_DIR="/etc/rpi-panel"
XRAY_CONFIG="/usr/local/etc/xray/config.json"
REDSOCKS_PORT=12345
SOCKS_PORT=1080

# ============================================================
# Проверки перед запуском
# ============================================================
check_root() {
  if [ "$EUID" -ne 0 ]; then
    error "Запусти скрипт с правами root: sudo bash install.sh"
  fi
}

check_wifi() {
  if ! ip link show "$WIFI_INTERFACE" &>/dev/null; then
    error "WiFi интерфейс $WIFI_INTERFACE не найден. Проверь подключение."
  fi
}

check_os() {
  if ! grep -q "Raspberry Pi" /proc/cpuinfo 2>/dev/null; then
    warn "Не определён Raspberry Pi — продолжаем на свой страх и риск"
  fi
}

# ============================================================
# 1. Зависимости
# ============================================================
install_dependencies() {
  section "Установка зависимостей"

  apt-get update -qq
  apt-get install -y \
    hostapd \
    dnsmasq \
    redsocks \
    iptables \
    iw \
    wireless-tools \
    curl \
    wget \
    ca-certificates \
    --no-install-recommends

  log "Зависимости установлены"
}

install_xray() {
  section "Установка Xray-core"

  if command -v xray &>/dev/null; then
    log "Xray уже установлен: $(xray version | head -1)"
    return
  fi

  bash -c "$(curl -sL https://github.com/XTLS/Xray-install/raw/main/install-release.sh)" @ install
  log "Xray установлен: $(xray version | head -1)"
}

# ============================================================
# 2. Настройка uap0 (виртуальный AP интерфейс)
# ============================================================
setup_uap0_service() {
  section "Настройка uap0 интерфейса"

  cat > /etc/systemd/system/uap0.service << EOF
[Unit]
Description=Create uap0 virtual WiFi interface
Before=hostapd.service
After=network.target

[Service]
Type=oneshot
RemainAfterExit=yes
ExecStart=/sbin/iw dev ${WIFI_INTERFACE} interface add ${AP_INTERFACE} type __ap
ExecStartPost=/bin/sleep 1
ExecStartPost=/sbin/ip addr add ${AP_IP}/24 dev ${AP_INTERFACE}
ExecStartPost=/sbin/ip link set ${AP_INTERFACE} up
ExecStartPost=/sbin/iptables-restore /etc/iptables.ipv4.nat

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  systemctl enable uap0
  log "Сервис uap0 создан"
}

# ============================================================
# 3. Настройка hostapd (точка доступа)
# ============================================================
setup_hostapd() {
  section "Настройка hostapd (AP)"

  # Определяем текущий канал роутера
  CURRENT_CHANNEL=$(iw dev "$WIFI_INTERFACE" link 2>/dev/null | grep -i freq | awk '{print $2}' | head -1)
  if [ -n "$CURRENT_CHANNEL" ]; then
    # Конвертация частоты в канал
    AP_CHANNEL=$(( (CURRENT_CHANNEL - 2412) / 5 + 1 ))
    info "Определён канал роутера: $AP_CHANNEL (${CURRENT_CHANNEL} MHz)"
  fi

  cat > /etc/hostapd/hostapd.conf << EOF
interface=${AP_INTERFACE}
ssid=${AP_SSID}
hw_mode=g
channel=${AP_CHANNEL}
wmm_enabled=0
macaddr_acl=0
auth_algs=1
ignore_broadcast_ssid=0
# wpa_passphrase= (будет добавлен через Setup Wizard в панели)
EOF

  # Указываем путь к конфигу
  sed -i 's|#DAEMON_CONF=.*|DAEMON_CONF="/etc/hostapd/hostapd.conf"|' /etc/default/hostapd 2>/dev/null || true

  systemctl unmask hostapd
  systemctl enable hostapd
  log "hostapd настроен (открытая AP: ${AP_SSID})"
}

# ============================================================
# 4. Настройка dnsmasq (DHCP для AP)
# ============================================================
setup_dnsmasq() {
  section "Настройка dnsmasq (DHCP)"

  # Бэкап оригинального конфига
  if [ -f /etc/dnsmasq.conf ] && [ ! -f /etc/dnsmasq.conf.backup ]; then
    cp /etc/dnsmasq.conf /etc/dnsmasq.conf.backup
  fi

  cat > /etc/dnsmasq.d/rpi-panel.conf << EOF
interface=${AP_INTERFACE}
dhcp-range=${AP_DHCP_START},${AP_DHCP_END},255.255.255.0,24h
domain=local
address=/gw.local/${AP_IP}
EOF

  systemctl enable dnsmasq
  log "dnsmasq настроен (DHCP: ${AP_DHCP_START}-${AP_DHCP_END})"
}

# ============================================================
# 5. Настройка redsocks (прозрачный SOCKS прокси)
# ============================================================
setup_redsocks() {
  section "Настройка redsocks"

  cat > /etc/redsocks.conf << EOF
base {
  log_debug = off;
  log_info = on;
  log = "syslog:daemon";
  daemon = on;
  user = redsocks;
  group = redsocks;
  redirector = iptables;
}

redsocks {
  local_ip = 0.0.0.0;
  local_port = ${REDSOCKS_PORT};
  ip = 127.0.0.1;
  port = ${SOCKS_PORT};
  type = socks5;
}
EOF

  systemctl enable redsocks
  log "redsocks настроен (порт ${REDSOCKS_PORT} → SOCKS ${SOCKS_PORT})"
}

# ============================================================
# 6. Настройка xray (заготовка конфига)
# ============================================================
setup_xray() {
  section "Настройка Xray"

  mkdir -p "$(dirname $XRAY_CONFIG)"

  # Записываем базовый конфиг (без сервера — добавляется через панель)
  cat > "$XRAY_CONFIG" << EOF
{
  "inbounds": [
    {
      "port": ${SOCKS_PORT},
      "listen": "127.0.0.1",
      "protocol": "socks",
      "settings": {
        "auth": "noauth",
        "udp": true
      }
    }
  ],
  "outbounds": [
    {
      "protocol": "freedom",
      "tag": "direct"
    }
  ]
}
EOF

  # Исправляем пользователя в сервисе (nobody → root)
  if [ -f /etc/systemd/system/xray.service ]; then
    sed -i 's/User=nobody/User=root/' /etc/systemd/system/xray.service
  elif [ -f /usr/lib/systemd/system/xray.service ]; then
    cp /usr/lib/systemd/system/xray.service /etc/systemd/system/xray.service
    sed -i 's/User=nobody/User=root/' /etc/systemd/system/xray.service
  fi

  systemctl daemon-reload
  systemctl enable xray
  log "Xray настроен (добавь VLESS сервер через панель)"
}

# ============================================================
# 7. Настройка IP форвардинга и iptables
# ============================================================
setup_networking() {
  section "Настройка сети и iptables"

  # IP форвардинг
  if ! grep -q "net.ipv4.ip_forward=1" /etc/sysctl.conf 2>/dev/null; then
    echo "net.ipv4.ip_forward=1" >> /etc/sysctl.conf
  fi
  sysctl -p /etc/sysctl.conf -q
  log "IP форвардинг включён"

  # Сбрасываем старые NAT правила
  iptables -t nat -F
  iptables -t nat -X 2>/dev/null || true

  # MASQUERADE — выход в интернет
  iptables -t nat -A POSTROUTING -o "$WIFI_INTERFACE" -j MASQUERADE

  # REDSOCKS chain — прозрачный прокси для AP клиентов
  iptables -t nat -N REDSOCKS

  # Исключения — локальные адреса не трогаем
  iptables -t nat -A REDSOCKS -d 0.0.0.0/8    -j RETURN
  iptables -t nat -A REDSOCKS -d 10.0.0.0/8   -j RETURN
  iptables -t nat -A REDSOCKS -d 127.0.0.0/8  -j RETURN
  iptables -t nat -A REDSOCKS -d 192.168.0.0/16 -j RETURN

  # Весь TCP с uap0 → redsocks
  iptables -t nat -A REDSOCKS -p tcp -j REDIRECT --to-ports "$REDSOCKS_PORT"
  iptables -t nat -A PREROUTING -i "$AP_INTERFACE" -p tcp -j REDSOCKS

  # Сохраняем правила
  iptables-save > /etc/iptables.ipv4.nat
  log "iptables настроен и сохранён"
}

# ============================================================
# 8. Создание директории для конфигов панели
# ============================================================
setup_panel_config() {
  section "Подготовка конфигурации панели"

  mkdir -p "$PANEL_CONFIG_DIR"

  # Начальный config.json (setup_done: false = онбординг при первом входе)
  if [ ! -f "${PANEL_CONFIG_DIR}/config.json" ]; then
    cat > "${PANEL_CONFIG_DIR}/config.json" << EOF
{
  "setup_done": false,
  "ap_secured": false,
  "port": ${PANEL_PORT},
  "username": "admin",
  "password_hash": ""
}
EOF
  fi

  # Начальный servers.json
  if [ ! -f "${PANEL_CONFIG_DIR}/servers.json" ]; then
    cat > "${PANEL_CONFIG_DIR}/servers.json" << EOF
{
  "servers": [],
  "active_index": -1
}
EOF
  fi

  # Начальный subs.json
  if [ ! -f "${PANEL_CONFIG_DIR}/subs.json" ]; then
    cat > "${PANEL_CONFIG_DIR}/subs.json" << EOF
{
  "subscriptions": []
}
EOF
  fi

  log "Конфигурация панели подготовлена в ${PANEL_CONFIG_DIR}"
}

# ============================================================
# 9. Настройка systemd сервиса для панели
# ============================================================
setup_panel_service() {
  section "Настройка systemd сервиса RPi Panel"

  cat > /etc/systemd/system/rpi-panel.service << EOF
[Unit]
Description=RPi Panel — Web management interface
After=network.target uap0.service hostapd.service xray.service redsocks.service
Wants=uap0.service

[Service]
Type=simple
User=root
WorkingDirectory=${PANEL_DIR}
ExecStart=${PANEL_DIR}/rpi-panel
Restart=always
RestartSec=5
Environment=PANEL_PORT=${PANEL_PORT}
Environment=PANEL_CONFIG=${PANEL_CONFIG_DIR}

[Install]
WantedBy=multi-user.target
EOF

  systemctl daemon-reload
  # Не включаем автозапуск пока нет бинарника
  log "Сервис rpi-panel создан (запустится после установки бинарника)"
}

# ============================================================
# 10. Первый запуск сервисов
# ============================================================
start_services() {
  section "Запуск сервисов"

  # Поднимаем uap0
  systemctl start uap0 || warn "uap0: не удалось запустить (возможно уже существует)"

  # Запускаем сервисы
  systemctl restart dnsmasq  && log "dnsmasq запущен"
  systemctl restart hostapd  && log "hostapd запущен"
  systemctl restart redsocks && log "redsocks запущен"
  systemctl restart xray     && log "xray запущен"
}

# ============================================================
# 11. Итог
# ============================================================
print_summary() {
  echo ""
  echo -e "${BOLD}${GREEN}╔══════════════════════════════════════════╗${NC}"
  echo -e "${BOLD}${GREEN}║        Установка завершена успешно!      ║${NC}"
  echo -e "${BOLD}${GREEN}╚══════════════════════════════════════════╝${NC}"
  echo ""
  echo -e "${BOLD}Что настроено:${NC}"
  echo -e "  ${GREEN}✓${NC} AP точка доступа: ${BOLD}${AP_SSID}${NC} (открытая, канал ${AP_CHANNEL})"
  echo -e "  ${GREEN}✓${NC} DHCP для AP: ${AP_DHCP_START} — ${AP_DHCP_END}"
  echo -e "  ${GREEN}✓${NC} Прозрачный прокси: redsocks → xray"
  echo -e "  ${GREEN}✓${NC} IP форвардинг и iptables"
  echo -e "  ${GREEN}✓${NC} Все сервисы в автозапуске"
  echo ""
  echo -e "${BOLD}Следующий шаг — установка RPi Panel:${NC}"
  echo -e "  Скачай бинарник и скопируй на RPi:"
  echo -e "  ${BLUE}scp rpi-panel ${PANEL_USER}@<ip>:~/rpi-panel${NC}"
  echo -e "  ${BLUE}sudo systemctl enable --now rpi-panel${NC}"
  echo ""
  echo -e "${BOLD}После запуска панели:${NC}"
  echo -e "  1. Подключись к WiFi: ${BOLD}${AP_SSID}${NC}"
  echo -e "  2. Открой браузер: ${BOLD}http://${AP_IP}:${PANEL_PORT}${NC}"
  echo -e "  3. Пройди Setup Wizard — установи пароли"
  echo -e "  4. Добавь VLESS сервер через вставку vless:// ссылки"
  echo ""
  echo -e "${YELLOW}Статус сервисов:${NC}"
  for svc in uap0 hostapd dnsmasq redsocks xray; do
    STATUS=$(systemctl is-active "$svc" 2>/dev/null)
    if [ "$STATUS" = "active" ]; then
      echo -e "  ${GREEN}●${NC} $svc"
    else
      echo -e "  ${RED}●${NC} $svc (${STATUS})"
    fi
  done
  echo ""
}

# ============================================================
# Точка входа
# ============================================================
main() {
  echo ""
  echo -e "${BOLD}${BLUE}  RPi Panel — Installer${NC}"
  echo -e "  Raspberry Pi Zero W / Zero 2W"
  echo -e "  $(date '+%Y-%m-%d %H:%M:%S')"
  echo ""

  check_root
  check_os
  check_wifi

  install_dependencies
  install_xray
  setup_uap0_service
  setup_hostapd
  setup_dnsmasq
  setup_redsocks
  setup_xray
  setup_networking
  setup_panel_config
  setup_panel_service
  start_services
  print_summary
}

main "$@"
