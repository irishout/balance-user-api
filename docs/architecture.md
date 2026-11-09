# Архитектура balance-user-api

Сервис разделён на транспортный, сервисный и слой хранения данных. Четыре операции доступны через HTTP JSON или gRPC. Переменная `TRANSPORT` выбирает один сервер на процесс; service и storage общие для обоих транспортов.

Инструкции по запуску, примеры запросов и команды проверки приведены в [README](../README.md).

## Оглавление

- [Слои и зависимости](#слои-и-зависимости)
  - [Структура пакетов](#структура-пакетов)
- [Модель данных и правила операций](#модель-данных-и-правила-операций)
  - [Схема PostgreSQL](#схема-postgresql)
- [Контракт Storage](#контракт-storage)
  - [Хранилище в памяти](#хранилище-в-памяти)
  - [PostgreSQL](#postgresql)
  - [Преобразование ошибок базы](#преобразование-ошибок-базы)
- [HTTP API](#http-api)
  - [Ошибки операций](#ошибки-операций)
- [gRPC](#grpc)
  - [Контракт и обработчики](#контракт-и-обработчики)
  - [Статусы ошибок](#статусы-ошибок)
  - [Выбор транспорта](#выбор-транспорта)
- [Конфигурация](#конфигурация)
- [Жизненный цикл сервера](#жизненный-цикл-сервера)
  - [Запуск и обработка запросов](#запуск-и-обработка-запросов)
  - [Graceful shutdown](#graceful-shutdown)
  - [Контексты](#контексты)
- [Развёртывание](#развёртывание)
- [Тестирование](#тестирование)

## Слои и зависимости

```text
Выбранный транспорт:
  HTTP → http.Server → LoggingMiddleware → ServeMux → transport.Handler
  gRPC → grpc.Server → grpcapi.Handler
                            ↓
                     service.Service
                            ↓
                     service.Storage
                       ├→ local.MapStorage
                       └→ postgresdb.PostgresStorage → pgxpool → PostgreSQL
```

В процессе активна одна из двух транспортных ветвей.

| Компонент | Ответственность |
|---|---|
| `cmd/app` | Чтение конфигурации, создание зависимостей, запуск и остановка сервера |
| `internal/transport` | HTTP-маршрутизация, декодирование запросов, формирование ответов и кодов ошибок |
| `internal/transport/grpcapi` | Реализация RPC, формирование Protobuf-ответов и статусов gRPC |
| `internal/service` | Проверка входных параметров операций, вызов интерфейса `Storage` |
| `internal/storage/local` | Хранение балансов в памяти, атомарность через мьютекс |
| `internal/storage/postgresdb` | SQL-операции, транзакции и преобразование ошибок PostgreSQL |
| `internal/domain` | Общие ошибки предметной области |
| `api/balance/v1` | Контракт Protobuf |
| `gen/go/balance/v1` | Сгенерированные сообщения, gRPC-клиент и серверный интерфейс |

Зависимости создаются в `main` через конструкторы. Интерфейс `Storage` объявлен в пакете-потребителе `service`. Хранилища импортируют `domain` и не зависят от транспортного или сервисного слоя.

### Структура пакетов

```text
cmd/app/main.go
api/balance/v1/balance.proto
gen/go/balance/v1/
  balance.pb.go
  balance_grpc.pb.go
internal/
  domain/errors.go
  service/
    balance.go
    balance_test.go
  transport/
    handler.go
    handler_test.go
    router.go
    middleware.go
    grpcapi/
      handler.go
      handler_test.go
  storage/
    local/map_storage.go
    postgresdb/
      postgres_storage.go
      postgres_storage_test.go
migrations/001_init.sql
```

## Модель данных и правила операций

Деньги представлены целым числом копеек: `int64` в Go и `BIGINT` в PostgreSQL. Валюта хранения — RUB. Пользователь определяется положительным `user_id`.

| Операция | Правила |
|---|---|
| Начисление | Положительные ID и сумма; отсутствующий пользователь создаётся; переполнение отклоняется |
| Списание | Положительные ID и сумма; пользователь существует; средств достаточно |
| Перевод | Положительные и разные ID, положительная сумма; оба пользователя существуют; достаточно средств у отправителя и места в `int64` у получателя |
| Получение баланса | Положительный ID; пользователь существует |

Service проверяет параметры запроса: положительные ID и сумму, различие участников перевода. Проверки текущего состояния — наличие пользователя, достаточность средств, переполнение — выполняются в storage атомарно с изменением данных.

### Схема PostgreSQL

Схема определена в [migrations/001_init.sql](../migrations/001_init.sql).

| Колонка | Тип | Ограничения |
|---|---|---|
| `user_id` | `BIGINT` | `PRIMARY KEY`, `user_id_positive CHECK (user_id > 0)` |
| `balance` | `BIGINT` | `NOT NULL DEFAULT 0`, `balance_non_negative CHECK (balance >= 0)` |

## Контракт Storage

```go
type Storage interface {
    Balance(ctx context.Context, userID int64) (int64, error)
    Deposit(ctx context.Context, userID, amount int64) (int64, error)
    Withdraw(ctx context.Context, userID, amount int64) (int64, error)
    Transfer(ctx context.Context, fromID, toID, amount int64) (int64, int64, error)
}
```

Методы возвращают балансы после успешной операции и ошибки из `domain` для ожидаемых отказов. Остальные ошибки передаются вызывающему слою. Перевод атомарно изменяет оба счёта.

### Хранилище в памяти

`MapStorage` содержит `map[int64]int64` и один `sync.Mutex`. Каждый метод удерживает мьютекс на протяжении чтения, проверок и изменений. Все проверки перевода выполняются до изменения балансов.

Данные принадлежат процессу и не разделяются между экземплярами приложения. Параметр `ctx` принимается для соответствия интерфейсу и не используется.

### PostgreSQL

`PostgresStorage` получает `*pgxpool.Pool` через конструктор и передаёт контекст операции в pgx.

| Метод | Реализация | Атомарность |
|---|---|---|
| `Balance` | `SELECT balance ... WHERE user_id = $1` | Один запрос |
| `Deposit` | `INSERT ... ON CONFLICT (user_id) DO UPDATE SET balance = user_balance.balance + EXCLUDED.balance RETURNING balance` | Создание и начисление выполняются одним запросом |
| `Withdraw` | `UPDATE ... SET balance = balance - $2 WHERE user_id = $1 RETURNING balance` | `UPDATE` блокирует строку, ограничение `CHECK` запрещает отрицательный результат |
| `Transfer` | Транзакция, блокировка участников, два `UPDATE ... RETURNING`, `COMMIT` | Оба изменения фиксируются вместе; при ошибке предусмотрен `Rollback` |

При переводе строки блокируются запросом `SELECT ... WHERE user_id IN ($1, $2) ORDER BY user_id FOR UPDATE`. Единый порядок по `user_id` предотвращает циклическое ожидание блокировок между встречными переводами. Все запросы перевода выполняются через `tx`.

### Преобразование ошибок базы

| Ошибка pgx / PostgreSQL | Ошибка предметной области |
|---|---|
| `pgx.ErrNoRows` при чтении или обновлении | `ErrNotFound`; в переводе — `ErrSenderNotFound` или `ErrRecipientNotFound` |
| Нарушение ограничения `balance_non_negative` | `ErrInsufficientFunds` |
| SQLSTATE `22003`, переполнение `BIGINT` | `ErrBalanceOverflow` |
| Прочие ошибки | Возвращаются без изменения |

Ошибки проверяются через `errors.Is` и `errors.As`. Для ограничения используется `PgError.ConstraintName`, для переполнения — `PgError.Code`. Текст сообщения PostgreSQL не участвует в классификации.

## HTTP API

Роутер использует шаблоны `net/http` вида `METHOD /path`. Операции с балансом возвращают JSON.

| Метод и путь | Запрос | Ответ 200 |
|---|---|---|
| `POST /accrual` | `{"user_id": 12, "amount": 1000}` | `{"balance": 1500}` |
| `POST /debiting` | `{"user_id": 12, "amount": 1000}` | `{"balance": 500}` |
| `POST /transfer` | `{"user_from_id": 12, "user_to_id": 15, "amount": 1000}` | `{"balance_user_from": 500, "balance_user_to": 1900}` |
| `GET /balance/{user_id}` | ID из пути | `{"balance": 1500}` |
| `GET /ping` | Без тела | Текст `pong` |

Значения балансов в таблице иллюстрируют формат ответа и зависят от исходных данных. Последовательные [примеры запросов](../README.md#http-api) приведены в README.

Декодер JSON использует `DisallowUnknownFields`. Ошибки синтаксиса, типов и неизвестные поля приводят к 400 `invalid JSON body`. Пропущенные числовые поля принимают нулевое значение и отклоняются сервисной валидацией. ID из пути разбирается через `strconv.ParseInt`; ошибка разбора даёт 400 `user_id must be a number`.

Обработчик устанавливает `Content-Type: application/json`, записывает HTTP-код, затем тело. Ошибка кодирования ответа логируется. Ответы роутера 404 и 405 имеют текстовый формат.

### Ошибки операций

Формат JSON-ошибки — `{"error": "текст"}`. `errorHandler` сопоставляет ошибки с HTTP-кодами через `errors.Is`.

| Ошибка | Источник | HTTP-код | Текст ответа |
|---|---|---|---|
| `ErrInvalidUserID` | Service | 400 | `user_id must be greater than 0` |
| `ErrInvalidSenderID` | Service | 400 | `user_from_id must be greater than 0` |
| `ErrInvalidRecipientID` | Service | 400 | `user_to_id must be greater than 0` |
| `ErrInvalidAmount` | Service | 400 | `amount must be greater than 0` |
| `ErrSelfTransfer` | Service | 400 | `user_to_id and user_from_id must be different` |
| `ErrNotFound` | Storage | 404 | `user_id not found` |
| `ErrSenderNotFound` | Storage | 404 | `user_from_id not found` |
| `ErrRecipientNotFound` | Storage | 404 | `user_to_id not found` |
| `ErrInsufficientFunds` | Storage | 409 | `insufficient funds` |
| `ErrBalanceOverflow` | Storage | 409 | `balance overflow` |
| Неизвестная ошибка | Любой слой | 500 | `internal error` |

Ошибки предметной области объявлены в `internal/domain` и не содержат HTTP-кодов или имён JSON-полей. Публичные тексты формирует транспортный слой.

## gRPC

### Контракт и обработчики

[balance.proto](../api/balance/v1/balance.proto) определяет четыре unary RPC сервиса `balance.v1.BalanceService`. Числовые поля сообщений имеют тип `int64`.

| RPC | Сообщение запроса и поля | Сообщение ответа и поля |
|---|---|---|
| `GetBalance` | `BalanceRequest`: `user_id` | `BalanceResponse`: `balance` |
| `Deposit` | `DepositRequest`: `user_id`, `amount` | `BalanceResponse`: `balance` |
| `Withdraw` | `WithdrawRequest`: `user_id`, `amount` | `BalanceResponse`: `balance` |
| `Transfer` | `TransferRequest`: `user_from_id`, `user_to_id`, `amount` | `TransferResponse`: `balance_user_from`, `balance_user_to` |

Пространство имён Protobuf — `balance.v1`. Путь импорта Go — `github.com/irishout/balance-user-api/gen/go/balance/v1`; имя сгенерированного Go-пакета — `v1`.

| Файл | Содержимое |
|---|---|
| `balance.pb.go` | Типы сообщений и поддержка Protobuf |
| `balance_grpc.pb.go` | `BalanceServiceClient`, `BalanceServiceServer`, `UnimplementedBalanceServiceServer`, функция регистрации `RegisterBalanceServiceServer` |

`grpcapi.Handler` содержит `*service.Service` и встраивает `UnimplementedBalanceServiceServer` по значению. Методы читают поля запроса через сгенерированные getters, передают параметры и входящий контекст в service. При успехе возвращается Protobuf-сообщение, при ошибке — `nil` и результат `toGRPCError`.

Контракт и результат генерации версионируются в Git. Изменения контрактов сопровождаются повторной генерацией; ручные изменения сгенерированных файлов не предусмотрены. Dockerfile использует готовые Go-исходники. Команды генерации находятся в [README](../README.md#генерация-кода).

### Статусы ошибок

`toGRPCError` проверяет ошибки через `errors.Is`. Ошибки service и storage не зависят от транспорта.

| Ошибка | Статус gRPC | Сообщение |
|---|---|---|
| `ErrInvalidUserID` | `InvalidArgument` | `user_id must be greater than 0` |
| `ErrInvalidSenderID` | `InvalidArgument` | `user_from_id must be greater than 0` |
| `ErrInvalidRecipientID` | `InvalidArgument` | `user_to_id must be greater than 0` |
| `ErrInvalidAmount` | `InvalidArgument` | `amount must be greater than 0` |
| `ErrSelfTransfer` | `InvalidArgument` | `user_to_id and user_from_id must be different` |
| `ErrNotFound` | `NotFound` | `user_id not found` |
| `ErrSenderNotFound` | `NotFound` | `user_from_id not found` |
| `ErrRecipientNotFound` | `NotFound` | `user_to_id not found` |
| `ErrInsufficientFunds` | `FailedPrecondition` | `insufficient funds` |
| `ErrBalanceOverflow` | `OutOfRange` | `balance overflow` |
| `context.Canceled` | `Canceled` | `request canceled` |
| `context.DeadlineExceeded` | `DeadlineExceeded` | `request deadline exceeded` |
| Неизвестная ошибка | `Internal` | `internal error` |

При отсутствии ошибки `toGRPCError` возвращает `nil`. HTTP-handler не выделяет отмену и дедлайн в отдельные статусы: они попадают в общую ветвь HTTP 500.

### Выбор транспорта

Service и storage создаются до выбора `TRANSPORT=http|grpc`. В зависимости от значения `main` создаёт HTTP- или gRPC-handler и запускает соответствующий сервер на `PORT` (по умолчанию 8080). Отсутствующее или неизвестное значение `TRANSPORT` завершает приложение с ошибкой.

gRPC-сервер работает без TLS и server reflection. Клиент `grpcurl` использует локальный `.proto`; JSON в его вводе и выводе — представление Protobuf-сообщений. На сетевом уровне используется Protobuf. HTTP-маршруты, в том числе `/ping`, доступны только при `TRANSPORT=http`.

## Конфигурация

| Переменная | Поведение |
|---|---|
| `STORAGE` | Обязательный выбор `postgres` или `memory`; неизвестное значение завершает приложение с ошибкой |
| `DATABASE_URL` | Обязательна при `STORAGE=postgres` |
| `PORT` | Порт выбранного HTTP- или gRPC-сервера, по умолчанию 8080 |
| `TEST_DATABASE_URL` | Подключение интеграционных тестов к базе `balance_test` |
| `TRANSPORT` | Обязательный выбор `http` или `grpc`; неизвестное значение завершает приложение с ошибкой |

Конфигурация читается из окружения процесса. Локальный `.env` загружается оболочкой; Compose использует его для подстановок. При запуске в режиме PostgreSQL `main` создаёт пул и проверяет соединение через `Ping`. Ошибка подключения завершает приложение; перехода на другое хранилище нет.

## Жизненный цикл сервера

### Запуск и обработка запросов

`main` подписывается на `SIGINT` и `SIGTERM` через `signal.NotifyContext`. Выбранный сервер запускается в отдельной горутине, а `main` ожидает закрытия `stopCtx.Done()`.

| Транспорт | Запуск |
|---|---|
| HTTP | `http.Server` с адресом `":" + port`, роутер обёрнут в `LoggingMiddleware`, вызов `ListenAndServe` |
| gRPC | `grpc.NewServer`, регистрация handler через `RegisterBalanceServiceServer`, `net.Listen("tcp", ":" + port)`, вызов `Serve` |

HTTP-middleware записывает метод, путь и длительность после возврата из следующего обработчика. Логируются также обращения, для которых роутер возвращает 404 или 405. HTTP-код ответа в лог не включён. Для gRPC interceptor логирования не реализован.

### Graceful shutdown

После получения сигнала создаётся отдельный `shutdownCtx` от `context.Background()` с таймаутом 10 секунд.

| Транспорт | Корректное завершение | Принудительное завершение |
|---|---|---|
| HTTP | `Shutdown` закрывает слушающий сокет и простаивающие соединения, ожидает активные обработчики | При ошибке `Shutdown` вызывается `Close` |
| gRPC | `GracefulStop` в отдельной горутине прекращает приём новых RPC и ожидает активные; после возврата закрывается канал `stopped` | Если дедлайн наступает раньше закрытия `stopped`, вызывается `Stop` |

`http.ErrServerClosed` и `grpc.ErrServerStopped` исключены из аварийного завершения. Ошибки открытия gRPC-слушателя и остальные ошибки запуска серверов приводят к `log.Fatal`.

При штатном возврате из `main` пул PostgreSQL закрывается через `defer pool.Close()`. Лимит 10 секунд относится к ожиданию выбранного сервера, а не ко всему времени завершения процесса.

### Контексты

| Контекст | Назначение |
|---|---|
| Стартовый `context.Background()` | Создание пула и проверка PostgreSQL |
| Контекст запроса: `r.Context()` в HTTP или `ctx` RPC-метода | Передаётся через service и storage в pgx; несёт отмену и дедлайн клиента |
| `stopCtx` | Получение сигнала остановки в `main` |
| `shutdownCtx` | Ограниченное ожидание завершения HTTP-запросов или RPC после сигнала |

Контекст передаётся первым параметром методов. Контекст запроса и контекст остановки разделены, чтобы получение системного сигнала не отменяло активные запросы до истечения периода ожидания.

## Развёртывание

Dockerfile использует многоэтапную сборку: `golang:1.26-alpine` компилирует бинарник с `CGO_ENABLED=0`, затем бинарник копируется в `alpine:3.22`.

Compose запускает два сервиса:

| Сервис | Настройки |
|---|---|
| `app` | Сборка из Dockerfile, порт 8080, `STORAGE=postgres`, `TRANSPORT` из окружения Compose, подключение к `db:5432` |
| `db` | `postgres:18`, проброс `5433:5432`, healthcheck через `pg_isready`, том `pgdata` |

```text
HTTP- или gRPC-клиент → localhost:8080 → app:8080
                                         │
                                         └→ db:5432 → pgdata
psql на хосте       → localhost:5433 ──────→ db:5432
```

`app` зависит от состояния `service_healthy` базы. SQL-схема подключена в `/docker-entrypoint-initdb.d/001_init.sql` и выполняется при инициализации пустого каталога данных. Именованный том PostgreSQL 18 подключён к `/var/lib/postgresql`. Удаление контейнеров сохраняет данные; `docker compose down -v` удаляет том.

## Тестирование

| Уровень | Зависимости | Проверки |
|---|---|---|
| Service | Мок `Storage` | Валидация параметров, передача аргументов, результатов и ошибок |
| HTTP | Service и `MapStorage`, `httptest` | Успешные ответы четырёх операций, выбранные ошибки, HTTP-коды и JSON |
| gRPC | Service и `MapStorage`, Protobuf-сообщения | Четыре операции, статусы ошибок, возвращаемые и сохранённые балансы |
| PostgreSQL | Отдельная база `balance_test` | Откат перевода, параллельные списания, встречные переводы |

Тесты service используют таблицы сценариев и `t.Run`. HTTP-тесты баланса проходят через роутер для заполнения `r.PathValue`; остальные вызывают метод handler напрямую.

gRPC-тесты вызывают handler напрямую, без сетевого сервера. Они проверяют нулевой баланс, новое и повторное начисление, частичное и полное списание, успешный перевод и отказы без изменения состояния. Для `toGRPCError` проверяются четыре базовых случая: `nil`, неверный ID, отсутствующий пользователь и неизвестная ошибка; полного перебора ветвей нет.

Интеграционные тесты проверяют имя базы перед очисткой таблицы через `TRUNCATE`. Тесты выполняются последовательно; параллельность организована внутри отдельных сценариев через горутины и `sync.WaitGroup`. Результат каждой операции записывается в отдельный элемент среза и проверяется после завершения всех горутин.

| Сценарий | Ожидаемый результат |
|---|---|
| Перевод 300 отсутствующему получателю с начального баланса 1000 | `ErrRecipientNotFound`, баланс отправителя 1000 |
| 200 списаний по 1000 с баланса 100000 | 100 успешных операций, 100 `ErrInsufficientFunds`, баланс 0 |
| По 100 переводов в обоих направлениях между балансами по 1000 | 200 успешных операций, конечные балансы по 1000 |

У встречных переводов общий дедлайн 10 секунд. Без `TEST_DATABASE_URL` интеграционные тесты пропускаются; ошибки подключения приводят к неуспешному результату. Подготовка базы и [команды тестирования](../README.md#тестирование) приведены в README.
