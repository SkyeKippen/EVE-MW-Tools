-- Stores market transactions (purchases and sales) from each character's and
-- corporation's wallet, from ESI's /wallet/transactions routes. ESI only
-- keeps 30 days, so the app saves them every hour and this table keeps
-- everything. The My Worth page uses them to split ISK earned and spent
-- into items, PLEX and everything else.
--   owner_type     character or corporation
--   division       1 to 7 for corporations, 0 for a character
--   is_personal    character transactions only: false when the character
--                  traded for their corporation, which the corporation's
--                  own transactions also show
--   client_id      who was on the other side of the trade
-- Safe to run more than once.
begin;
create schema if not exists lifetime_indy;
set search_path to lifetime_indy;

create table if not exists wallet_transactions (
    owner_id bigint not null,
    owner_type text not null check (owner_type in ('character', 'corporation')),
    division int not null check (division between 0 and 7),
    transaction_id bigint not null,
    date timestamptz not null,
    type_id bigint not null,
    quantity bigint not null,
    unit_price numeric(20,2) not null,
    is_buy bool not null,
    is_personal bool,
    client_id bigint not null,
    location_id bigint not null,
    journal_ref_id bigint not null,
    fetched_at timestamptz not null default now(),
    primary key (owner_id, division, transaction_id)
);
create index if not exists wallet_transactions_type_id_idx on wallet_transactions (type_id);
commit;
