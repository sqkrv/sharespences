import { useEffect, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { api, unwrap, type Schemas } from "../api/client";
import { useCategories } from "../hooks";
import { BankBadge, Card, ErrMsg, Pct, PosTypeIcon, SegTabs, Spinner } from "../components/ui";
import { FALLBACK_EMOJI, POS_ORIGIN_MARK, normalizeTitle } from "../lib";
import { recentEntries, type RecentEntry } from "../recent";

// CB-04 v2 «Поиск» (redesign 2c/2i): one field, tabs filter the entered
// query — магазины (точки продаж), категории, MCC — and every result leads
// to the «Точка продаж» screen (CB-11). Works from the very first launch:
// MCC and the merchant base are bank-independent, so rows simply render
// without percentages until banks exist.



type Tab = "all" | "shops" | "cats" | "mcc";

// Point-of-sale type filter for the «Магазины» tab. "" is «все» — the same
// merchant is often a different MCC at the till than in its app, so which
// counter you are standing at is a question the base can answer.
type PosType = "" | "offline" | "online" | "app";
const POS_TYPES: [PosType, string][] = [
  ["", "все"],
  ["offline", "офлайн"],
  ["online", "онлайн"],
  ["app", "приложение"],
];

function GroupLabel({ children }: { children: React.ReactNode }) {
  return <p className="mx-0.5 pt-1 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">{children}</p>;
}

// One page of the merchant base. Small on purpose: the list is scrolled on a
// phone, and a wrong query should cost one round trip, not fifty rows.
const MERCHANT_PAGE = 20;

// LoadMore is an explicit button, not an infinite scroll (owner 2026-08-28):
// the next batch loads when asked, so the list ends where the user stopped
// asking — the license footer and the other groups stay reachable instead
// of running away from the scroll.
function LoadMore({ onLoad, busy, remaining }: { onLoad: () => void; busy: boolean; remaining: number }) {
  return (
    <button
      type="button"
      onClick={onLoad}
      disabled={busy}
      className="w-full rounded-2xl border border-dashed border-dash py-2.5 text-center text-[12px] font-semibold text-tx3 disabled:opacity-50"
    >
      {busy ? "Загрузка…" : `Показать ещё${remaining > 0 ? ` · ${remaining}` : ""}`}
    </button>
  );
}

export default function Search() {
  const navigate = useNavigate();
  const [q, setQ] = useState("");
  const [debouncedQ, setDebouncedQ] = useState("");
  const [tab, setTab] = useState<Tab>("all");
  const categories = useCategories();
  // Current-month feed data (cached by the shell prefetch) — the source of
  // «какой процент у этой категории» without extra requests.
  const overview = useQuery({
    queryKey: ["overview"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview")),
    staleTime: 60_000,
  });

  useEffect(() => {
    const t = setTimeout(() => setDebouncedQ(q.trim()), 400);
    return () => clearTimeout(t);
  }, [q]);

  const isCode = /^\d{3,4}$/.test(debouncedQ);
  const active = debouncedQ.length >= 2;
  // Point-of-sale type filter, shown only on the «Магазины» tab. Component
  // state on purpose, not localStorage: privacy policy §3.2 lists the storage
  // keys the app uses, so a persisted filter would be a policy edit — and a
  // filter the user cannot see (they are on another tab) is worse than one
  // that resets.
  const [posType, setPosType] = useState<PosType>("");

  const codes = useQuery({
    queryKey: ["mcc-search", debouncedQ],
    enabled: active && (tab === "all" || tab === "mcc"),
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/codes", { params: { query: { query: debouncedQ } } })) ?? [],
  });
  // Paged, because the base is 60k rows and a plain word matches hundreds:
  // «яндекс» alone has 500+ points of sale, so a fixed 20-row answer hid rows
  // the user knew existed (report 2026-08-24). The list loads the next page
  // as it is scrolled; `total` is what makes «есть ещё» knowable at all.
  const merchants = useInfiniteQuery({
    queryKey: ["mcc-merchants", debouncedQ, posType],
    enabled: active && !isCode && (tab === "all" || tab === "shops"),
    initialPageParam: 0,
    queryFn: async ({ pageParam }) =>
      unwrap(
        await api.GET("/api/v1/mcc/merchants", {
          params: { query: { query: debouncedQ, ...(posType ? { type: posType } : {}), limit: MERCHANT_PAGE, offset: pageParam } },
        }),
      ),
    getNextPageParam: (last, pages) => {
      const loaded = pages.reduce((n, p) => n + (p?.items?.length ?? 0), 0);
      return loaded < (last?.total ?? 0) ? loaded : undefined;
    },
  });
  const merchantRows = (merchants.data?.pages ?? []).flatMap((p) => p?.items ?? []);
  const merchantTotal = merchants.data?.pages[0]?.total ?? 0;

  // Categories filter client-side over the canonical list; the winner line
  // comes from the cached feed (best → friend → available, same fallback
  // order the feed itself renders).
  const nq = normalizeTitle(debouncedQ);
  const catGroups = new Map<string, Schemas["OverviewCategoryDTO"]>(
    (overview.data?.categories ?? []).map((g) => [g.slug, g]),
  );
  const matchedCats = active
    ? (categories.data ?? []).filter((c) => normalizeTitle(c.title_ru).includes(nq))
    : [];

  const recent = recentEntries();
  const showShops = tab === "all" || tab === "shops";
  const showCats = tab === "all" || tab === "cats";
  const showMcc = tab === "all" || tab === "mcc";
  // Which queries the active tab actually shows. This matters because a
  // DISABLED query in TanStack v5 reports status 'pending' forever — nothing
  // will ever fetch it — so keying the spinner on isPending left «Загрузка…»
  // up permanently wherever a query was switched off: on «Категории» (both
  // off), on «MCC» (merchants off), and on «Всё» whenever the query looked
  // like a code (report 2026-08-28). It only looked intermittent because a
  // previous search on «Всё» leaves cached data under the same key, which
  // flips the query to 'success' and hides the bug.
  const codesActive = active && showMcc;
  const merchantsActive = active && showShops && !isCode;
  // isFetching is «a request is in flight», which is what a spinner claims.
  const busy = (codesActive && codes.isFetching) || (merchantsActive && merchants.isFetching);
  // Paused = the request cannot even start (offline, or the server is not
  // answering — queries run networkMode 'offlineFirst'). Spinning forever is
  // the one thing that must not happen there: the search has not started and
  // never will until the connection is back.
  const stalled =
    (codesActive && codes.fetchStatus === "paused") || (merchantsActive && merchants.fetchStatus === "paused");
  const filteredOut = tab === "shops" && posType !== "" && merchantRows.length === 0 && !busy && !stalled;
  // The empty state answers the tab the user is standing on. Adding a точка
  // продаж is an answer to «магазин не нашёлся» and to nothing else — on
  // «Категории» and «MCC» it is a non sequitur (report 2026-08-28), and on
  // the MCC tab the old «введи MCC-код с чека» was circular advice. Both of
  // those tabs are closed reference lists — a canonical set and a published
  // dictionary — so the only honest offer there is to widen the search.
  const emptyOnList = tab === "cats" || tab === "mcc";
  const emptyText =
    tab === "cats"
      ? "Такой категории нет — список категорий закрытый. Поищи по названию магазина или по MCC."
      : tab === "mcc"
        ? "Такого кода нет в справочнике MCC — проверь цифры в чеке или поищи по названию."
        : "Ничего не найдено — попробуй иначе или введи MCC-код с чека.";
  const nothingFound =
    active &&
    !busy &&
    !stalled &&
    (!showShops || merchantRows.length === 0) &&
    (!showCats || matchedCats.length === 0) &&
    (!showMcc || (codes.data ?? []).length === 0);

  return (
    <>
      <div className="flex items-center gap-2.5">
        <div className="flex h-11 min-w-0 flex-1 items-center gap-2.5 rounded-2xl border border-acc bg-srf2 px-3.5" data-sid="CB-04.c">
          <svg width="15" height="15" viewBox="0 0 24 24" fill="none" stroke="var(--t-tx4)" strokeWidth="2.4" strokeLinecap="round" className="flex-none">
            <circle cx="10.5" cy="10.5" r="7" />
            <path d="M16 16l5 5" />
          </svg>
          <input
            autoFocus
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Магазин, категория или MCC"
            inputMode="search"
            className="min-w-0 flex-1 bg-transparent text-sm font-medium outline-none placeholder:text-tx4"
          />
        </div>
        <button type="button" className="flex-none text-[13px] font-semibold text-accl" onClick={() => navigate("/")}>
          Отмена
        </button>
      </div>

      {/* The tabs filter results — with nothing typed there is nothing to
          filter, so they appear with the first character. */}
      {q.trim() !== "" && (
        <SegTabs
          sid="CB-04.a"
          value={tab}
          onChange={setTab}
          options={[
            { value: "all", label: "Всё" },
            { value: "shops", label: "Магазины" },
            { value: "cats", label: "Категории" },
            { value: "mcc", label: "MCC" },
          ]}
        />
      )}

      {!active && (
        <>
          {recent.length > 0 && (
            <>
              <GroupLabel>Недавнее</GroupLabel>
              <div className="space-y-1.5">
                {recent.map((r: RecentEntry) => (
                  <button
                    key={r.to}
                    type="button"
                    onClick={() => navigate(r.to)}
                    className="flex w-full items-center gap-2.5 rounded-2xl border border-brd bg-srf px-3 py-2.5 text-left"
                  >
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-bold">{r.label}</span>
                      {r.sub && <span className="block truncate text-[11px] font-semibold text-tx4">{r.sub}</span>}
                    </span>
                    <span className="text-tx4">›</span>
                  </button>
                ))}
              </div>
            </>
          )}
          <GroupLabel>Рядом</GroupLabel>
          <div className="flex flex-col items-center gap-2.5 rounded-2xl border border-dashed border-dash bg-srf/50 px-4 py-4 text-center" data-sid="CB-04.b">
            <span className="flex h-10 w-10 items-center justify-center rounded-full bg-inset">
              <svg width="19" height="19" viewBox="0 0 24 24" fill="none" stroke="var(--t-tx3)" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
                <path d="M12 21s-6.8-5.4-6.8-11a6.8 6.8 0 0 1 13.6 0c0 5.6-6.8 11-6.8 11Z" />
                <circle cx="12" cy="10" r="2.4" />
              </svg>
            </span>
            <p className="text-[12.5px] leading-snug font-medium text-tx3">
              База точек уже в приложении, но пока без координат — «Рядом» появится, когда они будут.
            </p>
            <p className="text-[10.5px] font-medium text-tx4">Пока быстрее всего — начать вводить название.</p>
          </div>
        </>
      )}

      {active && (
        <div className="space-y-1.5">
          {/* Filters belong to the dedicated tab: on «Всё» the shops are one
              group among several, and a filter there would silently narrow a
              list the user is not looking at. */}
          {tab === "shops" && !isCode && active && (
            <div data-sid="CB-04.i" className="flex gap-1.5 overflow-x-auto pb-0.5">
              {POS_TYPES.map(([value, label]) => (
                <button
                  key={value || "all"}
                  type="button"
                  onClick={() => setPosType(value)}
                  className={`flex-none rounded-full border px-2.5 py-1 text-[11px] font-semibold transition ${
                    posType === value ? "border-acc bg-acc/15 text-accl" : "border-brd bg-srf text-tx3"
                  }`}
                >
                  {label}
                </button>
              ))}
            </div>
          )}

          {showShops && !isCode && merchantRows.length > 0 && (
            <div data-sid="CB-04.g" className="space-y-1.5">
              <GroupLabel>
                Магазины
                {merchantTotal > merchantRows.length && (
                  <span className="ml-1.5 font-medium text-tx4">
                    {merchantRows.length} из {merchantTotal}
                  </span>
                )}
              </GroupLabel>
              {merchantRows.map((m) => (
                <button
                  key={m.id}
                  type="button"
                  onClick={() => navigate(`/pos?mcc=${m.mcc}&merchant=${encodeURIComponent(m.name)}&pos=${m.id}`)}
                  className="flex w-full items-start gap-2.5 rounded-2xl border border-brd bg-srf px-3 py-2.5 text-left hover:bg-srf2"
                >
                  <PosTypeIcon type={m.type} />
                  {/* Three fixed roles, one per row: the human name, the
                      string a выписка shows, the address. */}
                  <div className="min-w-0 flex-1 space-y-[3px]">
                    <p className="truncate text-[12.5px] leading-tight font-semibold text-tx2">
                      {m.name}
                      {/* Only the author sees their pending точка (5e). */}
                      {m.status === "pending" && (
                        <span className="ml-1.5 rounded bg-gold/10 px-1 py-[1px] text-[9px] font-bold text-gold">на модерации</span>
                      )}
                    </p>
                    {m.merchant_title && (
                      <p className="truncate font-mono text-[10px] leading-tight font-semibold tracking-wide text-tx3">{m.merchant_title}</p>
                    )}
                    {m.address && <p className="truncate text-[10px] leading-tight font-medium text-tx4">{m.address}</p>}
                  </div>
                  <div className="flex flex-none flex-col items-end gap-1">
                    <span className="font-mono text-[12px] leading-tight font-bold text-accl">{m.mcc}</span>
                    {POS_ORIGIN_MARK[m.origin] && (
                      <span className="rounded-md bg-inset px-1 py-[1px] text-[9px] font-semibold text-tx4">
                        {POS_ORIGIN_MARK[m.origin]}
                      </span>
                    )}
                    {m.confirmations > 0 && (
                      <span
                        title={`Подтверждений: ${m.confirmations}`}
                        className="flex items-center gap-[3px] rounded-md bg-mint/15 px-1 py-[1px] text-[9.5px] font-bold text-mint"
                      >
                        <svg width="8" height="8" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="4.5" strokeLinecap="round" strokeLinejoin="round">
                          <path d="M5 12.5 10 17.5 19 6.5" />
                        </svg>
                        {m.confirmations}
                      </span>
                    )}
                  </div>
                </button>
              ))}
              {/* Asked for, never crossed into: the next page loads on a tap,
                  so the list ends where the matches end rather than growing
                  under the finger. */}
              {merchants.hasNextPage && (
                <LoadMore onLoad={merchants.fetchNextPage} busy={merchants.isFetchingNextPage} remaining={merchantTotal - merchantRows.length} />
              )}
            </div>
          )}

          {showCats && matchedCats.length > 0 && (
            <div className="space-y-1.5">
              <GroupLabel>Категории</GroupLabel>
              {matchedCats.map((c) => {
                const g = catGroups.get(c.slug);
                const winner = g?.friend_best ?? g?.best ?? g?.available;
                return (
                  <button
                    key={c.id}
                    type="button"
                    onClick={() => navigate(`/pos?cat=${c.slug}`)}
                    className="flex w-full items-center gap-2.5 rounded-2xl border border-brd bg-srf px-3 py-2.5 text-left hover:bg-srf2"
                  >
                    <span className="w-[21px] flex-none text-center text-base leading-none">{c.emoji || FALLBACK_EMOJI}</span>
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-bold">{c.title_ru}</span>
                      {winner && (
                        <span className="mt-0.5 flex items-center gap-1.5 text-[11.5px] font-semibold text-tx4">
                          <BankBadge name={winner.bank_name} size={16} />
                          {winner.bank_name}
                          {"stacked_super" in winner && winner.stacked_super != null && (
                            <span className="rounded-[5px] bg-gold/15 px-1.5 py-px text-[9.5px] font-bold text-gold">
                              {winner.stacked_regular} + {winner.stacked_super} барабан
                            </span>
                          )}
                        </span>
                      )}
                    </span>
                    {winner && <Pct percent={winner.percent} currency={winner.currency_kind} className="text-base" />}
                  </button>
                );
              })}
            </div>
          )}

          {showMcc && (codes.data ?? []).length > 0 && (
            <div className="space-y-1.5">
              <GroupLabel>MCC</GroupLabel>
              {(codes.data ?? []).map((s) => (
                <button
                  key={s.code}
                  type="button"
                  onClick={() => navigate(`/mcc/${s.code}`)}
                  className="flex w-full items-center gap-2.5 rounded-2xl border border-brd bg-srf px-3 py-2.5 text-left hover:bg-srf2"
                >
                  <span className="flex-none font-mono text-[13px] font-extrabold text-accl">{s.code}</span>
                  <span className="min-w-0 flex-1 truncate text-[13px] font-bold">{s.name}</span>
                  <span className="text-tx4">›</span>
                </button>
              ))}
            </div>
          )}

          {busy && <Spinner />}
          {stalled && (
            <Card className="p-4 text-center" data-sid="CB-04.k">
              <p className="text-sm font-medium text-tx3">Нет связи с сервером — поиск недоступен.</p>
            </Card>
          )}
          {codes.isError && <ErrMsg error={codes.error} />}
          {merchants.isError && <ErrMsg error={merchants.error} />}
          {/* The zero-results tail leads to manual creation (5e) — the
              каталог grows where it failed to answer. */}
          {/* An empty list because of the filter is not an empty base: offering
              «добавить точку» there would push the user to create a duplicate
              of a row that exists under another type. */}
          {filteredOut && (
            <Card className="space-y-2.5 p-4 text-center" data-sid="CB-04.j">
              <p className="text-sm font-medium text-tx3">
                Ничего не найдено с этим фильтром — точка может быть другого типа.
              </p>
              <button
                type="button"
                onClick={() => setPosType("")}
                className="w-full rounded-2xl border border-dashed border-dash py-2.5 text-sm font-semibold text-tx3"
              >
                Показать все типы
              </button>
            </Card>
          )}

          {nothingFound && !filteredOut && (
            <Card className="space-y-2.5 p-4 text-center" data-sid="CB-04.e">
              <p className="text-sm font-medium text-tx3">{emptyText}</p>
              {emptyOnList ? (
                <button
                  type="button"
                  onClick={() => setTab("all")}
                  className="w-full rounded-2xl border border-dashed border-dash py-2.5 text-sm font-semibold text-tx3"
                >
                  Искать везде
                </button>
              ) : (
                <button
                  type="button"
                  onClick={() => navigate(`/pos/new?query=${encodeURIComponent(q.trim())}`)}
                  className="w-full rounded-2xl border border-dashed border-dash py-2.5 text-sm font-semibold text-tx3"
                >
                  + Добавить точку вручную
                </button>
              )}
            </Card>
          )}
        </div>
      )}

      {/* mcc-codes.ru permits copying with attribution at the point of use
          (2026-07-29) — this line is the license, keep it. Since the base is
          mixed (00027), the credit is scoped: rows people wrote carry their
          own mark, so what's left unmarked is exactly the scrape. */}
      <p className="mx-1 text-[10.5px] leading-snug font-medium text-tx4">
        Справочник MCC и база точек — данные <span className="font-semibold text-tx3">mcc-codes.ru</span>, кроме точек с
        пометкой «от пользователей» или «Sharespences»; код можно узнать из чека или выписки.
      </p>
    </>
  );
}
