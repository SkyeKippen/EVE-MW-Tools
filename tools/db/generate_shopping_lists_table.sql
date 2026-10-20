-- Creates the table behind the Shopping List page's Save List button: the
-- one Multibuy list each user saved, kept until they clear it or save over it.
-- Run after generate_users_tables.sql. Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists shopping_lists (
    user_id bigint primary key references users (user_id) on delete cascade,
    list text not null,
    saved_at timestamptz not null default now()
);
commit;
