import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ApiError, api, unwrap, type LookupEntry } from "../api/client";
import { Link, useSearchParams } from "react-router-dom";
import { useCards, useCategories } from "../hooks";
import { BankBadge, Btn, Card, ErrMsg, GradientCard, Pct, Spinner } from "../components/ui";
import {
  ACTIONABLE_VERDICTS,
  FALLBACK_EMOJI,
  FRIENDS_KEY,
  capNote,
  currencyWord,
  fmtPercent,
  initWithFriends,
  verdictNote,
} from "../lib";
import { pushRecent } from "../recent";

// CB-11 «Точка продаж» (redesign 2b): the former CB-04 result state as its
// own screen, zero clicks after a search tap — the card to pay with on top,
// the MCC chip, then «как считает каждый банк». Reached with ?mcc= (+
// optional &merchant= for the title) or ?cat=<canonical slug>.
//
// Points never convert to rubles (invariant 5): the ranking groups рубли
// first and the balance rows keep their own currency, stated in place.

function KindBadge({ kind, stacked }: { kind?: string; stacked?: boolean }) {
  if (!stacked && kind !== "super" && kind !== "special" && kind !== "partner") return null;
  return (
    <span className="ml-1.5 rounded bg-gold/10 px-1 py-[1px] text-[9px] font-bold text-gold">
      {kind === "special" ? "спец" : kind === "partner" ? "партнёрка" : "барабан"}
    </span>
  );
}

function stackNote(e: LookupEntry): string {
  if (e.stacked_super == null) return "";
  return `${fmtPercent(e.stacked_regular ?? undefined)} + ${fmtPercent(e.stacked_super)}`;
}

function specialNote(e: LookupEntry): string {
  return [e.raw_title, "проверь условие в банке"].filter(Boolean).join(" · ");
}

// One bank's line in the breakdown: the bank, ITS OWN menu title for the
// category (raw_title), the state, the rate.
function BankRow({ e, note }: { e: LookupEntry; note?: string }) {
  return (
    <div className={`flex items-center gap-2.5 rounded-2xl border px-3 py-2.5 ${e.friend_name ? "border-acc/40 bg-srf" : e.kind === "special" ? "border-gold/30 bg-gold/5" : "border-brd bg-srf"}`}>
      <BankBadge name={e.bank_name} size={26} />
      <div className="min-w-0 flex-1">
        <p className="text-[13px] font-semibold">
          {e.bank_name}
          {e.raw_title && <span className="font-medium text-tx4"> · «{e.raw_title}»</span>}
          <KindBadge kind={e.kind} stacked={e.stacked_super != null} />
        </p>
        <p className="truncate text-[10px] font-medium text-tx4">
          {e.friend_name
            ? `друг · ${e.friend_name} — попроси оплатить`
            : e.kind === "partner"
              ? [
                  e.partner_scope === "merchant" ? `только в «${e.raw_title}»` : "партнёрская акция",
                  e.needs_activation && "требует активации",
                ]
                  .filter(Boolean)
                  .join(" · ")
              : [e.holder_label && `держатель ${e.holder_label}`, note ?? "выбрано у тебя", e.kind === "special" ? specialNote(e) : stackNote(e) || capNote(e)]
                  .filter(Boolean)
                  .join(" · ")}
        </p>
      </div>
      <Pct percent={e.percent} currency={e.currency_kind} className="text-[15px]" />
    </div>
  );
}

export default function Pos() {
  const [params] = useSearchParams();
  const mcc = params.get("mcc");
  const merchant = params.get("merchant");
  const catParam = params.get("cat");
  const [withFriends, setWithFriends] = useState(initWithFriends);
  const categories = useCategories();
  const cards = useCards();
  const qc = useQueryClient();

  const resolve = useQuery({
    queryKey: ["mcc-resolve", mcc],
    enabled: mcc != null,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/resolve", { params: { query: { code: mcc! } } })),
  });
  const unknownCode = resolve.isError && resolve.error instanceof ApiError && resolve.error.status === 404;

  const slug = catParam ?? (resolve.data?.canonicals ?? [])[0]?.slug ?? null;
  const canon = (categories.data ?? []).find((c) => c.slug === slug);
  const title = merchant ?? canon?.title_ru ?? (mcc ? `MCC ${mcc}` : "Точка продаж");

  const lookup = useQuery({
    queryKey: ["lookup", slug],
    enabled: slug != null,
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/lookup", { params: { query: { category: slug! } } })),
  });

  // Everything a bank client answers with is above; clients with no line at
  // all get an honest grey row — «меню не занесено» ≠ «нет в меню» (mock
  // defect 7: unknown is not absence).
  const overview = useQuery({
    queryKey: ["overview"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview")),
    staleTime: 60_000,
  });

  const mark = useMutation({
    mutationFn: async (offerID: number) =>
      unwrap(await api.POST("/api/v1/cashback/selections", { body: { category_offer_id: offerID } })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["lookup"] });
      qc.invalidateQueries({ queryKey: ["overview"] });
    },
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
  const coveredClients = new Set([
    ...ranked.filter((e) => !e.friend_name).map((e) => e.bank_client_id),
    ...available.map((e) => e.bank_client_id),
  ]);
  const uncovered = (overview.data?.clients ?? []).filter((c) => !coveredClients.has(c.bank_client_id));
  const currencies = new Set(ranked.map((e) => e.currency_kind));
  const cardChipsOf = (e: LookupEntry) =>
    (cards.data ?? [])
      .filter((c) => c.bank_client_id === e.bank_client_id)
      .map((c) => `··${String(c.last_4_digits).padStart(4, "0")}`)
      .join(" ");

  return (
    <>
      <div className="flex items-center gap-2.5">
        <Link to="/search" className="flex h-[33px] w-[33px] flex-none items-center justify-center rounded-[11px] border border-brd bg-srf">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--t-tx2)" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
            <path d="M14.5 5 8 12l6.5 7" />
          </svg>
        </Link>
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

      {best && (
        <GradientCard className="p-4" data-sid="CB-11.a" data-sid-inside="">
          <p className="text-[9.5px] font-bold tracking-[.16em] text-white/75 uppercase">Платите этой картой</p>
          <div className="mt-3 flex items-end justify-between">
            <div className="min-w-0">
              <p className="text-[22px] leading-none font-extrabold tracking-tight">{best.bank_name}</p>
              {best.friend_name && (
                <span className="mt-1.5 mr-1 inline-flex rounded-[8px] bg-white/20 px-2 py-0.5 text-[10px] font-bold">
                  карта друга · {best.friend_name}
                </span>
              )}
              {best.stacked_super != null && (
                <span className="mt-1.5 inline-flex rounded-[8px] bg-white/20 px-2 py-0.5 text-[10px] font-bold">барабан · {stackNote(best)}</span>
              )}
              {best.kind === "super" && (
                <span className="mt-1.5 inline-flex rounded-[8px] bg-white/20 px-2 py-0.5 text-[10px] font-bold">барабан · суммируется</span>
              )}
              {best.kind === "special" && (
                <span className="mt-1.5 inline-flex rounded-[8px] bg-white/20 px-2 py-0.5 text-[10px] font-bold">спец · {specialNote(best)}</span>
              )}
              <p className="mt-1.5 text-[11px] font-semibold text-white/85">
                {best.friend_name
                  ? [best.holder_label, `попроси оплатить — @${best.friend_username}`].filter(Boolean).join(" · ")
                  : [best.holder_label, cardChipsOf(best) || "любая карта"].filter(Boolean).join(" · ")}
              </p>
              {capNote(best) && (
                <span className="mt-2.5 inline-flex rounded-[10px] bg-white/20 px-2.5 py-1 text-[10.5px] font-bold">{capNote(best)}</span>
              )}
            </div>
            <div className="flex-none text-right">
              <p className="text-[44px] leading-[.8] font-extrabold tracking-tighter">{fmtPercent(best.percent)}</p>
              <p className="mt-1.5 text-[10.5px] font-semibold text-white/85">{currencyWord(best.currency_kind, best.points_label)}</p>
            </div>
          </div>
        </GradientCard>
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
        <Card className="flex items-center gap-2.5 p-3.5" data-sid="CB-11.b">
          <span className="flex-none font-mono text-[15px] font-extrabold text-accl">{resolve.data.code.code}</span>
          <div className="min-w-0 flex-1">
            <p className="text-[13px] font-bold">{resolve.data.code.name}</p>
            <p className="mt-0.5 text-[10.5px] font-medium text-tx4">
              {canon ? `канон: ${canon.emoji || FALLBACK_EMOJI} ${canon.title_ru}` : "канонической категории нет"}
            </p>
          </div>
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

          <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Как считает каждый банк</p>
          <div className="space-y-1.5" data-sid="CB-11.c">
            {others.map((e, i) => (
              <BankRow key={i} e={e} />
            ))}
            {available.map((e) => {
              const actionable = ACTIONABLE_VERDICTS.has(e.verdict);
              return (
                <div
                  key={e.offer_id}
                  className={`flex items-center gap-2.5 rounded-2xl border px-3 py-2.5 ${
                    actionable ? "border-dashed border-dash bg-srf/50" : "border-brd2 bg-transparent opacity-65"
                  }`}
                >
                  <BankBadge name={e.bank_name} size={26} />
                  <div className="min-w-0 flex-1">
                    <p className="text-[13px] font-semibold">
                      {e.bank_name}
                      {e.raw_title && <span className="font-medium text-tx4"> · «{e.raw_title}»</span>}
                      <KindBadge kind={e.kind} />
                    </p>
                    <p className="text-[10px] font-medium text-tx4">в меню, не выбрано · {verdictNote(e)}</p>
                  </div>
                  <Pct percent={e.percent} currency={e.currency_kind} className="text-[14px]" />
                  {actionable && (
                    <Btn variant="soft" className="!px-2.5 !py-1.5 text-xs whitespace-nowrap" disabled={mark.isPending} onClick={() => mark.mutate(e.offer_id)}>
                      Отметить
                    </Btn>
                  )}
                </div>
              );
            })}
            {uncovered.map((c) => (
              <div key={c.bank_client_id} className="flex items-center gap-2.5 rounded-2xl border border-brd bg-srf/45 px-3 py-2.5 opacity-65">
                <BankBadge name={c.bank_name} size={26} />
                <div className="min-w-0 flex-1">
                  <p className="text-[13px] font-semibold text-tx3">
                    {c.bank_name}
                    {c.holder_label && <span className="font-medium text-tx4"> · {c.holder_label}</span>}
                  </p>
                  <p className="text-[10px] font-medium text-tx4">{c.period_id == null ? "меню не занесено" : "нет в меню месяца"}</p>
                </div>
                <span className="text-[15px] font-extrabold text-tx4">—</span>
              </div>
            ))}
          </div>
          <ErrMsg error={mark.error} />

          {(lookup.data.fallback ?? []).length > 0 && (
            <>
              <p className="mx-0.5 text-[11px] font-semibold text-tx3">Остальное — «За все покупки»</p>
              <div className="space-y-1.5">
                {(lookup.data.fallback ?? []).map((e, i) => (
                  <BankRow key={`f-${i}`} e={e} note="база" />
                ))}
              </div>
            </>
          )}

          {currencies.has("points") && currencies.size > 1 && (
            <p className="mx-0.5 text-[10.5px] leading-snug font-medium text-tx4">
              Баллы показываются отдельно и в рубли не пересчитываются — рублёвый победитель просто стоит выше.
            </p>
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
