-- Fixes base_ore on mining rows saved before the app knew moon ore variants,
-- e.g. "Brimful Zeolites" or "Glistening Zeolites" becomes "Zeolites". Each
-- moon ore rarity has its own pair of prefixes (+15% and +100%). The Mining
-- page already groups by the ore type's name, so this only tidies the saved
-- column. Safe to run more than once.
begin;
set search_path to lifetime_indy;

update mining_ledger
set base_ore = regexp_replace(base_ore, '^(Brimful|Glistening|Copious|Twinkling|Lavish|Shimmering|Replete|Glowing|Bountiful|Shining) ', '')
where base_ore ~ '^(Brimful|Glistening|Copious|Twinkling|Lavish|Shimmering|Replete|Glowing|Bountiful|Shining) ';

update corp_mining_ledger
set base_ore = regexp_replace(base_ore, '^(Brimful|Glistening|Copious|Twinkling|Lavish|Shimmering|Replete|Glowing|Bountiful|Shining) ', '')
where base_ore ~ '^(Brimful|Glistening|Copious|Twinkling|Lavish|Shimmering|Replete|Glowing|Bountiful|Shining) ';
commit;
