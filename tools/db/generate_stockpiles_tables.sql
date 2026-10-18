-- Creates the tables behind the Stockpiles page. Each user has their own
-- stockpiles: named groups of items, or runs of blueprints, wanted at one
-- station or structure.
--   kind   item (quantity is units) or blueprint_runs (quantity is runs)
-- Run after generate_users_tables.sql and generate_places_table.sql.
-- Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists stockpiles (
    stockpile_id bigserial primary key,
    user_id bigint not null references users (user_id) on delete cascade,
    name text not null,
    place_id bigint not null references places (location_id),
    created_by_character_id bigint not null,
    created_by_name text not null,
    created_at timestamptz not null default now()
);
create index if not exists stockpiles_user_id_idx on stockpiles (user_id);

create table if not exists stockpile_items (
    stockpile_item_id bigserial primary key,
    stockpile_id bigint not null references stockpiles (stockpile_id) on delete cascade,
    type_id bigint not null,
    name text not null,
    kind text not null check (kind in ('item', 'blueprint_runs')),
    quantity bigint not null check (quantity > 0),
    unique (stockpile_id, type_id, kind)
);
commit;
