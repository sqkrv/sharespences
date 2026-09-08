import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { useBanks, usePrograms, useTierMap } from "../hooks";
import { BackButton, BankBadge, Btn, Card, ErrMsg, Field, Input, Spinner } from "../components/ui";
import { monthKey, normalizeTitle, todayISO } from "../lib";

// CB-10 «Новый банк» (mock 2g): the catalog is the single source — bank,
// tariff and держатель are all that is asked; cycle, slots, currency and
// limits ride in with the tariff. A bank without a seeded program (Сбербанк,
// reference-only) gets no tariff section and an honest note instead — the
// wiki holds no facts to offer.

const PERIOD_WORD: Record<string, string> = {
  calendar_month: "месяц",
  quarter: "квартал",
  week: "неделя",
  rolling: "скользящий период",
};

export default function BankNew() {
  const [params] = useSearchParams();
  const banks = useBanks();
  const programs = usePrograms();
  const tierMap = useTierMap();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const [query, setQuery] = useState("");
  const [bankID, setBankID] = useState<number | null>(params.get("bank") ? Number(params.get("bank")) : null);
  const [tierID, setTierID] = useState<number | null>(null);
  const [holder, setHolder] = useState("");

  const list = useMemo(() => {
    const q = normalizeTitle(query);
    return (banks.data ?? []).filter((b) => !q || normalizeTitle(b.name).includes(q));
  }, [banks.data, query]);

  const program = bankID != null ? (programs.data ?? []).find((p) => p.bank_id === bankID) : undefined;
  const tiers = program ? [...(tierMap.data?.values() ?? [])].filter((ti) => ti.program.id === program.id) : [];

  // «месяц · до 5 категорий · Баллы Плюс» — the program summary line, built
  // from seeded facts only (never invented for catalog-less banks).
  const programSummary = (p: typeof program) => {
    if (!p) return "каталог не знает КБ-программу";
    const slots = tiers.map((t) => t.tier.max_categories ?? 0).filter(Boolean);
    return [
      PERIOD_WORD[p.period_type] ?? p.period_type,
      slots.length > 0 ? `до ${Math.max(...slots)} категорий` : "",
      p.currency_kind === "points" ? p.points_label || "баллы" : "рубли",
    ]
      .filter(Boolean)
      .join(" · ");
  };

  const create = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/api/v1/bank-clients", {
          body: {
            bank_id: bankID!,
            ...(holder.trim() ? { label: holder.trim() } : {}),
            ...(tierID != null ? { program_tier_id: tierID } : {}),
          },
        }),
      ),
    onSuccess: (client) => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      qc.invalidateQueries({ queryKey: ["clients"] });
      // Straight into the month menu (2h) — the bank without its menu is a
      // grey row in the feed, so filling is the natural next step.
      navigate(`/periods/new?client=${client.id}&month=${monthKey(todayISO())}`);
    },
  });

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/banks" />
        <h1 className="min-w-0 flex-1 text-xl font-extrabold tracking-tight">Новый банк</h1>
      </div>

      {banks.isPending ? (
        <Spinner />
      ) : (
        <Card className="p-4" data-sid="CB-10.a">
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (bankID != null) create.mutate();
            }}
          >
            <Field label="Банк">
              <Input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Начните вводить название"
                autoFocus={bankID == null}
              />
            </Field>
            <div className="max-h-64 space-y-0.5 overflow-y-auto rounded-xl border border-brd2 bg-srf2 p-1.5">
              {list.length === 0 && <p className="px-2 py-3 text-center text-xs font-medium text-tx4">Такого банка в каталоге нет</p>}
              {list.map((b) => {
                const p = (programs.data ?? []).find((pr) => pr.bank_id === b.id);
                const active = bankID === b.id;
                return (
                  <button
                    key={b.id}
                    type="button"
                    onClick={() => {
                      setBankID(b.id);
                      setTierID(null);
                    }}
                    className={`flex w-full items-center gap-2.5 rounded-[9px] p-2 text-left ${active ? "grad-acc text-white" : ""}`}
                  >
                    <BankBadge name={b.name} size={22} color={b.color_hex} />
                    <span className="min-w-0 flex-1">
                      <span className="block text-[13px] font-bold">{b.name}</span>
                      <span className={`block text-[10px] font-medium ${active ? "text-white/75" : "text-tx4"}`}>
                        {p
                          ? [
                              PERIOD_WORD[p.period_type] ?? p.period_type,
                              p.currency_kind === "points" ? p.points_label || "баллы" : "рубли",
                            ].join(" · ")
                          : "каталог не знает КБ-программу"}
                      </span>
                    </span>
                  </button>
                );
              })}
            </div>

            {bankID != null && program && tiers.length > 0 && (
              <Field label="Тариф">
                <div className="flex overflow-hidden rounded-xl border border-brd2">
                  {tiers.map(({ tier }) => (
                    <button
                      key={tier.id}
                      type="button"
                      onClick={() => setTierID(tierID === tier.id ? null : tier.id)}
                      className={`flex-1 px-1 py-2.5 text-center text-xs font-semibold whitespace-nowrap ${
                        tierID === tier.id ? "grad-acc text-white" : "bg-srf2 text-tx3"
                      }`}
                    >
                      {tier.name}
                    </button>
                  ))}
                </div>
                <span className="mt-1.5 block text-[10px] font-medium text-tx4">
                  лимиты и слоты приходят с тарифом из каталога · {programSummary(program)}
                </span>
              </Field>
            )}
            {bankID != null && !program && (
              <p className="rounded-xl border border-brd2 bg-srf2 px-3 py-2.5 text-[11px] leading-snug font-medium text-tx3">
                Каталог не знает КБ-программу этого банка — слоты и лимиты можно будет указать вручную при заполнении меню.
              </p>
            )}

            <Field label="Держатель первой карты">
              <Input value={holder} onChange={(e) => setHolder(e.target.value)} placeholder="Я" />
            </Field>

            <Btn type="submit" className="w-full" disabled={bankID == null || create.isPending}>
              Добавить банк
            </Btn>
            <ErrMsg error={create.error} />
          </form>
        </Card>
      )}

      <p className="mx-0.5 text-[11px] leading-snug font-medium text-tx4">
        Каталог знает названия категорий банка и их сопоставление с каноном — первый месяц заполнится подсказками, а не с нуля.
      </p>
    </>
  );
}
