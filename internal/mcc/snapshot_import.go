package mcc

// DB side of the snapshot import (`mcc-import` subcommand): load the
// current state for the snapshot's bank, plan (snapshot.go), render the
// review report, apply in one transaction. Raw SQL over the module's own
// tables plus the documented read-only reference reads (bank,
// bank_category) — the import.go / seed precedent, no sqlc surface.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/sqkrv/sharespences/internal/cashback"
)

// ImportSnapshot parses, plans and — unless dryRun — applies one snapshot.
// Re-importing an already-applied snapshot is a no-op. The rendered plan
// goes through logf either way; dry-run writes nothing.
func ImportSnapshot(ctx context.Context, pool *pgxpool.Pool, data []byte, dryRun bool, logf func(format string, args ...any)) error {
	if logf == nil {
		logf = func(string, ...any) {}
	}
	snap, err := ParseSnapshot(data)
	if err != nil {
		return err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("mcc-import: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var bankID int32
	err = tx.QueryRow(ctx, `select id from bank where name = $1`, snap.Bank).Scan(&bankID)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("mcc-import: bank %q is not in the database (bank.name is the exact lookup key)", snap.Bank)
	}
	if err != nil {
		return fmt.Errorf("mcc-import: resolve bank: %w", err)
	}

	in, err := loadPlanInput(ctx, tx, bankID, snap.Source.ID)
	if err != nil {
		return err
	}
	plan, err := PlanImport(snap, in)
	if err != nil {
		return err
	}
	plan.Render(snap.Bank, logf)
	if dryRun {
		logf("dry run — nothing written")
		return nil
	}
	if plan.Empty() {
		return nil
	}
	if err := applyPlan(ctx, tx, bankID, snap.Source.ID, plan); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("mcc-import: commit: %w", err)
	}
	return nil
}

// loadPlanInput reads the bank's current state; exclusions are scoped to the
// snapshot's source document, so sibling documents' rows are invisible to
// the diff and survive it.
func loadPlanInput(ctx context.Context, tx pgx.Tx, bankID int32, sourceID string) (PlanInput, error) {
	in := PlanInput{
		Membership:      map[int64]map[int16]string{},
		KnownCodes:      map[int16]bool{},
		Exclusions:      map[string]map[string]string{},
		JournaledTitles: map[string]bool{},
	}

	rows, err := tx.Query(ctx, `
		select id, bank_id, title from bank_category
		where bank_id = $1 and created_by is null`, bankID)
	if err != nil {
		return in, fmt.Errorf("mcc-import: load catalog: %w", err)
	}
	for rows.Next() {
		var c CatalogEntry
		if err := rows.Scan(&c.ID, &c.BankID, &c.Title); err != nil {
			return in, fmt.Errorf("mcc-import: scan catalog: %w", err)
		}
		in.Catalog = append(in.Catalog, c)
	}
	if err := rows.Err(); err != nil {
		return in, fmt.Errorf("mcc-import: catalog rows: %w", err)
	}

	rows, err = tx.Query(ctx, `
		select bcm.bank_category_id, bcm.mcc_code, coalesce(bcm.note, '')
		from bank_category_mcc bcm
		         join bank_category bc on bc.id = bcm.bank_category_id
		where bc.bank_id = $1 and bc.created_by is null`, bankID)
	if err != nil {
		return in, fmt.Errorf("mcc-import: load membership: %w", err)
	}
	for rows.Next() {
		var id int64
		var code int16
		var note string
		if err := rows.Scan(&id, &code, &note); err != nil {
			return in, fmt.Errorf("mcc-import: scan membership: %w", err)
		}
		if in.Membership[id] == nil {
			in.Membership[id] = map[int16]string{}
		}
		in.Membership[id][code] = note
	}
	if err := rows.Err(); err != nil {
		return in, fmt.Errorf("mcc-import: membership rows: %w", err)
	}

	rows, err = tx.Query(ctx, `select code from mcc`)
	if err != nil {
		return in, fmt.Errorf("mcc-import: load dictionary: %w", err)
	}
	for rows.Next() {
		var code int16
		if err := rows.Scan(&code); err != nil {
			return in, fmt.Errorf("mcc-import: scan dictionary: %w", err)
		}
		in.KnownCodes[code] = true
	}
	if err := rows.Err(); err != nil {
		return in, fmt.Errorf("mcc-import: dictionary rows: %w", err)
	}

	rows, err = tx.Query(ctx, `
		select kind::text, value, coalesce(note, '') from bank_exclusion
		where bank_id = $1 and source_id = $2`, bankID, sourceID)
	if err != nil {
		return in, fmt.Errorf("mcc-import: load exclusions: %w", err)
	}
	for rows.Next() {
		var kind, value, note string
		if err := rows.Scan(&kind, &value, &note); err != nil {
			return in, fmt.Errorf("mcc-import: scan exclusions: %w", err)
		}
		if in.Exclusions[kind] == nil {
			in.Exclusions[kind] = map[string]string{}
		}
		in.Exclusions[kind][ExclusionIdentity(value, note)] = note
	}
	if err := rows.Err(); err != nil {
		return in, fmt.Errorf("mcc-import: exclusion rows: %w", err)
	}

	rows, err = tx.Query(ctx, `
		select category_title from mcc_change
		where bank_id = $1 and action = 'category_added'`, bankID)
	if err != nil {
		return in, fmt.Errorf("mcc-import: load journal: %w", err)
	}
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			return in, fmt.Errorf("mcc-import: scan journal: %w", err)
		}
		in.JournaledTitles[cashback.NormalizeTitle(title)] = true
	}
	if err := rows.Err(); err != nil {
		return in, fmt.Errorf("mcc-import: journal rows: %w", err)
	}
	return in, nil
}

func applyPlan(ctx context.Context, tx pgx.Tx, bankID int32, sourceID string, plan *Plan) error {
	journal := func(categoryID *int64, title string, code *int16, action, note string) error {
		_, err := tx.Exec(ctx, `
			insert into mcc_change (bank_id, bank_category_id, category_title, mcc_code, action, source, note)
			values ($1, $2, $3, $4, $5::mcc_change_action, $6, nullif($7, ''))`,
			bankID, categoryID, title, code, action, plan.SourceID, note)
		return err
	}

	for _, g := range plan.DictionaryAdds {
		if _, err := tx.Exec(ctx, `
			insert into mcc (code, name) values ($1, $2)
			on conflict (code) do nothing`, mustCode(g.MCC), g.Name); err != nil {
			return fmt.Errorf("mcc-import: dictionary %s: %w", g.MCC, err)
		}
	}

	for _, m := range plan.Adds {
		if _, err := tx.Exec(ctx, `
			insert into bank_category_mcc (bank_category_id, mcc_code, note)
			values ($1, $2, nullif($3, ''))`, m.Category.ID, m.Code, m.Note); err != nil {
			return fmt.Errorf("mcc-import: add %s/%s: %w", m.Category.Title, FormatCode(m.Code), err)
		}
		action := "added"
		if plan.Baseline[m.Category.ID] {
			action = "imported"
		}
		if err := journal(&m.Category.ID, m.Category.Title, &m.Code, action, m.Note); err != nil {
			return fmt.Errorf("mcc-import: journal add: %w", err)
		}
	}
	for _, m := range plan.Removes {
		if _, err := tx.Exec(ctx, `
			delete from bank_category_mcc where bank_category_id = $1 and mcc_code = $2`,
			m.Category.ID, m.Code); err != nil {
			return fmt.Errorf("mcc-import: remove %s/%s: %w", m.Category.Title, FormatCode(m.Code), err)
		}
		if err := journal(&m.Category.ID, m.Category.Title, &m.Code, "removed", ""); err != nil {
			return fmt.Errorf("mcc-import: journal remove: %w", err)
		}
	}
	// Note-only drift updates silently — not a ±MCC event (seed precedent).
	for _, m := range plan.NoteUpdates {
		if _, err := tx.Exec(ctx, `
			update bank_category_mcc set note = nullif($3, '')
			where bank_category_id = $1 and mcc_code = $2`, m.Category.ID, m.Code, m.Note); err != nil {
			return fmt.Errorf("mcc-import: note update: %w", err)
		}
	}

	for _, title := range plan.NewTitles {
		if err := journal(nil, title, nil, "category_added", ""); err != nil {
			return fmt.Errorf("mcc-import: journal new title: %w", err)
		}
	}

	for _, e := range plan.ExclusionAdds {
		if _, err := tx.Exec(ctx, `
			insert into bank_exclusion (bank_id, kind, value, note, source_id)
			values ($1, $2::bank_exclusion_kind, $3, nullif($4, ''), $5)`,
			bankID, e.Kind, e.Value, e.Note, sourceID); err != nil {
			return fmt.Errorf("mcc-import: exclusion add %s/%s: %w", e.Kind, e.Value, err)
		}
		action := "excluded_added"
		if plan.ExclusionBaseline[e.Kind] {
			action = "excluded_imported"
		}
		if err := journal(nil, "", exclusionCode(e), action, exclusionJournalNote(e)); err != nil {
			return fmt.Errorf("mcc-import: journal exclusion add: %w", err)
		}
	}
	for _, e := range plan.ExclusionRemoves {
		if _, err := tx.Exec(ctx, `
			delete from bank_exclusion
			where bank_id = $1 and kind = $2::bank_exclusion_kind and value = $3
			  and coalesce(note, '') = $4 and source_id = $5`,
			bankID, e.Kind, e.Value, e.Note, sourceID); err != nil {
			return fmt.Errorf("mcc-import: exclusion remove %s/%s: %w", e.Kind, e.Value, err)
		}
		if err := journal(nil, "", exclusionCode(e), "excluded_removed", exclusionJournalNote(e)); err != nil {
			return fmt.Errorf("mcc-import: journal exclusion remove: %w", err)
		}
	}
	return nil
}

// exclusionCode: the journal's mcc_code column carries the code for the mcc
// kinds and null for prose kinds (class, descriptor).
func exclusionCode(e ExclusionChange) *int16 {
	if e.Kind == "mcc" || e.Kind == "mcc_qualified" {
		code := mustCode(e.Value)
		return &code
	}
	return nil
}

// exclusionJournalNote keeps the row self-sufficient in the journal: the
// condition for a qualified code, the verbatim clause/descriptor for the
// prose kinds.
func exclusionJournalNote(e ExclusionChange) string {
	if e.Kind == "class" || e.Kind == "descriptor" {
		return e.Value
	}
	return e.Note
}
