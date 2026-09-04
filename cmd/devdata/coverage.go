package main

// Coverage passes: the shapes the history pass never produces on its own.
//
// fillPeriods draws a *plausible* wallet — which is exactly why it leaves the
// design untested at the edges. A generated month always has offers, every
// client always has cards, every partner offer is a live merchant one, and the
// target user has no friends at all. The passes here fill those gaps on
// purpose, so every branch a screen can take has a row behind it:
//
//   - friends in all four заявка states, sharing into the target user;
//   - clients that stress layout (no cards, five cards, a bank with no
//     program, a label long enough to wrap);
//   - a period with awkward offers and a period with none;
//   - partner offers across scope, currency, activation and lifecycle.
//
// Everything routes through the same services as the history pass, so the
// invariants hold and nothing here can express state the app could not.

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/shopspring/decimal"

	"github.com/sqkrv/sharespences/internal/auth"
	"github.com/sqkrv/sharespences/internal/cashback"
	"github.com/sqkrv/sharespences/internal/db"
	"github.com/sqkrv/sharespences/internal/friends"
)

// devPassword is the password every generated friend account gets, so they can
// be signed into to check a screen from the other side of a share.
const devPassword = "devpass123"

// forUser returns a generator writing as another user. Reference data (banks,
// tiers, catalogs) is shared; the per-user maps are not, and counters are a
// pointer so a friend's wallet reports into the same totals.
func (g *gen) forUser(uid uuid.UUID) *gen {
	c := *g
	c.userID = uid
	c.clients = map[string]int64{}
	c.tierOf = map[int64]tierRef{}
	c.bankOf = map[int64]string{}
	c.labelOf = map[int64]string{}
	return &c
}

// loadClients fills the per-user client maps from storage. A sub-generator
// starts empty, so without this a re-run tries to re-create a friend's wallet
// and hits the (user, bank, label) unique constraint.
func (g *gen) loadClients(ctx context.Context) error {
	bankByID := map[int32]string{}
	for name, id := range g.banks {
		bankByID[id] = name
	}
	existing, err := g.q.ListBankClientsForUser(ctx, g.userID)
	if err != nil {
		return err
	}
	for _, c := range existing {
		label := ""
		if c.Label != nil {
			label = *c.Label
		}
		bank := bankByID[c.BankID]
		g.clients[bank+"|"+label] = c.ID
		g.bankOf[c.ID] = bank
		g.labelOf[c.ID] = label
		if c.ProgramTierID != nil {
			for _, t := range g.tiers[bank] {
				if t.id == *c.ProgramTierID {
					g.tierOf[c.ID] = t
				}
			}
		}
	}
	return nil
}

type relation int

const (
	relAccepted relation = iota
	relPendingIncoming
	relPendingOutgoing
	relDeclined
)

type friendSpec struct {
	username string
	display  string
	rel      relation
	shares   int // how many of their clients they share INTO the target user
	wallet   []clientSpec
}

// friendProfiles covers every state «Друзья» can render, and — for the
// accepted ones — both sides of sharing: someone who shares two wallets,
// someone who shares one, and a friend who shares nothing (the empty state
// that is easy to forget exists).
var friendProfiles = []friendSpec{
	{
		// The richest friend: three wallets, all shared, including the Альфа
		// one that carries the барабан/pick stack.
		username: "marina", display: "Марина Ковалёва", rel: relAccepted, shares: 4,
		wallet: []clientSpec{
			{bank: "Т-Банк", tier: "Premium", cards: []cardSpec{{4415, "mir"}}},
			{bank: "Альфа-Банк", tier: "Альфа-Смарт", cards: []cardSpec{{9021, "visa"}, {9022, "mir"}}},
			{bank: "Ozon Банк", tier: "Ozon Premium", cards: []cardSpec{{6310, "mir"}}},
			{bank: "СберБанк", tier: "Подписка СберПрайм+", cards: []cardSpec{{2255, "mir"}}},
		},
	},
	{
		// Shares everything too, and carries a держатель label so the shared
		// view has to render «Павел · Общая», not just a bank name.
		username: "pavel", display: "Павел", rel: relAccepted, shares: 4,
		wallet: []clientSpec{
			{bank: "ВТБ", tier: "Привилегия", cards: []cardSpec{{7788, "mir"}}},
			{bank: "Ozon Банк", label: "Общая", tier: "Стандартный", cards: []cardSpec{{1290, "mastercard"}}},
			{bank: "Яндекс Пэй", tier: "Стандартный", cards: []cardSpec{{4477, "mir"}}},
			{bank: "Совкомбанк", tier: "Стандартный", cards: []cardSpec{{5140, "mir"}}},
		},
	},
	{
		// Friends, but shares nothing — «Пока ничем не делится». Deliberately
		// kept at shares: 0; it is the only empty state on that screen.
		username: "olga.dev", display: "Ольга", rel: relAccepted, shares: 0,
		wallet: []clientSpec{
			{bank: "Яндекс Пэй", tier: "Стандартный", cards: []cardSpec{{3040, "mir"}}},
			{bank: "МКБ", tier: "Выгодный", cards: []cardSpec{{8123, "mir"}}},
			{bank: "УБРиР", tier: "Подписка «Моя жизнь+»", cards: []cardSpec{{7350, "mir"}}},
		},
	},
	{
		// Sitting in the target user's inbox, waiting to be accepted. Has a
		// wallet, but nothing is visible until the заявка is answered.
		username: "nikita", display: "Никита", rel: relPendingIncoming,
		wallet: []clientSpec{{bank: "МКБ", tier: "Премиальный", cards: []cardSpec{{5566, "mir"}}}},
	},
	{
		// The target user asked; no answer yet.
		username: "sveta", display: "Света", rel: relPendingOutgoing,
	},
	{
		// Asked and was turned down — the row that must NOT look pending.
		username: "anton", display: "Антон Д.", rel: relDeclined,
	},
}

// ensureFriends builds the friend graph around the target user: accounts,
// their own small wallets, the заявка in whichever state the profile calls
// for, and the shares pointing back at the target.
func (g *gen) ensureFriends(ctx context.Context, last time.Time, months int) error {
	fs := &friends.Service{Q: g.q, Pool: g.pool}

	for _, spec := range friendProfiles {
		if g.dry {
			g.counters.friends++
			g.counters.shares += spec.shares
			continue
		}
		uid, created, err := g.ensureUser(ctx, spec.username, spec.display)
		if err != nil {
			return fmt.Errorf("friend %s: %w", spec.username, err)
		}
		if created {
			g.counters.friends++
		}

		// Their own wallet, built by the same code that builds the target's,
		// so a shared period looks exactly like a real one.
		sub := g.forUser(uid)
		if err := sub.loadClients(ctx); err != nil {
			return fmt.Errorf("friend %s clients: %w", spec.username, err)
		}
		if len(spec.wallet) > 0 {
			if err := sub.ensureClients(ctx, spec.wallet); err != nil {
				return fmt.Errorf("friend %s wallet: %w", spec.username, err)
			}
			if err := sub.fillPeriods(ctx, spec.wallet, last, months); err != nil {
				return fmt.Errorf("friend %s periods: %w", spec.username, err)
			}
		}

		if err := g.linkFriend(ctx, fs, uid, spec); err != nil {
			return fmt.Errorf("friend %s link: %w", spec.username, err)
		}

		// Shares run from the friend's side: they own the client, the target
		// is the viewer.
		if spec.rel == relAccepted && spec.shares > 0 {
			shared := 0
			for _, w := range spec.wallet {
				if shared >= spec.shares {
					break
				}
				clientID, ok := sub.clients[w.bank+"|"+w.label]
				if !ok {
					continue
				}
				var had int
				if err := g.pool.QueryRow(ctx, `select count(*) from friend_cashback_share
					where bank_client_id = $1`, clientID).Scan(&had); err != nil {
					return err
				}
				if err := fs.SetSharing(ctx, uid, clientID, g.userID, true); err != nil {
					return fmt.Errorf("share %s: %w", w.bank, err)
				}
				shared++
				if had == 0 {
					g.counters.shares++
				}
			}
		}
	}

	// The other direction: the target shares one of their own wallets out, so
	// «Чем я делюсь» is not empty either.
	if !g.dry {
		if marina, err := g.q.GetUserByUsername(ctx, "marina"); err == nil {
			for key, id := range g.clients {
				if key == "Альфа-Банк|" {
					var had int
					if err := g.pool.QueryRow(ctx, `select count(*) from friend_cashback_share
						where bank_client_id = $1`, id).Scan(&had); err != nil {
						return err
					}
					if err := fs.SetSharing(ctx, g.userID, id, marina.ID, true); err != nil {
						return fmt.Errorf("outgoing share: %w", err)
					}
					if had == 0 {
						g.counters.shares++
					}
					break
				}
			}
		}
		// An invite link nobody has claimed yet.
		if invites, err := fs.ListInvites(ctx, g.userID); err == nil && len(invites) == 0 {
			if _, _, err := fs.CreateInvite(ctx, g.userID); err != nil {
				return fmt.Errorf("invite: %w", err)
			}
			g.counters.invites++
		}
	}
	return nil
}

// linkFriend drives the заявка into the state the profile asks for, using the
// real service so invariant 6 (mutual pending collapses) applies as usual.
func (g *gen) linkFriend(ctx context.Context, fs *friends.Service, uid uuid.UUID, spec friendSpec) error {
	// Already connected or already asked? Leave it alone — re-running must not
	// resurrect a заявка the reviewer just answered by hand.
	existing, err := fs.ListFriends(ctx, g.userID)
	if err != nil {
		return err
	}
	for _, f := range existing {
		if f.UserID == uid {
			return nil
		}
	}
	// Any prior заявка counts, whatever its status: a declined or cancelled
	// row is neither a friendship nor pending, so checking only the inbox
	// re-sent (and re-declined) it on every run.
	var prior int
	if err := g.pool.QueryRow(ctx, `select count(*) from friend_request
		where (from_user_id = $1 and to_user_id = $2)
		   or (from_user_id = $2 and to_user_id = $1)`, g.userID, uid).Scan(&prior); err != nil {
		return err
	}
	if prior > 0 {
		return nil
	}

	switch spec.rel {
	case relAccepted:
		out, err := fs.SendRequest(ctx, uid, g.username)
		if err != nil {
			return err
		}
		g.counters.requests++
		if !out.Accepted && out.Request != nil {
			return fs.Accept(ctx, g.userID, out.Request.ID)
		}
	case relPendingIncoming:
		if _, err := fs.SendRequest(ctx, uid, g.username); err != nil {
			return err
		}
		g.counters.requests++
	case relPendingOutgoing:
		if _, err := fs.SendRequest(ctx, g.userID, spec.username); err != nil {
			return err
		}
		g.counters.requests++
	case relDeclined:
		out, err := fs.SendRequest(ctx, uid, g.username)
		if err != nil {
			return err
		}
		g.counters.requests++
		if !out.Accepted && out.Request != nil {
			return fs.Decline(ctx, g.userID, out.Request.ID)
		}
	}
	return nil
}

// ensureUser finds or creates an account. Usernames must already satisfy the
// 00016 canonical form; the normalizer runs before the check constraint, so a
// profile that violates it fails loudly here rather than silently.
func (g *gen) ensureUser(ctx context.Context, username, display string) (uuid.UUID, bool, error) {
	u, err := g.q.GetUserByUsername(ctx, auth.NormalizeUsername(username))
	if err == nil {
		return u.ID, false, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, false, err
	}
	hash, err := auth.HashPassword(devPassword)
	if err != nil {
		return uuid.Nil, false, err
	}
	created, err := g.q.CreateUser(ctx, db.CreateUserParams{
		Username: auth.NormalizeUsername(username), DisplayName: display,
		Email: username + "@dev.local", PasswordHash: &hash,
	})
	if err != nil {
		return uuid.Nil, false, err
	}
	return created.ID, true, nil
}

// edgeClients are the wallets that break layouts rather than represent
// typical use: a bank with no cashback program at all, a client with no cards,
// one with a card of every payment system, and a label long enough to wrap.
func (g *gen) fillEdgeClients(ctx context.Context) error {
	type edgeSpec struct {
		bank  string
		label string
		tier  string // empty = leave program_tier NULL
		cards []cardSpec
	}
	specs := []edgeSpec{
		// No tier chosen: the bank has a programme, the client has not said
		// which tier they are on, so slots are unknown — the branch every
		// other client avoids.
		{bank: "СберБанк", cards: []cardSpec{{2202, "mir"}}},
		// Longest tier name in the seed, against a long держатель label.
		{bank: "Банк Синара", label: "Свекровь", tier: "Опция «Можно больше»",
			cards: []cardSpec{{6677, "mir"}}},
		// Tier whose max_categories is NULL — slot count genuinely unknown.
		{bank: "ОТП Банк", label: "Private", tier: "Private", cards: []cardSpec{{9100, "visa"}}},
		// No cards at all: «Добавить карту» empty state on the client.
		{bank: "Газпромбанк", label: "Дача", tier: "Стандартный"},
		// Every payment system at once, and more cards than a row can show.
		{bank: "Т-Банк", label: "Общий счёт", tier: "Pro", cards: []cardSpec{
			{1001, "mir"}, {1002, "visa"}, {1003, "mastercard"},
			{1004, "unionpay"}, {1005, "american_express"},
		}},
		// A держатель label that has to wrap or truncate somewhere.
		{bank: "ВТБ", label: "Бабушка Валентина Дмитриевна", tier: "Стандартный",
			cards: []cardSpec{{7401, "mir"}}},
	}

	for _, spec := range specs {
		bankID, ok := g.banks[spec.bank]
		if !ok {
			log.Printf("skip edge client %s: bank not seeded", spec.bank)
			continue
		}
		key := spec.bank + "|" + spec.label
		if _, exists := g.clients[key]; exists {
			continue
		}
		if g.dry {
			g.counters.clients++
			g.counters.cards += len(spec.cards)
			continue
		}
		var tierID *int64
		if spec.tier != "" {
			t, ok := g.tiers[spec.bank][spec.tier]
			if !ok {
				return fmt.Errorf("edge tier %q not found for %s", spec.tier, spec.bank)
			}
			tierID = &t.id
		}
		var label *string
		if spec.label != "" {
			l := spec.label
			label = &l
		}
		c, err := g.q.CreateBankClient(ctx, db.CreateBankClientParams{
			UserID: g.userID, BankID: bankID, Label: label, ProgramTierID: tierID,
		})
		if err != nil {
			return fmt.Errorf("edge client %s: %w", key, err)
		}
		g.clients[key] = c.ID
		g.bankOf[c.ID] = spec.bank
		g.labelOf[c.ID] = spec.label
		g.counters.clients++

		for _, cd := range spec.cards {
			if _, err := g.q.CreateCard(ctx, db.CreateCardParams{
				BankClientID: c.ID, UserID: g.userID,
				Last4Digits: cd.last4, PaymentSystem: db.PaymentSystem(cd.system),
			}); err != nil {
				return fmt.Errorf("edge card %d: %w", cd.last4, err)
			}
			g.counters.cards++
		}
	}
	return nil
}

// fillEdgePeriod builds the awkward month: offers with no percent, a
// half-percent, a headline 100%, a title long enough to wrap twice, notes, and
// all three kinds side by side. A second client gets a period with nothing in
// it — the «месяц заведён, категорий нет» state.
//
// The month is found by scanning forward rather than assuming: the history
// pass may already have reached next month, and an overlap there previously
// meant the edge offers were skipped silently.
func (g *gen) fillEdgePeriod(ctx context.Context, last time.Time) error {
	if g.dry {
		g.counters.periods += 2
		return nil
	}
	pct := func(s string) *decimal.Decimal { d := decimal.RequireFromString(s); return &d }
	notes := "действует только при оплате картой Мир, до 3 операций"
	long := "Кафе, рестораны, бары, кофейни и доставка готовой еды по всей России"

	edge := []struct {
		title string
		pct   *decimal.Decimal
		kind  cashback.OfferKind
		notes *string
		cap   *decimal.Decimal
	}{
		{long, pct("5"), cashback.OfferRegular, nil, nil},
		{"Без процента", nil, cashback.OfferRegular, &notes, nil},
		{"Полпроцента", pct("0.5"), cashback.OfferRegular, nil, nil},
		{"Максимум", pct("100"), cashback.OfferSpecial, nil, pct("10000")},
		{"Суперкэшбек", pct("33"), cashback.OfferSuper, &notes, nil},
		{"Ёлки-палки", pct("7"), cashback.OfferRegular, nil, pct("500")},
	}

	// Idempotent: the awkward set is identified by one of its titles, so a
	// re-run neither duplicates it nor burns a fresh month on it.
	var already int
	if err := g.pool.QueryRow(ctx, `select count(*) from category_offer o
		join offer_period p on p.id = o.offer_period_id
		join bank_client c on c.id = p.bank_client_id
		where c.user_id = $1 and o.raw_title = 'Полпроцента'`, g.userID).Scan(&already); err != nil {
		return err
	}
	if already > 0 {
		return nil
	}

	if clientID, ok := g.clients["Альфа-Банк|"]; ok {
		period, err := g.freeMonthPeriod(ctx, clientID, last)
		if err != nil {
			return err
		}
		if period != nil {
			for _, e := range edge {
				if _, err := g.svc.CreateCategoryOffer(ctx, g.userID, *period, e.title,
					nil, e.pct, e.kind, e.notes, nil, e.cap); err != nil {
					return fmt.Errorf("edge offer %q: %w", e.title, err)
				}
				g.counters.offers++
			}
		} else {
			log.Printf("edge period: no free month within a year — skipped")
		}
	}

	// A period with nothing in it, on a different client.
	if empty, ok := g.clients["ВТБ|"]; ok {
		if _, err := g.freeMonthPeriod(ctx, empty, last); err != nil {
			return err
		}
	}
	return nil
}

// freeMonthPeriod creates a period in the first month after `last` that the
// client does not already have one for, and returns its id. nil means every
// month in the next year was taken.
func (g *gen) freeMonthPeriod(ctx context.Context, clientID int64, last time.Time) (*int64, error) {
	base := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= 12; i++ {
		start := base.AddDate(0, i, 0)
		end := start.AddDate(0, 1, -1)
		p, err := g.svc.CreateOfferPeriod(ctx, g.userID, clientID, start, end, nil)
		if err != nil {
			if errors.Is(err, cashback.ErrPeriodOverlap) {
				g.counters.skipped++
				continue
			}
			return nil, fmt.Errorf("edge period: %w", err)
		}
		g.counters.periods++
		id := p.ID
		return &id, nil
	}
	return nil, nil
}

// fillPartnerMatrix covers the partner-offer dimensions the history pass never
// varies. Every generated one there is a live, card-scoped, merchant-scope
// offer with no activation and no end — which leaves the category scope, the
// points currency, the merchant kinds, the activation pair and the ended state
// with no row behind them at all.
//
// The rows are written out explicitly rather than sampled: a design matrix is
// only useful if it is exhaustive and stable between runs.
func (g *gen) fillPartnerMatrix(ctx context.Context, last time.Time) error {
	const marker = "Лента (онлайн)" // first row; its presence means the matrix ran

	existing, err := g.q.ListPartnerOffersForUser(ctx, g.userID)
	if err != nil {
		return err
	}
	for _, o := range existing {
		if o.MerchantTitle == marker {
			return nil // already generated
		}
	}
	if g.dry {
		g.counters.partners += 14
		return nil
	}

	day := func(d time.Time) *time.Time { return &d }
	dec := func(s string) *decimal.Decimal { v := decimal.RequireFromString(s); return &v }
	str := func(s string) *string { return &s }
	kind := func(k db.PointOfSaleType) db.NullPointOfSaleType {
		return db.NullPointOfSaleType{PointOfSaleType: k, Valid: true}
	}
	cur := func(c db.CashbackCurrencyKind) db.NullCashbackCurrencyKind {
		return db.NullCashbackCurrencyKind{CashbackCurrencyKind: c, Valid: true}
	}

	now := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.UTC)
	past, pastEnd := now.AddDate(0, -4, 0), now.AddDate(0, -3, 0)
	future, futureEnd := now.AddDate(0, 2, 0), now.AddDate(0, 3, 0)
	curStart, curEnd := now, now.AddDate(0, 1, -1)

	// A canonical category for the category-scope rows (the check constraint
	// requires one), and a client for the card-scoped rows.
	var canonicalID *int64
	if rows, err := g.q.ListCanonicalCategories(ctx); err == nil && len(rows) > 0 {
		for _, r := range rows {
			if r.Slug == "supermarkets" {
				id := r.ID
				canonicalID = &id
				break
			}
		}
	}
	clientID, hasClient := g.clients["Альфа-Банк|"]
	var client *int64
	if hasClient {
		client = &clientID
	}

	alfa, vtb, mkb := g.banks["Альфа-Банк"], g.banks["ВТБ"], g.banks["МКБ"]

	type row struct {
		p       db.CreatePartnerOfferParams
		end     bool // close it afterwards, giving the «завершено» state a row
		skipCat bool // needs a canonical category that may not be seeded
	}
	rows := []row{
		// merchant kinds ×4, all live and card-scoped
		{p: db.CreatePartnerOfferParams{BankID: alfa, MerchantTitle: marker, Percent: dec("12"),
			ScopeKind: db.PartnerScopeMerchant, MerchantKind: kind(db.PointOfSaleTypeOnline),
			CurrencyKind: cur(db.CashbackCurrencyKindRub), ValidFrom: day(curStart), ValidTo: day(curEnd),
			BankClientID: client}},
		{p: db.CreatePartnerOfferParams{BankID: alfa, MerchantTitle: "Пятёрочка (офлайн)", Percent: dec("7"),
			ScopeKind: db.PartnerScopeMerchant, MerchantKind: kind(db.PointOfSaleTypeOffline),
			CurrencyKind: cur(db.CashbackCurrencyKindRub), BankClientID: client}},
		{p: db.CreatePartnerOfferParams{BankID: vtb, MerchantTitle: "Самокат (в приложении)", Percent: dec("15"),
			ScopeKind: db.PartnerScopeMerchant, MerchantKind: kind(db.PointOfSaleTypeApp),
			CurrencyKind: cur(db.CashbackCurrencyKindRub), MinAmount: dec("1500")}},
		{p: db.CreatePartnerOfferParams{BankID: vtb, MerchantTitle: "Прочее партнёрское", Percent: dec("3"),
			ScopeKind: db.PartnerScopeMerchant, MerchantKind: kind(db.PointOfSaleTypeOther)}},

		// category scope, both currencies
		{skipCat: true, p: db.CreatePartnerOfferParams{BankID: alfa, MerchantTitle: "Супермаркеты — категория",
			Percent: dec("5"), ScopeKind: db.PartnerScopeCategory, CurrencyKind: cur(db.CashbackCurrencyKindRub),
			CapValue: dec("3000"), ValidFrom: day(curStart), ValidTo: day(curEnd)}},
		{skipCat: true, p: db.CreatePartnerOfferParams{BankID: mkb, MerchantTitle: "Супермаркеты — баллами",
			Percent: dec("8"), ScopeKind: db.PartnerScopeCategory, CurrencyKind: cur(db.CashbackCurrencyKindPoints),
			Notes: str("баллами, списываются в приложении банка")}},

		// activation pair: one needing it and not yet activated, one activated
		{p: db.CreatePartnerOfferParams{BankID: alfa, MerchantTitle: "Лэтуаль (нужна активация)", Percent: dec("20"),
			ScopeKind: db.PartnerScopeMerchant, CurrencyKind: cur(db.CashbackCurrencyKindRub),
			RequiresActivation: true, ValidFrom: day(curStart), ValidTo: day(curEnd), BankClientID: client}},
		{p: db.CreatePartnerOfferParams{BankID: alfa, MerchantTitle: "Öko (активирован)", Percent: dec("25"),
			ScopeKind: db.PartnerScopeMerchant, CurrencyKind: cur(db.CashbackCurrencyKindRub),
			RequiresActivation: true, ActivatedAt: day(curStart.AddDate(0, 0, 3)), BankClientID: client}},

		// date windows: expired, upcoming, and no dates at all
		{p: db.CreatePartnerOfferParams{BankID: vtb, MerchantTitle: "Азбука Вкуса (истёк)", Percent: dec("10"),
			ScopeKind: db.PartnerScopeMerchant, CurrencyKind: cur(db.CashbackCurrencyKindRub),
			ValidFrom: day(past), ValidTo: day(pastEnd)}},
		{p: db.CreatePartnerOfferParams{BankID: vtb, MerchantTitle: "Ozon fresh (скоро)", Percent: dec("18"),
			ScopeKind: db.PartnerScopeMerchant, CurrencyKind: cur(db.CashbackCurrencyKindRub),
			ValidFrom: day(future), ValidTo: day(futureEnd)}},
		{p: db.CreatePartnerOfferParams{BankID: mkb, MerchantTitle: "Бессрочное предложение", Percent: dec("4"),
			ScopeKind: db.PartnerScopeMerchant, CurrencyKind: cur(db.CashbackCurrencyKindPoints)}},

		// no percent at all, and everything-at-once
		{p: db.CreatePartnerOfferParams{BankID: alfa, MerchantTitle: "Подарок без процента",
			ScopeKind: db.PartnerScopeMerchant, Notes: str("фиксированный подарок, процент не указан")}},
		{p: db.CreatePartnerOfferParams{BankID: alfa,
			MerchantTitle: "Магазин с очень длинным названием, которое обязано переноситься на вторую строку",
			Percent:       dec("9.5"), ScopeKind: db.PartnerScopeMerchant, MerchantKind: kind(db.PointOfSaleTypeOffline),
			CurrencyKind: cur(db.CashbackCurrencyKindRub), CapValue: dec("2500"), MinAmount: dec("999"),
			Notes: str("максимум 2 500 ₽ в месяц, от 999 ₽ в чеке"), ValidFrom: day(curStart),
			ValidTo: day(curEnd), RequiresActivation: true, BankClientID: client}},

		// closed: created live, then ended, so «завершено» has a row
		{end: true, p: db.CreatePartnerOfferParams{BankID: vtb, MerchantTitle: "Перекрёсток (завершено)",
			Percent: dec("6"), ScopeKind: db.PartnerScopeMerchant, CurrencyKind: cur(db.CashbackCurrencyKindRub),
			BankClientID: client}},
	}

	for _, r := range rows {
		if r.skipCat {
			if canonicalID == nil {
				continue
			}
			r.p.CanonicalCategoryID = canonicalID
		}
		if r.p.BankID == 0 {
			continue // bank not seeded
		}
		r.p.UserID = g.userID
		created, err := g.q.CreatePartnerOffer(ctx, r.p)
		if err != nil {
			return fmt.Errorf("partner matrix %q: %w", r.p.MerchantTitle, err)
		}
		g.counters.partners++
		if r.end {
			if _, err := g.q.EndPartnerOfferForUser(ctx, db.EndPartnerOfferForUserParams{
				ID: created.ID, UserID: g.userID,
			}); err != nil {
				return fmt.Errorf("end partner %q: %w", r.p.MerchantTitle, err)
			}
		}
	}
	return nil
}

// fillBarabanStack guarantees the case where Альфа's монthly барабан lands on
// a category that is ALREADY picked in the same period.
//
// This is not a collision — DetectCollisions is cross-client and regular-only,
// and super is granted rather than chosen, so it never warns (invariant 6).
// It is the stacking case: super is «a full-period STACKING bonus … stacks
// with the monthly pick», so when the барабан repeats a pick the same category
// carries both rates at once, and the period shows one category twice.
//
// The history pass cannot produce it: it draws the барабан title from a fixed
// list and passes no canonical category, so its super rows are category-less
// (and therefore not «the same category» to ranking or to the overview at all).
// Here the super row is given the picked row's own title AND canonical, which
// is what makes the two genuinely one category.
func (g *gen) fillBarabanStack(ctx context.Context) error {
	if g.dry {
		return nil
	}
	targets := []uuid.UUID{g.userID}
	for _, spec := range friendProfiles {
		if spec.rel != relAccepted || spec.shares == 0 {
			continue
		}
		hasAlfa := false
		for _, w := range spec.wallet {
			if w.bank == "Альфа-Банк" {
				hasAlfa = true
			}
		}
		if !hasAlfa {
			continue
		}
		u, err := g.q.GetUserByUsername(ctx, spec.username)
		if err != nil {
			continue
		}
		targets = append(targets, u.ID)
	}
	for _, uid := range targets {
		if err := g.barabanStackFor(ctx, uid); err != nil {
			return err
		}
	}
	return nil
}

// barabanStackFor adds the stack to one user's newest Альфа period that does
// not already have one. Idempotent per user: if the user has any such pair
// already, nothing happens — otherwise a re-run would add one more each time.
func (g *gen) barabanStackFor(ctx context.Context, uid uuid.UUID) error {
	const havePair = `
		select count(*)
		from category_offer r
		join category_offer s
		  on s.offer_period_id = r.offer_period_id
		 and s.kind = 'super'
		 and s.canonical_category_id = r.canonical_category_id
		join offer_period p on p.id = r.offer_period_id
		join bank_client c on c.id = p.bank_client_id
		where c.user_id = $1 and r.kind = 'regular'
		  and r.canonical_category_id is not null`
	var have int
	if err := g.pool.QueryRow(ctx, havePair, uid).Scan(&have); err != nil {
		return err
	}
	if have > 0 {
		return nil
	}

	// The newest Альфа period holding a SELECTED regular row with a canonical
	// category — the барабан has to repeat something actually picked.
	const pick = `
		select p.id, o.raw_title, o.canonical_category_id, p.period_start, p.period_end
		from category_offer o
		join selection sel on sel.category_offer_id = o.id
		join offer_period p on p.id = o.offer_period_id
		join bank_client c on c.id = p.bank_client_id
		join bank b on b.id = c.bank_id
		where c.user_id = $1 and b.name = 'Альфа-Банк'
		  and o.kind = 'regular' and o.canonical_category_id is not null
		order by p.period_start desc
		limit 1`
	var (
		periodID   int64
		title      string
		canonical  int64
		start, end time.Time
	)
	if err := g.pool.QueryRow(ctx, pick, uid).Scan(&periodID, &title, &canonical, &start, &end); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil // no Альфа wallet, or nothing picked in it
		}
		return err
	}

	pct := decimal.NewFromInt(9)
	notes := "барабан суперкэшбека — выпал на уже выбранную категорию, ставки складываются"
	offer, err := g.svc.CreateCategoryOffer(ctx, uid, periodID, title,
		&canonical, &pct, cashback.OfferSuper, &notes, nil, nil)
	if err != nil {
		return fmt.Errorf("барабан stack %q: %w", title, err)
	}
	g.counters.offers++
	if _, err := g.svc.CreateSelection(ctx, uid, offer.ID, midPeriod(start, end), false); err != nil {
		return fmt.Errorf("select барабан stack: %w", err)
	}
	g.counters.selections++
	log.Printf("барабан stack: %q now carries both a pick and the барабан in one Альфа period", title)
	return nil
}
