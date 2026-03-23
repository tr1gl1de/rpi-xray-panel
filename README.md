# rpi-xray-panel

Web-панель управления Xray VPN на Raspberry Pi Zero W.

## Требования

- Go 1.21+
- Raspberry Pi Zero W (ARMv6)
- SSH-доступ к RPi

## Сборка

Кросс-компиляция для RPi Zero W (ARMv6):

```bash
make build
```

Это выполнит:
```
GOOS=linux GOARCH=arm GOARM=6 go build -o rpi-panel ./...
```

## Деплой

### 1. Установка systemd-сервиса (один раз)

```bash
make install-service
```

Эта команда:
- Копирует `rpi-panel.service` на RPi через SCP
- Устанавливает unit-файл в `/etc/systemd/system/`
- Выполняет `systemctl daemon-reload`
- Включает автозапуск: `systemctl enable rpi-panel`

### 2. Деплой бинарника

```bash
make deploy
```

Эта команда:
- Собирает бинарник (`make build`)
- Копирует его на RPi через SCP в `~/rpi-panel`
- Перезапускает сервис: `systemctl restart rpi-panel`

### 3. Ручное управление сервисом на RPi

```bash
sudo systemctl start rpi-panel
sudo systemctl stop rpi-panel
sudo systemctl status rpi-panel
sudo journalctl -u rpi-panel -f
```

## Разработка

Запуск локально:

```bash
make run
```

Тесты:

```bash
make test
```
