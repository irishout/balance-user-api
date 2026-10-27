create table if not exists user_balance (
    user_id bigint check (user_id > 0) primary key,
    balance bigint check (balance >= 0) not null default 0
);