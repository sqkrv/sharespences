import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { api, unwrap, type Schemas } from "../api/client";
import { useClients, usePeriods } from "../hooks";
import { BankBadge, Btn, Card, Chip, ErrMsg, ListRow, Pct, Spinner } from "../components/ui";
import { MonthPicker } from "../components/MonthPicker";
import {
  FALLBACK_EMOJI,
  capNote,
  fmtDate,
  initWithFriends,
  monthKey,
  monthNameOf,
  todayISO,
  verdictNote,
} from "../lib";
import { rememberMonth, viewedMonth } from "../month";

type CategoryGroup = Schemas["OverviewCategoryDTO"];
type LookupEntry = Schemas["LookupEntryDTO"];
type PartnerFeed = Schemas["PartnerFeedDTO"];

function useOverview(date: string) {
  return useQuery({
    queryKey: ["overview", date],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview", { params: { query: { date } } })),
  });
}

// The feed sort (ТУР 2): «по алфавиту» default, «по проценту» keeps the API
// order (currency group → percent desc). Persisted under the policy-listed
// overview-cats-sort key; the retired «по банку» value reads as the default.
type CatsSort = "percent" | "alpha";
const CATS_SORT_KEY = "overview-cats-sort";
function storedCatsSort(): CatsSort {
  return localStorage.getItem(CATS_SORT_KEY) === "percent" ? "percent" : "alpha";
}

// The row's displayed winner under the friend rule: the friend only when the
// pref shows friends; otherwise the own best; the dashed available entry when
// nothing is selected at all. Never nothing — the API drops empty groups.
function winnerOf(g: CategoryGroup, friendsOn: boolean): { entry: LookupEntry; state: "friend" | "own" | "available" } | null {
  if (friendsOn && g.friend_best) return { entry: g.friend_best, state: "friend" };
  if (g.best) return { entry: g.best, state: "own" };
  if (g.available) return { entry: g.available, state: "available" };
  return null;
}

// Gold mechanic chip for a winner row: the stacked барабан shows its parts
// («7 + 7 барабан» — the sum is only trustworthy if it shows them), a bare
// super is «барабан», a special is «спец · проверь условие».
function mechanicChip(e: LookupEntry) {
  if (e.stacked_super != null) return <Chip tone="gold">{e.stacked_regular} + {e.stacked_super} барабан</Chip>;
  if (e.kind === "super") return <Chip tone="gold">барабан</Chip>;
  if (e.kind === "special") return <Chip tone="gold">спец</Chip>;
  return null;
}

// The 3a expansion: the category's full ranking (rubles, then points — never
// converted), plus the «можно выбрать» rows with verdicts. Lazily fetched on
// first expand; usePrefetchOffline warms the active slugs for offline.
function ExpandedCategory({ slug, date, friendsOn }: { slug: string; date: string | null; friendsOn: boolean }) {
  const qc = useQueryClient();
  // date null = the current month: the key then matches what
  // usePrefetchOffline warmed, so expansion works at a no-signal checkout.
  const lookup = useQuery({
    queryKey: date ? ["lookup", slug, date] : ["lookup", slug],
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/cashback/lookup", { params: { query: { category: slug, ...(date ? { date } : {}) } } })),
  });
  const mark = useMutation({
    mutationFn: async (offerID: number) =>
      unwrap(await api.POST("/api/v1/cashback/selections", { body: { category_offer_id: offerID } })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      qc.invalidateQueries({ queryKey: ["lookup"] });
    },
  });

  if (lookup.isPending) return <Spinner />;
  if (lookup.isError) return <ErrMsg error={lookup.error} />;
  const d = lookup.data;
  const ranked = (d.ranked ?? []).filter((e) => friendsOn || !e.friend_name);
  const available = d.available ?? [];
  const currencies = new Set(ranked.map((e) => e.currency_kind));

  return (
    <div className="mt-2.5 ml-8 space-y-2 border-t border-brd/60 pt-2.5" data-sid="CB-01.f">
      {ranked.map((e, i) => (
        <div key={`${e.bank_client_id}-${i}`} className="flex items-center gap-2">
          <BankBadge name={e.bank_name} size={18} />
          <span className="min-w-0 flex-1 truncate text-xs font-semibold text-tx2">
            {e.bank_name}
            {e.holder_label ? ` · ${e.holder_label}` : e.friend_name || e.kind === "partner" ? "" : " · Я"}
            {e.friend_name && <span className="ml-1.5"><Chip tone="friend">друг · {e.friend_name}</Chip></span>}
            {e.kind === "partner" && (
              <span className="ml-1.5">
                <Chip tone="gold">партнёрка{e.partner_scope === "merchant" ? ` · только в «${e.raw_title}»` : ""}</Chip>
              </span>
            )}
            {e.currency_kind === "points" && <span className="ml-1.5"><Chip tone="points">{e.points_label || "баллы"}</Chip></span>}
            {!e.friend_name && e.kind !== "partner" && capNote(e) && <span className="font-medium text-tx4"> · {capNote(e)}</span>}
          </span>
          <Pct percent={e.percent} currency={e.currency_kind} className="text-[13px]" />
        </div>
      ))}
      {/* Every served «можно выбрать» row is pickable — the API drops the
          dead ends (slots_full/locked), so no dimmed excuses here. */}
      {available.map((e) => (
        <div key={e.offer_id} className="flex items-center gap-2">
          <BankBadge name={e.bank_name} size={18} />
          <span className="min-w-0 flex-1 text-xs font-semibold text-tx2">
            {e.bank_name}
            {e.holder_label && ` · ${e.holder_label}`}
            <span className="block text-[10px] font-medium text-tx4">{verdictNote(e)}</span>
          </span>
          <Pct percent={e.percent} currency={e.currency_kind} className="text-[13px]" />
          <Btn
            variant="soft"
            className="!px-2.5 !py-1.5 text-xs whitespace-nowrap"
            disabled={mark.isPending}
            onClick={() => mark.mutate(e.offer_id)}
          >
            Отметить
          </Btn>
        </div>
      ))}
      {mark.isError && <ErrMsg error={mark.error} />}
      {currencies.has("points") && currencies.size > 1 && (
        <p className="text-[10px] leading-snug font-medium text-tx4">
          Баллы не сравниваются с рублями напрямую — в строке рублёвый победитель, балльный здесь.
        </p>
      )}
    </div>
  );
}

// One feed row (строка как в 1a): category-first, bank second line, percent
// right; expands in place — the tap that used to be a screen hop (CB-04).
function FeedRow({ g, date, friendsOn }: { g: CategoryGroup; date: string | null; friendsOn: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const w = winnerOf(g, friendsOn);
  if (!w) return null;
  const { entry: e, state } = w;
  const variant = state === "friend" ? "friend" : state === "available" ? "dashed" : "solid";
  return (
    <ListRow
      emoji={g.emoji || FALLBACK_EMOJI}
      variant={variant}
      onClick={() => setExpanded(!expanded)}
      title={
        <>
          {g.title_ru}
          {mechanicChip(e) && <span className="ml-1.5 align-[1px]">{mechanicChip(e)}</span>}
        </>
      }
      sub={
        <>
          <BankBadge name={e.bank_name} size={16} />
          <span>{e.bank_name}</span>
          {e.holder_label && <span className="text-tx4">· {e.holder_label}</span>}
          {state === "friend" && <Chip tone="friend">друг · {e.friend_name}</Chip>}
          {state !== "available" && e.currency_kind === "points" && <Chip tone="points">{e.points_label || "баллы"}</Chip>}
          {state === "available" && <span className="text-tx4">{verdictNote(g.available!)}</span>}
          {state !== "available" && g.others_count > 0 && <span className="text-tx4">+{g.others_count}</span>}
        </>
      }
      right={
        <span className="w-11 flex-none text-right">
          <Pct percent={e.percent} currency={e.currency_kind} className="text-base" />
          {e.stacked_super != null && (
            <span className="block text-[9px] font-semibold text-tx4">
              {e.stacked_regular}+{e.stacked_super}
            </span>
          )}
        </span>
      }
    >
      {expanded && <ExpandedCategory slug={g.slug} date={date} friendsOn={friendsOn} />}
    </ListRow>
  );
}

// A партнёрка feed row (v2): merchant-named, gold, alive even when the
// month's menus are empty. Tap goes to its home on the bank card.
function PartnerFeedRow({ p }: { p: PartnerFeed }) {
  const navigate = useNavigate();
  return (
    <ListRow
      lead={<span className="flex h-[21px] w-[21px] flex-none items-center justify-center rounded-md bg-gold/15 text-[11px] font-extrabold text-gold">★</span>}
      variant="gold"
      onClick={() => navigate("/banks")}
      title={p.raw_title}
      sub={
        <>
          <BankBadge name={p.bank_name} size={16} />
          <span>{p.bank_name}</span>
          <Chip tone="gold">партнёрка{p.valid_to ? ` · по ${fmtDate(p.valid_to)}` : ""}</Chip>
          {p.needs_activation && <span className="font-semibold text-warn">требует активации</span>}
          {p.currency_kind === "points" && <Chip tone="points">{p.points_label || "баллы"}</Chip>}
        </>
      }
      right={
        <span className="w-11 flex-none text-right">
          <Pct percent={p.percent} currency={p.currency_kind} className="text-base" />
        </span>
      }
    />
  );
}

// Interleave category and партнёрка rows without breaking invariant 5: the
// percent sort merges by (currency group, percent desc) — both lists arrive
// from the API already in that order — and the alphabet sort is by name.
type FeedItem = { key: string; title: string; entry: LookupEntry; cat?: CategoryGroup; partner?: PartnerFeed };

function mergeFeed(categories: CategoryGroup[], partners: PartnerFeed[], sort: CatsSort, friendsOn: boolean): FeedItem[] {
  const items: FeedItem[] = [];
  for (const g of categories) {
    const w = winnerOf(g, friendsOn);
    if (w) items.push({ key: `c${g.category_id}`, title: g.title_ru, entry: w.entry, cat: g });
  }
  for (const p of partners) {
    items.push({ key: `p${p.partner_id}`, title: p.raw_title, entry: p, partner: p });
  }
  if (sort === "alpha") return items.sort((a, b) => a.title.localeCompare(b.title, "ru"));
  const group = (k?: string) => (k === "rub" ? 0 : k === "points" ? 1 : 2);
  return items.sort((a, b) => {
    if (group(a.entry.currency_kind) !== group(b.entry.currency_kind)) return group(a.entry.currency_kind) - group(b.entry.currency_kind);
    const pa = a.entry.percent != null ? parseFloat(a.entry.percent) : -1;
    const pb = b.entry.percent != null ? parseFloat(b.entry.percent) : -1;
    if (pa !== pb) return pb - pa;
    return a.title.localeCompare(b.title, "ru");
  });
}

// 5a — the honest zero-banks state: search and friends already work, only
// the month context is missing. The CTA opens the bank catalog.
function FirstRun() {
  const navigate = useNavigate();
  return (
    <div className="space-y-3 pt-4" data-sid="CB-01.h">
      <span className="mx-auto flex h-16 w-16 items-center justify-center rounded-[22px] bg-acc/15">
        <svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="var(--t-accl)" strokeWidth="1.9" strokeLinecap="round">
          <line x1="6.5" y1="17.5" x2="17.5" y2="6.5" />
          <circle cx="8" cy="8" r="1.9" />
          <circle cx="16" cy="16" r="1.9" />
        </svg>
      </span>
      <h2 className="text-center text-xl font-extrabold tracking-tight">Один список вместо пяти приложений</h2>
      {[
        ["🏦", "Добавь банки — слоты и лимиты подставятся из каталога"],
        ["📸", "Скинь скрины меню месяца — категории распознаются сами"],
        ["💳", "На кассе приложение подскажет, какой картой платить"],
      ].map(([icon, text]) => (
        <Card key={icon} className="flex items-center gap-3 px-3.5 py-3">
          <span className="text-base">{icon}</span>
          <span className="text-[12.5px] leading-snug font-medium text-tx2">{text}</span>
        </Card>
      ))}
      <Btn className="w-full" onClick={() => navigate("/banks/new")}>
        Добавить первый банк
      </Btn>
      <p className="text-center text-[11px] font-medium text-tx4">Есть друг в Sharespences? Его кешбеки появятся здесь же.</p>
    </div>
  );
}

export default function Overview() {
  const [catsSort, setCatsSortState] = useState<CatsSort>(storedCatsSort);
  // Shared with CB-09 (web/src/month.ts): the picked month survives the
  // CB-01 ↔ CB-09 hop instead of snapping back to the current one.
  const [monthDate, setMonthDateState] = useState(viewedMonth);
  const setMonthDate = (iso: string) => {
    rememberMonth(iso);
    setMonthDateState(iso);
  };
  const [showSingles, setShowSingles] = useState(false);
  const overview = useOverview(monthDate);
  const clientsQ = useClients();
  const periods = usePeriods();
  const navigate = useNavigate();
  const friendsOn = initWithFriends();
  const monthName = monthNameOf(monthDate);
  const isCurrentMonth = monthKey(monthDate) === monthKey(todayISO());

  const setCatsSort = (s: CatsSort) => {
    localStorage.setItem(CATS_SORT_KEY, s);
    setCatsSortState(s);
  };

  if (overview.isPending || clientsQ.isPending) return <Spinner />;
  if (overview.isError) return <ErrMsg error={overview.error} />;
  const data = overview.data;
  const categories = data.categories ?? [];
  const singles = data.single_bank ?? [];
  const roster = clientsQ.data ?? [];

  // Clients whose viewed month has no entered menu — the 5b card. Quarter
  // periods cover their three months, so МКБ stays «заполнен» mid-quarter.
  const filledClientIDs = new Set(
    (periods.data ?? [])
      .filter((p) => p.offer_count > 0 && p.period_start <= monthDate && monthDate <= p.period_end)
      .map((p) => p.bank_client_id),
  );
  const unfilled = roster.filter((c) => !filledClientIDs.has(c.id));
  const monthEmpty = roster.length > 0 && filledClientIDs.size === 0;

  const feed = mergeFeed(categories, data.partners ?? [], catsSort, friendsOn);

  return (
    <>
      <div className="flex items-center justify-between gap-2.5" data-sid="CB-01.a">
        <h1 className="text-[23px] font-extrabold tracking-tight">Кешбек</h1>
        <div className="flex items-center gap-2">
          {roster.length > 0 && <MonthPicker value={monthDate} onChange={setMonthDate} opensDay={data.selection_opens_day} />}
          <Link to="/banks" title="Банки и карты" className="flex h-[33px] w-[33px] items-center justify-center rounded-[11px] bg-inset">
            <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="var(--t-accl)" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
              <rect x="3" y="6" width="18" height="13" rx="3" />
              <path d="M3 10.5h18" />
            </svg>
          </Link>
          <Link to="/friends" title="Кешбек друзей" className="flex h-[33px] w-[33px] items-center justify-center rounded-[11px] bg-inset">
            <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="var(--t-accl)" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
              <circle cx="9" cy="8.5" r="3.2" />
              <path d="M3.5 19.5c0-3 2.5-4.8 5.5-4.8s5.5 1.8 5.5 4.8" />
              <path d="M16 5.7a3.2 3.2 0 0 1 0 5.6" />
              <path d="M17.5 14.9c1.8.6 3 2 3 4.6" />
            </svg>
          </Link>
        </div>
      </div>

      {/* One search entry for magазины/категории/MCC — works from the very
          first launch, before any bank exists. */}
      <button
        type="button"
        onClick={() => navigate("/search")}
        className="flex h-11 w-full items-center gap-2.5 rounded-2xl border border-brd2 bg-srf2 px-3.5 text-left"
      >
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--t-tx4)" strokeWidth="2.4" strokeLinecap="round" className="flex-none">
          <circle cx="10.5" cy="10.5" r="7" />
          <path d="M16 16l5 5" />
        </svg>
        <span className="text-sm font-medium text-tx4">Магазин, категория или MCC</span>
      </button>

      {roster.length === 0 ? (
        <>
          <FirstRun />
          {feed.length > 0 && (
            <div className="space-y-1.5">
              <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Кешбеки друзей</p>
              {feed.map((it) =>
                it.cat ? (
                  <FeedRow key={it.key} g={it.cat} date={isCurrentMonth ? null : monthDate} friendsOn={friendsOn} />
                ) : (
                  <PartnerFeedRow key={it.key} p={it.partner!} />
                ),
              )}
            </div>
          )}
        </>
      ) : (
        <>
          {unfilled.length > 0 && (
            <Card className="border-acc/30 bg-acc/8 p-3.5" data-sid="CB-01.d">
              <p className="text-[15px] font-extrabold tracking-tight">
                {monthEmpty ? `${monthName[0].toUpperCase()}${monthName.slice(1)} ещё пуст` : "Меню занесены не везде"}
              </p>
              <p className="mt-1 text-[11.5px] leading-snug font-medium text-tx2">
                {monthEmpty
                  ? "Меню месяца не занесены — лента не знает ставок твоих банков."
                  : "Часть банков без меню месяца — их ставок в ленте нет."}
              </p>
              <div className="mt-2.5 space-y-1.5">
                {unfilled.map((c) => (
                  <div key={c.id} className="flex items-center gap-2">
                    <BankBadge name={c.bank_name ?? ""} size={20} />
                    <span className="min-w-0 flex-1 text-xs font-semibold text-tx2">
                      {c.bank_name} · {c.label ?? "Я"}
                    </span>
                    <Btn
                      variant="soft"
                      className="!px-2.5 !py-1.5 text-xs"
                      onClick={() => navigate(`/periods/new?client=${c.id}&month=${monthKey(monthDate)}`)}
                    >
                      Заполнить
                    </Btn>
                  </div>
                ))}
              </div>
            </Card>
          )}

          {(feed.length > 0 || singles.length > 0) && (
            <div className="mx-0.5 flex items-baseline justify-between" data-sid="CB-01.b">
              <span className="text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">
                {monthEmpty ? "Доступно сейчас" : `${categories.length} категорий`}
              </span>
              <span className="flex gap-2.5">
                {(
                  [
                    ["alpha", "по алфавиту"],
                    ["percent", "по проценту"],
                  ] as const
                ).map(([s, label]) => (
                  <button
                    key={s}
                    type="button"
                    onClick={() => setCatsSort(s)}
                    className={`text-[11px] ${catsSort === s ? "font-bold text-accl" : "font-semibold text-tx4"}`}
                  >
                    {label}
                  </button>
                ))}
              </span>
            </div>
          )}

          {feed.length === 0 && singles.length === 0 && unfilled.length === 0 && (
            <Card className="p-4 text-center text-sm font-medium text-tx3">
              Меню занесены, но ничего не выбрано — отметь выборы в «Банках».
            </Card>
          )}

          <div className="space-y-1.5" data-sid="CB-01.c">
            {feed.map((it) =>
              it.cat ? (
                <FeedRow key={it.key} g={it.cat} date={isCurrentMonth ? null : monthDate} friendsOn={friendsOn} />
              ) : (
                <PartnerFeedRow key={it.key} p={it.partner!} />
              ),
            )}

            {singles.length > 0 && (
              <div className="rounded-xl border border-brd bg-srf/60 px-3 py-2.5" data-sid="CB-01.e">
                <button type="button" onClick={() => setShowSingles(!showSingles)} className="flex w-full items-center gap-2 text-left">
                  <span className="min-w-0 flex-1 truncate text-xs font-semibold text-tx4">
                    Только в одном банке · {singles.length} — {singles.map((e) => e.raw_title).join(", ")}
                  </span>
                  <span className="text-[9px] text-tx4">{showSingles ? "▲" : "▼"}</span>
                </button>
                {showSingles && (
                  <div className="mt-2 space-y-2 border-t border-brd/60 pt-2">
                    {singles.map((e, i) => (
                      <div key={i} className="flex items-center gap-2">
                        <BankBadge name={e.bank_name} size={18} />
                        <span className="min-w-0 flex-1 truncate text-xs font-semibold text-tx2">
                          {e.raw_title}
                          <span className="font-medium text-tx4"> · {e.bank_name}{e.holder_label ? ` · ${e.holder_label}` : ""}</span>
                        </span>
                        <Pct percent={e.percent} currency={e.currency_kind} className="text-[13px]" />
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {data.base && (
              <ListRow
                emoji={data.base.emoji || FALLBACK_EMOJI}
                variant="dim"
                onClick={() => navigate("/pos?cat=all-purchases")}
                title={<span className="text-tx3">Остальное — за все покупки</span>}
                sub={
                  <>
                    <BankBadge name={data.base.best.bank_name} size={16} />
                    <span>{data.base.best.bank_name}</span>
                    {data.base.best.holder_label && <span className="text-tx4">· {data.base.best.holder_label}</span>}
                    {data.base.others_count > 0 && <span className="text-tx4">+{data.base.others_count}</span>}
                  </>
                }
                right={
                  <span className="w-11 flex-none text-right text-base font-extrabold text-tx4">
                    {data.base.best.percent != null ? `${data.base.best.percent}%` : "—"}
                  </span>
                }
              />
            )}
          </div>

          {isCurrentMonth && data.selection_opens_day != null && (
            <p className="text-center text-[10.5px] font-medium text-tx4">
              Ритуал 25-го живёт в «Банках» — там же меню следующего месяца.
            </p>
          )}
        </>
      )}
    </>
  );
}
