# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

rpi-xray-panel — панель управления рентгеновским аппаратом на Raspberry Pi. Подробная спецификация — `docs/rpi-panel-prd-v3.docx` (PRD).

## Language & Toolchain

- **Language:** Go
- **Build:** `go build ./...`
- **Test:** `go test ./...`
- **Single test:** `go test -run TestName ./path/to/package`
- **Lint:** `go vet ./...` (add `golangci-lint run` if configured)

## Git Workflow

- **Коммиты делать только в ветку `dev`**, никогда напрямую в `main`
- Автор коммитов: `Maxim <max.stack.player@gmail.com>` (git config). Не добавлять Co-Authored-By от Anthropic/Claude

## Рабочий процесс (обязателен)

Вся работа ведётся через `docs/tasks.json` и логируется в `docs/Progress.txt`.

### Правило задач

Если пользователь просит что-то сделать, а задачи для этого нет в `tasks.json`:
1. Сначала создай задачу в `docs/tasks.json`
2. Затем выполняй её

### Роли агентов

- **architect** (`.claude/agents/architect.md`) — планирует, декомпозирует задачи, ревьюит результаты. Редактирует только `docs/tasks.json` и `docs/Progress.txt`. Не пишет код.
- **worker** (`.claude/agents/worker.md`) — строго выполняет задачи из `tasks.json`. Не создаёт новые задачи. Обновляет статус и пишет в Progress.txt.

### Память проекта

- `docs/tasks.json` — актуальный список задач, статусы, зависимости
- `docs/Progress.txt` — хронологический лог всех действий и решений
- `docs/rpi-panel-prd-v3.docx` — PRD, источник истины по функциональности

Перед началом работы всегда читай `docs/tasks.json` и `docs/Progress.txt` чтобы понять текущее состояние проекта.

## Тестирование

- Каждая задача с кодом имеет парную задачу с юнит-тестами (задачи #16-#25)
- Запуск всех тестов: `go test ./...`
- Запуск одного теста: `go test -run TestName ./path/to/package`
- Системные команды мокируются через интерфейс `CommandRunner`
- HTTP-хендлеры: `httptest.NewRequest` / `httptest.NewRecorder`
- Файлы: `t.TempDir()` для изоляции

## Параллельная работа воркеров

После выполнения задачи #1 (инициализация) можно запускать несколько воркеров параллельно по независимым веткам:

```
#1 (init) ──┬── #2 (storage) → #16 (тесты) → #3 (middleware) → #17 → #4/#5 (setup/auth) → #18/#19 → #7 (vless) → #21 → #12 → #13
            ├── #6 (services) → #20 (тесты) → #11 (логи)
            ├── #9 (wifi) → #23 (тесты)
            ├── #10 (AP) → #24 (тесты)
            └── #15 (деплой)
```

## Environment

- `.env` file is gitignored — use it for local secrets/configuration
