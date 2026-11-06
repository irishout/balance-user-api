# Архитектура balance-user-api

Все четыре операции реализованы для PostgreSQL и памяти. Запуск — через Docker Compose или локально. Есть тесты service, HTTP-обработчиков и PostgreSQL, middleware логирования и graceful shutdown. Следующий этап расширения — gRPC.

## Путь запроса

```
клиент (curl, другой сервис)
  │
  ▼
http.Server   слушает порт, на каждое соединение запускает горутину
  │
  ▼
middleware    запоминает время → вызывает роутер → пишет метод, путь и длительность
  │
  ▼
роутер        метод + путь → handler; 404 и 405 отвечает сам
  │
  ▼
handler       JSON ↔ структуры Go, ошибки → коды ответа; берёт ctx = r.Context()
  │
  ▼
service       бизнес-правила (проверка данных запроса)
  │
  ▼
storage       данные: PostgreSQL или map в памяти (выбирается в main)
```

`ctx` идёт первым параметром через все слои: handler → service → storage → pgx.

## Структура папок

```
.
├── cmd/
│   └── app/
│       └── main.go                 конфиг из env, сборка слоёв, запуск и остановка
├── internal/
│   ├── domain/
│   │   └── errors.go               ошибки, общие для всех слоёв
│   ├── transport/
│   │   ├── router.go
│   │   ├── handler.go
│   │   ├── middleware.go           логирование запросов
│   │   └── handler_test.go         HTTP-коды и JSON через httptest
│   ├── service/
│   │   ├── balance.go              Service + интерфейс Storage
│   │   └── balance_test.go         табличные тесты, мок Storage
│   └── storage/
│       ├── local/
│       │   └── map_storage.go      хранилище в памяти: map + sync.Mutex
│       └── postgresdb/
│           ├── postgres_storage.go хранилище в PostgreSQL через pgxpool
│           └── postgres_storage_test.go  интеграционные тесты с отдельной базой
├── migrations/
│   └── 001_init.sql                схема таблицы
├── Dockerfile                      сборка бинарника и образа сервиса
├── .dockerignore                   исключения из контекста сборки
├── docker-compose.yml              app + db, сеть, том, healthcheck
└── .env.example                    шаблон переменных окружения
```

Кто кого импортирует в рабочем коде (циклы в Go запрещены):

| пакет | импортирует |
|---|---|
| `cmd/app` | `transport`, `service`, `storage/local`, `storage/postgresdb`, `pgxpool` (собирает слои) |
| `transport` | `service`, `domain` |
| `service` | `domain` |
| `storage/local` | `domain` |
| `storage/postgresdb` | `domain`, `pgx` |
| `domain` | ничего из проекта |

Каждый пакет импортирует только пакеты ниже себя, поэтому циклов нет. `domain` можно импортировать из любого слоя. Про Postgres знают только `storage/postgresdb` и `cmd/app`.

## Конфигурация

Из переменных окружения. Локально они лежат в `.env` (в `.gitignore`), Go сам его не читает: `set -a; source .env; set +a`.

| переменная | значения | если не задана или неверная |
|---|---|---|
| `STORAGE` | `postgres` или `memory` | `log.Fatal` |
| `DATABASE_URL` | `postgres://user:pass@host:port/db` | при `STORAGE=postgres` — `log.Fatal` |
| `PORT` | номер порта HTTP-сервера | если не задана — `8080`; ошибка прослушивания порта завершает сервис |
| `TEST_DATABASE_URL` | подключение к отдельной базе `balance_test` | без переменной интеграционные тесты пропускаются; ошибка подключения или другое имя базы приводит к падению теста |

При `STORAGE=postgres` main создаёт пул и проверяет связь через `Ping`: `pgxpool.New` к базе не ходит. Если `Ping` не прошёл — `log.Fatal`.

В Compose `app` получает `STORAGE=postgres`, `PORT=8080` и строку подключения с хостом `db:5432`. Значения `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` подставляются из `.env` и используются для настройки базы и строки подключения сервиса.

## Docker и Compose

Dockerfile собирает Go-бинарник на этапе `build`, затем копирует его в образ на базе Alpine. База данных запускается отдельным контейнером из образа `postgres:18`.

```text
клиент на Mac → localhost:8080 → app:8080
                                  │
                                  └→ db:5432 → том pgdata

psql на Mac  → localhost:5433 ──────→ db:5432
```

`app` обращается к базе по имени сервиса `db` во внутренней сети Compose. Проброс `5433:5432` нужен для подключения с машины.

- `db` проверяется через `pg_isready`; `depends_on: condition: service_healthy` задерживает запуск `app` до готовности базы. При запуске сервис также вызывает `Ping`.
- `migrations/001_init.sql` подключён в `/docker-entrypoint-initdb.d/001_init.sql` и выполняется при инициализации пустого каталога данных.
- Именованный том `pgdata` подключён к `/var/lib/postgresql`. `docker compose down` сохраняет данные, `docker compose down -v` удаляет том.
- После изменения Go-кода нужен `docker compose up --build`, чтобы собрать образ с новым бинарником. `restart` перезапускает существующий контейнер.

## Middleware логирования

`transport.LoggingMiddleware(next http.Handler) http.Handler` возвращает `http.HandlerFunc`, который запоминает `next` в замыкании. В `main` обёртка получает роутер и устанавливается в поле `Handler` структуры `http.Server`.

На каждый запрос обёртка сохраняет `time.Now()`, вызывает `next.ServeHTTP(w, r)` и после его возврата пишет в лог метод, `r.URL.Path` и `time.Since(start)`. Через неё проходят также ответы роутера 404 и 405. HTTP-код ответа middleware пока не записывает.

## Запуск и graceful shutdown

`main` создаёт явный `http.Server` с адресом `":" + port` и роутером, обёрнутым в middleware. `ListenAndServe` запускается в отдельной горутине, а `main` ожидает отмены `stopCtx` через чтение из `stopCtx.Done()`.

```text
SIGINT (Ctrl+C) или SIGTERM
  → signal.NotifyContext отменяет stopCtx
  → ожидание в main заканчивается
  → создаётся новый shutdownCtx с таймаутом 10 секунд
  → server.Shutdown(shutdownCtx)
      → закрывает слушающий сокет и простаивающие соединения
      → ждёт завершения активных HTTP-обработчиков
  → при ошибке: запись в лог и server.Close()
  → выход из main, defer pool.Close() в режиме PostgreSQL
```

При начале остановки `ListenAndServe` возвращает `http.ErrServerClosed`. Эта ошибка исключена из аварийного завершения через `errors.Is`; остальные ошибки запуска приводят к `log.Fatal`.

`Shutdown` сам процесс не завершает. `main` дожидается возврата из него перед выходом. Новый контекст ожидания создаётся от `context.Background()`: `stopCtx` к этому моменту уже отменён. Таймаут ограничивает ожидание внутри `Shutdown`, а не гарантирует завершение всего процесса за 10 секунд. В текущем коде ошибка, возвращаемая принудительным `server.Close()`, не обрабатывается.

Для остановки приложения через Compose с запасом на ожидание можно использовать `docker compose stop -t 15 app`, оставив базу работающей. Штатная остановка без активных запросов проверена для `SIGINT` и `SIGTERM` на отдельном экземпляре с `STORAGE=memory`: после успешного `/ping` процесс завершился с кодом 0. Проверки остановки с активным запросом и сценария превышения таймаута ещё предстоят.

## API

Суммы — целые числа в копейках.

Ошибки возвращаются как `{"error": "текст"}`. Неизвестные ошибки → 500 `{"error": "internal error"}`. Сейчас `errorHandler` не логирует исходную ошибку.

### Начисление

**Метод и путь:** POST /accrual

**Запрос:**

```json
{"user_id": 12, "amount": 1000}
```

**Ответ 200:**

```json
{"balance": 1500}
```

**Ошибки:**

| ситуация | код | текст |
|---|---|---|
| битый JSON или неизвестное поле | 400 | invalid JSON body |
| `user_id` ≤ 0 | 400 | user_id must be greater than 0 |
| `amount` ≤ 0 | 400 | amount must be greater than 0 |
| баланс переполнит `int64` | 409 | balance overflow |

Ошибки «пользователь не найден» у начисления нет: пользователь создаётся при первом начислении.

### Списание

**Метод и путь:** POST /debiting

**Запрос:**

```json
{"user_id": 12, "amount": 1000}
```

**Ответ 200:**

```json
{"balance": 1500}
```

**Ошибки:**

| ситуация | код | текст |
|---|---|---|
| битый JSON или неизвестное поле | 400 | invalid JSON body |
| `user_id` не найден |404| user_id not found |
| `user_id` ≤ 0 | 400 | user_id must be greater than 0 |
| `amount` ≤ 0 | 400 | amount must be greater than 0 |
| `balance` < `amount` | 409 | insufficient funds |

### Перевод

**Метод и путь:** POST /transfer

**Запрос:**

```json
{"user_from_id": 12, "user_to_id": 15,  "amount": 1000}
```

**Ответ 200:**

```json
{"balance_user_from": 500, "balance_user_to": 1900}
```

**Ошибки:**

| ситуация | код | текст |
|---|---|---|
| битый JSON или неизвестное поле | 400 | invalid JSON body |
| `user_from_id` не найден |404| user_from_id not found |
| `user_to_id` не найден |404| user_to_id not found |
| `user_from_id` ≤ 0 | 400 | user_from_id must be greater than 0 |
| `user_to_id` ≤ 0 | 400 | user_to_id must be greater than 0 |
| `user_to_id` = `user_from_id` | 400 | user_to_id and user_from_id must be different |
| `amount` ≤ 0 | 400 | amount must be greater than 0 |
| `balance_user_from` < `amount` | 409 | insufficient funds |
| баланс переполнит `int64` | 409 | balance overflow |


### Баланс

**Метод и путь:** GET /balance/{user_id}

**Ответ 200:**

```json
{"balance": 500}
```

**Ошибки:**

| ситуация | код | текст |
|---|---|---|
| клиент передаст невалидное значение в запросе вместо {user_id} | 400 | user_id must be a number |
| `user_id` не найден |404| user_id not found |
| `user_id` ≤ 0 | 400 | user_id must be greater than 0 |


Пропущенные поля получают нулевые значения в Go-структуре. Их отклоняет service как невалидный id или сумму.

## Тестирование

Тесты находятся рядом с исходным кодом и используют стандартный пакет `testing`.

### Service

`internal/service/balance_test.go` проверяет все четыре метода. Мок `storageMock` реализует интерфейс `Storage`, записывает переданные аргументы и возвращает заранее заданные результаты и ошибки.

Проверяются успешные вызовы, валидация id и суммы, запрет перевода самому себе, передача аргументов в storage и возврат его ошибок. Повторяющиеся сценарии оформлены как таблицы с `t.Run`. В тестах ранней валидации используется `NewService(nil)`: обращение к storage при ошибочных данных привело бы к падению теста.

### HTTP-обработчики

`internal/transport/handler_test.go` создаёт запрос через `httptest.NewRequest`, а ответ записывает в `httptest.NewRecorder`. Настоящий сетевой сервер не запускается.

| операция | успешный сценарий | ошибочный сценарий |
|---|---|---|
| Deposit | 200 и новый баланс | 400 и `invalid JSON body` |
| Withdraw | 200 и остаток | 409 и `insufficient funds` |
| Transfer | 200 и оба баланса | 400 и запрет перевода самому себе |
| Balance | 200 и баланс | 404 и `user_id not found` |

Для операций используются настоящий service и `local.MapStorage`; тест невалидного JSON создаёт handler без service, поскольку запрос отклоняется до его вызова. Balance вызывается через `NewRouter(h).ServeHTTP`, чтобы роутер заполнил `r.PathValue("user_id")`. Остальные тесты вызывают метод handler напрямую.

Всего восемь HTTP-тестов, каждый проверяет код и JSON-тело ответа. Отдельных тестов всех веток `errorHandler` пока нет.

### PostgreSQL

`internal/storage/postgresdb/postgres_storage_test.go` использует настоящий `PostgresStorage` и пул `pgxpool`. HTTP и service в этих тестах не участвуют.

Подключение берётся только из `TEST_DATABASE_URL`. При пустой переменной вызывается `t.Skip`; при ошибке создания пула или `Ping` — `t.Fatalf`. Перед очисткой проверяется точное имя базы `balance_test`, затем выполняется `TRUNCATE TABLE user_balance`. Схема применяется заранее из `migrations/001_init.sql`.

| тест | сценарий и ожидаемый результат |
|---|---|
| `TestTransferRollbackRecipientNotFound` | отправитель с 1000 переводит 300 отсутствующему получателю; возвращается `ErrRecipientNotFound`, отдельное чтение баланса подтверждает откат и остаток 1000 |
| `TestWithdrawConcurrent` | 200 горутин списывают по 1000 с начального баланса 100000; ровно 100 успешных операций, 100 `ErrInsufficientFunds`, баланс 0 |
| `TestTransferConcurrentOppositeDirections` | два пользователя с балансами по 1000, по 100 переводов в каждую сторону по 1 копейке; все 200 успешны, конечные балансы по 1000 |

Параллельные операции запускаются внутри теста. До запуска каждой горутины вызывается `wg.Add(1)`, внутри откладывается `wg.Done()`. Каждая горутина записывает локальную ошибку в свой элемент среза; чтение результатов и проверка балансов выполняются после `wg.Wait()`.

Сами тесты идут последовательно, без `t.Parallel()`, поскольку очищают общую таблицу. Несколько процессов с этими тестами на одной базе также будут мешать друг другу.

Тест встречных переводов использует `context.WithTimeout` на 10 секунд с `defer cancel()`. Дедлайн общий для подготовки, операций и финальных чтений. Контекст не останавливает горутины сам: pgx реагирует на отмену и возвращает ошибку, которую обнаруживают проверки. Флаг `-race` проверяет гонки в Go-коде; защиту данных в базе подтверждают результаты операций и конечные балансы.

### Запуск

Service и HTTP-тесты не требуют PostgreSQL или переменных окружения:

```bash
go test -race ./internal/service ./internal/transport
go test -v ./internal/service
go test -run '^TestBalance' -v ./internal/transport
```

Для интеграционных тестов сначала один раз создаётся база `balance_test` и применяется схема. Точные команды подготовки — в [README](../README.md#интеграционные-тесты-postgresql). Перед запуском из корня проекта:

```bash
docker compose up -d --wait db
set -a; source .env; set +a
go test -race -count=1 -timeout=30s -v ./internal/storage/postgresdb
```

Для всех пакетов используется `go test -race -count=1 -timeout=30s ./...`: с заданным `TEST_DATABASE_URL` выполнятся и интеграционные тесты, без него они будут пропущены. Результаты не берутся из кеша благодаря `-count=1`; `-timeout=30s` ограничивает время работы каждого тестового процесса.

## Storage

Интерфейс объявлен в `service` (там, где он нужен):

```go
type Storage interface {
	Balance(ctx context.Context, userID int64) (int64, error)
	Deposit(ctx context.Context, userID, amount int64) (int64, error)
	Withdraw(ctx context.Context, userID, amount int64) (int64, error)
	Transfer(ctx context.Context, fromID, toID, amount int64) (int64, int64, error)
}
```

Каждая операция атомарна: проверки, которые зависят от текущих данных, выполняются внутри операции вместе с изменением.

| операция | принимает | возвращает | ошибки |
|---|---|---|---|
| Deposit (начисление) | user_id, amount | balance | `ErrBalanceOverflow` |
| Withdraw (списание) | user_id, amount | balance | `ErrNotFound`, `ErrInsufficientFunds` |
| Transfer (перевод) | user_from_id, user_to_id, amount | balance_user_from, balance_user_to | `ErrSenderNotFound`, `ErrRecipientNotFound`, `ErrInsufficientFunds` (у отправителя), `ErrBalanceOverflow` (у получателя) |
| Balance (баланс) | user_id | balance | `ErrNotFound` |

Storage не проверяет перевод самому себе: при `from = to` ответ был бы неверным. Эту проверку обязан сделать service.

### Две реализации

| | `local.MapStorage` | `postgresdb.PostgresStorage` |
|---|---|---|
| данные | `map[int64]int64` | таблица `user_balance` |
| атомарность | один `sync.Mutex` на всё хранилище | блокировки строк в Postgres |
| `ctx` | принимает, не использует | передаёт в каждый запрос pgx |
| несколько копий сервиса | у каждой свои данные и свой мьютекс | общая база, защита работает |

### PostgreSQL

Схема (`migrations/001_init.sql`):

| колонка | тип | ограничения |
|---|---|---|
| `user_id` | `bigint` | `PRIMARY KEY`, `user_id_positive CHECK (user_id > 0)` |
| `balance` | `bigint` | `NOT NULL DEFAULT 0`, `balance_non_negative CHECK (balance >= 0)` |

`CHECK` — последняя линия защиты: даже при баге в коде минус в таблицу не попадёт.

Как устроена каждая операция:

| операция | SQL | почему атомарно |
|---|---|---|
| Balance | `SELECT balance ... WHERE user_id = $1` | один запрос |
| Deposit | `INSERT ... ON CONFLICT (user_id) DO UPDATE SET balance = user_balance.balance + EXCLUDED.balance RETURNING balance` | один запрос; «создать или прибавить» решает сама база, без гонки SELECT → INSERT |
| Withdraw | `UPDATE ... SET balance = balance - $2 WHERE user_id = $1 RETURNING balance` | `UPDATE` блокирует строку; параллельное списание ждёт и считает от нового значения; минус отклоняет `CHECK` |
| Transfer | транзакция: `SELECT ... WHERE user_id IN ($1, $2) ORDER BY user_id FOR UPDATE`, два `UPDATE ... RETURNING`, `COMMIT` | обе строки заблокированы до конца транзакции; при любой ошибке `defer tx.Rollback` откатывает оба изменения |

Блокировки в переводе берутся в порядке возрастания `user_id`. Иначе встречные переводы A → B и B → A ждут друг друга (дедлок, `40P01`). Проверено: 200 встречных переводов — без сортировки 171 дедлок, с сортировкой ни одного.

Ошибки Postgres → ошибки `domain`:

| что вернул pgx | когда | ошибка `domain` |
|---|---|---|
| `pgx.ErrNoRows` (`errors.Is`) | `UPDATE ... RETURNING` не нашёл строку | `ErrNotFound`, в переводе `ErrSenderNotFound` / `ErrRecipientNotFound` |
| `*pgconn.PgError`, `ConstraintName = balance_non_negative` (`errors.As`) | баланс ушёл бы в минус | `ErrInsufficientFunds` |
| `*pgconn.PgError`, `Code = 22003` | `bigint out of range` | `ErrBalanceOverflow` |
| любая другая | база недоступна, отмена `ctx` и т.п. | возвращается как есть → 500 |

Сравнивается `ConstraintName`, а не `Code 23514`: этот код у любого нарушенного `CHECK`. И не `Message`: это текст для человека, он зависит от версии и локали. Коды и имена ограничений — константы в `postgresdb`, `domain` про Postgres не знает.

## Ошибки между слоями

Ошибки объявлены один раз в `internal/domain`. Service возвращает свои ошибки проверки запроса, а ошибки storage пропускает наверх без изменений. Все реализации storage (память и Postgres) возвращают одни и те же значения. Handler переводит ошибку в HTTP-код через `errors.Is`:

| ошибка `domain` | кто возвращает | HTTP-код | текст ответа |
|---|---|---|---|
| `ErrInvalidUserID` | service | 400 | `user_id must be greater than 0` |
| `ErrInvalidSenderID` | service | 400 | `user_from_id must be greater than 0` |
| `ErrInvalidRecipientID` | service | 400 | `user_to_id must be greater than 0` |
| `ErrInvalidAmount` | service | 400 | `amount must be greater than 0` |
| `ErrSelfTransfer` | service | 400 | `user_to_id and user_from_id must be different` |
| `ErrNotFound` | storage | 404 | `user_id not found` |
| `ErrSenderNotFound` | storage | 404 | `user_from_id not found` |
| `ErrRecipientNotFound` | storage | 404 | `user_to_id not found` |
| `ErrInsufficientFunds` | storage | 409 | `insufficient funds` |
| `ErrBalanceOverflow` | storage | 409 | `balance overflow` |
| любая другая | — | 500 | `internal error` |

Ошибки разбора запроса (битый JSON, `user_id` в пути не число) handler находит сам, до вызова service, и отвечает 400.

Тексты ответа с именами полей JSON подставляет handler. Тексты самих ошибок в `domain` не должны знать про JSON.

## Context

| где | какой | зачем |
|---|---|---|
| `main` | `context.Background()` | создание пула и `Ping` при запуске: запроса ещё нет |
| ожидание сигнала в `main` | `signal.NotifyContext` → `stopCtx` | отмена по `SIGINT` или `SIGTERM`; закрытие канала `Done` завершает ожидание |
| остановка HTTP-сервера | отдельный `context.WithTimeout` → `shutdownCtx` | до 10 секунд на завершение обработки запросов; родитель `Background`, поскольку `stopCtx` уже отменён |
| handler | `r.Context()` | отменяется, когда клиент отключился; pgx прерывает запрос, транзакция откатывается, соединение возвращается в пул |
| тест встречных переводов | `context.WithTimeout` | общий дедлайн 10 секунд на тест, `defer cancel()` освобождает ресурсы таймера |

`ctx` — первый параметр, в поля структур его не кладём: у каждого запроса он свой.


## План расширения

После проверки graceful shutdown с активным запросом и по таймауту:

1. gRPC как второй транспорт для четырёх операций с общим service.
2. История операций: запись вместе с изменением баланса в одной транзакции, пагинация и сортировка.
3. Конвертация валют через внешний API, Redis для кеширования курсов с TTL.
4. Kafka: события операций через transactional outbox в PostgreSQL и отдельный consumer уведомлений.

Эти расширения согласованы как учебный план и пока не реализованы.

## Решения и вопросы по ТЗ

| вопрос | решение |
|---|---|
| код ошибки недостаточно средств | 409 «запрос корректный, но конфликтует с текущим состоянием баланса»  |
| В чём отдавать баланс, если в ТЗ «баланс всегда в рублях»? | В копейках (`int64`): валюта — рубли, единицы — копейки. |
| Что, если начисление переполнит `int64`? | Проверка в storage , ошибка 409 «запрос корректный, но конфликтует с текущим состоянием баланса» |
| Начисление пользователю, которого ещё нет? | Создаём его (ТЗ, детали, п. 2). |
| обращение к несуществующему пользователю | перевод несуществующему получателю → 404, начисление → создание нового пользователя |
| Где объявлять общие ошибки? | В отдельном пакете `internal/domain`. В storage — handler зависел бы от конкретной реализации; в service — storage импортировал бы верхний слой, риск циклического импорта. |
| Как в переводе сообщить, кто именно не найден? | Две отдельные ошибки: `ErrSenderNotFound` и `ErrRecipientNotFound`. Для «недостаточно средств» и переполнения хватает общих ошибок: первое бывает только у отправителя, второе — только у получателя. Альтернатива — тип ошибки с полем `UserID` и `errors.As`, для четырёх методов избыточно. |
| Где проверять «хватает ли денег» в Postgres? | В самой базе: `CHECK (balance >= 0)` + один `UPDATE`. Отдельный `SELECT` перед `UPDATE` — гонка «прочитал → проверил → изменил». |
| Где начинать транзакцию? | Внутри метода storage. Не в middleware и не в service: у `MapStorage` транзакций нет, интерфейс обещает только атомарность операции. |
| Что делать, если база недоступна при старте? | Падать (`log.Fatal`), а не переключаться молча на map: иначе деньги лягут в память и пропадут при перезапуске. Хранилище выбирается явно через `STORAGE`. |
| Механизм миграций? | По ТЗ не нужен: итоговая схема в `migrations/001_init.sql`. Пока боевой базы нет, правим этот файл; потом изменения — только новыми файлами `002_...`. |
| Где хранить `pgErr` для `errors.As`? | Локальная переменная в каждом методе. Глобальная — гонка данных между запросами (проверено `-race`). |

