-- Stores each item type's buy and sell prices once a day in the markets the
-- import calculator and price pages compare: Jita 4-4 (market 'jita'), the
-- C-J6MT Keepstar ('cj6mt') and PLEX's global market ('plex'). Fetched with
-- the daily Jita prices after downtime:
--   sell_price   the volume-weighted average of the cheapest 5% of units
--                listed, capped at 3x CCP's average traded price
--   min_sell     the cheapest sell order
--   buy_price    the volume-weighted average of the highest-paying 5% of
--                units wanted
--   max_buy      the best buy order
--   *_volume     units listed, in *_orders orders
-- A side with no orders is null. lifetime_indy.jita_sell_prices stays the
-- price everything is valued at.
-- packaged_volumes caches each ship's packaged volume from ESI, since the
-- SDE only has the assembled one. Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists market_prices (
    market text not null,
    type_id int not null,
    price_date date not null,
    sell_price numeric(20,2),
    min_sell numeric(20,2),
    sell_volume bigint,
    sell_orders int,
    buy_price numeric(20,2),
    max_buy numeric(20,2),
    buy_volume bigint,
    buy_orders int,
    primary key (market, type_id, price_date)
);
create index if not exists market_prices_type_idx on market_prices (type_id, price_date);

create table if not exists packaged_volumes (
    type_id int primary key,
    packaged_volume double precision not null,
    fetched_at timestamptz not null default now()
);
commit;
