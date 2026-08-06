import { useMemo, useState } from "react";
import { useClients, usePeriods } from "../hooks";
import { BankBadge, Chip } from "./ui";
import { Sheet } from "./Sheet";
import { MONTHS_NOM, fmtMonthYear, midMonthISO, monthKey, pad2, todayISO } from "../lib";

// The feed header's month chip (W-01), redesigned as a bottom sheet: one row
// per month, each carrying the user's bank clients as logos — bright = the
// month's menu is entered (offer rows exist), dim = not. ANY month up to next
// month stays selectable — backfilling needs months with no data yet
// (2026-07-15). The quarter case falls out of period coverage: an МКБ period
// spanning июль–сентябрь lights МКБ in all three rows.
export function MonthPicker({
  value,
  onChange,
  opensDay,
}: {
  value: string; // mid-month ISO the overview API's ?date= accepts
  onChange: (iso: string) => void;
  opensDay?: number | null; // selection_opens_day — the future-month hint
}) {
  const [open, setOpen] = useState(false);
  const [year, setYear] = useState(() => Number(value.slice(0, 4)));
  const clients = useClients();
  const periods = usePeriods();

  const selectedKey = monthKey(value);
  const currentKey = monthKey(todayISO());
  const now = new Date();
  const next = new Date(now.getFullYear(), now.getMonth() + 1, 1);
  const maxKey = `${next.getFullYear()}-${pad2(next.getMonth() + 1)}`;

  // client id → months its periods cover with a non-empty menu.
  const filled = useMemo(() => {
    const m = new Map<number, Set<string>>();
    for (const p of periods.data ?? []) {
      if (!p.offer_count) continue;
      let d = new Date(Number(p.period_start.slice(0, 4)), Number(p.period_start.slice(5, 7)) - 1, 1);
      const end = new Date(Number(p.period_end.slice(0, 4)), Number(p.period_end.slice(5, 7)) - 1, 1);
      const set = m.get(p.bank_client_id) ?? new Set<string>();
      while (d <= end) {
        set.add(`${d.getFullYear()}-${pad2(d.getMonth() + 1)}`);
        d = new Date(d.getFullYear(), d.getMonth() + 1, 1);
      }
      m.set(p.bank_client_id, set);
    }
    return m;
  }, [periods.data]);

  const dataYears = [...filled.values()].flatMap((s) => [...s]).map((k) => Number(k.slice(0, 4)));
  const minYear = Math.min(now.getFullYear() - 1, Number(selectedKey.slice(0, 4)), ...dataYears);
  const maxYear = Math.max(next.getFullYear(), Number(selectedKey.slice(0, 4)));

  const roster = clients.data ?? [];

  return (
    <>
      <button
        type="button"
        aria-haspopup="dialog"
        aria-expanded={open}
        data-sid="W-01"
        onClick={() => {
          setYear(Number(value.slice(0, 4)));
          setOpen(true);
        }}
        className="flex items-center gap-1.5 rounded-[11px] bg-inset px-2.5 py-1.5 text-[13px] font-bold text-tx"
      >
        {fmtMonthYear(new Date(Number(value.slice(0, 4)), Number(value.slice(5, 7)) - 1))}
        <span className="text-[9px] text-tx4">{open ? "▲" : "▼"}</span>
      </button>

      {open && (
        <Sheet onClose={() => setOpen(false)} title="Месяц" sid="W-01">
          <div className="mb-2 flex items-center justify-between">
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
          <div className="space-y-1.5">
            {MONTHS_NOM.map((name, m) => {
              const key = `${year}-${pad2(m + 1)}`;
              const isFuture = key > maxKey; // beyond next month → not real data yet
              const isNext = key === maxKey;
              const isSelected = key === selectedKey;
              const fills = roster.map((c) => filled.get(c.id)?.has(key) ?? false);
              const filledCount = fills.filter(Boolean).length;
              const cls = isSelected
                ? "border border-acc shadow-[0_10px_26px_-14px_rgba(139,111,255,.5)]"
                : isNext && filledCount === 0
                  ? "border border-dashed border-dash bg-srf/60"
                  : "border border-brd bg-srf";
              return (
                <button
                  key={key}
                  type="button"
                  disabled={isFuture && !isNext}
                  onClick={() => {
                    onChange(midMonthISO(year, m));
                    setOpen(false);
                  }}
                  className={`flex w-full items-center gap-2.5 rounded-[13px] px-3 py-2.5 text-left disabled:opacity-35 ${cls}`}
                >
                  <span className="min-w-0 flex-1 text-[13.5px] font-semibold text-tx2 capitalize">
                    {name}
                    {key === currentKey && (
                      <span className="ml-1.5 align-[2px]">
                        <Chip tone="friend">сейчас</Chip>
                      </span>
                    )}
                  </span>
                  {roster.length > 0 && !isFuture && (
                    <span className="flex gap-1">
                      {roster.map((c, i) => (
                        <span key={c.id} className={fills[i] ? "" : "opacity-25 saturate-[.3]"}>
                          <BankBadge name={c.bank_name ?? ""} size={16} />
                        </span>
                      ))}
                    </span>
                  )}
                  {isNext && filledCount === 0 && opensDay != null ? (
                    <span className="text-[10.5px] font-semibold text-tx3">банки откроют ~{opensDay}.{pad2(now.getMonth() + 1)}</span>
                  ) : (
                    roster.length > 0 &&
                    !isFuture && (
                      <span className="text-[10.5px] font-semibold text-tx4">
                        {filledCount} из {roster.length}
                      </span>
                    )
                  )}
                </button>
              );
            })}
          </div>
          <p className="mt-2 px-0.5 text-[10px] leading-snug font-medium text-tx4">
            Логотип — клиент банка: яркий — меню занесено, погасший — нет. Любой месяц до следующего можно открыть и заполнить.
          </p>
        </Sheet>
      )}
    </>
  );
}
