-- Stores each character's and corporation's current blueprints. Every
-- refresh replaces an owner's whole list, like assets.
--   quantity     -1 an original, -2 a copy, above 0 a stack of originals
--   runs         runs left on a copy; -1 for an original
--   place_id     the station or structure the blueprint is in, following it
--                out of containers through the owner's assets; NULL when it
--                isn't in one
-- Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists blueprints (
    owner_id bigint not null,
    owner_type text not null check (owner_type in ('character', 'corporation')),
    item_id bigint not null,
    type_id bigint not null,
    location_id bigint not null,
    location_flag text not null,
    quantity bigint not null,
    runs bigint not null,
    material_efficiency int not null,
    time_efficiency int not null,
    place_id bigint,
    updated_at timestamptz not null default now(),
    primary key (owner_id, item_id)
);
create index if not exists blueprints_place_idx on blueprints (owner_id, place_id);
commit;
