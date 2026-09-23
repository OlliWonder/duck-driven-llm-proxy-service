# AlfaGen: модуль защиты персональных данных

HTTP-сервис находит персональные данные в русском тексте, маскирует их перед
передачей в LLM и по тому же `payload_id` восстанавливает исходные значения в
ответе. Production-детектор сочетает правила для структурированных данных с
локальным NER sidecar на Slovnet.

## Стек

- Go 1.25 (`net/http`) — API, политики, маскирование, метрики и in-memory
  хранилище соответствий.
- Python 3.12, Slovnet 0.6, Navec 0.10, Razdel 0.5 и NumPy — извлечение
  кандидатов `PER`, `LOC` и `ORG`.
- Docker и Docker Compose — сборка и запуск API вместе с NER sidecar.
- AES-256-GCM — шифрование значений, сохранённых в оперативной памяти.

Поддерживаются 17 типов ПДн: `full_name`, `birth_date`, `birth_place`,
`passport_series`, `citizenship`, `passport_issuer`, `passport_dept_code`,
`passport_issue_date`, `driving_license`, `address`, `email`, `phone`, `inn`,
`cvv`, `pin`, `card_holder`, `card_number`.

## Как это устроено

Клиент обращается только к Go API на порту `8080`. API запускает rule-based
детекторы и отправляет NER-запросы по внутренней сети Compose на
`http://ner:8090`. Python sidecar наружу не публикуется.

```text
клиент -> Go API :8080 -> production detector -> Python NER :8090
                      -> masking / restore -> in-memory store
```

## Быстрый запуск через Docker

Нужен только запущенный Docker Desktop либо Docker Engine с Docker Compose v2.
Локальная установка Go, Python и Python-библиотек не требуется.

1. Соберите и запустите оба контейнера из корня репозитория:

   ```bash
   docker compose up --build -d
   ```

2. Дождитесь состояния `healthy` у сервисов `alfagen-api-1` и `alfagen-ner-1`:

   ```bash
   docker compose ps
   ```

3. Проверьте Go API:

   ```bash
   curl http://localhost:8080/healthz
   ```

   Ожидаемый ответ: `ok`.

4. Отправьте тестовый запрос с новым `payload_id`:

   ```bash
   curl -X POST http://localhost:8080/process \
     -H "Content-Type: application/json" \
     -d '{"payload":"клиент Иван Петров, email ivan@example.com","payload_id":"readme-demo-1"}'
   ```

   В PowerShell тот же запрос выглядит так:

   ```powershell
   $body = @{
       payload = "клиент Иван Петров, email ivan@example.com"
       payload_id = "readme-demo-1"
   } | ConvertTo-Json
   Invoke-RestMethod -Method Post `
       -Uri http://localhost:8080/process `
       -ContentType "application/json" `
       -Body $body
   ```

5. Остановите сервисы:

   ```bash
   docker compose down
   ```

В PowerShell запуск и остановку можно выполнить короткими обёртками над теми
же командами:

```powershell
.\scripts\start.ps1
.\scripts\stop.ps1
```

Если контейнер не перешёл в `healthy`, посмотрите его журнал:

```bash
docker compose logs api
docker compose logs ner
```

### Модели NER

Веса Navec и Slovnet уже находятся в `ner/models` и копируются в NER-образ при
сборке. Во время запуска контейнер не скачивает модели и не зависит от путей на
компьютере разработчика. При первой сборке Docker загрузит только базовые
образы и Python-зависимости, если их ещё нет в локальном кеше.

### Ключ шифрования

Для локального запуска задавать `PII_AES_KEY` необязательно: entrypoint
API-контейнера создаёт случайный временный 32-байтовый ключ. Хранилище находится
в памяти, поэтому при пересоздании контейнера его содержимое и временный ключ
теряются.

Для стабильного окружения передайте через менеджер секретов или переменную
окружения `PII_AES_KEY` постоянный 32-байтовый ключ в hex-формате: ровно 64
шестнадцатеричных символа. Не добавляйте реальный ключ в репозиторий.

## HTTP API

### `POST /process`

Запрос:

```json
{
  "payload": "клиент Иван Петров",
  "payload_id": "request-1"
}
```

Ответ:

```json
{
  "result": "<замаскированная или восстановленная строка>"
}
```

Первый запрос с новым `payload_id` маскирует `payload` и сохраняет соответствия.
Запрос с тем же `payload_id` и полученной маской восстанавливает исходную
строку. Повтор исходной строки возвращает ту же маску.

Потребитель задаётся заголовком `X-Consumer`; если заголовок отсутствует,
используется `default`. Политика потребителя определяет доступные типы ПДн,
режим `token`/`mask` и возможность восстановления.

### Служебные endpoints

- `GET /healthz` — проверка состояния Go API.
- `GET /metrics` — метрики в текстовом формате Prometheus.

## Структура проекта

- `cmd/server` — точка входа Go API и инициализация production detector.
- `internal/detection` — rule-based детекторы структурированных ПДн.
- `internal/ner` — Go-клиент sidecar, batching, cache, chunking и контекстная
  валидация NER-кандидатов.
- `internal/api` — HTTP-обработчик и координация маскирования/восстановления.
- `internal/masking` — создание токенов и восстановление исходных значений.
- `internal/store` — ограниченное in-memory хранилище с TTL и шифрованием.
- `internal/policy` — политики потребителей.
- `internal/config` — конфигурация из JSON и переменных окружения.
- `internal/metrics` — метрики сервиса.
- `ner` — Python sidecar, зависимости и локальные веса моделей.
- `deploy` — Dockerfile и entrypoint Go API.
- `compose.yaml` — совместный запуск API и NER sidecar.
- `scripts` — PowerShell-команды запуска и остановки Compose.

`internal/tempdetect` используется только как вспомогательная реализация в
тестах и не подключён к production-сервису.
