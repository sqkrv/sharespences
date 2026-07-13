// Package seed loads reference data derived from the knowledge base
// (docs/knowledge in the private meta-repo): the owner's six banks with
// their КБ programs and tiers, ~55 canonical categories, and the known
// bank-title aliases (five banks, from concepts/categories-taxonomy.md,
// 2026-07-14). Program/tier numbers are as of 2025-05 (wiki table) — notes
// on every program/tier say so; re-verify against live bank apps before
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
	// Additions from the categories taxonomy (knowledge:
	// concepts/categories-taxonomy.md, five banks synthesized 2026-07-14).
	{"avia-tickets", "Авиабилеты"},
	{"rail-tickets", "Ж/д билеты"},
	{"hotels", "Отели"},
	{"travel-agencies", "Турагентства"},
	{"duty-free", "Duty Free"},
	{"auto-parts", "Автозапчасти"},
	{"auto-services", "Автоуслуги"},
	{"car-purchase", "Покупка авто"},
	{"toll-roads", "Платные дороги"},
	{"cosmetics", "Косметика и парфюмерия"},
	{"accessories", "Аксессуары"},
	{"online-cinema", "Онлайн-кинотеатры"},
	{"culture", "Культура и искусство"},
	{"cinema", "Кино и театры"},
	{"music", "Музыка"},
	{"active-leisure", "Активный отдых и фитнес"},
	{"health", "Здоровье"},
	{"health-goods", "Товары для здоровья"},
	{"alcohol", "Алкоголь"},
	{"hobby", "Хобби"},
	{"insurance", "Страхование"},
	{"household-services", "Бытовые услуги"},
	{"photo-video", "Фото и видео"},
	{"souvenirs", "Сувениры"},
	{"fines", "Штрафы"},
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
	// Additions from the categories taxonomy (2026-07-14) — documented raw
	// titles only; service/partner rows stay alias-less by design.
	{"Альфа-Банк", "Продукты (Супермаркеты)", "supermarkets"},
	{"Альфа-Банк", "Фастфуд, кафе и рестораны", "restaurants"},
	{"Альфа-Банк", "За все покупки", "all-purchases"},
	{"Альфа-Банк", "Маркетплейсы", "marketplaces"},
	{"Альфа-Банк", "Медицинские услуги", "medicine"},
	{"Альфа-Банк", "Детские товары", "kids"},
	{"Альфа-Банк", "Ювелирные изделия", "jewelry"},
	{"Альфа-Банк", "Автозапчасти", "auto-parts"},
	{"Альфа-Банк", "Автоуслуги", "auto-services"},
	{"Альфа-Банк", "Покупка авто", "car-purchase"},
	{"Альфа-Банк", "Аксессуары", "accessories"},
	{"Альфа-Банк", "Активный отдых", "active-leisure"},
	{"Альфа-Банк", "Алкоголь", "alcohol"},
	{"Альфа-Банк", "Хобби", "hobby"},
	{"Альфа-Банк", "Культура и искусство", "culture"},
	{"Альфа-Банк", "Здоровье", "health"},
	{"Альфа-Банк", "Товары для здоровья", "health-goods"},
	{"Альфа-Банк", "Штрафы ГАИ", "fines"},
	{"Озон Банк", "Супермаркеты", "supermarkets"},
	{"Озон Банк", "Рестораны", "restaurants"},
	{"Озон Банк", "Топливо и АЗС", "gas-stations"},
	{"Озон Банк", "Топливо и автомобильные заправочные станции", "gas-stations"},
	{"Озон Банк", "Такси", "taxi"},
	{"Озон Банк", "Транспорт", "transport"},
	{"Озон Банк", "Каршеринг", "car-rental"},
	{"Озон Банк", "Аренда автомобилей", "car-rental"},
	{"Озон Банк", "Автоуслуги", "auto-services"},
	{"Озон Банк", "Автомобильные услуги", "auto-services"},
	{"Озон Банк", "Авиабилеты", "avia-tickets"},
	{"Озон Банк", "ЖД билеты", "rail-tickets"},
	{"Озон Банк", "Железнодорожные билеты", "rail-tickets"},
	{"Озон Банк", "Отели", "hotels"},
	{"Озон Банк", "Турагентства", "travel-agencies"},
	{"Озон Банк", "Магазины беспошлинной торговли - Duty Free", "duty-free"},
	{"Озон Банк", "VIP-залы", "travel"},
	{"Озон Банк", "Круизы", "travel"},
	{"Озон Банк", "Салоны красоты и СПА", "beauty"},
	{"Озон Банк", "Салоны красоты и SPA", "beauty"},
	{"Озон Банк", "Косметика", "cosmetics"},
	{"Озон Банк", "Косметика и парфюмерия", "cosmetics"},
	{"Озон Банк", "Дом и ремонт", "home-repair"},
	{"Озон Банк", "Дом, ремонт", "home-repair"},
	{"Озон Банк", "Ветклиники и зоомагазины", "pets"},
	{"Озон Банк", "Зоотовары", "pets"},
	{"Озон Банк", "Ветеринарные клиники", "pets"},
	{"Озон Банк", "Искусство", "culture"},
	{"Озон Банк", "Выставки и музеи", "culture"},
	{"Озон Банк", "Книги", "books"},
	{"Озон Банк", "Кино", "cinema"},
	{"Озон Банк", "Развлечения", "entertainment"},
	{"Озон Банк", "Всё для геймеров", "entertainment"},
	{"Озон Банк", "Фитнес", "active-leisure"},
	{"Озон Банк", "Медицинские клиники", "medicine"},
	{"Озон Банк", "Медицинские клиники и эстетическая медицина", "medicine"},
	{"Озон Банк", "Медицинские услуги", "medicine"},
	{"Озон Банк", "Стоматология", "medicine"},
	{"Озон Банк", "Музыка", "music"},
	{"Озон Банк", "Музыкальные инструменты", "music"},
	{"Озон Банк", "Электроника и бытовая техника", "electronics"},
	{"Озон Банк", "Связь и Интернет", "telecom"},
	{"Озон Банк", "Химчистки", "household-services"},
	{"Озон Банк", "Фото и видео", "photo-video"},
	{"Озон Банк", "Фото/Видео", "photo-video"},
	{"Озон Банк", "Сувениры", "souvenirs"},
	{"Озон Банк", "Цветы", "flowers"},
	{"Озон Банк", "Спорттовары", "sport-goods"},
	{"Озон Банк", "Одежда и обувь", "clothes"},
	{"Озон Банк", "Одежда, обувь", "clothes"},
	{"Озон Банк", "Товары для детей", "kids"},
	{"Озон Банк", "Ювелирные изделия", "jewelry"},
	{"Озон Банк", "ЖКХ", "utilities"},
	{"Озон Банк", "Образование", "education"},
	{"Озон Банк", "На все покупки", "all-purchases"},
	{"Озон Банк", "Стандартный кешбэк 1%", "all-purchases"},
	{"ВТБ", "Супермаркеты", "supermarkets"},
	{"ВТБ", "Аптеки", "pharmacies"},
	{"ВТБ", "Здоровье", "health"},
	{"ВТБ", "Кафе и рестораны", "restaurants"},
	{"ВТБ", "Одежда и обувь", "clothes"},
	{"ВТБ", "Детские товары", "kids"},
	{"ВТБ", "Транспорт", "transport"},
	{"ВТБ", "Такси", "taxi"},
	{"ВТБ", "Аренда авто", "car-rental"},
	{"ВТБ", "Красота", "beauty"},
	{"ВТБ", "Электроника", "electronics"},
	{"ВТБ", "Авиабилеты", "avia-tickets"},
	{"ВТБ", "Duty Free", "duty-free"},
	{"ВТБ", "Отели", "hotels"},
	{"ВТБ", "Ж/д билеты", "rail-tickets"},
	{"ВТБ", "Турагентства", "travel-agencies"},
	{"ВТБ", "АЗС", "gas-stations"},
	{"ВТБ", "Автоуслуги", "auto-services"},
	{"ВТБ", "Платные дороги", "toll-roads"},
	{"ВТБ", "Дом и ремонт", "home-repair"},
	{"ВТБ", "Образование", "education"},
	{"ВТБ", "Фитнес", "active-leisure"},
	{"ВТБ", "Спортивные товары", "sport-goods"},
	{"ВТБ", "Страхование", "insurance"},
	{"ВТБ", "Бытовые услуги", "household-services"},
	{"ВТБ", "Цветы", "flowers"},
	{"ВТБ", "Украшения и бижутерия", "jewelry"},
	{"ВТБ", "Зоотовары", "pets"},
	{"ВТБ", "Цифровой контент", "digital-goods"},
	{"ВТБ", "Развлечения", "entertainment"},
	{"ВТБ", "Театры и кино", "cinema"},
	{"ВТБ", "Услуги ЖКХ", "utilities"},
	{"ВТБ", "Алкоголь", "alcohol"},
	{"ВТБ", "Искусство", "culture"},
	{"ВТБ", "Книги и канцтовары", "books"},
	{"ВТБ", "Маркетплейсы", "marketplaces"},
	{"ВТБ", "Продажа авто", "car-purchase"},
	{"ВТБ", "Услуги связи", "telecom"},
	{"ВТБ", "Все покупки", "all-purchases"},
	{"ВТБ", "Все остальные покупки", "all-purchases"},
	{"Т-Банк", "Автоуслуги", "auto-services"},
	{"Т-Банк", "Заправки", "gas-stations"},
	{"Т-Банк", "Платные дороги", "toll-roads"},
	{"Т-Банк", "Животные", "pets"},
	{"Т-Банк", "Ремонт и мебель", "home-repair"},
	{"Т-Банк", "Кино", "cinema"},
	{"Т-Банк", "Онлайн-кинотеатры", "online-cinema"},
	{"Т-Банк", "Развлечения", "entertainment"},
	{"Т-Банк", "Цифровые товары", "digital-goods"},
	{"Т-Банк", "Косметика", "cosmetics"},
	{"Т-Банк", "Красота", "beauty"},
	{"Т-Банк", "Аптеки", "pharmacies"},
	{"Т-Банк", "Рестораны", "restaurants"},
	{"Т-Банк", "Супермаркеты", "supermarkets"},
	{"Т-Банк", "Фастфуд", "fastfood"},
	{"Т-Банк", "Искусство", "culture"},
	{"Т-Банк", "Музыка", "music"},
	{"Т-Банк", "Образование", "education"},
	{"Т-Банк", "Книги и канцтовары", "books"},
	{"Т-Банк", "Авиабилеты", "avia-tickets"},
	{"Т-Банк", "Ж/д билеты", "rail-tickets"},
	{"Т-Банк", "Duty Free", "duty-free"},
	{"Т-Банк", "Спорттовары", "sport-goods"},
	{"Т-Банк", "Тренировки", "active-leisure"},
	{"Т-Банк", "Каршеринг", "car-rental"},
	{"Т-Банк", "Местный транспорт", "transport"},
	{"Т-Банк", "Самокаты", "transport"},
	{"Т-Банк", "Такси", "taxi"},
	{"Т-Банк", "Гаджеты и техника", "electronics"},
	{"Т-Банк", "Детские товары", "kids"},
	{"Т-Банк", "Маркетплейсы", "marketplaces"},
	{"Т-Банк", "Одежда и обувь", "clothes"},
	{"Т-Банк", "Подарки и творчество", "hobby"},
	{"Т-Банк", "Цветы", "flowers"},
	{"Яндекс Пэй", "Кафе, рестораны и бары", "restaurants"},
	{"Яндекс Пэй", "Образование", "education"},
	{"Яндекс Пэй", "Одежда и обувь", "clothes"},
	{"Яндекс Пэй", "Развлечения", "entertainment"},
	{"Яндекс Пэй", "Городской транспорт", "transport"},
	{"Яндекс Пэй", "Цветы", "flowers"},
	{"Яндекс Пэй", "Аптеки", "pharmacies"},
	{"Яндекс Пэй", "Товары для дома", "home-repair"},
	{"Яндекс Пэй", "Красота", "beauty"},
	{"Яндекс Пэй", "Спорт и фитнес", "active-leisure"},
	{"Яндекс Пэй", "Медицина", "medicine"},
	{"Яндекс Пэй", "Супермаркеты", "supermarkets"},
	{"Яндекс Пэй", "Кино", "cinema"},
	{"Яндекс Пэй", "Автоуслуги", "auto-services"},
	{"Яндекс Пэй", "Онлайн-кинотеатры", "online-cinema"},
	{"Яндекс Пэй", "Питомцы", "pets"},
	{"Яндекс Пэй", "Книги", "books"},
	{"Яндекс Пэй", "Электроника и бытовая техника", "electronics"},
	{"Яндекс Пэй", "Ювелирные изделия", "jewelry"},
	{"Яндекс Пэй", "Платные дороги и парковки", "toll-roads"},
	{"Яндекс Пэй", "Товары для детей", "kids"},
	{"Яндекс Пэй", "АЗС", "gas-stations"},
	{"Яндекс Пэй", "На всё", "all-purchases"},
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
