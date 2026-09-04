-- Bank-level cashback exclusions (ADR-0004): the flat state for «за это не
-- заплатят», fed by the snapshot import (`mcc-import`), journaled through
-- mcc_change like membership. Four kinds, because banks exclude on more
-- axes than MCC:
--   mcc            a code that never earns («4829»)
--   mcc_qualified  a code excluded only under a condition — the RU
--                  ecosystem block 3990-3999 resolves by merchant name and
--                  payment purpose, so one code carries SEVERAL rows with
--                  different conditions (the condition lives in note and is
--                  part of the row's identity, hence the expression index
--                  instead of a plain unique)
--   class          a non-MCC exclusion clause verbatim («при оплате … с
--                  использованием СБП», geography, payment channel)
--   descriptor     a named-merchant statement descriptor (ВТБ's blocklist,
--                  «www.verkkokauppa.com»)
-- value is text, not smallint: it holds codes for the mcc kinds (4-digit,
-- zero-padded — sub-1000 codes like 0005 exist) and prose for the rest.
-- source_id is the registry id of the document the row came from
-- (utils/mcc_sources.tsv): a bank publishes several exclusion documents
-- (Т-Банк's «Финансы» list and its cash-equivalent list both carry MCC
-- rows, ВТБ's ПЛ and its ТСП blocklist both carry descriptors), and the
-- import syncs each document against its own rows only — otherwise the
-- second document's import removes everything the first one added. The
-- same code listed by two documents is two attributable rows; resolve
-- deduplicates.

-- +goose Up
create type bank_exclusion_kind as enum ('mcc', 'mcc_qualified', 'class', 'descriptor');

create table bank_exclusion
(
    id        bigint generated always as identity primary key,
    bank_id   integer             not null references bank (id),
    kind      bank_exclusion_kind not null,
    value     text                not null,
    note      text,
    source_id text                not null
);
create unique index idx_bank_exclusion_identity
    on bank_exclusion (bank_id, source_id, kind, value, coalesce(note, ''));

-- Journal actions for exclusion movements. `excluded_imported` exists for
-- the same reason `imported` does: the first load of a bank's exclusion
-- list must never render as «bank excluded 74 codes today» in the digest.
-- Enum-value additions are one-way (00007 precedent): the Down deletes the
-- journal rows carrying these labels but cannot drop the labels themselves.
alter type mcc_change_action add value if not exists 'excluded_imported';
alter type mcc_change_action add value if not exists 'excluded_added';
alter type mcc_change_action add value if not exists 'excluded_removed';

-- +goose Down
delete
from mcc_change
where action in ('excluded_imported', 'excluded_added', 'excluded_removed');
drop table bank_exclusion;
drop type bank_exclusion_kind;
-- (the three mcc_change_action labels remain — PostgreSQL cannot drop them)
