-- Two name alignments, both from the banks' own documents, both needing a
-- migration for the same reason 00030 did: the seed inserts under a NOT EXISTS
-- guard, so a renamed literal adds a new row and leaves every existing
-- reference on an orphan the seed no longer maintains.
--
-- 1. bank «Сбербанк» → «СберБанк». The bank renders itself «СберБанк» (its app
--    is «СберБанк Онлайн»), and the closest-to-user naming rule takes the name
--    a client actually meets. bank.name is the seed's lookup key for programs,
--    tiers, catalogs, aliases, colors and the MCC membership CSV, so the
--    literals move in the same commit — the 00012 precedent, now also backed by
--    the unique constraint added in 00031.
--
-- 2. ВТБ's tiers → the package names the loyalty rules actually use. The seed
--    shipped «Стандартный» and «Привилегия», which are this project's own
--    labels; Таблица 2 of the «Мультибонус» rules names the packages
--    «Мультикарта», «Привилегия-Мультикарта» and «Прайм+». Renaming keeps every
--    bank_client on the tier it already references while making all three rows
--    document-sourced — the alternative was a set reading «Стандартный»,
--    «Привилегия», «Прайм+», mixing two invented names with one real one.

-- +goose Up
update bank
set name = 'СберБанк'
where name = 'Сбербанк';

update program_tier pt
set name = 'Мультикарта'
from cashback_program cp
         join bank b on b.id = cp.bank_id
where pt.program_id = cp.id
  and b.name = 'ВТБ'
  and pt.name = 'Стандартный';

update program_tier pt
set name = 'Привилегия-Мультикарта'
from cashback_program cp
         join bank b on b.id = cp.bank_id
where pt.program_id = cp.id
  and b.name = 'ВТБ'
  and pt.name = 'Привилегия';

-- +goose Down
update bank
set name = 'Сбербанк'
where name = 'СберБанк';

update program_tier pt
set name = 'Стандартный'
from cashback_program cp
         join bank b on b.id = cp.bank_id
where pt.program_id = cp.id
  and b.name = 'ВТБ'
  and pt.name = 'Мультикарта';

update program_tier pt
set name = 'Привилегия'
from cashback_program cp
         join bank b on b.id = cp.bank_id
where pt.program_id = cp.id
  and b.name = 'ВТБ'
  and pt.name = 'Привилегия-Мультикарта';
