import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, unwrap, attachmentURL, uploadAttachment, ApiError, type CanonicalCategory, type CategoryOffer, type HelperRow } from "../api/client";
import { useCards, useCategories, useTierMap } from "../hooks";
import { Badge, Btn, Card, CheckDot, ErrMsg, Field, GradientCard, Input, Pct, Select, Spinner } from "../components/ui";
import { currencyBadge, fmtRange } from "../lib";

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

async function suggestCanonical(periodID: number, rawTitle: string) {
  const res = unwrap(
    await api.GET("/api/v1/cashback/alias-suggestion", {
      params: { query: { offer_period_id: periodID, raw_title: rawTitle } },
    }),
  );
  return res.suggestion ?? null;
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
  const [kind, setKind] = useState<"regular" | "special" | "base">("regular");
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
        const s = await suggestCanonical(periodID, rawTitle);
        setSuggestion(s);
        if (s && !canonicalTouched) setCanonicalID(String(s.id));
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
      // The debounce may not have fired yet (fast submit) — resolve the
      // suggestion synchronously so the row doesn't silently lose its
      // canonical mapping and vanish from «Какой картой?».
      if (catID == null && !newCat && !canonicalTouched) {
        try {
          const s = await suggestCanonical(periodID, rawTitle);
          if (s) catID = s.id;
        } catch {
          // best effort
        }
      }
      return unwrap(
        await api.POST("/api/v1/cashback/category-offers", {
          body: {
            offer_period_id: periodID,
            raw_title: rawTitle,
            ...(catID != null ? { canonical_category_id: catID } : {}),
            ...(percent ? { percent } : {}),
            kind,
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
      setKind("regular");
      setNewCat(false);
      setNewSlug("");
      setNewTitle("");
      qc.invalidateQueries({ queryKey: ["period", periodID] });
      qc.invalidateQueries({ queryKey: ["helper", periodID] });
      qc.invalidateQueries({ queryKey: ["overview"] });
    },
  });

  return (
    <Card className="p-4">
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <h3 className="text-[13px] font-bold">Добавить категорию из меню банка</h3>
        <Field label="Название — как в приложении банка">
          <Input required value={rawTitle} onChange={(e) => setRawTitle(e.target.value)} placeholder="Супермаркеты" />
        </Field>
        {suggestion && (
          <p className="text-xs font-medium text-mint">
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
        <div className="flex flex-wrap items-center gap-3 text-[12px] font-medium text-tx2">
          <span className="inline-flex overflow-hidden rounded-lg border border-brd2">
            {(
              [
                ["regular", "обычная"],
                ["special", "спец"],
                ["base", "база"],
              ] as const
            ).map(([k, label]) => (
              <button
                key={k}
                type="button"
                onClick={() => setKind(k)}
                className={`px-2.5 py-1.5 text-[11px] font-semibold ${kind === k ? "grad-acc text-white" : "bg-srf2 text-tx3"}`}
              >
                {label}
              </button>
            ))}
          </span>
          <label className="flex items-center gap-2">
            <input type="checkbox" checked={newCat} onChange={(e) => setNewCat(e.target.checked)} />
            новая категория
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
    </Card>
  );
}

// Inline editor for an existing row (owner feedback 2026-07-04: entered
// rows must be correctable — fixing the canonical mapping here is what
// makes the row appear in «Какой картой?»). Deletion lives here too.
function EditOfferForm({
  offer,
  categories,
  onDone,
}: {
  offer: CategoryOffer;
  categories: CanonicalCategory[];
  onDone: () => void;
}) {
  const qc = useQueryClient();
  const [rawTitle, setRawTitle] = useState(offer.raw_title);
  const [canonicalID, setCanonicalID] = useState(offer.canonical_category_id != null ? String(offer.canonical_category_id) : "");
  const [percent, setPercent] = useState(offer.percent ?? "");
  const [kind, setKind] = useState(offer.kind);
  const [notes, setNotes] = useState(offer.notes ?? "");

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["period", offer.offer_period_id] });
    qc.invalidateQueries({ queryKey: ["helper", offer.offer_period_id] });
    qc.invalidateQueries({ queryKey: ["lookup"] });
    qc.invalidateQueries({ queryKey: ["overview"] });
  };

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.PUT("/api/v1/cashback/category-offers/{id}", {
          params: { path: { id: offer.id } },
          body: {
            raw_title: rawTitle,
            ...(canonicalID ? { canonical_category_id: Number(canonicalID) } : {}),
            ...(percent ? { percent } : {}),
            kind: kind as "regular" | "special" | "base",
            ...(notes ? { notes } : {}),
          },
        }),
      ),
    onSuccess: () => {
      invalidate();
      onDone();
    },
  });

  const remove = useMutation({
    mutationFn: async () =>
      unwrap(await api.DELETE("/api/v1/cashback/category-offers/{id}", { params: { path: { id: offer.id } } })),
    onSuccess: () => {
      invalidate();
      onDone();
    },
  });

  return (
    <form
      className="mt-3 space-y-3 rounded-xl bg-srf2 p-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <Field label="Название">
        <Input required value={rawTitle} onChange={(e) => setRawTitle(e.target.value)} />
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Каноническая категория">
          <Select value={canonicalID} onChange={(e) => setCanonicalID(e.target.value)}>
            <option value="">— без категории —</option>
            {categories.map((c) => (
              <option key={c.id} value={c.id}>
                {c.title_ru}
              </option>
            ))}
          </Select>
        </Field>
        <Field label="Процент">
          <Input inputMode="decimal" value={percent} onChange={(e) => setPercent(e.target.value)} />
        </Field>
      </div>
      <div className="flex items-center justify-between gap-3">
        <Select value={kind} onChange={(e) => setKind(e.target.value)} className="w-32">
          <option value="regular">обычная</option>
          <option value="special">спец</option>
          <option value="base">база</option>
        </Select>
        <Input placeholder="Заметки" value={notes} onChange={(e) => setNotes(e.target.value)} className="flex-1" />
      </div>
      <div className="flex gap-2">
        <Btn type="submit" disabled={save.isPending}>
          Сохранить
        </Btn>
        <Btn type="button" variant="ghost" onClick={onDone}>
          Отмена
        </Btn>
        <span className="flex-1" />
        <Btn
          type="button"
          variant="danger"
          disabled={remove.isPending}
          onClick={() => {
            if (window.confirm(`Удалить «${offer.raw_title}»${offer.selection_id != null ? " вместе с выбором" : ""}?`)) {
              remove.mutate();
            }
          }}
        >
          Удалить
        </Btn>
      </div>
      <ErrMsg error={save.error ?? remove.error} />
    </form>
  );
}

// Screenshot evidence, editable after creation (owner 2026-07-09): add via
// the dashed tile, remove via ✕ — a detached orphan file is cleaned up
// server-side.
function ScreenshotStrip({ periodID, attachmentIDs }: { periodID: number; attachmentIDs: string[] }) {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["period", periodID] });

  const add = useMutation({
    mutationFn: async (files: File[]) => {
      for (const f of files) {
        const a = await uploadAttachment(f);
        unwrap(
          await api.POST("/api/v1/cashback/offer-periods/{id}/attachments", {
            params: { path: { id: periodID } },
            body: { attachment_id: a.id },
          }),
        );
      }
    },
    onSuccess: invalidate,
  });

  const detach = useMutation({
    mutationFn: async (attachmentID: string) =>
      unwrap(
        await api.DELETE("/api/v1/cashback/offer-periods/{id}/attachments/{attachment_id}", {
          params: { path: { id: periodID, attachment_id: attachmentID } },
        }),
      ),
    onSuccess: invalidate,
  });

  return (
    <div>
      <div className="flex gap-2 overflow-x-auto">
        {attachmentIDs.map((aid) => (
          <div key={aid} className="relative flex-none">
            <a href={attachmentURL(aid)} target="_blank" rel="noreferrer">
              <img src={attachmentURL(aid)} alt="скриншот меню" className="h-20 rounded-xl border border-brd object-cover" />
            </a>
            <button
              type="button"
              title="Убрать скриншот"
              className="absolute -top-1.5 -right-1.5 flex h-5 w-5 items-center justify-center rounded-full border border-brd bg-srf text-[10px] font-bold text-tx3 hover:text-warn"
              onClick={() => {
                if (window.confirm("Убрать скриншот?")) detach.mutate(aid);
              }}
            >
              ✕
            </button>
          </div>
        ))}
        <label className="flex h-20 w-16 flex-none cursor-pointer flex-col items-center justify-center gap-1 rounded-xl border border-dashed border-dash text-tx4">
          <span className="text-lg leading-none">+</span>
          <span className="text-[9px] font-semibold">скрин</span>
          <input type="file" accept="image/*" multiple className="hidden" onChange={(e) => e.target.files?.length && add.mutate([...e.target.files])} />
        </label>
      </div>
      <ErrMsg error={add.error ?? detach.error} />
    </div>
  );
}

// Editable slot count (owner feedback 2026-07-04: the offered number of
// categories is not constant — override per period, null = tier default).
function SlotsEditor({
  periodID,
  used,
  max,
  override,
}: {
  periodID: number;
  used: number;
  max?: number | null;
  override?: number | null;
}) {
  const qc = useQueryClient();
  const [editing, setEditing] = useState(false);
  const [value, setValue] = useState(max != null ? String(max) : "");

  const save = useMutation({
    mutationFn: async (v: number | null) =>
      unwrap(
        await api.PUT("/api/v1/cashback/offer-periods/{id}/max-categories", {
          params: { path: { id: periodID } },
          body: { value: v },
        }),
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["period", periodID] });
      qc.invalidateQueries({ queryKey: ["helper", periodID] });
      qc.invalidateQueries({ queryKey: ["overview"] });
      setEditing(false);
    },
  });

  if (editing) {
    return (
      <form
        className="flex items-center gap-1.5"
        onSubmit={(e) => {
          e.preventDefault();
          save.mutate(value === "" ? null : Number(value));
        }}
      >
        <Input type="number" min={1} inputMode="numeric" value={value} onChange={(e) => setValue(e.target.value)} className="w-16 !py-1.5" placeholder="—" />
        <Btn type="submit" disabled={save.isPending} className="!px-2.5 !py-1.5 text-xs">
          ОК
        </Btn>
        {override != null && (
          <Btn type="button" variant="ghost" className="!px-2.5 !py-1.5 text-xs" onClick={() => save.mutate(null)} title="Вернуть значение тарифа">
            Сброс
          </Btn>
        )}
        <Btn type="button" variant="ghost" className="!px-2.5 !py-1.5 text-xs" onClick={() => setEditing(false)}>
          ✕
        </Btn>
      </form>
    );
  }
  return (
    <button
      type="button"
      className="text-[11px] font-semibold text-tx3"
      onClick={() => {
        setValue(max != null ? String(max) : "");
        setEditing(true);
      }}
    >
      выбрано <b className="text-tx">{used}</b>
      {max != null && <> из {max}</>}
      {override != null && <span className="font-medium text-tx4"> (вручную)</span>} <span className="text-tx4">✎</span>
    </button>
  );
}

export default function Period() {
  const id = Number(useParams().id);
  const period = usePeriod(id);
  const helper = useHelper(id);
  const cards = useCards();
  const tierMap = useTierMap();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const categories = useCategories();
  const [backfill, setBackfill] = useState(false);
  const [rowErrors, setRowErrors] = useState<Record<number, string>>({});
  const [editingID, setEditingID] = useState<number | null>(null);

  const helperByOffer = useMemo(() => {
    const m = new Map<number, HelperRow>();
    for (const row of helper.data?.rows ?? []) m.set(row.category_offer_id, row);
    return m;
  }, [helper.data]);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["period", id] });
    qc.invalidateQueries({ queryKey: ["helper", id] });
    qc.invalidateQueries({ queryKey: ["lookup"] });
    qc.invalidateQueries({ queryKey: ["overview"] });
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

  const removePeriod = useMutation({
    mutationFn: async () =>
      unwrap(await api.DELETE("/api/v1/cashback/offer-periods/{id}", { params: { path: { id } } })),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      navigate("/");
    },
  });

  if (period.isPending || helper.isPending) return <Spinner />;
  if (period.isError) return <ErrMsg error={period.error} />;
  if (helper.isError) return <ErrMsg error={helper.error} />;

  const p = period.data;
  const h = helper.data;
  const slotsFull = h.max_categories != null && h.slots_used >= h.max_categories;
  const collisionCount = (h.rows ?? []).filter((r) => (r.collisions ?? []).length > 0).length;

  const card = (cards.data ?? []).find((c) => c.id === p.card_id);
  const tierInfo = card?.program_tier_id != null ? tierMap.data?.get(card.program_tier_id) : undefined;
  const currency = tierInfo ? tierInfo.program.currency_kind : undefined;

  return (
    <>
      <div className="flex items-center gap-2.5">
        <Link to="/" className="flex h-8 w-8 flex-none items-center justify-center rounded-[10px] border border-brd bg-srf">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-tx2">
            <path d="M14.5 5 8 12l6.5 7" />
          </svg>
        </Link>
        <h1 className="min-w-0 flex-1 truncate text-lg font-extrabold tracking-tight">{p.bank_name}</h1>
        {tierInfo && <Badge tone="indigo">{tierInfo.tier.name}</Badge>}
      </div>

      {tierInfo && (
        <GradientCard className="p-4">
          <div className="flex items-start justify-between">
            <div>
              <p className="text-[9.5px] font-bold uppercase tracking-[.14em] text-white/70">
                {tierInfo.tier.base_percent != null ? "Базовая ставка" : "Тариф"}
              </p>
              <p className="mt-1.5 text-[34px] leading-none font-extrabold tracking-tight">
                {tierInfo.tier.base_percent != null ? `${tierInfo.tier.base_percent}%` : tierInfo.tier.name}
              </p>
              <p className="mt-2 text-[11px] font-semibold text-white/85">
                ···· {String(card!.last_4_digits).padStart(4, "0")} · {currencyBadge(currency, tierInfo.program.points_label ?? undefined) === "₽" ? "рубли" : currencyBadge(currency, tierInfo.program.points_label ?? undefined)}
              </p>
            </div>
            <div className="text-right">
              <p className="text-[9.5px] font-bold uppercase tracking-[.1em] text-white/70">Лимит</p>
              <p className="mt-1.5 text-lg font-extrabold whitespace-nowrap">
                {tierInfo.tier.cap_value ?? "—"}
                {currency === "rub" ? " ₽" : ""}
              </p>
              {h.max_categories != null && (
                <span className="mt-2 inline-flex rounded-lg bg-white/20 px-2 py-1 text-[10px] font-bold">до {h.max_categories} категорий</span>
              )}
            </div>
          </div>
        </GradientCard>
      )}

      <div className="flex items-baseline justify-between px-0.5">
        <span className="text-[13px] font-bold">Меню · {fmtRange(p.period_start, p.period_end)}</span>
        <SlotsEditor periodID={id} used={h.slots_used} max={h.max_categories} override={h.max_categories_override} />
      </div>

      <ScreenshotStrip periodID={id} attachmentIDs={p.attachment_ids ?? []} />

      <div className="space-y-1.5">
        {(p.offers ?? []).length === 0 && (
          <p className="rounded-xl border border-brd bg-srf px-3 py-4 text-center text-sm font-medium text-tx3">
            Введите категории из приложения банка — как на скриншоте.
          </p>
        )}
        {(p.offers ?? []).map((offer) => {
          const hrow = helperByOffer.get(offer.id);
          const selected = offer.selection_id != null;
          const isSpecial = offer.kind === "special";
          const isBase = offer.kind === "base";
          const unmapped = !isSpecial && !isBase && offer.canonical_category_id == null;
          const blocked = !selected && !isSpecial && !isBase && slotsFull;
          const canonTitle = (categories.data ?? []).find((c) => c.id === offer.canonical_category_id)?.title_ru;
          if (isBase) {
            return (
              <div key={offer.id} className="rounded-xl border border-dashed border-brd px-3 py-2.5">
                <div className="flex items-center gap-2.5">
                  <span className="flex h-[21px] w-[21px] flex-none items-center justify-center rounded-md bg-acc/10 text-[10px] font-extrabold text-accl">Б</span>
                  <div className="min-w-0 flex-1">
                    <p className="text-[12.5px] font-semibold text-tx2">{offer.raw_title}</p>
                    <p className="text-[9.5px] font-medium text-tx4">база · не занимает слот · на всё остальное</p>
                  </div>
                  <Pct percent={offer.percent} currency={currency} className="text-[13px]" />
                  {selected ? (
                    <Btn variant="danger" className="!px-2.5 !py-1.5 text-xs" onClick={() => unselect.mutate(offer.selection_id!)}>
                      Снять
                    </Btn>
                  ) : (
                    <Btn variant="soft" className="!px-2.5 !py-1.5 text-xs" onClick={() => select.mutate(offer.id)}>
                      Активна
                    </Btn>
                  )}
                  <button type="button" className="px-1 text-tx4" onClick={() => setEditingID(editingID === offer.id ? null : offer.id)} title="Редактировать">
                    ✎
                  </button>
                </div>
                {rowErrors[offer.id] && <p className="mt-1.5 ml-8 rounded-lg bg-warn/10 px-2 py-1 text-[10.5px] font-medium text-warn">{rowErrors[offer.id]}</p>}
                {editingID === offer.id && <EditOfferForm offer={offer} categories={categories.data ?? []} onDone={() => setEditingID(null)} />}
              </div>
            );
          }
          if (isSpecial) {
            return (
              <div key={offer.id} className="rounded-xl border border-dashed border-gold/30 bg-gold/5 px-3 py-2.5">
                <div className="flex items-center gap-2.5">
                  <span className="flex h-[21px] w-[21px] flex-none items-center justify-center rounded-md bg-gold/15 text-[11px] font-extrabold text-gold">★</span>
                  <div className="min-w-0 flex-1">
                    <p className="text-[12.5px] font-semibold text-gold">{offer.raw_title} · спец</p>
                    <p className="text-[9.5px] font-medium text-tx4">не занимает слот · сверх меню</p>
                  </div>
                  {selected ? (
                    <Btn variant="danger" className="!px-2.5 !py-1.5 text-xs" onClick={() => unselect.mutate(offer.selection_id!)}>
                      Снять
                    </Btn>
                  ) : (
                    <Btn variant="soft" className="!px-2.5 !py-1.5 text-xs" onClick={() => select.mutate(offer.id)}>
                      Отметить
                    </Btn>
                  )}
                  <button type="button" className="px-1 text-tx4" onClick={() => setEditingID(editingID === offer.id ? null : offer.id)} title="Редактировать">
                    ✎
                  </button>
                </div>
                {editingID === offer.id && <EditOfferForm offer={offer} categories={categories.data ?? []} onDone={() => setEditingID(null)} />}
              </div>
            );
          }
          return (
            <div key={offer.id} className={`rounded-xl border px-3 py-2.5 ${selected ? "border-brd bg-srf" : "border-brd2 bg-transparent"}`}>
              <div className="flex items-center gap-2.5">
                <CheckDot checked={selected} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px] font-semibold">
                    {offer.raw_title}
                    {canonTitle && canonTitle !== offer.raw_title && <span className="text-[10px] font-medium text-tx4"> → {canonTitle}</span>}
                  </p>
                  {unmapped && (
                    <p className="mt-0.5 flex items-center gap-1.5 text-[9.5px] font-semibold text-warn">
                      <span className="h-[5px] w-[5px] rounded-full bg-warn" />
                      сопоставьте категорию — не попадёт в «Какой картой?»
                    </p>
                  )}
                  {selected && offer.selected_at && (
                    <p className="mt-0.5 text-[9.5px] font-medium text-tx4">выбрано {new Date(offer.selected_at).toLocaleDateString("ru-RU")}</p>
                  )}
                </div>
                {selected ? (
                  <>
                    <Pct percent={offer.percent} currency={currency} className="text-[14px]" />
                    <Btn variant="danger" className="!px-2.5 !py-1.5 text-xs" onClick={() => unselect.mutate(offer.selection_id!)} disabled={unselect.isPending}>
                      Снять
                    </Btn>
                  </>
                ) : (
                  <Btn
                    variant="soft"
                    className="!px-2.5 !py-1.5 text-xs whitespace-nowrap"
                    onClick={() => select.mutate(offer.id)}
                    disabled={select.isPending || blocked}
                    title={blocked ? "Лимит категорий исчерпан" : undefined}
                  >
                    выбрать {offer.percent != null ? `${offer.percent}%` : ""}
                  </Btn>
                )}
                <button type="button" className="px-1 text-tx4" onClick={() => setEditingID(editingID === offer.id ? null : offer.id)} title="Редактировать">
                  ✎
                </button>
              </div>
              {blocked && <p className="mt-1 ml-8 text-[10px] font-medium text-tx4">лимит категорий исчерпан</p>}
              {rowErrors[offer.id] && <p className="mt-1.5 ml-8 rounded-lg bg-warn/10 px-2 py-1 text-[10.5px] font-medium text-warn">{rowErrors[offer.id]}</p>}
              {(hrow?.collisions ?? []).map((c, i) => (
                <p key={i} className="mt-1.5 ml-8 flex items-center gap-1.5 rounded-lg border border-gold/25 bg-gold/10 px-2 py-1 text-[10px] font-medium text-gold">
                  <span className="h-[5px] w-[5px] flex-none rounded-full bg-gold" />
                  {c.message}
                </p>
              ))}
              {(hrow?.comparisons ?? []).length > 0 && (
                <p className="mt-1.5 ml-8 text-[10px] font-medium text-tx4">
                  Сравнение: {(hrow?.comparisons ?? []).map((cmp) => `${cmp.card_label} — ${cmp.percent != null ? cmp.percent + "%" : "—"}`).join(" · ")}
                </p>
              )}
              {editingID === offer.id && <EditOfferForm offer={offer} categories={categories.data ?? []} onDone={() => setEditingID(null)} />}
            </div>
          );
        })}
      </div>

      <div className="sticky bottom-[92px] z-10 flex items-center gap-2.5 rounded-2xl border border-brd2 bg-srf2/95 px-3.5 py-2.5 backdrop-blur">
        <span className="text-[11px] font-bold text-accl">
          Слоты {h.slots_used}
          {h.max_categories != null && `/${h.max_categories}`}
        </span>
        {collisionCount > 0 && (
          <>
            <span className="h-3.5 w-px bg-brd" />
            <span className="text-[11px] font-semibold text-gold">
              {collisionCount} совпаден{collisionCount === 1 ? "ие" : collisionCount < 5 ? "ия" : "ий"}
            </span>
          </>
        )}
        <span className="flex-1" />
        <label className="flex items-center gap-1 text-[10px] font-medium text-tx4">
          <input type="checkbox" checked={backfill} onChange={(e) => setBackfill(e.target.checked)} />
          бэкфилл
        </label>
        <Btn className="!px-4 !py-1.5 text-xs" onClick={() => navigate("/")}>
          Готово
        </Btn>
      </div>

      <AddOfferForm periodID={id} />

      <div className="pt-1 text-right">
        <Btn
          variant="danger"
          onClick={() => {
            if (window.confirm("Удалить период целиком — с меню и выборами?")) removePeriod.mutate();
          }}
          disabled={removePeriod.isPending}
        >
          Удалить период
        </Btn>
      </div>
    </>
  );
}
