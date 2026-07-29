package mcc

// Merchant-base import (the `import-pos` subcommand): loads the mcc-codes.ru
// points-of-sale scrape into point_of_sale from a local CSV — the dataset
// stays out of the repo (hygiene decision, PROJECT_MAP 2026-07) and enters
// each deployment at run time, ADR-0003 import-first style.
//
// Semantics:
//   - upsert by id — the site's own row UUID, so re-imports are idempotent
//     and a resumed/partial scrape just re-lands;
//   - rows already in the DB but absent from the CSV are KEPT: absence is
//     indistinguishable from a partial scrape, and future crowd-sourced rows
//     will never come from this CSV at all;
//   - rows whose MCC is missing from the dictionary are skipped and counted
//     (no stub dictionary rows — they would surface in the dictionary search);
//   - location is never written: the source has no coordinates, and a future
//     geocoded value must survive re-imports.

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// posHeader is the scraper's exact column set (utils/points_of_sale-parser.py).
var posHeader = []string{"id", "title", "merchant_title", "mcc", "type", "address", "confirmations", "created_at", "actual_at"}

// posTypes mirrors the point_of_sale_type enum labels (00001).
var posTypes = []string{"offline", "online", "app", "other"}

const posBatchSize = 1000

// posRow is one parsed CSV record, DB-shaped.
type posRow struct {
	ID              uuid.UUID
	Name            string
	MerchantTitle   *string
	MCC             int16
	Type            *string
	Address         *string
	Confirmations   int64
	CreatedAt       time.Time
	LastConfirmedAt *time.Time
}

// ImportStats summarizes an import run.
type ImportStats struct {
	Upserted        int
	BadRows         int
	UnknownMCCRows  int
	UnknownMCCCodes map[int16]int // code → skipped-row count
}

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// parsePosRow converts one CSV record into a posRow. Pure — unit-tested
// without a DB.
func parsePosRow(rec []string) (posRow, error) {
	if len(rec) != len(posHeader) {
		return posRow{}, fmt.Errorf("want %d fields, got %d", len(posHeader), len(rec))
	}
	id, err := uuid.Parse(rec[0])
	if err != nil {
		return posRow{}, fmt.Errorf("bad id %q: %w", rec[0], err)
	}
	if rec[1] == "" {
		return posRow{}, fmt.Errorf("empty title")
	}
	mcc, err := strconv.ParseInt(rec[3], 10, 16)
	if err != nil || mcc < 0 {
		return posRow{}, fmt.Errorf("bad mcc %q", rec[3])
	}
	if rec[4] != "" && !slices.Contains(posTypes, rec[4]) {
		return posRow{}, fmt.Errorf("bad type %q", rec[4])
	}
	confirmations, err := strconv.ParseInt(rec[6], 10, 64)
	if err != nil {
		return posRow{}, fmt.Errorf("bad confirmations %q", rec[6])
	}
	createdAt, err := time.Parse(time.DateOnly, rec[7])
	if err != nil {
		return posRow{}, fmt.Errorf("bad created_at %q", rec[7])
	}
	var lastConfirmedAt *time.Time
	if rec[8] != "" {
		t, err := time.Parse(time.DateOnly, rec[8])
		if err != nil {
			return posRow{}, fmt.Errorf("bad actual_at %q", rec[8])
		}
		lastConfirmedAt = &t
	}
	return posRow{
		ID:              id,
		Name:            rec[1],
		MerchantTitle:   optional(rec[2]),
		MCC:             int16(mcc),
		Type:            optional(rec[4]),
		Address:         optional(rec[5]),
		Confirmations:   confirmations,
		CreatedAt:       createdAt,
		LastConfirmedAt: lastConfirmedAt,
	}, nil
}

const posUpsertSQL = `
	insert into point_of_sale
		(id, name, merchant_title, mcc_code, type, address, confirmations, created_at, last_confirmed_at)
	values ($1, $2, $3, $4, $5::point_of_sale_type, $6, $7, $8, $9)
	on conflict (id) do update set
		name              = excluded.name,
		merchant_title    = excluded.merchant_title,
		mcc_code          = excluded.mcc_code,
		type              = excluded.type,
		address           = excluded.address,
		confirmations     = excluded.confirmations,
		created_at        = excluded.created_at,
		last_confirmed_at = excluded.last_confirmed_at`

// ImportPointsOfSale reads the scraper CSV and upserts every valid row.
// Malformed rows are skipped and counted, never abort the run; the whole
// import is one transaction, so a hard failure changes nothing.
func ImportPointsOfSale(ctx context.Context, pool *pgxpool.Pool, r io.Reader, logf func(format string, args ...any)) (ImportStats, error) {
	stats := ImportStats{UnknownMCCCodes: map[int16]int{}}
	if logf == nil {
		logf = func(string, ...any) {}
	}

	reader := csv.NewReader(r)
	reader.Comma = ';'
	header, err := reader.Read()
	if err != nil {
		return stats, fmt.Errorf("import pos: read header: %w", err)
	}
	if !slices.Equal(header, posHeader) {
		return stats, fmt.Errorf("import pos: unexpected header %v (want %v)", header, posHeader)
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return stats, fmt.Errorf("import pos: begin: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	known := map[int16]bool{}
	codes, err := tx.Query(ctx, `select code from mcc`)
	if err != nil {
		return stats, fmt.Errorf("import pos: load dictionary: %w", err)
	}
	for codes.Next() {
		var code int16
		if err := codes.Scan(&code); err != nil {
			return stats, fmt.Errorf("import pos: scan dictionary: %w", err)
		}
		known[code] = true
	}
	if err := codes.Err(); err != nil {
		return stats, fmt.Errorf("import pos: load dictionary: %w", err)
	}

	batch := &pgx.Batch{}
	flush := func() error {
		if batch.Len() == 0 {
			return nil
		}
		if err := tx.SendBatch(ctx, batch).Close(); err != nil {
			return fmt.Errorf("import pos: batch: %w", err)
		}
		batch = &pgx.Batch{}
		return nil
	}

	for line := 2; ; line++ {
		rec, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			// A structurally broken line (stray quote); skip it, keep going.
			stats.BadRows++
			logf("line %d: %v", line, err)
			continue
		}
		row, err := parsePosRow(rec)
		if err != nil {
			stats.BadRows++
			logf("line %d: %v", line, err)
			continue
		}
		if !known[row.MCC] {
			stats.UnknownMCCRows++
			stats.UnknownMCCCodes[row.MCC]++
			continue
		}
		batch.Queue(posUpsertSQL,
			row.ID, row.Name, row.MerchantTitle, row.MCC, row.Type,
			row.Address, row.Confirmations, row.CreatedAt, row.LastConfirmedAt)
		stats.Upserted++
		if batch.Len() >= posBatchSize {
			if err := flush(); err != nil {
				return stats, err
			}
		}
	}
	if err := flush(); err != nil {
		return stats, err
	}
	if err := tx.Commit(ctx); err != nil {
		return stats, fmt.Errorf("import pos: commit: %w", err)
	}
	return stats, nil
}
