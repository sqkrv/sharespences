// Command devdata fills a development database with a realistic multi-period
// cashback history for one user: bank clients, cards, offer periods, menu
// rows drawn from each bank's seeded catalog, dated selections, and partner
// offers.
//
// It writes through internal/cashback.Service rather than raw SQL, so every
// domain invariant applies exactly as it would to a real user — slot limits,
// no overlapping periods per client, selections dated inside their period.
// Data that would be rejected in the app is rejected here too.
//
// Re-running is safe and is the intended workflow: generation is seeded by
// (user, bank client, period), so an existing period is detected and skipped
// while a newly-reachable one is filled. Run it again next month and only
// next month's periods appear.
//
//	DATABASE_URL=postgres://... go run ./cmd/devdata -user sqkrv -months 18
//
// This is a development tool. It is not part of the server binary (the
// Dockerfile builds ./cmd/sharespences only) and it must never be pointed at
// production: the rows it writes are indistinguishable from hand-entered
// ones and there is no undo beyond deleting them.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math/rand"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/sqkrv/sharespences/internal/cashback"
	"github.com/sqkrv/sharespences/internal/db"
)

// clientSpec is one bank_client to guarantee for the target user. Label is
// the держатель; empty means the user themselves, matching the app's own
// convention (a NULL label is «you»).
type clientSpec struct {
	bank  string
	label string
	tier  string
	cards []cardSpec
}

type cardSpec struct {
	last4  int32
	system string
}

// profile is the wallet to build: the owner's six banks plus two household
// members, so the «по держателям» grouping and the cross-client collision
// warnings both have something to show.
var profile = []clientSpec{
	{bank: "Альфа-Банк", tier: "Alfa Only", cards: []cardSpec{{4321, "mir"}, {8890, "visa"}}},
	{bank: "Альфа-Банк", label: "Мама", tier: "Стандартный", cards: []cardSpec{{1174, "mir"}}},
	{bank: "ВТБ", tier: "Привилегия", cards: []cardSpec{{2038, "mir"}}},
	{bank: "Ozon Банк", tier: "Ozon Premium", cards: []cardSpec{{7712, "mir"}, {9930, "mastercard"}}},
	{bank: "Ozon Банк", label: "Жена", tier: "Стандартный", cards: []cardSpec{{5501, "mir"}}},
	{bank: "Яндекс Пэй", tier: "Стандартный", cards: []cardSpec{{8868, "mir"}}},
	{bank: "Т-Банк", tier: "Premium", cards: []cardSpec{{3357, "mir"}}},
	{bank: "МКБ", tier: "Премиальный", cards: []cardSpec{{6604, "mir"}}},
}

// slotFallback supplies a slot count for programs whose tier leaves
// max_categories NULL. Without it those periods are unbounded and the
// generator would fill every menu row, which no real picker allows.
// Sources: Яндекс «Выберите 5 из 12», МКБ 3 slots (knowledge/banks).
var slotFallback = map[string]int32{
	"Яндекс Пэй":  5,
	"МКБ":         3,
	"Газпромбанк": 3,
}

// quarterly banks get one period per quarter instead of per month.
var quarterly = map[string]bool{"МКБ": true}

// basePurchaseTitles are the «всё подряд» rows: they are ordinary
// slot-consuming entries but pay 1–2%, never a headline rate.
var basePurchaseTitles = map[string]bool{
	"Все покупки": true, "За все покупки": true, "на все покупки": true,
	"Все остальные покупки": true,
}

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	var (
		username = flag.String("user", "", "username to fill (required)")
		months   = flag.Int("months", 18, "how many months of history to generate, counting back from -until")
		until    = flag.String("until", "", "last month to generate, YYYY-MM (default: current month)")
		seedVal  = flag.Int64("seed", 1, "RNG seed; the same seed reproduces the same data")
		confirm  = flag.Bool("confirm", false, "required when DATABASE_URL is not local")
		dryRun   = flag.Bool("dry-run", false, "report what would be written, write nothing")
	)
	flag.Parse()

	if *username == "" {
		return errors.New("-user is required")
	}
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	if !isLocalDSN(dsn) && !*confirm {
		return fmt.Errorf("DATABASE_URL points at a non-local host — this tool writes "+
			"data indistinguishable from real entries and has no undo; pass -confirm "+
			"to proceed anyway (host: %s)", dsnHost(dsn))
	}

	lastMonth := time.Now().UTC()
	if *until != "" {
		t, err := time.Parse("2006-01", *until)
		if err != nil {
			return fmt.Errorf("-until must be YYYY-MM: %w", err)
		}
		lastMonth = t
	}

	ctx := context.Background()
	pool, err := db.NewPool(ctx, dsn)
	if err != nil {
		return err
	}
	defer pool.Close()

	g := &gen{
		q:    db.New(pool),
		pool: pool,
		rng:  rand.New(rand.NewSource(*seedVal)),
		dry:  *dryRun,
	}
	g.svc = &cashback.Service{Q: g.q}

	if err := g.resolveUser(ctx, *username); err != nil {
		return err
	}
	if err := g.loadReference(ctx); err != nil {
		return err
	}
	log.Printf("target: user %s (%s) on %s", *username, g.userID, dsnHost(dsn))
	if g.dry {
		log.Printf("dry run — nothing will be written")
	}

	if err := g.ensureClients(ctx); err != nil {
		return err
	}
	if err := g.fillPeriods(ctx, lastMonth, *months); err != nil {
		return err
	}
	g.report()
	return nil
}

type gen struct {
	q    *db.Queries
	pool *pgxpool.Pool
	svc  *cashback.Service
	rng  *rand.Rand
	dry  bool

	userID uuid.UUID

	banks    map[string]int32              // name → bank id
	tiers    map[string]map[string]tierRef // bank → tier name → tier
	catalog  map[string][]db.ListBankCategoriesRow
	clients  map[string]int64 // "bank|label" → bank_client id
	tierOf   map[int64]tierRef
	bankOf   map[int64]string
	labelOf  map[int64]string
	counters struct{ clients, cards, periods, offers, selections, partners, skipped int }
}

type tierRef struct {
	id            int64
	maxCategories *int32
}

// resolveUser looks the user up by username. This is the one raw query in
// the tool: the app has no GetUserByUsername (it authenticates by email) and
// a dev tool should not grow the production query set.
func (g *gen) resolveUser(ctx context.Context, username string) error {
	row := g.pool.QueryRow(ctx, `select id from "user" where username = $1`, strings.ToLower(username))
	if err := row.Scan(&g.userID); err != nil {
		return fmt.Errorf("user %q not found: %w", username, err)
	}
	return nil
}

func (g *gen) loadReference(ctx context.Context) error {
	g.banks = map[string]int32{}
	g.tiers = map[string]map[string]tierRef{}
	g.catalog = map[string][]db.ListBankCategoriesRow{}
	g.clients = map[string]int64{}
	g.tierOf = map[int64]tierRef{}
	g.bankOf = map[int64]string{}
	g.labelOf = map[int64]string{}

	banks, err := g.q.ListBanks(ctx)
	if err != nil {
		return err
	}
	for _, b := range banks {
		g.banks[b.Name] = int32(b.ID)
	}

	programs, err := g.q.ListPrograms(ctx)
	if err != nil {
		return err
	}
	bankByID := map[int32]string{}
	for name, id := range g.banks {
		bankByID[id] = name
	}
	for _, p := range programs {
		tiers, err := g.q.ListTiersForProgram(ctx, p.ID)
		if err != nil {
			return err
		}
		name := bankByID[p.BankID]
		if g.tiers[name] == nil {
			g.tiers[name] = map[string]tierRef{}
		}
		for _, t := range tiers {
			g.tiers[name][t.Name] = tierRef{id: t.ID, maxCategories: t.MaxCategories}
		}
	}

	for name, id := range g.banks {
		rows, err := g.svc.ListBankCategories(ctx, g.userID, id)
		if err != nil {
			return err
		}
		g.catalog[name] = rows
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

func (g *gen) ensureClients(ctx context.Context) error {
	for _, spec := range profile {
		bankID, ok := g.banks[spec.bank]
		if !ok {
			log.Printf("skip %s: bank not seeded", spec.bank)
			continue
		}
		tier, ok := g.tiers[spec.bank][spec.tier]
		if !ok {
			return fmt.Errorf("tier %q not found for %s — run `sharespences seed` first", spec.tier, spec.bank)
		}
		key := spec.bank + "|" + spec.label
		if id, exists := g.clients[key]; exists {
			// Adopt the tier we planned so slot maths matches, but never
			// touch a client the user set up differently on purpose.
			if _, known := g.tierOf[id]; !known {
				g.tierOf[id] = tier
			}
			continue
		}
		if g.dry {
			// Register a placeholder so the period pass can still project
			// what this client would get; a dry run that reported «0 periods»
			// because it created no clients would be useless.
			g.counters.clients++
			g.counters.cards += len(spec.cards)
			id := int64(-len(g.clients) - 1)
			g.clients[key] = id
			g.tierOf[id] = tier
			g.bankOf[id] = spec.bank
			g.labelOf[id] = spec.label
			continue
		}
		var label *string
		if spec.label != "" {
			l := spec.label
			label = &l
		}
		c, err := g.q.CreateBankClient(ctx, db.CreateBankClientParams{
			UserID: g.userID, BankID: bankID, Label: label, ProgramTierID: &tier.id,
		})
		if err != nil {
			return fmt.Errorf("create client %s: %w", key, err)
		}
		g.clients[key] = c.ID
		g.tierOf[c.ID] = tier
		g.bankOf[c.ID] = spec.bank
		g.labelOf[c.ID] = spec.label
		g.counters.clients++

		for _, cd := range spec.cards {
			if _, err := g.q.CreateCard(ctx, db.CreateCardParams{
				BankClientID: c.ID, UserID: g.userID,
				Last4Digits: cd.last4, PaymentSystem: db.PaymentSystem(cd.system),
			}); err != nil {
				return fmt.Errorf("create card %d: %w", cd.last4, err)
			}
			g.counters.cards++
		}
	}
	return nil
}

// fillPeriods walks backwards from the newest month so a re-run reaches the
// new month first and stops doing anything interesting once it hits periods
// that already exist.
func (g *gen) fillPeriods(ctx context.Context, last time.Time, months int) error {
	for _, spec := range profile {
		clientID, ok := g.clients[spec.bank+"|"+spec.label]
		if !ok {
			continue
		}
		for i := 0; i < months; i++ {
			month := time.Date(last.Year(), last.Month(), 1, 0, 0, 0, 0, time.UTC).AddDate(0, -i, 0)
			start, end := periodBounds(spec.bank, month)
			if quarterly[spec.bank] && month.Month() != quarterStart(month.Month()) {
				continue // one period per quarter, generated on its first month
			}
			if err := g.fillOnePeriod(ctx, clientID, spec.bank, start, end); err != nil {
				return err
			}
		}
	}
	return nil
}

func (g *gen) fillOnePeriod(ctx context.Context, clientID int64, bank string, start, end time.Time) error {
	if g.dry {
		g.counters.periods++
		return nil
	}
	period, err := g.svc.CreateOfferPeriod(ctx, g.userID, clientID, start, end, nil)
	if err != nil {
		if errors.Is(err, cashback.ErrPeriodOverlap) {
			g.counters.skipped++
			return nil // already generated (or hand-entered) — leave it alone
		}
		return fmt.Errorf("period %s %s: %w", bank, start.Format("2006-01"), err)
	}
	g.counters.periods++

	slots := g.slotsFor(clientID, bank)
	if g.tierOf[clientID].maxCategories == nil {
		if _, err := g.svc.SetPeriodMaxOverride(ctx, g.userID, period.ID, &slots); err != nil {
			return fmt.Errorf("slots %s: %w", bank, err)
		}
	}

	rows := g.pickMenu(bank, start)
	var regular []int64
	for _, r := range rows {
		kind := cashback.OfferKind(r.Kind)
		pct := g.percentFor(r, bank)
		bcID := r.ID
		offer, err := g.svc.CreateCategoryOffer(ctx, g.userID, period.ID, r.Title,
			r.CanonicalCategoryID, &pct, kind, nil, &bcID, g.capFor(bank, r))
		if err != nil {
			return fmt.Errorf("offer %q: %w", r.Title, err)
		}
		g.counters.offers++
		if kind == cashback.OfferRegular {
			regular = append(regular, offer.ID)
		}
	}

	// Альфа's барабан is granted, not picked: it consumes no slot and is
	// always active, so it is recorded as kind=super and always selected.
	if bank == "Альфа-Банк" && g.rng.Intn(100) < 70 {
		pct := decimal.NewFromInt(int64(3 + g.rng.Intn(8)))
		title := barabanTitles[g.rng.Intn(len(barabanTitles))]
		offer, err := g.svc.CreateCategoryOffer(ctx, g.userID, period.ID, title,
			nil, &pct, cashback.OfferSuper, strPtr("барабан суперкэшбека"), nil, nil)
		if err != nil {
			return fmt.Errorf("баран %q: %w", title, err)
		}
		g.counters.offers++
		if _, err := g.svc.CreateSelection(ctx, g.userID, offer.ID, midPeriod(start, end), false); err != nil {
			return fmt.Errorf("select баран: %w", err)
		}
		g.counters.selections++
	}

	// Fill slots, but not always to the brim — a real month sometimes has an
	// unused slot, which is exactly what CB-01's «+ слот» affordance is for.
	want := int(slots)
	if g.rng.Intn(100) < 25 && want > 1 {
		want--
	}
	g.rng.Shuffle(len(regular), func(i, j int) { regular[i], regular[j] = regular[j], regular[i] })
	for i, offerID := range regular {
		if i >= want {
			break
		}
		at := start.AddDate(0, 0, g.rng.Intn(3))
		if _, err := g.svc.CreateSelection(ctx, g.userID, offerID, at, false); err != nil {
			if errors.Is(err, cashback.ErrSlotsExhausted) {
				break
			}
			return fmt.Errorf("select: %w", err)
		}
		g.counters.selections++
	}

	// A partner offer every few periods, per bank. These are record-only —
	// no slot, no invariants — so unlike everything else here they go
	// straight through the query, exactly as the HTTP handler does.
	if g.rng.Intn(100) < 35 {
		m := partnerMerchants[g.rng.Intn(len(partnerMerchants))]
		pct := decimal.NewFromInt(int64(5 + g.rng.Intn(26)))
		from, to := start, end
		if _, err := g.q.CreatePartnerOffer(ctx, db.CreatePartnerOfferParams{
			UserID: g.userID, BankID: g.banks[bank], BankClientID: &clientID,
			MerchantTitle: m, Percent: &pct, ValidFrom: &from, ValidTo: &to,
		}); err != nil {
			return fmt.Errorf("partner %q: %w", m, err)
		}
		g.counters.partners++
	}
	return nil
}

// slotsFor is the effective slot count: the tier's, else the per-program
// fallback, else a sane default.
func (g *gen) slotsFor(clientID int64, bank string) int32 {
	if t, ok := g.tierOf[clientID]; ok && t.maxCategories != nil {
		return *t.maxCategories
	}
	if n, ok := slotFallback[bank]; ok {
		return n
	}
	return 3
}

// pickMenu draws a plausible month's menu from the bank's seeded catalog:
// the base «все покупки» row when it exists, plus a rotating sample. The
// sample is seeded by month so consecutive months differ but a re-run of the
// same month would produce the same menu.
func (g *gen) pickMenu(bank string, month time.Time) []db.ListBankCategoriesRow {
	all := g.catalog[bank]
	if len(all) == 0 {
		return nil
	}
	local := rand.New(rand.NewSource(int64(month.Year()*100+int(month.Month())) ^ int64(len(bank))))

	var base, rest []db.ListBankCategoriesRow
	for _, r := range all {
		if !r.Active {
			continue
		}
		if basePurchaseTitles[r.Title] {
			base = append(base, r)
		} else {
			rest = append(rest, r)
		}
	}
	local.Shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })

	size := 8 + local.Intn(7) // 8–14 rows, the observed picker range
	if size > len(rest) {
		size = len(rest)
	}
	out := append([]db.ListBankCategoriesRow{}, base...)
	return append(out, rest[:size]...)
}

// percentFor keeps the numbers in the bands the corpus actually shows:
// base rows 1–2%, merchant/service rows are the headline rates, ordinary
// categories in between.
func (g *gen) percentFor(r db.ListBankCategoriesRow, bank string) decimal.Decimal {
	switch {
	case basePurchaseTitles[r.Title]:
		if g.rng.Intn(4) == 0 {
			return decimal.RequireFromString("1.5")
		}
		return decimal.NewFromInt(int64(1 + g.rng.Intn(2)))
	case r.CanonicalCategoryID == nil: // service / merchant row
		return decimal.NewFromInt(int64(10 + g.rng.Intn(31)))
	case bank == "Т-Банк" && strings.Contains(r.Title, "в Городе"):
		return decimal.NewFromInt(int64(10 + g.rng.Intn(21)))
	default:
		return decimal.NewFromInt(int64(3 + g.rng.Intn(8)))
	}
}

// capFor gives ВТБ rows their own «Кешбэк до N ₽» chip, which is the bank
// this behaviour was modelled from (migration 00011).
func (g *gen) capFor(bank string, r db.ListBankCategoriesRow) *decimal.Decimal {
	if bank != "ВТБ" || g.rng.Intn(100) >= 40 {
		return nil
	}
	v := decimal.NewFromInt(int64([]int{500, 1000, 2000, 3000, 5000}[g.rng.Intn(5)]))
	return &v
}

func (g *gen) report() {
	c := g.counters
	log.Printf("bank clients +%d, cards +%d", c.clients, c.cards)
	log.Printf("periods +%d (%d already existed, skipped)", c.periods, c.skipped)
	log.Printf("menu rows +%d, selections +%d, partner offers +%d", c.offers, c.selections, c.partners)
}

var barabanTitles = []string{"Такси", "Кафе и рестораны", "Продукты", "Транспорт", "Аптеки", "Дом и ремонт"}

var partnerMerchants = []string{
	"Читай-город", "ВкусВилл", "Zolla", "Лабиринт", "Спортмастер",
	"Много Лосося", "Golden Apple", "Магнит Косметик", "Детский мир",
}

// periodBounds returns the period a bank uses for the given month: a
// calendar month, or the containing quarter for quarterly programs (МКБ).
func periodBounds(bank string, month time.Time) (time.Time, time.Time) {
	if quarterly[bank] {
		qs := time.Date(month.Year(), quarterStart(month.Month()), 1, 0, 0, 0, 0, time.UTC)
		return qs, qs.AddDate(0, 3, -1)
	}
	start := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, time.UTC)
	return start, start.AddDate(0, 1, -1)
}

func quarterStart(m time.Month) time.Month {
	return time.Month(((int(m)-1)/3)*3 + 1)
}

func midPeriod(start, end time.Time) time.Time {
	return start.Add(end.Sub(start) / 2)
}

func strPtr(s string) *string { return &s }

// isLocalDSN is the production guard. It is deliberately conservative: any
// host that is not obviously a loopback address requires -confirm.
func isLocalDSN(dsn string) bool {
	h := dsnHost(dsn)
	return h == "localhost" || h == "127.0.0.1" || h == "::1" || h == "db" || h == ""
}

func dsnHost(dsn string) string {
	s := dsn
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.LastIndex(s, "@"); i >= 0 {
		s = s[i+1:]
	}
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	if i := strings.LastIndex(s, ":"); i >= 0 {
		s = s[:i]
	}
	return s
}
