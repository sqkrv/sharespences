package perks

import (
	"encoding/json"
	"os"
	"testing"
	"time"
)

// TestImportPlanReconciles replays a backfill plan — windows, inferred events
// and the snapshots they were inferred from — through this module's own
// arithmetic, and fails on any snapshot the ledger cannot reproduce.
//
// A backfill reconstructs events from readings of a bank's counter, so it can
// only be as good as its model of how those counters compose. That model is
// EffectiveSize/Consumed/CheckSnapshot, and an importer living outside this
// repo carries its own copy of it; this is what stops the two from drifting
// apart silently, and it answers the question that matters before anything is
// written: how many windows will open already flagged «расходится с банком»?
//
// Skipped unless PERKS_PLAN names a plan file. Import plans are built from
// personal records, so none is checked in.
func TestImportPlanReconciles(t *testing.T) {
	path := os.Getenv("PERKS_PLAN")
	if path == "" {
		t.Skip("set PERKS_PLAN to a plan produced by utils/perks_routine-importer.py")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var plan struct {
		Perks []struct {
			Row    string `json:"row"`
			Quotas []struct {
				Key    string  `json:"key"`
				Start  string  `json:"window_start"`
				End    string  `json:"window_end"`
				Size   int     `json:"size"`
				Parent *string `json:"parent"`
			} `json:"quotas"`
			Events []struct {
				Quota string `json:"quota"`
				Kind  string `json:"kind"`
				Qty   int    `json:"qty"`
				Date  string `json:"date"`
			} `json:"events"`
			Snapshots []struct {
				Quota     string `json:"quota"`
				Remaining int    `json:"remaining"`
				Date      string `json:"date"`
			} `json:"snapshots"`
		} `json:"perks"`
	}
	if err := json.Unmarshal(raw, &plan); err != nil {
		t.Fatal(err)
	}

	day := func(s string) time.Time {
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			t.Fatalf("bad date %q: %v", s, err)
		}
		return d
	}

	var quotas, events, snaps, bad int
	for _, p := range plan.Perks {
		// Ids are synthetic: the plan keys windows by name, the domain by id.
		id := map[string]int64{}
		byID := map[int64]Quota{}
		for i, q := range p.Quotas {
			id[q.Key] = int64(i + 1)
		}
		for _, q := range p.Quotas {
			row := Quota{ID: id[q.Key], Size: q.Size}
			if q.Parent != nil {
				pid := id[*q.Parent]
				row.ParentID = &pid
			}
			byID[row.ID] = row
		}
		ledger := make([]Event, 0, len(p.Events))
		for i, e := range p.Events {
			q := byID[id[e.Quota]]
			ledger = append(ledger, Event{
				ID: int64(i + 1), QuotaID: q.ID, ParentID: q.ParentID,
				Kind: Kind(e.Kind), Qty: e.Qty, Date: day(e.Date),
			})
		}
		quotas += len(p.Quotas)
		events += len(p.Events)
		for _, s := range p.Snapshots {
			snaps++
			q := byID[id[s.Quota]]
			snap := &Snapshot{QuotaID: q.ID, ObservedOn: day(s.Date), Remaining: s.Remaining}
			if d := CheckSnapshot(q, ledger, snap); d != nil {
				bad++
				t.Errorf("%s %s %s: ledger says %d, the note says %d", p.Row, s.Quota, s.Date, d.Computed, d.Bank)
			}
		}
	}
	t.Logf("replayed %d windows, %d events, %d snapshots — %d disagree", quotas, events, snaps, bad)
}
