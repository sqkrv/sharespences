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
// derived reference facts (program policy, program tiers, emoji, brand colors,
// seeded bank_category rows) are refreshed on existing rows; user-created
// custom bank_category rows are never touched — a custom row with the same
// (bank, title) always wins over the seed.
package seed

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

const asOf = "as of 2025-05 (wiki table); re-verify in the bank app"

// ВТБ's «Мультибонус» rules, read 2026-08-26. Таблица 2 gives the caps per
// package; Таблица 3.1 (in force from 26.08.2026) gives the slot counts.
const vtbAsOf = "as of 2026-08-26, официальные Условия ПЛ «Мультибонус» (Таблица 2 — лимиты, Таблица 3.1 — слоты, действует с 26.08.2026)"

// СберСпасибо rules, редакция 28.07.2026, Таблица 2 — sixteen priority rungs
// keyed on package/subscription/card, each with its own slot count and cap.
const sberAsOf = "as of 2026-07-28, официальные Правила «СберСпасибо», Таблица 2"

// Ozon Банк Ultra levels, from the bank's own pages (finance.ozon.ru/promo/ultra
// and its blog explainer, published 2026-05-26, «действительна на май 2026»).
const ozonUltraAsOf = "as of 2026-05, first-party finance.ozon.ru (промо Ultra + блог)"

// МКБ's quarterly picker as the channel reported it in two independent
// quarterly posts (#4155 for Q2 2026, #4554 for Q3 2026) — identical figures
// both times, and the 3-slot count matches the 2025-09 picker screenshots.
// ⚠️ Still channel-class: mkb.ru is behind ServicePipe and its ПЛ has never
// been fetched. The quarterly category list is (via /file/<uuid>, 2026-09-02),
// but it states no caps or slot counts — only the catalog rows below.
// The six banks promoted to Tier A on 2026-08-26 (docs/knowledge/banks/
// ru-bank-landscape.md). ⚠️ None is in the owner's wallet, so NO screenshot
// exists for any of them: titles below are document- or channel-class evidence,
// never observed picker strings. Per-bank as-of constants say which.
const otpAsOf = "as of 2026-09, официальная инструкция ОТП по МСС (матрица категорий × уровней привилегий)"
const ubrrAsOf = "as of 2026-08-01, официальные Правила ПЛ «Моя жизнь» (пп. 3.2–3.12 + Приложение №1)"
const sovcomAsOf = "as of 2026-04-21 (перекройка ПЛ и запуск подписки «Оптима»)"
const mtsAsOf = "as of 2026-08"
const pskbAsOf = "as of 2026-04"
const sinaraAsOf = "as of 2026-03"

// Карта Яндекс Про, from Yandex's own partner knowledge base (2025-12-17) and
// the ПЛ at yandex.ru/legal/card_and_pay_points.
const yandexProAsOf = "as of 2025-12-17, first-party pro.yandex.ru (база знаний Яндекс Про)"

const mkbAsOf = "as of 2026-06; квартальный список категорий (III кв. 2026) лимитов и слотов не содержит"

// Альфа-Банк's cap ladder is read off the bank's own MCCD appendix (2026-08),
// which states it verbatim: cards outside the Alfa Only / Максимум packages get
// 5000, or 7000 with an Альфа-Смарт subscription; cards inside them get 30 000.
// That supersedes the 2025-05 wiki table, which had Alfa Only at 15 000.
const alfaAsOf = "as of 2026-08 (MCCD appendix, банковский документ)"

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
			{name: "Стандартный", capValue: "5000", capScope: "total", maxCategories: 3, notes: alfaAsOf},
			// Альфа-Смарт is two products (bank's subscription page, 2026-08): a
			// set of 9 привилегий at 199 ₽/мес and one of 15 at 399 ₽ личный /
			// 499 ₽ семейный. Only the 15-privilege set carries the cashback
			// privileges — the 4th slot, the extra барабан spin, and the
			// 5000 → 7000 cap — so S is seeded at base-equivalent terms.
			// Migration 00030 renames the pre-split row to M, which preserves
			// the terms every existing subscriber already has.
			{name: "Альфа-Смарт S", paid: true, capValue: "5000", capScope: "total", maxCategories: 3,
				notes: alfaAsOf + "; набор из 9 привилегий (199 ₽/мес) — кэшбэк на базовых условиях"},
			{name: "Альфа-Смарт M", paid: true, capValue: "7000", capScope: "total", maxCategories: 4,
				notes: alfaAsOf + "; набор из 15 привилегий (399 ₽ личный / 499 ₽ семейный)"},
			// Same cap covers the Максимум package; А-Клуб (30 000 / 200 000) is
			// a further level and is not modelled yet.
			{name: "Alfa Only", paid: true, capValue: "30000", capScope: "total", maxCategories: 5, notes: alfaAsOf},
		},
	},
	{
		bank: "ВТБ", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub", opensDay: 26,
		midPeriodAdd: "locked_after_first", activation: "immediate", // one-shot (п. 3.5); +5 мин ≈ immediate
		notes: vtbAsOf + "; Отчетный период = календарный месяц (п. 1.7); бонусные рубли зачисляются на счёт 1:1, баланс обнуляется в начале периода (пп. 2.13–2.14); дополнительные категории за хранение остатков — record-only",
		tiers: []tier{
			// Names are the packages the rules themselves use (Таблица 2);
			// 00032 renames the project's earlier «Стандартный»/«Привилегия».
			// maxCategories is the STANDARD count — Таблица 3.1 adds slots for
			// a client's Уровень (+1 Серебряный / +2 Золотой on Мультикарта;
			// +2 for Изумруд/Сапфир/Рубин/Бриллиант on Привилегия-Мультикарта;
			// +2 for Private Banking on Прайм+), which is a per-period fact and
			// belongs in offer_period.max_categories_override, not here.
			{name: "Мультикарта", capValue: "3000", capScope: "total", maxCategories: 3,
				notes: vtbAsOf + "; +1 категория для Уровня «Серебряный», +2 для «Золотой»; % варьируется до 15%"},
			{name: "Привилегия-Мультикарта", capValue: "30000", capScope: "total", maxCategories: 3,
				notes: vtbAsOf + "; +2 категории для Уровней «Изумруд»/«Сапфир»/«Рубин»/«Бриллиант»"},
			{name: "Прайм+", capValue: "100000", capScope: "total", maxCategories: 3,
				notes: vtbAsOf + "; +2 категории для Уровня «Private Banking»"},
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
			// Ozon Банк Ultra is premium SERVICE, a different product from the
			// Ozon Premium subscription above — the bank's own FAQ has a
			// question devoted to the difference, and they can coexist. Four
			// levels, each raising the cashback cap and lifting the slot count
			// to 5. Per-category cap deliberately left unset: the first-party
			// pages state only the total, so cap_scope stays `total` here while
			// the base card keeps `both` (the channel reports 10 000 ₽ per
			// category, which no first-party source confirms).
			//
			// ⚠️ `paid` is true for all four, but that is only half the truth:
			// a level is EITHER bought (Бронзовый, 2 990 ₽/мес) OR earned by
			// holding a balance (2 / 3 / 6 / 12 млн ₽). is_paid_subscription is
			// a property of the tier, and here it is a property of how a given
			// client reached it — the same gap ОТП Premium and Газпромбанк have.
			{name: "Ultra Бронзовый", paid: true, capValue: "20000", capScope: "total", maxCategories: 5,
				notes: ozonUltraAsOf + "; 2 990 ₽/мес либо бесплатно от 2 млн ₽ на счетах; 2 бизнес-зала, компенсации до 1 000 ₽"},
			{name: "Ultra Серебряный", paid: true, capValue: "30000", capScope: "total", maxCategories: 5,
				notes: ozonUltraAsOf + "; от 3 млн ₽ на счетах; 4 бизнес-зала, компенсации до 1 500 ₽"},
			{name: "Ultra Золотой", paid: true, capValue: "40000", capScope: "total", maxCategories: 5,
				notes: ozonUltraAsOf + "; от 6 млн ₽ на счетах; 6 бизнес-залов, компенсации до 2 000 ₽, персональный менеджер"},
			{name: "Ultra Платиновый", paid: true, capValue: "50000", capScope: "total", maxCategories: 5,
				notes: ozonUltraAsOf + "; от 12 млн ₽ на счетах; безлимит бизнес-залов, компенсации до 2 500 ₽, персональный менеджер"},
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
		notes: mkbAsOf + "; баллами возвращают стоимость покупок прошлого месяца от 1 ₽ (1 б = 1 ₽), выплата до 20 числа; бонусируются покупки от 300 ₽ (кроме соц. карты); платная смена категории посреди квартала, активация на следующий день; ⚠️ с 30.06.25 банк следит, чтобы на повышенные категории приходилось не более 70% всех покупок, иначе ПЛ урезают",
		tiers: []tier{
			// Стандарт and Премиальный carry the 2026 figures, confirmed twice.
			// Выгодный and Эксклюзивный are 2025-05 wiki-table rungs that no
			// 2026 source mentions — the channel describes only two groups
			// («обычные и соц. карты» vs «Премиум»). They are kept rather than
			// deleted because bank_client rows may reference them, and their
			// notes say plainly that the numbers are unverified.
			{name: "Стандарт", capValue: "3000", capScope: "total", maxCategories: 3,
				notes: mkbAsOf + "; обычные и соц. карты, 5% в 3 категориях (было 1 500 б по таблице 2025-05)"},
			{name: "Выгодный", capValue: "3000", capScope: "total",
				notes: asOf + "; ⚠️ ступень не упоминается ни в одном источнике 2026 года — лимит не перепроверен"},
			{name: "Премиальный", capValue: "20000", capScope: "total", maxCategories: 4,
				notes: mkbAsOf + "; 7% в 4 категориях"},
			{name: "Эксклюзивный", capValue: "50000", capScope: "total",
				notes: asOf + "; ⚠️ ступень не упоминается ни в одном источнике 2026 года — лимит не перепроверен"},
		},
	},
	{
		// Таблица 2 is a sixteen-rung PRIORITY ladder, not a set of products: a
		// client matching several criteria gets exactly one — the highest
		// порядковый номер — except Детская СберКарта, which is pinned to rung 1
		// however else the holder qualifies (п. 2.1.7). Seeded here are the ten
		// distinct (slots, cap) shapes; rungs sharing a shape are listed in the
		// notes rather than duplicated as rows.
		bank: "СберБанк", name: "СберСпасибо", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "points", pointsLabel: "бонусы СберСпасибо",
		// «Активация Категории» is per-period and one-shot; purchases in the
		// first 15 minutes after activating may not count (п. 2.1.9), which is
		// a delay no `activation` value expresses — recorded as immediate,
		// with the caveat in the notes.
		midPeriodAdd: "locked_after_first", activation: "immediate",
		notes: sberAsOf + "; Расчетный период = календарный месяц; начисление за каждые полные 100 ₽ с округлением вниз (п. 3.2.4); покупки в первые 15 минут после активации категории могут не учитываться (п. 2.1.9); ⚠️ курс бонуса к рублю устанавливается Уполномоченной компанией и в Правилах не зафиксирован",
		tiers: []tier{
			{name: "Детская СберКарта", capValue: "3000", capScope: "total", maxCategories: 2,
				notes: sberAsOf + "; категории назначаются банком, самостоятельная активация не требуется; приоритет 1 независимо от других критериев"},
			{name: "Карты Standart, Gold, Classic", capValue: "2000", capScope: "total", maxCategories: 3, notes: sberAsOf},
			{name: "СберКарта", capValue: "2000", capScope: "total", maxCategories: 3,
				notes: sberAsOf + "; либо владелец Платежного счета"},
			{name: "Тарифный план «СберПрайм Зарплатный»", paid: true, capValue: "2000", capScope: "total", maxCategories: 4,
				notes: sberAsOf + "; 5 000 бонусов, если зарплатные зачисления за предыдущий месяц ≥ 70 000 ₽; та же ступень — ТП «Релиз» и ТП «Патриот»"},
			{name: "Тарифный план «СберПрайм Старт»", paid: true, capValue: "5000", capScope: "total", maxCategories: 4,
				notes: sberAsOf + "; либо пакет услуг «СберПрайм Старт Зарплатный»"},
			{name: "Карты с «большими бонусами»", capValue: "5000", capScope: "total", maxCategories: 5,
				notes: sberAsOf + "; премиальные пластики (Visa Infinite/Platinum, World MasterCard Elite/Black Edition, МИР Премиальная) и Кредитная СберКарта"},
			{name: "Подписка СберПрайм", paid: true, capValue: "10000", capScope: "total", maxCategories: 5,
				notes: sberAsOf + "; та же ступень — ТП «Формула выгоды»"},
			{name: "Подписка СберПрайм+", paid: true, capValue: "15000", capScope: "total", maxCategories: 5, notes: sberAsOf},
			{name: "СберПремьер", capValue: "20000", capScope: "total", maxCategories: 5,
				notes: sberAsOf + "; пакет услуг «Премиальное обслуживание» уровней 1–3; та же ступень — «СберПервый» и уровень 5"},
			{name: "Премиальное обслуживание, уровень 4", capValue: "50000", capScope: "total", maxCategories: 5,
				notes: sberAsOf + "; 20 000 бонусов для держателя дополнительной карты, а не владельца пакета"},
			{name: "Sber Private Banking", capScope: "total", maxCategories: 6,
				notes: sberAsOf + "; лимит не установлен; категории назначаются банком, самостоятельная активация не требуется"},
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
	{
		// A SEPARATE bank row, not a second programme under Яндекс Пэй. Both are
		// products of Яндекс Банк, but `bank` here means the cashback surface a
		// user actually deals with — which is why «Яндекс Пэй» was chosen over
		// «Яндекс Банк» in the first place. Про lives in a different app, is
		// issued to a different audience, and above all carries its OWN
		// 10 000-балл pool: two separate pools only model correctly as two
		// bank_clients with independent periods, which needs two banks.
		//
		// It also sidesteps a latent bug: the S3b policy lookup resolves
		// mid_period_add/activation with `where cpb.bank_id = cl.bank_id limit 1`,
		// unordered — a second programme under one bank would make it read an
		// arbitrary row. Still worth fixing before anything else grows a second
		// programme, but nothing here depends on it.
		//
		// ⚠️ NOT a retail card: only партнёры Яндекс Про (Такси / Доставка /
		// Смена, RF citizenship) can hold it, and it doubles as their payout
		// card. ⚠️ Its «уровни» (Начальный/Базовый/Продвинутый) are
		// IDENTIFICATION tiers set by KYC depth and govern payment limits — the
		// cashback terms are identical across all three, so they are
		// deliberately not modelled as program_tier.
		bank: "Яндекс Про", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "points", pointsLabel: "Баллы Плюса", opensDay: 28,
		midPeriodAdd: "unknown", activation: "unknown",
		notes: yandexProAsOf + "; карта для исполнителей Яндекс Про, она же карта для выплат; выбор категорий с 28 числа, период — полный календарный месяц; ⚠️ кешбэк начисляется ТОЛЬКО за покупки вне сервисов Яндекса; ⚠️ балл = 1 ₽ скидки внутри сервисов Яндекса, в рубли не выводится; уровни Начальный/Базовый/Продвинутый — это ступени идентификации (лимиты хранения и трат), на кешбэк не влияют",
		tiers: []tier{
			// One tier: the card has no cashback levels at all. «Стандартный» is
			// this project's own label, as with Т-Банк's base level.
			{name: "Стандартный", capValue: "10000", capScope: "total", maxCategories: 6,
				notes: yandexProAsOf + "; до 30% в 6 категориях + постоянная строка «Все покупки»; баллы приходят в течение 21 дня, тратятся сразу"},
		},
	},
	{
		bank: "Совкомбанк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub",
		midPeriodAdd: "unknown", activation: "unknown",
		// ⚠️ periodType is «monthly cadence», NOT the calendar month: Совкомбанк
		// accounts caps per расчетный период, which runs from each card's issue
		// date. The start day belongs on bank_client.period_anchor_day (00033,
		// ADR-0009 §2); this row only says how LONG a period is. The 2026-04
		// switchover misfiring on the old РП (#4306) is the evidence the
		// boundary is real.
		notes: sovcomAsOf + "; ⚠️ расчетный период у каждого клиента свой (от даты выдачи карты) — день начала задаётся в bank_client.period_anchor_day, а не здесь; «Халва» — отдельный продукт со своей ПЛ, здесь не отражён",
		tiers: []tier{
			{name: "Стандартный", capValue: "3000", capScope: "total", maxCategories: 3,
				notes: sovcomAsOf + "; до 15% в 3 категориях, либо 1,5% на всё — режимы взаимоисключающие"},
			{name: "Подписка «Оптима»", paid: true, capValue: "5000", capScope: "total", maxCategories: 5,
				notes: sovcomAsOf + "; 399 ₽/мес; до 30% в 5 категориях, либо 3% на всё"},
		},
	},
	{
		bank: "ОТП Банк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "points", pointsLabel: "баллы ОТП",
		midPeriodAdd: "unknown", activation: "unknown",
		// ⚠️ The offered set is RANDOM after a client's first two months — the
		// catalog below is a POOL no client ever sees whole (ADR-0009 §1).
		notes: otpAsOf + "; баллы 1:1 в рубли от 500 б, начисление по 10 числам; ⚠️ покупки округляются вниз до 100 ₽ — покупка за 99 ₽ не приносит ничего; ⚠️ после первых двух месяцев набор предлагаемых категорий случаен, каталог — это пул; лимит общий на все карты клиента, но дебетовая и кредитная не суммируются (в seed — дебетовые лимиты)",
		tiers: []tier{
			{name: "ОТП Карта", capValue: "3000", capScope: "total", maxCategories: 4,
				notes: otpAsOf + "; кредитная карта того же уровня — 1 000 б"},
			{name: "Зарплатный клиент", capValue: "5000", capScope: "total", maxCategories: 4,
				notes: otpAsOf + "; статус подтверждается зачислением от 1 ₽ до 19 числа прошлого месяца; кредитная — 1 000 б"},
			{name: "Premium Light", capValue: "5000", capScope: "total",
				notes: otpAsOf + "; ⚠️ число категорий не указано в документе; кредитная — 1 000 б"},
			{name: "Premium", paid: true, capValue: "20000", capScope: "total", maxCategories: 8,
				notes: otpAsOf + "; ⚠️ отдельный набор категорий, а не расширенный — пять строк доступны только здесь, и восемнадцать базовых недоступны; какие именно предлагают, зависит от остатка (≥2 млн ₽ → Супермаркеты и Искусство); кредитная — 3 000 б"},
			{name: "Private", paid: true, capValue: "100000", capScope: "total",
				notes: otpAsOf + "; ⚠️ число категорий не указано в документе; в канале эта ступень не упоминается вовсе; кредитная — 3 000 б"},
		},
	},
	{
		bank: "МТС Деньги", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "points", pointsLabel: "баллы МТС",
		midPeriodAdd: "unknown", activation: "unknown",
		// ⚠️ Not МТС Банк — a different credit institution. The card is issued
		// by Экси Банк, and it is Экси Банк that runs this picker.
		notes: mtsAsOf + "; карту выпускает Экси Банк (не МТС Банк — это другая кредитная организация); ⚠️ баллы тратятся только в экосистеме МТС и только при активной подписке — для большинства клиентов они НЕ превращаются в рубли; части клиентов доступна компенсация покупок от 500 ₽ (1 б = 1 ₽), и это единственный путь обратно в деньги",
		tiers: []tier{
			{name: "Стандартный", capValue: "10000", capScope: "both", capPerCategory: "1000", maxCategories: 5,
				notes: mtsAsOf + "; до 30% в сервисах МТС, до 20% в остальных категориях; подписка МТС Premium 99 ₽ первый месяц, далее 349 ₽/мес"},
		},
	},
	{
		bank: "УБРиР", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub", opensDay: 25,
		midPeriodAdd: "locked_after_first", activation: "immediate",
		// ⚠️ `activation: immediate` is an approximation the document does not
		// quite support: chosen BEFORE the period → retroactive to the 1st;
		// chosen DURING it → from the moment of confirmation (п. 3.4). Neither
		// `immediate` nor `next_day` says that. ADR-0009 §5 keeps it as text.
		notes: ubrrAsOf + "; карта «Моя Жизнь»; выбор открывается 25 числа, подтверждённые категории изменению не подлежат (п. 3.5), без выбора кешбэка нет вовсе кроме Транспорта и Онлайн-покупок (п. 3.11); ⚠️ активация: выбрано до начала месяца → с 1 числа, выбрано внутри месяца → с момента подтверждения; ⚠️ кешбэк только при обороте от 5 000 ₽/мес (ЖКХ в оборот не входит) и с покупок от 200 ₽; ⚠️ у части строк условие ежедневного остатка от 10 000 ₽ в течение 20 дней, а при нехватке остатка банк выбирает оплачиваемую категорию СЛУЧАЙНО (п. 3.12)",
		tiers: []tier{
			{name: "Базовый", capValue: "2100", capScope: "both", capPerCategory: "500", maxCategories: 3,
				notes: ubrrAsOf + "; ⚠️ банк устанавливает 3 ИЛИ 4 категории на каждый расчетный месяц (пп. 3.2, 3.7) — здесь базовое значение, фактическое пишется в offer_period.max_categories_override"},
			{name: "Подписка «Моя жизнь+»", paid: true, capValue: "4100", capScope: "both", capPerCategory: "500", maxCategories: 3,
				notes: ubrrAsOf + "; 299 ₽/мес; открывает 10% на онлайн-покупки (до 600 ₽/мес) — эффективные 7,5% после вычета подписки"},
		},
	},
	{
		bank: "Примсоцбанк", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "rub",
		midPeriodAdd: "unknown", activation: "unknown",
		notes: pskbAsOf + "; выплата до 15 числа, учёт по дате покупки; ⚠️ покупки округляются до 100 ₽ (кроме общественного транспорта); ⚠️ с 2026-05 весь кешбэк умножается на 0,8 — заявленная ставка не равна выплачиваемой; список категорий ОДИНАКОВ для всех клиентов, что среди банков wiki редкость",
		tiers: []tier{
			{name: "Стандартный", capValue: "2500", capScope: "both", capPerCategory: "1000", maxCategories: 4,
				notes: pskbAsOf + "; ставки 3–10%; соцкарта получает повышение в отдельных категориях (аптеки, супермаркеты, книги), а не общий множитель; per-category лимит действует лишь на часть строк"},
			{name: "Премиальный", paid: true, capValue: "7000", capScope: "both", capPerCategory: "1500", maxCategories: 4,
				notes: pskbAsOf + "; тот же лимит у зарплатных клиентов; зарплатным ежемесячно дают ещё одну категорию до 10% сверх четырёх"},
		},
	},
	{
		bank: "Банк Синара", name: "Кэшбэк", periodType: "calendar_month", selectionMode: "atomic",
		currencyKind: "points", pointsLabel: "баллы Синары",
		midPeriodAdd: "unknown", activation: "unknown",
		notes: sinaraAsOf + "; карта «Та Самая», 4 категории из 11, список одинаков для всех клиентов; ⚠️ баллы меняются на рубли 1:1 ТОЛЬКО от 1 000 б — заработанное ниже порога нереализуемо; у кредитной карты своя ПЛ (1 категория с разбавлением 70/30, новые карты не выдают)",
		tiers: []tier{
			{name: "Стандартный", capValue: "3000", capScope: "total", maxCategories: 4,
				notes: sinaraAsOf + "; до 15%; первые 3 месяца +5% по всем категориям в рамках продлеваемой акции"},
			{name: "Опция «Можно больше»", paid: true, capValue: "5000", capScope: "total", maxCategories: 4,
				notes: sinaraAsOf + "; 599 ₽/мес; ⚠️ покупает лимит, но не слоты — 4 категории на обеих ступенях"},
		},
	},
}

// Reference-only banks (wiki): no programs seeded.
// Reference-only banks (wiki): no programs seeded. СберБанк left this list on
// 2026-08-26 when its programme was read off the official СберСпасибо rules.
var extraBanks = []string{}

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
	"СберБанк":    "#21A038",
	"Т-Банк":      "#FFDD2D",
	// Tier A, promoted 2026-08-26. Read off the background of each bank's own
	// logo in web/src/assets/banks — which is the right source rather than a
	// convenient one: the color exists to tint the two-letter chip that stands
	// in for that very logo, so matching the icon's own background is what
	// makes the fallback look like a placeholder for the mark instead of an
	// unrelated square.
	"Совкомбанк":  "#213A8B",
	"ОТП Банк":    "#C3FF0B",
	"УБРиР":       "#CC163F",
	"Примсоцбанк": "#BEA980",
	"Банк Синара": "#E40134",
	// МТС Деньги's icon is a gradient (#FD2D42 → #E8001F), and a chip takes one
	// flat color — this is the midpoint of the two stops, not a value the brand
	// states anywhere.
	"МТС Деньги": "#F21630",
	"Яндекс Про": "#FFC806",
}

// Per-bank picker catalogs: the CURRENTLY selectable menu rows, from the
// taxonomy page's «Bank catalogs» section (⚠️-flagged rows there pending
// live-app re-verification). slug "" = canonical-less service/channel row —
// still ordinary kind=regular (corrected 2026-07-21: special is for
// granted bonus mechanics like Пятница/колесо, never catalog rows); emoji
// "" = inherit the canonical's.
var bankCategories = []struct {
	bank, title, slug, kind, emoji string
	inactive                       bool // seeded hidden; the insert-only active flag stays an operator control afterwards
}{
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
	// 2026-08 MCCD document rows (no screenshot yet — titles are the document's):
	{bank: "Альфа-Банк", title: "Спорт и красота у партнера", emoji: "💪"}, // WellPass partner category, new in the 2026-08 issue
	// The umbrella duplicate of Фастфуд + Кафе и рестораны: in the document
	// (likely another card product's menu row) but never seen in a picker —
	// seeded hidden so mcc-import can attach its codes while the picker and
	// resolve keep showing only the two separate rows (2026-09-01).
	{bank: "Альфа-Банк", title: "Фастфуд, кафе и рестораны", inactive: true},
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
	{bank: "Ozon Банк", title: "Вет клиники и зоомагазины", slug: "pets"}, // two words, as both bank documents write it (2026-09-02)
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
	{bank: "МКБ", title: "МКБ Путешествия", emoji: "✈️"}, // «МКБ Travel» until 2026-Q2; the bank renamed the service
	{bank: "МКБ", title: "Бургер Кинг", emoji: "🍔"},
	// The catalog is re-cut every quarter, so rows accumulate across issues
	// (a row absent this quarter is an unused search hit, a missing one
	// blocks recording). III квартал 2026 («Список категорий на выбор»
	// document, 2026-09-02): the base row carries its rate in the title;
	// special rows are gated (pension card, mortgage, «Просто» subscription,
	// Премиум / 5-й уровень, age 14–18) and their titles are the document's.
	{bank: "МКБ", title: "1% на все покупки", slug: "all-purchases"},
	{bank: "МКБ", title: "Фастфуд", slug: "fastfood"},
	{bank: "МКБ", title: "Книги и канцтовары", slug: "books"},
	{bank: "МКБ", title: "Цветы и подарки", slug: "flowers"},
	{bank: "МКБ", title: "Салоны красоты", slug: "beauty"},
	{bank: "МКБ", title: "Образование", slug: "education"},
	{bank: "МКБ", title: "Золотое Яблоко", emoji: "🍏"},
	{bank: "МКБ", title: "Аптеки и оптика", slug: "pharmacies"},
	{bank: "МКБ", title: "Кафе и рестораны", slug: "restaurants"},
	{bank: "МКБ", title: "ЦУМ", emoji: "🏬"},
	{bank: "МКБ", title: "Дом и ремонт", slug: "home-repair"},
	{bank: "МКБ", title: "АЗС в рамках подписки «Просто»", slug: "gas-stations"},

	// ─── Tier A banks promoted 2026-08-26 ───────────────────────────────────
	// ⚠️ No screenshot exists for any of these six. Titles below come from the
	// banks' own documents where marked, and from channel paraphrase otherwise
	// — the 2026-07-27 rule says a picker title must come from the app, so
	// every row here is provisional and retires on the first sighting that
	// disagrees. Seeded regardless on «prefer more rows over fewer»: a missing
	// row blocks recording a month, a surplus one is an unused search hit.

	// УБРиР — Приложение №1 to the ПЛ effective 2026-08-01. This is a POOL of
	// ~45 rows; the bank offers a restricted subset (~12 observed) each month
	// and the client picks 3–4 of those (п. 3.7). Seeding the pool is correct:
	// seeding a month's offering would make eight months of rows look absent.
	{bank: "УБРиР", title: "Оплата ЖКУ", slug: "utilities"},
	{bank: "УБРиР", title: "Салоны красоты", slug: "beauty"},
	{bank: "УБРиР", title: "Аптеки", slug: "pharmacies"},
	{bank: "УБРиР", title: "Авиабилеты", slug: "avia-tickets"},
	{bank: "УБРиР", title: "Прокат авто", slug: "car-rental"},
	{bank: "УБРиР", title: "Автозапчасти и сервисы", slug: "auto-parts"},
	{bank: "УБРиР", title: "Автомойки", slug: "auto-services"},
	{bank: "УБРиР", title: "Бизнес услуги", slug: "household-services"},
	{bank: "УБРиР", title: "Уход и уборка", slug: "household-services"},
	{bank: "УБРиР", title: "Компьютеры и сервисы", slug: "electronics"},
	{bank: "УБРиР", title: "Маркетплейсы", slug: "marketplaces"},
	{bank: "УБРиР", title: "Образование", slug: "education"},
	{bank: "УБРиР", title: "Кино", slug: "cinema"},
	{bank: "УБРиР", title: "Танцы", slug: "active-leisure"},
	{bank: "УБРиР", title: "Театр", slug: "culture"},
	{bank: "УБРиР", title: "Музыка", slug: "music"},
	{bank: "УБРиР", title: "Цветы", slug: "flowers"},
	{bank: "УБРиР", title: "АЗС и зарядные станции", slug: "gas-stations"},
	{bank: "УБРиР", title: "Охрана и безопасность", slug: "household-services"},
	{bank: "УБРиР", title: "Живопись и декор", slug: "culture"},
	{bank: "УБРиР", title: "Книги", slug: "books"},
	{bank: "УБРиР", title: "Подарки и Сувениры", slug: "souvenirs"},
	{bank: "УБРиР", title: "Творчество", slug: "hobby"},
	{bank: "УБРиР", title: "Фото", slug: "photo-video"},
	{bank: "УБРиР", title: "Гостиницы", slug: "hotels"},
	{bank: "УБРиР", title: "Украшения и аксессуары", slug: "jewelry"},
	{bank: "УБРиР", title: "Аренда лодок и яхт", slug: "active-leisure"},
	{bank: "УБРиР", title: "Медицина", slug: "medicine"},
	{bank: "УБРиР", title: "Автоперевозки", slug: "transport"},
	{bank: "УБРиР", title: "Домашние питомцы", slug: "pets"},
	{bank: "УБРиР", title: "Антиквариат", slug: "culture"},
	{bank: "УБРиР", title: "Спорттовары", slug: "sport-goods"},
	{bank: "УБРиР", title: "Активный спорт", slug: "active-leisure"},
	{bank: "УБРиР", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "УБРиР", title: "Платные дороги", slug: "toll-roads"},
	{bank: "УБРиР", title: "Такси", slug: "taxi"},
	{bank: "УБРиР", title: "Универсамы", slug: "supermarkets"},
	{bank: "УБРиР", title: "Детские товары", slug: "kids"},
	{bank: "УБРиР", title: "Магазины косметики", slug: "cosmetics"},
	// The eleven pool rows the 2026-08-31 transcription dropped — every one of
	// them wraps or carries a comma in the PDF's title cell (2026-09-02, found
	// by the mcc-import title check against the parsed appendix).
	{bank: "УБРиР", title: "Бытовая электроника и сервис", slug: "electronics"},
	{bank: "УБРиР", title: "Аттракционы и видео игры", slug: "entertainment"},
	{bank: "УБРиР", title: "Дом, Ремонт", slug: "home-repair"},
	{bank: "УБРиР", title: "Магазин беспошлинной торговли (DUTY FREE)", slug: "duty-free"},
	{bank: "УБРиР", title: "Одежда, обувь", slug: "clothes"},
	{bank: "УБРиР", title: "Аренда оборудования и инструментов", emoji: "🛠️"},
	{bank: "УБРиР", title: "Кафе, рестораны", slug: "restaurants"},
	{bank: "УБРиР", title: "ТВ, связь", slug: "telecom"},
	{bank: "УБРиР", title: "Туристические агентства и операторы", slug: "travel-agencies"},
	{bank: "УБРиР", title: "Игровые и медиа платформы, программное обеспечение", slug: "digital-goods"},
	{bank: "УБРиР", title: "Железнодорожный и водный транспорт", slug: "rail-tickets"},
	{bank: "УБРиР", title: "Остальные покупки", slug: "all-purchases"},
	// The two rows Приложение №1 lists as «не требуется выбирать» — credited
	// automatically, consuming no slot, and the only thing that still pays when
	// a client picks nothing at all (п. 3.11). They are catalog rows because
	// they appear in the appendix, not because they are pickable.
	{bank: "УБРиР", title: "Транспорт (местный и пригородный)", slug: "transport"},
	{bank: "УБРиР", title: "Онлайн-покупки", slug: "marketplaces"},
	// Single-merchant rows: their MCC is defined as «коды, присвоенные
	// <магазином>», so they are merchants, not categories — canonical-less by
	// the 2026-07-27 rule, and they carry their own emoji since there is no
	// canonical to inherit one from.
	{bank: "УБРиР", title: "Библиотека ароматов онлайн", emoji: "🕯️"},
	{bank: "УБРиР", title: "Бубль Гум онлайн", emoji: "🧸"},
	{bank: "УБРиР", title: "Издательство «МИФ» онлайн", emoji: "📚"},
	{bank: "УБРиР", title: "Мегафон онлайн", emoji: "📱"},
	{bank: "УБРиР", title: "Билайн онлайн", emoji: "📱"},
	{bank: "УБРиР", title: "Очкарик онлайн", emoji: "👓"},
	{bank: "УБРиР", title: "Котофото онлайн", emoji: "📷"},

	// ОТП Банк — the official MCC instruction's category × tier matrix
	// (сентябрь 2026). ⚠️ A POOL: after a client's first two months the bank
	// draws which rows they are offered.
	{bank: "ОТП Банк", title: "Автоуслуги", slug: "auto-services"},
	{bank: "ОТП Банк", title: "Запчасти и аксессуары", slug: "auto-parts"},
	{bank: "ОТП Банк", title: "Прокат авто и каршеринг", slug: "car-rental"},
	{bank: "ОТП Банк", title: "АЗС", slug: "gas-stations"},
	{bank: "ОТП Банк", title: "Авиабилеты", slug: "avia-tickets"},
	{bank: "ОТП Банк", title: "Аптеки", slug: "pharmacies"},
	{bank: "ОТП Банк", title: "Детские товары", slug: "kids"},
	{bank: "ОТП Банк", title: "Дом, ремонт", slug: "home-repair"},
	{bank: "ОТП Банк", title: "ЖКХ", slug: "utilities"},
	{bank: "ОТП Банк", title: "Животные", slug: "pets"},
	{bank: "ОТП Банк", title: "Здоровье и медицина", slug: "medicine"},
	{bank: "ОТП Банк", title: "Кафе и рестораны", slug: "restaurants"},
	{bank: "ОТП Банк", title: "Книги", slug: "books"},
	{bank: "ОТП Банк", title: "Творчество и хобби", slug: "hobby"},
	{bank: "ОТП Банк", title: "Маркетплейсы", slug: "marketplaces"},
	{bank: "ОТП Банк", title: "Обучение и образование", slug: "education"},
	{bank: "ОТП Банк", title: "Магазины одежды", slug: "clothes"},
	{bank: "ОТП Банк", title: "Продуктовые магазины", slug: "supermarkets"},
	{bank: "ОТП Банк", title: "Кино и развлечения", slug: "entertainment"},
	{bank: "ОТП Банк", title: "Такси", slug: "taxi"},
	{bank: "ОТП Банк", title: "Алкоголь", slug: "alcohol"},
	{bank: "ОТП Банк", title: "Отдых и путешествия", slug: "travel"},
	{bank: "ОТП Банк", title: "Транспорт", slug: "transport"},
	{bank: "ОТП Банк", title: "Часы, ювелирные изделия", slug: "jewelry"},
	{bank: "ОТП Банк", title: "Сувениры", slug: "souvenirs"},
	{bank: "ОТП Банк", title: "Цветы", slug: "flowers"},
	{bank: "ОТП Банк", title: "Фастфуд", slug: "fastfood"},
	{bank: "ОТП Банк", title: "Красота", slug: "beauty"},
	// The four base-tier rows and the paraphrased base row the 2026-08-31
	// transcription dropped or renamed (2026-09-02, found by mcc-import's
	// title check against the parsed matrix). «Спортивные товары» is
	// credit-card-only per the matrix's footnote 1.
	{bank: "ОТП Банк", title: "Спортивные товары", slug: "sport-goods"},
	{bank: "ОТП Банк", title: "Duty Free", slug: "duty-free"},
	{bank: "ОТП Банк", title: "Цифровые товары", slug: "digital-goods"},
	{bank: "ОТП Банк", title: "Бытовая техника и электроника", slug: "electronics"},
	{bank: "ОТП Банк", title: "На все покупки", slug: "all-purchases"},
	// ⚠️ Premium-only rows. They are NOT renamings of the base rows above —
	// the matrix gives them different MCC sets, and the base card cannot pick
	// them at all. Folding either pair together would erase that.
	{bank: "ОТП Банк", title: "Искусство", slug: "culture"},
	{bank: "ОТП Банк", title: "Рестораны, фастфуд", slug: "restaurants"},
	{bank: "ОТП Банк", title: "Одежда и обувь", slug: "clothes"},
	{bank: "ОТП Банк", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "ОТП Банк", title: "Такси и каршеринг", slug: "taxi"},
	// Premium-only too; pharmacies plus the medical block in one row, which
	// maps cleanly to neither canonical.
	{bank: "ОТП Банк", title: "Аптеки и медицинские услуги", emoji: "💊"},
	// A manufacturer, not a merchant and not a category — 10% at MCC 5722/5712.
	{bank: "ОТП Банк", title: "Tefal", emoji: "🍳"},

	// Примсоцбанк — ⚠️ channel paraphrase across 10 monthly posts, not picker
	// strings. Several rows are plainly the same category worded differently
	// between months; both spellings are kept rather than guessed at, since a
	// wrong merge is harder to notice than a duplicate.
	{bank: "Примсоцбанк", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "Примсоцбанк", title: "Аптеки", slug: "pharmacies"},
	{bank: "Примсоцбанк", title: "Здоровье", slug: "health"},
	{bank: "Примсоцбанк", title: "Красота", slug: "beauty"},
	{bank: "Примсоцбанк", title: "Красота, парикмахерские, салоны", slug: "beauty"},
	{bank: "Примсоцбанк", title: "Детские товары", slug: "kids"},
	{bank: "Примсоцбанк", title: "Путешествия", slug: "travel"},
	{bank: "Примсоцбанк", title: "Рестораны, кафе, закусочные, фастфуд", slug: "restaurants"},
	{bank: "Примсоцбанк", title: "Рестораны и фастфуд", slug: "restaurants"},
	{bank: "Примсоцбанк", title: "Маркетплейсы", slug: "marketplaces"},
	{bank: "Примсоцбанк", title: "Парки аттракционов, детские центры", slug: "entertainment"},
	{bank: "Примсоцбанк", title: "Яндекс+АЗС", slug: "gas-stations"},
	{bank: "Примсоцбанк", title: "Домашние животные", slug: "pets"},
	{bank: "Примсоцбанк", title: "Одежда и обувь", slug: "clothes"},
	{bank: "Примсоцбанк", title: "Образование", slug: "education"},
	{bank: "Примсоцбанк", title: "Бытовые услуги", slug: "household-services"},
	{bank: "Примсоцбанк", title: "Цветы, флористика", slug: "flowers"},
	{bank: "Примсоцбанк", title: "Муз. инструменты", slug: "hobby"},
	{bank: "Примсоцбанк", title: "Книги, канцелярия", slug: "books"},
	{bank: "Примсоцбанк", title: "Техника", slug: "electronics"},
	{bank: "Примсоцбанк", title: "Спорттовары, спорт", slug: "sport-goods"},
	{bank: "Примсоцбанк", title: "Дом и ремонт", slug: "home-repair"},
	{bank: "Примсоцбанк", title: "Развлечения, кинотеатры", slug: "entertainment"},
	// ⚠️ Priced as a flat 4 ₽ per trip from 25 ₽, not as a percentage —
	// category_offer stores a percent, so this row cannot be recorded honestly
	// until that gap is addressed (ADR-0009 fact 11).
	{bank: "Примсоцбанк", title: "Транспорт", slug: "transport"},

	// МТС Деньги — ⚠️ 11 rows, nearly all from a single 2025-08 post; the
	// thinnest catalog of the six and the most likely to be stale.
	{bank: "МТС Деньги", title: "Связь МТС", emoji: "📱"},
	{bank: "МТС Деньги", title: "Кино и развлечения", slug: "entertainment"},
	{bank: "МТС Деньги", title: "Здоровье", slug: "health"},
	{bank: "МТС Деньги", title: "Кафе и рестораны", slug: "restaurants"},
	{bank: "МТС Деньги", title: "Топливо и АЗС", slug: "gas-stations"},
	{bank: "МТС Деньги", title: "Маркетплейсы", slug: "marketplaces"},
	{bank: "МТС Деньги", title: "ЖКХ", slug: "utilities"},
	{bank: "МТС Деньги", title: "Путешествия", slug: "travel"},
	{bank: "МТС Деньги", title: "Одежда, обувь, юв. изделия и часы", slug: "clothes"},
	{bank: "МТС Деньги", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "МТС Деньги", title: "Все остальное", slug: "all-purchases"},

	// Яндекс Про — only one row is documented: «Все покупки» stands permanently
	// alongside the six chosen categories. The six themselves are picked from a
	// menu no first-party source lists, so the catalog stops here rather than
	// borrowing Яндекс Пэй's — a different card in a different app.
	{bank: "Яндекс Про", title: "Все покупки", slug: "all-purchases"},

	// Газпромбанк — the bank's own MCC appendix («Категории покупок и MCC»,
	// in force 01.08.2026): document-class titles, no screenshot yet. The
	// picker shows 5–10 of these per client, six of them «постоянные» (АЗС,
	// ЖКХ, Транспорт и такси, Кафе и рестораны, Спортивные товары, and
	// Маркетплейсы with a subscription). The 1/3/4/6 % rate is a property of
	// the card balance, not of a row (ADR-0009). Seeded 2026-09-02 so the
	// membership snapshot can import — catalog and MCC data ship in lockstep.
	{bank: "Газпромбанк", title: "АЗС", slug: "gas-stations"},
	{bank: "Газпромбанк", title: "ЖКХ", slug: "utilities"},
	// Umbrella: 4111/4121/4131/4789/7512 — taxi and carsharing inside.
	{bank: "Газпромбанк", title: "Транспорт и такси", slug: "transport"},
	{bank: "Газпромбанк", title: "Кафе и рестораны", slug: "restaurants"},
	{bank: "Газпромбанк", title: "Фастфуд", slug: "fastfood"},
	{bank: "Газпромбанк", title: "Спортивные товары", slug: "sport-goods"},
	{bank: "Газпромбанк", title: "Маркетплейсы", slug: "marketplaces"},
	{bank: "Газпромбанк", title: "Супермаркеты", slug: "supermarkets"},
	{bank: "Газпромбанк", title: "Аптеки", slug: "pharmacies"},
	{bank: "Газпромбанк", title: "Медицинские услуги", slug: "medicine"},
	{bank: "Газпромбанк", title: "Автозапчасти", slug: "auto-parts"},
	{bank: "Газпромбанк", title: "Автоуслуги", slug: "auto-services"},
	{bank: "Газпромбанк", title: "Аренда авто", slug: "car-rental"},
	{bank: "Газпромбанк", title: "Алкоголь", slug: "alcohol"},
	{bank: "Газпромбанк", title: "Детские товары", slug: "kids"},
	{bank: "Газпромбанк", title: "Дом и ремонт", slug: "home-repair"},
	{bank: "Газпромбанк", title: "Домашние животные", slug: "pets"},
	{bank: "Газпромбанк", title: "Книги", slug: "books"},
	// Stationery has no canonical and does not map cleanly to books.
	{bank: "Газпромбанк", title: "Канцтовары", emoji: "✏️"},
	{bank: "Газпромбанк", title: "Косметика", slug: "cosmetics"},
	{bank: "Газпромбанк", title: "Образование", slug: "education"},
	{bank: "Газпромбанк", title: "Одежда и обувь", slug: "clothes"},
	// Umbrella over avia/rail/hotels/tours (649 codes).
	{bank: "Газпромбанк", title: "Путешествия", slug: "travel"},
	{bank: "Газпромбанк", title: "Развлечения", slug: "entertainment"},
	{bank: "Газпромбанк", title: "Салоны красоты", slug: "beauty"},
	{bank: "Газпромбанк", title: "Техника и электроника", slug: "electronics"},
	{bank: "Газпромбанк", title: "Фото и видео", slug: "photo-video"},
	{bank: "Газпромбанк", title: "Цветы", slug: "flowers"},
	{bank: "Газпромбанк", title: "Ювелирные изделия", slug: "jewelry"},
	{bank: "Газпромбанк", title: "Дьюти-фри", slug: "duty-free"},
	// Not in the appendix: offered to new clients alongside Супермаркеты
	// (2026-08-25); the title is the channel's wording, no screenshot.
	{bank: "Газпромбанк", title: "1% на всё", slug: "all-purchases"},

	// Банк Синара — ⚠️ NO catalog. The channel covers Синара monthly but has
	// never published its menu (0 rows across 7 own picker posts); it only
	// names four rows as «стабильно доступны». Seeding those four alone would
	// imply the menu is four long. Left empty deliberately — the picker falls
	// back to canonical categories for a bank without a catalog, which is the
	// honest state until a screenshot or the ПЛ arrives.
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
			// Caps and slot counts are knowledge-derived reference facts, same
			// as the program policy above — refresh them on existing rows.
			// Without this the insert guard pins a database to whatever the
			// tiers were on first seed, so a corrected cap never lands: Alfa
			// Only shipped 15 000 ₽ against a documented 30 000 ₽ because the
			// constant was the only thing anyone changed.
			if _, err := pool.Exec(ctx, `
				update program_tier pt
				set is_paid_subscription = $4,
				    cap_value            = nullif($5, '')::numeric,
				    cap_scope            = $6::cashback_cap_scope,
				    cap_per_category     = nullif($7, '')::numeric,
				    max_categories       = nullif($8, 0),
				    notes                = nullif($9, '')
				from cashback_program cp
				         join bank b on b.id = cp.bank_id
				where pt.program_id = cp.id
				  and b.name = $1
				  and cp.name = $2
				  and pt.name = $3`,
				p.bank, p.name, t.name, t.paid, t.capValue, t.capScope,
				t.capPerCategory, t.maxCategories, t.notes); err != nil {
				return fmt.Errorf("refresh tier %s/%s: %w", p.bank, t.name, err)
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
			insert into bank_category (bank_id, title, canonical_category_id, kind, emoji, is_custom, created_by, active)
			select b.id, $2,
			       (select cc.id from canonical_category cc where cc.slug = nullif($3, '')),
			       $4::cashback_offer_kind, nullif($5, ''), false, null, $6
			from bank b
			where b.name = $1
			on conflict (bank_id, title, created_by) do update
				set canonical_category_id = excluded.canonical_category_id,
				    kind                  = excluded.kind,
				    emoji                 = excluded.emoji
			where not bank_category.is_custom`,
			c.bank, c.title, c.slug, kind, c.emoji, !c.inactive); err != nil {
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
	// bank.name is unique since 00031, so this is both race-free and the
	// database's own guarantee rather than this query's. The previous
	// `where not exists` form let two concurrent seed runs both pass the check
	// and both insert.
	_, err := pool.Exec(ctx, `
		insert into bank (name)
		values ($1)
		on conflict (name) do nothing`, name)
	if err != nil {
		return fmt.Errorf("seed bank %s: %w", name, err)
	}
	return nil
}
