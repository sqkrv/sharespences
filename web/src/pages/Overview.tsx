import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { useBanks, useClients, usePeriods, usePrograms, useTierMap } from "../hooks";
import { BankBadge, Btn, Card, ErrMsg, Field, Input, Pct, SegTabs, Select, Spinner, Badge } from "../components/ui";
import { MonthPicker } from "../components/MonthPicker";
import { capNote, midMonthISO, monthKey, monthNameOf, opensStripParts, pad2, todayISO } from "../lib";

const PAYMENT_SYSTEMS = ["mir", "visa", "mastercard", "unionpay", "american_express"] as const;
type PaySystem = (typeof PAYMENT_SYSTEMS)[number];

function useOverview(date: string) {
  return useQuery({
    queryKey: ["overview", date],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview", { params: { query: { date } } })),
  });
}

// «Альфа» из «Альфа-Банк» — the design's short bank name in the table cut.
function bankShort(name: string): string {
  return name.split(/[\s-]/)[0] || name;
}

function last4(n: number): string {
  return String(n).padStart(4, "0");
}

// Bank clients grouped by держатель: unlabeled (your own) first, then people
// alphabetically — the family-fleet view (owner 2026-07-09).
function groupByHolder<T extends { holder_label?: string | null }>(clients: T[]): [string, T[]][] {
  const groups = new Map<string, T[]>();
  for (const c of clients) {
    const k = c.holder_label ?? "";
    groups.set(k, [...(groups.get(k) ?? []), c]);
  }
  return [...groups.entries()].sort((a, b) =>
    a[0] === "" ? -1 : b[0] === "" ? 1 : a[0].localeCompare(b[0], "ru"),
  );
}

// Держатель + тариф live on the bank client — the cards merely hang off it.
function ClientEditForm({
  client,
  onDone,
}: {
  client: { bank_client_id: number; bank_id: number; holder_label?: string | null };
  onDone: () => void;
}) {
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

  return (
    <form
      className="mt-3 space-y-3 rounded-xl bg-srf2 p-3"
      onClick={(e) => e.stopPropagation()}
      onSubmit={(e) => {
        e.preventDefault();
        e.stopPropagation();
        save.mutate();
      }}
    >
      <div className="grid grid-cols-2 gap-3">
        <Field label="Держатель">
          <Input value={holder} onChange={(e) => setHolder(e.target.value)} placeholder="Мама" />
        </Field>
        <Field label="Тариф">
          <Select value={tierID} onChange={(e) => setTierID(e.target.value)}>
            <option value="">— не указан —</option>
            {tiers.map(({ tier }) => (
              <option key={tier.id} value={tier.id}>
                {tier.name}
              </option>
            ))}
          </Select>
        </Field>
      </div>
      <div className="flex gap-2">
        <Btn type="submit" disabled={save.isPending}>
          Сохранить
        </Btn>
        <Btn type="button" variant="ghost" onClick={onDone}>
          Отмена
        </Btn>
      </div>
      <ErrMsg error={save.error} />
    </form>
  );
}

// Adding a card = picking (or creating) the bank client it belongs to, then
// the plastic itself. Держатель/тариф are asked only for a new client.
function AddCardForm({ onDone }: { onDone: () => void }) {
  const banks = useBanks();
  const clients = useClients();
  const programs = usePrograms();
  const tierMap = useTierMap();
  const qc = useQueryClient();
  const [bankID, setBankID] = useState("");
  const [clientID, setClientID] = useState(""); // "" = new client
  const [tierID, setTierID] = useState("");
  const [last4Str, setLast4Str] = useState("");
  const [paySystem, setPaySystem] = useState<PaySystem>("mir");
  const [holder, setHolder] = useState("");

  const bankClients = (clients.data ?? []).filter((c) => String(c.bank_id) === bankID);
  const isNewClient = clientID === "";
  const program = (programs.data ?? []).find((p) => String(p.bank_id) === bankID);
  const tiers = program ? [...(tierMap.data?.values() ?? [])].filter((ti) => ti.program.id === program.id) : [];

  const create = useMutation({
    mutationFn: async () => {
      let targetClientID = clientID ? Number(clientID) : null;
      if (targetClientID == null) {
        const created = unwrap(
          await api.POST("/api/v1/bank-clients", {
            body: {
              bank_id: Number(bankID),
              ...(holder.trim() ? { label: holder.trim() } : {}),
              ...(tierID ? { program_tier_id: Number(tierID) } : {}),
            },
          }),
        );
        targetClientID = created.id;
      }
      return unwrap(
        await api.POST("/api/v1/cards", {
          body: {
            bank_client_id: targetClientID,
            last_4_digits: Number(last4Str),
            payment_system: paySystem,
          },
        }),
      );
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      qc.invalidateQueries({ queryKey: ["clients"] });
      qc.invalidateQueries({ queryKey: ["cards"] });
      onDone();
    },
  });

  return (
    <Card className="p-4">
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <Field label="Банк">
          <Select
            required
            value={bankID}
            onChange={(e) => {
              setBankID(e.target.value);
              setClientID("");
              setTierID("");
            }}
          >
            <option value="">— выберите банк —</option>
            {(banks.data ?? []).map((b) => (
              <option key={b.id} value={b.id}>
                {b.name}
              </option>
            ))}
          </Select>
        </Field>
        {bankClients.length > 0 && (
          <Field label="Чья карта">
            <Select value={clientID} onChange={(e) => setClientID(e.target.value)}>
              <option value="">— новый держатель —</option>
              {bankClients.map((c) => (
                <option key={c.id} value={c.id}>
                  {c.label ?? "Я"}
                </option>
              ))}
            </Select>
          </Field>
        )}
        {isNewClient && (
          <>
            <Field label="Держатель (необязательно)">
              <Input value={holder} onChange={(e) => setHolder(e.target.value)} placeholder="Мама" />
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
          </>
        )}
        <div className="grid grid-cols-2 gap-3">
          <Field label="Последние 4 цифры">
            <Input required inputMode="numeric" pattern="\d{4}" maxLength={4} value={last4Str} onChange={(e) => setLast4Str(e.target.value)} placeholder="1234" />
          </Field>
          <Field label="Платёжная система">
            <Select value={paySystem} onChange={(e) => setPaySystem(e.target.value as PaySystem)}>
              {PAYMENT_SYSTEMS.map((ps) => (
                <option key={ps} value={ps}>
                  {ps}
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

// Months any offer period covers (a quarter period spans three), keyed
// "YYYY-MM" — what the picker offers beyond the current month.
function availableMonths(periods: { period_start: string; period_end: string }[]): Set<string> {
  const keys = new Set<string>();
  for (const p of periods) {
    let d = new Date(Number(p.period_start.slice(0, 4)), Number(p.period_start.slice(5, 7)) - 1, 1);
    const end = new Date(Number(p.period_end.slice(0, 4)), Number(p.period_end.slice(5, 7)) - 1, 1);
    while (d <= end) {
      keys.add(`${d.getFullYear()}-${pad2(d.getMonth() + 1)}`);
      d = new Date(d.getFullYear(), d.getMonth() + 1, 1);
    }
  }
  return keys;
}

export default function Overview() {
  const [cut, setCut] = useState<"cats" | "cards">("cats");
  const [addingCard, setAddingCard] = useState(false);
  const [editingClientID, setEditingClientID] = useState<number | null>(null);
  const now = new Date();
  const [monthDate, setMonthDate] = useState(midMonthISO(now.getFullYear(), now.getMonth()));
  const isCurrentMonth = monthKey(monthDate) === monthKey(todayISO());
  const monthName = monthNameOf(monthDate);
  const overview = useOverview(monthDate);
  const periods = usePeriods();
  const months = useMemo(() => availableMonths(periods.data ?? []), [periods.data]);
  const navigate = useNavigate();

  if (overview.isPending) return <Spinner />;
  if (overview.isError) return <ErrMsg error={overview.error} />;
  const data = overview.data;
  const categories = data.categories ?? [];
  const clients = data.clients ?? [];

  return (
    <>
      <div className="flex items-baseline justify-between">
        <h1 className="text-[25px] font-extrabold tracking-tight">Кешбек</h1>
        {/* The month chip is a period picker: browse past periods (owner 2026-07-09);
            all data-backed months are offered, back to the imported history. */}
        <MonthPicker value={monthDate} available={months} onChange={setMonthDate} />
      </div>

      <SegTabs
        value={cut}
        onChange={setCut}
        options={[
          { value: "cats", label: "Категории" },
          { value: "cards", label: "Карты" },
        ]}
      />

      {cut === "cats" && (
        <>
          <div className="mx-0.5 flex items-center justify-between">
            <span className="text-[11px] font-semibold text-tx3">Лучшая карта по категории</span>
            <span className="text-[10px] font-semibold uppercase tracking-wide text-tx4">{monthName}</span>
          </div>
          <Card className="px-4 py-1">
            {categories.length === 0 ? (
              <p className="py-4 text-center text-sm font-medium text-tx3">
                Нет активных выборов — откройте период во вкладке «Карты».
              </p>
            ) : (
              <>
                <div className="flex items-center gap-2.5 py-2.5 text-[9px] font-bold uppercase tracking-[.1em] text-tx4">
                  <span className="flex-1">Категория</span>
                  <span>Карта</span>
                  <span className="w-10 text-right">%</span>
                </div>
                {categories.map((g) => (
                  <button
                    key={g.category_id}
                    type="button"
                    onClick={() => navigate(`/lookup?cat=${g.slug}`)}
                    className="flex w-full items-center gap-2.5 border-t border-brd/60 py-2.5 text-left"
                  >
                    <span className="flex-1 text-sm font-semibold">{g.title_ru}</span>
                    <span className="flex items-center gap-1.5 text-[11.5px] font-medium text-tx3">
                      <BankBadge name={g.best.bank_name} size={22} />
                      {bankShort(g.best.bank_name)}
                      {g.best.holder_label && <span className="text-tx4">· {g.best.holder_label}</span>}
                      {g.others_count > 0 && <span className="text-tx4">+{g.others_count}</span>}
                    </span>
                    <Pct percent={g.best.percent} currency={g.best.currency_kind} className="w-10 text-right text-[14.5px]" />
                  </button>
                ))}
                {data.base && (
                  <button
                    type="button"
                    onClick={() => navigate("/lookup?cat=all-purchases")}
                    className="flex w-full items-center gap-2.5 border-t border-brd/60 py-2.5 text-left"
                  >
                    <span className="flex-1 text-sm font-semibold text-tx3">Остальное</span>
                    <span className="flex items-center gap-1.5 text-[11.5px] font-medium text-tx3">
                      <BankBadge name={data.base.best.bank_name} size={22} />
                      {bankShort(data.base.best.bank_name)}
                      {data.base.best.holder_label && <span className="text-tx4">· {data.base.best.holder_label}</span>}
                      {data.base.others_count > 0 && <span className="text-tx4">+{data.base.others_count}</span>}
                    </span>
                    <span className="w-10 text-right text-[14.5px] font-extrabold text-tx4">
                      {data.base.best.percent != null ? `${data.base.best.percent}%` : "—"}
                    </span>
                  </button>
                )}
              </>
            )}
          </Card>
          {categories.length > 0 && (
            <p className="text-center text-[10.5px] font-medium text-tx4">Тап по строке — детали и лимит</p>
          )}
        </>
      )}

      {cut === "cards" && (
        <div className="space-y-2.5">
          {data.selection_opens_day != null && isCurrentMonth && (
            <div className="flex items-center gap-2 rounded-xl border border-acc/25 bg-acc/10 px-3 py-2">
              <span className="h-1.5 w-1.5 flex-none rounded-full bg-acc" />
              <span className="text-[11px] font-medium text-tx2">
                {opensStripParts(data.selection_opens_day).text} <b className="font-bold text-tx">{opensStripParts(data.selection_opens_day).date}</b>
              </span>
            </div>
          )}
          {/* Family fleet: bank clients grouped by держатель (owner 2026-07-09);
              one row per client — its plastics share the selection. */}
          {groupByHolder(clients).map(([holder, group]) => (
            <div key={holder || "_own"} className="space-y-2.5">
              {holder !== "" && <p className="mx-0.5 pt-1 text-[11px] font-bold text-tx2">{holder}</p>}
              {group.map((c) =>
                c.period_id != null ? (
                  <Card key={c.bank_client_id} className="p-3.5">
                    <div className="cursor-pointer" onClick={() => navigate(`/periods/${c.period_id}`)}>
                      <div className="flex items-center gap-2.5">
                        <BankBadge name={c.bank_name} />
                        <div className="min-w-0 flex-1">
                          <p className="text-[13.5px] font-bold">
                            {c.bank_name}{" "}
                            <span className="font-semibold text-tx4">
                              {(c.cards ?? []).map((cc) => `··${last4(cc.last_4_digits)}`).join(" ")}
                            </span>
                          </p>
                          <p className="mt-px truncate text-[10.5px] font-medium text-tx4">
                            {[c.tier_name, capNote(c)].filter(Boolean).join(" · ") || "без тарифа"}
                            {c.selection_mode === "incremental" && " · инкрементально"}
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
                            setEditingClientID(editingClientID === c.bank_client_id ? null : c.bank_client_id);
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
                            {chip.raw_title} · спец
                          </span>
                        ))}
                        {c.max_categories != null && c.slots_used < c.max_categories && (
                          <span className="rounded-lg border border-dashed border-dash px-2 py-1 text-[10.5px] font-semibold text-tx4">+ слот</span>
                        )}
                      </div>
                    </div>
                    {editingClientID === c.bank_client_id && <ClientEditForm client={c} onDone={() => setEditingClientID(null)} />}
                  </Card>
                ) : (
                  <div key={c.bank_client_id} className="rounded-2xl border border-dashed border-dash bg-srf/50 p-3.5">
                    <div className="flex items-center gap-2.5">
                      <BankBadge name={c.bank_name} />
                      <div className="min-w-0 flex-1">
                        <p className="text-[13.5px] font-bold text-tx3">
                          {c.bank_name}{" "}
                          <span className="font-semibold text-tx4">
                            {(c.cards ?? []).map((cc) => `··${last4(cc.last_4_digits)}`).join(" ")}
                          </span>
                        </p>
                        <p className="mt-px text-[10.5px] font-medium text-tx4">нет периода на {monthName}</p>
                      </div>
                      <Btn variant="soft" onClick={() => navigate(`/periods/new?client=${c.bank_client_id}`)}>
                        Добавить
                      </Btn>
                      <button
                        type="button"
                        className="px-1 text-tx4"
                        title="Держатель / тариф"
                        onClick={() => setEditingClientID(editingClientID === c.bank_client_id ? null : c.bank_client_id)}
                      >
                        ✎
                      </button>
                    </div>
                    {editingClientID === c.bank_client_id && <ClientEditForm client={c} onDone={() => setEditingClientID(null)} />}
                  </div>
                ),
              )}
            </div>
          ))}

          {addingCard ? (
            <AddCardForm onDone={() => setAddingCard(false)} />
          ) : (
            <button
              type="button"
              onClick={() => setAddingCard(true)}
              className="w-full rounded-2xl border border-dashed border-dash py-3 text-sm font-semibold text-tx4"
            >
              + Добавить карту
            </button>
          )}
        </div>
      )}

      <div className="space-y-2.5 pt-1">
        <Link to="/lookup" className="block">
          <Card className="flex items-center gap-3 p-3.5">
            <span className="grad-acc flex h-9 w-9 flex-none items-center justify-center rounded-xl text-white">
              <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                <path d="M20 10c0 6-8 12-8 12s-8-6-8-12a8 8 0 1 1 16 0Z" />
                <circle cx="12" cy="10" r="3" />
              </svg>
            </span>
            <span className="flex-1 text-sm font-bold">Какой картой платить?</span>
            <span className="text-tx4">›</span>
          </Card>
        </Link>
        <Link to="/partners" className="block">
          <Card className="flex items-center gap-3 p-3.5">
            <span className="flex h-9 w-9 flex-none items-center justify-center rounded-xl border border-gold/25 bg-gold/10 text-gold">★</span>
            <span className="flex-1 text-sm font-bold">Партнёрские предложения</span>
            <span className="text-tx4">›</span>
          </Card>
        </Link>
      </div>
    </>
  );
}
