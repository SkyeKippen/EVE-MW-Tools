-- Stores each character's and corporation's current assets. Every refresh
-- replaces an owner's whole list, so items that were sold or moved away
-- disappear.
--   owner_type     character or corporation
--   location_id    a station, structure, solar system, or the item_id of the
--                  container, ship or office the item is in
--   place_id       the station or structure the item is in, following it out
--                  of containers, ships and offices; NULL in space
-- Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists assets (
    owner_id bigint not null,
    owner_type text not null check (owner_type in ('character', 'corporation')),
    item_id bigint not null,
    type_id bigint not null,
    quantity bigint not null,
    location_id bigint not null,
    location_flag text not null,
    location_type text not null,
    is_singleton bool not null,
    is_blueprint_copy bool not null default false,
    place_id bigint,
    updated_at timestamptz not null default now(),
    primary key (owner_id, item_id)
);
create index if not exists assets_type_id_idx on assets (owner_id, type_id);
create index if not exists assets_location_id_idx on assets (location_id);
-- added for stockpiles; filled in from the next refresh
alter table assets add column if not exists place_id bigint;
create index if not exists assets_place_id_idx on assets (owner_id, place_id);
commit;
