import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useSearchParams } from "react-router-dom";
import { api, unwrap, type LookupEntry, type Schemas } from "../api/client";
import { useCards, useCategories } from "../hooks";
import { BankBadge, Btn, Card, ErrMsg, GradientCard, Pct, SegTabs, Spinner } from "../components/ui";
import { capNote, currencyBadge, fmtPercent } from "../lib";

type AvailableEntry = Schemas["AvailableEntryDTO"];

// S3b verdict copy — fact-based states, never guesses (spec S3b).
function verdictNote(e: AvailableEntry): string {
  const parts: string[] = [];
  switch (e.verdict) {
    case "free":
      parts.push(e.kind === "super" ? "барабан — не занимает слот" : "можно выбрать сейчас");
      break;
    case "paid":
      parts.push("смена платная у банка");
      break;
    case "locked":
      parts.push("выбор в банке уже зафиксирован");
      break;
    case "slots_full":
      parts.push("слоты заняты — сначала сними другую");
      break;
    default:
      parts.push("правила банка неизвестны — проверь в приложении");
  }
  if (e.activation === "next_day") parts.push("активируется завтра");
  return parts.join(" · ");
}

const ACTIONABLE = new Set(["free", "paid", "unknown"]);

type Mode = "near" | "search" | "cat";

// The design's map placeholder: striped blocks, two «roads», pulsing dot.
function MapStub() {
  return (
    <div className="relative h-16 overflow-hidden rounded-2xl border border-brd" style={{ background: "repeating-linear-gradient(126deg, var(--t-srf2) 0 13px, var(--t-srf) 13px 26px)" }}>
      <div className="absolute top-[58%] -right-[12%] -left-[12%] h-2 -rotate-[9deg] bg-inset" />
      <div className="absolute -top-1/4 -bottom-1/4 left-2/3 w-2 rotate-[7deg] bg-inset" />
      <div className="absolute top-1/2 left-1/2">
        <span className="absolute top-1/2 left-1/2 h-[22px] w-[22px] rounded-full bg-acc" style={{ animation: "locpulse 2.2s ease-out infinite" }} />
        <span className="absolute top-1/2 left-1/2 h-[15px] w-[15px] -translate-x-1/2 -translate-y-1/2 rounded-full border-[2.5px] border-white bg-acc shadow-[0_4px_12px_-2px_rgba(139,111,255,.9)]" />
      </div>
      <span className="absolute bottom-2 left-2.5 rounded-md bg-bg/70 px-1.5 py-0.5 font-mono text-[10px] font-semibold text-tx3">гео · скоро</span>
    </div>
  );
}

function ComingSoon({ text, onPickCategory }: { text: string; onPickCategory: () => void }) {
  return (
    <Card className="space-y-3 p-4 text-center">
      <p className="text-sm font-medium text-tx3">{text}</p>
      <Btn variant="soft" onClick={onPickCategory}>
        Выбрать категорию вручную
      </Btn>
    </Card>
  );
}

function OtherCardRow({ e }: { e: LookupEntry }) {
  const cap = capNote(e);
  return (
    <div className="flex items-center gap-2.5 rounded-xl border border-brd bg-srf px-3 py-2.5">
      <BankBadge name={e.bank_name} size={26} />
      <div className="min-w-0 flex-1">
        <p className="text-[13px] font-semibold">
          {e.bank_name}
          {e.holder_label && <span className="font-medium text-tx4"> · {e.holder_label}</span>}
          {e.kind === "super" && <span className="ml-1.5 rounded bg-gold/10 px-1 py-[1px] text-[9px] font-bold text-gold">барабан</span>}
        </p>
        <p className="text-[10px] font-medium text-tx4">{cap || currencyBadge(e.currency_kind, e.points_label)}</p>
      </div>
      <Pct percent={e.percent} currency={e.currency_kind} className="text-[14px]" />
    </div>
  );
}

// «Какой картой платить?» — design screens 04–06. «Рядом» and «Поиск» need
// a places/MCC base the backend doesn't have yet (OUT of v1) — they render
// the designed shell with an honest «скоро»; «Категория» is fully wired.
export default function Lookup() {
  const categories = useCategories();
  const cards = useCards();
  const [params, setParams] = useSearchParams();
  const preselect = params.get("cat") ?? "";
  const [mode, setMode] = useState<Mode>("cat");
  const [slug, setSlug] = useState(preselect);
  const [showAll, setShowAll] = useState(false);

  // Categories with active selections come first — those are the answers
  // the owner actually taps mid-month (design 06 shows the common ones).
  const overview = useQuery({
    queryKey: ["overview"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview")),
    staleTime: 60_000,
  });

  const lookup = useQuery({
    queryKey: ["lookup", slug],
    enabled: slug !== "",
    queryFn: async () =>
      unwrap(
        await api.GET("/api/v1/cashback/lookup", {
          params: { query: { category: slug } },
        }),
      ),
  });

  // S3b «Отметить выбранной»: records reality via the ordinary selection
  // endpoint (invariants 1–2 still guard) — the app never picks in the bank.
  const qc = useQueryClient();
  const mark = useMutation({
    mutationFn: async (offerID: number) =>
      unwrap(await api.POST("/api/v1/cashback/selections", { body: { category_offer_id: offerID } })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["lookup"] });
      qc.invalidateQueries({ queryKey: ["overview"] });
    },
  });

  const pick = (s: string) => {
    setSlug(s);
    setParams(s ? { cat: s } : {}, { replace: true });
  };

  const activeSlugs = (overview.data?.categories ?? []).map((g) => g.slug);
  const cats = [...(categories.data ?? [])].sort((a, b) => {
    const ai = activeSlugs.indexOf(a.slug);
    const bi = activeSlugs.indexOf(b.slug);
    if (ai !== -1 || bi !== -1) return (ai === -1 ? 1e9 : ai) - (bi === -1 ? 1e9 : bi);
    return a.title_ru.localeCompare(b.title_ru, "ru");
  });
  const mustShow = slug !== "" && cats.findIndex((c) => c.slug === slug) >= 8;
  const shown = showAll || mustShow ? cats : cats.slice(0, 8);
  const selectedCat = cats.find((c) => c.slug === slug);
  const best = (lookup.data?.ranked ?? [])[0];
  const others = (lookup.data?.ranked ?? []).slice(1);
  // The client's plastics — any of them pays with the shared selection.
  const cardChipsOf = (e: LookupEntry) =>
    (cards.data ?? [])
      .filter((c) => c.bank_client_id === e.bank_client_id)
      .map((c) => `··${String(c.last_4_digits).padStart(4, "0")}`)
      .join(" ");

  return (
    <>
      <h1 className="text-[22px] font-extrabold tracking-tight">Какой картой платить?</h1>

      <SegTabs
        value={mode}
        onChange={setMode}
        options={[
          { value: "near", label: "Рядом" },
          { value: "search", label: "Поиск" },
          { value: "cat", label: "Категория" },
        ]}
      />

      {mode === "near" && (
        <>
          <MapStub />
          <ComingSoon
            text="Определение места по геолокации появится вместе с базой точек и MCC — пока подскажем по категории."
            onPickCategory={() => setMode("cat")}
          />
        </>
      )}

      {mode === "search" && (
        <>
          <div className="flex items-center gap-2.5 rounded-xl border border-brd2 bg-srf2 px-3 py-2.5">
            <svg width="17" height="17" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" className="flex-none text-tx4">
              <circle cx="11" cy="11" r="7" />
              <path d="m20 20-3.4-3.4" />
            </svg>
            <input disabled placeholder="Название, сайт или адрес" className="flex-1 bg-transparent text-sm font-medium outline-none placeholder:text-tx4" />
          </div>
          <p className="mx-1 text-[10.5px] leading-snug font-medium text-tx4">Онлайн-оплата? Гео не нужно — поиск по названию или сайту (как mcc-codes.ru).</p>
          <ComingSoon text="Поиск мест появится вместе с базой точек и MCC — пока подскажем по категории." onPickCategory={() => setMode("cat")} />
        </>
      )}

      {mode === "cat" && (
        <>
          <p className="mx-0.5 text-[11px] font-semibold text-tx3">Категория покупки</p>
          <div className="grid grid-cols-3 gap-1.5">
            {shown.map((c) => (
              <button
                key={c.id}
                type="button"
                onClick={() => pick(c.slug)}
                className={`rounded-xl px-1 py-2.5 text-center text-[11.5px] transition ${
                  slug === c.slug
                    ? "grad-acc font-bold text-white shadow-[0_8px_18px_-8px_rgba(139,111,255,.9)]"
                    : "border border-brd bg-srf font-semibold text-tx2"
                }`}
              >
                {c.title_ru}
              </button>
            ))}
            {!showAll && !mustShow && cats.length > 8 && (
              <button type="button" onClick={() => setShowAll(true)} className="rounded-xl border border-brd bg-srf px-1 py-2.5 text-center text-[11.5px] font-semibold text-tx3">
                Ещё…
              </button>
            )}
          </div>

          {slug !== "" && (
            <>
              {lookup.isPending && <Spinner />}
              {lookup.isError && <ErrMsg error={lookup.error} />}
              {lookup.data && (
                <>
                  {lookup.data.message ? (
                    <Card className="space-y-1.5 p-4 text-center">
                      <p className="text-sm font-semibold text-tx2">{lookup.data.message}</p>
                      <p className="text-[10.5px] font-medium text-tx4">
                        Карта попадает сюда, когда в её периоде есть строка с этой категорией и она выбрана.
                      </p>
                    </Card>
                  ) : (
                    best && (
                      <>
                        <p className="mx-0.5 text-[11px] font-semibold text-tx3">Лучшая карта · {selectedCat?.title_ru}</p>
                        <GradientCard className="p-4">
                          <p className="text-[9.5px] font-bold uppercase tracking-[.16em] text-white/75">Платите этой картой</p>
                          <div className="mt-3 flex items-end justify-between">
                            <div className="min-w-0">
                              <p className="text-[22px] leading-none font-extrabold tracking-tight">{best.bank_name}</p>
                              {best.kind === "super" && (
                                <span className="mt-1.5 inline-flex rounded-[8px] bg-white/20 px-2 py-0.5 text-[10px] font-bold">барабан · суммируется</span>
                              )}
                              <p className="mt-1.5 text-[11px] font-semibold text-white/85">
                                {[best.holder_label, cardChipsOf(best) || "любая карта"].filter(Boolean).join(" · ")}
                              </p>
                              {capNote(best) && (
                                <span className="mt-2.5 inline-flex rounded-[10px] bg-white/20 px-2.5 py-1 text-[10.5px] font-bold">{capNote(best)}</span>
                              )}
                            </div>
                            <div className="flex-none text-right">
                              <p className="text-[44px] leading-[.8] font-extrabold tracking-tighter">{fmtPercent(best.percent)}</p>
                              <p className="mt-1.5 text-[10.5px] font-semibold text-white/85">
                                {best.currency_kind === "rub" ? "рублями" : best.points_label || "баллами"}
                              </p>
                            </div>
                          </div>
                        </GradientCard>
                      </>
                    )
                  )}

                  {others.length > 0 && (
                    <>
                      <p className="mx-0.5 text-[11px] font-semibold text-tx3">Другие карты для «{selectedCat?.title_ru}»</p>
                      <div className="space-y-1.5">
                        {others.map((e, i) => (
                          <OtherCardRow key={i} e={e} />
                        ))}
                      </div>
                    </>
                  )}

                  {(lookup.data.available ?? []).length > 0 && (
                    <>
                      <p className="mx-0.5 text-[11px] font-semibold text-accl">
                        Можно выбрать · есть в меню, но не выбрано
                      </p>
                      <div className="space-y-1.5">
                        {(lookup.data.available ?? []).map((e) => {
                          const actionable = ACTIONABLE.has(e.verdict);
                          return (
                            <div
                              key={e.offer_id}
                              className={`flex items-center gap-2.5 rounded-xl border px-3 py-2.5 ${
                                actionable ? "border-acc/30 bg-acc/5" : "border-brd2 bg-transparent opacity-70"
                              }`}
                            >
                              <BankBadge name={e.bank_name} size={26} />
                              <div className="min-w-0 flex-1">
                                <p className="text-[13px] font-semibold">
                                  {e.bank_name}
                                  {e.holder_label && <span className="font-medium text-tx4"> · {e.holder_label}</span>}
                                  {e.kind === "super" && (
                                    <span className="ml-1.5 rounded bg-gold/10 px-1 py-[1px] text-[9px] font-bold text-gold">барабан</span>
                                  )}
                                </p>
                                <p className="text-[10px] font-medium text-tx4">{verdictNote(e)}</p>
                              </div>
                              <Pct percent={e.percent} currency={e.currency_kind} className="text-[14px]" />
                              {actionable && (
                                <Btn
                                  variant="soft"
                                  className="!px-2.5 !py-1.5 text-xs whitespace-nowrap"
                                  disabled={mark.isPending}
                                  onClick={() => mark.mutate(e.offer_id)}
                                >
                                  Отметить
                                </Btn>
                              )}
                            </div>
                          );
                        })}
                      </div>
                      <p className="mx-0.5 text-[10px] font-medium text-tx4">
                        Сначала выбери категорию в приложении банка, потом отметь здесь.
                      </p>
                      <ErrMsg error={mark.error} />
                    </>
                  )}

                  {(lookup.data.special ?? []).length > 0 && (
                    <>
                      <p className="mx-0.5 text-[11px] font-semibold text-gold">Спец-предложения (вне рейтинга)</p>
                      <div className="space-y-1.5">
                        {(lookup.data.special ?? []).map((e, i) => (
                          <div key={i} className="flex items-center gap-2.5 rounded-xl border border-dashed border-gold/30 bg-gold/5 px-3 py-2.5">
                            <BankBadge name={e.bank_name} size={26} />
                            <span className="flex-1 text-[13px] font-semibold">{e.bank_name}</span>
                            <span className="text-[13px] font-extrabold text-gold">{fmtPercent(e.percent)}</span>
                          </div>
                        ))}
                      </div>
                    </>
                  )}

                  {(lookup.data.fallback ?? []).length > 0 && (
                    <>
                      <p className="mx-0.5 text-[11px] font-semibold text-tx3">Остальное — «За все покупки»</p>
                      <div className="space-y-1.5">
                        {(lookup.data.fallback ?? []).map((e, i) => (
                          <OtherCardRow key={`f-${i}`} e={e} />
                        ))}
                      </div>
                    </>
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
            </>
          )}
        </>
      )}
    </>
  );
}
