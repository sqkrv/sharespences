// Small display helpers shared by screens. Domain language is Russian.

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

// Static cap reference, e.g. «лимит 1500₽/кат, всего 3000₽» (Озон) or
// «лимит 7000₽» (Альфа-Смарт). Caps are configured values, not remaining.
export function capNote(e: {
  cap_value?: string;
  cap_per_category?: string;
  cap_scope?: string;
  currency_kind?: string;
  points_label?: string;
}): string {
  const unit = e.currency_kind === "points" ? ` ${e.points_label || "баллов"}` : "₽";
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

const MONTHS_NOM = ["январь", "февраль", "март", "апрель", "май", "июнь", "июль", "август", "сентябрь", "октябрь", "ноябрь", "декабрь"];
const MONTHS_GEN = ["января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"];

// «июль 2026» — the overview header chip.
export function fmtMonthYear(d = new Date()): string {
  return `${MONTHS_NOM[d.getMonth()]} ${d.getFullYear()}`;
}

// «Выбор категорий на август откроется **25 июля**» — passive display of
// the program's selection_opens_day (spec: dates shown, never pushed).
// Returns the date part separately: the design renders it bold.
export function opensStripParts(day: number, now = new Date()): { text: string; date: string } {
  let opens = new Date(now.getFullYear(), now.getMonth(), day);
  if (opens < new Date(now.getFullYear(), now.getMonth(), now.getDate())) {
    opens = new Date(now.getFullYear(), now.getMonth() + 1, day);
  }
  const target = new Date(opens.getFullYear(), opens.getMonth() + 1, 1);
  return {
    text: `Выбор категорий на ${MONTHS_NOM[target.getMonth()]} откроется`,
    date: `${day} ${MONTHS_GEN[opens.getMonth()]}`,
  };
}

// Recent months for the overview period picker: current month first,
// value = mid-month ISO date the API's ?date= accepts.
export function monthOptions(count = 12, now = new Date()): { value: string; label: string }[] {
  return Array.from({ length: count }, (_, i) => {
    const d = new Date(now.getFullYear(), now.getMonth() - i, 15);
    return { value: isoDate(d), label: fmtMonthYear(d) };
  });
}

// The month word of an ISO date («июль» for 2026-07-15).
export function monthNameOf(iso: string): string {
  return MONTHS_NOM[Number(iso.slice(5, 7)) - 1];
}
