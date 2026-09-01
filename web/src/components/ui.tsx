import type { ReactNode } from "react";
import { ApiError } from "../api/client";
import { STATUS_URL } from "../lib";

// Components mirror the Claude Design «Кэшбеки - Модуль» idiom: srf cards
// with brd borders, the accent gradient for the primary action, mint for
// ruble cashback, lilac (accl) for points, gold for спец-предложения.

// Card/Section forward the rest of their props so a caller can tag the block
// with `data-sid` (dev mode — web/src/dev/), which costs one attribute and
// no wrapper element.
export function Card({ children, className = "", ...rest }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div {...rest} className={`rounded-2xl border border-brd bg-srf ${className}`}>
      {children}
    </div>
  );
}

export function Section({
  title,
  children,
  ...rest
}: { title?: string } & React.HTMLAttributes<HTMLElement>) {
  return (
    <section {...rest}>
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

// min-w-0: a Field is often a grid/flex item, and the default
// min-width:auto refuses to shrink below its content — an iOS
// input[type=date] is wider than its box, so the column would overflow
// the card (see the input rule in index.css).
export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block min-w-0">
      <span className="mb-1 block text-[10px] font-medium uppercase tracking-[.06em] text-tx4">{label}</span>
      {children}
    </label>
  );
}

// The browser's own constraint bubbles («Please fill out this field») follow
// the BROWSER locale, not the interface — an English-locale browser answers a
// Russian UI in English, which is the thing internal/i18n exists to prevent on
// the server side (2026-07-28: anything a user can read is Russian).
// setCustomValidity replaces the bubble text, and an element carries only one
// message, so it has to be recomputed from whichever constraint just failed.
type Constrained = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

// «1 символ» / «22 символа» / «8 символов».
function chars(n: number): string {
  const one = n % 10 === 1 && n % 100 !== 11;
  const few = n % 10 >= 2 && n % 10 <= 4 && (n % 100 < 12 || n % 100 > 14);
  return `${n} ${one ? "символ" : few ? "символа" : "символов"}`;
}

// `.type` reads «select-one»/«select-multiple» on a <select> and «textarea» on
// a <textarea>, so one table covers all three elements.
const MISSING: Record<string, string> = {
  "select-one": "Выберите значение",
  "select-multiple": "Выберите значение",
  checkbox: "Отметьте, чтобы продолжить",
  radio: "Выберите вариант",
  file: "Выберите файл",
  date: "Укажите дату",
  month: "Укажите месяц",
  time: "Укажите время",
};

const MISMATCH: Record<string, string> = {
  email: "Введите адрес почты — например, name@example.com",
  url: "Введите ссылку целиком — например, https://example.com",
};

// Empty when no native constraint failed. That also self-heals the one case
// onChange cannot cover: a stale custom message is itself a reason to be
// invalid, so an element left holding one after its value changed some other
// way (a controlled parent, a reset) clears it on the next validation pass.
function validityMessage(el: Constrained): string {
  const v = el.validity;
  if (v.valueMissing) return MISSING[el.type] ?? "Заполните поле";
  if (v.typeMismatch) return MISMATCH[el.type] ?? "Проверьте формат";
  // A pattern means nothing without its explanation — browsers append `title`
  // to their own message for the same reason.
  if (v.patternMismatch) return el.title || "Проверьте формат";
  if (v.tooShort && "minLength" in el) return `Не меньше ${chars(el.minLength)}`;
  if (v.tooLong && "maxLength" in el) return `Не больше ${chars(el.maxLength)}`;
  if (v.rangeUnderflow && "min" in el) return `Не меньше ${el.min}`;
  if (v.rangeOverflow && "max" in el) return `Не больше ${el.max}`;
  if (v.stepMismatch || v.badInput) return "Проверьте формат";
  return "";
}

// Fold into any element carrying a native constraint. onInvalid rewrites the
// message from the constraint that failed; onChange drops it on every edit —
// a custom message keeps the element invalid by itself, so a stale one would
// freeze a field the user has already fixed. Both chain to the caller's own
// handler, which is what a form's state is wired through.
export function validityProps<E extends Constrained>(
  props: { onInvalid?: React.FormEventHandler<E>; onChange?: React.ChangeEventHandler<E> } = {},
) {
  return {
    onInvalid: (e: React.FormEvent<E>) => {
      e.currentTarget.setCustomValidity(validityMessage(e.currentTarget));
      props.onInvalid?.(e);
    },
    onChange: (e: React.ChangeEvent<E>) => {
      e.currentTarget.setCustomValidity("");
      props.onChange?.(e);
    },
  };
}

const inputCls =
  "w-full rounded-xl border border-brd2 bg-srf2 px-3 py-2.5 text-sm font-medium text-tx placeholder:text-tx4 focus:border-acc focus:outline-none";

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} {...validityProps<HTMLInputElement>(props)} className={`${inputCls} ${props.className ?? ""}`} />;
}

export function Select(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} {...validityProps<HTMLSelectElement>(props)} className={`${inputCls} ${props.className ?? ""}`} />;
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
  sid,
}: {
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: string }[];
  sid?: string; // dev-mode region ID; the generic props rule out a props spread
}) {
  return (
    <div data-sid={sid} className="flex gap-1 rounded-2xl border border-brd2 bg-srf2 p-1">
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
  "Ozon Банк": "ozon",
  "Яндекс Пэй": "yandex-pay",
  Газпромбанк: "gazprombank",
  МКБ: "mkb",
  СберБанк: "sber",
  "Т-Банк": "tbank",
  // Tier A, promoted 2026-08-26. The SVG files are not in the repo yet, so
  // these fall back to the two-letter chip until each mark is added — which is
  // what the assets README prescribes for a partial set. The keys must exist
  // now regardless: they are keyed on the seeded bank.name, and a bank missing
  // here would never pick up its logo even once the file lands.
  Совкомбанк: "sovkom",
  "ОТП Банк": "otp",
  "МТС Деньги": "mtsmoney",
  УБРиР: "ubrr",
  Примсоцбанк: "pskb",
  "Банк Синара": "sinara",
  "Яндекс Про": "yandexpro",
};

export function bankLogo(name: string): string | undefined {
  const slug = BANK_SLUG[name];
  return slug ? BANK_LOGOS[slug] : undefined;
}

// The bank's own mark, rendered as-is with no plaque behind it (2026-07-27)
// — logos are supplied in their self-contained app-icon form, so
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
  "Ozon Банк": "ОЗ",
  "Яндекс Пэй": "ЯП",
  Газпромбанк: "ГП",
  МКБ: "МК",
  СберБанк: "СБ",
  "Т-Банк": "ТБ",
  // Tier A, 2026-08-26. bankAbbrev() derives two letters on its own, but its
  // rule takes the first letter of the first two words — which gives «БС» for
  // «Банк Синара» and «МД» for «МТС Деньги». Spelled out where the derived
  // pair would read wrong.
  Совкомбанк: "СК",
  "ОТП Банк": "ОТ",
  "МТС Деньги": "МТ",
  УБРиР: "УБ",
  Примсоцбанк: "ПС",
  "Банк Синара": "СН",
  // «ЯП» is already Яндекс Пэй's; the distinguishing word is what the chip
  // has to carry, or the two banks are indistinguishable in the list.
  "Яндекс Про": "ПР",
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
export function GradientCard({ children, className = "", ...rest }: React.HTMLAttributes<HTMLDivElement>) {
  return (
    <div {...rest} className={`grad-acc relative overflow-hidden rounded-[20px] text-white ${className}`}>
      <div className="pointer-events-none absolute -top-6 -right-4 h-28 w-28 rounded-full bg-white/10" />
      <div className="relative">{children}</div>
    </div>
  );
}

// The API answers in Russian (internal/i18n), so an ApiError is shown as-is.
// What reaches here in English is the transport layer — a dropped connection
// surfaces as the browser's own «Failed to fetch»/«Load failed» TypeError —
// and genuine JS bugs, which get a neutral line while the original goes to
// the console for debugging (2026-07-28: errors follow the UI language).
// A request that never reached the server fails as a TypeError («Failed to
// fetch» / «Load failed»), which is also the one case where the answer is not
// in the app: ErrMsg turns it into a link to the status page.
export function isNetworkError(error: unknown): boolean {
  return error instanceof TypeError;
}

export function errorText(error: unknown): string {
  if (error instanceof ApiError) return error.message;
  if (isNetworkError(error)) return "Сервер не отвечает — проверь интернет.";
  if (error instanceof Error) {
    console.error(error);
    return "Что-то пошло не так — попробуй ещё раз";
  }
  console.error(error);
  return "Что-то пошло не так — попробуй ещё раз";
}

export function ErrMsg({ error }: { error: unknown }) {
  if (!error) return null;
  return (
    <p className="mt-2 rounded-xl bg-warn/10 px-3 py-2 text-sm font-medium text-warn">
      {errorText(error)}
      {isNetworkError(error) && (
        <>
          {" "}
          <a href={STATUS_URL} target="_blank" rel="noopener noreferrer" className="font-bold underline underline-offset-2">
            Статус сервиса ↗
          </a>
        </>
      )}
    </p>
  );
}

export function Spinner() {
  return <p className="p-6 text-center text-sm font-medium text-tx4">Загрузка…</p>;
}

export function Empty({ children }: { children: ReactNode }) {
  return (
    <p className="rounded-xl border border-brd bg-srf px-3 py-4 text-center text-sm font-medium text-tx3">{children}</p>
  );
}
