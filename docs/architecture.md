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
handler       JSON ↔ структуры Go, ошибки → коды ответа
  │
  ▼
service       бизнес-правила
  │
  ▼
storage       данные: map (ступень 2), PostgreSQL (ступень 3)
```

## Структура папок

```
.
├── cmd/
│   └── app/
│       └── main.go            сборка слоёв и запуск сервера
└── internal/
    ├── domain/
    │   └── errors.go          ошибки, общие для всех слоёв
    ├── transport/
    │   ├── router.go
    │   └── handler.go
    ├── service/
    └── storage/
        └── local/
            └── map_storage.go хранилище в памяти: map + sync.Mutex
```

Кто кого импортирует (циклы в Go запрещены):

| пакет | импортирует |
|---|---|
| `cmd/app` | `transport`, `service`, `storage/local` (собирает слои) |
| `transport` | `service`, `domain` |
| `service` | `domain` |
| `storage/local` | `domain` |
| `domain` | ничего из проекта |

Каждый пакет импортирует только пакеты ниже себя, поэтому циклов нет. `domain` можно импортировать из любого слоя.

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

Каждая операция атомарна: проверки, которые зависят от текущих данных, выполняются внутри операции вместе с изменением.

| операция | принимает | возвращает | ошибки |
|---|---|---|---|
| Deposit (начисление) | user_id, amount | balance | `ErrBalanceOverflow` |
| Withdraw (списание) | user_id, amount | balance | `ErrNotFound`, `ErrInsufficientFunds` |
| Transfer (перевод) | user_from_id, user_to_id, amount | balance_user_from, balance_user_to | `ErrNotFound`, `ErrInsufficientFunds`, `ErrBalanceOverflow` |
| Balance (баланс) | user_id | balance | `ErrNotFound` |

## Ошибки между слоями

Ошибки объявлены один раз в `internal/domain`. Все реализации storage (память, позже Postgres) возвращают одни и те же значения. Service пропускает их наверх, handler переводит в HTTP-код через `errors.Is`:

| ошибка `domain` | HTTP-код | готово |
|---|---|---|
| `ErrNotFound` | 404 | да |
| `ErrInsufficientFunds` | 409 | нет |
| `ErrBalanceOverflow` | 409 | нет |
| любая другая | 500 `internal error` | — |

В переводе `ErrNotFound` один на обоих пользователей. Если клиенту нужно знать, кто именно не найден (`user_from_id not found` / `user_to_id not found`), понадобится либо две ошибки, либо ошибка с полем. _Не решено._


## Решения и вопросы по ТЗ

| вопрос | решение |
|---|---|
| код ошибки недостаточно средств | 409 «запрос корректный, но конфликтует с текущим состоянием баланса»  |
| В чём отдавать баланс, если в ТЗ «баланс всегда в рублях»? | В копейках (`int64`): валюта — рубли, единицы — копейки. |
| Что, если начисление переполнит `int64`? | Проверка в storage , ошибка 409 «запрос корректный, но конфликтует с текущим состоянием баланса» |
| Начисление пользователю, которого ещё нет? | Создаём его (ТЗ, детали, п. 2). |
| обращение к несуществующему пользователю | перевод несуществующему получателю → 404, начисление → создание нового пользователя |
| Где объявлять общие ошибки? | В отдельном пакете `internal/domain`. В storage — handler зависел бы от конкретной реализации; в service — storage импортировал бы верхний слой, риск циклического импорта. |

