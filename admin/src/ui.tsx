// Shared admin-UI primitives. Hand-built Tailwind like web/src/components/
// ui.tsx, but desktop-first: real tables instead of flex rows.
import { useRef, useState, type ReactNode } from "react";
import { errorText } from "./api";

export function Card({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section className="rounded-2xl border border-brd bg-srf p-4">
      {title && <h2 className="mb-3 text-sm font-semibold text-tx2">{title}</h2>}
      {children}
    </section>
  );
}

const btnBase =
  "rounded-lg px-3 py-1.5 text-sm font-medium transition-colors disabled:cursor-not-allowed disabled:opacity-50";
const btnKind = {
  primary: "grad-acc text-white",
  soft: "bg-inset text-tx hover:bg-brd",
  ghost: "text-tx3 hover:text-tx",
  danger: "bg-inset text-warn hover:bg-brd",
};

export function Btn({
  kind = "soft",
  ...rest
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { kind?: keyof typeof btnKind }) {
  return <button type="button" {...rest} className={`${btnBase} ${btnKind[kind]} ${rest.className ?? ""}`} />;
}

export const inputCls =
  "w-full rounded-lg border border-brd bg-srf2 px-2.5 py-1.5 text-sm text-tx outline-none placeholder:text-tx4 focus:border-acc";

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block min-w-0">
      <span className="mb-1 block text-[10px] font-semibold uppercase tracking-wide text-tx3">{label}</span>
      {children}
    </label>
  );
}

export function Badge({ tone, children }: { tone: "slate" | "amber" | "green" | "warn"; children: ReactNode }) {
  const tones = {
    slate: "bg-inset text-tx3",
    amber: "bg-inset text-gold",
    green: "bg-inset text-mint",
    warn: "bg-inset text-warn",
  };
  return (
    <span className={`inline-block rounded-md px-1.5 py-0.5 text-[11px] font-medium ${tones[tone]}`}>{children}</span>
  );
}

/** The read-only banner every seed-managed screen carries (ADR-0008). */
export function SeedBanner({ what }: { what: string }) {
  return (
    <p className="rounded-xl border border-brd2 bg-srf2 px-3 py-2 text-xs text-tx3">
      🔒 {what} управляется сидом — правки здесь невозможны: их перезапишет следующий деплой. Долговечный путь — база
      знаний → seed.
    </p>
  );
}

export function ErrMsg({ error }: { error: unknown }) {
  if (!error) return null;
  return <p className="text-sm text-warn">{errorText(error)}</p>;
}

export function Spinner() {
  return <p className="p-4 text-sm text-tx3">Загрузка…</p>;
}

export function Th({ children }: { children?: ReactNode }) {
  return <th className="px-2 py-1.5 text-left text-[11px] font-semibold uppercase tracking-wide text-tx3">{children}</th>;
}

export function Td({ children, className = "" }: { children?: ReactNode; className?: string }) {
  return <td className={`px-2 py-1.5 align-top text-sm ${className}`}>{children}</td>;
}

/** Wide tables scroll inside the card, never the page. */
export function TableWrap({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="w-full border-separate border-spacing-0 [&_td]:border-t [&_td]:border-brd2">{children}</table>
    </div>
  );
}

export function Pager({
  total,
  limit,
  offset,
  onOffset,
}: {
  total: number;
  limit: number;
  offset: number;
  onOffset: (o: number) => void;
}) {
  if (total <= limit) return null;
  const page = Math.floor(offset / limit) + 1;
  const pages = Math.ceil(total / limit);
  return (
    <div className="flex items-center gap-2 pt-2 text-sm text-tx3">
      <Btn disabled={offset === 0} onClick={() => onOffset(Math.max(0, offset - limit))}>
        ←
      </Btn>
      <span>
        {page} / {pages} · всего {total}
      </span>
      <Btn disabled={offset + limit >= total} onClick={() => onOffset(offset + limit)}>
        →
      </Btn>
    </div>
  );
}

/** Debounced text input for server-side search. */
export function SearchInput({ onQuery, placeholder }: { onQuery: (q: string) => void; placeholder: string }) {
  const [v, setV] = useState("");
  const timer = useRef(0);
  return (
    <input
      className={`${inputCls} max-w-xs`}
      value={v}
      placeholder={placeholder}
      onChange={(e) => {
        const q = e.target.value;
        setV(q);
        window.clearTimeout(timer.current);
        timer.current = window.setTimeout(() => onQuery(q.trim()), 300);
      }}
    />
  );
}
