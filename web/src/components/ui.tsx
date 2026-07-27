import type { ReactNode } from "react";
import { ApiError } from "../api/client";

// Components mirror the Claude Design «Кэшбеки - Модуль» idiom: srf cards
// with brd borders, the accent gradient for the primary action, mint for
// ruble cashback, lilac (accl) for points, gold for спец-предложения.

export function Card({ children, className = "" }: { children: ReactNode; className?: string }) {
  return <div className={`rounded-2xl border border-brd bg-srf ${className}`}>{children}</div>;
}

export function Section({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section>
      {title && <h2 className="mx-0.5 mb-2 text-[11px] font-semibold tracking-wide text-tx3">{title}</h2>}
      {children}
    </section>
  );
}

export function Btn({
  children,
  variant = "primary",
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "ghost" | "danger" | "soft" }) {
  const styles = {
    primary: "grad-acc text-white shadow-[0_6px_16px_-8px_rgba(139,111,255,.9)] disabled:opacity-40",
    soft: "bg-acc/15 text-accl disabled:opacity-40",
    ghost: "bg-srf2 border border-brd2 text-tx2 disabled:opacity-40",
    danger: "bg-warn/10 text-warn disabled:opacity-40",
  }[variant];
  return (
    <button
      {...props}
      className={`rounded-xl px-3.5 py-2 text-sm font-semibold transition active:scale-[.98] disabled:cursor-not-allowed ${styles} ${props.className ?? ""}`}
    >
      {children}
    </button>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-[10px] font-medium uppercase tracking-[.06em] text-tx4">{label}</span>
      {children}
    </label>
  );
}

const inputCls =
  "w-full rounded-xl border border-brd2 bg-srf2 px-3 py-2.5 text-sm font-medium text-tx placeholder:text-tx4 focus:border-acc focus:outline-none";

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`${inputCls} ${props.className ?? ""}`} />;
}

export function Select(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={`${inputCls} ${props.className ?? ""}`} />;
}

export function Badge({ children, tone = "slate" }: { children: ReactNode; tone?: "slate" | "amber" | "green" | "indigo" }) {
  const tones = {
    slate: "bg-inset text-tx3",
    amber: "bg-gold/10 text-gold border border-gold/25",
    green: "bg-mint/15 text-mint",
    indigo: "bg-acc/15 text-accl",
  }[tone];
  return <span className={`inline-block rounded-lg px-2 py-0.5 text-[10.5px] font-semibold ${tones}`}>{children}</span>;
}

// Segmented control — the design's Категории/Карты and Рядом/Поиск/Категория.
export function SegTabs<T extends string>({
  value,
  onChange,
  options,
}: {
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: string }[];
}) {
  return (
    <div className="flex gap-1 rounded-2xl border border-brd2 bg-srf2 p-1">
      {options.map((o) => (
        <button
          key={o.value}
          type="button"
          onClick={() => onChange(o.value)}
          className={`flex-1 rounded-[10px] py-2 text-center text-[12.5px] transition ${
            value === o.value
              ? "grad-acc font-bold text-white shadow-[0_6px_16px_-8px_rgba(139,111,255,.9)]"
              : "font-semibold text-tx3"
          }`}
        >
          {o.label}
        </button>
      ))}
    </div>
  );
}

// Bank logos are dropped into src/assets/banks/<slug>.{svg,png} (see the
// README there) and picked up at build time — no per-file import to edit, and
// a missing file simply falls back to the two-letter avatar below. Vite
// hashes and emits them, so the PWA precache covers them offline.
const LOGO_FILES = import.meta.glob("../assets/banks/*.{svg,png}", {
  eager: true,
  query: "?url",
  import: "default",
}) as Record<string, string>;

const BANK_LOGOS: Record<string, string> = Object.fromEntries(
  Object.entries(LOGO_FILES).map(([path, url]) => [path.split("/").pop()!.replace(/\.(svg|png)$/, ""), url]),
);

// Seeded bank.name → asset slug. Keep in sync with the seed's bank list
// (internal/seed/seed.go) and the assets README.
const BANK_SLUG: Record<string, string> = {
  "Альфа-Банк": "alfabank",
  ВТБ: "vtb",
  "Озон Банк": "ozon",
  "Яндекс Пэй": "yandex-pay",
  Газпромбанк: "gazprombank",
  МКБ: "mkb",
  Сбербанк: "sber",
  "Т-Банк": "tbank",
};

export function bankLogo(name: string): string | undefined {
  const slug = BANK_SLUG[name];
  return slug ? BANK_LOGOS[slug] : undefined;
}

// The bank's own mark, rendered as-is with no plaque behind it (owner
// 2026-07-27) — logos are supplied in their self-contained app-icon form, so
// they carry their own background. Banks without a file keep the two-letter
// avatar («АБ», «ОЗ»…), lilac on soft accent or tinted with the brand color.
export function BankBadge({ name, size = 33, color }: { name: string; size?: number; color?: string | null }) {
  const logo = bankLogo(name);
  if (logo) {
    return (
      <img
        src={logo}
        alt={name}
        width={size}
        height={size}
        loading="lazy"
        className="flex-none rounded-[10px] object-contain"
        style={{ width: size, height: size }}
      />
    );
  }
  const style: React.CSSProperties = { width: size, height: size, fontSize: Math.max(8, Math.round(size / 3)) };
  if (color) {
    style.background = `${color}26`; // ~15% alpha tint
    style.color = color;
  }
  return (
    <span className="flex flex-none items-center justify-center rounded-[10px] bg-acc/15 font-bold text-accl" style={style}>
      {bankAbbrev(name)}
    </span>
  );
}

const BANK_ABBREV: Record<string, string> = {
  "Альфа-Банк": "АБ",
  ВТБ: "ВТ",
  "Озон Банк": "ОЗ",
  "Яндекс Пэй": "ЯП",
  Газпромбанк: "ГП",
  МКБ: "МК",
  Сбербанк: "СБ",
  "Т-Банк": "ТБ",
};

export function bankAbbrev(name: string): string {
  if (BANK_ABBREV[name]) return BANK_ABBREV[name];
  const words = name.split(/[\s-]+/).filter(Boolean);
  if (words.length >= 2) return (words[0][0] + words[1][0]).toUpperCase();
  return name.slice(0, 2).toUpperCase();
}

// Percent colored by currency: mint = rubles, lilac = points (the design's
// core legend). Unknown currency stays muted.
export function Pct({
  percent,
  currency,
  className = "",
}: {
  percent?: string | null;
  currency?: string;
  className?: string;
}) {
  const color = currency === "rub" ? "text-mint" : currency === "points" ? "text-accl" : "text-tx4";
  return (
    <span className={`font-extrabold ${color} ${className}`}>{percent != null ? `${percent}%` : "—%"}</span>
  );
}

// The mint check-in-circle used for selected menu rows.
export function CheckDot({ checked }: { checked: boolean }) {
  if (!checked) {
    return <span className="h-[21px] w-[21px] flex-none rounded-full border-[1.5px] border-dash" />;
  }
  return (
    <span className="flex h-[21px] w-[21px] flex-none items-center justify-center rounded-full bg-mint/15">
      <svg width="11" height="11" viewBox="0 0 24 24" fill="none" stroke="var(--t-mint)" strokeWidth="3" strokeLinecap="round" strokeLinejoin="round">
        <path d="M5 12.5 10 17.5 19 6.5" />
      </svg>
    </span>
  );
}

// Filled accent-gradient hero card (tier header, «платите этой картой»).
export function GradientCard({ children, className = "" }: { children: ReactNode; className?: string }) {
  return (
    <div className={`grad-acc relative overflow-hidden rounded-[20px] text-white ${className}`}>
      <div className="pointer-events-none absolute -top-6 -right-4 h-28 w-28 rounded-full bg-white/10" />
      <div className="relative">{children}</div>
    </div>
  );
}

export function ErrMsg({ error }: { error: unknown }) {
  if (!error) return null;
  const msg = error instanceof ApiError ? error.message : error instanceof Error ? error.message : String(error);
  return <p className="mt-2 rounded-xl bg-warn/10 px-3 py-2 text-sm font-medium text-warn">{msg}</p>;
}

export function Spinner() {
  return <p className="p-6 text-center text-sm font-medium text-tx4">Загрузка…</p>;
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-xl border border-brd bg-srf px-3 py-4 text-center text-sm font-medium text-tx3">{children}</p>
  );
}
