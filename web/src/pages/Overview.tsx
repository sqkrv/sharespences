import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
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

// Percent as a number for display ordering; unknown last. Nominal across
// currencies — the same rule the боards use (2026-08-27): ordering is not
// conversion.
function pctNum(p?: string | null): number {
  return p != null ? parseFloat(p) : -1;
}

// The row's displayed winner: the best rate the row can honestly show —
// 9a's own rule, «передний логотип — банк с максимальным процентом, ему и
// принадлежит цифра». A friend's 9% must not front a row that has a
// still-pickable 10% below it (feedback 2026-08-28). Ties resolve by the
// least action needed: an own selected card already pays, a friend's needs
// asking, a «свободный слот» needs picking first.
function winnerOf(g: CategoryGroup, friendsOn: boolean): { entry: LookupEntry; state: "friend" | "own" | "available" | "friend-available" } | null {
  const candidates: { entry: LookupEntry; state: "friend" | "own" | "available"; prio: number }[] = [];
  if (g.best) candidates.push({ entry: g.best, state: "own", prio: 0 });
  if (friendsOn && g.friend_best) candidates.push({ entry: g.friend_best, state: "friend", prio: 1 });
  if (g.available) candidates.push({ entry: g.available, state: "available", prio: 2 });
  if (candidates.length === 0) {
    // Nothing of the viewer's own, and no friend has picked here — but a
    // friend still holds the category unpicked. It fronts the row rather than
    // dropping it, and only here: a rate nobody has taken must never outrank
    // a card that already pays.
    if (friendsOn && g.friend_available) return { entry: g.friend_available, state: "friend-available" };
    return null;
  }
  candidates.sort((a, b) => pctNum(b.entry.percent) - pctNum(a.entry.percent) || a.prio - b.prio);
  return candidates[0];
}

// Gold mechanic chip for a winner row: the stacked барабан shows its parts
// («7 + 7 барабан» — the sum is only trustworthy if it shows them), a bare
// super is «барабан», a special carries its own title («спец · Остатки»).
function mechanicChip(e: LookupEntry) {
  if (e.stacked_super != null) return <Chip tone="gold">{e.stacked_regular} + {e.stacked_super} барабан</Chip>;
  if (e.kind === "super") return <Chip tone="gold">барабан</Chip>;
  if (e.kind === "special") return <Chip tone="gold">спец{e.raw_title ? ` · ${e.raw_title}` : ""}</Chip>;
  return null;
}

// The row's overlap logo stack (9a): every bank where the category exists
// this month, the displayed winner in front — nearest the percent, last in
// DOM. Behind it the others in rank order, nearer = higher. The 2px ring in
// the surface color is what makes the overlap read as a stack.
function BankStack({ banks, winner }: { banks: string[]; winner: string }) {
  const rest = banks.filter((b) => b !== winner);
  const shown = [...rest.slice(0, 3).reverse(), winner];
  return (
    <span className="flex flex-none">
      {shown.map((b, i) => (
        <span key={b} className="flex flex-none" style={{ marginLeft: i ? -7 : 0, borderRadius: 7, boxShadow: "0 0 0 2px var(--t-srf)" }}>
          <BankBadge name={b} size={18} />
        </span>
      ))}
    </span>
  );
}

// The 3a expansion: the category's full ranking (rubles, then points — never
// converted), plus the «можно выбрать» rows. Lazily fetched on first expand;
// usePrefetchOffline warms the active slugs for offline. Rows navigate to
// the bank's menu — marking a selection lives there, not here (feedback
// 2026-08-25, same rule as CB-11).
function ExpandedCategory({
  slug,
  date,
  friendsOn,
  openEntry,
}: {
  slug: string;
  date: string | null;
  friendsOn: boolean;
  openEntry: (e: { bank_client_id?: number; friend_name?: string; kind?: string }) => void;
}) {
  // date null = the current month: the key then matches what
  // usePrefetchOffline warmed, so expansion works at a no-signal checkout.
  const lookup = useQuery({
    queryKey: date ? ["lookup", slug, date] : ["lookup", slug],
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/cashback/lookup", { params: { query: { category: slug, ...(date ? { date } : {}) } } })),
  });

  if (lookup.isPending) return <Spinner />;
  if (lookup.isError) return <ErrMsg error={lookup.error} />;
  const d = lookup.data;
  const ranked = (d.ranked ?? []).filter((e) => friendsOn || !e.friend_name);
  const available = d.available ?? [];
  // A friend's unpicked rows land here rather than on the collapsed row: this
  // is where the per-bank picture lives, and «попроси Марину» is detail, not
  // the answer to «чем платить».
  const friendAvailable = friendsOn ? (d.friend_available ?? []) : [];
  const currencies = new Set(ranked.map((e) => e.currency_kind));

  return (
    <div className="mt-2.5 ml-8 space-y-2 border-t border-brd/60 pt-2.5" data-sid="CB-01.f">
      {/* One list, nominal percent descending (feedback 2026-08-28) — a
          10% «свободный слот» must not hide under a 9% selected row. The
          outlined dot + «свободный слот» is what tells the states apart;
          every served available row is pickable (the API drops dead ends),
          picking happens in the bank menu. */}
      {[
        ...ranked.map((e, i) => ({ key: `r-${e.bank_client_id}-${i}`, avail: false as const, unpicked: false, e })),
        ...available.map((e) => ({ key: `a-${e.offer_id}`, avail: true as const, unpicked: false, e })),
        ...friendAvailable.map((e, i) => ({ key: `f-${e.bank_client_id}-${i}`, avail: false as const, unpicked: true, e })),
      ]
        .sort((a, b) => pctNum(b.e.percent) - pctNum(a.e.percent))
        .map(({ key, avail, unpicked, e }) => (
          <button
            key={key}
            type="button"
            onClick={() => openEntry(e)}
            className={`flex w-full items-center gap-2 text-left ${unpicked ? "opacity-60" : ""}`}
          >
            <BankBadge name={e.bank_name} size={18} />
            <span className="min-w-0 flex-1 truncate text-xs font-semibold text-tx2">
              {e.bank_name}
              {e.holder_label ? ` · ${e.holder_label}` : e.friend_name || e.kind === "partner" || avail ? "" : " · Я"}
              {avail && (
                <span className="ml-1.5 inline-flex items-baseline gap-1 text-[10px] font-semibold text-tx3">
                  <span className="h-1.5 w-1.5 flex-none self-center rounded-full border-[1.5px] border-tx4" />
                  свободный слот
                </span>
              )}
              {avail && verdictNote(e as Schemas["AvailableEntryDTO"]) && (
                <span className="ml-1 text-[10px] font-medium text-tx4">· {verdictNote(e as Schemas["AvailableEntryDTO"])}</span>
              )}
              {!avail && e.friend_name && <span className="ml-1.5"><Chip tone="friend">друг · {e.friend_name}</Chip></span>}
              {/* Same marker their own unpicked rows carry, so one legend
                  covers both: the outlined dot means «в меню, не выбрано». */}
              {unpicked && (
                <span className="ml-1.5 inline-flex items-baseline gap-1 text-[10px] font-semibold text-tx3">
                  <span className="h-1.5 w-1.5 flex-none self-center rounded-full border-[1.5px] border-tx4" />
                  не выбрано
                </span>
              )}
              {!avail && e.kind === "partner" && (
                <span className="ml-1.5">
                  <Chip tone="gold">партнёрка{e.partner_scope === "merchant" ? ` · только в «${e.raw_title}»` : ""}</Chip>
                </span>
              )}
              {e.currency_kind === "points" && <span className="ml-1.5"><Chip tone="points">{e.points_label || "баллы"}</Chip></span>}
              {!avail && !e.friend_name && e.kind !== "partner" && capNote(e) && (
                <span className="font-medium text-tx4"> · {capNote(e)}</span>
              )}
            </span>
            <Pct percent={e.percent} currency={e.currency_kind} className="text-[13px]" />
            <span className="flex-none text-[10px] text-tx4">›</span>
          </button>
        ))}
      {currencies.has("points") && currencies.size > 1 && (
        <p className="text-[10px] leading-snug font-medium text-tx4">
          Баллы в рубли не пересчитываются — лиловый процент считается баллами.
        </p>
      )}
    </div>
  );
}

// One feed row — the final anatomy (9a, 8d-2): single line, no bank name.
// Status chips ride the title's tail; the right side is the overlap logo
// stack with the percent, and the держатель/друг caption sits under them.
// Full bank names live a tap away, in the expansion.
function FeedRow({
  g,
  date,
  friendsOn,
  openEntry,
}: {
  g: CategoryGroup;
  date: string | null;
  friendsOn: boolean;
  openEntry: (e: { bank_client_id?: number; friend_name?: string; kind?: string }) => void;
}) {
  const [expanded, setExpanded] = useState(false);
  const w = winnerOf(g, friendsOn);
  if (!w) return null;
  const { entry: e, state } = w;
  const variant = state === "friend" ? "friend" : state === "available" || state === "friend-available" ? "dashed" : "solid";
  const stackBanks = (g.bank_stack ?? []).filter((b) => friendsOn || !b.friend).map((b) => b.bank_name);
  // A friend's unpicked row says itself: the dashed border means nobody has
  // taken this, the caption says whose menu it is, and the logo joins the
  // stack. Words on top of that were the third copy of one fact.
  const availChip = state === "available" ? verdictNote(g.available!) || "свободен слот" : "";
  return (
    <ListRow
      emoji={g.emoji || FALLBACK_EMOJI}
      variant={variant}
      onClick={() => setExpanded(!expanded)}
      title={
        <>
          {g.title_ru}
          {mechanicChip(e) && <span className="ml-1.5 align-[1px]">{mechanicChip(e)}</span>}
          {availChip && <span className="ml-1.5 align-[1px]"><Chip tone="friend">{availChip}</Chip></span>}
        </>
      }
      right={
        <span className="flex flex-none flex-col items-end gap-0.5">
          <span className="flex items-center gap-2">
            <BankStack banks={stackBanks} winner={e.bank_name} />
            <Pct percent={e.percent} currency={e.currency_kind} className="text-base" />
          </span>
          {/* One place names the friend. Which of the two friend states this
              is comes from the border: dashed = nobody has taken it. Note
              the explicit branch — `holder_label` on a friend's entry is
              THEIR держатель, and falling through would print it as if the
              viewer had a card there. */}
          {state === "friend" || state === "friend-available" ? (
            <span className="text-[10px] font-bold text-accl">друг · {e.friend_name}</span>
          ) : (
            e.holder_label && <span className="text-[10px] font-semibold text-tx4">{e.holder_label}</span>
          )}
        </span>
      }
    >
      {expanded && <ExpandedCategory slug={g.slug} date={date} friendsOn={friendsOn} openEntry={openEntry} />}
    </ListRow>
  );
}

// A партнёрка feed row (v2, 9a): the gold frame and the ★ already say what
// it is — no word, only the term chip («по 31.08»), none without a date.
// The bank is its logo by the percent. Tap opens that bank's menu, the same
// answer every other feed row gives.
function PartnerFeedRow({ p, onOpen }: { p: PartnerFeed; onOpen: () => void }) {
  return (
    <ListRow
      lead={<span className="flex h-[21px] w-[21px] flex-none items-center justify-center rounded-md bg-gold/15 text-[11px] font-extrabold text-gold">★</span>}
      variant="gold"
      onClick={onOpen}
      title={
        <>
          {p.raw_title}
          {p.valid_to && <span className="ml-1.5 align-[1px]"><Chip tone="gold">по {fmtDate(p.valid_to)}</Chip></span>}
          {p.needs_activation && <span className="ml-1.5 align-[1px]"><Chip tone="gold">требует активации</Chip></span>}
        </>
      }
      right={
        <span className="flex flex-none items-center gap-2">
          <BankBadge name={p.bank_name} size={18} />
          <Pct percent={p.percent} currency={p.currency_kind} className="text-base" />
        </span>
      }
    />
  );
}

// «За все покупки» is an ordinary feed row since 9a — alphabetized among
// the rest, not a dim tail. Tap answers as the точка продаж does.
function BaseFeedRow({ b }: { b: Schemas["OverviewBaseDTO"] }) {
  const navigate = useNavigate();
  const e = b.best;
  return (
    <ListRow
      emoji={b.emoji || FALLBACK_EMOJI}
      variant="solid"
      onClick={() => navigate("/pos?cat=all-purchases")}
      title="За все покупки"
      right={
        <span className="flex flex-none flex-col items-end gap-0.5">
          <span className="flex items-center gap-2">
            <BankStack banks={(b.bank_stack ?? []).map((s) => s.bank_name)} winner={e.bank_name} />
            <Pct percent={e.percent} currency={e.currency_kind} className="text-base" />
          </span>
          {e.holder_label && <span className="text-[10px] font-semibold text-tx4">{e.holder_label}</span>}
        </span>
      }
    />
  );
}

// Interleave category, партнёрка and base rows without breaking invariant
// 5: the percent sort merges by (currency group, percent desc) — the lists
// arrive from the API already in that order — and the alphabet sort is by
// name. «За все покупки» rides the same list since 9a.
type FeedItem = {
  key: string;
  title: string;
  entry: LookupEntry;
  cat?: CategoryGroup;
  partner?: PartnerFeed;
  base?: Schemas["OverviewBaseDTO"];
};

function mergeFeed(
  categories: CategoryGroup[],
  partners: PartnerFeed[],
  base: Schemas["OverviewBaseDTO"] | undefined,
  sort: CatsSort,
  friendsOn: boolean,
): FeedItem[] {
  const items: FeedItem[] = [];
  for (const g of categories) {
    const w = winnerOf(g, friendsOn);
    if (w) items.push({ key: `c${g.category_id}`, title: g.title_ru, entry: w.entry, cat: g });
  }
  for (const p of partners) {
    items.push({ key: `p${p.partner_id}`, title: p.raw_title, entry: p, partner: p });
  }
  if (base) items.push({ key: "base", title: "За все покупки", entry: base.best, base });
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
  // Banks that have opened a selection whose menu is still empty. The rule is
  // the server's (PendingMenu), the same field CB-09 marks its bank rows
  // with, so both screens speak about one fact. A period for the month being
  // viewed is left out: the list above already says that one.
  const openings = (data.clients ?? [])
    .filter((c) => c.pending_from != null && monthKey(c.pending_from) !== monthKey(monthDate))
    .sort((a, b) => a.pending_from!.localeCompare(b.pending_from!) || a.bank_name.localeCompare(b.bank_name));
  const openingMonths = new Set(openings.map((c) => monthKey(c.pending_from!)));
  const monthEmpty = roster.length > 0 && filledClientIDs.size === 0;

  const feed = mergeFeed(categories, data.partners ?? [], data.base ?? undefined, catsSort, friendsOn);

  // A ranking row leads to its bank's menu for the viewed month (feedback
  // 2026-08-25, same rule as CB-11): marking a selection lives there. A
  // friend's menu isn't ours to open. A партнёрка is a row of the same bank
  // and answers the same way — it used to divert to the bank list, which
  // made one row in the feed behave unlike its neighbours.
  const openClient = (clientID?: number) => {
    const c = (data.clients ?? []).find((x) => x.bank_client_id === clientID);
    // No client on the row means there is no one menu to open — a bank-wide
    // партнёрка has none, and the API now omits the field rather than
    // sending 0. Its home is the bank card it is edited from. Falling
    // through silently made the row unclickable instead.
    if (c == null) {
      navigate("/banks");
      return;
    }
    if (c.period_id != null) navigate(`/periods/${c.period_id}`);
    else navigate(`/periods/new?client=${c.bank_client_id}&month=${monthKey(monthDate)}`);
  };
  const openEntry = (e: { bank_client_id?: number; friend_name?: string; kind?: string }) => {
    if (e.friend_name) navigate("/friends");
    else openClient(e.bank_client_id);
  };

  return (
    <>
      <div className="flex items-center justify-between gap-2.5" data-sid="CB-01.a">
        <h1 className="text-[23px] font-extrabold tracking-tight">Кешбек</h1>
        <div className="flex items-center gap-2">
          {roster.length > 0 && <MonthPicker value={monthDate} onChange={setMonthDate} />}
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
          first launch, before any bank exists. A real input, not a styled
          button: iOS opens the keyboard only for a focused editable field
          inside the tap gesture, so this field takes the focus and CB-04's
          autoFocus inherits the already-open keyboard after the hop. */}
      <div className="flex h-11 w-full items-center gap-2.5 rounded-2xl border border-brd2 bg-srf2 px-3.5">
        <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--t-tx4)" strokeWidth="2.4" strokeLinecap="round" className="flex-none">
          <circle cx="10.5" cy="10.5" r="7" />
          <path d="M16 16l5 5" />
        </svg>
        <input
          value=""
          onChange={() => {}}
          onFocus={() => navigate("/search")}
          placeholder="Магазин, категория или MCC"
          inputMode="search"
          className="min-w-0 flex-1 bg-transparent text-sm font-medium outline-none placeholder:text-tx4"
        />
      </div>

      {roster.length === 0 ? (
        <>
          <FirstRun />
          {feed.length > 0 && (
            <div className="space-y-1.5">
              <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Кешбеки друзей</p>
              {feed.map((it) =>
                it.cat ? (
                  <FeedRow key={it.key} g={it.cat} date={isCurrentMonth ? null : monthDate} friendsOn={friendsOn} openEntry={openEntry} />
                ) : it.base ? (
                  <BaseFeedRow key={it.key} b={it.base} />
                ) : (
                  <PartnerFeedRow key={it.key} p={it.partner!} onOpen={() => openEntry(it.partner!)} />
                ),
              )}
            </div>
          )}
        </>
      ) : (
        <>
          {(unfilled.length > 0 || openings.length > 0) && (
            <Card className="border-acc/30 bg-acc/8 p-3.5" data-sid="CB-01.d">
              {unfilled.length > 0 && (
                <>
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
                </>
              )}
              {/* The 25th, where it is actionable. The ритуал used to be a
                  sentence pointing at «Банки»; naming the banks that opened
                  and handing each one its own button is the same hint with
                  the trip removed. The month rides on the row when several
                  differ (a quarterly program opens its own period). */}
              {openings.length > 0 && (
                <div className={unfilled.length > 0 ? "mt-3 border-t border-brd/60 pt-2.5" : ""}>
                  <p className="text-[15px] font-extrabold tracking-tight">
                    Открыт выбор
                    {openingMonths.size === 1 ? ` на ${monthNameOf(openings[0].pending_from!)}` : ""}
                  </p>
                  <p className="mt-1 text-[11.5px] leading-snug font-medium text-tx2">
                    Выбери категории в приложении банка и отметь здесь.
                  </p>
                  <div className="mt-2.5 space-y-1.5">
                    {openings.map((c) => (
                      <div key={c.bank_client_id} className="flex items-center gap-2">
                        <BankBadge name={c.bank_name} size={20} />
                        <span className="min-w-0 flex-1 text-xs font-semibold text-tx2">
                          {c.bank_name} · {c.holder_label ?? "Я"}
                          {openingMonths.size > 1 && (
                            <span className="font-medium text-tx4"> · {monthNameOf(c.pending_from!)}</span>
                          )}
                        </span>
                        <Btn
                          variant="soft"
                          className="!px-2.5 !py-1.5 text-xs"
                          onClick={() => navigate(`/periods/new?client=${c.bank_client_id}&month=${monthKey(c.pending_from!)}`)}
                        >
                          Заполнить
                        </Btn>
                      </div>
                    ))}
                  </div>
                </div>
              )}
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
                <FeedRow key={it.key} g={it.cat} date={isCurrentMonth ? null : monthDate} friendsOn={friendsOn} openEntry={openEntry} />
              ) : it.base ? (
                <BaseFeedRow key={it.key} b={it.base} />
              ) : (
                <PartnerFeedRow key={it.key} p={it.partner!} onOpen={() => openEntry(it.partner!)} />
              ),
            )}

            {singles.length > 0 && (
              // The bank's own rows: menu rows with no canonical category, so
              // nothing across banks compares with them. They used to sit in a
              // collapsed fold that inlined every title into one truncated
              // line — unreadable past a handful, and its «только в одном
              // банке» heading was not even true (the same title exists at two
              // banks often enough). One labelled section at the end instead:
              // always open, ordinary rows, and the label states the fact once
              // rather than a chip repeating it on each row. They stay a group
              // rather than sorting into the list above, because that list is
              // «which card pays most for X» across banks, and these answer a
              // different question.
              <div className="space-y-1.5 pt-1" data-sid="CB-01.e">
                <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Свои категории банков</p>
                {singles.map((e, i) => (
                  <ListRow
                    key={`s-${e.bank_client_id ?? 0}-${i}`}
                    emoji={e.emoji || FALLBACK_EMOJI}
                    variant="solid"
                    onClick={() => openEntry(e)}
                    title={
                      <>
                        {e.raw_title}
                        {mechanicChip(e) && <span className="ml-1.5 align-[1px]">{mechanicChip(e)}</span>}
                      </>
                    }
                    right={
                      <span className="flex flex-none flex-col items-end gap-0.5">
                        <span className="flex items-center gap-2">
                          <BankBadge name={e.bank_name} size={18} />
                          <Pct percent={e.percent} currency={e.currency_kind} className="text-base" />
                        </span>
                        {e.holder_label && <span className="text-[10px] font-semibold text-tx4">{e.holder_label}</span>}
                      </span>
                    }
                  />
                ))}
              </div>
            )}

          </div>

        </>
      )}
    </>
  );
}
