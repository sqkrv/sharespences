package perks

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/sqkrv/sharespences/internal/db"
)

// Service wires the ledger to storage. Every read and write is scoped through
// perk.user_id — the create statements do it inside the INSERT itself, so a
// foreign id yields 0 rows rather than a row someone else owns.
type Service struct {
	Q *db.Queries
}

func notFound(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func isPgCode(err error, code string) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == code
}

// day strips a timestamp to its date. Windows and event dates are `date`
// columns; comparing them against a wall clock that carries a time of day
// would put «today» after the last day of a window that is still open.
func day(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

// ---------------------------------------------------------------- perks

// PerkRow is a perk definition with its держатель and bank resolved. Since
// 00025 the perk belongs to one bank client, so both come from it.
type PerkRow struct {
	ID           int64
	BankClientID int64
	ClientLabel  *string
	BankID       int32
	BankName     string
	Name         string
	Unit         string
	Note         *string
}

func (s *Service) CreatePerk(ctx context.Context, userID uuid.UUID, bankClientID int64, name, unit string, note *string) (db.Perk, error) {
	name, unit = NormalizeName(name), NormalizeName(unit)
	if err := ValidatePerk(name, unit); err != nil {
		return db.Perk{}, err
	}
	p, err := s.Q.CreatePerk(ctx, db.CreatePerkParams{
		UserID: userID, BankClientID: bankClientID, Name: name, Unit: unit, Note: note,
	})
	if isPgCode(err, "23505") {
		return db.Perk{}, ErrPerkExists
	}
	// 0 rows: the держатель is missing or someone else's — the same 404 either
	// way (invariant 1).
	return p, notFound(err)
}

func (s *Service) ListPerks(ctx context.Context, userID uuid.UUID) ([]PerkRow, error) {
	rows, err := s.Q.ListPerksForUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]PerkRow, len(rows))
	for i, r := range rows {
		out[i] = PerkRow{
			ID: r.ID, BankClientID: r.BankClientID, ClientLabel: r.ClientLabel,
			BankID: r.BankID, BankName: r.BankName, Name: r.Name, Unit: r.Unit, Note: r.Note,
		}
	}
	return out, nil
}

func (s *Service) GetPerk(ctx context.Context, userID uuid.UUID, id int64) (PerkRow, error) {
	r, err := s.Q.GetPerkForUser(ctx, db.GetPerkForUserParams{ID: id, UserID: userID})
	if err != nil {
		return PerkRow{}, notFound(err)
	}
	return PerkRow{
		ID: r.ID, BankClientID: r.BankClientID, ClientLabel: r.ClientLabel,
		BankID: r.BankID, BankName: r.BankName, Name: r.Name, Unit: r.Unit, Note: r.Note,
	}, nil
}

// UpdatePerk patches name, unit and note; nil leaves a field alone. setNote
// separates «don't touch the note» from «clear it».
func (s *Service) UpdatePerk(ctx context.Context, userID uuid.UUID, id int64, name, unit *string, setNote bool, note *string) (db.Perk, error) {
	if name != nil {
		n := NormalizeName(*name)
		if err := ValidatePerkName(n); err != nil {
			return db.Perk{}, err
		}
		name = &n
	}
	if unit != nil {
		u := NormalizeName(*unit)
		if err := ValidatePerkUnit(u); err != nil {
			return db.Perk{}, err
		}
		unit = &u
	}
	p, err := s.Q.UpdatePerkForUser(ctx, db.UpdatePerkForUserParams{
		ID: id, UserID: userID, Name: name, Unit: unit, SetNote: setNote, Note: note,
	})
	if isPgCode(err, "23505") {
		return db.Perk{}, ErrPerkExists
	}
	return p, notFound(err)
}

// DeletePerk refuses while the perk still has windows (spec invariant 6): the
// quotas hold a manual ledger, and a cascade here would take it down silently.
func (s *Service) DeletePerk(ctx context.Context, userID uuid.UUID, id int64) error {
	n, err := s.Q.DeletePerkForUser(ctx, db.DeletePerkForUserParams{ID: id, UserID: userID})
	if isPgCode(err, "23503") {
		return ErrPerkHasQuotas
	}
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- quotas

// CreateQuota opens a window. A child is validated against its parent here so
// the user gets a sentence rather than a constraint name; the composite self-FK
// in migration 00024 backstops the same three rules.
func (s *Service) CreateQuota(ctx context.Context, userID uuid.UUID, perkID int64, parentID *int64, w Window, size int, note *string) (db.PerkQuotum, error) {
	w = Window{Start: day(w.Start), End: day(w.End)}
	if err := ValidateWindow(w, size); err != nil {
		return db.PerkQuotum{}, err
	}
	if parentID != nil {
		parent, err := s.Q.GetPerkQuotaForUser(ctx, db.GetPerkQuotaForUserParams{ID: *parentID, UserID: userID})
		if err != nil {
			return db.PerkQuotum{}, notFound(err)
		}
		p := ParentQuota{
			Quota:  Quota{ID: parent.ID, ParentID: parent.ParentQuotaID, Size: int(parent.Size)},
			Window: Window{Start: day(parent.WindowStart), End: day(parent.WindowEnd)},
			PerkID: parent.PerkID,
		}
		if err := ValidateChild(w, perkID, p); err != nil {
			return db.PerkQuotum{}, err
		}
	}
	q, err := s.Q.CreatePerkQuota(ctx, db.CreatePerkQuotaParams{
		PerkID: perkID, UserID: userID, ParentQuotaID: parentID,
		WindowStart: w.Start, WindowEnd: w.End, Size: int32(size), Note: note,
	})
	if err != nil {
		return db.PerkQuotum{}, notFound(err)
	}
	return q, nil
}

// UpdateQuota patches the note freely and `size` only while the window has no
// history (spec invariant 5) — once anything has been recorded against it, the
// size is a dated fact and changes through a resize event.
func (s *Service) UpdateQuota(ctx context.Context, userID uuid.UUID, id int64, size *int, setNote bool, note *string) (db.PerkQuotum, error) {
	if _, err := s.Q.GetPerkQuotaForUser(ctx, db.GetPerkQuotaForUserParams{ID: id, UserID: userID}); err != nil {
		return db.PerkQuotum{}, notFound(err)
	}
	var size32 *int32
	if size != nil {
		if *size < 0 {
			return db.PerkQuotum{}, ErrSizeNegative
		}
		h, err := s.Q.CountPerkQuotaHistory(ctx, id)
		if err != nil {
			return db.PerkQuotum{}, err
		}
		if h.Events > 0 || h.Snapshots > 0 {
			return db.PerkQuotum{}, ErrSizeLocked
		}
		v := int32(*size)
		size32 = &v
	}
	q, err := s.Q.UpdatePerkQuotaForUser(ctx, db.UpdatePerkQuotaForUserParams{
		ID: id, UserID: userID, Size: size32, SetNote: setNote, Note: note,
	})
	return q, notFound(err)
}

// DeleteQuota takes the window's events, snapshots and child windows with it —
// all cascades from migration 00024. The UI confirms first: this is the user's
// own manual ledger, not derived data that could be rebuilt.
func (s *Service) DeleteQuota(ctx context.Context, userID uuid.UUID, id int64) error {
	n, err := s.Q.DeletePerkQuotaForUser(ctx, db.DeletePerkQuotaForUserParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- ledger

func (s *Service) CreateEvent(ctx context.Context, userID uuid.UUID, quotaID int64, kind Kind, qty int, date time.Time, note *string) (db.PerkEvent, error) {
	if !ValidKind(string(kind)) {
		return db.PerkEvent{}, ErrUnknownKind
	}
	if err := ValidateEventQty(kind, qty); err != nil {
		return db.PerkEvent{}, err
	}
	e, err := s.Q.CreatePerkEvent(ctx, db.CreatePerkEventParams{
		QuotaID: quotaID, UserID: userID, Kind: db.PerkEventKind(kind),
		Qty: int32(qty), EventDate: day(date), Note: note,
	})
	if err != nil {
		return db.PerkEvent{}, notFound(err)
	}
	return e, nil
}

// DeleteEvent is the mistake undo. Nothing recomputes on the way out — the
// next read simply sees a shorter ledger.
func (s *Service) DeleteEvent(ctx context.Context, userID uuid.UUID, id int64) error {
	n, err := s.Q.DeletePerkEventForUser(ctx, db.DeletePerkEventForUserParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// CreateSnapshot records what the bank displayed. It never writes back to the
// ledger (spec invariant 3): a reading that disagrees raises the badge, and
// only an explicit adjust closes it.
func (s *Service) CreateSnapshot(ctx context.Context, userID uuid.UUID, quotaID int64, observedOn time.Time, remaining int, note *string) (db.PerkSnapshot, error) {
	if err := ValidateSnapshot(remaining); err != nil {
		return db.PerkSnapshot{}, err
	}
	snap, err := s.Q.CreatePerkSnapshot(ctx, db.CreatePerkSnapshotParams{
		QuotaID: quotaID, UserID: userID, ObservedOn: day(observedOn),
		Remaining: int32(remaining), Note: note,
	})
	if err != nil {
		return db.PerkSnapshot{}, notFound(err)
	}
	return snap, nil
}

func (s *Service) DeleteSnapshot(ctx context.Context, userID uuid.UUID, id int64) error {
	n, err := s.Q.DeletePerkSnapshotForUser(ctx, db.DeletePerkSnapshotForUserParams{ID: id, UserID: userID})
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ---------------------------------------------------------------- views

// QuotaView is a window with its counters worked out. Size is the EFFECTIVE
// size (grants and re-ratings applied), not the size the window opened at —
// that one is only interesting on the history screen, next to the events that
// moved it.
type QuotaView struct {
	ID          int64
	ParentID    *int64
	Window      Window
	InitialSize int
	Size        int
	Used        int
	Remaining   int
	Note        *string
	Discrepancy *Discrepancy
	LastSeenOn  *time.Time
	Children    []QuotaView
}

// PerkView groups one perk's windows for one bank client.
type PerkView struct {
	PerkID int64
	Name   string
	Unit   string
	Note   *string
	Quotas []QuotaView
}

// ClientView is one card on PV-01 — a bank client with everything currently
// running on it.
type ClientView struct {
	ClientID int64
	Label    *string
	BankID   int32
	BankName string
	Perks    []PerkView
}

// eventOf converts a ledger row into the domain's view of it.
func eventOf(id, quotaID int64, parentID *int64, kind db.PerkEventKind, qty int32, date time.Time) Event {
	return Event{ID: id, QuotaID: quotaID, ParentID: parentID, Kind: Kind(kind), Qty: int(qty), Date: day(date)}
}

// view computes one window's counters as of a date.
func view(id int64, parentID *int64, w Window, initial int, note *string, events []Event, snap *Snapshot, asOf time.Time) QuotaView {
	q := Quota{ID: id, ParentID: parentID, Size: initial}
	v := QuotaView{
		ID: id, ParentID: parentID, Window: w, InitialSize: initial, Note: note,
		Size: EffectiveSize(q, events, asOf), Used: Consumed(q, events, asOf),
		Discrepancy: CheckSnapshot(q, events, snap),
	}
	v.Remaining = v.Size - v.Used
	if snap != nil {
		on := snap.ObservedOn
		v.LastSeenOn = &on
	}
	return v
}

// Overview drives PV-01: every bank client the user has perk windows on, with
// each perk's currently running windows and the monthly children inside them.
func (s *Service) Overview(ctx context.Context, userID uuid.UUID, on time.Time) ([]ClientView, error) {
	asOf := day(on)
	rows, err := s.Q.ListActivePerkQuotasForUser(ctx, db.ListActivePerkQuotasForUserParams{UserID: userID, OnDate: asOf})
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []ClientView{}, nil
	}

	ids := make([]int64, len(rows))
	for i, r := range rows {
		ids[i] = r.ID
	}
	// The tree read covers children of the active windows too: a pool's
	// remaining counts uses recorded on months that closed long ago.
	eventRows, err := s.Q.ListPerkEventsForQuotaTree(ctx, ids)
	if err != nil {
		return nil, err
	}
	events := make([]Event, len(eventRows))
	for i, e := range eventRows {
		events[i] = eventOf(e.ID, e.QuotaID, e.ParentQuotaID, e.Kind, e.Qty, e.EventDate)
	}
	snaps, err := s.latestSnapshots(ctx, ids)
	if err != nil {
		return nil, err
	}

	// The query orders by bank, client, perk, roots before their children and
	// then by window start, so one pass appends in render order.
	var out []ClientView
	clientAt := map[int64]int{}
	perkAt := map[[2]int64]int{}
	quotaAt := map[int64][2]int{} // quota id → (client index, perk index)

	for _, r := range rows {
		ci, ok := clientAt[r.ClientID]
		if !ok {
			ci = len(out)
			clientAt[r.ClientID] = ci
			out = append(out, ClientView{
				ClientID: r.ClientID, Label: r.ClientLabel, BankID: r.BankID, BankName: r.BankName,
			})
		}
		pk := [2]int64{r.ClientID, r.PerkID}
		pi, ok := perkAt[pk]
		if !ok {
			pi = len(out[ci].Perks)
			perkAt[pk] = pi
			out[ci].Perks = append(out[ci].Perks, PerkView{
				PerkID: r.PerkID, Name: r.PerkName, Unit: r.Unit, Note: r.PerkNote,
			})
		}
		v := view(r.ID, r.ParentQuotaID, Window{Start: day(r.WindowStart), End: day(r.WindowEnd)},
			int(r.Size), r.Note, events, snaps[r.ID], asOf)

		if r.ParentQuotaID == nil {
			quotaAt[r.ID] = [2]int{ci, pi}
			out[ci].Perks[pi].Quotas = append(out[ci].Perks[pi].Quotas, v)
			continue
		}
		// A child whose pool is not in the active set (windows are data — the
		// two need not line up) still has to render, so it falls back to
		// standing on its own.
		at, ok := quotaAt[*r.ParentQuotaID]
		if !ok {
			out[ci].Perks[pi].Quotas = append(out[ci].Perks[pi].Quotas, v)
			continue
		}
		qs := out[at[0]].Perks[at[1]].Quotas
		for i := range qs {
			if qs[i].ID == *r.ParentQuotaID {
				qs[i].Children = append(qs[i].Children, v)
				break
			}
		}
	}
	return out, nil
}

// EventRow is one ledger row on the history screen.
type EventRow struct {
	ID      int64
	QuotaID int64
	Kind    Kind
	Qty     int
	Date    time.Time
	Note    *string
}

// SnapshotRow is one recorded reading of the bank's counter, with the
// ledger's own answer for the same date next to it — every reading is judged
// as of the day it was taken, not just the latest one.
type SnapshotRow struct {
	ID         int64
	QuotaID    int64
	ObservedOn time.Time
	Remaining  int
	Computed   int
	Note       *string
}

// PerkHistory drives PV-02: every window of one perk — running and closed —
// with the ledger and the readings behind each.
type PerkHistory struct {
	Perk      PerkRow
	Quotas    []HistoryQuota
	Events    []EventRow
	Snapshots []SnapshotRow
}

// HistoryQuota is a window as the ledger shows it. The держатель and bank come
// from the perk, which owns both.
type HistoryQuota struct {
	QuotaView
	ClientLabel *string
	BankName    string
}

func (s *Service) PerkHistory(ctx context.Context, userID uuid.UUID, perkID int64, on time.Time) (PerkHistory, error) {
	asOf := day(on)
	perk, err := s.GetPerk(ctx, userID, perkID)
	if err != nil {
		return PerkHistory{}, err
	}
	quotas, err := s.Q.ListPerkQuotasForPerk(ctx, db.ListPerkQuotasForPerkParams{PerkID: perkID, UserID: userID})
	if err != nil {
		return PerkHistory{}, err
	}
	eventRows, err := s.Q.ListPerkEventsForPerk(ctx, db.ListPerkEventsForPerkParams{PerkID: perkID, UserID: userID})
	if err != nil {
		return PerkHistory{}, err
	}
	snapRows, err := s.Q.ListPerkSnapshotsForPerk(ctx, db.ListPerkSnapshotsForPerkParams{PerkID: perkID, UserID: userID})
	if err != nil {
		return PerkHistory{}, err
	}

	events := make([]Event, len(eventRows))
	out := PerkHistory{Perk: perk, Events: make([]EventRow, len(eventRows)), Snapshots: make([]SnapshotRow, len(snapRows))}
	for i, e := range eventRows {
		events[i] = eventOf(e.ID, e.QuotaID, e.ParentQuotaID, e.Kind, e.Qty, e.EventDate)
		out.Events[i] = EventRow{ID: e.ID, QuotaID: e.QuotaID, Kind: Kind(e.Kind), Qty: int(e.Qty), Date: day(e.EventDate), Note: e.Note}
	}
	qByID := make(map[int64]Quota, len(quotas))
	for _, q := range quotas {
		qByID[q.ID] = Quota{ID: q.ID, ParentID: q.ParentQuotaID, Size: int(q.Size)}
	}

	// The latest reading per window drives the badge; the rest are history the
	// screen lists underneath — each carrying the ledger's number for its own
	// date, so an old reading stays honestly judged, not stamped by the badge.
	latest := map[int64]*Snapshot{}
	for i, sn := range snapRows {
		out.Snapshots[i] = SnapshotRow{
			ID: sn.ID, QuotaID: sn.QuotaID, ObservedOn: day(sn.ObservedOn), Remaining: int(sn.Remaining),
			Computed: Remaining(qByID[sn.QuotaID], events, day(sn.ObservedOn)), Note: sn.Note,
		}
		// ListPerkSnapshotsForPerk is ordered newest first, so the first row
		// seen for a window is the one to reconcile against.
		if _, seen := latest[sn.QuotaID]; !seen {
			latest[sn.QuotaID] = &Snapshot{ID: sn.ID, QuotaID: sn.QuotaID, ObservedOn: day(sn.ObservedOn), Remaining: int(sn.Remaining)}
		}
	}

	out.Quotas = make([]HistoryQuota, len(quotas))
	for i, q := range quotas {
		out.Quotas[i] = HistoryQuota{
			QuotaView: view(q.ID, q.ParentQuotaID, Window{Start: day(q.WindowStart), End: day(q.WindowEnd)},
				int(q.Size), q.Note, events, latest[q.ID], asOf),
			ClientLabel: q.ClientLabel, BankName: q.BankName,
		}
	}
	return out, nil
}

func (s *Service) latestSnapshots(ctx context.Context, quotaIDs []int64) (map[int64]*Snapshot, error) {
	rows, err := s.Q.ListLatestPerkSnapshots(ctx, quotaIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[int64]*Snapshot, len(rows))
	for _, r := range rows {
		out[r.QuotaID] = &Snapshot{ID: r.ID, QuotaID: r.QuotaID, ObservedOn: day(r.ObservedOn), Remaining: int(r.Remaining)}
	}
	return out, nil
}
