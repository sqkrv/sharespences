import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, unwrap, type LookupEntry } from "../api/client";
import { useCategories } from "../hooks";
import { Badge, Empty, ErrMsg, Field, Input, Section, Select, Spinner } from "../components/ui";
import { capNote, currencyBadge, fmtPercent, fmtRange, todayISO } from "../lib";

function EntryCard({ e, rank }: { e: LookupEntry; rank?: number }) {
  const cap = capNote(e);
  return (
    <li className="flex items-center justify-between gap-3 rounded-lg border border-slate-200 p-3">
      <div>
        <p className="font-medium">
          {rank != null && <span className="mr-1 text-slate-400">{rank}.</span>}
          {e.card_label}
        </p>
        <p className="text-xs text-slate-500">
          {fmtRange(e.period_start, e.period_end)}
          {cap && ` · ${cap}`}
        </p>
      </div>
      <div className="text-right">
        <p className="text-xl font-bold text-indigo-700">{fmtPercent(e.percent)}</p>
        <Badge tone={e.currency_kind === "points" ? "indigo" : "green"}>
          {currencyBadge(e.currency_kind, e.points_label)}
        </Badge>
      </div>
    </li>
  );
}

// S3: «Каким банком платить?» — category-level lookup of active selections.
export default function Lookup() {
  const categories = useCategories();
  const [slug, setSlug] = useState("");
  const [date, setDate] = useState(todayISO());

  const lookup = useQuery({
    queryKey: ["lookup", slug, date],
    enabled: slug !== "",
    queryFn: async () =>
      unwrap(
        await api.GET("/api/v1/cashback/lookup", {
          params: { query: { category: slug, date } },
        }),
      ),
  });

  return (
    <>
      <Section title="Какой картой платить?">
        <div className="grid grid-cols-2 gap-3">
          <Field label="Категория">
            <Select value={slug} onChange={(e) => setSlug(e.target.value)}>
              <option value="">— выберите —</option>
              {(categories.data ?? []).map((c) => (
                <option key={c.id} value={c.slug}>
                  {c.title_ru}
                </option>
              ))}
            </Select>
          </Field>
          <Field label="Дата">
            <Input type="date" value={date} onChange={(e) => setDate(e.target.value)} />
          </Field>
        </div>
      </Section>

      {slug !== "" && (
        <Section>
          {lookup.isPending && <Spinner />}
          {lookup.isError && <ErrMsg error={lookup.error} />}
          {lookup.data && (
            <>
              {lookup.data.message ? (
                <>
                  <Empty>{lookup.data.message}</Empty>
                  <p className="mt-2 text-xs text-slate-400">
                    Карта попадает сюда, когда в её периоде есть строка меню, сопоставленная с этой канонической
                    категорией, и она <b>выбрана</b>. Строки «без категории» отмечены в периоде предупреждением.
                  </p>
                </>
              ) : (
                <ul className="space-y-2">
                  {(lookup.data.ranked ?? []).map((e, i) => (
                    <EntryCard key={`${e.card_label}-${i}`} e={e} rank={i + 1} />
                  ))}
                </ul>
              )}
              {(lookup.data.special ?? []).length > 0 && (
                <>
                  <h3 className="mt-4 mb-2 text-sm font-semibold text-amber-700">Спец-предложения (вне рейтинга)</h3>
                  <ul className="space-y-2">
                    {(lookup.data.special ?? []).map((e, i) => (
                      <EntryCard key={`s-${i}`} e={e} />
                    ))}
                  </ul>
                </>
              )}
              {(lookup.data.partner ?? []).length > 0 && (
                <div className="mt-4 border-t border-slate-100 pt-3">
                  <h3 className="mb-1 text-xs font-semibold uppercase text-slate-400">Партнёрские (справочно)</h3>
                  {(lookup.data.partner ?? []).map((p) => (
                    <p key={p.id} className="text-sm text-slate-600">
                      {p.merchant_title} — {fmtPercent(p.percent)} ({p.bank_name}
                      {p.valid_to && ` · до ${p.valid_to}`})
                    </p>
                  ))}
                </div>
              )}
            </>
          )}
        </Section>
      )}
    </>
  );
}
