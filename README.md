# balance-user-api

Учебный сервис балансов по [тестовому заданию Авито](docs/task.md): начисление, списание, перевод и получение остатка. Деньги хранятся в копейках. Есть HTTP JSON API и gRPC, хранилище в PostgreSQL или в памяти.

Go 1.26.3 · net/http · pgx · PostgreSQL 18 · gRPC · Docker Compose

## Оглавление

- [Запуск](#запуск)
- [HTTP API](#http-api)
- [gRPC](#grpc)
- [Тесты](#тесты)
- [Решения по заданию](#решения-по-заданию)

Устройство слоёв, транзакции и генерация Protobuf описаны в [архитектуре](docs/architecture.md).

## Запуск

Нужен Docker с Compose. Из корня проекта:

```bash
cp .env.example .env
docker compose up --build
```

HTTP API доступен на `localhost:8080`, PostgreSQL — на `localhost:5433`. Проверка:

```bash
curl localhost:8080/ping
# pong
```

Compose ждёт готовности базы и применяет [SQL-схему](migrations/001_init.sql) при первом запуске с пустым томом. `docker compose down` сохраняет данные, `docker compose down -v` удаляет их вместе с томом.

Для запуска приложения локально нужен Go 1.26.3 или новее. После создания `.env`:

```bash
docker compose up -d --wait db
set -a; source .env; set +a
go run ./cmd/app
```

Контейнер `app` при этом должен быть остановлен, чтобы освободить порт. Без PostgreSQL можно запустить версию в памяти:

```bash
STORAGE=memory TRANSPORT=http go run ./cmd/app
```

Данные в памяти теряются при остановке процесса.

Настройки читаются из окружения. Go сам не загружает `.env`; пример всех переменных есть в [.env.example](.env.example).

| Переменная | Значение |
|---|---|
| `STORAGE` | Обязательна: `postgres` или `memory` |
| `TRANSPORT` | Обязательна: `http` или `grpc` |
| `DATABASE_URL` | Обязательна для PostgreSQL; с хоста — `localhost:5433`, из Compose — `db:5432` |
| `PORT` | Порт выбранного сервера, по умолчанию `8080` |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Настройки базы для Compose |
| `TEST_DATABASE_URL` | Адрес отдельной базы `balance_test` для интеграционных тестов |

В Compose порт приложения закреплён как `8080`, хранилище — `postgres`. Неизвестные `STORAGE` / `TRANSPORT` или ошибка подключения к базе останавливают приложение.

## HTTP API

Все суммы — целые числа в копейках: `100000` = 1000 ₽. Успешные операции возвращают HTTP 200 и новый баланс.

| Метод | Путь | Запрос |
|---|---|---|
| POST | `/accrual` | `{"user_id": 1, "amount": 100000}` |
| POST | `/debiting` | `{"user_id": 1, "amount": 30000}` |
| POST | `/transfer` | `{"user_from_id": 1, "user_to_id": 2, "amount": 20000}` |
| GET | `/balance/{user_id}` | Без тела |

Пример для пустого хранилища — начислить обоим пользователям, списать и перевести:

```bash
curl -X POST localhost:8080/accrual -d '{"user_id": 1, "amount": 100000}'
curl -X POST localhost:8080/accrual -d '{"user_id": 2, "amount": 50000}'
curl -X POST localhost:8080/debiting -d '{"user_id": 1, "amount": 30000}'
curl -X POST localhost:8080/transfer -d '{"user_from_id": 1, "user_to_id": 2, "amount": 20000}'
curl localhost:8080/balance/1
```

Начисление, списание и получение баланса возвращают `{"balance": ...}`. В этом примере ответ перевода — `{"balance_user_from":50000,"balance_user_to":70000}`, последний запрос вернёт `{"balance":50000}`.

Ошибки обработчика возвращаются в JSON, например HTTP 409 с телом `{"error":"insufficient funds"}`. Неверные данные дают 400, отсутствующий пользователь — 404, нехватка средств или переполнение — 409, внутренняя ошибка — 500. Роутер сам отвечает текстом на неизвестные пути (404) и неподдерживаемые методы (405).

## gRPC

Чтобы переключить приложение с HTTP на gRPC:

```bash
TRANSPORT=grpc docker compose up --build
```

На том же порту 8080 запустится один gRPC-сервер. HTTP-маршруты, включая `/ping`, в этом режиме недоступны.

Сервис `balance.v1.BalanceService` предоставляет `GetBalance`, `Deposit`, `Withdraw` и `Transfer`. Поля запросов и ответов — в [balance.proto](api/balance/v1/balance.proto).

Пример через `grpcurl` из корня проекта. На macOS клиент можно установить командой `brew install grpcurl`:

```bash
grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_id": 1, "amount": 1000}' localhost:8080 balance.v1.BalanceService/Deposit

grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_id": 1}' localhost:8080 balance.v1.BalanceService/GetBalance
```

Для нового пользователя ответ — `{"balance":"1000"}`. `grpcurl` показывает `int64` строками, поля перевода — как `balanceUserFrom` и `balanceUserTo`. Сервер работает без TLS и reflection, поэтому нужны `-plaintext` и локальный `.proto`.

## Тесты

Сборка, статический анализ и тесты без PostgreSQL:

```bash
go build ./...
go vet ./...
go test -race -count=1 ./internal/service ./internal/transport/...
```

В service используется мок `Storage`, в тестах HTTP и gRPC — настоящий service и хранилище в памяти.

Интеграционные тесты требуют отдельную базу. Однократная подготовка с настройками из `.env.example`:

```bash
docker compose up -d --wait db
docker compose exec -T db psql -U balance -d balance -c 'CREATE DATABASE balance_test;'
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U balance -d balance_test < migrations/001_init.sql
```

Если `balance_test` уже существует, пропустите `CREATE DATABASE`. Затем загрузите `.env` с `TEST_DATABASE_URL` и запустите все тесты:

```bash
set -a; source .env; set +a
go test -race -count=1 -timeout=30s ./...
```

Без `TEST_DATABASE_URL` тесты PostgreSQL пропускаются. Они проверяют откат перевода, параллельные списания и встречные переводы. Перед каждым сценарием очищается `user_balance` в базе `balance_test`; несколько запусков на одной базе одновременно не поддерживаются.

