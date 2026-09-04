import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type PerkEvent, type PerkHistoryQuota, type PerkSnapshot } from "../api/client";
import { BackButton, BankBadge, Btn, Card, ErrMsg, Field, Input, Spinner } from "../components/ui";
import { QuotaBar } from "./Perks";
import { Sheet } from "../components/Sheet";
import { fmtDate, isoDate, monthNameOf, todayISO, unitWord } from "../lib";

// PV-02 «Привилегия — леджер» (design4 «Perks - Module», board B): the ledger
// that replaces archaeology across 32 columns.
//
// Every number here has a date and a note, which is the whole answer to «почему
// у Мамы 2 из 4»: a gifted ride is a начисление, the bank spending it somewhere
// unexpected is a сверка catching «−1», and a корректировка with a note closes
// the badge for good. Scoped to one держатель — a perk spans the family, but a
// ledger is one person's.
//
// «Новый период» is a copy helper, never automation: a bank changing its rules
// (ВТБ moved its window from the 20th to the 1st in July) is just different
// dates typed into the same form.

const KIND_LABEL: Record<string, string> = {
  use: "Списание",
  grant: "Начисление",
  resize: "Пересчёт",
  adjust: "Корректировка",
};

type Row =
  | { kind: "event"; at: string; e: PerkEvent }
  | { kind: "snapshot"; at: string; s: PerkSnapshot; gap: number | null };

function nextWindow(start: string, end: string): { start: string; end: string } {
  const s = new Date(`${start}T00:00:00`);
  const e = new Date(`${end}T00:00:00`);
  const isYear = s.getMonth() === 0 && s.getDate() === 1 && e.getMonth() === 11 && e.getDate() === 31;
  if (isYear) {
    return { start: isoDate(new Date(s.getFullYear() + 1, 0, 1)), end: isoDate(new Date(s.getFullYear() + 1, 11, 31)) };
  }
  const lastOfMonth = new Date(e.getFullYear(), e.getMonth() + 1, 0).getDate();
  const isMonth =
    s.getDate() === 1 && e.getDate() === lastOfMonth && s.getMonth() === e.getMonth() && s.getFullYear() === e.getFullYear();
  if (isMonth) {
    return {
      start: isoDate(new Date(s.getFullYear(), s.getMonth() + 1, 1)),
      end: isoDate(new Date(s.getFullYear(), s.getMonth() + 2, 0)),
    };
  }
  const days = Math.round((e.getTime() - s.getTime()) / 86_400_000) + 1;
  const ns = new Date(e.getTime() + 86_400_000);
  return { start: isoDate(ns), end: isoDate(new Date(ns.getTime() + (days - 1) * 86_400_000)) };
}

// «2026 · 01.01 — 31.12»: the year already titles the row, so dates repeat it
// only when the window crosses into another year (ВТБ's programme year does).
function windowTitle(q: { window_start: string; window_end: string }): string {
  const y = q.window_start.slice(0, 4);
  const end = q.window_end.startsWith(y) ? fmtDate(q.window_end).slice(0, 5) : fmtDate(q.window_end);
  return `${y} · ${fmtDate(q.window_start).slice(0, 5)} — ${end}`;
}

// A window's short name: the month, or its dates when two share the month —
// the same disambiguation the chips use.
function focusLabel(q: PerkHistoryQuota, months: PerkHistoryQuota[]): string {
  const name = monthNameOf(q.window_start);
  const ambiguous = months.filter((m) => monthNameOf(m.window_start) === name).length > 1;
  if (ambiguous) return `${fmtDate(q.window_start).slice(0, 5)}–${fmtDate(q.window_end).slice(0, 5)}`;
  return name.replace(/^./, (m) => m.toUpperCase());
}

function plural(n: number, one: string, few: string, many: string): string {
  const t = n % 10;
  const h = n % 100;
  if (t === 1 && h !== 11) return `${n} ${one}`;
  if (t >= 2 && t <= 4 && (h < 12 || h > 14)) return `${n} ${few}`;
  return `${n} ${many}`;
}

function Stepper({ value, onChange, min }: { value: number; onChange: (n: number) => void; min?: number }) {
  const step = (d: number) => onChange(min != null ? Math.max(min, value + d) : value + d);
  return (
    <div className="flex items-center gap-2">
      <button
        type="button"
        onClick={() => step(-1)}
        className="h-9 w-9 flex-none rounded-xl border border-brd2 bg-srf2 text-base font-bold text-tx2"
      >
        −
      </button>
      <input
        type="number"
        inputMode="numeric"
        className="min-w-0 flex-1 rounded-xl border border-brd2 bg-srf px-3 py-2 text-center text-lg font-extrabold"
        value={value}
        onChange={(e) => onChange(Number(e.target.value))}
      />
      <button
        type="button"
        onClick={() => step(1)}
        className="h-9 w-9 flex-none rounded-xl border border-brd2 bg-srf2 text-base font-bold text-tx2"
      >
        +
      </button>
    </div>
  );
}

// PV-02.a — the resolution of PV-S3: an explicit adjust with a note, so the
// «почему» stays in the history forever. The green line promises what the badge
// will do, before the button is pressed.
function AdjustSheet({
  quota,
  context,
  onClose,
}: {
  quota: PerkHistoryQuota;
  context: string;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const gap = quota.discrepancy;
  const [qty, setQty] = useState(gap ? gap.delta : -1);
  const [note, setNote] = useState("");
  const [on, setOn] = useState(todayISO());

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/api/v1/perks/quotas/{id}/events", {
          params: { path: { id: quota.id } },
          body: { kind: "adjust", qty, event_date: on, note: note.trim() || undefined },
        }),
      ),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      onClose();
    },
  });

  const after = quota.remaining + qty;
  const closes = gap != null && after === gap.bank;
  return (
    <Sheet onClose={onClose} title="Корректировка" sid="PV-02.a">
      <p className="-mt-1 text-[11.5px] font-semibold text-tx3">{context}</p>
      <div className="mt-3">
        <Stepper value={qty} onChange={setQty} />
      </div>
      <div className="mt-3 space-y-2.5">
        <Field label="Заметка">
          <Input
            placeholder="банк списал подарочную поездку из годовой"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
        <Field label="Дата">
          <Input type="date" value={on} onChange={(e) => setOn(e.target.value)} />
        </Field>
      </div>
      {qty !== 0 && (
        <p className={`mt-3 text-[10.5px] font-semibold ${closes ? "text-ok" : "text-tx4"}`}>
          {closes
            ? `✓ расчёт станет ${after} — совпадёт со сверкой от ${fmtDate(gap!.observed_on)}, бейдж снимется`
            : `расчёт станет ${after}${gap ? ` · в банке ${gap.bank}` : ""}`}
        </p>
      )}
      <ErrMsg error={save.error} />
      <div className="mt-3 flex gap-2">
        <Btn className="flex-1" disabled={qty === 0 || save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? "Сохраняем…" : "Записать корректировку"}
        </Btn>
        <Btn variant="ghost" onClick={onClose}>
          Отмена
        </Btn>
      </div>
    </Sheet>
  );
}

// PV-02.n — everything pre-filled from the previous window, and every field
// editable. Windows are data: a bank changing its rules is just other dates.
//
// The pre-fill follows the LATEST window, not the running one: after September
// exists, the next suggestion is October. Seeding from «active» instead
// suggested September for a second time, every time.
function PeriodSheet({
  perkID,
  unit,
  context,
  quotas,
  onClose,
}: {
  perkID: number;
  unit: string;
  context: string;
  // Every window this perk has; the pre-fill follows the latest of them.
  quotas: PerkHistoryQuota[];
  onClose: () => void;
}) {
  const qc = useQueryClient();

  const byStart = (a: PerkHistoryQuota, b: PerkHistoryQuota) => a.window_start.localeCompare(b.window_start);
  const pools = quotas.filter((q) => q.parent_quota_id == null).sort(byStart);
  const monthsOf = (pid?: number) => quotas.filter((q) => q.parent_quota_id === pid).sort(byStart);

  const [level, setLevel] = useState<"pool" | "month">(pools.length > 0 ? "month" : "pool");
  const [parentID, setParentID] = useState<number | undefined>(pools.at(-1)?.id);

  // A month copies the latest sibling month; with none yet, firstWindow()
  // seeds this month inside the pool — continuing the POOL would suggest next
  // year's window and a pool-sized quota, both wrong for a sub-limit.
  const base = level === "pool" ? pools.at(-1) : monthsOf(parentID).at(-1);

  // With nothing to copy — the perk's very first window — fall back to the
  // calendar shape the level implies: this year for a pool, this month for a
  // month. «Today to today» is a window nobody has.
  const firstWindow = () => {
    const now = new Date();
    if (level === "pool") {
      return { start: isoDate(new Date(now.getFullYear(), 0, 1)), end: isoDate(new Date(now.getFullYear(), 11, 31)) };
    }
    const parent = pools.find((p) => p.id === parentID);
    const y = parent && parent.window_end < todayISO() ? Number(parent.window_start.slice(0, 4)) : now.getFullYear();
    const m = parent && parent.window_end < todayISO() ? Number(parent.window_start.slice(5, 7)) - 1 : now.getMonth();
    // A month has to fit inside its pool (invariant 2), so a closed pool seeds
    // from its own first month rather than from today.
    return { start: isoDate(new Date(y, m, 1)), end: isoDate(new Date(y, m + 1, 0)) };
  };
  const suggested = base ? nextWindow(base.window_start, base.window_end) : firstWindow();

  const [start, setStart] = useState(suggested.start);
  const [end, setEnd] = useState(suggested.end);
  const [size, setSize] = useState(base?.size ?? 0);
  const [note, setNote] = useState("");

  // Re-seed whenever the thing being copied from changes — the level, the
  // держатель, or the parent pool. Edits survive within one such choice.
  const seedKey = `${level}:${parentID}`;
  const [seeded, setSeeded] = useState(seedKey);
  if (seeded !== seedKey) {
    setSeeded(seedKey);
    setStart(suggested.start);
    setEnd(suggested.end);
    setSize(base?.size ?? 0);
    if (level === "month" && !monthsOf(parentID).length && pools.length && !pools.some((p) => p.id === parentID)) {
      setParentID(pools.at(-1)?.id);
    }
  }

  // Overlapping windows are legitimate — ВТБ's monthly window moved from the
  // 20th to the 1st in July 2026 and the two genuinely overlap — so this warns
  // rather than blocks. An EXACT repeat, though, is a double tap.
  const duplicate = quotas.some(
    (q) =>
      q.window_start === start &&
      q.window_end === end &&
      (level === "month" ? q.parent_quota_id === parentID : q.parent_quota_id == null),
  );

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/api/v1/perks/{id}/quotas", {
          params: { path: { id: perkID } },
          body: {
            parent_quota_id: level === "month" ? parentID : undefined,
            window_start: start,
            window_end: end,
            size,
            note: note.trim() || undefined,
          },
        }),
      ),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      onClose();
    },
  });

  return (
    <Sheet onClose={onClose} title="Новый период" sid="PV-02.n">
      <p className="-mt-1 text-[11.5px] font-semibold text-tx3">{context}</p>

      <div className="mt-3 flex gap-1.5">
        {(
          [
            ["pool", "Годовой"],
            ["month", "Месяц внутри года"],
          ] as const
        ).map(([v, l]) => (
          <button
            key={v}
            type="button"
            disabled={v === "month" && pools.length === 0}
            onClick={() => setLevel(v)}
            className={`flex-1 rounded-xl px-2 py-2 text-[12px] transition disabled:opacity-40 ${
              level === v ? "grad-acc font-bold text-white" : "border border-brd2 bg-srf2 font-semibold text-tx3"
            }`}
          >
            {l}
          </button>
        ))}
      </div>

      {level === "month" && pools.length > 0 && (
        <div className="mt-3">
          <Field label="Родительский период">
            <select
              className="w-full rounded-xl border border-brd2 bg-srf2 px-3 py-2 text-sm font-semibold text-tx2"
              value={parentID ?? ""}
              onChange={(e) => setParentID(Number(e.target.value))}
            >
              {[...pools].reverse().map((p) => (
                <option key={p.id} value={p.id}>
                  {p.window_start.slice(0, 4)} · {fmtDate(p.window_start)} — {fmtDate(p.window_end)} · осталось{" "}
                  {p.remaining} из {p.size}
                </option>
              ))}
            </select>
          </Field>
        </div>
      )}

      <div className="mt-3 grid grid-cols-2 gap-2">
        <Field label="Начало окна">
          <Input type="date" value={start} onChange={(e) => setStart(e.target.value)} />
        </Field>
        <Field label="Конец окна">
          <Input type="date" value={end} onChange={(e) => setEnd(e.target.value)} />
        </Field>
      </div>

      <div className="mt-3">
        <p className="mb-1 text-[10px] font-medium uppercase tracking-[.06em] text-tx4">Размер · {unit} в этом окне</p>
        <Stepper value={size} onChange={setSize} min={0} />
      </div>

      <div className="mt-3">
        <Field label="Заметка">
          <Input placeholder="например: до 1 000 ₽ за поездку" value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
      </div>

      {duplicate && (
        <p className="mt-3 rounded-lg border border-warn/25 bg-warn/10 px-2.5 py-1.5 text-[10.5px] font-semibold text-warn">
          Период с такими датами уже есть. Создать второй можно — так у ВТБ выглядела смена окна в июле — но чаще это
          повторный тап.
        </p>
      )}
      <p className="mt-2 text-[10.5px] font-medium leading-relaxed text-tx4">
        {base ? "Подставлено из прошлого периода. " : ""}Окна — данные: если банк сменит правила, просто введи другие
        даты.
      </p>
      {!base && size === 0 && (
        <p className="mt-2 text-[10.5px] font-semibold text-warn">Укажи размер — период на 0 ничего не даёт.</p>
      )}
      <ErrMsg error={save.error} />
      <div className="mt-3 flex gap-2">
        <Btn className="flex-1" disabled={save.isPending || (!base && size === 0)} onClick={() => save.mutate()}>
          {save.isPending ? "Создаём…" : "Создать период"}
        </Btn>
        <Btn variant="ghost" onClick={onClose}>
          Отмена
        </Btn>
      </div>
      <p className="mt-2 text-[10px] font-medium text-tx4">
        Размер можно править до первого события — дальше только событием «пересчёт»
      </p>
    </Sheet>
  );
}

function EventSheet({
  quotaID,
  context,
  initialKind = "use",
  onClose,
}: {
  quotaID: number;
  context: string;
  initialKind?: "use" | "grant" | "resize";
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [kind, setKind] = useState<"use" | "grant" | "resize">(initialKind);
  const [qty, setQty] = useState(1);
  const [on, setOn] = useState(todayISO());
  const [note, setNote] = useState("");

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/api/v1/perks/quotas/{id}/events", {
          params: { path: { id: quotaID } },
          body: { kind, qty, event_date: on, note: note.trim() || undefined },
        }),
      ),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      onClose();
    },
  });

  const hint = {
    use: "заявленная в банке компенсация — жжёт и месяц, и годовой пул",
    grant: "внеплановая выдача («подарили поездку») — только на этот период",
    resize: "период пересчитали: впиши НОВЫЙ размер целиком, не разницу",
  }[kind];

  return (
    <Sheet onClose={onClose} title="Событие" sid="PV-02.e">
      <p className="-mt-1 text-[11.5px] font-semibold text-tx3">{context}</p>
      <div className="mt-3 flex gap-1.5">
        {(["use", "grant", "resize"] as const).map((k) => (
          <button
            key={k}
            type="button"
            onClick={() => setKind(k)}
            className={`flex-1 rounded-xl px-2 py-2 text-[11.5px] transition ${
              kind === k ? "grad-acc font-bold text-white" : "border border-brd2 bg-srf2 font-semibold text-tx3"
            }`}
          >
            {KIND_LABEL[k]}
          </button>
        ))}
      </div>
      <p className="mt-2 text-[11px] font-medium leading-relaxed text-tx3">{hint}</p>
      <div className="mt-3">
        <p className="mb-1 text-[10px] font-medium uppercase tracking-[.06em] text-tx4">
          {kind === "resize" ? "Новый размер" : "Количество"}
        </p>
        <Stepper value={qty} onChange={setQty} min={0} />
      </div>
      <div className="mt-3 space-y-2.5">
        <Field label="Дата">
          <Input type="date" value={on} onChange={(e) => setOn(e.target.value)} />
        </Field>
        <Field label="Заметка">
          <Input
            placeholder={kind === "resize" ? "не выполнил условия" : "подарочная поездка"}
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
      </div>
      <ErrMsg error={save.error} />
      <div className="mt-3 flex gap-2">
        <Btn className="flex-1" disabled={save.isPending || (kind !== "resize" && qty <= 0)} onClick={() => save.mutate()}>
          {save.isPending ? "Сохраняем…" : "Записать"}
        </Btn>
        <Btn variant="ghost" onClick={onClose}>
          Отмена
        </Btn>
      </div>
    </Sheet>
  );
}

// Editing the definition. Name, unit and note are the three typed fields; the
// bank is not among them, because a perk's identity is (пользователь, банк,
// название) — moving one to another bank is a different perk with a different
// history.
function EditPerkSheet({
  perk,
  windows,
  onDeleted,
  onClose,
}: {
  perk: { id: number; name: string; unit: string; note?: string };
  // Every window of the perk, across держатели: invariant 6 refuses a delete
  // while any of them exists.
  windows: number;
  onDeleted: () => void;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [name, setName] = useState(perk.name);
  const [unit, setUnit] = useState(perk.unit);
  const [note, setNote] = useState(perk.note ?? "");

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PATCH("/api/v1/perks/{id}", {
          params: { path: { id: perk.id } },
          // set_note always travels, so clearing the field clears the note
          // rather than silently leaving the old one.
          body: { name: name.trim(), unit: unit.trim(), set_note: true, note: note.trim() || undefined },
        }),
      ),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      onClose();
    },
  });

  const drop = useMutation({
    mutationFn: async () => unwrap(await api.DELETE("/api/v1/perks/{id}", { params: { path: { id: perk.id } } })),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      onDeleted();
    },
  });

  return (
    <Sheet onClose={onClose} title="Изменить привилегию" sid="PV-02.p">
      <div className="space-y-2.5">
        <Field label="Название">
          <Input value={name} onChange={(e) => setName(e.target.value)} maxLength={64} />
        </Field>
        <Field label="В чём считается">
          <Input value={unit} onChange={(e) => setUnit(e.target.value)} maxLength={32} />
        </Field>
        <Field label="Заметка">
          <Input
            placeholder="например: до 1 000 ₽ за поездку"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
      </div>
      <ErrMsg error={save.error ?? drop.error} />
      <div className="mt-4 flex gap-2">
        <Btn className="flex-1" disabled={!name.trim() || !unit.trim() || save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? "Сохраняем…" : "Сохранить"}
        </Btn>
        <Btn variant="ghost" onClick={onClose}>
          Отмена
        </Btn>
      </div>

      {/* The rule is stated, not hidden: a perk holds a hand-kept ledger, so
          its periods go first and the app says how many are in the way. */}
      <div className="mt-4 border-t border-brd pt-3">
        <Btn
          variant="danger"
          className="w-full"
          disabled={windows > 0 || drop.isPending}
          onClick={() => confirm(`Удалить привилегию «${perk.name}»?`) && drop.mutate()}
        >
          {drop.isPending ? "Удаляем…" : "Удалить привилегию"}
        </Btn>
        <p className="mt-1.5 text-[10px] font-medium leading-relaxed text-tx4">
          {windows > 0
            ? `Сначала удали ${plural(windows, "период", "периода", "периодов")} — в них журнал, который приложение не восстановит.`
            : "Периодов нет — удалится сразу."}
        </p>
      </div>
    </Sheet>
  );
}

// Editing a window. The note is always editable; the size only while nothing
// has been recorded against it (spec invariant 5) — after that the size is a
// dated fact and moves through a «пересчёт» event, which is what this offers
// instead of a disabled field with no way forward.
function EditQuotaSheet({
  quota,
  locked,
  context,
  counts,
  onResize,
  onDeleted,
  onClose,
}: {
  quota: PerkHistoryQuota;
  locked: boolean;
  context: string;
  // What deleting this window would take with it — the confirm has to say so,
  // because none of it can be reconstructed.
  counts: { events: number; snapshots: number; children: number };
  onResize: () => void;
  onDeleted: () => void;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [size, setSize] = useState(quota.initial_size);
  const [note, setNote] = useState(quota.note ?? "");

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PATCH("/api/v1/perks/quotas/{id}", {
          params: { path: { id: quota.id } },
          body: {
            size: locked || size === quota.initial_size ? undefined : size,
            set_note: true,
            note: note.trim() || undefined,
          },
        }),
      ),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      onClose();
    },
  });

  const drop = useMutation({
    mutationFn: async () => unwrap(await api.DELETE("/api/v1/perks/quotas/{id}", { params: { path: { id: quota.id } } })),
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      onDeleted();
    },
  });

  const cascade = [
    counts.children > 0 && plural(counts.children, "месячным периодом", "месячными периодами", "месячными периодами"),
    counts.events > 0 && plural(counts.events, "событием", "событиями", "событиями"),
    counts.snapshots > 0 && plural(counts.snapshots, "сверкой", "сверками", "сверками"),
  ].filter(Boolean);

  return (
    <Sheet onClose={onClose} title="Изменить период" sid="PV-02.w">
      <p className="-mt-1 text-[11.5px] font-semibold text-tx3">
        {context} · {fmtDate(quota.window_start)} — {fmtDate(quota.window_end)}
      </p>
      <div className="mt-3">
        <p className="mb-1 text-[10px] font-medium uppercase tracking-[.06em] text-tx4">Размер, с которым период открылся</p>
        {locked ? (
          <div className="rounded-xl border border-brd2 bg-inset px-3 py-2.5">
            <p className="text-base font-extrabold text-tx3">{quota.initial_size}</p>
            <p className="mt-1 text-[10.5px] font-medium leading-relaxed text-tx4">
              У периода уже есть история, поэтому размер здесь не меняется — иначе прошлое переписалось бы задним
              числом. Банк пересчитал квоту? Это событие «пересчёт» со своей датой.
            </p>
            <Btn variant="soft" className="mt-2 w-full py-1.5 text-[11.5px]" onClick={onResize}>
              Записать пересчёт
            </Btn>
          </div>
        ) : (
          <Stepper value={size} onChange={setSize} min={0} />
        )}
      </div>
      <div className="mt-3">
        <Field label="Заметка">
          <Input placeholder="например: Alfa Only M" value={note} onChange={(e) => setNote(e.target.value)} />
        </Field>
      </div>
      <ErrMsg error={save.error ?? drop.error} />
      <div className="mt-4 flex gap-2">
        <Btn className="flex-1" disabled={save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? "Сохраняем…" : "Сохранить"}
        </Btn>
        <Btn variant="ghost" onClick={onClose}>
          Отмена
        </Btn>
      </div>

      <div className="mt-4 border-t border-brd pt-3">
        <Btn
          variant="danger"
          className="w-full"
          disabled={drop.isPending}
          onClick={() => {
            const tail = cascade.length ? ` вместе с ${cascade.join(" и ")}` : "";
            if (confirm(`Удалить период ${fmtDate(quota.window_start)} — ${fmtDate(quota.window_end)}${tail}? Это не восстановить.`)) {
              drop.mutate();
            }
          }}
        >
          {drop.isPending ? "Удаляем…" : "Удалить период"}
        </Btn>
        <p className="mt-1.5 text-[10px] font-medium leading-relaxed text-tx4">
          {cascade.length
            ? `Удалится вместе с ${cascade.join(" и ")} — журнал заведён руками, приложение его не восстановит.`
            : "В периоде пока ничего не записано."}
        </p>
      </div>
    </Sheet>
  );
}

// Сверка (PV-01.s — drawn as a sheet over the overview, but variant 2d moves
// its ENTRY here: the overview only reports a date, and the ledger is where
// the counter is compared).
//
// PV-S1/S3: the numbers are checked against the ledger as they are typed, so
// «совпадает» or «станет расходится» is visible before saving. Invariant 3 is
// in the footnote — a reading never recomputes anything.
function SyncSheet({
  context,
  targets,
  onClose,
}: {
  context: string;
  targets: { q: PerkHistoryQuota; label: string }[];
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const [on, setOn] = useState(todayISO());
  const [vals, setVals] = useState<Record<number, string>>({});

  const save = useMutation({
    mutationFn: async () => {
      for (const { q } of targets) {
        const raw = (vals[q.id] ?? "").trim();
        if (raw === "") continue;
        unwrap(
          await api.POST("/api/v1/perks/quotas/{id}/snapshots", {
            params: { path: { id: q.id } },
            body: { remaining: Number(raw), observed_on: on },
          }),
        );
      }
    },
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      onClose();
    },
  });

  const filled = targets.some(({ q }) => (vals[q.id] ?? "").trim() !== "");
  return (
    <Sheet onClose={onClose} title="Сверка" sid="PV-02.s">
      <p className="-mt-1 text-[11.5px] font-semibold text-tx3">{context}</p>
      <div className="mt-3">
        <Field label="Дата сверки">
          <Input type="date" value={on} onChange={(e) => setOn(e.target.value)} />
        </Field>
      </div>
      <div className="mt-3 space-y-2.5">
        {targets.map(({ q, label }) => {
          const raw = (vals[q.id] ?? "").trim();
          const bank = Number(raw);
          const known = raw !== "" && Number.isFinite(bank);
          const delta = bank - q.remaining;
          return (
            <div key={q.id} className="rounded-xl border border-brd bg-srf2 px-3 py-2.5">
              <p className="text-[11px] font-bold text-tx2">{label}</p>
              <div className="mt-1.5 flex items-center gap-2">
                <span className="min-w-0 flex-1 text-[11px] font-semibold text-tx4">в банке осталось</span>
                <input
                  type="number"
                  min={0}
                  inputMode="numeric"
                  className="w-[84px] flex-none rounded-lg border border-brd2 bg-srf px-2 py-1.5 text-right text-base font-extrabold"
                  value={vals[q.id] ?? ""}
                  onChange={(e) => setVals({ ...vals, [q.id]: e.target.value })}
                />
              </div>
              {known && (
                <p className={`mt-1.5 text-[10.5px] font-semibold ${delta === 0 ? "text-ok" : "text-warn"}`}>
                  {delta === 0
                    ? `✓ расчёт: ${q.remaining} — совпадает`
                    : `расчёт: ${q.remaining} · банк на ${Math.abs(delta)} ${delta < 0 ? "меньше" : "больше"} — станет «расходится с банком»`}
                </p>
              )}
            </div>
          );
        })}
      </div>
      <p className="mt-3 text-[10.5px] font-medium leading-relaxed text-tx4">
        Сверка ничего не пересчитывает — расхождение станет бейджем. Закрыть его можно корректировкой ниже.
      </p>
      <ErrMsg error={save.error} />
      <div className="mt-3 flex gap-2">
        <Btn className="flex-1" disabled={!filled || save.isPending} onClick={() => save.mutate()}>
          {save.isPending ? "Сохраняем…" : "Записать сверку"}
        </Btn>
        <Btn variant="ghost" onClick={onClose}>
          Отмена
        </Btn>
      </div>
    </Sheet>
  );
}

export default function Perk() {
  const { perkId } = useParams();
  const perkID = Number(perkId);
  const navigate = useNavigate();
  const qc = useQueryClient();

  const [adjusting, setAdjusting] = useState<PerkHistoryQuota | null>(null);
  const [editingPerk, setEditingPerk] = useState(false);
  const [syncing, setSyncing] = useState(false);
  const [editingQuota, setEditingQuota] = useState<PerkHistoryQuota | null>(null);
  const [addingPeriod, setAddingPeriod] = useState(false);
  const [addingEvent, setAddingEvent] = useState<{ id: number; context: string; kind?: "use" | "grant" | "resize" } | null>(
    null,
  );
  const [pastOpen, setPastOpen] = useState(false);
  // The same instant «Списать» PV-01 has (ТУР 3 puts it on the hero too):
  // the safety net is the undo toast, not a form.
  const [undo, setUndo] = useState<{ id: number; at: number } | null>(null);

  // «Сегодня» приходит с клиента: серверные сутки — UTC, и в первые часы
  // местного дня свежее списание иначе не попадало бы в остатки.
  const today = todayISO();
  const history = useQuery({
    queryKey: ["perks", "history", perkID, today],
    enabled: Number.isFinite(perkID),
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/perks/{id}/quotas", { params: { path: { id: perkID }, query: { on: today } } })),
  });

  const delEvent = useMutation({
    mutationFn: async (id: number) => unwrap(await api.DELETE("/api/v1/perks/events/{id}", { params: { path: { id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["perks"] }),
  });
  const delSnapshot = useMutation({
    mutationFn: async (id: number) =>
      unwrap(await api.DELETE("/api/v1/perks/snapshots/{id}", { params: { path: { id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["perks"] }),
  });
  const spend = useMutation({
    mutationFn: async (quotaID: number) =>
      unwrap(
        await api.POST("/api/v1/perks/quotas/{id}/events", {
          params: { path: { id: quotaID } },
          body: { kind: "use", qty: 1, event_date: todayISO() },
        }),
      ),
    onSuccess: async (e) => {
      setUndo({ id: e.id, at: Date.now() });
      await qc.invalidateQueries({ queryKey: ["perks"] });
    },
  });
  const undoSpend = useMutation({
    mutationFn: async (id: number) => unwrap(await api.DELETE("/api/v1/perks/events/{id}", { params: { path: { id } } })),
    onSuccess: async () => {
      setUndo(null);
      await qc.invalidateQueries({ queryKey: ["perks"] });
    },
  });
  useEffect(() => {
    if (!undo) return;
    const t = setTimeout(() => setUndo(null), 6000);
    return () => clearTimeout(t);
  }, [undo]);

  // Every window here is this perk's, and a perk is one держатель's (00025) —
  // there is nothing left to scope by. The label shows only when there IS one;
  // most people track their own quotas and «мой» everywhere reads like data.
  const mine = history.data?.quotas ?? [];
  const label = history.data?.perk.client_label ?? null;

  // Invariant 5's gate: once anything is recorded against a window its size is
  // a dated fact, and the only way to move it is a «пересчёт» event.
  const countsOf = (id: number) => ({
    events: (history.data?.events ?? []).filter((e) => e.quota_id === id).length,
    snapshots: (history.data?.snapshots ?? []).filter((sn) => sn.quota_id === id).length,
    children: (history.data?.quotas ?? []).filter((q) => q.parent_quota_id === id).length,
  });
  const hasHistory = (id: number) => {
    const c = countsOf(id);
    return c.events > 0 || c.snapshots > 0;
  };

  const desc = (a: PerkHistoryQuota, b: PerkHistoryQuota) => b.window_start.localeCompare(a.window_start);
  const pools = mine.filter((q) => q.parent_quota_id == null).sort(desc);
  const hero = pools.find((p) => p.active) ?? pools[0];
  const months = mine.filter((q) => q.parent_quota_id === hero?.id).sort((a, b) => a.window_start.localeCompare(b.window_start));
  // A quick claim lands on the leaf — the running month if there is one, the
  // pool otherwise — regardless of what the ledger is focused on.
  const leaf = months.filter((m) => m.active).at(-1) ?? hero;
  const exhausted = (hero != null && hero.remaining <= 0) || (leaf != null && leaf.remaining <= 0);
  const [focusID, setFocus] = useState<number | null>(null);
  // Focus may name any window — a month chip, the hero pool (tap on its
  // dates), or a past pool — else the год-level ledger (пересчёт 15→12,
  // сверки годового счётчика) has no way in once a month exists.
  const focus =
    mine.find((q) => q.id === focusID) ?? months.find((m) => m.active) ?? months[months.length - 1] ?? hero;

  // The feed of one window, newest first, with a сверка row carrying its own
  // resolution button when it disagrees.
  const feed: Row[] = useMemo(() => {
    if (!focus) return [];
    const es: Row[] = (history.data?.events ?? [])
      .filter((e) => e.quota_id === focus.id)
      .map((e) => ({ kind: "event" as const, at: e.event_date, e }));
    const ss: Row[] = (history.data?.snapshots ?? [])
      .filter((s) => s.quota_id === focus.id)
      .map((s) => ({
        kind: "snapshot" as const,
        at: s.observed_on,
        s,
        // Judged by its own date on the server; the badge's own reading is the
        // one that offers the corrective way out.
        gap: s.computed != null && s.computed !== s.remaining ? s.remaining - s.computed : null,
      }));
    return [...es, ...ss].sort((a, b) => b.at.localeCompare(a.at));
  }, [history.data, focus]);

  if (history.isLoading) return <Spinner />;
  if (history.error) return <ErrMsg error={history.error} />;
  const perk = history.data?.perk;
  if (!perk) return <ErrMsg error={new Error("не найдено")} />;

  const past = pools.filter((p) => p.id !== hero?.id);
  const context = label ? `${perk.name} · ${label}` : perk.name;

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/perks" />
        <BankBadge name={perk.bank_name ?? ""} size={30} />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <h1 className="min-w-0 truncate text-lg font-extrabold tracking-tight">{perk.name}</h1>
            <button
              type="button"
              className="flex-none px-1 text-[13px] text-tx4"
              title="Изменить привилегию"
              aria-label="Изменить привилегию"
              onClick={() => setEditingPerk(true)}
            >
              ✎
            </button>
          </div>
          <p className="text-[11px] font-semibold text-tx4">
            {perk.bank_name}
            {label && ` · ${label}`} · {perk.unit}
          </p>
          {perk.note && <p className="mt-0.5 text-[10.5px] font-medium text-tx4">{perk.note}</p>}
        </div>
        <button
          type="button"
          onClick={() => setAddingPeriod(true)}
          className="flex h-8 w-8 flex-none items-center justify-center rounded-xl bg-acc/15 text-lg font-bold text-accl"
          aria-label="Новый период"
        >
          +
        </button>
      </div>

      {!hero ? (
        <Card className="p-4">
          <p className="text-[12.5px] font-medium leading-relaxed text-tx3">
            У этого держателя ещё нет периодов. Открой первый — годовой пул или сразу месяц, если у банка нет
            двухуровневой квоты.
          </p>
          <Btn className="mt-3" onClick={() => setAddingPeriod(true)}>
            Новый период
          </Btn>
        </Card>
      ) : (
        <Card className="p-4" data-sid="PV-02.h">
          <div className="flex items-center gap-2">
            <button
              type="button"
              onClick={() => setFocus(hero.id)}
              className={`min-w-0 flex-1 text-left text-[11.5px] font-bold ${focus?.id === hero.id ? "text-accl" : "text-tx3"}`}
            >
              {windowTitle(hero)}
            </button>
            {hero.discrepancy && (
              <span className="flex-none rounded-lg border border-warn/30 bg-warn/10 px-2 py-0.5 text-[10.5px] font-bold text-warn">
                расходится {hero.discrepancy.delta > 0 ? `+${hero.discrepancy.delta}` : hero.discrepancy.delta}
              </span>
            )}
          </div>
          <p className="mt-2 flex items-baseline gap-2">
            <span className={`text-[34px] font-extrabold leading-none ${hero.remaining < 0 ? "text-warn" : "text-tx"}`}>
              {hero.remaining}
            </span>
            <span className="text-[12px] font-semibold text-tx4">
              из {hero.size} {unitWord(perk.unit, hero.size)} осталось
            </span>
          </p>
          {hero.note && <p className="mt-1 text-[10.5px] font-medium text-tx4">{hero.note}</p>}
          <QuotaBar q={hero} className="mt-2.5 h-[7px]" />

          {months.length > 0 && (
            <div className="mt-3 flex flex-wrap gap-1.5" data-sid="PV-02.c">

              {months.map((m) => {
                // Two windows can legitimately fall in one month — ВТБ moved
                // its window mid-2026 — and a repeat tap makes it by accident.
                // Either way, chips that read the same cannot be told apart,
                // so an ambiguous one shows its dates instead of its name.
                const name = monthNameOf(m.window_start);
                const ambiguous = months.filter((o) => monthNameOf(o.window_start) === name).length > 1;
                const active = m.id === focus?.id;
                return (
                  <button
                    key={m.id}
                    type="button"
                    onClick={() => setFocus(m.id)}
                    className={`relative min-w-[64px] flex-1 rounded-[10px] px-1 py-[7px] text-center ${
                      active ? "border border-acc/50 bg-acc/10" : "border border-brd2 bg-srf2"
                    }`}
                  >
                    {m.discrepancy && <span className="absolute right-1.5 top-[5px] h-[5px] w-[5px] rounded-full bg-warn" />}
                    <span className={`block text-[12.5px] ${active ? "font-extrabold text-tx" : "font-bold text-tx3"}`}>
                      {m.remaining}/{m.size}
                    </span>
                    <span className={`mt-px block text-[9.5px] font-semibold ${active ? "text-accl" : "text-tx4"}`}>
                      {ambiguous
                        ? `${fmtDate(m.window_start).slice(0, 5)}–${fmtDate(m.window_end).slice(0, 5)}`
                        : name}
                    </span>
                  </button>
                );
              })}
            </div>
          )}

          <div className="mt-3 flex items-center gap-2">
            {!exhausted && leaf && (
              <button
                type="button"
                disabled={spend.isPending}
                onClick={() => spend.mutate(leaf.id)}
                className="flex-1 rounded-xl border border-acc/35 bg-acc/[.07] py-2.5 text-[13px] font-bold text-accl transition active:scale-[.99] disabled:opacity-40"
                data-sid="PV-02.u"
              >
                − Списать
              </button>
            )}
            <Btn variant="soft" className={exhausted || !leaf ? "flex-1" : "flex-none"} onClick={() => setSyncing(true)}>
              Сверить
            </Btn>
          </div>
        </Card>
      )}

      {focus && (
        <>
          <div className="flex items-center justify-between px-0.5">
            <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">
              История · {focus.parent_quota_id ? focusLabel(focus, months).toLowerCase() : focus.id === hero?.id ? "период" : focus.window_start.slice(0, 4)}
            </p>
            <span className="flex items-center gap-2.5">
              <button
                type="button"
                className="px-1 text-[13px] text-tx4"
                title="Изменить период"
                aria-label="Изменить период"
                onClick={() => setEditingQuota(focus)}
              >
                ✎
              </button>
              <button
                type="button"
                className="text-[11.5px] font-bold text-accl"
                onClick={() => setAddingEvent({ id: focus.id, context })}
              >
                + событие
              </button>
            </span>
          </div>

          {focus.id !== hero?.id && focus.note && (
            <p className="-mt-1 px-0.5 text-[10.5px] font-medium text-tx4">{focus.note}</p>
          )}

          <div className="space-y-1.5" data-sid="PV-02.b">
            {feed.length === 0 && (
              <p className="rounded-xl border border-brd bg-srf px-3 py-2.5 text-[11.5px] font-medium text-tx3">
                Пока ничего не записано.
              </p>
            )}
            {feed.map((row) =>
              row.kind === "event" ? (
                <div key={`e${row.e.id}`} className="flex items-center gap-2.5 rounded-[14px] border border-brd bg-srf px-3.5 py-2.5">
                  <div className="min-w-0 flex-1">
                    <p className="text-[12.5px] font-bold">{KIND_LABEL[row.e.kind]}</p>
                    {row.e.note && <p className="mt-px truncate text-[11px] font-medium text-tx4">{row.e.note}</p>}
                  </div>
                  <span
                    className={`flex-none text-[14px] font-extrabold ${
                      row.e.kind === "grant" ? "text-ok" : row.e.kind === "use" ? "text-tx2" : "text-gold"
                    }`}
                  >
                    {row.e.kind === "use"
                      ? `−${row.e.qty}`
                      : row.e.kind === "resize"
                        ? `= ${row.e.qty}`
                        : row.e.qty < 0
                          ? `−${Math.abs(row.e.qty)}`
                          : `+${row.e.qty}`}
                  </span>
                  <span className="w-[34px] flex-none text-right text-[10.5px] font-semibold text-tx4">
                    {fmtDate(row.e.event_date).slice(0, 5)}
                  </span>
                  <button
                    type="button"
                    className="flex-none text-[11px] font-bold text-warn"
                    onClick={() => delEvent.mutate(row.e.id)}
                  >
                    ✕
                  </button>
                </div>
              ) : (
                <div
                  key={`s${row.s.id}`}
                  className={`rounded-[14px] border px-3.5 py-2.5 ${row.gap != null ? "border-warn/30 bg-warn/5" : "border-brd bg-srf"}`}
                >
                  <div className="flex items-center gap-2.5">
                    <div className="min-w-0 flex-1">
                      <p className={`text-[12.5px] font-bold ${row.gap != null ? "text-warn" : "text-ok"}`}>
                        {row.gap != null ? "Сверка — расходится" : "Сверка ✓"}
                      </p>
                      <p className="mt-px truncate text-[11px] font-medium text-tx4">
                        банк: {row.s.remaining}
                        {row.gap != null
                          ? ` · расчёт: ${row.s.computed} · разница ${row.gap > 0 ? `+${row.gap}` : row.gap}`
                          : " · совпало"}
                      </p>
                    </div>
                    <span className="w-[34px] flex-none text-right text-[10.5px] font-semibold text-tx4">
                      {fmtDate(row.s.observed_on).slice(0, 5)}
                    </span>
                    <button
                      type="button"
                      className="flex-none text-[11px] font-bold text-warn"
                      onClick={() => delSnapshot.mutate(row.s.id)}
                    >
                      ✕
                    </button>
                  </div>
                  {row.s.id === focus.discrepancy?.snapshot_id && (
                    <button
                      type="button"
                      className="mt-2 inline-flex rounded-[9px] border border-warn/40 px-3 py-[5px] text-[11px] font-bold text-warn"
                      onClick={() => setAdjusting(focus)}
                    >
                      Скорректировать
                    </button>
                  )}
                </div>
              ),
            )}
          </div>
        </>
      )}

      {past.length > 0 && (
        <div className="space-y-1.5">
          <button
            type="button"
            onClick={() => setPastOpen(!pastOpen)}
            className="flex w-full items-center gap-2 rounded-xl border border-brd bg-srf2/60 px-3.5 py-2.5 text-left"
          >
            <span className="min-w-0 flex-1 text-[11.5px] font-bold text-tx3">Прошлые окна · {past.length}</span>
            <span className="flex-none text-[11px] font-bold text-tx4">{pastOpen ? "▲" : "▼"}</span>
          </button>
          {pastOpen &&
            past.map((p) => (
              <button
                key={p.id}
                type="button"
                onClick={() => setFocus(p.id)}
                className={`w-full rounded-xl border px-3 py-2 text-left ${
                  focus?.id === p.id ? "border-acc/35 bg-acc/[.06]" : "border-brd bg-srf/50"
                }`}
              >
                <p className={`text-[11.5px] font-bold ${focus?.id === p.id ? "text-accl" : "text-tx2"}`}>{windowTitle(p)}</p>
                <p className="text-[10.5px] font-semibold text-tx4">
                  использовано {p.used} из {p.size} · осталось {p.remaining}
                  {p.note && ` · ${p.note}`}
                </p>
              </button>
            ))}
        </div>
      )}

      <ErrMsg error={delEvent.error ?? delSnapshot.error ?? spend.error ?? undoSpend.error} />

      {undo && (
        <div className="fixed inset-x-0 bottom-24 z-40 flex justify-center px-4">
          <div className="flex w-full max-w-md items-center gap-3 rounded-2xl border border-brd2 bg-srf px-3.5 py-2.5 shadow-[0_16px_40px_-18px_rgba(0,0,0,.7)]">
            <span className="min-w-0 flex-1 text-[12.5px] font-semibold">Списано −1</span>
            <button
              type="button"
              className="flex-none text-[12px] font-bold text-accl"
              onClick={() => undoSpend.mutate(undo.id)}
            >
              Отменить
            </button>
          </div>
        </div>
      )}

      {editingPerk && (
        <EditPerkSheet
          perk={{ id: perkID, name: perk.name, unit: perk.unit, note: perk.note }}
          windows={mine.length}
          onDeleted={() => {
            setEditingPerk(false);
            navigate("/perks");
          }}
          onClose={() => setEditingPerk(false)}
        />
      )}
      {editingQuota && (
        <EditQuotaSheet
          quota={editingQuota}
          locked={hasHistory(editingQuota.id)}
          context={context}
          counts={countsOf(editingQuota.id)}
          onResize={() => {
            setAddingEvent({ id: editingQuota.id, context, kind: "resize" });
            setEditingQuota(null);
          }}
          onDeleted={() => {
            // The focused chip may be the window that just went.
            if (focusID === editingQuota.id) setFocus(null);
            setEditingQuota(null);
          }}
          onClose={() => setEditingQuota(null)}
        />
      )}
      {syncing && focus && (
        <SyncSheet
          context={context}
          targets={[
            ...(hero ? [{ q: hero, label: `Год · до ${fmtDate(hero.window_end)}` }] : []),
            ...(focus.id !== hero?.id ? [{ q: focus, label: focusLabel(focus, months) }] : []),
          ]}
          onClose={() => setSyncing(false)}
        />
      )}
      {adjusting && <AdjustSheet quota={adjusting} context={context} onClose={() => setAdjusting(null)} />}
      {addingEvent && (
        <EventSheet
          quotaID={addingEvent.id}
          context={addingEvent.context}
          initialKind={addingEvent.kind}
          onClose={() => setAddingEvent(null)}
        />
      )}
      {addingPeriod && (
        <PeriodSheet
          perkID={perkID}
          unit={perk.unit}
          context={context}
          quotas={mine}
          onClose={() => setAddingPeriod(false)}
        />
      )}
    </>
  );
}
