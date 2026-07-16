// Package seed loads reference data derived from the knowledge base
// (docs/knowledge in the private meta-repo): the owner's six banks with
// their КБ programs and tiers, ~55 canonical categories (with UI emoji),
// the known bank-title aliases, the per-bank picker catalogs
// (bank_category), and bank brand colors (five banks + catalogs/emoji/
// colors from concepts/categories-taxonomy.md and the bank pages,
// 2026-07-16). Program/tier numbers are as of 2025-05 (wiki table) — notes
// on every program/tier say so; re-verify against live bank apps before
// relying on them for real decisions.
//
// Idempotent: safe to run repeatedly (natural-key upserts). Knowledge-
// derived reference facts (program policy, emoji, brand colors) are
// refreshed unconditionally on existing rows; user-editable rows
// (bank_category — users add custom ones) are insert-only: an existing row
// with the same (bank, title), custom or not, always wins over the seed.
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
	// midPeriodAdd: can a category be ADDED mid-period? (allowed |
	// locked_after_first | paid | unknown). NOT derivable from
	// selectionMode — Альфа is atomic yet allows adds (owner 2026-07-16).
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
		midPeriodAdd: "allowed", activation: "immediate", // owner 2026-07-16: add while a slot is free
		notes: asOf,
		tiers: []tier{
			{name: "Стандартный", capValue: "5000", capScope: "total", maxCategories: 3, basePercent: "5", notes: asOf},
			{name: "Альфа-Смарт", paid: true, capValue: "7000", capScope: "total", maxCategories: 4, basePercent: "5", notes: asOf},
			{name: "Alfa Only", paid: true, capValue: "15000", capScope: "total", maxCategories: 5, basePercent: "7", notes: asOf},
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
		bank: "Озон Банк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
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
			{name: "Стандартный", capValue: "10000", capScope: "total", notes: asOf + "; max категорий неизвестен"},
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
			{name: "Выгодный", capValue: "3000", capScope: "total", basePercent: "5", notes: asOf},
			{name: "Премиальный", capValue: "20000", capScope: "total", basePercent: "7", notes: asOf},
			{name: "Эксклюзивный", capValue: "50000", capScope: "total", basePercent: "7", notes: asOf},
		},
	},
}

// Reference-only banks (wiki): no programs seeded.
var extraBanks = []string{"Сбербанк", "Т-Банк"}

// Emoji are UI icons for the picker, owner-curated in the taxonomy page
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

// Brand colors for UI bank tinting (knowledge: bank pages + index,
// as of 2026-07).
var bankColors = map[string]string{
	"Альфа-Банк":  "#EF3124",
	"ВТБ":         "#0A2896",
	"Озон Банк":   "#005BFF",
	"Яндекс Пэй":  "#FC3F1D",
	"Газпромбанк": "#10069F",
	"МКБ":         "#E31E24",
	"Сбербанк":    "#21A038",
	"Т-Банк":      "#FFDD2D",
}

// Per-bank picker catalogs: the CURRENTLY selectable menu rows, from the
// taxonomy page's «Bank catalogs» section (⚠️-flagged rows there pending
// live-app re-verification). slug "" = special/service row without a
// canonical; kind "" = regular; emoji "" = inherit the canonical's.
var bankCategories = []struct{ bank, title, slug, kind, emoji string }{
	// Альфа-Банк (as of 2026-01 PDF; menus 2025-01/02)
	{bank: "Альфа-Банк", title: "Супермаркеты", slug: "supermarkets"},
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
	{bank: "Альфа-Банк", title: "Альфа-Тревел", kind: "special", emoji: "🧳"},
	{bank: "Альфа-Банк", title: "Альфа-Заправки", kind: "special", emoji: "⛽"},
	{bank: "Альфа-Банк", title: "Альфа-Маркет", kind: "special", emoji: "🛍️"},
	{bank: "Альфа-Банк", title: "Альфа-Афиша", kind: "special", emoji: "🎟️"},
	{bank: "Альфа-Банк", title: "Интернет", kind: "special", emoji: "🌐"},
	{bank: "Альфа-Банк", title: "Налоги", kind: "special", emoji: "🏛️"},
	{bank: "Альфа-Банк", title: "Транспортные карты", kind: "special", emoji: "🚇"},
	{bank: "Альфа-Банк", title: "На связь Билайн", kind: "special", emoji: "📶"},
	{bank: "Альфа-Банк", title: "Яндекс Еда", kind: "special", emoji: "🍱"},
	{bank: "Альфа-Банк", title: "Яндекс Плюс", kind: "special", emoji: "➕"},
	{bank: "Альфа-Банк", title: "Яндекс Такси", kind: "special", emoji: "🚖"},
	{bank: "Альфа-Банк", title: "Деливери", kind: "special", emoji: "🛵"},
	{bank: "Альфа-Банк", title: "KASSIR.RU", kind: "special", emoji: "🎫"},
	{bank: "Альфа-Банк", title: "Подели", kind: "special", emoji: "💳"},
	// Озон Банк (as of 2026-07 help page)
	{bank: "Озон Банк", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "Озон Банк", title: "Рестораны", slug: "restaurants"},
	{bank: "Озон Банк", title: "Фастфуд", slug: "fastfood"},
	{bank: "Озон Банк", title: "Топливо и АЗС", slug: "gas-stations"},
	{bank: "Озон Банк", title: "Аптеки", slug: "pharmacies"},
	{bank: "Озон Банк", title: "Такси", slug: "taxi"},
	{bank: "Озон Банк", title: "Транспорт", slug: "transport"},
	{bank: "Озон Банк", title: "Авиабилеты", slug: "avia-tickets"},
	{bank: "Озон Банк", title: "ЖД билеты", slug: "rail-tickets"},
	{bank: "Озон Банк", title: "Отели", slug: "hotels"},
	{bank: "Озон Банк", title: "Каршеринг", slug: "car-rental"},
	{bank: "Озон Банк", title: "Автоуслуги", slug: "auto-services"},
	{bank: "Озон Банк", title: "Дом и ремонт", slug: "home-repair"},
	{bank: "Озон Банк", title: "Ветклиники и зоомагазины", slug: "pets"},
	{bank: "Озон Банк", title: "Книги", slug: "books"},
	{bank: "Озон Банк", title: "Салоны красоты и СПА", slug: "beauty"},
	{bank: "Озон Банк", title: "Косметика", slug: "cosmetics"},
	{bank: "Озон Банк", title: "Образование", slug: "education"},
	{bank: "Озон Банк", title: "Одежда и обувь", slug: "clothes"},
	{bank: "Озон Банк", title: "Развлечения", slug: "entertainment"},
	{bank: "Озон Банк", title: "Искусство", slug: "culture"},
	{bank: "Озон Банк", title: "Выставки и музеи", slug: "culture"},
	{bank: "Озон Банк", title: "Музыка", slug: "music"},
	{bank: "Озон Банк", title: "Спорттовары", slug: "sport-goods"},
	{bank: "Озон Банк", title: "Фитнес", slug: "active-leisure"},
	{bank: "Озон Банк", title: "Электроника и бытовая техника", slug: "electronics"},
	{bank: "Озон Банк", title: "Цветы", slug: "flowers"},
	{bank: "Озон Банк", title: "Товары для детей", slug: "kids"},
	{bank: "Озон Банк", title: "Ювелирные изделия", slug: "jewelry"},
	{bank: "Озон Банк", title: "Медицинские клиники", slug: "medicine"},
	{bank: "Озон Банк", title: "Химчистки", slug: "household-services"},
	{bank: "Озон Банк", title: "Фото и видео", slug: "photo-video"},
	{bank: "Озон Банк", title: "На все покупки", slug: "all-purchases"},
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
	{bank: "ВТБ", title: "Оплата ЖКУ в ВТБ-Онлайн", kind: "special", emoji: "🧾"},
	{bank: "ВТБ", title: "Оплата сотовой связи в ВТБ-Онлайн", kind: "special", emoji: "📱"},
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
	// Яндекс Пэй (as of 2026-07-14 rules)
	{bank: "Яндекс Пэй", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "Яндекс Пэй", title: "Кафе, рестораны и бары", slug: "restaurants"},
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
	{bank: "Яндекс Пэй", title: "На всё", slug: "all-purchases"},
	{bank: "Яндекс Пэй", title: "Сервис «Яндекс Маркет»", kind: "special", emoji: "🛍️"},
	{bank: "Яндекс Пэй", title: "Сервис «Яндекс Go»", kind: "special", emoji: "🚖"},
	{bank: "Яндекс Пэй", title: "1% в Сервисах Яндекса", kind: "special", emoji: "🟡"},
	{bank: "Яндекс Пэй", title: "Оплата через Яндекс Сплит", kind: "special", emoji: "💳"},
	{bank: "Яндекс Пэй", title: "Оплата через СБП со Счёта в Яндексе", kind: "special", emoji: "💸"},
	{bank: "Яндекс Пэй", title: "Оплата через SberPay QR", kind: "special", emoji: "🔳"},
	{bank: "Яндекс Пэй", title: "Оплата токеном по NFC", kind: "special", emoji: "📲"},
	{bank: "Яндекс Пэй", title: "Яндекс Такси", kind: "special", emoji: "🚕"},
	{bank: "Яндекс Пэй", title: "Подписка Яндекс Плюс", kind: "special", emoji: "➕"},
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

	// Picker catalogs are insert-only: a pre-existing row with the same
	// (bank, title) — typically a user-created custom one — always wins, and
	// re-running seed never mutates kind/canonical on existing rows
	// (knowledge corrections need a new title or manual SQL).
	for _, c := range bankCategories {
		kind := c.kind
		if kind == "" {
			kind = "regular"
		}
		if _, err := pool.Exec(ctx, `
			insert into bank_category (bank_id, title, canonical_category_id, kind, emoji, is_custom)
			select b.id, $2,
			       (select cc.id from canonical_category cc where cc.slug = nullif($3, '')),
			       $4::cashback_offer_kind, nullif($5, ''), false
			from bank b
			where b.name = $1
			on conflict (bank_id, title) do nothing`,
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
