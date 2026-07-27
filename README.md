# Notification Service

Асинхронный сервис доставки уведомлений по трём каналам: **email**, **SMS** и **push**.
Написан на Go, использует RabbitMQ как очередь сообщений и PostgreSQL как хранилище
статусов/шаблонов. Полностью контейнеризирован через Docker Compose.

## Архитектура

```
                 ┌──────────────┐
  HTTP клиент ──▶│   API (Go)   │──▶ PostgreSQL (сохраняет notification, status=pending)
                 └──────┬───────┘
                        │ publish(routing_key = channel)
                        ▼
              ┌───────────────────┐
              │  RabbitMQ topic   │  exchange: notifications
              │  exchange         │
              └───┬───────┬───────┘
                  │       │       │
          email queue  sms queue  push queue
                  │       │       │
                  ▼       ▼       ▼
              ┌─────────────────────┐
              │   Worker (Go)       │  consumes все три очереди
              │  email/sms/push     │  goroutine на канал
              │  sender-ы           │
              └─────────┬───────────┘
                         │ update status (sent/failed/retrying)
                         ▼
                   PostgreSQL
```

### Ключевые решения

- **API и Worker — отдельные бинарники** (`cmd/api`, `cmd/worker`), из одного кода можно
  собрать оба образа (см. `Dockerfile`, `ARG BINARY`). Это позволяет масштабировать worker
  независимо от API (`docker compose up --scale worker=3`).
- **Надёжная доставка**: сообщение публикуется в RabbitMQ только после успешной записи
  в PostgreSQL со статусом `pending`. Worker при обработке заново читает состояние из БД
  (защита от рассинхронизации, если запись успели изменить).
- **Retry с задержкой**: при неудачной отправке сообщение публикуется в retry-очередь
  с TTL (`RETRY_DELAY_MS`), после истечения TTL RabbitMQ dead-letter'ит его обратно
  в основной exchange с тем же routing key — так реализована задержка без плагинов.
- **Dead-letter/failed очередь**: после исчерпания `MAX_ATTEMPTS` уведомление помечается
  `status=failed` в БД и сообщение уходит в очередь `notifications.failed` для ручного
  разбора/алертинга.
- **Шаблоны**: можно один раз сохранить шаблон (`POST /api/v1/templates`) и затем
  создавать уведомления через `template_name` + `template_data` (простая подстановка
  `{{key}}`).

## Стек

| Компонент       | Технология                                      |
|-----------------|--------------------------------------------------|
| Язык            | Go 1.22                                          |
| HTTP роутер     | chi                                              |
| Очередь         | RabbitMQ (amqp091-go)                            |
| БД              | PostgreSQL 16 (pgx/v5, pgxpool)                  |
| Email           | SMTP (net/smtp), в dev — MailHog                 |
| SMS             | HTTP REST провайдер, совместимый с Twilio API    |
| Push            | Firebase Cloud Messaging (legacy HTTP API)       |
| Контейнеризация | Docker + Docker Compose                          |

## Быстрый старт

```bash
git clone <repo>
cd notification-service
cp .env.example .env      # при необходимости отредактируйте

go mod tidy                # сгенерировать go.sum перед первой сборкой

docker compose up --build
```

После старта:
- API:              http://localhost:8080
- RabbitMQ UI:       http://localhost:15672 (guest/guest)
- MailHog (письма):  http://localhost:8025

## API

### Создать уведомление

```bash
curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
        "channel": "email",
        "recipient": "user@example.com",
        "subject": "Добро пожаловать",
        "body": "Спасибо за регистрацию!"
      }'
```

Ответ (`202 Accepted`):
```json
{
  "id": "b1f3c2...",
  "channel": "email",
  "recipient": "user@example.com",
  "status": "pending",
  "attempts": 0,
  "max_attempts": 3,
  "created_at": "2026-07-27T10:00:00Z"
}
```

### Проверить статус

```bash
curl http://localhost:8080/api/v1/notifications/b1f3c2...
```

### Список уведомлений (с фильтром по статусу)

```bash
curl "http://localhost:8080/api/v1/notifications?status=failed&limit=20"
```

### Создать шаблон и использовать его

```bash
curl -X POST http://localhost:8080/api/v1/templates \
  -H "Content-Type: application/json" \
  -d '{
        "name": "order_confirmation",
        "channel": "sms",
        "body": "Заказ {{order_id}} подтверждён, ожидайте {{eta}}."
      }'

curl -X POST http://localhost:8080/api/v1/notifications \
  -H "Content-Type: application/json" \
  -d '{
        "channel": "sms",
        "recipient": "+79001234567",
        "template_name": "order_confirmation",
        "template_data": {"order_id": "12345", "eta": "30 минут"}
      }'
```

## Структура проекта

```
cmd/
  api/            точка входа HTTP API
  worker/         точка входа воркера (консьюмер RabbitMQ)
internal/
  api/            роутер и HTTP-хендлеры
  config/         загрузка конфигурации из env
  models/         доменные модели
  queue/          топология и клиент RabbitMQ
  repository/     доступ к PostgreSQL
  sender/         реализации отправки (email/sms/push)
  service/        бизнес-логика (создание, обработка, ретраи)
migrations/       SQL-миграции (авто-применяются при первом старте postgres)
```

## Расширение

- Чтобы добавить нового SMS/push провайдера — реализуйте интерфейс `sender.Sender`
  и зарегистрируйте его в `cmd/worker/main.go`.
- Чтобы поменять политику ретраев — измените `MAX_ATTEMPTS`/`RETRY_DELAY_MS` в `.env`.
- Для продакшена рекомендуется:
  - вынести миграции на отдельный шаг деплоя (например, `golang-migrate`) вместо
    авто-применения через `docker-entrypoint-initdb.d`;
  - добавить TLS для RabbitMQ/PostgreSQL и секреты через Vault/SSM вместо `.env`;
  - подключить метрики (Prometheus) и трассировку (OpenTelemetry).
