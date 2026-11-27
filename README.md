# balance-user-api

Сервис балансов с HTTP и gRPC. Поддерживает начисление, списание, переводы, получение баланса и историю операций. Суммы передаются целыми числами в копейках. [Тестовое задание Авито](docs/task.md)

Go 1.26.3 · net/http · pgx · PostgreSQL 18 · gRPC · Docker Compose

## Оглавление

- [Запуск](#запуск)
- [HTTP API](#http-api)
- [История операций](#история-операций)
- [gRPC](#grpc)
- [Тесты](#тесты)

Устройство слоёв, транзакции и генерация Protobuf описаны в [архитектуре](docs/architecture.md).

## Запуск

Для запуска нужен Docker Compose. Выполните команды из корня проекта.

```bash
cp .env.example .env
docker compose up --build
```

По умолчанию HTTP доступен на `localhost:8080`, PostgreSQL — на `localhost:5433`. Команда `curl localhost:8080/ping` возвращает `pong`.

При первом запуске с пустым томом Compose применяет обе [миграции](migrations). Если том создан до добавления истории, примените вторую миграцию к работающей базе.

```bash
docker compose exec -T db sh -c \
  'psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' \
  < migrations/002_operations.sql
```

Для локального запуска приложения нужен Go 1.26.3 или новее. Создайте `.env` и освободите порт `8080`, если его занимает контейнер `app`.

```bash
docker compose up -d --wait db
set -a; source .env; set +a
go run ./cmd/app
```

Запуск с хранилищем в памяти.

```bash
STORAGE=memory TRANSPORT=http go run ./cmd/app
```

При локальном запуске укажите `STORAGE` и `TRANSPORT`. Для `STORAGE=postgres` нужен `DATABASE_URL`. Порт `PORT` по умолчанию равен `8080`. [Пример настроек](.env.example)

## HTTP API

| Метод | Путь | Запрос |
|---|---|---|
| POST | `/accrual` | `{"user_id": 1, "amount": 100000}` |
| POST | `/debiting` | `{"user_id": 1, "amount": 30000}` |
| POST | `/transfer` | `{"user_from_id": 1, "user_to_id": 2, "amount": 20000}` |
| GET | `/balance/{user_id}` | Без тела |
| GET | `/operations/{user_id}` | Параметры истории в URL |

Пример начисления с комментарием и запроса истории.

```bash
curl -sS localhost:8080/accrual -H 'Content-Type: application/json' \
  -d '{"user_id":1,"amount":100000,"comment":"Пополнение картой"}'
curl -sS 'http://localhost:8080/operations/1?sort=amount&page_size=5'
```

Начисление, списание и чтение баланса возвращают `{"balance":100000}` с актуальной суммой. Перевод возвращает `balance_user_from` и `balance_user_to`. Ошибки API содержат поле `error`.

## История операций

Начисление, списание и перевод принимают необязательный `comment`. История доступна через HTTP и gRPC с хранилищем PostgreSQL. Перевод виден обоим участникам.

По умолчанию `page=1`, `page_size=20` и `sort=date`. `sort=date` показывает свежие операции первыми, `sort=amount` — операции с большей суммой первыми. HTTP возвращает массив в поле `operations`.

## gRPC

Для запуска gRPC на порту `8080` выполните команду.

```bash
TRANSPORT=grpc docker compose up --build
```

Сервис `balance.v1.BalanceService` предоставляет `GetBalance`, `Deposit`, `Withdraw`, `Transfer` и `Operations`. Для вызовов через `grpcurl` используйте `-plaintext` и локальный [balance.proto](api/balance/v1/balance.proto).

```bash
grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_id":1,"amount":1000,"comment":"Пополнение картой"}' \
  localhost:8080 balance.v1.BalanceService/Deposit

grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_id":1,"page_size":5,"sort":"amount"}' \
  localhost:8080 balance.v1.BalanceService/Operations
```

`grpcurl` выводит значения `int64` строками. Например, баланс нового пользователя после начисления выше будет `{"balance":"1000"}`.

## Тесты

Проверка сборки и запуск тестов.

```bash
go build ./...
go vet ./...
go test -race -count=1 -timeout=30s ./...
```

Без `TEST_DATABASE_URL` интеграционные тесты PostgreSQL пропускаются. Для их запуска подготовьте `balance_test` и примените обе миграции. Команды используют настройки из `.env.example`.

```bash
docker compose up -d --wait db
docker compose exec -T db psql -U balance -d balance -c 'CREATE DATABASE balance_test;'
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U balance -d balance_test < migrations/001_init.sql
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U balance -d balance_test < migrations/002_operations.sql
```

Если `balance_test` уже существует, пропустите `CREATE DATABASE`. Загрузите `.env` и запустите тесты повторно.

```bash
set -a; source .env; set +a
go test -race -count=1 -timeout=30s ./...
```

Интеграционные тесты очищают `user_balance` и `operations` в `balance_test` перед каждым сценарием. Запускайте их на этой базе по одному процессу.
