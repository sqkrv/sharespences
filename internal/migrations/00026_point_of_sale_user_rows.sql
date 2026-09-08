-- User-created точки продаж (redesign 5e, 2026-08-12): the zero-results
-- tail of «Поиск» lets anyone add a point by MCC. A submission is pending
-- until moderated in the admin sidecar; its author sees it in search right
-- away, the общий каталог gets it after approval. Existing rows (the
-- mcc-codes.ru import) stay approved via the column default.
--
-- author_user_id is ON DELETE SET NULL deliberately: a contributed point
-- stays in the каталог when its author's account is erased — the row itself
-- carries no personal data (runbooks/account-deletion.md covers the FK).

-- +goose Up
create type point_of_sale_status as enum ('approved', 'pending');

alter table point_of_sale
    add column status point_of_sale_status not null default 'approved',
    add column author_user_id uuid references "user" (id) on delete set null;

create index point_of_sale_pending_idx on point_of_sale (created_at)
    where status = 'pending';
create index point_of_sale_author_idx on point_of_sale (author_user_id)
    where author_user_id is not null;

-- +goose Down
alter table point_of_sale
    drop column author_user_id,
    drop column status;

drop type point_of_sale_status;
