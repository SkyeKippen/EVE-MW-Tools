-- Stores each item type's Jita sell price once a day, so mined ore,
-- delivered jobs, assets and stockpiles can be valued. Prices come from every
-- sell order at Jita IV - Moon 4 - Caldari Navy Assembly Plant (PLEX from its
-- global market), fetched once a day after downtime:
--   sell_price    the volume-weighted average of the cheapest 5% of units
--                 listed, capped at 3x CCP's average traded price, and for
--                 blueprints at their NPC price
--   min_price     the cheapest sell order
--   volume        units for sale, in order_count orders
-- ESI has no sell price history, so prices start the day the app first
-- saves them; days before that use the closest day saved.
-- Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists jita_sell_prices (
    type_id bigint not null,
    price_date date not null,
    sell_price numeric(20,2) not null,
    min_price numeric(20,2),
    volume bigint,
    order_count int,
    updated_at timestamptz not null default now(),
    primary key (type_id, price_date)
);
alter table jita_sell_prices add column if not exists min_price numeric(20,2);
alter table jita_sell_prices add column if not exists volume bigint;
alter table jita_sell_prices add column if not exists order_count int;
create index if not exists jita_sell_prices_date_idx on jita_sell_prices (price_date);
commit;
