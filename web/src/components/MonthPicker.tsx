import { useEffect, useRef, useState } from "react";
import { MONTHS_SHORT, fmtMonthYear, midMonthISO, monthKey, pad2, todayISO } from "../lib";

// The overview header's period picker: a chip («июль 2026 ▾») opening a
// year-navigable month grid. ANY month up to next month is selectable — you
// must be able to open a month with no data yet to backfill it (owner
// 2026-07-15). A dot marks months that already have offer periods; only
// future months beyond next month are disabled.
export function MonthPicker({
  value,
  available,
  onChange,
}: {
  value: string; // mid-month ISO the overview API's ?date= accepts
  available: ReadonlySet<string>; // "YYYY-MM" keys that have offer periods
  onChange: (iso: string) => void;
}) {
  const [open, setOpen] = useState(false);
  const [year, setYear] = useState(() => Number(value.slice(0, 4)));
  const ref = useRef<HTMLDivElement>(null);

  const selectedKey = monthKey(value);
  const currentKey = monthKey(todayISO());
  // Forward limit: next month — the selection ritual happens before the month
  // starts (banks open next-month categories ~25th), so it must be reachable
  // even before its period exists. Anything past it isn't real data yet.
  const now = new Date();
  const next = new Date(now.getFullYear(), now.getMonth() + 1, 1);
  const maxKey = `${next.getFullYear()}-${pad2(next.getMonth() + 1)}`;

  const dataYears = [...available, selectedKey].map((k) => Number(k.slice(0, 4)));
  // At least a year of backfill room even with no data; never past next month.
  const minYear = Math.min(now.getFullYear() - 1, ...dataYears);
  const maxYear = Math.max(next.getFullYear(), Number(selectedKey.slice(0, 4)));

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div ref={ref} data-sid="W-01" className="relative">
      <button
        type="button"
        aria-haspopup="true"
        aria-expanded={open}
        onClick={() => {
          setYear(Number(value.slice(0, 4)));
          setOpen(!open);
        }}
        className="flex items-center gap-1 text-xs font-medium text-tx3"
      >
        {fmtMonthYear(new Date(Number(value.slice(0, 4)), Number(value.slice(5, 7)) - 1))}
        <span className="text-[9px] text-tx4">▾</span>
      </button>

      {open && (
        <div className="absolute right-0 top-full z-30 mt-2 w-60 rounded-2xl border border-brd bg-srf p-3 shadow-[0_14px_40px_-12px_rgba(0,0,0,.55)]">
          <div className="flex items-center justify-between">
            <button
              type="button"
              disabled={year <= minYear}
              onClick={() => setYear(year - 1)}
              className="flex h-7 w-7 items-center justify-center rounded-lg border border-brd2 bg-srf2 text-tx3 disabled:opacity-30"
            >
              ‹
            </button>
            <span className="text-sm font-bold">{year}</span>
            <button
              type="button"
              disabled={year >= maxYear}
              onClick={() => setYear(year + 1)}
              className="flex h-7 w-7 items-center justify-center rounded-lg border border-brd2 bg-srf2 text-tx3 disabled:opacity-30"
            >
              ›
            </button>
          </div>
          <div className="mt-2.5 grid grid-cols-4 gap-1.5">
            {MONTHS_SHORT.map((label, m) => {
              const key = `${year}-${pad2(m + 1)}`;
              const isSelected = key === selectedKey;
              const isFuture = key > maxKey; // beyond next month → not real data yet
              const hasData = available.has(key);
              const cls = isSelected
                ? "grad-acc font-bold text-white shadow-[0_6px_16px_-8px_rgba(139,111,255,.9)]"
                : isFuture
                  ? "text-tx4 opacity-40"
                  : key === currentKey
                    ? "border border-acc/40 bg-srf2 font-semibold text-tx2"
                    : "bg-srf2 font-semibold text-tx2";
              return (
                <button
                  key={key}
                  type="button"
                  disabled={isFuture}
                  onClick={() => {
                    onChange(midMonthISO(year, m));
                    setOpen(false);
                  }}
                  className={`relative rounded-[10px] py-2 text-center text-[12px] transition disabled:cursor-not-allowed ${cls}`}
                >
                  {label}
                  {hasData && !isSelected && (
                    <span className="absolute bottom-1 left-1/2 h-1 w-1 -translate-x-1/2 rounded-full bg-accl" />
                  )}
                </button>
              );
            })}
          </div>
          <p className="mt-2.5 flex items-center gap-1.5 text-[10px] font-medium text-tx4">
            <span className="h-1 w-1 rounded-full bg-accl" />
            есть данные · любой месяц можно открыть и заполнить
          </p>
        </div>
      )}
    </div>
  );
}
