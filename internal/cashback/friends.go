package cashback

// The friends-sharing read path (docs/specs/friends-sharing.md): the
// friends module resolves WHO the viewer's friends are and WHICH clients
// they granted, handing both over through the injected ListSharedWithMe —
// this file turns the granted client ids into the shared picture.
// Cap/limit values never reach a viewer (invariant 4): the view structs
// simply have no cap fields, and lookup entries get theirs cleared before
// ranking.

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/sqkrv/sharespences/internal/db"
)

// SharedFriend is one friend as the friends module resolves them: named
// for display, with the client ids granted to the viewer (possibly none —
// «ничего не расшарено» is a real state the browse screen explains).
// Declared here so the injection crosses the seam as a function value, not
// a package import (ADR-0002).
type SharedFriend struct {
	UserID        uuid.UUID
	Username      string
	DisplayName   string
	BankClientIDs []int64
}

// FriendOfferView is one menu row as a friend sees it — deliberately
// without cap/limit fields.
type FriendOfferView struct {
	RawTitle     string
	Percent      *decimal.Decimal
	Kind         OfferKind
	CurrencyKind CurrencyKind
	PointsLabel  string
	Selected     bool
}

// FriendPeriodView is one period inside the shared window, with its rows
// split into the three chip groups.
type FriendPeriodView struct {
	Period   DateRange
	Selected []FriendOfferView // chosen regular rows
	Granted  []FriendOfferView // marked super/special rows (gold chips)
	Menu     []FriendOfferView // unselected rows — «ты можешь выбрать X»
}

// FriendClientView is one shared client's picture inside the shared window
// (invariant 8): the current period plus anything reaching into next month
// — the ritual's coordination horizon. Never history.
type FriendClientView struct {
	BankClientID int64
	BankName     string
	HolderLabel  string
	Periods      []FriendPeriodView // window periods, earliest first; empty — nothing shared yet
}

// FriendView groups one friend's shared clients.
type FriendView struct {
	UserID      uuid.UUID
	Username    string
	DisplayName string
	Clients     []FriendClientView
}

// FriendsOffers builds the browse payload (CB-06): every friend appears,
// zero-client friends included. `now` is server time — the shared window
// derives from it and from nothing the caller sends.
func (s *Service) FriendsOffers(ctx context.Context, viewerID uuid.UUID, now time.Time) ([]FriendView, error) {
	friends, rows, err := s.sharedRows(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	window := FriendShareWindow(now)
	byClient := make(map[int64][]db.ListOffersForClientsRow)
	for _, r := range rows {
		byClient[r.BankClientID] = append(byClient[r.BankClientID], r)
	}
	out := make([]FriendView, len(friends))
	for i, f := range friends {
		v := FriendView{
			UserID: f.UserID, Username: f.Username, DisplayName: f.DisplayName,
			Clients: []FriendClientView{},
		}
		for _, clientID := range f.BankClientIDs {
			v.Clients = append(v.Clients, friendClientView(clientID, byClient[clientID], window))
		}
		out[i] = v
	}
	return out, nil
}

// friendClientView keeps only the periods overlapping the shared window and
// splits each into the three chip groups.
func friendClientView(clientID int64, rows []db.ListOffersForClientsRow, window DateRange) FriendClientView {
	v := FriendClientView{
		BankClientID: clientID,
		Periods:      []FriendPeriodView{},
	}
	// Indexes, not pointers: appending to v.Periods may reallocate its
	// backing array, which would strand any *FriendPeriodView taken earlier.
	byPeriod := make(map[int64]int)
	for _, r := range rows {
		// Bank/держатель render even when no period falls in the window.
		v.BankName = r.BankName
		v.HolderLabel = holderOf(r.HolderLabel)
		period := rowRange(r.PeriodStart, r.PeriodEnd)
		if !period.Overlaps(window) {
			continue
		}
		idx, ok := byPeriod[r.OfferPeriodID]
		if !ok {
			idx = len(v.Periods)
			byPeriod[r.OfferPeriodID] = idx
			v.Periods = append(v.Periods, FriendPeriodView{
				Period:   period,
				Selected: []FriendOfferView{},
				Granted:  []FriendOfferView{},
				Menu:     []FriendOfferView{},
			})
		}
		row := FriendOfferView{
			RawTitle:     r.RawTitle,
			Percent:      r.Percent,
			Kind:         OfferKind(r.Kind),
			CurrencyKind: currencyOf(db.ListUserOffersRow(r)),
			Selected:     r.Selected,
		}
		if r.PointsLabel != nil {
			row.PointsLabel = *r.PointsLabel
		}
		p := &v.Periods[idx]
		switch {
		case r.Selected && row.Kind == OfferRegular:
			p.Selected = append(p.Selected, row)
		case r.Selected:
			p.Granted = append(p.Granted, row)
		default:
			p.Menu = append(p.Menu, row)
		}
	}
	sort.SliceStable(v.Periods, func(i, j int) bool { return v.Periods[i].Period.Start.Before(v.Periods[j].Period.Start) })
	byPercentDesc := func(s []FriendOfferView) func(i, j int) bool {
		return func(i, j int) bool { return cmpPercentDesc(s[i].Percent, s[j].Percent) < 0 }
	}
	for i := range v.Periods {
		sort.SliceStable(v.Periods[i].Selected, byPercentDesc(v.Periods[i].Selected))
		sort.SliceStable(v.Periods[i].Granted, byPercentDesc(v.Periods[i].Granted))
		sort.SliceStable(v.Periods[i].Menu, byPercentDesc(v.Periods[i].Menu))
	}
	return v
}

// friendLookupEntries returns friends' SELECTED rows of one category as
// rankable entries — never Available (verdicts are owner actions), never
// the fallback (base rates stay personal), caps cleared (invariant 4).
// Entries outside the shared window are dropped here (invariant 8): the
// lookup date param may pick a day WITHIN the window (e.g. next month's
// coordination), but can never reach history — the window itself derives
// from server time.
func (s *Service) friendLookupEntries(ctx context.Context, viewerID uuid.UUID, categoryID int64) ([]LookupEntry, error) {
	friends, rows, err := s.sharedRows(ctx, viewerID)
	if err != nil {
		return nil, err
	}
	window := FriendShareWindow(time.Now())
	owner := make(map[int64]*SharedFriend)
	for i := range friends {
		for _, id := range friends[i].BankClientIDs {
			owner[id] = &friends[i]
		}
	}
	var entries []LookupEntry
	for _, r := range rows {
		if !r.Selected || r.CanonicalCategoryID == nil || *r.CanonicalCategoryID != categoryID {
			continue
		}
		if !rowRange(r.PeriodStart, r.PeriodEnd).Overlaps(window) {
			continue
		}
		f := owner[r.BankClientID]
		if f == nil {
			continue
		}
		e := entryOf(db.ListUserOffersRow(r))
		e.CapValue, e.CapPerCategory, e.OfferCapValue, e.CapScope = nil, nil, nil, ""
		e.FriendName = f.DisplayName
		e.FriendUsername = f.Username
		entries = append(entries, e)
	}
	return entries, nil
}

// sharedRows resolves the viewer's friends + grants (via the injected seam)
// and loads the granted clients' offer rows. A nil injection means the
// module runs without the friends feature — an empty picture, not an error.
func (s *Service) sharedRows(ctx context.Context, viewerID uuid.UUID) ([]SharedFriend, []db.ListOffersForClientsRow, error) {
	if s.ListSharedWithMe == nil {
		return nil, nil, nil
	}
	friends, err := s.ListSharedWithMe(ctx, viewerID)
	if err != nil {
		return nil, nil, err
	}
	var ids []int64
	for _, f := range friends {
		ids = append(ids, f.BankClientIDs...)
	}
	if len(ids) == 0 {
		return friends, nil, nil
	}
	rows, err := s.Q.ListOffersForClients(ctx, ids)
	if err != nil {
		return nil, nil, err
	}
	return friends, rows, nil
}
