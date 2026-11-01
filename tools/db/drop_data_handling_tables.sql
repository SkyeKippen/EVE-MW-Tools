-- Drops the tables the old Data-Handling market collector wrote to, now that
-- Meadow-Works Tools keeps its own prices and EVE Ref's market history
-- (lifetime_indy.jita_sell_prices and lifetime_indy.market_history).
--
-- Stop Data-Handling and Front-End first: Front-End reads these tables, and
-- Data-Handling's hourly run would hold them locked or fail without them.
--
-- Data-Handling's scripts put its tables in different schemas over time, so
-- this looks for them in meadow_works, meadow_works_test and public. A table
-- is only dropped when its columns are exactly Data-Handling's, so QS-Indy's
-- tables in meadow_works (including its own locations table) are never
-- touched. Nothing is dropped with CASCADE: if anything else depends on one
-- of these tables, nothing is dropped at all.
--
-- It's plain SQL, so it runs the same in psql or GoLand's database console:
--   psql "$DATABASE_URL" -f tools/db/drop_data_handling_tables.sql
-- As it is, it only lists what it would drop and how big it is. To drop
-- them, which can't be undone, change "confirm boolean := false" below to
-- true and run it again.

create or replace temp view data_handling_found as
with data_handling_tables (drop_order, table_name, columns) as (values
    (1, 'daily_data', '{id,is_buy_order,location_id,max_price,min_price,price_95,time_pulled,type_id,volume_remain,volume_sold}'::text[]),
    (2, 'order_price_history', '{id,is_buy_order,location_id,order_id,price,recorded_at,type_id,volume_remain}'),
    (3, 'orders', '{is_buy_order,last_updated,location_id,order_id,price,type_id,volume_remain}'),
    (4, 'historical_data', '{avg_price,date,max_price,min_price,region_id,type_id,volume_sold}'),
    (5, 'locations', '{location_id,name,region_id}'),
    (6, 'regions', '{name,region_id}'))
select n.nspname::text as schema_name, c.relname::text as table_name, d.drop_order,
       array(select a.attname::text from pg_attribute a
             where a.attrelid = c.oid and a.attnum > 0 and not a.attisdropped
             order by a.attname) = d.columns as matches,
       pg_total_relation_size(c.oid) as bytes,
       greatest(c.reltuples, 0)::bigint as approx_rows
from pg_class c
join pg_namespace n on n.oid = c.relnamespace
join data_handling_tables d on d.table_name = c.relname
where c.relkind = 'r'
  and n.nspname in ('meadow_works', 'meadow_works_test', 'public');

-- what's there, and what would be dropped
select schema_name || '.' || table_name as "table",
       case when matches then 'drop' else 'keep: not Data-Handling''s columns' end as action,
       pg_size_pretty(bytes) as size,
       approx_rows as "rows (approx.)",
       pg_size_pretty(sum(bytes) filter (where matches) over ()) as "total freed"
from data_handling_found
order by schema_name, drop_order;

-- A DO block is one transaction: any error drops nothing.
do $$
declare
    confirm boolean := false; -- set to true to drop the tables marked drop
    t record;
    test_schema oid := to_regnamespace('meadow_works_test');
begin
    if not confirm then
        raise notice 'Nothing was dropped. Set confirm to true in the script to drop the tables marked drop.';
        return;
    end if;
    for t in select schema_name, table_name from data_handling_found where matches order by drop_order loop
        execute format('drop table %I.%I', t.schema_name, t.table_name);
        raise notice 'dropped %.%', t.schema_name, t.table_name;
    end loop;
    -- meadow_works_test only ever held Data-Handling's tables
    if test_schema is not null and not exists (select from pg_class where relnamespace = test_schema) then
        drop schema meadow_works_test;
        raise notice 'dropped schema meadow_works_test';
    end if;
end $$;

drop view data_handling_found;
