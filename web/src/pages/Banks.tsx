import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api, unwrap, type Schemas } from "../api/client";
import { useBanks, useClients, usePrograms, useTierMap } from "../hooks";
import { BackButton, Badge, BankBadge, Btn, Card, Empty, ErrMsg, Field, Input, Pct, Select, Spinner } from "../components/ui";
import { MonthPicker } from "../components/MonthPicker";
import { PartnerChips, PartnerSheet } from "../components/Partners";
import { capNote, monthKey, monthNameOf, quarterNote } from "../lib";
import { rememberMonth, viewedMonth } from "../month";

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

// Where a roster stops being readable at a glance.
const FILTER_FROM = 6;

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
function AddCardForm({ onDone }: { onDone: () => void }) {
  const clients = useClients();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const [clientID, setClientID] = useState("");
  const [last4Str, setLast4Str] = useState("");
  const [paySystem, setPaySystem] = useState<PaySystem>("mir");

  const options = clients.data ?? [];

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
        <p className="text-sm font-medium text-tx3">Сначала добавь банк — карта появится под ним.</p>
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
            <option value="">— выбери —</option>
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
            <p className="truncate text-[13.5px] font-bold text-tx3">
              {title} <span className="font-semibold text-tx4">{cardNums}</span>
            </p>
            <p className="mt-px truncate text-[10.5px] font-medium text-tx4">{monthName} не заполнен</p>
          </div>
          {/* The card's one job is this button, so it carries the primary
              weight (design 2026-09-05): a menu-less client is the only
              thing on CB-09 asking to be acted on, and the tinted variant
              read as one more chip among the slot badges. */}
          <Btn
            className="flex-none"
            onClick={() => navigate(`/periods/new?client=${c.bank_client_id}&month=${monthKey(monthDate)}`)}
          >
            Заполнить {monthName}
          </Btn>
          <button type="button" className="px-1 text-tx4" title="Держатель / тариф" onClick={onToggleEdit}>
            ✎
          </button>
        </div>
        {/* Партнёрки live independently of the month menu — alive even here. */}
        <PartnerChips offers={c.partner_offers ?? []} onOpen={onOpenPartner} />
        {editing && <ClientEditForm client={c} onDone={onToggleEdit} />}
      </div>
    );
  }

  // Полное меню — рамка и фон вместо слов (2e): the fill state used to be a
  // «можно добавить/добавить нельзя» policy note, which read as advice.
  const full = c.max_categories != null && c.slots_used >= c.max_categories;
  const nothingPicked = (c.selected ?? []).length === 0 && (c.specials ?? []).length === 0;

  return (
    <Card className={`p-3.5 ${full ? "border-acc/50 bg-acc/10" : ""}`}>
      <div className="cursor-pointer" onClick={() => navigate(`/periods/${c.period_id}`)}>
        <div className="flex items-center gap-2.5">
          {titleMode === "bank" && <BankBadge name={c.bank_name} />}
          <div className="min-w-0 flex-1">
            <p className="text-[13.5px] font-bold">
              {title} <span className="font-semibold text-tx4">{cardNums}</span>
            </p>
            <p className="mt-px truncate text-[10.5px] font-medium text-tx4">
              {[c.tier_name, capNote(c), quarterNote(c.period_start, c.period_end), nothingPicked ? "ничего не выбрано" : ""]
                .filter(Boolean)
                .join(" · ") || "без тарифа"}
            </p>
          </div>
          {c.max_categories != null ? (
            <span
              className={`flex-none rounded-lg px-2 py-1 text-[11px] font-extrabold ${
                full ? "bg-acc/30 text-tx" : "bg-inset text-tx2"
              }`}
            >
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
              {chip.emoji && `${chip.emoji} `}
              {chip.raw_title} <Pct percent={chip.percent} currency={c.currency_kind} className="text-[10.5px]" />
            </span>
          ))}
          {/* Gold already says «granted» (2e v3) — no kind word on the chip. */}
          {(c.specials ?? []).map((chip) => (
            <span key={chip.offer_id} className="rounded-lg border border-gold/25 bg-gold/10 px-2 py-1 text-[10.5px] font-semibold text-gold">
              {chip.emoji && `${chip.emoji} `}
              {chip.raw_title}
              {chip.percent != null && ` ${chip.percent}%`}
            </span>
          ))}
          {c.max_categories != null && c.slots_used < c.max_categories && (
            <span className="rounded-lg border border-dashed border-dash px-2 py-1 text-[10.5px] font-semibold text-tx3">+ слот</span>
          )}
        </div>
        <PartnerChips offers={c.partner_offers ?? []} onOpen={onOpenPartner} />
      </div>
      {editing && <ClientEditForm client={c} onDone={onToggleEdit} />}
    </Card>
  );
}

// The per-bank ритуал date, and only that: «когда откроется ВТБ?» has an
// answer before it is too late (2026-08-27). A bank whose opens-day is
// unknown shows nothing rather than a guessed date.
//
// It used to also carry an accent pill about a month with an empty menu —
// which, on a screen whose picker says «Август» and whose cards say «август
// не заполнен», announced «сентябрь не заполнен» beside them. That nudge now
// lives on CB-01.d, where it names the month and carries a button per
// клиент; here it only contradicted the month being viewed.
function OpensLine({ clients }: { clients: OverviewClient[] }) {
  const day = clients.find((c) => c.selection_opens_day != null)?.selection_opens_day;
  if (day == null) return null;
  return <span className="text-[10.5px] font-medium text-tx4">выбор с {day}-го</span>;
}

export default function Banks() {
  const [params] = useSearchParams();
  const initialMonth = params.get("month");
  // The month follows the feed's pick (web/src/month.ts) — CB-01 ↔ CB-09
  // hops must not snap a backfilling user to the current month.
  const [monthDate, setMonthDateState] = useState(initialMonth ? `${initialMonth}-15` : viewedMonth());
  const setMonthDate = (iso: string) => {
    rememberMonth(iso);
    setMonthDateState(iso);
  };
  const [grouping, setGroupingState] = useState<BanksGrouping>(storedGrouping);
  const [addingCard, setAddingCard] = useState(false);
  const [filter, setFilter] = useState("");
  const [editingClientID, setEditingClientID] = useState<number | null>(null);
  const [partnerID, setPartnerID] = useState<number | null>(null);
  const overview = useOverview(monthDate);
  const banks = useBanks();
  const navigate = useNavigate();
  const monthName = monthNameOf(monthDate);

  const setGrouping = (g: BanksGrouping) => {
    localStorage.setItem(GROUP_KEY, g);
    setGroupingState(g);
  };

  if (overview.isPending) return <Spinner />;
  if (overview.isError) return <ErrMsg error={overview.error} />;
  const data = overview.data;
  const clients = data.clients ?? [];
  const bankColor = new Map((banks.data ?? []).map((b) => [b.id, b.color_hex]));

  // Word-by-word in any order, the same rule the точки-продаж search follows:
  // «альфа юля» has to find the client that «юля альфа» finds. One haystack
  // per client — банк, держатель, тариф, and the plastics by their last four,
  // so «4417» lands on the card the user is holding.
  const words = filter.trim().toLowerCase().split(/\s+/).filter(Boolean);
  const shown =
    words.length === 0
      ? clients
      : clients.filter((c) => {
          const hay = [c.bank_name, c.holder_label ?? "", c.tier_name ?? "", ...(c.cards ?? []).map((cc) => last4(cc.last_4_digits))]
            .join(" ")
            .toLowerCase();
          return words.every((w) => hay.includes(w));
        });

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
        <BackButton />
        <h1 className="min-w-0 flex-1 text-xl font-extrabold tracking-tight">Банки и карты</h1>
        <MonthPicker value={monthDate} onChange={setMonthDate} />
      </div>

      <div data-sid="CB-09.b" className="space-y-2.5">
        {/* One user keeps three bank clients, another sixteen. The field
            appears only for the second: on a short roster it is furniture,
            on a long one there is no other way to reach the card in hand. */}
        {clients.length > FILTER_FROM && (
          <Input
            type="search"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            placeholder="Банк, держатель или ··1234"
            data-sid="CB-09.g"
          />
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
        {clients.length > 0 && shown.length === 0 && <Empty>Ничего не нашлось — попробуй другое слово.</Empty>}

        {grouping === "holder"
          ? /* Family fleet: bank clients grouped by держатель (2026-07-09);
               one row per client — its plastics share the selection. */
            groupByHolder(shown).map(([holder, group]) => (
              <div key={holder || "_own"} className="space-y-2.5">
                {holder !== "" && <p className="mx-0.5 pt-1 text-[11px] font-bold text-tx2">{holder}</p>}
                {group.map((c) => (
                  <div key={c.bank_client_id} className="space-y-1">
                    {clientCard(c, "bank")}
                    {/* Grouped by держатель there is no bank header to hang the
                        ритуал date on, so it sits under its own card. */}
                    <div className="mx-0.5 flex justify-end">
                      <OpensLine clients={[c]} />
                    </div>
                  </div>
                ))}
              </div>
            ))
          : /* No per-section «+ держатель»/«+ карта» (2e): the case is rare
               and the bottom buttons cover it. */
            groupByBank(shown).map(([bankID, group]) => (
              <div key={bankID} className="space-y-2.5">
                <div className="mx-0.5 flex items-center gap-2 pt-1">
                  <BankBadge name={group[0].bank_name} size={22} color={bankColor.get(bankID)} />
                  <p className="min-w-0 flex-1 truncate text-[11px] font-bold text-tx2">{group[0].bank_name}</p>
                  <OpensLine clients={group} />
                </div>
                {group.map((c) => clientCard(c, "holder"))}
              </div>
            ))}

        {addingCard ? (
          <AddCardForm onDone={() => setAddingCard(false)} />
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
              onClick={() => setAddingCard(true)}
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
