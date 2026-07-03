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
