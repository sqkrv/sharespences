import { useEffect, useRef, useState } from "react";
import { useInfiniteQuery, useQuery } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { api, unwrap, type Schemas } from "../api/client";
import { useCategories } from "../hooks";
import { BankBadge, Card, ErrMsg, Pct, SegTabs, Spinner } from "../components/ui";
import { FALLBACK_EMOJI, POS_TYPE_RU, normalizeTitle } from "../lib";
import { recentEntries, type RecentEntry } from "../recent";

// CB-04 v2 «Поиск» (redesign 2c/2i): one field, tabs filter the entered
// query — магазины (точки продаж), категории, MCC — and every result leads
// to the «Точка продаж» screen (CB-11). Works from the very first launch:
// MCC and the merchant base are bank-independent, so rows simply render
// without percentages until banks exist.

const POS_TYPE_PATH: Record<string, React.ReactNode> = {
  offline: (
    <>
      <path d="M12 21s-6.8-5.4-6.8-11a6.8 6.8 0 0 1 13.6 0c0 5.6-6.8 11-6.8 11Z" />
      <circle cx="12" cy="10.5" r="2.4" />
    </>
  ),
  online: (
    <>
      <circle cx="12" cy="12" r="8.5" />
      <path d="M3.5 12h17" />
      <path d="M12 3.5c2.4 2.6 3.6 5.4 3.6 8.5S14.4 18.4 12 20.5c-2.4-2.6-3.6-5.4-3.6-8.5S9.6 5.6 12 3.5z" />
    </>
  ),
  app: (
    <>
      <rect x="7" y="2.5" width="10" height="19" rx="2.6" />
      <path d="M10.8 18.4h2.4" />
    </>
  ),
  other: <path d="M4.5 19.5h4L18 10a2.7 2.7 0 0 0-3.8-3.8L4.5 15.5v4z" />,
};

function PosTypeIcon({ type }: { type?: string | null }) {
  const label = (type && POS_TYPE_RU[type]) || "тип неизвестен";
  return (
    <span title={label} aria-label={label} className="flex h-[26px] w-[26px] flex-none items-center justify-center rounded-[9px] bg-inset text-tx3">
      <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round">
        {(type && POS_TYPE_PATH[type]) || <path d="M8 12h8" />}
      </svg>
    </span>
  );
}

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

// LoadMore is the bottom-of-list sentinel: it asks for the next page when it
// scrolls into view, one page ahead of the last row (rootMargin) so the list
// grows before the user reaches the end. It stays an observer rather than a
// scroll listener — no throttling to tune, and it works inside whatever
// scroll container the page ends up with. It is also a real button: the
// observer never fires for a keyboard user who tabs to the end, nor in a
// backgrounded tab, and a list that stops loading with no way to continue is
// the bug this replaced.
function LoadMore({ onVisible, busy }: { onVisible: () => void; busy: boolean }) {
  const ref = useRef<HTMLButtonElement>(null);
  // `busy` is a dependency on purpose: an IntersectionObserver only reports
  // *changes*, so a sentinel that stays in view after a page lands never
  // fires again and the list stops one page in. Re-observing once the fetch
  // settles re-delivers the current state, which continues the scroll for as
  // long as the sentinel is still on screen.
  useEffect(() => {
    const el = ref.current;
    if (!el || busy) return;
    const io = new IntersectionObserver(
      (entries) => {
        if (entries[0]?.isIntersecting) onVisible();
      },
      { rootMargin: "300px" },
    );
    io.observe(el);
    return () => io.disconnect();
  }, [onVisible, busy]);
  return (
    <button
      ref={ref}
      type="button"
      onClick={onVisible}
      disabled={busy}
      className="w-full py-2 text-center text-[10.5px] font-medium text-tx4"
    >
      {busy ? "Загрузка…" : "Ещё"}
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
  const filteredOut = tab === "shops" && posType !== "" && merchantRows.length === 0 && !merchants.isPending;
  const nothingFound =
    active &&
    !codes.isPending &&
    !merchants.isPending &&
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
              {/* Load-more sentinel: crossing it pulls the next page, so the
                  list ends where the matches end rather than at the page size. */}
              {merchants.hasNextPage && (
                <LoadMore onVisible={merchants.fetchNextPage} busy={merchants.isFetchingNextPage} />
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

          {(codes.isPending || merchants.isPending) && active && <Spinner />}
          {codes.isError && <ErrMsg error={codes.error} />}
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
              <p className="text-sm font-medium text-tx3">Ничего не найдено — попробуй иначе или введи MCC-код с чека.</p>
              <button
                type="button"
                onClick={() => navigate(`/pos/new?query=${encodeURIComponent(q.trim())}`)}
                className="w-full rounded-2xl border border-dashed border-dash py-2.5 text-sm font-semibold text-tx3"
              >
                + Добавить точку вручную
              </button>
            </Card>
          )}
        </div>
      )}

      {/* mcc-codes.ru permits copying with attribution at the point of use
          (2026-07-29) — this line is the license, keep it. */}
      <p className="mx-1 text-[10.5px] leading-snug font-medium text-tx4">
        Справочник MCC и база точек — данные <span className="font-semibold text-tx3">mcc-codes.ru</span>; код можно узнать из чека
        или выписки.
      </p>
    </>
  );
}
