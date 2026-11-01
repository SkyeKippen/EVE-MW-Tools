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
-- of these tables, the script stops and changes nothing.
--
-- Run it once without confirm to see what it would drop and how big it is:
--   psql "$DATABASE_URL" -f tools/db/drop_data_handling_tables.sql
-- then again to drop them, which can't be undone:
--   psql "$DATABASE_URL" -v confirm=yes -f tools/db/drop_data_handling_tables.sql
\set ON_ERROR_STOP on
\if :{?confirm}
\else
\set confirm no
\endif

begin;

create temp table data_handling_tables (
    drop_order int,
    table_name text,
    columns text[] -- sorted
) on commit drop;
insert into data_handling_tables values
    (1, 'daily_data', '{id,is_buy_order,location_id,max_price,min_price,price_95,time_pulled,type_id,volume_remain,volume_sold}'),
    (2, 'order_price_history', '{id,is_buy_order,location_id,order_id,price,recorded_at,type_id,volume_remain}'),
    (3, 'orders', '{is_buy_order,last_updated,location_id,order_id,price,type_id,volume_remain}'),
    (4, 'historical_data', '{avg_price,date,max_price,min_price,region_id,type_id,volume_sold}'),
    (5, 'locations', '{location_id,name,region_id}'),
    (6, 'regions', '{name,region_id}');

create temp view data_handling_found as
select c.relnamespace::regnamespace::text as schema_name, c.relname::text as table_name, d.drop_order,
       array(select a.attname::text from pg_attribute a
             where a.attrelid = c.oid and a.attnum > 0 and not a.attisdropped
             order by a.attname) = d.columns as matches,
       pg_total_relation_size(c.oid) as bytes,
       greatest(c.reltuples, 0)::bigint as approx_rows
from pg_class c
join data_handling_tables d on d.table_name = c.relname
where c.relkind = 'r'
  and c.relnamespace::regnamespace::text in ('meadow_works', 'meadow_works_test', 'public');

\echo
\echo 'Data-Handling tables found:'
select schema_name || '.' || table_name as "table",
       case when matches then 'drop' else 'keep: not Data-Handling''s columns' end as action,
       pg_size_pretty(bytes) as size,
       approx_rows as "rows (approx.)"
from data_handling_found
order by schema_name, drop_order;
select pg_size_pretty(coalesce(sum(bytes), 0)) as "space freed" from data_handling_found where matches;

\if :confirm
do $$
declare
    t record;
begin
    for t in select schema_name, table_name from data_handling_found where matches order by drop_order loop
        execute format('drop table %I.%I', t.schema_name, t.table_name);
        raise notice 'dropped %.%', t.schema_name, t.table_name;
    end loop;
    -- meadow_works_test only ever held Data-Handling's tables
    if exists (select from pg_namespace where nspname = 'meadow_works_test')
       and not exists (select from pg_class where relnamespace = 'meadow_works_test'::regnamespace) then
        drop schema meadow_works_test;
        raise notice 'dropped schema meadow_works_test';
    end if;
end $$;
drop view data_handling_found;
commit;
\echo 'Done.'
\else
rollback;
\echo 'Nothing was dropped. To drop the tables marked drop, run again with -v confirm=yes'
\endif
