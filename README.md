# Модуль безопасности персональных данных

Прокси-сервис между системой-потребителем и LLM: идентифицирует персональные
данные, маскирует их перед отправкой в LLM и демаскирует ответ.

## Контракт

`POST /process` — единый эндпоинт для маскирования и демаскирования.

Запрос:
```json
{ "payload": "<строка>", "payload_id": "<идентификатор>" }
```

Ответ:
```json
{ "result": "<строка>" }
```

Логика:
- Первый запрос с новым `payload_id` — маскирование (`payload` = исходная строка) → возвращается маска, соответствие запоминается.
- Второй запрос с тем же `payload_id` — демаскирование (`payload` = маска) → возвращается исходная строка.
- Повтор оригинала возвращает ту же маску; повтор маски — оригинал (идемпотентность).

## Запуск

Требуется Go 1.25+.

```bash
# 1. Собрать
go build -o bin/server ./cmd/server

# 2. Задать ключ шифрования (32 байта в hex) и запустить
PII_AES_KEY=$(openssl rand -hex 32) PII_LISTEN_ADDR=:8080 ./bin/server

# 3. Проверить
curl -X POST http://localhost:8080/process \
  -H "Content-Type: application/json" \
  -d '{"payload":"email ivan@example.com","payload_id":"demo-1"}'
```

Конфигурация через JSON-файл (`-config deploy/config.example.json`) или
переменные окружения `PII_*`. См. `deploy/config.example.json`.

## Docker

```bash
docker build -f deploy/Dockerfile -t pii-module .
docker run -p 8080:8080 -e PII_AES_KEY=$(openssl rand -hex 32) pii-module
```

## Эндпоинты

- `POST /process` — маскирование/демаскирование.
- `GET /metrics` — метрики (RPS, latency, TPS, счётчики).
- `GET /healthz` — проверка живости.

## Идентификация потребителя

Потребитель передаётся заголовком `X-Consumer` (по умолчанию `default`).
Список разрешённых систем и их политики настраиваются в конфиге
(`allowlist`, `consumers`). Политика задаёт: включение обработки, перечень
типов ПД, разрешение демаскирования, режим маскирования (`token`/`mask`).

## Структура

- `cmd/server` — точка входа.
- `internal/api` — HTTP-обработчик и сервис маскирования/демаскирования.
- `internal/masking` — токенизация и восстановление.
- `internal/store` — in-memory хранилище соответствий (TTL, лимит, шифрование).
- `internal/policy` — политики потребителей.
- `internal/detection` — контракт детекторов (реализация — участник 2).
- `internal/metrics` — метрики.
- `internal/tempdetect` — временный детектор (email/телефон) до интеграции реального.
- `deploy` — Dockerfile и пример конфигурации.