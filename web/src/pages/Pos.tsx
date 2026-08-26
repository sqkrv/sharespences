import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { ApiError, api, unwrap, type LookupEntry } from "../api/client";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useCards, useCategories } from "../hooks";
import { BackButton, BankBadge, Card, Chip, ErrMsg, Pct, Spinner } from "../components/ui";
import {
  FALLBACK_EMOJI,
  FRIENDS_KEY,
  POS_TYPE_RU,
  capNote,
  currencyWord,
  fmtDate,
  fmtPercent,
  initWithFriends,
  monthKey,
  plural,
  todayISO,
} from "../lib";
import { pushRecent } from "../recent";

// CB-11 «Точка продаж» (redesign 2b v3): the former CB-04 result state as
// its own screen, zero clicks after a search tap — the verdict is the first
// row of the list (6a: no plaque, only weight), then the MCC chip, then
// «Остальные банки». A row tap opens that bank's menu — categories are
// picked there, not here.
//
// Points never convert to rubles (invariant 5): the ranking groups рубли
// first and the balance rows keep their own currency, stated in place.

// «7+7 барабан» — the stacked pair in a row's sub-line.
function stackShort(e: LookupEntry): string {
  if (e.stacked_super == null) return "";
  return `${e.stacked_regular ?? "—"}+${e.stacked_super} барабан`;
}

// The cap in the row vocabulary: «до 7000₽», not «лимит 7000₽».
function capShort(e: LookupEntry): string {
  return capNote(e).replace(/^лимит /, "до ");
}

// The state vocabulary of an «Остальные банки» row (2b v3).
function stateOf(e: LookupEntry): { dot: string; word: string; tone: string } {
  if (e.kind === "partner") return { dot: "bg-gold", word: "партнёрка", tone: "text-gold" };
  if (e.kind === "super" || e.kind === "special") return { dot: "bg-gold", word: "выдано банком", tone: "text-gold" };
  return { dot: "bg-mint", word: "выбрана", tone: "text-mint" };
}

const CHEVRON = (
  <svg width="13" height="13" viewBox="0 0 24 24" fill="none" stroke="var(--t-tx4)" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round" className="flex-none">
    <path d="M9 5l7 7-7 7" />
  </svg>
);

export default function Pos() {
  const [params] = useSearchParams();
  const mcc = params.get("mcc");
  const merchant = params.get("merchant");
  const catParam = params.get("cat");
  const posID = params.get("pos");
  const [withFriends, setWithFriends] = useState(initWithFriends);
  const [showBase, setShowBase] = useState(false);
  const categories = useCategories();
  const cards = useCards();
  const navigate = useNavigate();

  const resolve = useQuery({
    queryKey: ["mcc-resolve", mcc],
    enabled: mcc != null,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/resolve", { params: { query: { code: mcc! } } })),
  });
  const unknownCode = resolve.isError && resolve.error instanceof ApiError && resolve.error.status === 404;

  // «О точке» (8b): everything the base knows about the point. Quietly
  // absent when the screen was reached without a concrete точка.
  const point = useQuery({
    queryKey: ["mcc-point", posID],
    enabled: posID != null,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/points-of-sale/{id}", { params: { path: { id: posID! } } })),
  });

  // Banks file the same MCC under different categories — sometimes several
  // in one bank. When the code maps to more than one canonical, the MCC chip
  // switches between them and the whole breakdown follows (decision
  // 2026-08-25). The concrete end state stays recorded: judge each bank by
  // ITS OWN category for the code, once per-bank MCC memberships cover the
  // wallet (today only Альфа/ВТБ/Озон are ingested).
  const [canonIdx, setCanonIdx] = useState(0);
  useEffect(() => setCanonIdx(0), [mcc]);
  const canonList = resolve.data?.canonicals ?? [];
  const slug = catParam ?? canonList[Math.min(canonIdx, Math.max(0, canonList.length - 1))]?.slug ?? null;
  const canon = (categories.data ?? []).find((c) => c.slug === slug);
  const title = merchant ?? canon?.title_ru ?? (mcc ? `MCC ${mcc}` : "Точка продаж");
  const canSwitchCanon = catParam == null && canonList.length > 1;

  const lookup = useQuery({
    queryKey: ["lookup", slug],
    enabled: slug != null,
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/lookup", { params: { query: { category: slug! } } })),
  });

  // client id → this month's period, for row navigation. Clients with no
  // answer for the category are simply not shown (feedback 2026-08-25 —
  // supersedes the earlier grey «меню не занесено» rows): the screen's
  // question is «which card pays here», not an inventory.
  const overview = useQuery({
    queryKey: ["overview"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview")),
    staleTime: 60_000,
  });

  // Партнёрки matched by the point's NAME (v2) — honest name-based matching,
  // shown as their own block: in Лавке such an offer often IS the answer.
  const partnerMatch = useQuery({
    queryKey: ["partner-match", merchant],
    enabled: merchant != null && merchant !== "",
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/cashback/partner-offers/match", { params: { query: { query: merchant! } } })),
  });
  const matches = partnerMatch.data?.matches ?? [];

  // «Недавнее» on the search screen — session-only, no localStorage.
  useEffect(() => {
    if (!merchant && !mcc && !canon) return;
    pushRecent({
      label: merchant ?? canon?.title_ru ?? `MCC ${mcc}`,
      sub: [canon && `${canon.emoji || ""} ${canon.title_ru}`.trim(), mcc && `MCC ${mcc}`].filter(Boolean).join(" · "),
      to: `/pos?${params.toString()}`,
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [merchant, mcc, canon?.slug]);

  const toggleFriends = (v: boolean) => {
    localStorage.setItem(FRIENDS_KEY, v ? "on" : "off");
    setWithFriends(v);
  };

  const rankedAll = lookup.data?.ranked ?? [];
  const hasFriendCards = rankedAll.some((e) => e.friend_name);
  const ranked = withFriends ? rankedAll : rankedAll.filter((e) => !e.friend_name);
  const best = ranked[0];
  const others = ranked.slice(1);
  const available = lookup.data?.available ?? [];
  // Base-paying clients fold into one line — their answer is «За все
  // покупки», not this category (2b v3).
  const baseClients = (lookup.data?.fallback ?? []).filter((e) => !e.friend_name);
  const cardChipsOf = (e: LookupEntry) =>
    (cards.data ?? [])
      .filter((c) => c.bank_client_id === e.bank_client_id)
      .map((c) => `··${String(c.last_4_digits).padStart(4, "0")}`)
      .join(" ");

  // A row tap opens that bank's menu — the pick lives there (2b v3). A
  // friend's menu isn't ours to open; their row goes to «Кешбек друзей»,
  // партнёрки to their home on the bank card.
  const openClient = (clientID?: number) => {
    const c = (overview.data?.clients ?? []).find((x) => x.bank_client_id === clientID);
    if (c == null) return;
    if (c.period_id != null) navigate(`/periods/${c.period_id}`);
    else navigate(`/periods/new?client=${c.bank_client_id}&month=${monthKey(todayISO())}`);
  };
  const openEntry = (e: LookupEntry) => {
    if (e.friend_name) navigate("/friends");
    else if (e.kind === "partner") navigate("/banks");
    else openClient(e.bank_client_id);
  };

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/search" />
        <h1 className="min-w-0 flex-1 truncate text-xl font-extrabold tracking-tight">{title}</h1>
        <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[10.5px] font-semibold text-tx3">точка продаж</span>
      </div>

      {mcc != null && resolve.isPending && <Spinner />}
      {unknownCode && (
        <Card className="p-4 text-center">
          <p className="text-sm font-medium text-tx3">Код {mcc} не найден в справочнике MCC.</p>
        </Card>
      )}
      {resolve.isError && !unknownCode && <ErrMsg error={resolve.error} />}

      {slug != null && lookup.isPending && <Spinner />}
      {lookup.isError && <ErrMsg error={lookup.error} />}

      {/* The verdict is a row, not a plaque (6a): logo, cards + mechanic +
          cap in one muted line, the rate at 40px. Tap opens the bank menu. */}
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

      {slug != null && lookup.data?.message && !best && (
        <Card className="space-y-1.5 p-4 text-center">
          <p className="text-sm font-semibold text-tx2">{lookup.data.message}</p>
          <p className="text-[10.5px] font-medium text-tx4">Карта попадает сюда, когда в её меню есть эта категория и она выбрана.</p>
        </Card>
      )}

      {matches.length > 0 && (
        <div className="space-y-1.5" data-sid="CB-11.e">
          <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-gold uppercase">Партнёрка в этой точке</p>
          {matches.map((e, i) => (
            <div key={i} className="flex items-center gap-2.5 rounded-2xl border border-gold/30 bg-gold/5 px-3 py-2.5">
              <span className="flex h-[26px] w-[26px] flex-none items-center justify-center rounded-[9px] bg-gold/15 text-xs font-extrabold text-gold">★</span>
              <div className="min-w-0 flex-1">
                <p className="truncate text-[13px] font-semibold text-gold">{e.raw_title}</p>
                <p className="truncate text-[10px] font-medium text-tx4">
                  {[e.bank_name, "совпадение по названию точки", e.needs_activation && "требует активации"].filter(Boolean).join(" · ")}
                </p>
              </div>
              <Pct percent={e.percent} currency={e.currency_kind} className="text-[15px]" />
            </div>
          ))}
        </div>
      )}

      {resolve.data && (
        <Card className="p-0" data-sid="CB-11.b">
          <button
            type="button"
            disabled={!canSwitchCanon}
            onClick={() => setCanonIdx((canonIdx + 1) % Math.max(1, canonList.length))}
            className="flex w-full items-center gap-2.5 p-3.5 text-left"
            title={canSwitchCanon ? "Код входит в несколько категорий — переключить" : undefined}
          >
            <span className="flex-none font-mono text-[15px] font-extrabold text-accl">{resolve.data.code.code}</span>
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-bold">{resolve.data.code.name}</p>
              <p className="mt-0.5 text-[10.5px] font-medium text-tx4">
                {canon ? `${canon.emoji || FALLBACK_EMOJI} ${canon.title_ru}` : "канонической категории нет"}
              </p>
            </div>
            {canSwitchCanon && (
              <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[10px] font-semibold text-tx3">
                {canonIdx + 1}/{canonList.length} ▼
              </span>
            )}
          </button>
        </Card>
      )}

      {/* «О точке» — the base's own facts about this точка (8b): the
          statement string, address + type, the record's freshness. */}
      {point.data && (
        <Card className="space-y-1.5 p-3.5" data-sid="CB-11.g">
          <p className="text-[10px] font-extrabold tracking-[.14em] text-tx3 uppercase">О точке</p>
          {(
            [
              ["в выписке", point.data.merchant_title && <span className="font-mono tracking-wide">{point.data.merchant_title}</span>],
              [
                "адрес",
                [point.data.address, point.data.type && POS_TYPE_RU[point.data.type]].filter(Boolean).join(" · ") || null,
              ],
              [
                "запись",
                [
                  `${point.data.confirmations} ${plural(Number(point.data.confirmations), "подтверждение", "подтверждения", "подтверждений")}`,
                  point.data.last_confirmed_at && `актуально на ${fmtDate(point.data.last_confirmed_at.slice(0, 10))}`,
                ]
                  .filter(Boolean)
                  .join(" · "),
              ],
            ] as const
          ).map(
            ([label, value]) =>
              value != null &&
              value !== "" && (
                <div key={label} className="flex items-baseline gap-2">
                  <dt className="w-[78px] flex-none text-[10px] font-medium tracking-[.06em] text-tx4 uppercase">{label}</dt>
                  <dd className="min-w-0 flex-1 text-[12px] font-semibold text-tx2">{value}</dd>
                </div>
              ),
          )}
        </Card>
      )}

      {slug != null && !!lookup.data && (
        <>
          {hasFriendCards && (
            <button
              type="button"
              data-sid="CB-11.d"
              onClick={() => toggleFriends(!withFriends)}
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

          <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Остальные банки</p>
          <div className="space-y-1.5" data-sid="CB-11.c">
            {others.map((e, i) => {
              const st = stateOf(e);
              return (
                <button
                  key={i}
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
            {/* Every served row here is pickable — the API drops the dead
                ends. Picking happens in the bank's menu, a tap away. */}
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

          {/* Base-paying banks fold into one line (2b v3): their answer is
              «За все покупки», not this category. */}
          {baseClients.length > 0 && (
            <div className="rounded-xl border border-brd bg-srf/60 px-3 py-2.5" data-sid="CB-11.f">
              <button type="button" onClick={() => setShowBase(!showBase)} className="flex w-full items-center gap-2 text-left">
                <span className="min-w-0 flex-1 text-xs font-semibold text-tx4">
                  Платят базу · {baseClients.length} —{" "}
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


          {(lookup.data.partner ?? []).length > 0 && (
            <div className="border-t border-brd pt-2">
              <p className="mx-0.5 mb-1.5 text-[10px] font-semibold tracking-wide text-tx4 uppercase">Партнёрские (справочно)</p>
              {(lookup.data.partner ?? []).map((p) => (
                <p key={p.id} className="text-[12.5px] font-medium text-tx3">
                  {p.merchant_title} — {fmtPercent(p.percent)} ({p.bank_name}
                  {p.valid_to && ` · до ${p.valid_to}`})
                </p>
              ))}
            </div>
          )}
        </>
      )}

      {/* A code with no canonical ranks nothing — but the catalog still
          knows which bank category it falls into; show that instead of a
          dead end. */}
      {slug == null && resolve.data && (
        <>
          {(resolve.data.banks ?? []).length === 0 ? (
            <Card className="p-4 text-center">
              <p className="text-sm font-medium text-tx3">
                Пока ни у одного банка нет этого кода в известных составах категорий — база пополняется из документов банков.
              </p>
            </Card>
          ) : (
            <>
              <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Как это считают банки</p>
              <div className="space-y-1.5">
                {(resolve.data.banks ?? []).map((b) => (
                  <div key={b.bank_category_id} className="flex items-center gap-2.5 rounded-2xl border border-brd bg-srf px-3 py-2.5">
                    <BankBadge name={b.bank_name} size={26} color={b.bank_color_hex} />
                    <div className="min-w-0 flex-1">
                      <p className="truncate text-[13px] font-semibold">
                        {b.emoji && <span className="mr-1">{b.emoji}</span>}
                        {b.title}
                        {b.kind === "special" && <span className="ml-1.5 rounded bg-gold/10 px-1 py-[1px] text-[9px] font-bold text-gold">спец</span>}
                      </p>
                      <p className="truncate text-[10px] font-medium text-tx4">
                        {b.bank_name}
                        {b.note ? ` · ${b.note}` : ""}
                      </p>
                    </div>
                  </div>
                ))}
              </div>
            </>
          )}
        </>
      )}
    </>
  );
}
