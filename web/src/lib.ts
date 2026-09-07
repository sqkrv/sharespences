import type { LookupEntry, Schemas } from "./api/client";

type CategoryGroup = Schemas["OverviewCategoryDTO"];

// Small display helpers shared by screens. Domain language is Russian.

// Latin lowercase letters that are pixel-identical to Cyrillic ones — real
// Альфа titles mix them into Cyrillic words. Mirrors the backend's
// NormalizeTitle fold (internal/cashback/domain.go).
const HOMOGLYPHS: Record<string, string> = { a: "а", c: "с", e: "е", o: "о", p: "р", x: "х", y: "у" };

// normalizeTitle canonicalizes a category title for client-side search
// filtering: NFC, lower, ё→е, Latin→Cyrillic homoglyph fold, collapsed
// whitespace. Must stay in sync with the backend rule.
export function normalizeTitle(s: string): string {
  return s
    .normalize("NFC")
    .toLowerCase()
    .replaceAll("ё", "е")
    .replace(/[aceopxy]/g, (ch) => HOMOGLYPHS[ch])
    .split(/\s+/)
    .filter(Boolean)
    .join(" ");
}

export function pad2(n: number): string {
  return String(n).padStart(2, "0");
}

// Local-date ISO (never UTC-shifted): 2026-07-01
export function isoDate(d: Date): string {
  return `${d.getFullYear()}-${pad2(d.getMonth() + 1)}-${pad2(d.getDate())}`;
}

export function todayISO(): string {
  return isoDate(new Date());
}

export function monthRange(now = new Date()): { start: string; end: string } {
  return {
    start: isoDate(new Date(now.getFullYear(), now.getMonth(), 1)),
    end: isoDate(new Date(now.getFullYear(), now.getMonth() + 1, 0)),
  };
}

export function quarterRange(now = new Date()): { start: string; end: string } {
  const q = Math.floor(now.getMonth() / 3);
  return {
    start: isoDate(new Date(now.getFullYear(), q * 3, 1)),
    end: isoDate(new Date(now.getFullYear(), q * 3 + 3, 0)),
  };
}

export function fmtDate(iso: string): string {
  const [y, m, d] = iso.split("-");
  return `${d}.${m}.${y}`;
}

export function fmtRange(start: string, end: string): string {
  return `${fmtDate(start)} – ${fmtDate(end)}`;
}

export function coversToday(start: string, end: string): boolean {
  const t = todayISO();
  return start <= t && t <= end;
}

// Currency badge text: ₽ or the program's points label (invariant 5: badge
// only, never cross-currency math).
export function currencyBadge(kind?: string, pointsLabel?: string): string {
  if (kind === "points") return pointsLabel || "баллы";
  if (kind === "rub") return "₽";
  return "?";
}

// Long form of the same fact, for the best-card headline. A tier-less bank
// client has currency_kind=unknown — say so rather than defaulting to
// «баллами», which reads as a claim the data does not support.
export function currencyWord(kind?: string, pointsLabel?: string): string {
  if (kind === "points") return pointsLabel || "баллами";
  if (kind === "rub") return "рублями";
  return "неизвестно чем";
}

// Monogram for a partner-offer avatar. Bank offers are habitually written
// with the rate in front («25% в Авито»), so taking title[0] renders a digit
// — the tile showed «2» for Авито. Skip leading digits, punctuation and the
// «в»/«в » connector to reach the merchant's own first letter, and fall back
// to the raw first character when there is no letter at all.
export function merchantMonogram(title: string): string {
  const words = title.split(/[\s·—–-]+/).filter(Boolean);
  for (const w of words) {
    const cleaned = w.replace(/^[^\p{L}]+/u, "");
    if (!cleaned) continue;
    const lower = cleaned.toLowerCase();
    if (lower === "в" || lower === "на" || lower === "от" || lower === "до") continue;
    return cleaned[0].toUpperCase();
  }
  return title.trim()[0]?.toUpperCase() ?? "?";
}

// Static cap reference, e.g. «лимит 1500₽/кат, всего 3000₽» (Озон) or
// «лимит 7000₽» (Альфа-Смарт). Caps are configured values, not remaining.
// A per-offer cap (ВТБ «Кешбэк до N ₽» rows) wins over the tier cap.
export function capNote(e: {
  cap_value?: string;
  cap_per_category?: string;
  cap_scope?: string;
  currency_kind?: string;
  points_label?: string;
  offer_cap_value?: string;
}): string {
  const unit = e.currency_kind === "points" ? ` ${e.points_label || "баллов"}` : "₽";
  if (e.offer_cap_value) return `лимит ${e.offer_cap_value}${unit}`;
  switch (e.cap_scope) {
    case "per_category":
      return e.cap_per_category ? `лимит ${e.cap_per_category}${unit}/кат` : "";
    case "both":
      return e.cap_per_category && e.cap_value
        ? `лимит ${e.cap_per_category}${unit}/кат, всего ${e.cap_value}${unit}`
        : "";
    default:
      return e.cap_value ? `лимит ${e.cap_value}${unit}` : "";
  }
}

export function fmtPercent(p?: string): string {
  return p != null ? `${p}%` : "—%";
}

export const MONTHS_NOM = ["январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"];
export const MONTHS_GEN = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"];

// «июль 2026» — the overview header chip.
export function fmtMonthYear(d = new Date()): string {
  return `${MONTHS_NOM[d.getMonth()]} ${d.getFullYear()}`;
}


// «III квартал» — a quarter-aligned period named on the bank card (2e): a
// three-month menu shown under a month chip needs saying why it spans the
// quarter (МКБ). Non-quarter ranges return "".
export function quarterNote(start?: string, end?: string): string {
  if (!start || !end || start.slice(0, 4) !== end.slice(0, 4)) return "";
  const sm = Number(start.slice(5, 7));
  if (sm % 3 !== 1 || Number(end.slice(5, 7)) !== sm + 2) return "";
  return `${["I", "II", "III", "IV"][(sm - 1) / 3]} квартал`;
}

export const MONTHS_SHORT = ["янв", "фев", "мар", "апр", "май", "июн", "июл", "авг", "сен", "окт", "ноя", "дек"];

// Логин rules, mirrored from internal/auth/domain.go for instant feedback in
// the fields. The server normalizes and validates independently and stays
// authoritative — this side exists so a typo is caught before a round trip.
export const USERNAME_PATTERN = "^[a-z][a-z0-9]*([._][a-z0-9]+)*$";
export const USERNAME_MIN = 3;
export const USERNAME_MAX = 32;
export const USERNAME_HINT =
  "3–32 символа: строчные латинские буквы и цифры, «.» и «_» внутри, начинается с буквы";

// normalizeUsername matches the backend normalizer: trim, drop one leading «@»
// (every screen renders the login as «@anna», so that prefix travels with it
// when someone copies or dictates it), lowercase.
export function normalizeUsername(s: string): string {
  return s.trim().replace(/^@/, "").toLowerCase();
}

// Mid-month ISO — the ?date= value the overview API samples a month by.
export function midMonthISO(year: number, month0: number): string {
  return `${year}-${pad2(month0 + 1)}-15`;
}

// "2026-07" month key of an ISO date.
export function monthKey(iso: string): string {
  return iso.slice(0, 7);
}

// The month word of an ISO date («июль» for 2026-07-15).
export function monthNameOf(iso: string): string {
  return MONTHS_NOM[Number(iso.slice(5, 7)) - 1];
}

// Genitive month of an ISO date («августа») — «Меню августа» headlines.
export function monthGenOf(iso: string): string {
  return MONTHS_GEN[Number(iso.slice(5, 7)) - 1];
}

// Months (0-based) the recognizer's verbatim period_texts hint at: «на
// август», «май», «до 31.07», «01.08–31.08». Used only to WARN when the
// screenshots disagree with the month being filled (3b) — never to change
// it silently; the strings stay unparsed on the server by design.
export function parseMonthHints(texts: string[]): number[] {
  const out = new Set<number>();
  for (const raw of texts) {
    const t = raw.toLowerCase();
    MONTHS_NOM.forEach((nom, i) => {
      // One stem covers «август/августа/августе»; ь-months stem the same
      // way («сентябр…»); «май» declines off a 2-letter stem, so its three
      // forms are spelled out rather than matching «март» by accident.
      const stems = nom === "май" ? ["май", "мая", "мае"] : [nom.endsWith("ь") ? nom.slice(0, -1) : nom];
      if (stems.some((s) => t.includes(s))) out.add(i);
    });
    for (const m of t.matchAll(/\b\d{1,2}\.(\d{2})(?:\.\d{2,4})?\b/g)) {
      const mm = Number(m[1]);
      if (mm >= 1 && mm <= 12) out.add(mm - 1);
    }
  }
  return [...out];
}

// «1 скрин / 2 скрина / 5 скринов» — the Russian numeral triad.
export function plural(n: number, one: string, few: string, many: string): string {
  const oneCase = n % 10 === 1 && n % 100 !== 11;
  const fewCase = n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14);
  return oneCase ? one : fewCase ? few : many;
}

// Human labels of point_of_sale.type — shared by the search rows and the
// «О точке» card.
// Provenance mark of a точка (origin, 00027). mcc-codes.ru rows carry the
// blanket license credit and get NO mark; everything people wrote here is
// marked, which is what keeps that credit exactly true on a mixed base.
export const POS_ORIGIN_MARK: Record<string, string> = {
  user_manual: "от пользователей",
  user_transaction: "от пользователей",
  admin: "Sharespences",
};

export const POS_TYPE_RU: Record<string, string> = {
  offline: "офлайн-точка",
  online: "онлайн",
  app: "приложение",
  other: "другое",
};

// Category icon fallback — a canonical category may carry no emoji yet
// (custom rows, un-curated additions); the icon column still aligns.
export const FALLBACK_EMOJI = "🏷️";

// S3b verdict copy — fact-based states, never guesses (spec S3b). Shared by
// the feed's dashed rows, the lookup's «Можно выбрать» section and the
// lookup boards' «В меню, но не выбрано» block. The commonest verdict —
// free, right now — says nothing at all: the dashed style already means
// «можно выбрать», so only the exceptions get words (feedback 2026-08-25).
// The two blocked verdicts do carry words: their rows are dim and reasonless
// otherwise (2026-08-28; the feed still never receives them).
export function verdictNote(e: { verdict: string; kind?: string; activation?: string }): string {
  const parts: string[] = [];
  switch (e.verdict) {
    case "free":
      if (e.kind === "super") parts.push("барабан — не занимает слот");
      break;
    case "paid":
      parts.push("платно");
      break;
    case "slots_full":
      parts.push("слоты заняты");
      break;
    case "locked":
      parts.push("выбор закрыт");
      break;
    default:
      parts.push("правила неизвестны");
  }
  // «активация завтра» is advice for a pick still ahead of you — a blocked
  // row has none, so the note would only muddle its reason.
  if (e.activation === "next_day" && e.verdict !== "slots_full" && e.verdict !== "locked") parts.push("активация завтра");
  return parts.join(" · ");
}

// «Карты друзей» in rankings (friends-sharing FR-S4) — persisted under the
// policy-listed lookup-friends key (privacy.html §3.2). Default on: the
// shared card is the feature's whole point. The feed reads the same key —
// with friends off a row falls back to the own winner, it never vanishes.
export const FRIENDS_KEY = "lookup-friends";
export function initWithFriends(): boolean {
  return localStorage.getItem(FRIENDS_KEY) !== "off";
}

// Public status page (Uptime Kuma, deliberately on separate infrastructure —
// it has to answer when sharespences.com does not). Linked from the offline
// chip and from network-failure messages; never fetched, embedded or
// preconnected, because privacy policy §2.4 states the app loads no
// third-party resources — an outbound link the user chooses to follow is not
// one, a request the page makes on its own would be.
export const STATUS_URL = "https://status.sharespences.com";

// The unit is a noun the user typed, and Russian genitive plural has no rule
// that survives arbitrary input — «поездка» loses its ending AND gains a fill
// vowel (поездок), «преференция» does something else again. Guessing produced
// «5 из 5 поездкок».
//
// So: the handful of nouns this domain actually uses are spelled out, and
// anything else is left in the nominative rather than manufactured. A slightly
// stiff «5 из 5 виза» beats a word that does not exist.
const UNITS: Record<string, [string, string, string]> = {
  поездка: ["поездка", "поездки", "поездок"],
  преференция: ["преференция", "преференции", "преференций"],
  проход: ["проход", "прохода", "проходов"],
  балл: ["балл", "балла", "баллов"],
  посещение: ["посещение", "посещения", "посещений"],
  компенсация: ["компенсация", "компенсации", "компенсаций"],
  билет: ["билет", "билета", "билетов"],
  день: ["день", "дня", "дней"],
};

export function unitWord(unit: string, n: number): string {
  const forms = UNITS[unit.trim().toLowerCase()];
  if (!forms) return unit;
  const t = n % 10;
  const h = n % 100;
  if (t === 1 && h !== 11) return forms[0];
  if (t >= 2 && t <= 4 && (h < 12 || h > 14)) return forms[1];
  return forms[2];
}

export function currencyRank(kind?: string): number {
  return kind === "rub" ? 0 : kind === "points" ? 1 : 2;
}

export function pctNum(p?: string | null): number {
  return p != null ? parseFloat(p) : -1;
}

// The row's displayed winner: the best rate the row can honestly show —
// 9a's own rule, «передний логотип — банк с максимальным процентом, ему и
// принадлежит цифра». A friend's 9% must not front a row that has a
// still-pickable 10% below it (feedback 2026-08-28). Ties resolve by the
// least action needed: an own selected card already pays, a friend's needs
// asking, a «свободный слот» needs picking first.
export function winnerOf(g: CategoryGroup, friendsOn: boolean): { entry: LookupEntry; state: "friend" | "own" | "available" | "friend-available" } | null {
  const candidates: { entry: LookupEntry; state: "friend" | "own" | "available"; prio: number }[] = [];
  if (g.best) candidates.push({ entry: g.best, state: "own", prio: 0 });
  if (friendsOn && g.friend_best) candidates.push({ entry: g.friend_best, state: "friend", prio: 1 });
  if (g.available) candidates.push({ entry: g.available, state: "available", prio: 2 });
  if (candidates.length === 0) {
    // Nothing of the viewer's own, and no friend has picked here — but a
    // friend still holds the category unpicked. It fronts the row rather than
    // dropping it, and only here: a rate nobody has taken must never outrank
    // a card that already pays.
    if (friendsOn && g.friend_available) return { entry: g.friend_available, state: "friend-available" };
    return null;
  }
  // Currency first, exactly as the server ranks and as the row expansion
  // now sorts: a friend's 9% в баллах must not headline a row whose list
  // opens on a 4% в рублях. Percent still decides inside a currency, and the
  // prio tiebreak still favours the card that needs the least action.
  candidates.sort(
    (a, b) =>
      currencyRank(a.entry.currency_kind) - currencyRank(b.entry.currency_kind) ||
      pctNum(b.entry.percent) - pctNum(a.entry.percent) ||
      a.prio - b.prio,
  );
  return candidates[0];
}
