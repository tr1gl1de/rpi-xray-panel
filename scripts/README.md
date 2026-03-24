# RPi Panel

Веб-панель управления для Raspberry Pi Zero W. Превращает RPi в WiFi роутер с прозрачным VLESS прокси и удобным интерфейсом управления.

## Что это

RPi подключается к домашнему WiFi и одновременно создаёт свою точку доступа. Все устройства подключённые к этой AP автоматически ходят в интернет через VLESS туннель — без каких-либо настроек на самих устройствах.

Управление через веб-интерфейс: добавляй серверы, переключай подключения, меняй WiFi — всё без SSH.

```
Телефон / Ноутбук
      │
      ▼
  AP (RPi-Panel)          ← RPi создаёт свою WiFi сеть
      │
      ▼
  RPi Zero W
  ├── wlan0 → Домашний роутер → Интернет
  └── uap0  → redsocks → xray → VLESS сервер
```

## Требования

- Raspberry Pi Zero W или Zero 2W
- Raspberry Pi OS (Bookworm или Trixie)
- Подключение к интернету через WiFi
- VLESS сервер (ссылка формата `vless://...`)

## Быстрый старт

### Шаг 1 — Запусти скрипт установки на RPi

Подключись к RPi по SSH и выполни:

```bash
curl -sL https://raw.githubusercontent.com/your-repo/rpi-panel/main/install.sh | sudo bash
```

Или скачай и запусти вручную:

```bash
wget https://raw.githubusercontent.com/your-repo/rpi-panel/main/install.sh
sudo bash install.sh
```

Скрипт установит и настроит:
- `hostapd` — точка доступа WiFi
- `dnsmasq` — DHCP сервер для клиентов AP
- `redsocks` — прозрачный SOCKS прокси
- `xray-core` — VLESS клиент
- `iptables` — форвардинг и NAT
- Все `systemd` сервисы в автозапуске

### Шаг 2 — Установи RPi Panel

Скачай бинарник с [Releases](https://github.com/your-repo/rpi-panel/releases) и скопируй на RPi:

```bash
# С твоего компьютера
scp rpi-panel max@stackplayer.local:~/rpi-panel
ssh max@stackplayer.local "sudo systemctl enable --now rpi-panel"
```

### Шаг 3 — Первый вход (Setup Wizard)

1. Подключись к WiFi сети **RPi-Panel** (открытая, без пароля)
2. Открой браузер: **http://192.168.4.1:8080**
3. Пройди мастер настройки:
   - Установи пароль для панели (обязательно)
   - Установи пароль для WiFi AP (опционально)
4. Добавь VLESS сервер — вставь ссылку `vless://...`
5. Готово — весь трафик с AP идёт через VPN

## Интерфейс

### Simple Mode (по умолчанию)

Минималистичный экран для повседневного использования:

- Статус VPN и текущий внешний IP
- Выбор активного сервера из списка
- Смена WiFi сети
- Быстрые кнопки: перезапустить VPN, обновить подписку

### Advanced Mode

Полный контроль — переключается тогглом в правом углу шапки:

- Статус всех системных сервисов
- Добавление серверов через `vless://` ссылку
- Загрузка подписок Remnawave (с HWID идентификацией)
- Просмотр логов сервисов
- Рестарт сервисов по отдельности
- Настройки: смена паролей, управление AP

## Добавление VLESS сервера

В панели (Advanced Mode → VLESS серверы) вставь ссылку формата:

```
vless://uuid@server:port?security=reality&type=tcp&flow=xtls-rprx-vision&sni=...&pbk=...&sid=...#Имя сервера
```

Параметры парсятся автоматически. Можно добавить несколько серверов и переключаться между ними.

## Подписки Remnawave

RPi Panel поддерживает [HWID Device Limit](https://docs.rw/docs/features/hwid-device-limit) от Remnawave.

При загрузке подписки отправляются заголовки:

```
x-hwid: <уникальный ID устройства на основе MAC>
x-device-os: Linux
x-device-model: RPi Zero W
```

HWID отображается в панели (Advanced → Подписки) — используй его при регистрации устройства в Remnawave.

## Структура после установки

```
/etc/rpi-panel/
├── config.json      # настройки панели (пароль, флаги)
├── servers.json     # список VLESS серверов
└── subs.json        # URL подписок

/etc/hostapd/
└── hostapd.conf     # настройки AP (SSID, канал, пароль)

/usr/local/etc/xray/
└── config.json      # конфиг xray (генерируется панелью)

/etc/systemd/system/
├── uap0.service     # виртуальный WiFi интерфейс
└── rpi-panel.service
```

## Сервисы

| Сервис | Назначение | Порт |
|--------|------------|------|
| `uap0` | Виртуальный WiFi интерфейс AP | — |
| `hostapd` | Точка доступа WiFi | — |
| `dnsmasq` | DHCP для клиентов AP | 53 |
| `redsocks` | Прозрачный SOCKS прокси | 12345 |
| `xray` | VLESS клиент | 1080 |
| `rpi-panel` | Веб-панель управления | 8080 |

Проверить статус всех сервисов:

```bash
systemctl status uap0 hostapd dnsmasq redsocks xray rpi-panel
```

## Ручная настройка (если нужно)

### Сменить SSID точки доступа

```bash
sudo nano /etc/hostapd/hostapd.conf
# Измени: ssid=Новое_Имя
sudo systemctl restart hostapd
```

### Сменить канал AP

Канал должен совпадать с каналом домашнего роутера:

```bash
# Узнать текущий канал
iw dev wlan0 link | grep freq

# Изменить в конфиге
sudo nano /etc/hostapd/hostapd.conf
# channel=6
sudo systemctl restart hostapd
```

### Сбросить настройки панели

```bash
sudo rm /etc/rpi-panel/config.json
sudo systemctl restart rpi-panel
# Панель запустит Setup Wizard заново
```

## Устранение проблем

### AP не появляется в списке WiFi сетей

```bash
# Проверить статус
sudo systemctl status uap0 hostapd

# Проверить что uap0 создан
ip addr show uap0

# Пересоздать интерфейс
sudo systemctl restart uap0
sudo systemctl restart hostapd
```

### Нет интернета через AP

```bash
# Проверить iptables
sudo iptables -t nat -L -n

# Если пусто — восстановить
sudo iptables-restore < /etc/iptables.ipv4.nat

# Проверить форвардинг
cat /proc/sys/net/ipv4/ip_forward  # должно быть 1
```

### VPN не работает

```bash
# Статус xray
sudo systemctl status xray
sudo journalctl -u xray -n 50

# Проверить что xray слушает порт
ss -tlnp | grep 1080

# Тест туннеля
curl --socks5 127.0.0.1:1080 https://ifconfig.me
```

### Панель недоступна

```bash
# Статус панели
sudo systemctl status rpi-panel

# Проверить порт
ss -tlnp | grep 8080

# Перезапустить
sudo systemctl restart rpi-panel
```

## Разработка

Проект написан на Go. Для локальной разработки на macOS:

```bash
# Клонировать
git clone https://github.com/your-repo/rpi-panel
cd rpi-panel

# Запустить локально (для разработки)
go run ./...

# Собрать под RPi Zero W (ARMv6)
GOOS=linux GOARCH=arm GOARM=6 go build -o rpi-panel ./...

# Задеплоить на RPi
make deploy
```

### Makefile

```bash
make build    # сборка под ARMv6
make deploy   # сборка + scp + рестарт сервиса
make logs     # логи панели в реальном времени
```

## Лицензия

MIT
