create table if not exists user_balance (
    user_id bigint CONSTRAINT user_id_positive CHECK (user_id > 0) primary key,
    balance bigint CONSTRAINT balance_non_negative CHECK (balance >= 0) not null default 0
);

