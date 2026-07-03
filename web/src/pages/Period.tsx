import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useParams } from "react-router-dom";
import { api, unwrap, attachmentURL, ApiError, type HelperRow } from "../api/client";
import { useCategories } from "../hooks";
import { Badge, Btn, Empty, ErrMsg, Field, Input, Section, Select, Spinner } from "../components/ui";
import { fmtPercent, fmtRange } from "../lib";

function usePeriod(id: number) {
  return useQuery({
    queryKey: ["period", id],
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/cashback/offer-periods/{id}", { params: { path: { id } } })),
  });
}

function useHelper(id: number) {
  return useQuery({
    queryKey: ["helper", id],
    queryFn: async () =>
      unwrap(
        await api.GET("/api/v1/cashback/helper-context", {
          params: { query: { offer_period_id: id } },
        }),
      ),
  });
}

// S1 menu entry: raw title (alias table pre-suggests the canonical
// category), %, spec-kind toggle; unknown titles can create a canonical
// category inline.
function AddOfferForm({ periodID }: { periodID: number }) {
  const categories = useCategories();
  const qc = useQueryClient();
  const [rawTitle, setRawTitle] = useState("");
  const [canonicalID, setCanonicalID] = useState("");
  const [canonicalTouched, setCanonicalTouched] = useState(false);
  const [suggestion, setSuggestion] = useState<{ id: number; title_ru: string } | null>(null);
  const [percent, setPercent] = useState("");
  const [special, setSpecial] = useState(false);
  const [newCat, setNewCat] = useState(false);
  const [newSlug, setNewSlug] = useState("");
  const [newTitle, setNewTitle] = useState("");

  // Debounced alias pre-suggestion (S1) — fills the canonical select unless
  // the user already picked one by hand.
  useEffect(() => {
    if (rawTitle.trim() === "") {
      setSuggestion(null);
      return;
    }
    const t = setTimeout(async () => {
      try {
        const res = unwrap(
          await api.GET("/api/v1/cashback/alias-suggestion", {
            params: { query: { offer_period_id: periodID, raw_title: rawTitle } },
          }),
        );
        setSuggestion(res.suggestion ?? null);
        if (res.suggestion && !canonicalTouched) setCanonicalID(String(res.suggestion.id));
      } catch {
        // suggestion is best-effort; entry must never block on it
      }
    }, 400);
    return () => clearTimeout(t);
  }, [rawTitle, periodID, canonicalTouched]);

  const create = useMutation({
    mutationFn: async () => {
      let catID = canonicalID ? Number(canonicalID) : undefined;
      if (newCat && newSlug && newTitle) {
        const created = unwrap(
          await api.POST("/api/v1/cashback/canonical-categories", {
            body: { slug: newSlug, title_ru: newTitle },
          }),
        );
        catID = created.id;
        qc.invalidateQueries({ queryKey: ["categories"] });
      }
      return unwrap(
        await api.POST("/api/v1/cashback/category-offers", {
          body: {
            offer_period_id: periodID,
            raw_title: rawTitle,
            ...(catID != null ? { canonical_category_id: catID } : {}),
            ...(percent ? { percent } : {}),
            kind: special ? "special" : "regular",
          },
        }),
      );
    },
    onSuccess: () => {
      setRawTitle("");
      setCanonicalID("");
      setCanonicalTouched(false);
      setSuggestion(null);
      setPercent("");
      setSpecial(false);
      setNewCat(false);
      setNewSlug("");
      setNewTitle("");
      qc.invalidateQueries({ queryKey: ["period", periodID] });
      qc.invalidateQueries({ queryKey: ["helper", periodID] });
    },
  });

  return (
    <form
      className="space-y-3 border-t border-slate-100 pt-4"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      <h3 className="text-sm font-semibold text-slate-600">Добавить категорию из меню банка</h3>
      <Field label="Название — как в приложении банка">
        <Input required value={rawTitle} onChange={(e) => setRawTitle(e.target.value)} placeholder="Супермаркеты" />
      </Field>
      {suggestion && (
        <p className="text-xs text-emerald-700">
          ≈ распознано: <b>{suggestion.title_ru}</b>
        </p>
      )}
      <div className="grid grid-cols-2 gap-3">
        <Field label="Каноническая категория">
          <Select
            value={canonicalID}
            onChange={(e) => {
              setCanonicalID(e.target.value);
              setCanonicalTouched(true);
            }}
            disabled={newCat}
          >
            <option value="">— без категории —</option>
            {(categories.data ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {c.title_ru}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Процент">
          <Input inputMode="decimal" value={percent} onChange={(e) => setPercent(e.target.value)} placeholder="5" />
        </Field>
      </div>
      <div className="flex flex-wrap items-center gap-4 text-sm">
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={special} onChange={(e) => setSpecial(e.target.checked)} />
          спец-предложение (барабан / колесо / пятница)
        </label>
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={newCat} onChange={(e) => setNewCat(e.target.checked)} />
          новая каноническая категория
        </label>
      </div>
      {newCat && (
        <div className="grid grid-cols-2 gap-3">
          <Field label="Slug (латиницей)">
            <Input value={newSlug} onChange={(e) => setNewSlug(e.target.value)} pattern="[a-z0-9-]+" placeholder="coffee-shops" />
          </Field>
          <Field label="Название (по-русски)">
            <Input value={newTitle} onChange={(e) => setNewTitle(e.target.value)} placeholder="Кофейни" />
          </Field>
        </div>
      )}
      <Btn type="submit" disabled={create.isPending || !rawTitle}>
        Добавить
      </Btn>
      <ErrMsg error={create.error} />
    </form>
  );
}

export default function Period() {
  const id = Number(useParams().id);
  const period = usePeriod(id);
  const helper = useHelper(id);
  const qc = useQueryClient();
  const [backfill, setBackfill] = useState(false);
  const [rowErrors, setRowErrors] = useState<Record<number, string>>({});

  const helperByOffer = useMemo(() => {
    const m = new Map<number, HelperRow>();
    for (const row of helper.data?.rows ?? []) m.set(row.category_offer_id, row);
    return m;
  }, [helper.data]);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["period", id] });
    qc.invalidateQueries({ queryKey: ["helper", id] });
    qc.invalidateQueries({ queryKey: ["lookup"] });
  };

  const select = useMutation({
    mutationFn: async (offerID: number) =>
      unwrap(
        await api.POST("/api/v1/cashback/selections", {
          body: { category_offer_id: offerID, ...(backfill ? { backfill_override: true } : {}) },
        }),
      ),
    onSuccess: (_, offerID) => {
      setRowErrors((e) => ({ ...e, [offerID]: "" }));
      invalidate();
    },
    onError: (err, offerID) => {
      const msg =
        err instanceof ApiError && err.status === 409
          ? err.message
          : err instanceof ApiError && err.status === 422
            ? `${err.message} — для ввода истории включите «бэкфилл»`
            : String(err);
      setRowErrors((e) => ({ ...e, [offerID]: msg }));
    },
  });

  const unselect = useMutation({
    mutationFn: async (selectionID: number) =>
      unwrap(await api.DELETE("/api/v1/cashback/selections/{id}", { params: { path: { id: selectionID } } })),
    onSuccess: invalidate,
  });

  if (period.isPending || helper.isPending) return <Spinner />;
  if (period.isError) return <ErrMsg error={period.error} />;
  if (helper.isError) return <ErrMsg error={helper.error} />;

  const p = period.data;
  const h = helper.data;
  const slotsFull = h.max_categories != null && h.slots_used >= h.max_categories;

  return (
    <>
      <Section>
        <div className="flex flex-wrap items-center justify-between gap-2">
          <div>
            <h1 className="text-lg font-bold">{h.card_label}</h1>
            <p className="text-sm text-slate-500">{fmtRange(p.period_start, p.period_end)}</p>
          </div>
          <div className="text-right">
            <p className="text-sm font-semibold">
              Выбрано {h.slots_used}
              {h.max_categories != null && ` из ${h.max_categories}`}
            </p>
            <label className="flex items-center gap-1 text-xs text-slate-500">
              <input type="checkbox" checked={backfill} onChange={(e) => setBackfill(e.target.checked)} />
              бэкфилл (ввод истории)
            </label>
          </div>
        </div>
        {(p.attachment_ids ?? []).length > 0 && (
          <div className="mt-3 flex gap-2 overflow-x-auto">
            {(p.attachment_ids ?? []).map((aid) => (
              <a key={aid} href={attachmentURL(aid)} target="_blank" rel="noreferrer">
                <img src={attachmentURL(aid)} alt="скриншот меню" className="h-20 rounded-lg border border-slate-200 object-cover" />
              </a>
            ))}
          </div>
        )}
      </Section>

      <Section title="Меню категорий">
        {(p.offers ?? []).length === 0 && <Empty>Введите категории из приложения банка — как на скриншоте.</Empty>}
        <ul className="space-y-3">
          {(p.offers ?? []).map((offer) => {
            const hrow = helperByOffer.get(offer.id);
            const selected = offer.selection_id != null;
            const isSpecial = offer.kind === "special";
            const blocked = !selected && !isSpecial && slotsFull;
            return (
              <li key={offer.id} className={`rounded-lg border p-3 ${selected ? "border-emerald-300 bg-emerald-50/50" : "border-slate-200"}`}>
                <div className="flex items-center justify-between gap-3">
                  <div>
                    <p className="font-medium">
                      {offer.raw_title}{" "}
                      <span className="text-slate-400">{fmtPercent(offer.percent)}</span>{" "}
                      {isSpecial && <Badge tone="amber">спец</Badge>}
                    </p>
                    {selected && offer.selected_at && (
                      <p className="text-xs text-emerald-700">
                        выбрано {new Date(offer.selected_at).toLocaleDateString("ru-RU")}
                      </p>
                    )}
                  </div>
                  {selected ? (
                    <Btn variant="danger" onClick={() => unselect.mutate(offer.selection_id!)} disabled={unselect.isPending}>
                      Снять
                    </Btn>
                  ) : (
                    <Btn
                      onClick={() => select.mutate(offer.id)}
                      disabled={select.isPending || blocked}
                      title={blocked ? "Лимит категорий исчерпан" : undefined}
                    >
                      Выбрать
                    </Btn>
                  )}
                </div>
                {blocked && <p className="mt-1 text-xs text-slate-400">лимит категорий исчерпан</p>}
                {rowErrors[offer.id] && <p className="mt-2 rounded bg-rose-50 px-2 py-1 text-xs text-rose-700">{rowErrors[offer.id]}</p>}
                {(hrow?.collisions ?? []).map((c, i) => (
                  <p key={i} className="mt-2 rounded bg-amber-50 px-2 py-1 text-xs text-amber-800">
                    ⚠ {c.message}
                  </p>
                ))}
                {(hrow?.comparisons ?? []).length > 0 && (
                  <p className="mt-2 text-xs text-slate-500">
                    Сравнение:{" "}
                    {(hrow?.comparisons ?? [])
                      .map((cmp) => `${cmp.card_label} — ${fmtPercent(cmp.percent)}`)
                      .join(" · ")}
                  </p>
                )}
              </li>
            );
          })}
        </ul>
        <AddOfferForm periodID={id} />
      </Section>
    </>
  );
}
