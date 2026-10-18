-- Copies what the old MWHI Stockpile Manager saved into Meadow-Works Tools,
-- from its stockpiles, networth and auth schemas in the same database:
--   stockpiles   each one goes to the user whose character created it, so
--                that character must have signed in to Meadow-Works Tools
--                first; ones made by anyone else are left behind
--   structures   become places, so the stockpiles can point at them
--   net worth    the hourly wallet and asset values become net worth
--                snapshots (wallets as liquid, assets as assets). The old
--                app counted PLEX as an asset, so imported hours show no PLEX
--   Jita prices  days the old app priced that this app hasn't
-- The old schemas are only read, never changed. Run it after
-- setup_all_tables.sql, e.g.
--   psql "$DATABASE_URL" -f tools/db/import_stockpile_manager.sql
-- Safe to run more than once: anything already copied is skipped.
begin;

insert into lifetime_indy.places
    (location_id, kind, accessible, name, type_name, owner_name, solar_system, region, checked_at)
select structure_id,
       case when structure_id >= 60000000 and structure_id < 64000000 then 'station' else 'structure' end,
       true, name, type, owner, solar_system, region, updated_at
from stockpiles.structures
on conflict (location_id) do nothing;

-- one new stockpile per old one; an old stockpile counts as copied when its
-- user already has one with the same name, place and creation time
insert into lifetime_indy.stockpiles
    (user_id, name, place_id, created_by_character_id, created_by_name, created_at)
select uc.user_id, s.name, s.structure_id, s.created_by_character_id, s.created_by_name, s.created_at
from stockpiles.stockpiles s
join lifetime_indy.user_characters uc on uc.character_id = s.created_by_character_id
where not exists (
    select 1 from lifetime_indy.stockpiles n
    where n.user_id = uc.user_id and n.name = s.name and n.place_id = s.structure_id and n.created_at = s.created_at);

insert into lifetime_indy.stockpile_items (stockpile_id, type_id, name, kind, quantity)
select n.stockpile_id, i.type_id, i.name, i.kind, i.quantity
from stockpiles.stockpile_items i
join stockpiles.stockpiles s on s.stockpile_id = i.stockpile_id
join lifetime_indy.user_characters uc on uc.character_id = s.created_by_character_id
join lifetime_indy.stockpiles n
  on n.user_id = uc.user_id and n.name = s.name and n.place_id = s.structure_id and n.created_at = s.created_at
on conflict (stockpile_id, type_id, kind) do nothing;

-- the old app took a snapshot a few minutes past each hour (and on demand),
-- saving wallets and assets separately; each hour either was saved becomes
-- a snapshot of the owner's latest wallet and asset values by its end
insert into lifetime_indy.networth_snapshots
    (owner_id, owner_type, taken_at, liquid, assets, plex, unpriced_types)
select h.owner_id, h.owner_kind, h.hour, coalesce(w.balance, 0), coalesce(a.value, 0), 0, coalesce(a.unpriced_types, 0)
from (
    select distinct owner_kind, owner_id, date_trunc('hour', taken_at) as hour from networth.wallet_balances
    union
    select distinct owner_kind, owner_id, date_trunc('hour', taken_at) from networth.asset_values
) h
left join lateral (
    select balance from networth.wallet_balances
    where owner_kind = h.owner_kind and owner_id = h.owner_id and taken_at < h.hour + interval '1 hour'
    order by taken_at desc limit 1
) w on true
left join lateral (
    select value, unpriced_types from networth.asset_values
    where owner_kind = h.owner_kind and owner_id = h.owner_id and taken_at < h.hour + interval '1 hour'
    order by taken_at desc limit 1
) a on true
on conflict (owner_id, taken_at) do nothing;

insert into lifetime_indy.jita_sell_prices
    (type_id, price_date, sell_price, min_price, volume, order_count, updated_at)
select type_id, price_date, sell_price, min_price, volume, order_count, fetched_at
from networth.jita_prices
on conflict (type_id, price_date) do nothing;

commit;
