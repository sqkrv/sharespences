import { useEffect, useState } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type LookupEntry, type Schemas } from "../api/client";
import { Navigate, useNavigate, useSearchParams } from "react-router-dom";
import { useBanks, useCards, useCategories } from "../hooks";
import { BackButton, BankBadge, Card, Chip, ErrMsg, Pct, Spinner } from "../components/ui";
import {
  FALLBACK_EMOJI,
  FRIENDS_KEY,
  POS_TYPE_RU,
  capNote,
  currencyWord,
  fmtDate,
  initWithFriends,
  monthKey,
  plural,
  todayISO,
  verdictNote,
} from "../lib";
import { pushRecent } from "../recent";

// CB-11 «Точка продаж» (redesign 12a): «О точке» first — the confirmation
// «это та самая точка» — then one ranked leaderboard «Чем платить — по
// убыванию», no hero. Every row got here through EXACT per-bank matching
// (10b variant 3): the bank's own category holds the point's MCC. Banks
// without ingested MCC memberships fall to the «Кешбек на всё» fold —
// approximate ranking on an MCC screen was rejected outright (2026-08-27).
//
// The board ranks by the nominal percent across currencies (owner decision
// 2026-08-27): ordering is not conversion, so invariant 5 stands — the
// lilac percent and «баллами» wording carry the currency.
//
// The category context (?cat=, from the feed's base row and search) keeps
// the canonical lookup and the 2b verdict-row layout.

type MccBoard = {
  ranked?: LookupEntry[] | null;
  available?: Schemas["AvailableEntryDTO"][] | null;
  blocked?: Schemas["AvailableEntryDTO"][] | null;
  friend_available?: Schemas["AvailableEntryDTO"][] | null;
  base?: LookupEntry[] | null;
};

// «7+7 барабан» — the stacked pair in a row's sub-line.
function stackShort(e: LookupEntry): string {
  if (e.stacked_super == null) return "";
  return `${e.stacked_regular ?? "—"}+${e.stacked_super} барабан`;
}

// The cap in the row vocabulary: «до 7000₽», not «лимит 7000₽».
function capShort(e: LookupEntry): string {
  return capNote(e).replace(/^лимит /, "до ");
}

// The state vocabulary of a board row (2b v3 / 12a).
function stateOf(e: LookupEntry): { dot: string; word: string; tone: string } {
  if (e.kind === "super" || e.kind === "special") return { dot: "bg-gold", word: "выдано банком", tone: "text-gold" };
  return { dot: "bg-mint", word: "выбрана", tone: "text-mint" };
}

const CHEVRON = (
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--t-tx4)" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" className="flex-none">
    <path d="M9 5l7 7-7 7" />
  </svg>
);

// Address behaviour (ТУР 11 rule): офлайн leads to the map, онлайн IS the
// site link. Both are outbound links the user taps — never loaded resources
// (policy §2.4 governs loading, not linking).
function addressHref(type: string | undefined | null, address: string): string | undefined {
  if (type === "online") return /^https?:\/\//.test(address) ? address : `https://${address}`;
  if (type === "offline") return `https://yandex.ru/maps/?text=${encodeURIComponent(address)}`;
  return undefined;
}

// The navigation shared by board rows: a bank's row opens that bank's menu
// for today's month; a friend's menu isn't ours to open; партнёрки go to
// their bank-card home.
function useOpenEntry() {
  const navigate = useNavigate();
  const qc = useQueryClient();
  // The roster is resolved when a row is TAPPED, not on mount: fetchQuery
  // answers from the cache while it is fresh and fetches otherwise, so «the
  // roster has not arrived yet» stops being a state a click can land in.
  // Before this, a tap in that window sent every row to /banks — and this key
  // is not the feed's ["overview", date], so nothing the feed fetched ever
  // filled it.
  const openClient = async (clientID?: number) => {
    let clients: Schemas["OverviewClientDTO"][] = [];
    try {
      const data = await qc.fetchQuery({
        queryKey: ["overview"],
        queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview")),
        staleTime: 60_000,
      });
      clients = data.clients ?? [];
    } catch {
      navigate("/banks"); // the roster is unreachable; its own screen is the honest destination
      return;
    }
    const c = clients.find((x) => x.bank_client_id === clientID);
    // Same rule as the feed: a row with no client — a bank-wide партнёрка,
    // whose bank_client_id the API omits — has no one menu to open, so it
    // goes to the bank card instead of nowhere.
    if (c == null) {
      navigate("/banks");
      return;
    }
    if (c.period_id != null) navigate(`/periods/${c.period_id}`);
    else navigate(`/periods/new?client=${c.bank_client_id}&month=${monthKey(todayISO())}`);
  };
  const openEntry = (e: { bank_client_id?: number; friend_name?: string; kind?: string }) => {
    if (e.friend_name) navigate("/friends");
    else void openClient(e.bank_client_id);
  };
  return { openClient, openEntry };
}

// «О точке» (12a): everything the base knows, first on the screen — MCC row
// linking to the code screen, memberships as category chips, the statement
// string, the channel, the address as a link, the record's freshness.
export function AboutPoint({
  mcc,
  point,
}: {
  mcc: string | null;
  point?: Schemas["MerchantDTO"];
}) {
  const navigate = useNavigate();
  const resolve = useQuery({
    queryKey: ["mcc-resolve", mcc],
    enabled: mcc != null,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/resolve", { params: { query: { code: mcc! } } })),
  });
  const canonicals = resolve.data?.canonicals ?? [];
  const categories = useCategories();
  const emojiOf = (slug?: string | null) => (categories.data ?? []).find((c) => c.slug === slug)?.emoji;
  const addr = point?.address;
  const href = addr ? addressHref(point?.type, addr) : undefined;

  const rows: [string, React.ReactNode][] = [];
  if (mcc && resolve.data) {
    rows.push([
      "MCC",
      <button key="mcc" type="button" className="text-left" onClick={() => navigate(`/mcc/${mcc}`)}>
        <span className="font-mono font-extrabold text-accl">{resolve.data.code.code}</span>
        <span className="text-tx2"> {resolve.data.code.name}</span>
        <span className="text-tx4"> ›</span>
      </button>,
    ]);
  }
  if (canonicals.length > 0) {
    rows.push([
      "категория",
      canonicals.map((c) => `${emojiOf(c.slug) || FALLBACK_EMOJI} ${c.title}`).join(" · "),
    ]);
  }
  if (point?.merchant_title) rows.push(["в выписке", <span key="mt" className="font-mono tracking-wide">{point.merchant_title}</span>]);
  if (point?.type) rows.push(["канал", POS_TYPE_RU[point.type] ?? point.type]);
  if (addr) {
    rows.push([
      "адрес",
      href ? (
        <a key="addr" href={href} target="_blank" rel="noreferrer" className="text-accl underline decoration-accl/40">
          {addr}
        </a>
      ) : (
        addr
      ),
    ]);
  }
  if (point) {
    // The credit rides the row it is true of (origin, 00027): scrape rows
    // name mcc-codes.ru — the license's point of use — the rest name who
    // actually wrote them.
    const source =
      point.origin === "mcc_codes"
        ? "mcc-codes.ru"
        : point.origin === "admin"
          ? "каталог Sharespences"
          : point.origin === "user_transaction"
            ? "из операций пользователей"
            : "добавлена пользователем";
    rows.push([
      "данные",
      [
        source,
        Number(point.confirmations) > 0
          ? `подтвердили ${point.confirmations} ${plural(Number(point.confirmations), "человек", "человека", "человек")}`
          : "пока без подтверждений",
        point.last_confirmed_at && `обновлено ${fmtDate(point.last_confirmed_at.slice(0, 10))}`,
      ]
        .filter(Boolean)
        .join(" · "),
    ]);
  }
  if (rows.length === 0) return null;
  return (
    <Card className="space-y-1.5 p-3.5" data-sid="CB-11.g">
      <p className="text-[10px] font-extrabold tracking-[.14em] text-tx3 uppercase">О точке</p>
      {rows.map(([label, value]) => (
        <div key={label} className="flex items-baseline gap-2">
          <dt className="w-[78px] flex-none text-[10px] font-medium tracking-[.06em] text-tx4 uppercase">{label}</dt>
          <dd className="min-w-0 flex-1 text-[12px] font-semibold text-tx2">{value}</dd>
        </div>
      ))}
    </Card>
  );
}

// One board row's percent as a number, unknown last.
function pctOf(p?: string | null): number {
  return p != null ? parseFloat(p) : -1;
}

// «В меню, но не выбрано»: rows the bank does hold for this answer, in a
// period that can no longer take them — slots full, or a one-shot menu
// already confirmed. Dead ends, so they stay out of the percent leaderboard
// (they pay nothing here) and carry no CTA; what they carry is the fact the
// board looked broken without — «у этого банка эта категория есть, просто не
// выбрана». The client's real answer for this point stays in the base fold
// below. A tap opens the bank's menu, where next period is picked.
function BlockedRows({ rows, friendRows = [] }: { rows: Schemas["AvailableEntryDTO"][]; friendRows?: Schemas["AvailableEntryDTO"][] }) {
  const { openClient } = useOpenEntry();
  if (rows.length === 0 && friendRows.length === 0) return null;
  return (
    <>
      <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">В меню, но не выбрано</p>
      <div className="space-y-1.5" data-sid="CB-11.h">
        {rows.map((e) => (
          <button
            key={e.offer_id}
            type="button"
            onClick={() => openClient(e.bank_client_id)}
            className="flex w-full items-center gap-2.5 rounded-2xl border border-brd bg-srf/45 px-3 py-2.5 text-left opacity-65"
          >
            <BankBadge name={e.bank_name} size={26} />
            <div className="min-w-0 flex-1">
              <p className="text-[13.5px] font-bold">
                {e.bank_name}
                {e.holder_label && <span className="font-semibold text-tx4"> · {e.holder_label}</span>}
              </p>
              <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10.5px] font-medium">
                <span className="h-1.5 w-1.5 flex-none rounded-full bg-tx4" />
                <span className="font-semibold text-tx3">{verdictNote(e)}</span>
                <span className="text-tx4">
                  {e.emoji && `${e.emoji} `}«{e.raw_title}»
                </span>
              </p>
            </div>
            <Pct percent={e.percent} currency={e.currency_kind} className="text-[13px]" />
            {CHEVRON}
          </button>
        ))}
        {/* A friend's shared row holding this code that they have not picked.
            Not a dead end like the rows above — they can still pick it — so it
            carries the friend's name and no chevron: the action is asking, and
            it is not the viewer's to take. */}
        {friendRows.map((e, i) => (
          <div
            key={`f-${i}-${e.bank_name}-${e.raw_title}`}
            className="flex w-full items-center gap-2.5 rounded-2xl border border-dashed border-accl/45 bg-srf/45 px-3 py-2.5 text-left"
          >
            <BankBadge name={e.bank_name} size={26} />
            <div className="min-w-0 flex-1">
              <p className="text-[13.5px] font-bold">
                {e.bank_name}
                <span className="font-semibold text-accl"> · {e.friend_name}</span>
              </p>
              <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10.5px] font-medium">
                <span className="h-1.5 w-1.5 flex-none rounded-full bg-accl" />
                <span className="font-semibold text-accl">можно попросить выбрать</span>
                <span className="text-tx4">
                  {e.emoji && `${e.emoji} `}«{e.raw_title}»
                </span>
              </p>
            </div>
            <Pct percent={e.percent} currency={e.currency_kind} className="text-[13px]" />
          </div>
        ))}
      </div>
    </>
  );
}

type BoardRow =
  | { key: string; kind: "entry"; percent?: string | null; e: LookupEntry }
  | { key: string; kind: "avail"; percent?: string | null; a: Schemas["AvailableEntryDTO"] }
  | { key: string; kind: "partner"; percent?: string | null; m: LookupEntry };

// The «Чем платить — по убыванию» leaderboard (12a): selected rows, the
// name-matched партнёрки and the still-pickable «свободный слот» rows in one
// list, nominal percent descending. The winner is the top row — bank-color
// stripe and a bigger rate, no hero plaque.
export function Leaderboard({ board, matches, sid }: { board: MccBoard; matches: LookupEntry[]; sid?: string }) {
  const [withFriends, setWithFriends] = useState(initWithFriends);
  const [showBase, setShowBase] = useState(false);
  const banks = useBanks();
  const { openClient, openEntry } = useOpenEntry();
  const colorOf = (bank: string) => (banks.data ?? []).find((b) => b.name === bank)?.color_hex ?? undefined;

  const ranked = (board.ranked ?? []).filter((e) => withFriends || !e.friend_name);
  // The toggle has to know about every friend surface, not just the ranked
  // one: a friend who has the row but has not picked it is exactly the case
  // where their name would otherwise be unhideable.
  const hasFriends = (board.ranked ?? []).some((e) => e.friend_name) || (board.friend_available ?? []).length > 0;
  const rows: BoardRow[] = [
    ...ranked.map((e, i): BoardRow => ({ key: `e${i}`, kind: "entry", percent: e.percent, e })),
    ...matches.map((m, i): BoardRow => ({ key: `p${i}`, kind: "partner", percent: m.percent, m })),
    ...(board.available ?? []).map((a): BoardRow => ({ key: `a${a.offer_id}`, kind: "avail", percent: a.percent, a })),
  ].sort((x, y) => pctOf(y.percent) - pctOf(x.percent));
  const base = board.base ?? [];
  const blocked = board.blocked ?? [];
  const friendAvailable = withFriends ? (board.friend_available ?? []) : [];

  // friendAvailable belongs in this guard: it is served independently of the
  // viewer's own rows, so «your banks say nothing, a friend could pick it» is
  // precisely the state that used to render «Точных ответов нет».
  if (rows.length === 0 && base.length === 0 && blocked.length === 0 && friendAvailable.length === 0) {
    return (
      <Card className="space-y-1.5 p-4 text-center">
        <p className="text-sm font-semibold text-tx2">Точных ответов нет</p>
        <p className="text-[10.5px] font-medium text-tx4">
          Банк попадает сюда, когда известно, что он считает этот код в одной из категорий своего меню.
        </p>
      </Card>
    );
  }

  return (
    <>
      {hasFriends && (
        <button
          type="button"
          data-sid="CB-11.d"
          onClick={() => {
            localStorage.setItem(FRIENDS_KEY, withFriends ? "off" : "on");
            setWithFriends(!withFriends);
          }}
          className={`flex w-full items-center justify-between rounded-xl border px-3 py-2 text-[12px] font-semibold transition ${
            withFriends ? "border-acc/40 bg-acc/10 text-accl" : "border-brd2 bg-srf2 text-tx3"
          }`}
        >
          Карты друзей в подборе
          <span className={`h-5 w-9 flex-none rounded-full p-0.5 transition ${withFriends ? "bg-acc" : "bg-inset"}`}>
            <span className={`block h-4 w-4 rounded-full bg-white transition ${withFriends ? "translate-x-4" : ""}`} />
          </span>
        </button>
      )}

      {/* The label belongs to the list — with nothing rankable it used to
          head an empty div, which now reads as a section that lost its
          rows above «В меню, но не выбрано». */}
      {rows.length > 0 && (
        <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Чем платить — по убыванию</p>
      )}
      <div className="space-y-1.5" data-sid={sid}>
        {rows.map((row, i) => {
          const lead = i === 0;
          const pctCls = lead ? "text-[19px]" : "text-[15px]";
          const stripe = lead
            ? { borderLeft: `3px solid ${(row.kind === "entry" && colorOf(row.e.bank_name)) || (row.kind === "avail" && colorOf(row.a.bank_name)) || (row.kind === "partner" && colorOf(row.m.bank_name)) || "var(--t-acc)"}` }
            : undefined;
          if (row.kind === "partner") {
            const m = row.m;
            return (
              <button
                key={row.key}
                type="button"
                onClick={() => openEntry(m)}
                style={stripe}
                className="flex w-full items-center gap-2.5 rounded-2xl border border-gold/30 bg-gold/5 px-3 py-2.5 text-left"
              >
                <span className="flex h-[26px] w-[26px] flex-none items-center justify-center rounded-[9px] bg-gold/15 text-xs font-extrabold text-gold">★</span>
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px] font-semibold text-gold">{m.raw_title}</p>
                  <p className="truncate text-[10px] font-medium text-tx4">
                    {[m.bank_name, "совпадение по названию", m.needs_activation && "требует активации"].filter(Boolean).join(" · ")}
                  </p>
                </div>
                <Pct percent={m.percent} currency={m.currency_kind} className={pctCls} />
                {CHEVRON}
              </button>
            );
          }
          if (row.kind === "avail") {
            const a = row.a;
            return (
              <button
                key={row.key}
                type="button"
                onClick={() => openClient(a.bank_client_id)}
                style={stripe}
                className="flex w-full items-center gap-2.5 rounded-2xl border border-dashed border-dash bg-srf/50 px-3 py-2.5 text-left"
              >
                <BankBadge name={a.bank_name} size={26} />
                <div className="min-w-0 flex-1">
                  <p className="text-[13.5px] font-bold">
                    {a.bank_name}
                    {a.holder_label && <span className="font-semibold text-tx4"> · {a.holder_label}</span>}
                  </p>
                  <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10.5px] font-medium">
                    <span className="h-1.5 w-1.5 flex-none rounded-full border-[1.5px] border-tx4" />
                    <span className="font-semibold text-tx3">свободный слот</span>
                    <span className="text-tx4">{a.emoji && `${a.emoji} `}«{a.raw_title}»</span>
                  </p>
                </div>
                <Pct percent={a.percent} currency={a.currency_kind} className={pctCls} />
                {CHEVRON}
              </button>
            );
          }
          const e = row.e;
          const st = stateOf(e);
          const extras = [stackShort(e) || (e.kind === "super" ? "барабан" : e.kind === "special" ? "спец" : ""), capShort(e)]
            .filter(Boolean)
            .join(" · ");
          return (
            <button
              key={row.key}
              type="button"
              onClick={() => openEntry(e)}
              style={stripe}
              className={`flex w-full items-center gap-2.5 rounded-2xl border px-3 py-2.5 text-left ${e.friend_name ? "border-acc/40 bg-srf" : "border-brd2 bg-srf"}`}
            >
              <BankBadge name={e.bank_name} size={26} />
              <div className="min-w-0 flex-1">
                <p className="text-[13.5px] font-bold">
                  {e.bank_name}
                  {e.holder_label && <span className="font-semibold text-tx4"> · {e.holder_label}</span>}
                </p>
                <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10.5px] font-medium">
                  <span className={`h-1.5 w-1.5 flex-none rounded-full ${st.dot}`} />
                  <span className={`font-bold ${st.tone}`}>{st.word}</span>
                  <span className="text-tx4">
                    {e.emoji && `${e.emoji} `}«{e.raw_title}»{extras && ` · ${extras}`}
                  </span>
                  {e.friend_name && <Chip tone="friend">друг · {e.friend_name}</Chip>}
                  {e.currency_kind === "points" && <span className="text-tx4">{currencyWord(e.currency_kind, e.points_label)}</span>}
                </p>
              </div>
              <Pct percent={e.percent} currency={e.currency_kind} className={pctCls} />
              {CHEVRON}
            </button>
          );
        })}
      </div>

      <BlockedRows rows={blocked} friendRows={friendAvailable} />

      {/* Everything without an exact answer — incl. every bank whose MCC
          memberships are not ingested yet — answers with its base row. */}
      {base.length > 0 && (
        <div className="rounded-xl border border-brd bg-srf/60 px-3 py-2.5" data-sid="CB-11.f">
          <button type="button" onClick={() => setShowBase(!showBase)} className="flex w-full items-center gap-2 text-left">
            <span className="min-w-0 flex-1 text-xs font-semibold text-tx4">
              Кешбек на всё · {base.length} —{" "}
              {base.map((e) => (e.holder_label ? `${e.bank_name} · ${e.holder_label}` : e.bank_name)).join(", ")}
            </span>
            <span className="text-[9px] text-tx4">{showBase ? "▲" : "▼"}</span>
          </button>
          {showBase && (
            <div className="mt-2 space-y-2 border-t border-brd/60 pt-2">
              {base.map((e, i) => (
                <div key={i} className="flex items-center gap-2">
                  <BankBadge name={e.bank_name} size={18} />
                  <span className="min-w-0 flex-1 truncate text-xs font-semibold text-tx2">
                    {e.bank_name}
                    {e.holder_label && <span className="font-medium text-tx4"> · {e.holder_label}</span>}
                    <span className="font-medium text-tx4"> · «{e.raw_title}»</span>
                  </span>
                  <Pct percent={e.percent} currency={e.currency_kind} className="text-[13px]" />
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </>
  );
}

// The точка screen (12a): «О точке» first, then the leaderboard.
function PointScreen({ mcc, merchant, posID }: { mcc: string; merchant: string | null; posID: string | null }) {
  const board = useQuery({
    queryKey: ["mcc-board", mcc],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/mcc-board", { params: { query: { code: Number(mcc) } } })),
  });
  const point = useQuery({
    queryKey: ["mcc-point", posID],
    enabled: posID != null,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/points-of-sale/{id}", { params: { path: { id: posID! } } })),
  });
  // The point's identity comes from the API, never from the address bar.
  // `?merchant=` used to be rendered as the title on its own, so any hand-made
  // link — including one to a row still in moderation or already rejected —
  // opened as a normal точка продаж, and even landed in «Недавнее» (report
  // 2026-08-27). The API scopes those rows away; the screen has to respect
  // that answer instead of drawing the name it was handed.
  const named = point.data?.name;
  const unresolved = merchant != null && merchant !== "" && named == null;
  const loadingPoint = posID != null && point.isPending;
  const title = named ?? `MCC ${mcc}`;
  const partnerMatch = useQuery({
    queryKey: ["partner-match", named],
    enabled: named != null && named !== "",
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/cashback/partner-offers/match", { params: { query: { query: named! } } })),
  });

  useEffect(() => {
    if (!named) return;
    pushRecent({
      label: named,
      sub: `MCC ${mcc}`,
      to: `/pos?mcc=${mcc}&merchant=${encodeURIComponent(named)}${posID ? `&pos=${posID}` : ""}`,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [named, mcc, posID]);

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/search" />
        <h1 className="min-w-0 flex-1 truncate text-xl font-extrabold tracking-tight">
          {loadingPoint ? "…" : title}
        </h1>
        {named != null && (
          <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[10.5px] font-semibold text-tx3">точка продаж</span>
        )}
      </div>

      {/* A link naming a point the base will not serve: on moderation, rejected,
          deleted, or simply invented. The MCC below is still a real answer, so
          the screen degrades to it rather than 404-ing the whole page. */}
      {unresolved && !loadingPoint && (
        <Card className="space-y-1 p-4" data-sid="CB-11.i">
          <p className="text-sm font-semibold text-tx2">Такой точки в базе нет</p>
          <p className="text-[11.5px] font-medium text-tx4">
            Ссылка могла вести на точку, которая ещё на модерации или отклонена. Ниже — чем платить по MCC {mcc}.
          </p>
        </Card>
      )}

      <AboutPoint mcc={mcc} point={point.data} />

      {board.isPending && <Spinner />}
      {board.isError && <ErrMsg error={board.error} />}
      {board.data && <Leaderboard board={board.data} matches={partnerMatch.data?.matches ?? []} sid="CB-11.c" />}
    </>
  );
}

// The category screen (?cat=): the canonical lookup, kept on the 2b verdict
// layout — a category is not MCC-driven and stays canonical by design.
function CategoryScreen({ slug }: { slug: string }) {
  const categories = useCategories();
  const cards = useCards();
  const [withFriends, setWithFriends] = useState(initWithFriends);
  const [showBase, setShowBase] = useState(false);
  const { openClient, openEntry } = useOpenEntry();
  const canon = (categories.data ?? []).find((c) => c.slug === slug);
  const title = canon?.title_ru ?? "Категория";

  const lookup = useQuery({
    queryKey: ["lookup", slug],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/lookup", { params: { query: { category: slug } } })),
  });

  useEffect(() => {
    if (!canon) return;
    pushRecent({
      label: canon.title_ru,
      sub: `${canon.emoji || ""}`.trim(),
      to: `/pos?cat=${slug}`,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [canon?.slug]);

  const rankedAll = lookup.data?.ranked ?? [];
  // A friend's unpicked row counts as a friend card here too: without it the
  // toggle vanished for a friend who has only those, leaving their name on
  // screen with no way to hide it — the defect the MCC board had.
  const friendAvailable = withFriends ? (lookup.data?.friend_available ?? []) : [];
  const hasFriendCards = rankedAll.some((e) => e.friend_name) || (lookup.data?.friend_available ?? []).length > 0;
  const ranked = withFriends ? rankedAll : rankedAll.filter((e) => !e.friend_name);
  const best = ranked[0];
  const others = ranked.slice(1);
  const available = lookup.data?.available ?? [];
  const blocked = lookup.data?.blocked ?? [];
  const baseClients = (lookup.data?.fallback ?? []).filter((e) => !e.friend_name);
  const cardChipsOf = (e: LookupEntry) =>
    (cards.data ?? [])
      .filter((c) => c.bank_client_id === e.bank_client_id)
      .map((c) => `··${String(c.last_4_digits).padStart(4, "0")}`)
      .join(" ");

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/search" />
        <h1 className="min-w-0 flex-1 truncate text-xl font-extrabold tracking-tight">{title}</h1>
        <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[10.5px] font-semibold text-tx3">категория</span>
      </div>

      {lookup.isPending && <Spinner />}
      {lookup.isError && <ErrMsg error={lookup.error} />}

      {/* The verdict is a row, not a plaque (6a). Tap opens the bank menu. */}
      {best && (
        <button
          type="button"
          data-sid="CB-11.a"
          onClick={() => openEntry(best)}
          className="flex w-full items-center gap-3 border-b border-brd px-0.5 pb-3 text-left"
        >
          <BankBadge name={best.bank_name} size={38} />
          <div className="min-w-0 flex-1">
            <p className="text-[17px] leading-tight font-extrabold tracking-tight">
              {best.bank_name}
              {best.holder_label && <span className="font-bold text-tx3"> · {best.holder_label}</span>}
              {best.friend_name && <span className="ml-1.5 align-[2px]"><Chip tone="friend">друг · {best.friend_name}</Chip></span>}
            </p>
            <p className="mt-0.5 truncate text-[11.5px] font-medium text-tx3">
              {(best.friend_name
                ? [`попроси оплатить — @${best.friend_username}`]
                : [
                    cardChipsOf(best) || "любая карта",
                    stackShort(best) || (best.kind === "super" ? "барабан" : best.kind === "special" ? `спец · «${best.raw_title}»` : ""),
                    capShort(best),
                    best.currency_kind === "points" ? currencyWord(best.currency_kind, best.points_label) : "",
                  ]
              )
                .filter(Boolean)
                .join(" · ")}
            </p>
          </div>
          <span className="flex-none text-right">
            <Pct percent={best.percent} currency={best.currency_kind} className="text-[40px] leading-[.8] tracking-tighter" />
          </span>
          {CHEVRON}
        </button>
      )}

      {lookup.data?.message && !best && (
        <Card className="space-y-1.5 p-4 text-center">
          <p className="text-sm font-semibold text-tx2">{lookup.data.message}</p>
          <p className="text-[10.5px] font-medium text-tx4">Карта попадает сюда, когда в её меню есть эта категория и она выбрана.</p>
        </Card>
      )}

      {!!lookup.data && (
        <>
          {hasFriendCards && (
            <button
              type="button"
              data-sid="CB-11.d"
              onClick={() => {
                localStorage.setItem(FRIENDS_KEY, withFriends ? "off" : "on");
                setWithFriends(!withFriends);
              }}
              className={`flex w-full items-center justify-between rounded-xl border px-3 py-2 text-[12px] font-semibold transition ${
                withFriends ? "border-acc/40 bg-acc/10 text-accl" : "border-brd2 bg-srf2 text-tx3"
              }`}
            >
              Карты друзей в подборе
              <span className={`h-5 w-9 flex-none rounded-full p-0.5 transition ${withFriends ? "bg-acc" : "bg-inset"}`}>
                <span className={`block h-4 w-4 rounded-full bg-white transition ${withFriends ? "translate-x-4" : ""}`} />
              </span>
            </button>
          )}

          {/* Guarded like its twin above: with nothing to list it used to head
              an empty div. */}
          {others.length > 0 && (
            <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Остальные банки</p>
          )}
          <div className="space-y-1.5" data-sid="CB-11.c">
            {/* Prefixed: the available list below keys by offer_id, and a
                fresh install numbers offers from 1 — bare indices collided. */}
            {others.map((e, i) => {
              const st = stateOf(e);
              return (
                <button
                  key={`o${i}`}
                  type="button"
                  onClick={() => openEntry(e)}
                  className={`flex w-full items-center gap-2.5 rounded-2xl border px-3 py-2.5 text-left ${e.friend_name ? "border-acc/40 bg-srf" : "border-brd2 bg-srf"}`}
                >
                  <BankBadge name={e.bank_name} size={26} />
                  <div className="min-w-0 flex-1">
                    <p className="text-[13.5px] font-bold">
                      {e.bank_name}
                      {e.holder_label && <span className="font-semibold text-tx4"> · {e.holder_label}</span>}
                    </p>
                    <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10.5px] font-medium">
                      <span className={`h-1.5 w-1.5 flex-none rounded-full ${st.dot}`} />
                      <span className={`font-bold ${st.tone}`}>{st.word}</span>
                      {e.raw_title && <span className="text-tx4">«{e.raw_title}»</span>}
                      {e.friend_name && <Chip tone="friend">друг · {e.friend_name}</Chip>}
                    </p>
                  </div>
                  <Pct percent={e.percent} currency={e.currency_kind} className="text-[15px]" />
                  {CHEVRON}
                </button>
              );
            })}
            {available.map((e) => (
              <button
                key={e.offer_id}
                type="button"
                onClick={() => openClient(e.bank_client_id)}
                className="flex w-full items-center gap-2.5 rounded-2xl border border-dashed border-dash bg-srf/50 px-3 py-2.5 text-left"
              >
                <BankBadge name={e.bank_name} size={26} />
                <div className="min-w-0 flex-1">
                  <p className="text-[13.5px] font-bold">
                    {e.bank_name}
                    {e.holder_label && <span className="font-semibold text-tx4"> · {e.holder_label}</span>}
                  </p>
                  <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-[10.5px] font-medium">
                    <span className="h-1.5 w-1.5 flex-none rounded-full border-[1.5px] border-tx4" />
                    <span className="font-semibold text-tx3">свободный слот</span>
                    <span className="text-tx4">«{e.raw_title}»</span>
                  </p>
                </div>
                <Pct percent={e.percent} currency={e.currency_kind} className="text-[15px]" />
                {CHEVRON}
              </button>
            ))}
          </div>

          <BlockedRows rows={blocked} friendRows={friendAvailable} />

          {baseClients.length > 0 && (
            <div className="rounded-xl border border-brd bg-srf/60 px-3 py-2.5" data-sid="CB-11.f">
              <button type="button" onClick={() => setShowBase(!showBase)} className="flex w-full items-center gap-2 text-left">
                <span className="min-w-0 flex-1 text-xs font-semibold text-tx4">
                  Кешбек на всё · {baseClients.length} —{" "}
                  {baseClients.map((e) => (e.holder_label ? `${e.bank_name} · ${e.holder_label}` : e.bank_name)).join(", ")}
                </span>
                <span className="text-[9px] text-tx4">{showBase ? "▲" : "▼"}</span>
              </button>
              {showBase && (
                <div className="mt-2 space-y-2 border-t border-brd/60 pt-2">
                  {baseClients.map((e, i) => (
                    <div key={i} className="flex items-center gap-2">
                      <BankBadge name={e.bank_name} size={18} />
                      <span className="min-w-0 flex-1 truncate text-xs font-semibold text-tx2">
                        {e.bank_name}
                        {e.holder_label && <span className="font-medium text-tx4"> · {e.holder_label}</span>}
                        <span className="font-medium text-tx4"> · «{e.raw_title}»</span>
                      </span>
                      <Pct percent={e.percent} currency={e.currency_kind} className="text-[13px]" />
                    </div>
                  ))}
                </div>
              )}
            </div>
          )}
        </>
      )}
    </>
  );
}

export default function Pos() {
  const [params] = useSearchParams();
  const mcc = params.get("mcc");
  const merchant = params.get("merchant");
  const catParam = params.get("cat");
  const posID = params.get("pos");

  // A bare code is a code question, not a точка — the code screen answers it.
  if (mcc != null && merchant == null && posID == null) {
    return <Navigate to={`/mcc/${mcc}`} replace />;
  }
  if (mcc != null) return <PointScreen mcc={mcc} merchant={merchant} posID={posID} />;
  if (catParam != null) return <CategoryScreen slug={catParam} />;
  return <Navigate to="/search" replace />;
}
