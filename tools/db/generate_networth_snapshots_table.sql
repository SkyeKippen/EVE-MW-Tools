-- Stores what each character and corporation was worth every hour, for the
-- net worth graph on My Worth. The hourly refresh adds a row per owner from
-- what it saved: liquid is wallets plus buy order escrow, assets are items
-- and sell orders at the latest Jita price, and PLEX is PLEX in hangars.
--   taken_at          the hour, so every owner's rows line up
--   unpriced_types    item types left out for having no Jita price
-- Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists networth_snapshots (
    owner_id bigint not null,
    owner_type text not null check (owner_type in ('character', 'corporation')),
    taken_at timestamptz not null,
    liquid numeric(24,2) not null,
    assets numeric(24,2) not null,
    plex numeric(24,2) not null,
    unpriced_types int not null default 0,
    primary key (owner_id, taken_at)
);
create index if not exists networth_snapshots_taken_at_idx on networth_snapshots (taken_at);
commit;
