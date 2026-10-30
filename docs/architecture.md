# Архитектура balance-user-api

Черновик, дополняется по ходу работы.

## Путь запроса

```
клиент (curl, другой сервис)
  │
  ▼
http.Server   слушает порт, на каждое соединение запускает горутину
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
│       └── main.go                 конфиг из env, сборка слоёв, запуск сервера
├── internal/
│   ├── domain/
│   │   └── errors.go               ошибки, общие для всех слоёв
│   ├── transport/
│   │   ├── router.go
│   │   └── handler.go
│   ├── service/
│   │   └── balance.go              Service + интерфейс Storage
│   └── storage/
│       ├── local/
│       │   └── map_storage.go      хранилище в памяти: map + sync.Mutex
│       └── postgresdb/
│           └── postgres_storage.go хранилище в PostgreSQL через pgxpool
└── migrations/
    └── 001_init.sql                схема таблицы
```

Кто кого импортирует (циклы в Go запрещены):

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

При `STORAGE=postgres` main создаёт пул и проверяет связь через `Ping`: `pgxpool.New` к базе не ходит. Если `Ping` не прошёл — `log.Fatal`.

Порт пока задан в коде: `:8080`.

## API

Суммы — целые числа в копейках.

Ошибки возвращаются как `{"error": "текст"}`. Неизвестные ошибки → 500 `{"error": "internal error"}`, подробности только в лог.

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
| битый JSON, нет полей | 400 | invalid JSON body |
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
| битый JSON, нет полей | 400 | invalid JSON body |
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
| битый JSON, нет полей | 400 | invalid JSON body |
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
| handler | `r.Context()` | отменяется, когда клиент отключился; pgx прерывает запрос, транзакция откатывается, соединение возвращается в пул |

`ctx` — первый параметр, в поля структур его не кладём: у каждого запроса он свой.


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

