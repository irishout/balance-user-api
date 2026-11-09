# balance-user-api

Сервис управления балансами пользователей: начисление, списание, перевод и получение баланса. Валюта хранения — рубль, единицы измерения — копейки (`int64`). Поддерживаются PostgreSQL и хранилище в памяти.

Реализованы HTTP JSON API и gRPC для четырёх операций, Docker Compose, логирование HTTP-запросов, graceful shutdown, unit- и интеграционные тесты. Транспорт выбирается через `TRANSPORT=http|grpc`: один сервер на процесс, общий сервисный слой и хранилище.

[Условие задания](docs/task.md) · [Оригинал задания Авито](https://github.com/avito-tech/job-backend-trainee-assignment) · [Архитектура](docs/architecture.md)

## Оглавление

- [Стек](#стек)
- [Запуск](#запуск)
  - [Docker Compose](#docker-compose)
  - [Локальный запуск](#локальный-запуск)
  - [Конфигурация](#конфигурация)
  - [Логирование и остановка](#логирование-и-остановка)
- [HTTP API](#http-api)
  - [Начисление](#начисление)
  - [Списание](#списание)
  - [Перевод](#перевод)
  - [Баланс](#баланс)
  - [Ошибки](#ошибки)
- [gRPC](#grpc)
  - [Вызовы через grpcurl](#вызовы-через-grpcurl)
  - [Ошибки RPC](#ошибки-rpc)
  - [Генерация кода](#генерация-кода)
- [Тестирование](#тестирование)
  - [Service и HTTP](#service-и-http)
  - [gRPC-обработчики](#grpc-обработчики)
  - [PostgreSQL](#postgresql)
  - [Общие проверки](#общие-проверки)
- [Вопросы по ТЗ и решения](#вопросы-по-тз-и-решения)
- [Структура проекта](#структура-проекта)
- [План развития](#план-развития)

## Стек

- Go 1.26.3 или новее, стандартная библиотека `net/http`.
- PostgreSQL 18, [pgx v5](https://github.com/jackc/pgx), пул соединений `pgxpool`.
- Docker и Docker Compose.
- gRPC и Protocol Buffers.

## Запуск

### Docker Compose

Требуется Docker с поддержкой Compose. При первом запуске создаётся локальный файл конфигурации:

```bash
cp .env.example .env
docker compose up --build
```

| Сервис | Назначение | Адрес с хостовой машины |
|---|---|---|
| `app` | HTTP API или gRPC, в зависимости от `TRANSPORT` | `localhost:8080` |
| `db` | PostgreSQL | `localhost:5433` |

Приложение запускается после успешного healthcheck базы. Схема [migrations/001_init.sql](migrations/001_init.sql) применяется при инициализации пустого каталога данных PostgreSQL. Данные хранятся в именованном томе `pgdata`.

| Команда | Результат |
|---|---|
| `docker compose down` | Удаление контейнеров с сохранением данных |
| `docker compose down -v` | Удаление контейнеров и тома с данными |

По умолчанию в `.env.example` задан `TRANSPORT=http`. Проверка HTTP-сервера:

```bash
curl localhost:8080/ping
```

Ответ — `pong`. Для запуска gRPC вместо HTTP:

```bash
TRANSPORT=grpc docker compose up --build
```

Оба транспорта используют порт 8080. В режиме gRPC HTTP-маршруты, включая `/ping`, недоступны; примеры вызовов — в разделе [gRPC](#grpc).

### Локальный запуск

Требуется Go 1.26.3 или новее. Для запуска приложения на хостовой машине с PostgreSQL из Compose:

```bash
docker compose up -d --wait db
set -a; source .env; set +a
go run ./cmd/app
```

Команды выполняются из корня проекта после создания `.env`. Переменные загружаются в окружение текущего терминала. Одновременно запущенный контейнер `app` использует тот же порт 8080.

Запуск с хранилищем в памяти:

```bash
STORAGE=memory TRANSPORT=http go run ./cmd/app
```

Для локального gRPC-сервера задаётся `TRANSPORT=grpc`. В режиме `memory` данные пропадают при завершении процесса. Неизвестное или отсутствующее значение `STORAGE`, некорректная конфигурация и недоступность PostgreSQL при `STORAGE=postgres` приводят к завершению приложения с ошибкой. Автоматического переключения между хранилищами нет.

### Конфигурация

| Переменная | Назначение | Значение |
|---|---|---|
| `STORAGE` | Хранилище, обязательная настройка | `postgres` или `memory` |
| `DATABASE_URL` | Подключение приложения к PostgreSQL | Обязательна при `STORAGE=postgres` |
| `PORT` | Порт выбранного сервера, HTTP или gRPC | По умолчанию `8080` |
| `TEST_DATABASE_URL` | Подключение интеграционных тестов | Отдельная база `balance_test` |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Инициализация PostgreSQL и строка подключения в Compose | Пример — `balance` |
| `TRANSPORT` | Транспорт, обязательная настройка | `http` или `grpc`; отсутствующее или неизвестное значение завершает приложение с ошибкой |

Шаблон конфигурации — [.env.example](.env.example). Локальный `.env` исключён из Git. Go не читает этот файл автоматически.

Для подключения с хостовой машины используется `localhost:5433`; контейнер `app` подключается к `db:5432` во внутренней сети Compose. Строка подключения внутри контейнера формируется из переменных `POSTGRES_*`.

### Логирование и остановка

В HTTP-режиме middleware записывает HTTP-метод, путь и длительность обработки каждого запроса:

```bash
docker compose logs -f app
```

При `SIGINT` или `SIGTERM` приложение начинает корректную остановку выбранного сервера:

- HTTP: `Shutdown` ожидает активные обработчики до 10 секунд; при ошибке вызывается `Close`.
- gRPC: `GracefulStop` ожидает активные RPC; через 10 секунд ожидания вызывается принудительный `Stop`.

При штатном выходе из `main` закрывается пул PostgreSQL. Лимит 10 секунд относится к ожиданию сервера. Логирование отдельных gRPC-запросов через interceptor не реализовано.

Остановка приложения с сохранением работающей базы:

```bash
docker compose stop -t 15 app
```

## HTTP API

Суммы передаются целыми числами в копейках: `100000` = 1000 ₽. Операции с балансом принимают и возвращают JSON. Формат ошибки обработчика — `{"error": "текст"}` с соответствующим HTTP-кодом.

| метод | путь | что делает |
|---|---|---|
| `POST` | `/accrual` | начисляет деньги пользователю |
| `POST` | `/debiting` | списывает деньги |
| `POST` | `/transfer` | переводит деньги от одного пользователя другому |
| `GET` | `/balance/{user_id}` | возвращает текущий баланс |

Примеры ниже выполняются по порядку на пустой базе.

### Начисление

```bash
curl -X POST localhost:8080/accrual -d '{"user_id": 1, "amount": 100000}'
```

```json
{"balance":100000}
```

Пользователь создаётся при первом начислении. Начальный баланс второго участника для примера перевода:

```bash
curl -X POST localhost:8080/accrual -d '{"user_id": 2, "amount": 50000}'
```

```json
{"balance":50000}
```

### Списание

```bash
curl -X POST localhost:8080/debiting -d '{"user_id": 1, "amount": 30000}'
```

```json
{"balance":70000}
```

### Перевод

```bash
curl -X POST localhost:8080/transfer -d '{"user_from_id": 1, "user_to_id": 2, "amount": 20000}'
```

```json
{"balance_user_from":50000,"balance_user_to":70000}
```

### Баланс

```bash
curl localhost:8080/balance/1
```

```json
{"balance":50000}
```

### Ошибки

| ситуация | код | `error` |
|---|---|---|
| битый JSON или неизвестное поле | 400 | `invalid JSON body` |
| `user_id` в пути не число | 400 | `user_id must be a number` |
| id или сумма ≤ 0 | 400 | `user_id must be greater than 0`, `amount must be greater than 0`; в переводе — `user_from_id …`, `user_to_id …` |
| перевод самому себе | 400 | `user_to_id and user_from_id must be different` |
| пользователь не найден | 404 | `user_id not found`; в переводе — `user_from_id not found`, `user_to_id not found` |
| недостаточно средств | 409 | `insufficient funds` |
| баланс превысит максимум `int64` | 409 | `balance overflow` |
| внутренняя ошибка | 500 | `internal error` |

Например, попытка списать больше, чем есть на балансе:

```bash
curl -X POST localhost:8080/debiting -d '{"user_id": 1, "amount": 1000000}'
```

```json
{"error":"insufficient funds"}
```

Неизвестные пути и неподдерживаемые HTTP-методы обрабатывает `http.ServeMux`: ответы 404 и 405 имеют текстовое тело. `GET /ping` возвращает текст `pong`.

## gRPC

Контракт [api/balance/v1/balance.proto](api/balance/v1/balance.proto) определяет четыре unary RPC сервиса `balance.v1.BalanceService`. Все суммы и балансы — `int64` в копейках.

| RPC | Запрос | Успешный ответ |
|---|---|---|
| `GetBalance` | `BalanceRequest`: `user_id` | `BalanceResponse`: `balance` |
| `Deposit` | `DepositRequest`: `user_id`, `amount` | `BalanceResponse`: `balance` |
| `Withdraw` | `WithdrawRequest`: `user_id`, `amount` | `BalanceResponse`: `balance` |
| `Transfer` | `TransferRequest`: `user_from_id`, `user_to_id`, `amount` | `TransferResponse`: `balance_user_from`, `balance_user_to` |

Обработчики находятся в [internal/transport/grpcapi](internal/transport/grpcapi), сгенерированные сообщения и клиент — в [gen/go/balance/v1](gen/go/balance/v1). Оба транспорта используют один `service.Service` и одинаковые правила операций.

### Вызовы через grpcurl

Сервер работает без TLS и server reflection. Для вызовов требуется локальный `.proto` и флаг `-plaintext`. Установка клиента на macOS:

```bash
brew install grpcurl
```

Следующие команды выполняются из корня проекта при `TRANSPORT=grpc` на пустом хранилище. Создание двух пользователей:

```bash
grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_id": 1, "amount": 100000}' localhost:8080 balance.v1.BalanceService/Deposit

grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_id": 2, "amount": 50000}' localhost:8080 balance.v1.BalanceService/Deposit
```

Списание и перевод:

```bash
grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_id": 1, "amount": 30000}' localhost:8080 balance.v1.BalanceService/Withdraw

grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_from_id": 1, "user_to_id": 2, "amount": 20000}' localhost:8080 balance.v1.BalanceService/Transfer
```

Ответ перевода в представлении ProtoJSON:

```json
{"balanceUserFrom":"50000","balanceUserTo":"70000"}
```

Получение баланса:

```bash
grpcurl -plaintext -import-path . -proto api/balance/v1/balance.proto \
  -d '{"user_id": 1}' localhost:8080 balance.v1.BalanceService/GetBalance
```

```json
{"balance":"50000"}
```

`grpcurl` принимает и выводит JSON, а по сети сообщения передаются в формате Protobuf. В ProtoJSON значения `int64` выводятся строками, имена полей — в camelCase; входные имена из `.proto`, например `user_from_id`, также поддерживаются.

### Ошибки RPC

| Причина | Статус gRPC |
|---|---|
| Неположительный ID или сумма, перевод самому себе | `InvalidArgument` |
| Пользователь, отправитель или получатель не найден | `NotFound` |
| Недостаточно средств | `FailedPrecondition` |
| Переполнение баланса | `OutOfRange` |
| Отмена контекста операции | `Canceled` |
| Истечение дедлайна операции | `DeadlineExceeded` |
| Неизвестная ошибка | `Internal` |

Неуспешный RPC возвращает статус с сообщением; успешное сообщение ответа отсутствует. Контекст входящего RPC передаётся через service в storage и pgx.

### Генерация кода

Используемые версии инструментов: `protoc` 36.2, `protoc-gen-go` v1.36.12, `protoc-gen-go-grpc` v1.6.2.

Установка на macOS:

```bash
brew install protobuf
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.12
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
export PATH="$PATH:$(go env GOPATH)/bin"
```

Генерация из корня проекта:

```bash
protoc \
  --go_out=. \
  --go_opt=module=github.com/irishout/balance-user-api \
  --go-grpc_out=. \
  --go-grpc_opt=module=github.com/irishout/balance-user-api \
  api/balance/v1/balance.proto
```

| Файл | Содержимое |
|---|---|
| [balance.pb.go](gen/go/balance/v1/balance.pb.go) | Типы сообщений и поддержка Protobuf |
| [balance_grpc.pb.go](gen/go/balance/v1/balance_grpc.pb.go) | Клиент, серверный интерфейс и регистрация RPC |

Контракт и оба сгенерированных файла хранятся в Git. Изменение `.proto` сопровождается повторной генерацией. Ручное редактирование `.pb.go` не предусмотрено. Dockerfile собирает готовые Go-исходники без запуска генераторов.

Установка инструментов через `go install` не изменяет зависимости приложения. Сгенерированный пакет требует библиотеки `google.golang.org/protobuf` и `google.golang.org/grpc`. При изменении импортов после генерации зависимости синхронизируются и сохраняются в `go.mod` и `go.sum`:

```bash
go mod tidy
go build ./...
```

## Тестирование

Команды выполняются из корня проекта. Требуется Go 1.26.3 или новее.

### Service и HTTP

Unit-тесты service используют мок `Storage`. HTTP-тесты используют `httptest`, настоящий service и хранилище в памяти. Запущенный сервер и PostgreSQL не требуются.

```bash
go test -race ./internal/service ./internal/transport
go test -v ./internal/service
go test -run '^TestBalance' -v ./internal/transport
```

### gRPC-обработчики

Тесты напрямую вызывают методы gRPC-handler с Protobuf-сообщениями, настоящим service и `MapStorage`. Сетевой сервер и PostgreSQL не требуются.

```bash
go test -race -count=1 -v ./internal/transport/grpcapi
```

Проверяются четыре операции, нулевой баланс, создание и повторное начисление, списание всего остатка, статусы отказов и сохранение балансов при неуспешных изменениях. Для `toGRPCError` проверяются базовые случаи: отсутствие ошибки, неверный ID, отсутствующий пользователь и неизвестная ошибка. Эти тесты не проверяют сетевой обмен или все ветви преобразования ошибок.

### PostgreSQL

Интеграционные тесты работают с отдельной базой `balance_test`. Однократная подготовка базы с настройками из `.env.example`:

```bash
docker compose up -d --wait db
docker compose exec -T db psql -U balance -d balance -c 'CREATE DATABASE balance_test;'
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U balance -d balance_test < migrations/001_init.sql
```

Если база уже создана, команда `CREATE DATABASE` пропускается. Для другой конфигурации меняются имя пользователя и основной базы в командах.

Подключение задаётся в `.env`:

```dotenv
TEST_DATABASE_URL=postgres://balance:balance@localhost:5433/balance_test
```

Запуск:

```bash
set -a; source .env; set +a
go test -race -count=1 -timeout=30s -v ./internal/storage/postgresdb
```

Один сценарий:

```bash
go test -race -count=1 -timeout=30s -v -run '^TestWithdrawConcurrent$' ./internal/storage/postgresdb
```

| Тест | Проверка |
|---|---|
| `TestTransferRollbackRecipientNotFound` | Откат списания при отсутствии получателя; начальный баланс отправителя 1000 сохраняется |
| `TestWithdrawConcurrent` | 200 списаний по 1000 с баланса 100000: 100 успешных операций, 100 `ErrInsufficientFunds`, баланс 0 |
| `TestTransferConcurrentOppositeDirections` | По 100 переводов в каждом направлении: 200 успешных операций, конечные балансы по 1000 |

Перед каждым тестом проверяется имя базы `balance_test` и выполняется `TRUNCATE TABLE user_balance`. Тесты удаляют содержимое этой таблицы и требуют последовательного запуска на одной базе, включая запуск из разных процессов.

Без `TEST_DATABASE_URL` интеграционные тесты получают `SKIP`. Ошибки подключения, схемы и неверное имя базы приводят к неуспешному результату теста. Загрузка `.env` требуется и при запуске тестов из редактора.

### Общие проверки

```bash
go build ./...
go vet ./...
go test -race -count=1 -timeout=30s ./...
```

С заданным `TEST_DATABASE_URL` выполняются и интеграционные тесты; без переменной они пропускаются. Флаг `-count=1` отключает кеш результатов, `-timeout=30s` ограничивает время работы каждого тестового процесса.

## Вопросы по ТЗ и решения

| Вопрос | Решение |
|---|---|
| Объединять ли начисление и списание? | Отдельные методы с положительной суммой и собственными наборами ошибок. |
| Как хранить рублёвый баланс? | Целое число копеек в `int64` / `BIGINT`. Дробные суммы в запросах отклоняются; вычисления не используют `float`. |
| Когда создаётся пользователь? | При первом начислении. Остальные операции для отсутствующего пользователя возвращают HTTP 404 / gRPC `NotFound`; перевод получателя не создаёт. |
| Как отвечать при нехватке средств? | HTTP 409: операция конфликтует с текущим состоянием баланса. Ошибки входных данных — HTTP 400. |
| Как обрабатывать переполнение баланса? | HTTP 409 `balance overflow`; изменение баланса не применяется. |
| Допустим ли перевод самому себе? | Нет, HTTP 400. |
| Допустимы ли неизвестные поля JSON? | Нет, HTTP 400. |
| Как обеспечивается корректность при параллельных запросах? | В памяти — `sync.Mutex`; в PostgreSQL — атомарные SQL-операции, `CHECK (balance >= 0)`, транзакция перевода и блокировки строк в порядке `user_id`. |
| Нужен ли механизм миграций? | По ТЗ достаточно итоговой схемы. Используется `migrations/001_init.sql`, автоматически применяемый при инициализации базы. |
| Как обеспечивается готовность базы при старте? | Compose ожидает healthcheck PostgreSQL; приложение дополнительно выполняет `Ping` и завершается при ошибке подключения. |

## Структура проекта

```text
cmd/app/                точка входа, конфигурация, запуск и остановка сервера
api/balance/v1/         контракт Protobuf
gen/go/balance/v1/      сгенерированные сообщения, клиент и серверный интерфейс
internal/transport/     HTTP-обработчики, роутер и middleware
  grpcapi/              gRPC-обработчики и преобразование статусов
internal/service/       валидация операций и интерфейс Storage
internal/storage/
  postgresdb/           PostgreSQL
  local/                map и sync.Mutex
internal/domain/        общие ошибки
migrations/             SQL-схема
docs/                   условие задания и архитектура
Dockerfile              сборка образа приложения
docker-compose.yml      окружение app + db
.env.example            шаблон конфигурации
```

## План развития

- История операций с комментариями, пагинацией и сортировкой.
- Конвертация валют через внешний API и Redis для кеширования курсов.
- События операций через Kafka, transactional outbox и отдельный consumer.
