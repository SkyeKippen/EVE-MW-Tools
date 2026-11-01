-- Stores CCP's daily market history for every region and item type, from
-- EVE Ref's archive (data.everef.net/market-history), one row per type,
-- region and day:
--   average, highest, lowest   the day's average, highest and lowest
--                              traded price
--   volume                     units traded that day
--   order_count                orders that traded that day
-- The background refresh imports every day it doesn't have yet, once a day.
-- All regions since EVE Ref's archive starts is around 85 million rows,
-- roughly 8-10 GB with the primary key.
-- market_history_days records each imported day: how many rows it saved and
-- how many records EVE Ref's totals.json listed for it then (0 when it listed
-- none), so a day is imported again when EVE Ref's file for it grows.
-- Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists market_history (
    type_id int not null,
    region_id int not null,
    date date not null,
    average double precision not null,
    highest double precision not null,
    lowest double precision not null,
    volume bigint not null,
    order_count bigint not null,
    primary key (type_id, region_id, date)
);

create table if not exists market_history_days (
    date date primary key,
    rows int not null,
    expected int not null default 0,
    imported_at timestamptz not null default now()
);
commit;
