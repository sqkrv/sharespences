import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { api, unwrap, type Schemas } from "../api/client";
import { useBanks, useClients, usePrograms, useTierMap } from "../hooks";
import { Badge, BankBadge, Btn, Card, Empty, ErrMsg, Field, Input, Pct, Select, Spinner } from "../components/ui";
import { MonthPicker } from "../components/MonthPicker";
import { Sheet } from "../components/Sheet";
import { capNote, fmtDate, midMonthISO, midPeriodAddNote, monthKey, monthNameOf, opensStripParts, todayISO } from "../lib";

// CB-09 «Банки и карты» — the fleet screen split out of the old CB-01
// «Банки» cut (redesign 2026-08-06). The monthly ritual has its address
// here: the opens-strip, the unfilled-month cards, the add affordances.

// [enum value, human label] — the API takes the lowercase enum, the user
// reads «Мир»/«Visa» (2026-07-15).
const PAYMENT_SYSTEMS = [
  ["mir", "Мир"],
  ["visa", "Visa"],
  ["mastercard", "Mastercard"],
  ["unionpay", "UnionPay"],
  ["american_express", "American Express"],
] as const;
type PaySystem = (typeof PAYMENT_SYSTEMS)[number][0];

type OverviewClient = Schemas["OverviewClientDTO"];

function useOverview(date: string) {
  return useQuery({
    queryKey: ["overview", date],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview", { params: { query: { date } } })),
  });
}

function last4(n: number): string {
  return String(n).padStart(4, "0");
}

// Bank clients grouped by держатель: unlabeled (your own) first, then people
// alphabetically — the family-fleet view (2026-07-09).
function groupByHolder<T extends { holder_label?: string | null }>(clients: T[]): [string, T[]][] {
  const groups = new Map<string, T[]>();
  for (const c of clients) {
    const k = c.holder_label ?? "";
    groups.set(k, [...(groups.get(k) ?? []), c]);
  }
  return [...groups.entries()].sort((a, b) => (a[0] === "" ? -1 : b[0] === "" ? 1 : a[0].localeCompare(b[0], "ru")));
}

// The bank-first cut: one section per bank, its clients (держатели) inside
// (2026-07-23).
function groupByBank<T extends { bank_id: number; bank_name: string }>(clients: T[]): [number, T[]][] {
  const groups = new Map<number, T[]>();
  for (const c of clients) groups.set(c.bank_id, [...(groups.get(c.bank_id) ?? []), c]);
  return [...groups.entries()].sort((a, b) => a[1][0].bank_name.localeCompare(b[1][0].bank_name, "ru"));
}

// The grouping toggle persists like the theme does.
type BanksGrouping = "bank" | "holder";
const GROUP_KEY = "overview-group";
function storedGrouping(): BanksGrouping {
  return localStorage.getItem(GROUP_KEY) === "holder" ? "holder" : "bank";
}

// Держатель + тариф live on the bank client — the cards merely hang off it.
// Deletes live here too: cards go one by one; the bank goes with its cards,
// unless КБ history holds it (the API refuses with 409).
function ClientEditForm({ client, onDone }: { client: OverviewClient; onDone: () => void }) {
  const programs = usePrograms();
  const tierMap = useTierMap();
  const clientsQ = useClients();
  const qc = useQueryClient();
  const full = (clientsQ.data ?? []).find((c) => c.id === client.bank_client_id);
  const [holder, setHolder] = useState(client.holder_label ?? "");
  const [tierID, setTierID] = useState(full?.program_tier_id != null ? String(full.program_tier_id) : "");

  const program = (programs.data ?? []).find((p) => p.bank_id === client.bank_id);
  const tiers = program ? [...(tierMap.data?.values() ?? [])].filter((ti) => ti.program.id === program.id) : [];

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PUT("/api/v1/bank-clients/{id}", {
          params: { path: { id: client.bank_client_id } },
          body: {
            ...(holder.trim() ? { label: holder.trim() } : {}),
            ...(tierID ? { program_tier_id: Number(tierID) } : {}),
          },
        }),
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      qc.invalidateQueries({ queryKey: ["clients"] });
      onDone();
    },
  });

  const delCard = useMutation({
    mutationFn: async (cardID: number) =>
      unwrap(await api.DELETE("/api/v1/cards/{id}", { params: { path: { id: cardID } } })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      qc.invalidateQueries({ queryKey: ["cards"] });
    },
  });

  const delClient = useMutation({
    mutationFn: async () =>
      unwrap(await api.DELETE("/api/v1/bank-clients/{id}", { params: { path: { id: client.bank_client_id } } })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      qc.invalidateQueries({ queryKey: ["clients"] });
      qc.invalidateQueries({ queryKey: ["cards"] });
      onDone();
    },
  });

  return (
    <form
      data-sid="CB-09.d"
      className="mt-3 space-y-3 rounded-xl bg-srf2 p-3"
      onClick={(e) => e.stopPropagation()}
      onSubmit={(e) => {
        e.preventDefault();
        e.stopPropagation();
        save.mutate();
      }}
    >
      <Field label="Держатель">
        <Input value={holder} onChange={(e) => setHolder(e.target.value)} placeholder="Я" />
      </Field>
      {tiers.length > 0 && (
        <Field label="Уровень (тариф КБ-программы)">
          <Select value={tierID} onChange={(e) => setTierID(e.target.value)}>
            <option value="">— не указан —</option>
            {tiers.map(({ tier }) => (
              <option key={tier.id} value={tier.id}>
                {tier.name}
                {tier.is_paid_subscription ? " (подписка)" : ""}
              </option>
            ))}
          </Select>
        </Field>
      )}
      {(client.cards ?? []).length > 0 && (
        <div className="flex flex-wrap gap-1.5">
          {(client.cards ?? []).map((cc) => (
            <span key={cc.card_id} className="flex items-center gap-1.5 rounded-lg bg-inset px-2 py-1 text-[11px] font-semibold text-tx3">
              ··{last4(cc.last_4_digits)}
              <button
                type="button"
                className="text-tx4"
                onClick={() => {
                  if (window.confirm(`Удалить карту ··${last4(cc.last_4_digits)}?`)) delCard.mutate(cc.card_id);
                }}
              >
                ✕
              </button>
            </span>
          ))}
        </div>
      )}
      <div className="flex gap-2">
        <Btn type="submit" disabled={save.isPending}>
          Сохранить
        </Btn>
        <Btn type="button" variant="ghost" onClick={onDone}>
          Отмена
        </Btn>
        <Btn
          type="button"
          variant="danger"
          className="ml-auto"
          disabled={delClient.isPending}
          onClick={() => {
            if (window.confirm(`Удалить ${client.bank_name} (${client.holder_label ?? "Я"}) вместе с картами?`)) delClient.mutate();
          }}
        >
          Удалить банк
        </Btn>
      </div>
      <ErrMsg error={save.error ?? delClient.error ?? delCard.error} />
    </form>
  );
}

// The plastic itself — strictly under an already-added bank (bank client);
// the bank comes first (2026-07-23).
function AddCardForm({ initialBankID, onDone }: { initialBankID?: number; onDone: () => void }) {
  const clients = useClients();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [clientID, setClientID] = useState("");
  const [last4Str, setLast4Str] = useState("");
  const [paySystem, setPaySystem] = useState<PaySystem>("mir");

  const options = (clients.data ?? []).filter((c) => initialBankID == null || c.bank_id === initialBankID);

  const create = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/api/v1/cards", {
          body: {
            bank_client_id: Number(clientID),
            last_4_digits: Number(last4Str),
            payment_system: paySystem,
          },
        }),
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      qc.invalidateQueries({ queryKey: ["cards"] });
      onDone();
    },
  });

  if (!clients.isPending && options.length === 0) {
    return (
      <Card className="space-y-3 p-4" data-sid="CB-09.c">
        <p className="text-sm font-medium text-tx3">Сначала добавьте банк — карта появится под ним.</p>
        <div className="flex gap-2">
          <Btn type="button" onClick={() => navigate("/banks/new")}>
            Добавить банк
          </Btn>
          <Btn type="button" variant="ghost" onClick={onDone}>
            Отмена
          </Btn>
        </div>
      </Card>
    );
  }

  return (
    <Card className="p-4" data-sid="CB-09.c">
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <Field label="Банк и держатель">
          <Select required value={clientID} onChange={(e) => setClientID(e.target.value)}>
            <option value="">— выберите —</option>
            {options.map((c) => (
              <option key={c.id} value={c.id}>
                {c.bank_name} — {c.label ?? "Я"}
              </option>
            ))}
          </Select>
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Последние 4 цифры">
            <Input required inputMode="numeric" pattern="\d{4}" maxLength={4} title="Ровно четыре цифры" value={last4Str} onChange={(e) => setLast4Str(e.target.value)} placeholder="1234" />
          </Field>
          <Field label="Платёжная система">
            <Select value={paySystem} onChange={(e) => setPaySystem(e.target.value as PaySystem)}>
              {PAYMENT_SYSTEMS.map(([ps, label]) => (
                <option key={ps} value={ps}>
                  {label}
                </option>
              ))}
            </Select>
          </Field>
        </div>
        <div className="flex gap-2">
          <Btn type="submit" disabled={create.isPending}>
            Добавить карту
          </Btn>
          <Btn type="button" variant="ghost" onClick={onDone}>
            Отмена
          </Btn>
        </div>
        <ErrMsg error={create.error} />
      </form>
    </Card>
  );
}

// Партнёрки on the bank card (3c): alive offers as gold chips, past ones
// folded behind a count — their home after the CB-05 list dissolved.
function PartnerChips({ c, onOpen }: { c: OverviewClient; onOpen: (id: number) => void }) {
  const [showPast, setShowPast] = useState(false);
  const offers = c.partner_offers ?? [];
  if (offers.length === 0) return null;
  const alive = offers.filter((p) => p.status === "active" || p.status === "scheduled");
  const past = offers.filter((p) => p.status === "ended" || p.status === "expired");
  const chip = (p: NonNullable<OverviewClient["partner_offers"]>[number], muted = false) => (
    <button
      key={p.id}
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        onOpen(p.id);
      }}
      className={`rounded-lg border px-2 py-1 text-[10.5px] font-semibold whitespace-nowrap ${
        muted ? "border-brd2 text-tx4" : "border-gold/30 bg-gold/10 text-gold"
      }`}
    >
      ★ {p.merchant_title}
      {p.percent != null && ` ${p.percent}%`}
      {!muted && p.valid_to && ` · по ${fmtDate(p.valid_to)}`}
      {muted && ` · ${p.status === "ended" ? "завершена" : "истекла"}`}
    </button>
  );
  return (
    <div className="mt-1.5 flex flex-wrap gap-1.5">
      {alive.map((p) => chip(p))}
      {past.length > 0 && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            setShowPast(!showPast);
          }}
          className="rounded-lg border border-dashed border-dash px-2 py-1 text-[10.5px] font-semibold text-tx4"
        >
          прошедшие · {past.length} {showPast ? "▲" : "▼"}
        </button>
      )}
      {showPast && past.map((p) => chip(p, true))}
    </div>
  );
}

// One bank client row — reused by both groupings. titleMode picks what the
// row leads with: the bank (держатель grouping) or the держатель (bank
// grouping, where the section header already names the bank).
function ClientCard({
  c,
  titleMode,
  monthName,
  monthDate,
  editing,
  onToggleEdit,
  onOpenPartner,
}: {
  c: OverviewClient;
  titleMode: "bank" | "holder";
  monthName: string;
  monthDate: string;
  editing: boolean;
  onToggleEdit: () => void;
  onOpenPartner: (id: number) => void;
}) {
  const navigate = useNavigate();
  const title = titleMode === "bank" ? c.bank_name : (c.holder_label ?? "Я");
  const cardNums = (c.cards ?? []).map((cc) => `··${last4(cc.last_4_digits)}`).join(" ");

  if (c.period_id == null) {
    return (
      <div className="rounded-2xl border border-dashed border-dash bg-srf/50 p-3.5">
        <div className="flex items-center gap-2.5">
          {titleMode === "bank" && <BankBadge name={c.bank_name} />}
          <div className="min-w-0 flex-1">
            <p className="text-[13.5px] font-bold text-tx3">
              {title} <span className="font-semibold text-tx4">{cardNums}</span>
            </p>
            <p className="mt-px text-[10.5px] font-medium text-tx4">{monthName} не заполнен</p>
          </div>
          <Btn variant="soft" onClick={() => navigate(`/periods/new?client=${c.bank_client_id}&month=${monthKey(monthDate)}`)}>
            Заполнить
          </Btn>
          <button type="button" className="px-1 text-tx4" title="Держатель / тариф" onClick={onToggleEdit}>
            ✎
          </button>
        </div>
        {/* Партнёрки live independently of the month menu — alive even here. */}
        <PartnerChips c={c} onOpen={onOpenPartner} />
        {editing && <ClientEditForm client={c} onDone={onToggleEdit} />}
      </div>
    );
  }

  return (
    <Card className="p-3.5">
      <div className="cursor-pointer" onClick={() => navigate(`/periods/${c.period_id}`)}>
        <div className="flex items-center gap-2.5">
          {titleMode === "bank" && <BankBadge name={c.bank_name} />}
          <div className="min-w-0 flex-1">
            <p className="text-[13.5px] font-bold">
              {title} <span className="font-semibold text-tx4">{cardNums}</span>
            </p>
            <p className="mt-px truncate text-[10.5px] font-medium text-tx4">
              {[c.tier_name, capNote(c), midPeriodAddNote(c.mid_period_add, c.activation)].filter(Boolean).join(" · ") || "без тарифа"}
            </p>
          </div>
          {c.max_categories != null ? (
            <span className="rounded-lg bg-inset px-2 py-1 text-[11px] font-bold text-tx3">
              {c.slots_used}/{c.max_categories}
            </span>
          ) : c.currency_kind === "points" ? (
            <Badge tone="indigo">баллы</Badge>
          ) : null}
          <button
            type="button"
            className="px-1 text-tx4"
            title="Держатель / тариф"
            onClick={(e) => {
              e.stopPropagation();
              onToggleEdit();
            }}
          >
            ✎
          </button>
        </div>
        <div className="mt-2.5 flex flex-wrap gap-1.5">
          {(c.selected ?? []).map((chip) => (
            <span key={chip.offer_id} className="rounded-lg bg-acc/15 px-2 py-1 text-[10.5px] font-semibold text-tx2">
              {chip.raw_title} <Pct percent={chip.percent} currency={c.currency_kind} className="text-[10.5px]" />
            </span>
          ))}
          {(c.specials ?? []).map((chip) => (
            <span key={chip.offer_id} className="rounded-lg border border-gold/25 bg-gold/10 px-2 py-1 text-[10.5px] font-semibold text-gold">
              {chip.raw_title}
              {chip.percent != null && ` ${chip.percent}%`} · {chip.kind === "super" ? "барабан" : "спец"}
            </span>
          ))}
          {c.max_categories != null && c.slots_used < c.max_categories && (
            <span className="rounded-lg border border-dashed border-dash px-2 py-1 text-[10.5px] font-semibold text-tx4">+ слот</span>
          )}
        </div>
        <PartnerChips c={c} onOpen={onOpenPartner} />
      </div>
      {editing && <ClientEditForm client={c} onDone={onToggleEdit} />}
    </Card>
  );
}

// The expanded партнёрка card (3c): scope, term, limit, activation — with
// «Завершить» as an undoable event and edit/delete a screen away.
function PartnerSheet({ id, onClose }: { id: number; onClose: () => void }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const offer = useQuery({
    queryKey: ["partner-offer", String(id)],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/partner-offers/{id}", { params: { path: { id } } })),
  });
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["overview"] });
    qc.invalidateQueries({ queryKey: ["partner-offer"] });
    qc.invalidateQueries({ queryKey: ["lookup"] });
  };
  const end = useMutation({
    mutationFn: async () =>
      unwrap(await api.POST("/api/v1/cashback/partner-offers/{id}/end", { params: { path: { id } } })),
    onSuccess: invalidate,
  });
  const reopen = useMutation({
    mutationFn: async () =>
      unwrap(await api.POST("/api/v1/cashback/partner-offers/{id}/reopen", { params: { path: { id } } })),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async () =>
      unwrap(await api.DELETE("/api/v1/cashback/partner-offers/{id}", { params: { path: { id } } })),
    onSuccess: () => {
      invalidate();
      onClose();
    },
  });

  const o = offer.data;
  const daysLeft =
    o?.valid_to && o.status === "active"
      ? Math.max(0, Math.ceil((new Date(o.valid_to).getTime() - Date.now()) / 86_400_000))
      : null;
  const unit = o?.currency_kind === "points" ? o.points_label || "баллов" : "₽";
  const scopeText =
    o?.scope_kind === "category"
      ? `категория · ${o.canonical_title_ru ?? "—"}`
      : `магазин · ${o?.merchant_title ?? ""}${o?.canonical_title_ru ? ` (канон: ${o.canonical_title_ru})` : ""}`;

  return (
    <Sheet onClose={onClose} sid="CB-09.e">
      {offer.isPending && <Spinner />}
      {offer.isError && <ErrMsg error={offer.error} />}
      {o && (
        <div className="space-y-3 pb-1">
          <div className="flex items-start gap-2.5">
            <span className="flex h-[30px] w-[30px] flex-none items-center justify-center rounded-[9px] bg-gold/15 text-sm font-extrabold text-gold">★</span>
            <div className="min-w-0 flex-1">
              <p className="text-sm leading-tight font-bold text-gold">{o.merchant_title}</p>
              <p className="mt-0.5 text-[10.5px] font-medium text-tx4">
                {o.bank_name}
                {o.holder_label ? ` · ${o.holder_label}` : o.bank_client_id == null ? " · весь банк" : ""}
                {" · "}
                {o.status === "active" ? "действует" : o.status === "scheduled" ? "ещё не началась" : o.status === "ended" ? "завершена" : "истекла"}
              </p>
            </div>
            <Pct percent={o.percent} currency={o.currency_kind ?? "unknown"} className="text-lg" />
          </div>
          <dl className="space-y-1.5">
            {(
              [
                ["Где действует", scopeText],
                [
                  "Срок",
                  [
                    o.valid_from && `с ${fmtDate(o.valid_from)}`,
                    o.valid_to ? `по ${fmtDate(o.valid_to)}` : "бессрочно",
                    daysLeft != null && `осталось ${daysLeft} дн.`,
                  ]
                    .filter(Boolean)
                    .join(" · "),
                ],
                ["Лимит", [o.cap_value && `${o.cap_value} ${unit}`, o.min_amount && `покупка от ${o.min_amount} ₽`].filter(Boolean).join(" · ") || "—"],
                [
                  "Активация",
                  !o.requires_activation ? "не требуется" : o.activated_at != null ? "активировано ✓" : "требует активации в банке",
                ],
                ...(o.notes ? ([["Заметки", o.notes]] as const) : []),
              ] as const
            ).map(([label, value]) => (
              <div key={label} className="flex items-baseline gap-2">
                <dt className="w-[88px] flex-none text-[10px] font-medium tracking-[.06em] text-tx4 uppercase">{label}</dt>
                <dd className={`min-w-0 flex-1 text-[12.5px] font-semibold ${label === "Активация" && o.requires_activation && o.activated_at == null ? "text-warn" : "text-tx2"}`}>
                  {value}
                </dd>
              </div>
            ))}
          </dl>
          <p className="text-[10px] leading-snug font-medium text-tx4">
            Не занимает слот и не трогает лимит месяца — но ранжируется в ленте, поиске и точке продаж.
          </p>
          <div className="flex gap-2">
            <Btn variant="soft" className="flex-1" onClick={() => navigate(`/partners/new?id=${o.id}`)}>
              Редактировать
            </Btn>
            {o.status === "ended" ? (
              <Btn variant="ghost" className="flex-1" disabled={reopen.isPending} onClick={() => reopen.mutate()}>
                Вернуть
              </Btn>
            ) : (
              <Btn variant="ghost" className="flex-1" disabled={end.isPending} onClick={() => end.mutate()}>
                Завершить
              </Btn>
            )}
            <Btn
              variant="danger"
              disabled={remove.isPending}
              onClick={() => {
                if (window.confirm(`Удалить партнёрку «${o.merchant_title}» насовсем? «Завершить» мягче — её можно вернуть.`)) remove.mutate();
              }}
            >
              🗑
            </Btn>
          </div>
          <ErrMsg error={end.error ?? reopen.error ?? remove.error} />
        </div>
      )}
    </Sheet>
  );
}

export default function Banks() {
  const [params] = useSearchParams();
  const now = new Date();
  const initialMonth = params.get("month");
  const [monthDate, setMonthDate] = useState(
    initialMonth ? `${initialMonth}-15` : midMonthISO(now.getFullYear(), now.getMonth()),
  );
  const [grouping, setGroupingState] = useState<BanksGrouping>(storedGrouping);
  const [addingCard, setAddingCard] = useState<{ bankID?: number } | null>(null);
  const [editingClientID, setEditingClientID] = useState<number | null>(null);
  const [partnerID, setPartnerID] = useState<number | null>(null);
  const overview = useOverview(monthDate);
  const banks = useBanks();
  const navigate = useNavigate();
  const monthName = monthNameOf(monthDate);
  const isCurrentMonth = monthKey(monthDate) === monthKey(todayISO());

  const setGrouping = (g: BanksGrouping) => {
    localStorage.setItem(GROUP_KEY, g);
    setGroupingState(g);
  };

  if (overview.isPending) return <Spinner />;
  if (overview.isError) return <ErrMsg error={overview.error} />;
  const data = overview.data;
  const clients = data.clients ?? [];
  const bankColor = new Map((banks.data ?? []).map((b) => [b.id, b.color_hex]));

  const clientCard = (c: OverviewClient, titleMode: "bank" | "holder") => (
    <ClientCard
      key={c.bank_client_id}
      c={c}
      titleMode={titleMode}
      monthName={monthName}
      monthDate={monthDate}
      editing={editingClientID === c.bank_client_id}
      onToggleEdit={() => setEditingClientID(editingClientID === c.bank_client_id ? null : c.bank_client_id)}
      onOpenPartner={setPartnerID}
    />
  );

  return (
    <>
      <div className="flex items-center gap-2.5">
        <Link to="/" className="flex h-[33px] w-[33px] flex-none items-center justify-center rounded-[11px] border border-brd bg-srf">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--t-tx2)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M14.5 5 8 12l6.5 7" />
          </svg>
        </Link>
        <h1 className="min-w-0 flex-1 text-xl font-extrabold tracking-tight">Банки и карты</h1>
        <MonthPicker value={monthDate} onChange={setMonthDate} opensDay={data.selection_opens_day} />
      </div>

      <div data-sid="CB-09.b" className="space-y-2.5">
        {data.selection_opens_day != null && isCurrentMonth && (
          <div className="flex items-center gap-2 rounded-xl border border-acc/25 bg-acc/10 px-3 py-2" data-sid="CB-09.a">
            <span className="h-1.5 w-1.5 flex-none rounded-full bg-acc" />
            <span className="text-[11px] font-medium text-tx2">
              {opensStripParts(data.selection_opens_day).text} <b className="font-bold text-tx">{opensStripParts(data.selection_opens_day).date}</b>
            </span>
          </div>
        )}

        {clients.length > 0 && (
          <div className="mx-0.5 flex items-center justify-between">
            <span className="text-[11px] font-semibold text-tx3">Меню на {monthName}</span>
            <div className="flex gap-2.5">
              {(
                [
                  ["bank", "по банкам"],
                  ["holder", "по держателям"],
                ] as const
              ).map(([g, label]) => (
                <button
                  key={g}
                  type="button"
                  onClick={() => setGrouping(g)}
                  className={`text-[10.5px] ${grouping === g ? "font-bold text-accl" : "font-semibold text-tx4"}`}
                >
                  {label}
                </button>
              ))}
            </div>
          </div>
        )}

        {clients.length === 0 && <Empty>Пока нет банков — начните с «+ Банк».</Empty>}

        {grouping === "holder"
          ? /* Family fleet: bank clients grouped by держатель (2026-07-09);
               one row per client — its plastics share the selection. */
            groupByHolder(clients).map(([holder, group]) => (
              <div key={holder || "_own"} className="space-y-2.5">
                {holder !== "" && <p className="mx-0.5 pt-1 text-[11px] font-bold text-tx2">{holder}</p>}
                {group.map((c) => clientCard(c, "bank"))}
              </div>
            ))
          : groupByBank(clients).map(([bankID, group]) => (
              <div key={bankID} className="space-y-2.5">
                <div className="mx-0.5 flex items-center gap-2 pt-1">
                  <BankBadge name={group[0].bank_name} size={22} color={bankColor.get(bankID)} />
                  <p className="flex-1 text-[11px] font-bold text-tx2">{group[0].bank_name}</p>
                  <button
                    type="button"
                    className="text-[10.5px] font-semibold text-tx4"
                    onClick={() => navigate(`/banks/new?bank=${bankID}`)}
                  >
                    + держатель
                  </button>
                  <button type="button" className="text-[10.5px] font-semibold text-tx4" onClick={() => setAddingCard({ bankID })}>
                    + карта
                  </button>
                </div>
                {group.map((c) => clientCard(c, "holder"))}
                {addingCard != null && addingCard.bankID === bankID && (
                  <AddCardForm initialBankID={bankID} onDone={() => setAddingCard(null)} />
                )}
              </div>
            ))}

        {addingCard != null && addingCard.bankID == null ? (
          <AddCardForm onDone={() => setAddingCard(null)} />
        ) : (
          <div className="flex gap-2.5">
            <button
              type="button"
              onClick={() => navigate("/banks/new")}
              className="flex-1 rounded-2xl border border-dashed border-dash py-3 text-sm font-semibold text-tx4"
            >
              + Банк
            </button>
            <button
              type="button"
              onClick={() => setAddingCard({})}
              className="flex-1 rounded-2xl border border-dashed border-dash py-3 text-sm font-semibold text-tx4"
            >
              + Карта
            </button>
            <button
              type="button"
              onClick={() => navigate("/partners/new")}
              className="flex-1 rounded-2xl border border-dashed border-dash py-3 text-sm font-semibold text-tx4"
            >
              + Партнёрка
            </button>
          </div>
        )}
      </div>

      {partnerID != null && <PartnerSheet id={partnerID} onClose={() => setPartnerID(null)} />}
    </>
  );
}
