// Package seed loads reference data derived from the knowledge base
// (docs/knowledge in the private meta-repo): the six banks with
// their КБ programs and tiers, ~55 canonical categories (with UI emoji),
// the known bank-title aliases, the per-bank picker catalogs
// (bank_category), and bank brand colors (five banks + catalogs/emoji/
// colors from concepts/categories-taxonomy.md and the bank pages,
// 2026-07-16). Program/tier numbers are as of 2025-05 (wiki table) — notes
// on every program/tier say so; re-verify against live bank apps before
// relying on them for real decisions.
//
// Idempotent: safe to run repeatedly (natural-key upserts). Knowledge-
// derived reference facts (program policy, emoji, brand colors, seeded
// bank_category rows) are refreshed on existing rows; user-created custom
// bank_category rows are never touched — a custom row with the same
// (bank, title) always wins over the seed.
package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const asOf = "as of 2025-05 (wiki table); re-verify in the bank app"

// Т-Банк's programme terms are newer than the wiki table and come from a different
// kind of source: the caps are stated terms, the slot count is an
// observation from the menu screenshots rather than the programme document.
const tbankAsOf = "as of 2026-07-31; 4 слота — по корпусу скриншотов меню (2025-06…2026-07), не из условий программы"

type tier struct {
	name           string
	paid           bool
	capValue       string // "" = unknown
	capScope       string
	capPerCategory string
	maxCategories  int // 0 = unknown
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
	// midPeriodAdd: can a category be ADDED mid-period? (allowed |
	// locked_after_first | paid | unknown). NOT derivable from
	// selectionMode — Альфа is atomic yet allows adds (2026-07-16).
	midPeriodAdd string
	// activation: when a fresh pick starts paying (immediate | next_day |
	// unknown). МКБ activates next day — «выбери перед покупкой» advice
	// would be wrong there.
	activation string
	notes      string
	tiers      []tier
}

var programs = []program{
	{
		bank: "Альфа-Банк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub", opensDay: 25,
		midPeriodAdd: "allowed", activation: "immediate", // 2026-07-16: add while a slot is free
		notes: asOf,
		tiers: []tier{
			{name: "Стандартный", capValue: "5000", capScope: "total", maxCategories: 3, notes: asOf},
			{name: "Альфа-Смарт", paid: true, capValue: "7000", capScope: "total", maxCategories: 4, notes: asOf},
			{name: "Alfa Only", paid: true, capValue: "15000", capScope: "total", maxCategories: 5, notes: asOf},
		},
	},
	{
		bank: "ВТБ", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub", opensDay: 26,
		midPeriodAdd: "locked_after_first", activation: "immediate", // one-shot (п. 3.5); +5 мин ≈ immediate
		notes: asOf + "; дополнительные категории за хранение остатков — record-only",
		tiers: []tier{
			{name: "Стандартный", capValue: "3000", capScope: "total", maxCategories: 3, notes: asOf + "; % варьируется до 15%"},
			{name: "Привилегия", capValue: "30000", capScope: "total", maxCategories: 3, notes: asOf},
		},
	},
	{
		bank: "Ozon Банк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub", opensDay: 25,
		midPeriodAdd: "locked_after_first", activation: "immediate", // «единожды» per the loyalty rules
		notes: asOf,
		tiers: []tier{
			{name: "Стандартный", capValue: "3000", capScope: "both", capPerCategory: "1500", maxCategories: 4, notes: asOf},
			{name: "Ozon Premium", paid: true, capValue: "3000", capScope: "both", capPerCategory: "1500", maxCategories: 4,
				notes: asOf + "; подписка добавляет выбираемые категории (Кафе и Рестораны 5%, Фастфуд 5%)"},
		},
	},
	{
		bank: "Яндекс Пэй", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "incremental",
		currencyKind: "points", pointsLabel: "Баллы Плюс",
		// rules: base selection is one-shot; the incremental feel comes from
		// GRANTED extra categories (мини-игра/Свои Плюсы), not from re-picking.
		midPeriodAdd: "locked_after_first", activation: "immediate",
		notes: asOf + "; баллы требуют активной подписки Яндекс Плюс; колесо фортуны — record-only",
		tiers: []tier{
			// 5 slots: «Выберите 5 из 12 категорий» / «Выберите ещё 4 …» + 1
			// selected, consistent across 2025-09, 2026-02 and 2026-07 (both
			// locales) in the picker corpus. The menu size varies (12–14) and
			// is NOT the slot count — «Select 4 more out of 14» reads as 14
			// slots if skimmed.
			{name: "Стандартный", capValue: "10000", capScope: "total", maxCategories: 5, notes: asOf + "; 5 слотов (скриншоты 2025-09 / 2026-02 / 2026-07)"},
		},
	},
	{
		bank: "Газпромбанк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub",
		midPeriodAdd: "unknown", activation: "unknown",
		notes: "факты не собраны (knowledge stub, 2026-07); period/mode/currency — предположения",
		tiers: []tier{
			{name: "Стандартный", capScope: "total", notes: "лимиты неизвестны (null = unknown)"},
		},
	},
	{
		bank: "МКБ", name: "Кэшбэк", periodType: "quarter", selectionMode: "atomic",
		currencyKind: "points", pointsLabel: "баллы МКБ",
		midPeriodAdd: "paid", activation: "next_day",
		notes: asOf + "; баллы 1:1 в рубли с месячными лимитами перевода; платная смена категории посреди квартала, активация на следующий день",
		tiers: []tier{
			{name: "Стандарт", capValue: "1500", capScope: "total", notes: asOf},
			{name: "Выгодный", capValue: "3000", capScope: "total", notes: asOf},
			{name: "Премиальный", capValue: "20000", capScope: "total", notes: asOf},
			{name: "Эксклюзивный", capValue: "50000", capScope: "total", notes: asOf},
		},
	},
	{
		bank: "Т-Банк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "incremental",
		currencyKind: "rub",
		// Filling a still-empty slot later is what the picker demonstrably
		// allows (the sheet re-titles to «Select another category»); whether
		// a TAKEN slot can be swapped stays unobserved, and that is a
		// different question from adding.
		midPeriodAdd: "allowed", activation: "unknown",
		notes: tbankAsOf,
		tiers: []tier{
			// The subscription buys cap, not slots: 4 is the only count ever
			// observed, across all eight sampled months.
			{name: "Стандартный", capValue: "3000", capScope: "total", maxCategories: 4, notes: tbankAsOf},
			{name: "Pro", paid: true, capValue: "5000", capScope: "total", maxCategories: 4, notes: tbankAsOf},
			{name: "Premium", paid: true, capValue: "30000", capScope: "total", maxCategories: 4, notes: tbankAsOf},
		},
	},
}

// Reference-only banks (wiki): no programs seeded.
var extraBanks = []string{"Сбербанк"}

// Emoji are UI icons for the picker, curated in the taxonomy page
// (2026-07-16, timeless).
var canonicalCategories = [][3]string{
	{"supermarkets", "Супермаркеты", "🛒"},
	{"restaurants", "Кафе и рестораны", "🍽️"},
	{"fastfood", "Фастфуд", "🍔"},
	{"gas-stations", "АЗС", "⛽"},
	{"pharmacies", "Аптеки", "💊"},
	{"taxi", "Такси", "🚕"},
	{"transport", "Транспорт", "🚌"},
	{"travel", "Путешествия", "🧳"},
	{"auto", "Авто", "🚗"},
	{"car-rental", "Аренда авто", "🚙"},
	{"home-repair", "Дом и ремонт", "🏠"},
	{"pets", "Животные", "🐾"},
	{"books", "Книги", "📚"},
	{"utilities", "Коммунальные услуги", "💡"},
	{"beauty", "Красота", "💅"},
	{"education", "Образование", "🎓"},
	{"clothes", "Одежда и обувь", "👕"},
	{"subscriptions", "Подписки", "🔁"},
	{"entertainment", "Развлечения", "🎉"},
	{"telecom", "Связь, интернет и ТВ", "📡"},
	{"sport-goods", "Спортивные товары", "⚽"},
	{"electronics", "Техника", "💻"},
	{"flowers", "Цветы", "💐"},
	{"digital-goods", "Цифровые товары", "🎮"},
	{"kids", "Детские товары", "🧸"},
	{"jewelry", "Ювелирные изделия", "💍"},
	{"marketplaces", "Маркетплейсы", "📦"},
	{"medicine", "Медицина", "🩺"},
	{"charity", "Благотворительность", "🤝"},
	{"all-purchases", "Все покупки", "🧾"},
	// Additions from the categories taxonomy (knowledge:
	// concepts/categories-taxonomy.md, five banks synthesized 2026-07-14).
	{"avia-tickets", "Авиабилеты", "✈️"},
	{"rail-tickets", "Ж/д билеты", "🚆"},
	{"hotels", "Отели", "🏨"},
	{"travel-agencies", "Турагентства", "🗺️"},
	{"duty-free", "Duty Free", "🛫"},
	{"auto-parts", "Автозапчасти", "🔧"},
	{"auto-services", "Автоуслуги", "🛠️"},
	{"car-purchase", "Покупка авто", "🚘"},
	{"toll-roads", "Платные дороги", "🛣️"},
	{"cosmetics", "Косметика и парфюмерия", "💄"},
	{"accessories", "Аксессуары", "👜"},
	{"online-cinema", "Онлайн-кинотеатры", "📺"},
	{"culture", "Культура и искусство", "🎭"},
	{"cinema", "Кино и театры", "🎬"},
	{"music", "Музыка", "🎵"},
	{"active-leisure", "Активный отдых и фитнес", "🏋️"},
	{"health", "Здоровье", "❤️"},
	{"health-goods", "Товары для здоровья", "🩹"},
	{"alcohol", "Алкоголь", "🍷"},
	{"hobby", "Хобби", "🎨"},
	{"insurance", "Страхование", "🛡️"},
	{"household-services", "Бытовые услуги", "🧺"},
	{"photo-video", "Фото и видео", "📷"},
	{"souvenirs", "Сувениры", "🎁"},
	{"fines", "Штрафы", "🚨"},
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
	{"Ozon Банк", "Продукты", "supermarkets"},
	{"Ozon Банк", "Аптеки", "pharmacies"},
	{"Ozon Банк", "Кафе и Рестораны", "restaurants"},
	{"Ozon Банк", "Фастфуд", "fastfood"},
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
	{"Ozon Банк", "Супермаркеты", "supermarkets"},
	{"Ozon Банк", "Рестораны", "restaurants"},
	{"Ozon Банк", "Топливо и АЗС", "gas-stations"},
	{"Ozon Банк", "Топливо и автомобильные заправочные станции", "gas-stations"},
	{"Ozon Банк", "Такси", "taxi"},
	{"Ozon Банк", "Транспорт", "transport"},
	{"Ozon Банк", "Каршеринг", "car-rental"},
	{"Ozon Банк", "Аренда автомобилей", "car-rental"},
	{"Ozon Банк", "Автоуслуги", "auto-services"},
	{"Ozon Банк", "Автомобильные услуги", "auto-services"},
	{"Ozon Банк", "Авиабилеты", "avia-tickets"},
	{"Ozon Банк", "ЖД билеты", "rail-tickets"},
	{"Ozon Банк", "Железнодорожные билеты", "rail-tickets"},
	{"Ozon Банк", "Отели", "hotels"},
	{"Ozon Банк", "Турагентства", "travel-agencies"},
	{"Ozon Банк", "Магазины беспошлинной торговли - Duty Free", "duty-free"},
	{"Ozon Банк", "VIP-залы", "travel"},
	{"Ozon Банк", "Круизы", "travel"},
	{"Ozon Банк", "Салоны красоты и СПА", "beauty"},
	{"Ozon Банк", "Салоны красоты и SPA", "beauty"},
	{"Ozon Банк", "Косметика", "cosmetics"},
	{"Ozon Банк", "Косметика и парфюмерия", "cosmetics"},
	{"Ozon Банк", "Дом и ремонт", "home-repair"},
	{"Ozon Банк", "Дом, ремонт", "home-repair"},
	{"Ozon Банк", "Ветклиники и зоомагазины", "pets"},
	{"Ozon Банк", "Зоотовары", "pets"},
	{"Ozon Банк", "Ветеринарные клиники", "pets"},
	{"Ozon Банк", "Искусство", "culture"},
	{"Ozon Банк", "Выставки и музеи", "culture"},
	{"Ozon Банк", "Книги", "books"},
	{"Ozon Банк", "Кино", "cinema"},
	{"Ozon Банк", "Развлечения", "entertainment"},
	{"Ozon Банк", "Всё для геймеров", "entertainment"},
	{"Ozon Банк", "Фитнес", "active-leisure"},
	{"Ozon Банк", "Медицинские клиники", "medicine"},
	{"Ozon Банк", "Медицинские клиники и эстетическая медицина", "medicine"},
	{"Ozon Банк", "Медицинские услуги", "medicine"},
	{"Ozon Банк", "Стоматология", "medicine"},
	{"Ozon Банк", "Музыка", "music"},
	{"Ozon Банк", "Музыкальные инструменты", "music"},
	{"Ozon Банк", "Электроника и бытовая техника", "electronics"},
	{"Ozon Банк", "Связь и Интернет", "telecom"},
	{"Ozon Банк", "Химчистки", "household-services"},
	{"Ozon Банк", "Фото и видео", "photo-video"},
	{"Ozon Банк", "Фото/Видео", "photo-video"},
	{"Ozon Банк", "Сувениры", "souvenirs"},
	{"Ozon Банк", "Цветы", "flowers"},
	{"Ozon Банк", "Спорттовары", "sport-goods"},
	{"Ozon Банк", "Одежда и обувь", "clothes"},
	{"Ozon Банк", "Одежда, обувь", "clothes"},
	{"Ozon Банк", "Товары для детей", "kids"},
	{"Ozon Банк", "Ювелирные изделия", "jewelry"},
	{"Ozon Банк", "ЖКХ", "utilities"},
	{"Ozon Банк", "Образование", "education"},
	{"Ozon Банк", "Все покупки", "all-purchases"},
	{"Ozon Банк", "На все покупки", "all-purchases"}, // 2024 loyalty PDF wording; app says «Все покупки»
	{"Ozon Банк", "Стандартный кешбэк 1%", "all-purchases"},
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
	// Beyond the 2026-04 MCC appendix — live app, 2026-07-24.
	{"Т-Банк", "Все покупки", "all-purchases"},
	// The «… в Городе» family maps to NO canonical (2026-07-28) — see
	// the catalog block below; aliasing them would make the lookup promise
	// the rate at any АЗС / supermarket.
	{"Яндекс Пэй", "Кафе, бары и рестораны", "restaurants"},
	{"Яндекс Пэй", "Кафе, рестораны и бары", "restaurants"}, // 2026-07 rules wording; app says «бары и рестораны»
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
	{"Яндекс Пэй", "Все покупки", "all-purchases"},
	// МКБ had no aliases at all — added with its first catalog (2026-07-27).
	{"МКБ", "на все покупки", "all-purchases"},
	{"МКБ", "на АЗС", "gas-stations"},
	{"МКБ", "Такси", "taxi"},
	{"МКБ", "Городской транспорт", "transport"},
	{"МКБ", "Каршеринг", "car-rental"},
	{"МКБ", "Автопутешествия", "toll-roads"},
	{"МКБ", "Спортивные клубы", "active-leisure"},
	{"МКБ", "Бытовые услуги", "household-services"},
	{"МКБ", "Детские товары", "kids"},
	{"МКБ", "Домашние питомцы", "pets"},
	{"МКБ", "Салоны красоты и барбершопы", "beauty"},
	{"МКБ", "Дьюти-фри", "duty-free"},
	{"МКБ", "Развлечения", "entertainment"},
	{"МКБ", "Кино и театры", "cinema"},
	{"МКБ", "Видеоигры", "digital-goods"},
	{"Яндекс Пэй", "На всё", "all-purchases"}, // 2026-07 rules wording; app says «Все покупки»
}

// Seeded titles that were later renamed/retired: the refresh upsert keys on
// (bank, title), so a rename leaves the old row behind — this list removes
// it from existing DBs. Deleting is safe: offers keep their raw_title
// snapshot (FK nulls out), MCC memberships cascade and re-land on the new
// title via the seed CSV.
var retiredBankCategories = []struct{ bank, title string }{
	{"Альфа-Банк", "Супермаркеты"}, // → «Продукты» (verified 2026-07-22)
	{"Т-Банк", "Бензин в городе"},  // never in the live app — «Топливо в Городе» is the real row (2026-07-27)
	// Corpus sweep 2026-07-27: these titles came from rules PDFs / recollection
	// and the app renders them differently. Right-hand side is the live wording.
	{"Т-Банк", "Топливо в городе"},           // → «Топливо в Городе»
	{"Т-Банк", "Шоппинг в городе"},           // → «Шопинг в Городе»
	{"Ozon Банк", "На все покупки"},          // → «Все покупки»
	{"Яндекс Пэй", "На всё"},                 // → «Все покупки»
	{"Яндекс Пэй", "Кафе, рестораны и бары"}, // → «Кафе, бары и рестораны»
	{"Яндекс Пэй", "Сервис «Яндекс Маркет»"}, // → «Яндекс Маркет»
	{"Яндекс Пэй", "Подписка Яндекс Плюс"},   // → «Яндекс Плюс»
}

// Aliases seeded for titles that turned out not to exist: same problem as
// retiredBankCategories, one table down (a stale alias would keep resolving a
// raw title nobody can enter).
var retiredAliases = []struct{ bank, raw string }{
	{"Т-Банк", "Бензин в городе"}, // 2026-07-27
	// The «… в Городе» family lost its canonical mappings (2026-07-28);
	// the aliases have to go too, or a raw title typed into a period would
	// still resolve to АЗС / Супермаркеты / Автоуслуги.
	{"Т-Банк", "Топливо в Городе"},
	{"Т-Банк", "Топливо в городе"},
	{"Т-Банк", "Супермаркеты в Городе"},
	{"Т-Банк", "Автосервисы в Городе"},
}

// Brand colors for UI bank tinting (knowledge: bank pages + index,
// as of 2026-07).
var bankColors = map[string]string{
	"Альфа-Банк":  "#EF3124",
	"ВТБ":         "#0A2896",
	"Ozon Банк":   "#005BFF",
	"Яндекс Пэй":  "#FC3F1D",
	"Газпромбанк": "#10069F",
	"МКБ":         "#E31E24",
	"Сбербанк":    "#21A038",
	"Т-Банк":      "#FFDD2D",
}

// Per-bank picker catalogs: the CURRENTLY selectable menu rows, from the
// taxonomy page's «Bank catalogs» section (⚠️-flagged rows there pending
// live-app re-verification). slug "" = canonical-less service/channel row —
// still ordinary kind=regular (corrected 2026-07-21: special is for
// granted bonus mechanics like Пятница/колесо, never catalog rows); emoji
// "" = inherit the canonical's.
var bankCategories = []struct{ bank, title, slug, kind, emoji string }{
	// Альфа-Банк (as of 2026-01 PDF; menus 2025-01/02; «Продукты» verified live 2026-07-22)
	{bank: "Альфа-Банк", title: "Продукты", slug: "supermarkets"},
	{bank: "Альфа-Банк", title: "Кафе и рестораны", slug: "restaurants"},
	{bank: "Альфа-Банк", title: "Фастфуд", slug: "fastfood"},
	{bank: "Альфа-Банк", title: "АЗС", slug: "gas-stations"},
	{bank: "Альфа-Банк", title: "Аптеки", slug: "pharmacies"},
	{bank: "Альфа-Банк", title: "Такси", slug: "taxi"},
	{bank: "Альфа-Банк", title: "Транспорт", slug: "transport"},
	{bank: "Альфа-Банк", title: "Путешествия", slug: "travel"},
	{bank: "Альфа-Банк", title: "Аренда авто", slug: "car-rental"},
	{bank: "Альфа-Банк", title: "Авто", slug: "auto"},
	{bank: "Альфа-Банк", title: "Автозапчасти", slug: "auto-parts"},
	{bank: "Альфа-Банк", title: "Автоуслуги", slug: "auto-services"},
	{bank: "Альфа-Банк", title: "Дом и ремонт", slug: "home-repair"},
	{bank: "Альфа-Банк", title: "Животные", slug: "pets"},
	{bank: "Альфа-Банк", title: "Книги", slug: "books"},
	{bank: "Альфа-Банк", title: "Коммунальные услуги", slug: "utilities"},
	{bank: "Альфа-Банк", title: "Красота", slug: "beauty"},
	{bank: "Альфа-Банк", title: "Образование", slug: "education"},
	{bank: "Альфа-Банк", title: "Одежда и обувь", slug: "clothes"},
	{bank: "Альфа-Банк", title: "Подписки", slug: "subscriptions"},
	{bank: "Альфа-Банк", title: "Развлечения", slug: "entertainment"},
	{bank: "Альфа-Банк", title: "Связь, интернет и ТВ", slug: "telecom"},
	{bank: "Альфа-Банк", title: "Спортивные товары", slug: "sport-goods"},
	{bank: "Альфа-Банк", title: "Активный отдых", slug: "active-leisure"},
	{bank: "Альфа-Банк", title: "Техника", slug: "electronics"},
	{bank: "Альфа-Банк", title: "Цветы", slug: "flowers"},
	{bank: "Альфа-Банк", title: "Цифровые товары", slug: "digital-goods"},
	{bank: "Альфа-Банк", title: "Детские товары", slug: "kids"},
	{bank: "Альфа-Банк", title: "Ювелирные изделия", slug: "jewelry"},
	{bank: "Альфа-Банк", title: "Маркетплейсы", slug: "marketplaces"},
	{bank: "Альфа-Банк", title: "Медицинские услуги", slug: "medicine"},
	{bank: "Альфа-Банк", title: "Здоровье", slug: "health"},
	{bank: "Альфа-Банк", title: "Алкоголь", slug: "alcohol"},
	{bank: "Альфа-Банк", title: "За все покупки", slug: "all-purchases"},
	{bank: "Альфа-Банк", title: "Альфа-Тревел", emoji: "🧳"},
	{bank: "Альфа-Банк", title: "Альфа-Заправки", emoji: "⛽"},
	{bank: "Альфа-Банк", title: "Альфа-Маркет", emoji: "🛍️"},
	{bank: "Альфа-Банк", title: "Альфа-Афиша", emoji: "🎟️"},
	{bank: "Альфа-Банк", title: "Интернет", emoji: "🌐"},
	{bank: "Альфа-Банк", title: "Налоги", emoji: "🏛️"},
	{bank: "Альфа-Банк", title: "Транспортные карты", emoji: "🚇"},
	{bank: "Альфа-Банк", title: "На связь Билайн", emoji: "📶"},
	{bank: "Альфа-Банк", title: "Яндекс Еда", emoji: "🍱"},
	{bank: "Альфа-Банк", title: "Яндекс Плюс", emoji: "➕"},
	{bank: "Альфа-Банк", title: "Яндекс Такси", emoji: "🚖"},
	{bank: "Альфа-Банк", title: "Деливери", emoji: "🛵"},
	{bank: "Альфа-Банк", title: "KASSIR.RU", emoji: "🎫"},
	{bank: "Альфа-Банк", title: "Подели", emoji: "💳"},
	// Ozon Банк (as of 2026-07 help page)
	{bank: "Ozon Банк", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "Ozon Банк", title: "Рестораны", slug: "restaurants"},
	{bank: "Ozon Банк", title: "Фастфуд", slug: "fastfood"},
	{bank: "Ozon Банк", title: "Топливо и АЗС", slug: "gas-stations"},
	{bank: "Ozon Банк", title: "Аптеки", slug: "pharmacies"},
	{bank: "Ozon Банк", title: "Такси", slug: "taxi"},
	{bank: "Ozon Банк", title: "Транспорт", slug: "transport"},
	{bank: "Ozon Банк", title: "Авиабилеты", slug: "avia-tickets"},
	{bank: "Ozon Банк", title: "ЖД билеты", slug: "rail-tickets"},
	{bank: "Ozon Банк", title: "Отели", slug: "hotels"},
	{bank: "Ozon Банк", title: "Каршеринг", slug: "car-rental"},
	{bank: "Ozon Банк", title: "Автоуслуги", slug: "auto-services"},
	{bank: "Ozon Банк", title: "Дом и ремонт", slug: "home-repair"},
	{bank: "Ozon Банк", title: "Ветклиники и зоомагазины", slug: "pets"},
	{bank: "Ozon Банк", title: "Книги", slug: "books"},
	{bank: "Ozon Банк", title: "Салоны красоты и СПА", slug: "beauty"},
	{bank: "Ozon Банк", title: "Косметика", slug: "cosmetics"},
	{bank: "Ozon Банк", title: "Образование", slug: "education"},
	{bank: "Ozon Банк", title: "Одежда и обувь", slug: "clothes"},
	{bank: "Ozon Банк", title: "Развлечения", slug: "entertainment"},
	{bank: "Ozon Банк", title: "Искусство", slug: "culture"},
	{bank: "Ozon Банк", title: "Выставки и музеи", slug: "culture"},
	{bank: "Ozon Банк", title: "Музыка", slug: "music"},
	{bank: "Ozon Банк", title: "Спорттовары", slug: "sport-goods"},
	{bank: "Ozon Банк", title: "Фитнес", slug: "active-leisure"},
	{bank: "Ozon Банк", title: "Электроника и бытовая техника", slug: "electronics"},
	{bank: "Ozon Банк", title: "Цветы", slug: "flowers"},
	{bank: "Ozon Банк", title: "Товары для детей", slug: "kids"},
	{bank: "Ozon Банк", title: "Ювелирные изделия", slug: "jewelry"},
	{bank: "Ozon Банк", title: "Медицинские клиники", slug: "medicine"},
	{bank: "Ozon Банк", title: "Химчистки", slug: "household-services"},
	{bank: "Ozon Банк", title: "Фото и видео", slug: "photo-video"},
	{bank: "Ozon Банк", title: "Все покупки", slug: "all-purchases"},
	// Merchant rows rendered inline in the Озон picker with their logos,
	// slot-consuming (2026-06/2026-07 screenshots). Акции outside the menu
	// («Партнёрские акции» history filter) stay partner_offer.
	{bank: "Ozon Банк", title: "Tasty Coffee", emoji: "☕"},
	{bank: "Ozon Банк", title: "START", emoji: "🎬"},
	{bank: "Ozon Банк", title: "Дикси Доставка", emoji: "🛒"},
	{bank: "Ozon Банк", title: "Пятёрочка Доставка", emoji: "🛒"},
	{bank: "Ozon Банк", title: "РИВ ГОШ", emoji: "💄"},
	{bank: "Ozon Банк", title: "Отели на Туту", emoji: "🏨"},
	{bank: "Ozon Банк", title: "Сварщица Екатерина", emoji: "🔧"},
	// ВТБ (as of 2025-12 «Мультибонус» rules)
	{bank: "ВТБ", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "ВТБ", title: "Кафе и рестораны", slug: "restaurants"},
	{bank: "ВТБ", title: "АЗС", slug: "gas-stations"},
	{bank: "ВТБ", title: "Аптеки", slug: "pharmacies"},
	{bank: "ВТБ", title: "Такси", slug: "taxi"},
	{bank: "ВТБ", title: "Транспорт", slug: "transport"},
	{bank: "ВТБ", title: "Авиабилеты", slug: "avia-tickets"},
	{bank: "ВТБ", title: "Ж/д билеты", slug: "rail-tickets"},
	{bank: "ВТБ", title: "Отели", slug: "hotels"},
	{bank: "ВТБ", title: "Турагентства", slug: "travel-agencies"},
	{bank: "ВТБ", title: "Duty Free", slug: "duty-free"},
	{bank: "ВТБ", title: "Аренда авто", slug: "car-rental"},
	{bank: "ВТБ", title: "Продажа авто", slug: "car-purchase"},
	{bank: "ВТБ", title: "Автоуслуги", slug: "auto-services"},
	{bank: "ВТБ", title: "Платные дороги", slug: "toll-roads"},
	{bank: "ВТБ", title: "Дом и ремонт", slug: "home-repair"},
	{bank: "ВТБ", title: "Зоотовары", slug: "pets"},
	{bank: "ВТБ", title: "Книги и канцтовары", slug: "books"},
	{bank: "ВТБ", title: "Услуги ЖКХ", slug: "utilities"},
	{bank: "ВТБ", title: "Красота", slug: "beauty"},
	{bank: "ВТБ", title: "Образование", slug: "education"},
	{bank: "ВТБ", title: "Одежда и обувь", slug: "clothes"},
	{bank: "ВТБ", title: "Развлечения", slug: "entertainment"},
	{bank: "ВТБ", title: "Искусство", slug: "culture"},
	{bank: "ВТБ", title: "Театры и кино", slug: "cinema"},
	{bank: "ВТБ", title: "Услуги связи", slug: "telecom"},
	{bank: "ВТБ", title: "Спортивные товары", slug: "sport-goods"},
	{bank: "ВТБ", title: "Фитнес", slug: "active-leisure"},
	{bank: "ВТБ", title: "Электроника", slug: "electronics"},
	{bank: "ВТБ", title: "Цветы", slug: "flowers"},
	{bank: "ВТБ", title: "Цифровой контент", slug: "digital-goods"},
	{bank: "ВТБ", title: "Детские товары", slug: "kids"},
	{bank: "ВТБ", title: "Украшения и бижутерия", slug: "jewelry"},
	{bank: "ВТБ", title: "Маркетплейсы", slug: "marketplaces"},
	{bank: "ВТБ", title: "Здоровье", slug: "health"},
	{bank: "ВТБ", title: "Алкоголь", slug: "alcohol"},
	{bank: "ВТБ", title: "Страхование", slug: "insurance"},
	{bank: "ВТБ", title: "Бытовые услуги", slug: "household-services"},
	{bank: "ВТБ", title: "Все покупки", slug: "all-purchases"},
	{bank: "ВТБ", title: "Все остальные покупки", slug: "all-purchases"},
	{bank: "ВТБ", title: "Оплата ЖКУ в ВТБ-Онлайн", emoji: "🧾"},
	{bank: "ВТБ", title: "Оплата сотовой связи в ВТБ-Онлайн", emoji: "📱"},
	// Own-service row seen in the picker corpus 2026-07 (ж/д и авиабилеты на
	// vtb.aviakassa.ru, «дополнительно к кешбэку в сервисе») — canonical-less:
	// it is a channel, not a spending category.
	{bank: "ВТБ", title: "ВТБ Путешествия", emoji: "🧳"},
	// Merchant rows inside the ВТБ picker (2026-01/2026-07 screenshots).
	{bank: "ВТБ", title: "Яндекс Лавка", emoji: "🥬"},
	{bank: "ВТБ", title: "М.Косметик", emoji: "💄"},
	{bank: "ВТБ", title: "Почта России", emoji: "📮"},
	// Т-Банк (as of 2026-04 MCC appendix; reference-only bank)
	{bank: "Т-Банк", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "Т-Банк", title: "Рестораны", slug: "restaurants"},
	{bank: "Т-Банк", title: "Фастфуд", slug: "fastfood"},
	{bank: "Т-Банк", title: "Заправки", slug: "gas-stations"},
	{bank: "Т-Банк", title: "Аптеки", slug: "pharmacies"},
	{bank: "Т-Банк", title: "Такси", slug: "taxi"},
	{bank: "Т-Банк", title: "Местный транспорт", slug: "transport"},
	{bank: "Т-Банк", title: "Самокаты", slug: "transport"},
	{bank: "Т-Банк", title: "Авиабилеты", slug: "avia-tickets"},
	{bank: "Т-Банк", title: "Ж/д билеты", slug: "rail-tickets"},
	{bank: "Т-Банк", title: "Duty Free", slug: "duty-free"},
	{bank: "Т-Банк", title: "Каршеринг", slug: "car-rental"},
	{bank: "Т-Банк", title: "Автоуслуги", slug: "auto-services"},
	{bank: "Т-Банк", title: "Платные дороги", slug: "toll-roads"},
	{bank: "Т-Банк", title: "Ремонт и мебель", slug: "home-repair"},
	{bank: "Т-Банк", title: "Животные", slug: "pets"},
	{bank: "Т-Банк", title: "Книги и канцтовары", slug: "books"},
	{bank: "Т-Банк", title: "Красота", slug: "beauty"},
	{bank: "Т-Банк", title: "Косметика", slug: "cosmetics"},
	{bank: "Т-Банк", title: "Образование", slug: "education"},
	{bank: "Т-Банк", title: "Одежда и обувь", slug: "clothes"},
	{bank: "Т-Банк", title: "Онлайн-кинотеатры", slug: "online-cinema"},
	{bank: "Т-Банк", title: "Развлечения", slug: "entertainment"},
	{bank: "Т-Банк", title: "Искусство", slug: "culture"},
	{bank: "Т-Банк", title: "Кино", slug: "cinema"},
	{bank: "Т-Банк", title: "Музыка", slug: "music"},
	{bank: "Т-Банк", title: "Спорттовары", slug: "sport-goods"},
	{bank: "Т-Банк", title: "Тренировки", slug: "active-leisure"},
	{bank: "Т-Банк", title: "Гаджеты и техника", slug: "electronics"},
	{bank: "Т-Банк", title: "Цветы", slug: "flowers"},
	{bank: "Т-Банк", title: "Цифровые товары", slug: "digital-goods"},
	{bank: "Т-Банк", title: "Детские товары", slug: "kids"},
	{bank: "Т-Банк", title: "Маркетплейсы", slug: "marketplaces"},
	{bank: "Т-Банк", title: "Подарки и творчество", slug: "hobby"},
	// Beyond the 2026-04 MCC appendix — live app, 2026-07-24 (the
	// appendix lacks these; full live-menu ingest queued in the knowledge log).
	{bank: "Т-Банк", title: "Все покупки", slug: "all-purchases"},
	// Geo-scoped «… в Городе» family — where Т-Банк's headline rates live
	// (10–30% vs 5–7% on plain rows). Spelling verified against the corpus
	// 2026-07-27: capital «Г», and «Шопинг» with one «п».
	// None of them carries a canonical (2026-07-28): «в Городе» pays
	// at partner merchants in the user's city, not across the whole MCC
	// category, so mapping «Топливо в Городе» onto АЗС would make the lookup
	// promise 10% at any filling station. Canonical-less means the rows stay
	// selectable and visible in their period, but never answer «какой картой
	// платить?» for АЗС / Супермаркеты / Автоуслуги. They carry their own
	// emoji — with no canonical there is nothing to inherit one from.
	{bank: "Т-Банк", title: "Топливо в Городе", emoji: "⛽"},
	{bank: "Т-Банк", title: "Супермаркеты в Городе", emoji: "🛒"},
	{bank: "Т-Банк", title: "Автосервисы в Городе", emoji: "🛠️"},
	// «Шопинг в Городе» was the first of the family ruled canonical-less
	// (2026-07-27): it also bundles одежда + техника + аксессуары +
	// маркетплейсы, so no single canonical was honest even before the
	// geo-scope argument.
	{bank: "Т-Банк", title: "Шопинг в Городе", emoji: "🛍️"},
	{bank: "Т-Банк", title: "Афиша в Городе", emoji: "🎭"},
	// Own-service rows — slot-consuming, hence catalog rows (2026-07-27).
	{bank: "Т-Банк", title: "Т-Страхование", emoji: "🛡️"},
	{bank: "Т-Банк", title: "Долями", emoji: "💳"},
	// Merchant + subscription-reward rows inside the Т-Банк picker.
	{bank: "Т-Банк", title: "MODI", emoji: "🛍️"},
	{bank: "Т-Банк", title: "Подписка Магнит в Т-Банке", emoji: "🧲"},
	{bank: "Т-Банк", title: "PREMIER в Т-Банке", emoji: "🎬"},
	// Яндекс Пэй (as of 2026-07-14 rules)
	{bank: "Яндекс Пэй", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "Яндекс Пэй", title: "Кафе, бары и рестораны", slug: "restaurants"},
	{bank: "Яндекс Пэй", title: "АЗС", slug: "gas-stations"},
	{bank: "Яндекс Пэй", title: "Аптеки", slug: "pharmacies"},
	{bank: "Яндекс Пэй", title: "Такси", slug: "taxi"},
	{bank: "Яндекс Пэй", title: "Городской транспорт", slug: "transport"},
	{bank: "Яндекс Пэй", title: "Платные дороги и парковки", slug: "toll-roads"},
	{bank: "Яндекс Пэй", title: "Автоуслуги", slug: "auto-services"},
	{bank: "Яндекс Пэй", title: "Товары для дома", slug: "home-repair"},
	{bank: "Яндекс Пэй", title: "Питомцы", slug: "pets"},
	{bank: "Яндекс Пэй", title: "Книги", slug: "books"},
	{bank: "Яндекс Пэй", title: "Красота", slug: "beauty"},
	{bank: "Яндекс Пэй", title: "Образование", slug: "education"},
	{bank: "Яндекс Пэй", title: "Одежда и обувь", slug: "clothes"},
	{bank: "Яндекс Пэй", title: "Онлайн-кинотеатры", slug: "online-cinema"},
	{bank: "Яндекс Пэй", title: "Кино", slug: "cinema"},
	{bank: "Яндекс Пэй", title: "Спорт и фитнес", slug: "active-leisure"},
	{bank: "Яндекс Пэй", title: "Электроника и бытовая техника", slug: "electronics"},
	{bank: "Яндекс Пэй", title: "Цветы", slug: "flowers"},
	{bank: "Яндекс Пэй", title: "Товары для детей", slug: "kids"},
	{bank: "Яндекс Пэй", title: "Ювелирные изделия", slug: "jewelry"},
	{bank: "Яндекс Пэй", title: "Медицина", slug: "medicine"},
	{bank: "Яндекс Пэй", title: "Все покупки", slug: "all-purchases"},
	// Own-service rows. Titles below carry the wording the app renders
	// («Яндекс Маркет», not «Сервис «Яндекс Маркет»» as the rules PDF has
	// it) — corpus-verified 2026-07-27 against Russian-locale screenshots.
	{bank: "Яндекс Пэй", title: "Яндекс Маркет", emoji: "🛍️"},
	{bank: "Яндекс Пэй", title: "Яндекс Лавка", emoji: "🛒"},
	{bank: "Яндекс Пэй", title: "Яндекс Заправки", emoji: "⛽"},
	{bank: "Яндекс Пэй", title: "Яндекс Такси", emoji: "🚕"},
	{bank: "Яндекс Пэй", title: "Яндекс Плюс", emoji: "➕"},
	// «Яндекс Музыка» here is концертные билеты, not the subscription —
	// the app's own subtitle is «Билеты на концерты».
	{bank: "Яндекс Пэй", title: "Яндекс Музыка", emoji: "🎫"},
	{bank: "Яндекс Пэй", title: "Кинопоиск", emoji: "🎬"},
	// «ОСАГО» sits under «С картой любого банка» and is scoped to Яндекс
	// Заботой (its own subtitle). Canonical-less despite `insurance`
	// existing, for the same reason «Яндекс Заправки» is canonical-less
	// despite `gas-stations`: the row pays only through Яндекс's service,
	// so mapping it would make «какой картой платить?» recommend it for an
	// ОСАГО bought anywhere else.
	{bank: "Яндекс Пэй", title: "ОСАГО", emoji: "🛡️"},
	// Channel rows (rules PDF, not seen in the picker corpus).
	{bank: "Яндекс Пэй", title: "Сервис «Яндекс Go»", emoji: "🚖"},
	{bank: "Яндекс Пэй", title: "1% в Сервисах Яндекса", emoji: "🟡"},
	{bank: "Яндекс Пэй", title: "Оплата через Яндекс Сплит", emoji: "💳"},
	{bank: "Яндекс Пэй", title: "Оплата через СБП со Счёта в Яндексе", emoji: "💸"},
	{bank: "Яндекс Пэй", title: "Оплата через SberPay QR", emoji: "🔳"},
	{bank: "Яндекс Пэй", title: "Оплата токеном по NFC", emoji: "📲"},
	// Merchant rows rendered inside the picker. Seeded canonical-less on the
	// slot test — picking one spends a slot, so it must exist to record a
	// month (2026-07-27: prefer more rows over fewer). These rotate
	// monthly; a row absent from the current menu is not wrong, just idle.
	{bank: "Яндекс Пэй", title: "Свои Плюсы в S7", emoji: "✈️"},
	{bank: "Яндекс Пэй", title: "РИВ ГОШ", emoji: "💄"},
	{bank: "Яндекс Пэй", title: "ЛЭТУАЛЬ", emoji: "💄"},
	{bank: "Яндекс Пэй", title: "Азбука вкуса", emoji: "🥬"},
	{bank: "Яндекс Пэй", title: "Tripster", emoji: "🧭"},
	{bank: "Яндекс Пэй", title: "AFINA", emoji: "🏛️"},
	{bank: "Яндекс Пэй", title: "Фоксфорд", emoji: "🦊"},
	{bank: "Яндекс Пэй", title: "Амедиатека", emoji: "🎬"},
	{bank: "Яндекс Пэй", title: "START", emoji: "🎬"},
	{bank: "Яндекс Пэй", title: "Иви", emoji: "📺"},
	// ⚠️ Titles below are RECONSTRUCTED from English-locale screenshots —
	// the corpus has no Russian rendering of these rows yet. That is the
	// exact move that produced the «Яндекс Забота»/«ОСАГО» miss, so treat
	// them as provisional and retire on the first Russian sighting that
	// disagrees. Seeded anyway on the «more rows over fewer» rule;
	// «Яндекс Еда» is the safest of the three (Альфа's catalog already
	// carries that exact Russian title).
	{bank: "Яндекс Пэй", title: "Яндекс Афиша", emoji: "🎭"},         // «Yandex Afisha», 25%, theater tickets
	{bank: "Яндекс Пэй", title: "Яндекс Еда", emoji: "🍱"},           // «Yandex Eats»
	{bank: "Яндекс Пэй", title: "Самокаты в Яндекс Go", emoji: "🛴"}, // «Scooters at Yandex Go»
	// МКБ (first catalog, from the 2025-09 screenshot ingest 2026-07-27).
	// The bank had a program but no catalog rows at all until now — the
	// quarter rotates the offered set rather than the rate (flat 5% almost
	// throughout), so this is a per-quarter snapshot, not a stable menu.
	// Lower-case «на …» prefixes are the app's own rendering, not a typo.
	{bank: "МКБ", title: "на все покупки", slug: "all-purchases"},
	{bank: "МКБ", title: "на АЗС", slug: "gas-stations"},
	{bank: "МКБ", title: "Такси", slug: "taxi"},
	{bank: "МКБ", title: "Городской транспорт", slug: "transport"},
	{bank: "МКБ", title: "Каршеринг", slug: "car-rental"},
	{bank: "МКБ", title: "Автопутешествия", slug: "toll-roads"},
	{bank: "МКБ", title: "Спортивные клубы", slug: "active-leisure"},
	{bank: "МКБ", title: "Бытовые услуги", slug: "household-services"},
	{bank: "МКБ", title: "Детские товары", slug: "kids"},
	{bank: "МКБ", title: "Домашние питомцы", slug: "pets"},
	{bank: "МКБ", title: "Салоны красоты и барбершопы", slug: "beauty"},
	{bank: "МКБ", title: "Дьюти-фри", slug: "duty-free"},
	{bank: "МКБ", title: "Развлечения", slug: "entertainment"},
	{bank: "МКБ", title: "Кино и театры", slug: "cinema"},
	{bank: "МКБ", title: "Видеоигры", slug: "digital-goods"},
	{bank: "МКБ", title: "МКБ Travel", emoji: "✈️"},
	{bank: "МКБ", title: "Бургер Кинг", emoji: "🍔"},
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
			                              points_label, selection_opens_day, mid_period_add, activation, notes)
			select b.id, $2, $3::cashback_period_type, $4::cashback_selection_mode,
			       $5::cashback_currency_kind, nullif($6, ''), nullif($7, 0),
			       $8::cashback_mid_period_add, $9::cashback_activation, nullif($10, '')
			from bank b
			where b.name = $1
			  and not exists (select 1
			                  from cashback_program cp
			                  where cp.bank_id = b.id and cp.name = $2)`,
			p.bank, p.name, p.periodType, p.selectionMode, p.currencyKind,
			p.pointsLabel, p.opensDay, p.midPeriodAdd, p.activation, p.notes); err != nil {
			return fmt.Errorf("seed program %s: %w", p.bank, err)
		}
		// Policy facts are knowledge-derived, not user data — refresh them on
		// existing rows too (the insert guard leaves pre-existing programs as
		// they were, which would strand prod on the 'unknown' defaults).
		if _, err := pool.Exec(ctx, `
			update cashback_program cp
			set mid_period_add = $3::cashback_mid_period_add,
			    activation     = $4::cashback_activation
			from bank b
			where b.id = cp.bank_id
			  and b.name = $1
			  and cp.name = $2`,
			p.bank, p.name, p.midPeriodAdd, p.activation); err != nil {
			return fmt.Errorf("seed program policy %s: %w", p.bank, err)
		}
		for _, t := range p.tiers {
			if _, err := pool.Exec(ctx, `
				insert into program_tier (program_id, name, is_paid_subscription, cap_value, cap_scope,
				                          cap_per_category, max_categories, notes)
				select cp.id, $3, $4, nullif($5, '')::numeric, $6::cashback_cap_scope,
				       nullif($7, '')::numeric, nullif($8, 0), nullif($9, '')
				from cashback_program cp
				         join bank b on b.id = cp.bank_id
				where b.name = $1
				  and cp.name = $2
				  and not exists (select 1
				                  from program_tier pt
				                  where pt.program_id = cp.id and pt.name = $3)`,
				p.bank, p.name, t.name, t.paid, t.capValue, t.capScope,
				t.capPerCategory, t.maxCategories, t.notes); err != nil {
				return fmt.Errorf("seed tier %s/%s: %w", p.bank, t.name, err)
			}
		}
	}

	for _, c := range canonicalCategories {
		if _, err := pool.Exec(ctx, `
			insert into canonical_category (slug, title_ru, emoji)
			values ($1, $2, $3)
			on conflict (slug) do nothing`, c[0], c[1], c[2]); err != nil {
			return fmt.Errorf("seed category %s: %w", c[0], err)
		}
		// Emoji are knowledge-derived reference data — refresh existing rows
		// too (same rationale as the program-policy pass above).
		if _, err := pool.Exec(ctx, `
			update canonical_category
			set emoji = $2
			where slug = $1`, c[0], c[2]); err != nil {
			return fmt.Errorf("seed category emoji %s: %w", c[0], err)
		}
	}

	// The global alias (user_id null) is knowledge-derived, so it REFRESHES on
	// re-runs like the catalog rows below. Until 00019 this was `do nothing`
	// while the API upserted the same row, so a mapping recorded by any single
	// account overwrote the seeded one for everyone and the seed could not take
	// it back. Per-account mappings are separate rows and stay untouched.
	for _, a := range aliases {
		if _, err := pool.Exec(ctx, `
			insert into bank_category_alias (canonical_category_id, bank_id, raw_title, user_id)
			select cc.id, b.id, $3, null
			from canonical_category cc,
			     bank b
			where cc.slug = $1
			  and b.name = $2
			on conflict (bank_id, raw_title, user_id)
				do update set canonical_category_id = excluded.canonical_category_id`, a.slug, a.bank, a.raw); err != nil {
			return fmt.Errorf("seed alias %s/%s: %w", a.bank, a.raw, err)
		}
	}

	for _, r := range retiredAliases {
		if _, err := pool.Exec(ctx, `
			delete from bank_category_alias a
			using bank b
			where b.id = a.bank_id
			  and b.name = $1
			  and a.raw_title = $2
			  and a.user_id is null`, r.bank, r.raw); err != nil {
			return fmt.Errorf("seed retire alias %s/%s: %w", r.bank, r.raw, err)
		}
	}

	// Retired titles go first, so a rename never leaves both spellings behind.
	for _, r := range retiredBankCategories {
		if _, err := pool.Exec(ctx, `
			delete from bank_category bc
			using bank b
			where b.id = bc.bank_id
			  and b.name = $1
			  and bc.title = $2
			  and bc.created_by is null
			  and not bc.is_custom`, r.bank, r.title); err != nil {
			return fmt.Errorf("seed retire bank category %s/%s: %w", r.bank, r.title, err)
		}
	}

	// Picker catalogs: seeded rows are knowledge-derived, so kind/canonical/
	// emoji REFRESH on re-runs (the 2026-07-21 спец→regular correction proved
	// manual-SQL-per-correction unworkable). The conflict target is the global
	// row (created_by null) — since 00019 a user's row with the same (bank,
	// title) is a separate row that no longer squats the title, and the
	// is_custom guard now only spares a pre-00019 custom row that could not be
	// attributed to an account.
	for _, c := range bankCategories {
		kind := c.kind
		if kind == "" {
			kind = "regular"
		}
		if _, err := pool.Exec(ctx, `
			insert into bank_category (bank_id, title, canonical_category_id, kind, emoji, is_custom, created_by)
			select b.id, $2,
			       (select cc.id from canonical_category cc where cc.slug = nullif($3, '')),
			       $4::cashback_offer_kind, nullif($5, ''), false, null
			from bank b
			where b.name = $1
			on conflict (bank_id, title, created_by) do update
				set canonical_category_id = excluded.canonical_category_id,
				    kind                  = excluded.kind,
				    emoji                 = excluded.emoji
			where not bank_category.is_custom`,
			c.bank, c.title, c.slug, kind, c.emoji); err != nil {
			return fmt.Errorf("seed bank category %s/%s: %w", c.bank, c.title, err)
		}
	}

	// Brand colors are knowledge-derived reference data — refresh
	// unconditionally.
	for name, hex := range bankColors {
		if _, err := pool.Exec(ctx, `
			update bank
			set color_hex = $2
			where name = $1`, name, hex); err != nil {
			return fmt.Errorf("seed bank color %s: %w", name, err)
		}
	}

	// MCC dictionary + per-bank category→MCC membership (embedded CSVs,
	// derived from the meta-repo curation — see mcc.go).
	return seedMCC(ctx, pool)
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
