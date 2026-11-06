# balance-user-api

Микросервис для работы с балансом пользователей (HTTP JSON API).
Это тестовое задание Авито, [оригинал условия](https://github.com/avito-tech/job-backend-trainee-assignment).

**Статус:** в разработке. Все четыре метода работают, доступны PostgreSQL и хранилище в памяти. Сервис и база поднимаются через Docker Compose. Есть тесты service, HTTP-обработчиков и PostgreSQL, логирование запросов и graceful shutdown. Следующее расширение — gRPC.

Архитектура, полный контракт API и принятые решения: [docs/architecture.md](docs/architecture.md).

## Стек

- Go 1.26, `net/http` с роутером Go 1.22+, без фреймворков.
- PostgreSQL 18, драйвер [pgx v5](https://github.com/jackc/pgx) (пул соединений `pgxpool`).
- Docker, Docker Compose.
- Деньги хранятся как `int64` (`BIGINT`) в копейках.

## Запуск

### Через Docker Compose

Нужен только Docker.

```bash
cp .env.example .env
docker compose up --build
```

Compose поднимает два контейнера:

- `db` — PostgreSQL 18. При первом запуске таблица создаётся из [migrations/001_init.sql](migrations/001_init.sql);
- `app` — сервис. Стартует, только когда база готова принимать подключения.

Сервис доступен на `localhost:8080`, база — на `localhost:5433` (для psql с машины). Оба порта должны быть свободны.

Данные хранятся в томе `pgdata` и переживают перезапуск:

| команда | что происходит |
|---|---|
| `docker compose down` | контейнеры удаляются, данные остаются |
| `docker compose down -v` | удаляются и данные, следующий запуск — с пустой базой |

Посмотреть таблицу:

```bash
docker compose exec db psql -U balance -d balance -c 'select * from user_balance'
```

### Для разработки: `go run`

Нужен Go 1.26 или новее.

Хранилище выбирается переменной окружения `STORAGE`:

| `STORAGE` | где данные | что ещё нужно |
|---|---|---|
| `postgres` | в PostgreSQL | `DATABASE_URL` и запущенная база |
| `memory` | в памяти процесса, пропадают при перезапуске | ничего |

Если `STORAGE` не задана или в ней опечатка, сервис не стартует. Если база недоступна, тоже не стартует: на хранение в памяти он сам не переключается.

С PostgreSQL — база из Compose, сервис на машине:

```bash
cp .env.example .env              # если ещё не сделано
docker compose up -d db           # только база, в фоне
set -a; source .env; set +a       # загрузить переменные из .env
go run ./cmd/app
```

Go не читает `.env` сам, поэтому переменные нужно загружать в каждом новом окне терминала. Контейнер `app` при этом запускать не нужно: он займёт тот же порт 8080.

В памяти:

```bash
STORAGE=memory go run ./cmd/app
```

### Переменные окружения

| переменная | кто использует | пример |
|---|---|---|
| `STORAGE` | сервис | `postgres` или `memory` |
| `DATABASE_URL` | сервис при `STORAGE=postgres` | `postgres://balance:balance@localhost:5433/balance` |
| `PORT` | сервис | `8080` (по умолчанию) |
| `TEST_DATABASE_URL` | интеграционные тесты | `postgres://balance:balance@localhost:5433/balance_test` |
| `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB` | Compose: база и строка подключения для `app` | `balance` |

`.env.example` — шаблон, `.env` — локальная копия, в git не попадает. В Compose контейнер `app` получает `DATABASE_URL` с хостом `db`, а не `localhost`: внутри контейнера `localhost` — это он сам.

### Проверка

```bash
curl localhost:8080/ping
```

### Логи и остановка

Для каждого обработанного запроса middleware пишет в лог HTTP-метод, путь и длительность. Посмотреть логи контейнера:

```bash
docker compose logs -f app
```

При `SIGINT` (Ctrl+C) или `SIGTERM` сервис перестаёт принимать новые соединения и ждёт завершения текущих HTTP-запросов до 10 секунд. При ошибке `Shutdown`, в том числе по таймауту, вызывается `server.Close()` для принудительного закрытия соединений. Затем при выходе из `main` закрывается пул PostgreSQL.

Для остановки только приложения с запасом времени на завершение:

```bash
docker compose stop -t 15 app
```

База при этом продолжает работать. `http.ErrServerClosed` при штатной остановке не считается аварией.

## API

Все суммы — целые числа в копейках: `100000` = 1000 ₽. Тела запросов и ответов — JSON. При ошибке приходит `{"error": "текст"}` с соответствующим HTTP-кодом.

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

Если пользователя ещё нет, он создаётся при первом начислении. Для перевода ниже начислим ещё и пользователю 2:

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

## Тесты

Запуск из корня проекта, нужен Go 1.26 или новее.

### Service и HTTP-обработчики

Этим тестам не нужны сервер, PostgreSQL, Docker или переменные окружения:

```bash
go test ./internal/service ./internal/transport
go test -race ./internal/service ./internal/transport
go test -v ./internal/service # подробно, включая табличные подтесты
go test -run '^TestBalance' -v ./internal/transport # только HTTP-тесты баланса
```

| файл | что проверяется |
|---|---|
| [internal/service/balance_test.go](internal/service/balance_test.go) | все четыре метода: валидация, передача аргументов в мок `Storage`, возврат результатов и ошибок; повторяющиеся сценарии оформлены таблицами |
| [internal/transport/handler_test.go](internal/transport/handler_test.go) | восемь сценариев через `httptest`: четыре успешных ответа, невалидный JSON, нехватка средств, перевод самому себе и отсутствующий пользователь |

HTTP-тесты используют настоящий service и `MapStorage`; для невалидного JSON service не нужен. Тесты баланса проходят через роутер, который заполняет `user_id` из пути. Проверяются HTTP-коды и JSON-тела ответов.

HTTP-тесты проверяют выбранные ошибки; отдельного теста на каждую ветку `errorHandler` пока нет.

### Интеграционные тесты PostgreSQL

[internal/storage/postgresdb/postgres_storage_test.go](internal/storage/postgresdb/postgres_storage_test.go) работает с настоящим PostgreSQL через `pgxpool`. Контейнер приложения `app` и HTTP-сервер для этих тестов не нужны.

Один раз подготовь отдельную базу `balance_test`:

```bash
cp .env.example .env # если локального .env ещё нет
docker compose up -d --wait db
docker compose exec -T db psql -U balance -d balance -c 'CREATE DATABASE balance_test;'
docker compose exec -T db psql -v ON_ERROR_STOP=1 -U balance -d balance_test < migrations/001_init.sql
```

Команда `CREATE DATABASE` нужна только при первом создании базы. Если база уже существует, повторять её не нужно. Команды используют пользователя `balance` из `.env.example`; при своей конфигурации подставь своё имя пользователя и основной базы.

В локальном `.env` должна быть переменная:

```dotenv
TEST_DATABASE_URL=postgres://balance:balance@localhost:5433/balance_test
```

В каждом новом терминале загрузи переменные и запусти тесты:

```bash
set -a; source .env; set +a
go test -race -count=1 -timeout=30s -v ./internal/storage/postgresdb
```

`-race` проверяет гонки данных в Go, `-count=1` отключает кеш результатов, `-timeout=30s` ограничивает время работы тестового процесса. Для одного сценария после `-run` укажи имя теста, а затем путь пакета:

```bash
go test -race -count=1 -timeout=30s -v -run '^TestWithdrawConcurrent$' ./internal/storage/postgresdb
```

| тест | что проверяет |
|---|---|
| `TestTransferRollbackRecipientNotFound` | перевод 300 отсутствующему получателю возвращает `ErrRecipientNotFound`, баланс отправителя остаётся 1000 |
| `TestWithdrawConcurrent` | 200 параллельных списаний по 1000 с баланса 100000: 100 успешных, 100 с `ErrInsufficientFunds`, итоговый баланс 0 |
| `TestTransferConcurrentOppositeDirections` | по 100 переводов в каждую сторону по 1 копейке: все 200 успешны, у каждого пользователя остаётся по 1000 |

Перед каждым тестом проверяется имя базы `balance_test` и очищается таблица `user_balance` через `TRUNCATE`. Данные тестовой таблицы удаляются. Тесты запускаются последовательно; не запускай несколько их копий одновременно на одной базе. У теста встречных переводов есть общий контекст с таймаутом 10 секунд.

Без `TEST_DATABASE_URL` интеграционные тесты получают `SKIP`. Если переменная задана, но база недоступна или имеет другое имя, тест завершится с ошибкой. Go и кнопка запуска тестов в редакторе сами `.env` не загружают.

### Все тесты и проверки

При загруженном `TEST_DATABASE_URL` и подготовленной тестовой базе команды ниже проверяют также PostgreSQL. Без этой переменной интеграционные тесты будут пропущены.

```bash
go build ./...
go vet ./...
go test -race -count=1 -timeout=30s ./...
```

## Вопросы по ТЗ и решения

1. **Объединять ли начисление и списание в один метод?** ТЗ это разрешает, но методов два. Так сумма всегда положительная, а у каждого метода свой набор ошибок: например, у начисления не бывает «недостаточно средств».
2. **В чём хранить баланс, если в ТЗ «баланс всегда в рублях»?** В копейках, тип `int64` (`BIGINT` в базе). Валюта — рубли, единицы — копейки. Без `float` нет ошибок округления, а дробные суммы отклоняются как невалидный JSON.
3. **Что делать с пользователем, которого ещё нет?** Начисление его создаёт (ТЗ, детали, п. 2). Списание, перевод и запрос баланса для несуществующего пользователя возвращают 404. Перевод пользователя не создаёт.
4. **Какой код у «недостаточно средств»?** 409 Conflict: запрос корректный, но конфликтует с текущим состоянием баланса. Код 400 оставлен для ошибок в самих данных запроса.
5. **Что, если начисление переполнит `int64`?** Ответ 409 `balance overflow`, баланс не меняется. PostgreSQL сам отклоняет такое сложение (`bigint out of range`), в памяти это проверяется до сложения.
6. **Можно ли перевести деньги самому себе?** Нет, ответ 400.
7. **Что делать с неизвестными полями в JSON?** Запрос отклоняется с кодом 400.
8. **Как не уйти в минус при одновременных запросах?** Проверка и изменение баланса выполняются атомарно:
   - на таблице стоит ограничение `CHECK (balance >= 0)`;
   - списание — один `UPDATE`: он блокирует строку, поэтому параллельные списания выполняются по очереди, а то, что увело бы баланс в минус, отклоняется ограничением;
   - перевод — транзакция: обе строки блокируются через `SELECT ... FOR UPDATE` в порядке возрастания `user_id`, чтобы встречные переводы не попали в дедлок.

   Проверено: 200 одновременных списаний по 1000 с баланса 100000 дают 100 ответов 200, 100 ответов 409 и баланс 0. В режиме `memory` ту же роль выполняет мьютекс.
9. **Механизм миграций?** По ТЗ не нужен. Итоговая схема — в [migrations/001_init.sql](migrations/001_init.sql), Compose применяет её при первом запуске базы.
10. **Что, если сервис стартует раньше базы?** В Compose у базы есть `healthcheck` (`pg_isready`), и `app` запускается только после того, как база стала `healthy`. Если база всё же недоступна, сервис падает с понятной ошибкой, а не работает без неё.

## Структура

```
cmd/app/                конфиг из env, сборка слоёв, запуск и остановка сервера
internal/transport/     HTTP: роутер, обработчики, JSON, коды ответов, логирование
internal/service/       проверка данных запроса, интерфейс Storage
internal/storage/
  postgresdb/           хранилище в PostgreSQL
  local/                хранилище в памяти: map + sync.Mutex
internal/domain/        ошибки, общие для всех слоёв
migrations/             SQL-схема
docs/                   условие задания и архитектура
Dockerfile              сборка образа сервиса в два этапа
docker-compose.yml      сервис + PostgreSQL
.env.example            шаблон переменных окружения
```

Тесты лежат рядом с кодом в файлах `*_test.go`.

## Что дальше

- Проверка graceful shutdown с активным запросом и при истечении времени ожидания
- gRPC как второй транспорт поверх существующего service
- История операций с пагинацией и сортировкой
- Конвертация валют через внешний API и Redis для кеширования курсов
- Kafka: события операций, transactional outbox и отдельный consumer
