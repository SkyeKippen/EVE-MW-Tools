-- Creates every table the tracker uses, in the lifetime_indy schema. Run it
-- with psql from the repo root, e.g.
--   psql "$DATABASE_URL" -f tools/db/setup_all_tables.sql
-- Each script is safe to run more than once, so this is too.
\ir generate_esi_tokens_table.sql
\ir generate_users_tables.sql
\ir generate_character_wallet_journal_table.sql
\ir generate_corp_wallet_journal_table.sql
\ir generate_wallet_balances_table.sql
\ir generate_wallet_transactions_table.sql
\ir generate_loyalty_points_table.sql
\ir generate_market_orders_table.sql
\ir generate_assets_table.sql
\ir generate_industry_jobs_table.sql
\ir generate_delivered_jobs_table.sql
\ir generate_mining_ledger_table.sql
\ir generate_corp_mining_ledger_table.sql
\ir generate_jita_sell_prices_table.sql
