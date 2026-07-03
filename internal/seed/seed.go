// Package seed loads reference data derived from the knowledge base
// (docs/knowledge in the private meta-repo): the owner's six banks with
// their КБ programs and tiers, ~30 canonical categories, and the known
// bank-title aliases. Numbers are as of 2025-05 (wiki table) — notes on
// every program/tier say so; re-verify against live bank apps before
// relying on them for real decisions.
//
// Idempotent: safe to run repeatedly (natural-key upserts).
package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const asOf = "as of 2025-05 (wiki table); re-verify in the bank app"

type tier struct {
	name           string
	paid           bool
	capValue       string // "" = unknown
	capScope       string
	capPerCategory string
	maxCategories  int // 0 = unknown
	basePercent    string
	notes          string
}

type program struct {
	bank          string
	name          string
	periodType    string
	selectionMode string
	currencyKind  string
	pointsLabel   string
	opensDay      int // 0 = unknown
	notes         string
	tiers         []tier
}

var programs = []program{
	{
		bank: "Альфа-Банк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub", opensDay: 25, notes: asOf,
		tiers: []tier{
			{name: "Стандартный", capValue: "5000", capScope: "total", maxCategories: 3, basePercent: "5", notes: asOf},
			{name: "Альфа-Смарт", paid: true, capValue: "7000", capScope: "total", maxCategories: 4, basePercent: "5", notes: asOf},
			{name: "Alfa Only", paid: true, capValue: "15000", capScope: "total", maxCategories: 5, basePercent: "7", notes: asOf},
		},
	},
	{
		bank: "ВТБ", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub", opensDay: 26, notes: asOf + "; дополнительные категории за хранение остатков — record-only",
		tiers: []tier{
			{name: "Стандартный", capValue: "3000", capScope: "total", maxCategories: 3, notes: asOf + "; % варьируется до 15%"},
			{name: "Привилегия", capValue: "30000", capScope: "total", maxCategories: 3, notes: asOf},
		},
	},
	{
		bank: "Озон Банк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub", opensDay: 25, notes: asOf,
		tiers: []tier{
			{name: "Стандартный", capValue: "3000", capScope: "both", capPerCategory: "1500", maxCategories: 4, notes: asOf},
			{name: "Ozon Premium", paid: true, capValue: "3000", capScope: "both", capPerCategory: "1500", maxCategories: 4,
				notes: asOf + "; подписка добавляет выбираемые категории (Кафе и Рестораны 5%, Фастфуд 5%)"},
		},
	},
	{
		bank: "Яндекс Пэй", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "incremental",
		currencyKind: "points", pointsLabel: "Баллы Плюс",
		notes: asOf + "; баллы требуют активной подписки Яндекс Плюс; колесо фортуны — record-only",
		tiers: []tier{
			{name: "Стандартный", capValue: "10000", capScope: "total", notes: asOf + "; max категорий неизвестен"},
		},
	},
	{
		bank: "Газпромбанк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub",
		notes:        "факты не собраны (knowledge stub, 2026-07); period/mode/currency — предположения",
		tiers: []tier{
			{name: "Стандартный", capScope: "total", notes: "лимиты неизвестны (null = unknown)"},
		},
	},
	{
		bank: "МКБ", name: "Кэшбэк", periodType: "quarter", selectionMode: "atomic",
		currencyKind: "points", pointsLabel: "баллы МКБ",
		notes: asOf + "; баллы 1:1 в рубли с месячными лимитами перевода; платная смена категории посреди квартала, активация на следующий день",
		tiers: []tier{
			{name: "Стандарт", capValue: "1500", capScope: "total", notes: asOf},
			{name: "Выгодный", capValue: "3000", capScope: "total", basePercent: "5", notes: asOf},
			{name: "Премиальный", capValue: "20000", capScope: "total", basePercent: "7", notes: asOf},
			{name: "Эксклюзивный", capValue: "50000", capScope: "total", basePercent: "7", notes: asOf},
		},
	},
}

// Reference-only banks (wiki): no programs seeded.
var extraBanks = []string{"Сбербанк", "Т-Банк"}

var canonicalCategories = [][2]string{
	{"supermarkets", "Супермаркеты"},
	{"restaurants", "Кафе и рестораны"},
	{"fastfood", "Фастфуд"},
	{"gas-stations", "АЗС"},
	{"pharmacies", "Аптеки"},
	{"taxi", "Такси"},
	{"transport", "Транспорт"},
	{"travel", "Путешествия"},
	{"auto", "Авто"},
	{"car-rental", "Аренда авто"},
	{"home-repair", "Дом и ремонт"},
	{"pets", "Животные"},
	{"books", "Книги"},
	{"utilities", "Коммунальные услуги"},
	{"beauty", "Красота"},
	{"education", "Образование"},
	{"clothes", "Одежда и обувь"},
	{"subscriptions", "Подписки"},
	{"entertainment", "Развлечения"},
	{"telecom", "Связь, интернет и ТВ"},
	{"sport-goods", "Спортивные товары"},
	{"electronics", "Техника"},
	{"flowers", "Цветы"},
	{"digital-goods", "Цифровые товары"},
	{"kids", "Детские товары"},
	{"jewelry", "Ювелирные изделия"},
	{"marketplaces", "Маркетплейсы"},
	{"medicine", "Медицина"},
	{"charity", "Благотворительность"},
	{"all-purchases", "Все покупки"},
}

// Known raw-title aliases. Альфа-Банк titles come from the captured category
// menus (categories.csv); Озон from the wiki/official docs. Other banks'
// exact raw titles are not yet known — the alias table grows inline (S1).
var aliases = []struct{ bank, raw, slug string }{
	{"Альфа-Банк", "Продукты", "supermarkets"},
	{"Альфа-Банк", "Супермаркеты", "supermarkets"},
	{"Альфа-Банк", "АЗС", "gas-stations"},
	{"Альфа-Банк", "Аптеки", "pharmacies"},
	{"Альфа-Банк", "Кафе и рестораны", "restaurants"},
	{"Альфа-Банк", "Рестораны", "restaurants"},
	{"Альфа-Банк", "Фастфуд", "fastfood"},
	{"Альфа-Банк", "Фастфуд, кафе, рестораны", "restaurants"},
	{"Альфа-Банк", "Такси", "taxi"},
	{"Альфа-Банк", "Транспорт", "transport"},
	{"Альфа-Банк", "Путешествия", "travel"},
	{"Альфа-Банк", "Авто", "auto"},
	{"Альфа-Банк", "Аренда авто", "car-rental"},
	{"Альфа-Банк", "Дом и ремонт", "home-repair"},
	{"Альфа-Банк", "Животные", "pets"},
	{"Альфа-Банк", "Книги", "books"},
	{"Альфа-Банк", "Коммунальные услуги", "utilities"},
	{"Альфа-Банк", "Красота", "beauty"},
	{"Альфа-Банк", "Образование", "education"},
	{"Альфа-Банк", "Одежда и обувь", "clothes"},
	{"Альфа-Банк", "Подписки", "subscriptions"},
	{"Альфа-Банк", "Развлечения", "entertainment"},
	{"Альфа-Банк", "Связь, интернет и ТВ", "telecom"},
	{"Альфа-Банк", "Спортивные товары", "sport-goods"},
	{"Альфа-Банк", "Техника", "electronics"},
	{"Альфа-Банк", "Цветы", "flowers"},
	{"Альфа-Банк", "Цифровые товары", "digital-goods"},
	{"Озон Банк", "Продукты", "supermarkets"},
	{"Озон Банк", "Аптеки", "pharmacies"},
	{"Озон Банк", "Кафе и Рестораны", "restaurants"},
	{"Озон Банк", "Фастфуд", "fastfood"},
}

// Run loads all seed data.
func Run(ctx context.Context, pool *pgxpool.Pool) error {
	for _, p := range programs {
		if err := seedBank(ctx, pool, p.bank); err != nil {
			return err
		}
	}
	for _, b := range extraBanks {
		if err := seedBank(ctx, pool, b); err != nil {
			return err
		}
	}

	for _, p := range programs {
		if _, err := pool.Exec(ctx, `
			insert into cashback_program (bank_id, name, period_type, selection_mode, currency_kind,
			                              points_label, selection_opens_day, notes)
			select b.id, $2, $3::cashback_period_type, $4::cashback_selection_mode,
			       $5::cashback_currency_kind, nullif($6, ''), nullif($7, 0), nullif($8, '')
			from bank b
			where b.name = $1
			  and not exists (select 1
			                  from cashback_program cp
			                  where cp.bank_id = b.id and cp.name = $2)`,
			p.bank, p.name, p.periodType, p.selectionMode, p.currencyKind,
			p.pointsLabel, p.opensDay, p.notes); err != nil {
			return fmt.Errorf("seed program %s: %w", p.bank, err)
		}
		for _, t := range p.tiers {
			if _, err := pool.Exec(ctx, `
				insert into program_tier (program_id, name, is_paid_subscription, cap_value, cap_scope,
				                          cap_per_category, max_categories, base_percent, notes)
				select cp.id, $3, $4, nullif($5, '')::numeric, $6::cashback_cap_scope,
				       nullif($7, '')::numeric, nullif($8, 0), nullif($9, '')::numeric, nullif($10, '')
				from cashback_program cp
				         join bank b on b.id = cp.bank_id
				where b.name = $1
				  and cp.name = $2
				  and not exists (select 1
				                  from program_tier pt
				                  where pt.program_id = cp.id and pt.name = $3)`,
				p.bank, p.name, t.name, t.paid, t.capValue, t.capScope,
				t.capPerCategory, t.maxCategories, t.basePercent, t.notes); err != nil {
				return fmt.Errorf("seed tier %s/%s: %w", p.bank, t.name, err)
			}
		}
	}

	for _, c := range canonicalCategories {
		if _, err := pool.Exec(ctx, `
			insert into canonical_category (slug, title_ru)
			values ($1, $2)
			on conflict (slug) do nothing`, c[0], c[1]); err != nil {
			return fmt.Errorf("seed category %s: %w", c[0], err)
		}
	}

	for _, a := range aliases {
		if _, err := pool.Exec(ctx, `
			insert into bank_category_alias (canonical_category_id, bank_id, raw_title)
			select cc.id, b.id, $3
			from canonical_category cc,
			     bank b
			where cc.slug = $1
			  and b.name = $2
			on conflict (bank_id, raw_title) do nothing`, a.slug, a.bank, a.raw); err != nil {
			return fmt.Errorf("seed alias %s/%s: %w", a.bank, a.raw, err)
		}
	}
	return nil
}

func seedBank(ctx context.Context, pool *pgxpool.Pool, name string) error {
	_, err := pool.Exec(ctx, `
		insert into bank (name)
		select $1
		where not exists (select 1 from bank where name = $1)`, name)
	if err != nil {
		return fmt.Errorf("seed bank %s: %w", name, err)
	}
	return nil
}
