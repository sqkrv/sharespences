package seed

// MCC seed: dictionary + per-bank category→MCC membership, from CSVs
// embedded at build time (first data-file embed — Go literals don't scale
// to ~1k dictionary rows + ~2.9k membership rows). The CSVs are DERIVED
// artifacts: generated from the meta-repo's sql_data/ curation by
// utils/mcc_seed_constructor.py (2026-07-21) — regenerate there, never
// hand-edit here. Provenance + mapping report: knowledge
// concepts/mcc-categories.md.
//
// Semantics (owner decisions 2026-07-20):
//   - dictionary: unconditional upsert (knowledge-derived refresh, like
//     emoji/brand colors);
//   - membership: refresh-via-diff, scoped to the catalog rows named in
//     the CSV — the seed IS the first import, so every actual change lands
//     in the mcc_change journal (`imported` on the baseline load, `added`/
//     `removed` on later refreshes). Re-running with no changes writes
//     nothing (E2E runs seed twice to enforce idempotency).

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/csv"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed data/mcc_codes.csv
var mccCodesCSV []byte

//go:embed data/bank_category_mcc.csv
var bankCategoryMCCCSV []byte

const mccImportSource = "import: sql_data/raw_categories.csv (owner curation 2025; derived 2026-07-21)"

type catalogRow struct {
	id     int64
	bankID int32
	title  string
}

func seedMCC(ctx context.Context, pool *pgxpool.Pool) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("seed mcc: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	// 1. Dictionary — unconditional refresh.
	dict, err := readCSV(mccCodesCSV, 3)
	if err != nil {
		return fmt.Errorf("seed mcc: dictionary csv: %w", err)
	}
	for _, rec := range dict {
		code, err := strconv.ParseInt(rec[0], 10, 16)
		if err != nil {
			return fmt.Errorf("seed mcc: bad dictionary code %q: %w", rec[0], err)
		}
		if _, err := tx.Exec(ctx, `
			insert into mcc (code, name, description)
			values ($1, $2, nullif($3, ''))
			on conflict (code) do update set name = excluded.name, description = excluded.description`,
			int16(code), rec[1], rec[2]); err != nil {
			return fmt.Errorf("seed mcc: dictionary %s: %w", rec[0], err)
		}
	}

	// 2. Catalog map (bank name, title) → bank_category row.
	catalog := map[[2]string]catalogRow{}
	rows, err := tx.Query(ctx, `
		select bc.id, bc.bank_id, b.name, bc.title
		from bank_category bc
		         join bank b on b.id = bc.bank_id`)
	if err != nil {
		return fmt.Errorf("seed mcc: load catalog: %w", err)
	}
	for rows.Next() {
		var c catalogRow
		var bankName string
		if err := rows.Scan(&c.id, &c.bankID, &bankName, &c.title); err != nil {
			return fmt.Errorf("seed mcc: scan catalog: %w", err)
		}
		catalog[[2]string{bankName, c.title}] = c
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("seed mcc: catalog rows: %w", err)
	}

	// 3. Desired membership from the CSV. An unknown (bank, title) is a HARD
	// error: the CSV and the bankCategories catalog ship in lockstep, and
	// silent drift here would strand MCC data on nonexistent categories.
	memb, err := readCSV(bankCategoryMCCCSV, 4)
	if err != nil {
		return fmt.Errorf("seed mcc: membership csv: %w", err)
	}
	desired := map[int64]map[int16]string{} // bank_category_id → code → note
	byID := map[int64]catalogRow{}
	for _, rec := range memb {
		key := [2]string{rec[0], rec[1]}
		c, ok := catalog[key]
		if !ok {
			return fmt.Errorf("seed mcc: membership references unknown catalog row %s / %s (regenerate the CSV against the current bankCategories)", rec[0], rec[1])
		}
		code, err := strconv.ParseInt(rec[2], 10, 16)
		if err != nil {
			return fmt.Errorf("seed mcc: bad membership code %q: %w", rec[2], err)
		}
		if desired[c.id] == nil {
			desired[c.id] = map[int16]string{}
			byID[c.id] = c
		}
		desired[c.id][int16(code)] = rec[3]
	}

	// 4. Current membership.
	current := map[int64]map[int16]*string{}
	rows, err = tx.Query(ctx, `select bank_category_id, mcc_code, note from bank_category_mcc`)
	if err != nil {
		return fmt.Errorf("seed mcc: load membership: %w", err)
	}
	for rows.Next() {
		var id int64
		var code int16
		var note *string
		if err := rows.Scan(&id, &code, &note); err != nil {
			return fmt.Errorf("seed mcc: scan membership: %w", err)
		}
		if current[id] == nil {
			current[id] = map[int16]*string{}
		}
		current[id][code] = note
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("seed mcc: membership rows: %w", err)
	}

	// 5. Diff — only over catalog rows the CSV names; other rows (future
	// crowd-sourced or manual memberships) are never seed's to touch.
	// Baseline is judged PER CATEGORY: a category whose membership was empty
	// (fresh DB, or a renamed catalog row re-landing its codes) journals as
	// `imported`, not as a burst of «bank added X» noise; real diffs on a
	// populated category journal as added/removed.
	journal := func(c catalogRow, code int16, act, source string) error {
		_, err := tx.Exec(ctx, `
			insert into mcc_change (bank_id, bank_category_id, category_title, mcc_code, action, source)
			values ($1, $2, $3, $4, $5::mcc_change_action, $6)`,
			c.bankID, c.id, c.title, code, act, source)
		return err
	}
	for id, want := range desired {
		c := byID[id]
		have := current[id]
		action, source := "added", "seed refresh"
		if len(have) == 0 {
			action, source = "imported", mccImportSource
		}
		for code, note := range want {
			if cur, ok := have[code]; ok {
				curNote := ""
				if cur != nil {
					curNote = *cur
				}
				if curNote != note { // note-only drift: update silently, not a ±MCC event
					if _, err := tx.Exec(ctx, `
						update bank_category_mcc set note = nullif($3, '')
						where bank_category_id = $1 and mcc_code = $2`, id, code, note); err != nil {
						return fmt.Errorf("seed mcc: note update: %w", err)
					}
				}
				continue
			}
			if _, err := tx.Exec(ctx, `
				insert into bank_category_mcc (bank_category_id, mcc_code, note)
				values ($1, $2, nullif($3, ''))`, id, code, note); err != nil {
				return fmt.Errorf("seed mcc: insert %d/%d: %w", id, code, err)
			}
			if err := journal(c, code, action, source); err != nil {
				return fmt.Errorf("seed mcc: journal add: %w", err)
			}
		}
		for code := range have {
			if _, ok := want[code]; ok {
				continue
			}
			if _, err := tx.Exec(ctx, `
				delete from bank_category_mcc where bank_category_id = $1 and mcc_code = $2`, id, code); err != nil {
				return fmt.Errorf("seed mcc: delete %d/%d: %w", id, code, err)
			}
			if err := journal(c, code, "removed", "seed refresh"); err != nil {
				return fmt.Errorf("seed mcc: journal remove: %w", err)
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("seed mcc: commit: %w", err)
	}
	return nil
}

func readCSV(data []byte, fields int) ([][]string, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = fields
	recs, err := r.ReadAll()
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, fmt.Errorf("empty csv")
	}
	return recs[1:], nil // drop header
}
