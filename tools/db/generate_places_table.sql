-- Stores the NPC stations and player structures where the users' characters
-- and corporations have assets, blueprints, industry jobs or market orders,
-- with their names, so stockpiles can be tied to one. The hourly refresh
-- looks each up on ESI, again once 30 days old (a day for structures that
-- couldn't be read).
--   kind         station or structure
--   accessible   false when no character could read the structure, so its
--                name may be missing or out of date
-- Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists places (
    location_id bigint primary key,
    kind text not null check (kind in ('station', 'structure')),
    accessible bool not null,
    name text not null default '',
    type_name text not null default '',
    owner_name text not null default '',
    solar_system text not null default '',
    region text not null default '',
    checked_at timestamptz not null default now()
);
commit;
