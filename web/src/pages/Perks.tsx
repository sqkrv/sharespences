import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type PerkClient, type PerkQuota } from "../api/client";
import { useBanks, useClients } from "../hooks";
import { BankBadge, Btn, Card, Chip, ErrMsg, Field, Input, Spinner } from "../components/ui";
import { Sheet } from "../components/Sheet";
import { fmtDate, isoDate, monthNameOf, todayISO, unitWord } from "../lib";

// PV-01 «Привилегии — обзор» (docs/specs/perks.md, design4 «Perks - Module»):
// the paper table as one screen, a card per bank client — ordered по держателям,
// the way the family reads the table.
//
// The card says three things at three sizes: the annual window as a bar (that
// is the question «сколько ещё осталось»), the monthly sub-limit as a second
// line, and the state of the reconciliation as a dated badge. «Списать» is the
// only button, because it is the only frequent gesture — one tap is −1 with an
// undo toast, no form. Everything rarer (quantity, backdating, notes, grants,
// re-ratings, corrections) lives in the ledger.
//
// An exhausted quota is NOT hidden: «0 из 12» with the date its window renews
// answers «можно ли ещё», and «нет» is an answer too.

// The bar is the annual window's remaining — the one number the routine turns
// on. It empties as the quota is spent, and goes warn when the ledger has
// drifted past zero.
export function QuotaBar({ q, className = "mt-1.5 h-[5px]" }: { q?: { size: number; remaining: number }; className?: string }) {
  if (!q) return <div className={`${className} w-full rounded-full bg-inset`} />;
  // Квоты дискретные — деление = поездка (ТУР 3, 3d): «2 из 3» видно без
  // чтения цифр. A pool too long to count at a glance falls back to the
  // continuous bar.
  if (q.size > 0 && q.size <= 30) {
    const filled = Math.max(0, Math.min(q.size, q.remaining));
    return (
      <div className={`${className} flex w-full gap-[3px]`}>
        {Array.from({ length: q.size }, (_, i) => (
          <div
            key={i}
            className={`h-full min-w-0 flex-1 rounded-full ${
              q.remaining < 0 ? "bg-warn" : i < filled ? "bg-acc" : "bg-inset"
            }`}
          />
        ))}
      </div>
    );
  }
  const share = q.size > 0 ? Math.max(0, Math.min(1, q.remaining / q.size)) : 0;
  return (
    <div className={`${className} w-full overflow-hidden rounded-full bg-inset`}>
      <div
        className={`h-full rounded-full ${q.remaining < 0 ? "bg-warn" : "grad-acc"}`}
        style={{ width: `${q.remaining < 0 ? 100 : share * 100}%` }}
      />
    </div>
  );
}

// «6 из 15 поездок» — remaining out of size, the way the bank's own app writes
// it, because comparing the two is the whole routine.
function plural(n: number, one: string, few: string, many: string): string {
  const t = n % 10;
  const h = n % 100;
  if (t === 1 && h !== 11) return `${n} ${one}`;
  if (t >= 2 && t <= 4 && (h < 12 || h > 14)) return `${n} ${few}`;
  return `${n} ${many}`;
}

type PerkLine = NonNullable<PerkClient["perks"]>[number];

// With overlapping windows — ВТБ moved its monthly window mid-2026, so two
// genuinely run at once — the later one is the current one.
function monthOf(root: PerkQuota | undefined, quotas: PerkQuota[]): PerkQuota | undefined {
  const kids = root ? (root.children ?? []) : quotas.filter((q) => q.parent_quota_id != null);
  return [...kids].sort((a, b) => a.window_start.localeCompare(b.window_start)).at(-1);
}

// The state a card reports in its header: a discrepancy outranks everything
// (PV-S3), an exhausted quota says so, and an ordinary card says nothing.
function headerState(head: PerkQuota | undefined): { text: string; tone: string } | null {
  if (!head) return null;
  if (head.discrepancy) {
    const d = head.discrepancy.delta;
    return { text: `расходится ${d > 0 ? `+${d}` : d}`, tone: "border-warn/30 bg-warn/10 text-warn" };
  }
  if (head.remaining <= 0) return { text: "исчерпано", tone: "border-brd2 bg-inset text-tx3" };
  return null;
}

// Variant 2d: the overview carries the reconciliation as a dated STATUS, not a
// control — сверка itself lives in the ledger, a tap away on the card. Green
// only while the reading belongs to the window that is running; an older one
// goes quiet, because «сверено» about last month is not «сверено».
function syncStatus(
  head: PerkQuota | undefined,
  month: PerkQuota | undefined,
): { text: string; tone: string } | null {
  if (!head) return null;
  const dry = head.remaining <= 0 ? head : month != null && month.remaining <= 0 ? month : null;
  if (dry) {
    // Local-calendar arithmetic: toISOString() would slide the day back to
    // UTC and promise «обновится» on the window's own last day.
    const e = new Date(`${dry.window_end}T00:00:00`);
    const renews = isoDate(new Date(e.getFullYear(), e.getMonth(), e.getDate() + 1));
    return { text: `обновится ${fmtDate(renews).slice(0, 5)}`, tone: "text-tx4" };
  }
  const seen = head.last_seen_on ?? month?.last_seen_on;
  if (!seen) return null;
  const current = month ?? head;
  const fresh = seen >= current.window_start && !head.discrepancy && !month?.discrepancy;
  return fresh
    ? { text: `✓ сверено ${fmtDate(seen).slice(0, 5)}`, tone: "text-ok" }
    : { text: `сверка ${fmtDate(seen).slice(0, 5)}`, tone: "text-tx4" };
}

function PerkCardRow({
  perk,
  to,
  onSpend,
  busy,
}: {
  perk: PerkLine;
  to: string;
  onSpend: (quotaID: number) => void;
  busy: boolean;
}) {
  const quotas = perk.quotas ?? [];
  const root = quotas.find((q) => q.parent_quota_id == null);
  const month = monthOf(root, quotas);
  const head = root ?? month;
  // A claim lands on the leaf — the running month if there is one, the pool
  // otherwise (a bank with no sub-limit takes claims directly).
  const leaf = month ?? root;
  // Either level being dry stops the invitation: a claim on a spent month (or
  // pool) is still recordable through the ledger, just not one tap away.
  const exhausted = (head != null && head.remaining <= 0) || (leaf != null && leaf.remaining <= 0);
  const sync = syncStatus(head, month);
  // A grant shows up as an effective size above the one the window opened at,
  // which is what «2 из 4 · +1 подарок» is saying.
  const gift = month && month.size > month.initial_size ? month.size - month.initial_size : 0;

  // ТУР 3, вариант 3c: the month is what the routine is about, so it is the
  // card's number and bar; the pool becomes the quiet line underneath.
  const main = month ?? root;
  return (
    <div className="mt-3" data-sid="PV-01.b">
      <div className="flex items-baseline gap-2">
        <Link to={to} className="min-w-0 flex-1 truncate text-[14.5px] font-extrabold">
          {perk.name}
        </Link>
        <p className="flex-none text-[11.5px] font-semibold text-tx4">
          {main ? (
            <>
              <span className={`text-[16px] font-extrabold ${main.remaining < 0 ? "text-warn" : "text-tx"}`}>
                {main.remaining}
              </span>{" "}
              из {main.size}{!month && ` ${unitWord(perk.unit, main.size)}`} осталось
              {month && ` · ${monthNameOf(month.window_start)}`}
              {gift ? ` · +${gift} подарок` : ""}
            </>
          ) : (
            <span className="text-[16px] font-extrabold text-tx4">—</span>
          )}
        </p>
      </div>
      <QuotaBar q={main} />
      <div className="mt-1.5 flex items-baseline gap-2 text-[11px] font-semibold">
        <span className="min-w-0 flex-1 truncate text-tx4">
          {month && root
            ? `за год осталось ${root.remaining} из ${root.size} · до ${fmtDate(root.window_end).slice(0, 5)}`
            : root
              ? `годовое окно · до ${fmtDate(root.window_end)}`
              : "нет активного периода"}
        </span>
        {sync && <span className={`flex-none ${sync.tone}`}>{sync.text}</span>}
      </div>
      {/* No shortcut once the quota is spent — the ledger still accepts a claim
          the bank allowed anyway (invariant 4 refuses nothing), the fast path
          just stops inviting it. */}
      {!exhausted && leaf && (
        <button
          type="button"
          disabled={busy}
          onClick={() => onSpend(leaf.id)}
          className="mt-2.5 w-full rounded-xl border border-acc/35 bg-acc/[.07] py-2.5 text-[13px] font-bold text-accl transition active:scale-[.99] disabled:opacity-40"
          data-sid="PV-01.u"
        >
          − Списать
        </button>
      )}
    </div>
  );
}

// A perk belongs to a держатель (00025), so this is where that is decided —
// «Компенсация такси» at Альфа is one row per account, each with its own size,
// note and history.
function NewPerkSheet({ onClose }: { onClose: () => void }) {
  const banks = useBanks();
  const clients = useClients();
  const existingPerks = useQuery({
    queryKey: ["perks", "list"],
    queryFn: async () => unwrap(await api.GET("/api/v1/perks")) ?? [],
  });
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [bankID, setBankID] = useState("");
  const [holderID, setHolderID] = useState("");
  const [name, setName] = useState("");
  const [unit, setUnit] = useState("поездка");
  const [note, setNote] = useState("");

  const holders = (clients.data ?? []).filter((c) => c.bank_id === Number(bankID));
  // Uniqueness is (держатель, название), so only the SAME person's list can
  // clash — and then the row already exists and should be opened, not refused.
  const existing = (existingPerks.data ?? []).find(
    (p) => p.bank_client_id === Number(holderID) && p.name.trim().toLowerCase() === name.trim().toLowerCase(),
  );
  // Default to the only держатель, and re-default when the bank changes.
  const pick = holderID && holders.some((h) => String(h.id) === holderID) ? holderID : String(holders[0]?.id ?? "");
  if (pick !== holderID) setHolderID(pick);
  const holderLabel = holders.find((h) => String(h.id) === pick)?.label ?? null;

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/api/v1/perks", {
          body: {
            bank_client_id: Number(holderID),
            name: name.trim(),
            unit: unit.trim(),
            note: note.trim() || undefined,
          },
        }),
      ),
    onSuccess: async (perk) => {
      await qc.invalidateQueries({ queryKey: ["perks"] });
      navigate(`/perks/${perk.id}`);
    },
  });

  return (
    <Sheet onClose={onClose} title="Новая привилегия" sid="PV-01.n">
      <div className="space-y-2.5">
        <Field label="Банк">
          <select
            className="w-full rounded-xl border border-brd2 bg-srf2 px-3 py-2 text-sm font-semibold text-tx2"
            value={bankID}
            onChange={(e) => setBankID(e.target.value)}
          >
            <option value="">— выбери банк —</option>
            {(banks.data ?? []).map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </select>
        </Field>
        {/* Only where there is a choice: one держатель needs no question. */}
        {holders.length > 1 && (
          <Field label="Держатель">
            <select
              className="w-full rounded-xl border border-brd2 bg-srf2 px-3 py-2 text-sm font-semibold text-tx2"
              value={holderID}
              onChange={(e) => setHolderID(e.target.value)}
            >
              {holders.map((h) => (
                <option key={h.id} value={h.id}>
                  {h.label ?? "я"}
                </option>
              ))}
            </select>
          </Field>
        )}
        <Field label="Название">
          <Input placeholder="Компенсация такси" value={name} onChange={(e) => setName(e.target.value)} maxLength={64} />
        </Field>
        <Field label="В чём считается">
          <Input placeholder="поездка" value={unit} onChange={(e) => setUnit(e.target.value)} maxLength={32} />
        </Field>
        <Field label="Заметка">
          <Input
            placeholder="например: до 1 000 ₽ за поездку"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </Field>
      </div>
      {bankID && !clients.isLoading && holders.length === 0 && (
        <p className="mt-3 rounded-lg border border-brd2 bg-srf2 px-2.5 py-2 text-[10.5px] font-semibold text-tx3">
          У этого банка пока нет держателей — сначала заведи карту в{" "}
          <Link to="/banks/new" className="font-bold text-accl">
            «Кешбеке»
          </Link>
          .
        </p>
      )}
      {existing && (
        <p className="mt-3 rounded-lg border border-acc/25 bg-acc/10 px-2.5 py-2 text-[10.5px] font-semibold text-accl">
          «{existing.name}» уже заведена{holderLabel ? ` у «${holderLabel}»` : ""} — откроем её.
        </p>
      )}
      <ErrMsg error={save.error} />
      <div className="mt-4 flex gap-2">
        {existing ? (
          <Btn
            className="flex-1"
            onClick={() => {
              onClose();
              navigate(`/perks/${existing.id}`);
            }}
          >
            Открыть
          </Btn>
        ) : (
          <Btn
            className="flex-1"
            disabled={!holderID || !name.trim() || !unit.trim() || save.isPending}
            onClick={() => save.mutate()}
          >
            {save.isPending ? "Создаём…" : "Создать"}
          </Btn>
        )}
        <Btn variant="ghost" onClick={onClose}>
          Отмена
        </Btn>
      </div>
    </Sheet>
  );
}

export default function Perks() {
  const qc = useQueryClient();
  const [creating, setCreating] = useState(false);
  // The undo toast for «Списать»: the gesture is instant, so the safety net is
  // an offer to take it back, not a confirmation before it.
  const [undo, setUndo] = useState<{ id: number; at: number } | null>(null);

  // «Сегодня» приходит с клиента: серверные сутки — UTC, и в первые часы
  // местного дня свежее списание иначе не попадало бы в остатки.
  const today = todayISO();
  const overview = useQuery({
    queryKey: ["perks", "overview", today],
    queryFn: async () => unwrap(await api.GET("/api/v1/perks/overview", { params: { query: { on: today } } })) ?? [],
  });
  const perks = useQuery({
    queryKey: ["perks", "list"],
    queryFn: async () => unwrap(await api.GET("/api/v1/perks")) ?? [],
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

  const clients = overview.data ?? [];
  const cards = clients.flatMap((c) => (c.perks ?? []).map((p) => ({ client: c, perk: p })));
  // A perk between periods has no card of its own — the ledger is where the
  // next window is opened, so it stays reachable as a quiet tail row.
  const running = new Set(cards.map(({ perk }) => perk.perk_id));
  const idle = (perks.data ?? []).filter((p) => !running.has(p.id));
  // Idle perks still belong to держатели, so the header counts them too.
  const clientCount = new Set([...clients.map((c) => c.bank_client_id), ...idle.map((p) => p.bank_client_id)]).size;
  const bankCount = new Set([...clients.map((c) => c.bank_id), ...idle.map((p) => p.bank_id)]).size;

  return (
    <>
      <div className="flex items-center justify-between gap-2">
        <h1 className="text-[25px] font-extrabold tracking-tight">Привилегии</h1>
        <button
          type="button"
          onClick={() => setCreating(true)}
          className="flex h-8 w-8 flex-none items-center justify-center rounded-xl bg-acc/15 text-lg font-bold text-accl"
          aria-label="Новая привилегия"
        >
          +
        </button>
      </div>

      {overview.isLoading && <Spinner />}
      <ErrMsg error={overview.error ?? spend.error ?? undoSpend.error} />

      {!overview.isLoading && cards.length === 0 && idle.length === 0 && (
        <Card className="p-5 text-center" data-sid="PV-01.empty">
          <p className="text-[30px] leading-none">🎟️</p>
          <p className="mt-2.5 text-base font-extrabold">Квоты банковских привилегий</p>
          <p className="mt-1.5 text-[12.5px] font-medium leading-relaxed text-tx3">
            Компенсации такси, бизнес-залы, преференции — всё, что банк даёт штуками на год или месяц. Заведи
            привилегию, укажи окно и размер, отмечай списания и сверяйся с банком.
          </p>
          <Btn className="mt-3.5 w-full" onClick={() => setCreating(true)}>
            Добавить привилегию
          </Btn>
          <p className="mt-4 text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">Например</p>
          <div className="mt-2 space-y-1.5 text-left">
            {[
              ["Компенсация такси", "15 в год · 3 в месяц"],
              ["Преференции", "12 в год · 2 в месяц"],
            ].map(([n, sub]) => (
              <div key={n} className="rounded-xl border border-dashed border-dash bg-srf2/60 px-3 py-2">
                <p className="text-[12.5px] font-bold text-tx2">{n}</p>
                <p className="text-[10.5px] font-semibold text-tx4">{sub}</p>
              </div>
            ))}
          </div>
          <p className="mt-3 text-[10.5px] font-medium text-tx4">
            Держатели и банки берутся из уже заведённого парка карт
          </p>
        </Card>
      )}

      {cards.length > 0 && (
        <div className="flex items-center justify-between px-0.5">
          <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">
            {plural(clientCount, "клиент", "клиента", "клиентов")} ·{" "}
            {plural(bankCount, "банк", "банка", "банков")}
          </p>
          <p className="text-[10px] font-semibold text-tx4">по держателям</p>
        </div>
      )}

      {clients.map((client) => (
        <Card key={client.bank_client_id} className="p-3.5" data-sid="PV-01.a">
          {(client.perks ?? []).map((perk, i) => {
            const quotas = perk.quotas ?? [];
            const root = quotas.find((q) => q.parent_quota_id == null);
            const state = headerState(root ?? monthOf(root, quotas));
            const to = `/perks/${perk.perk_id}`;
            return (
              <div key={perk.perk_id} className={i > 0 ? "mt-4 border-t border-brd pt-3" : undefined}>
                {/* The whole head is the way into the ledger — 2d puts сверка
                    there, so the card has to be tappable and say so. */}
                <Link to={to} className="flex items-center gap-2.5">
                  <BankBadge name={client.bank_name} size={33} />
                  <span className="truncate text-[15px] font-extrabold">{client.bank_name}</span>
                  {/* Only a real держатель gets a chip — «мой» on every card is noise. */}
                  {client.label && <Chip>{client.label}</Chip>}
                  <span className="flex-1" />
                  {state && (
                    <span className={`flex-none rounded-lg border px-2 py-0.5 text-[10.5px] font-bold ${state.tone}`}>
                      {state.text}
                    </span>
                  )}
                  <span className="flex-none text-[13px] font-bold text-tx4">›</span>
                </Link>
                <PerkCardRow perk={perk} to={to} busy={spend.isPending} onSpend={(id) => spend.mutate(id)} />
              </div>
            );
          })}
        </Card>
      ))}

      {idle.map((p) => (
        <Card key={`idle-${p.id}`} className="p-3.5" data-sid="PV-01.a">
          <Link to={`/perks/${p.id}`} className="flex items-center gap-2.5">
            <BankBadge name={p.bank_name ?? ""} size={33} />
            <span className="truncate text-[15px] font-extrabold">{p.bank_name}</span>
            {p.client_label && (
              <span className="flex-none rounded-lg border border-brd2 bg-srf2 px-2 py-0.5 text-[10.5px] font-bold text-tx3">
                {p.client_label}
              </span>
            )}
            <span className="flex-1" />
            <span className="flex-none text-[13px] font-bold text-tx4">›</span>
          </Link>
          <PerkCardRow
            perk={{ perk_id: p.id, name: p.name, unit: p.unit, note: p.note, quotas: [] }}
            to={`/perks/${p.id}`}
            busy={spend.isPending}
            onSpend={(id) => spend.mutate(id)}
          />
        </Card>
      ))}

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

      {creating && <NewPerkSheet onClose={() => setCreating(false)} />}
    </>
  );
}
