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

// FriendClientView is one shared client's current-period picture.
type FriendClientView struct {
	BankClientID int64
	BankName     string
	HolderLabel  string
	Period       *DateRange        // nil — no period covers the date
	Selected     []FriendOfferView // chosen regular rows
	Granted      []FriendOfferView // marked super/special rows (gold chips)
	Menu         []FriendOfferView // unselected rows — «ты можешь выбрать X»
}

// FriendView groups one friend's shared clients.
type FriendView struct {
	UserID      uuid.UUID
	Username    string
	DisplayName string
	Clients     []FriendClientView
}

// FriendsOffers builds the browse payload (CB-06): every friend appears,
// zero-client friends included.
func (s *Service) FriendsOffers(ctx context.Context, viewerID uuid.UUID, onDate time.Time) ([]FriendView, error) {
	friends, rows, err := s.sharedRows(ctx, viewerID)
	if err != nil {
		return nil, err
	}
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
			v.Clients = append(v.Clients, friendClientView(clientID, byClient[clientID], onDate))
		}
		out[i] = v
	}
	return out, nil
}

// friendClientView filters one client's rows to the period covering onDate
// and splits them into the three chip groups.
func friendClientView(clientID int64, rows []db.ListOffersForClientsRow, onDate time.Time) FriendClientView {
	v := FriendClientView{
		BankClientID: clientID,
		Selected:     []FriendOfferView{},
		Granted:      []FriendOfferView{},
		Menu:         []FriendOfferView{},
	}
	for _, r := range rows {
		// Bank/держатель render even when no period covers the date.
		v.BankName = r.BankName
		v.HolderLabel = holderOf(r.HolderLabel)
		period := rowRange(r.PeriodStart, r.PeriodEnd)
		if !period.Contains(onDate) {
			continue
		}
		if v.Period == nil {
			p := period
			v.Period = &p
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
		switch {
		case r.Selected && row.Kind == OfferRegular:
			v.Selected = append(v.Selected, row)
		case r.Selected:
			v.Granted = append(v.Granted, row)
		default:
			v.Menu = append(v.Menu, row)
		}
	}
	byPercentDesc := func(s []FriendOfferView) func(i, j int) bool {
		return func(i, j int) bool { return cmpPercentDesc(s[i].Percent, s[j].Percent) < 0 }
	}
	sort.SliceStable(v.Selected, byPercentDesc(v.Selected))
	sort.SliceStable(v.Granted, byPercentDesc(v.Granted))
	sort.SliceStable(v.Menu, byPercentDesc(v.Menu))
	return v
}

// friendLookupEntries returns friends' SELECTED rows of one category as
// rankable entries — never Available (verdicts are owner actions), never
// the fallback (base rates stay personal), caps cleared (invariant 4).
func (s *Service) friendLookupEntries(ctx context.Context, viewerID uuid.UUID, categoryID int64) ([]LookupEntry, error) {
	friends, rows, err := s.sharedRows(ctx, viewerID)
	if err != nil {
		return nil, err
	}
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
