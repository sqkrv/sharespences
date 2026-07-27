import { useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useParams } from "react-router-dom";
import { api, unwrap, attachmentURL, uploadAttachment, ApiError, type CanonicalCategory, type CategoryOffer, type HelperRow } from "../api/client";
import { useBankCategories, useBanks, useCards, useCategories, useClients, useTierMap } from "../hooks";
import { Badge, Btn, Card, CheckDot, ErrMsg, Field, GradientCard, Input, Pct, Select, Spinner } from "../components/ui";
import { CategoryPicker, type PickedCategory } from "../components/CategoryPicker";
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

// S1 menu entry: pick the category from the bank's catalog (searchable
// select with emoji; custom categories live inside the picker), %, kind.
// Picking a catalog row prefills the kind hint and the canonical mapping.
function AddOfferForm({
  periodID,
  bankID,
  bankName,
  bankColor,
}: {
  periodID: number;
  bankID: number;
  bankName: string;
  bankColor?: string | null;
}) {
  const qc = useQueryClient();
  const [picked, setPicked] = useState<PickedCategory | null>(null);
  const [percent, setPercent] = useState("");
  const [cap, setCap] = useState("");
  const [kind, setKind] = useState<"regular" | "super" | "special">("regular");

  const create = useMutation({
    mutationFn: async () => {
      if (!picked) return;
      return unwrap(
        await api.POST("/api/v1/cashback/category-offers", {
          body: {
            offer_period_id: periodID,
            raw_title: picked.title,
            ...(picked.bankCategoryID != null ? { bank_category_id: picked.bankCategoryID } : {}),
            ...(picked.canonicalID != null ? { canonical_category_id: picked.canonicalID } : {}),
            ...(percent ? { percent } : {}),
            ...(cap ? { cap_value: cap } : {}),
            kind,
          },
        }),
      );
    },
    onSuccess: () => {
      setPicked(null);
      setPercent("");
      setCap("");
      setKind("regular");
      qc.invalidateQueries({ queryKey: ["period", periodID] });
      qc.invalidateQueries({ queryKey: ["helper", periodID] });
      qc.invalidateQueries({ queryKey: ["overview"] });
    },
  });

  return (
    <Card className="p-4" data-sid="CB-03.a">
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <h3 className="text-[13px] font-bold">Добавить категорию из меню банка</h3>
        <div className="grid grid-cols-[1fr_88px] gap-3">
          <Field label="Категория">
            <CategoryPicker
              bankID={bankID}
              bankName={bankName}
              bankColor={bankColor}
              periodID={periodID}
              value={picked}
              onChange={(v) => {
                setPicked(v);
                if (v.kind) setKind(v.kind);
              }}
            />
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
                ["super", "барабан"],
                ["special", "спец"],
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
          {/* Per-offer cap (ВТБ «Кешбэк до N ₽») — static display, no tracking. */}
          <Input inputMode="decimal" className="w-32 flex-none" value={cap} onChange={(e) => setCap(e.target.value)} placeholder="лимит (до ₽)" />
        </div>
        {/* The compact toggle above invited mis-tagging a барабан as «спец»
            (owner report 2026-07-27) — spell the mechanics out. */}
        {kind !== "regular" && (
          <p className="text-[10.5px] font-medium text-tx4">
            {kind === "super"
              ? "барабан = суперкэшбэк на весь период, суммируется с выбранной категорией"
              : "спец = Пятница / колесо / флеш-акция — с условием (день, сервис)"}
          </p>
        )}
        <Btn type="submit" disabled={create.isPending || !picked}>
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
  bankID,
  bankName,
  bankColor,
  onDone,
}: {
  offer: CategoryOffer;
  categories: CanonicalCategory[];
  bankID: number;
  bankName: string;
  bankColor?: string | null;
  onDone: () => void;
}) {
  const qc = useQueryClient();
  const bankCats = useBankCategories(bankID);
  const [picked, setPicked] = useState<PickedCategory>(() => {
    const canon = categories.find((c) => c.id === offer.canonical_category_id);
    return {
      bankCategoryID: offer.bank_category_id ?? undefined,
      title: offer.raw_title,
      canonicalID: offer.canonical_category_id ?? null,
      canonicalTitle: canon?.title_ru ?? null,
      emoji:
        (bankCats.data ?? []).find((r) => r.id === offer.bank_category_id)?.emoji ?? canon?.emoji ?? null,
    };
  });
  const [percent, setPercent] = useState(offer.percent ?? "");
  const [cap, setCap] = useState(offer.cap_value ?? "");
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
            raw_title: picked.title,
            ...(picked.bankCategoryID != null ? { bank_category_id: picked.bankCategoryID } : {}),
            ...(picked.canonicalID != null ? { canonical_category_id: picked.canonicalID } : {}),
            ...(percent ? { percent } : {}),
            ...(cap ? { cap_value: cap } : {}),
            kind: kind as "regular" | "super" | "special",
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
      data-sid="CB-03.b"
      className="mt-3 space-y-3 rounded-xl bg-srf2 p-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <div className="grid grid-cols-[1fr_88px] gap-3">
        <Field label="Категория">
          <CategoryPicker
            bankID={bankID}
            bankName={bankName}
            bankColor={bankColor}
            periodID={offer.offer_period_id}
            value={picked}
            onChange={(v) => {
              setPicked(v);
              if (v.kind) setKind(v.kind);
            }}
          />
        </Field>
        <Field label="Процент">
          <Input inputMode="decimal" value={percent} onChange={(e) => setPercent(e.target.value)} />
        </Field>
      </div>
      <div className="grid grid-cols-2 gap-3">
        <Field label="Тип">
          <Select value={kind} onChange={(e) => setKind(e.target.value)}>
            <option value="regular">обычная</option>
            <option value="super">барабан (суперкэшбэк)</option>
            <option value="special">спец (Пятница/колесо)</option>
          </Select>
        </Field>
        <Field label="Лимит (до)">
          <Input inputMode="decimal" placeholder="необязательно" value={cap} onChange={(e) => setCap(e.target.value)} />
        </Field>
        <Field label="Заметки">
          <Input placeholder="необязательно" value={notes} onChange={(e) => setNotes(e.target.value)} />
        </Field>
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
    <div data-sid="CB-03.c">
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
      data-sid="CB-03.d"
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
  const clients = useClients();
  const cards = useCards();
  const tierMap = useTierMap();
  const qc = useQueryClient();
  const navigate = useNavigate();
  const categories = useCategories();
  const banks = useBanks();
  const bankCats = useBankCategories(period.data?.bank_id);
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
            ? `${err.message} — для ввода истории включите «задним числом»`
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
      // Keep the month picker's dots honest — this month may have lost its
      // only period.
      qc.invalidateQueries({ queryKey: ["periods"] });
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

  const bankColor = (banks.data ?? []).find((b) => b.id === p.bank_id)?.color_hex ?? null;
  // Emoji for a menu row: its catalog row's (override→canonical resolved
  // server-side), else the canonical's, else none.
  const offerEmoji = (offer: CategoryOffer) =>
    (bankCats.data ?? []).find((r) => r.id === offer.bank_category_id)?.emoji ??
    (categories.data ?? []).find((c) => c.id === offer.canonical_category_id)?.emoji ??
    null;

  const client = (clients.data ?? []).find((c) => c.id === p.bank_client_id);
  const tierInfo = client?.program_tier_id != null ? tierMap.data?.get(client.program_tier_id) : undefined;
  const currency = tierInfo ? tierInfo.program.currency_kind : undefined;
  // The client's plastics — any of them pays with this period's selection.
  const clientCards = (cards.data ?? []).filter((c) => c.bank_client_id === p.bank_client_id);
  const cardChips = clientCards.map((c) => `··${String(c.last_4_digits).padStart(4, "0")}`).join(" ");

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
                {[client?.label, cardChips].filter(Boolean).join(" · ") || "любая карта"} · {currencyBadge(currency, tierInfo.program.points_label ?? undefined) === "₽" ? "рубли" : currencyBadge(currency, tierInfo.program.points_label ?? undefined)}
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
          // super | special are both granted bonuses: gold card, no slot, and
          // never blocked by a full menu (super ranks in lookup, special is
          // display-only — the difference lives on the lookup/overview side).
          const isBonus = offer.kind !== "regular";
          // Canonical-less is only a data gap when the row wasn't picked
          // from the catalog: catalog rows without a canonical (Альфа-Тревел,
          // канальные…) are deliberately so (owner 2026-07-21).
          const unmapped = !isBonus && offer.canonical_category_id == null && offer.bank_category_id == null;
          const blocked = !selected && !isBonus && slotsFull;
          const canonTitle = (categories.data ?? []).find((c) => c.id === offer.canonical_category_id)?.title_ru;
          if (isBonus) {
            return (
              <div key={offer.id} className="rounded-xl border border-dashed border-gold/30 bg-gold/5 px-3 py-2.5">
                <div className="flex items-center gap-2.5">
                  <span className="flex h-[21px] w-[21px] flex-none items-center justify-center rounded-md bg-gold/15 text-[11px] font-extrabold text-gold">★</span>
                  <div className="min-w-0 flex-1">
                    <p className="text-[12.5px] font-semibold text-gold">
                      {offerEmoji(offer) && <span className="mr-1">{offerEmoji(offer)}</span>}
                      {offer.raw_title} · {offer.kind === "super" ? "барабан" : "спец"}
                    </p>
                    <p className="text-[9.5px] font-medium text-tx4">
                      не занимает слот · {offer.kind === "super" ? "ранжируется в «Какой картой?»" : "сверх меню"}
                      {offer.cap_value && ` · до ${offer.cap_value} ${currency === "points" ? "баллов" : "₽"}`}
                    </p>
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
                {editingID === offer.id && (
                  <EditOfferForm offer={offer} categories={categories.data ?? []} bankID={p.bank_id} bankName={p.bank_name} bankColor={bankColor} onDone={() => setEditingID(null)} />
                )}
              </div>
            );
          }
          return (
            <div key={offer.id} className={`rounded-xl border px-3 py-2.5 ${selected ? "border-brd bg-srf" : "border-brd2 bg-transparent"}`}>
              <div className="flex items-center gap-2.5">
                <CheckDot checked={selected} />
                <div className="min-w-0 flex-1">
                  <p className="truncate text-[13px] font-semibold">
                    {offerEmoji(offer) && <span className="mr-1">{offerEmoji(offer)}</span>}
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
                  {offer.cap_value && (
                    <p className="mt-0.5 text-[9.5px] font-medium text-tx4">кешбэк до {offer.cap_value} {currency === "points" ? "баллов" : "₽"}</p>
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
                  <>
                    <Pct percent={offer.percent} currency={currency} className="text-[14px]" />
                    <Btn
                      variant="soft"
                      className="!px-2.5 !py-1.5 text-xs whitespace-nowrap"
                      onClick={() => select.mutate(offer.id)}
                      disabled={select.isPending || blocked}
                      title={blocked ? "Лимит категорий исчерпан" : undefined}
                    >
                      выбрать
                    </Btn>
                  </>
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
                  Сравнение: {(hrow?.comparisons ?? []).map((cmp) => `${cmp.client_label} — ${cmp.percent != null ? cmp.percent + "%" : "—"}`).join(" · ")}
                </p>
              )}
              {editingID === offer.id && (
                <EditOfferForm offer={offer} categories={categories.data ?? []} bankID={p.bank_id} bankName={p.bank_name} bankColor={bankColor} onDone={() => setEditingID(null)} />
              )}
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
          задним числом
        </label>
        <Btn className="!px-4 !py-1.5 text-xs" onClick={() => navigate("/")}>
          Готово
        </Btn>
      </div>

      <AddOfferForm periodID={id} bankID={p.bank_id} bankName={p.bank_name} bankColor={bankColor} />

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
