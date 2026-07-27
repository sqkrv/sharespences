import { useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, type BankCategory } from "../api/client";
import { useBankCategories, useCategories } from "../hooks";
import { BankBadge, Badge, Btn, ErrMsg, Field, Input, Select } from "./ui";
import { FALLBACK_EMOJI, normalizeTitle } from "../lib";

// What a pick means for the entry form: the snapshot title, the canonical
// mapping and the kind hint. bankCategoryID is absent in fallback mode
// (bank without a catalog) — the offer is then recorded without the
// traceability FK, exactly like before the picker existed.
export type PickedCategory = {
  bankCategoryID?: number;
  title: string;
  canonicalID?: number | null;
  canonicalTitle?: string | null;
  kind?: "regular" | "super" | "special";
  emoji?: string | null;
};

function pickFromBankCategory(r: BankCategory): PickedCategory {
  return {
    bankCategoryID: r.id,
    title: r.title,
    canonicalID: r.canonical_category_id ?? null,
    canonicalTitle: r.canonical_title_ru ?? null,
    kind: r.kind as PickedCategory["kind"],
    emoji: r.emoji ?? null,
  };
}

// Searchable popover select over the bank's category catalog (seeded from
// the knowledge base), with an inline «добавить свою» escape hatch for
// categories the bank introduced before the base learned them. Banks
// without a catalog fall back to the global canonical list.
export function CategoryPicker({
  bankID,
  bankName,
  bankColor,
  periodID,
  value,
  onChange,
}: {
  bankID: number;
  bankName: string;
  bankColor?: string | null;
  periodID?: number; // enables the alias pre-suggestion in the custom form
  value: PickedCategory | null;
  onChange: (v: PickedCategory) => void;
}) {
  const [open, setOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [adding, setAdding] = useState(false);
  const ref = useRef<HTMLDivElement>(null);
  const bankCats = useBankCategories(bankID);
  const categories = useCategories();

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (!ref.current?.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  const catalog = bankCats.data ?? [];
  const fallbackMode = bankCats.isSuccess && catalog.length === 0;
  const nq = normalizeTitle(query);

  const rows = useMemo(
    () =>
      catalog.filter(
        (r) =>
          nq === "" ||
          normalizeTitle(r.title).includes(nq) ||
          (r.canonical_title_ru != null && normalizeTitle(r.canonical_title_ru).includes(nq)),
      ),
    [catalog, nq],
  );
  const fallbackRows = useMemo(
    () => (categories.data ?? []).filter((c) => nq === "" || normalizeTitle(c.title_ru).includes(nq)),
    [categories.data, nq],
  );

  const pick = (v: PickedCategory) => {
    onChange(v);
    setOpen(false);
    setQuery("");
    setAdding(false);
  };

  return (
    <div ref={ref} data-sid="W-02" className="relative">
      <button
        type="button"
        aria-haspopup="listbox"
        aria-expanded={open}
        onClick={() => setOpen(!open)}
        className="flex w-full items-center gap-2 rounded-xl border border-brd2 bg-srf2 px-3 py-2.5 text-left text-sm font-medium text-tx focus:border-acc focus:outline-none"
      >
        {value ? (
          <>
            <span className="flex-none">{value.emoji ?? FALLBACK_EMOJI}</span>
            <span className="min-w-0 flex-1 truncate">
              {value.title}
              {value.canonicalTitle && value.canonicalTitle !== value.title && (
                <span className="text-[10px] font-medium text-tx4"> → {value.canonicalTitle}</span>
              )}
            </span>
          </>
        ) : (
          <span className="flex-1 text-tx4">Выберите категорию…</span>
        )}
        <span className="flex-none text-[9px] text-tx4">▾</span>
      </button>

      {open && (
        <div className="absolute left-0 right-0 top-full z-30 mt-2 rounded-2xl border border-brd bg-srf p-3 shadow-[0_14px_40px_-12px_rgba(0,0,0,.55)]">
          <div className="mb-2 flex items-center gap-2">
            <BankBadge name={bankName} size={24} color={bankColor} />
            <span className="min-w-0 flex-1 truncate text-[11px] font-bold">{bankName}</span>
            <span className="text-[9.5px] font-medium text-tx4">
              {fallbackMode ? "канонические категории" : "каталог банка"}
            </span>
          </div>

          {adding ? (
            <AddCustomForm
              bankID={bankID}
              periodID={periodID}
              initialTitle={query}
              onCancel={() => setAdding(false)}
              onCreated={(r) => pick(pickFromBankCategory(r))}
            />
          ) : (
            <>
              <Input
                autoFocus
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Поиск…"
                className="!py-2"
              />
              <div className="mt-2 max-h-72 space-y-0.5 overflow-y-auto">
                {bankCats.isPending && <p className="px-2 py-3 text-center text-xs font-medium text-tx4">Загрузка…</p>}
                {!fallbackMode &&
                  rows.map((r) => {
                    const selected = value?.bankCategoryID === r.id;
                    // Canonical-less catalog rows (Альфа-Тревел, канальные…)
                    // are deliberate — ordinary categories without a
                    // cross-bank identity, no warning (owner 2026-07-21).
                    return (
                      <button
                        key={r.id}
                        type="button"
                        onClick={() => pick(pickFromBankCategory(r))}
                        className={`flex w-full items-center gap-2 rounded-[10px] px-2 py-2 text-left ${
                          selected ? "grad-acc font-bold text-white" : "font-semibold text-tx2 hover:bg-srf2"
                        }`}
                      >
                        <span className="flex-none text-[15px]">{r.emoji ?? FALLBACK_EMOJI}</span>
                        <span className="min-w-0 flex-1">
                          <span className="block truncate text-[13px]">{r.title}</span>
                          {r.canonical_title_ru && r.canonical_title_ru !== r.title && (
                            <span className={`block truncate text-[10px] font-medium ${selected ? "text-white/75" : "text-tx4"}`}>
                              → {r.canonical_title_ru}
                            </span>
                          )}
                        </span>
                        {r.kind !== "regular" && <Badge tone="amber">{r.kind === "super" ? "барабан" : "спец"}</Badge>}
                        {r.is_custom && <Badge tone="slate">своя</Badge>}
                      </button>
                    );
                  })}
                {fallbackMode &&
                  fallbackRows.map((c) => {
                    const selected = value?.bankCategoryID == null && value?.canonicalID === c.id;
                    return (
                      <button
                        key={c.id}
                        type="button"
                        onClick={() => pick({ title: c.title_ru, canonicalID: c.id, canonicalTitle: c.title_ru, emoji: c.emoji ?? null })}
                        className={`flex w-full items-center gap-2 rounded-[10px] px-2 py-2 text-left ${
                          selected ? "grad-acc font-bold text-white" : "font-semibold text-tx2 hover:bg-srf2"
                        }`}
                      >
                        <span className="flex-none text-[15px]">{c.emoji ?? FALLBACK_EMOJI}</span>
                        <span className="min-w-0 flex-1 truncate text-[13px]">{c.title_ru}</span>
                      </button>
                    );
                  })}
                {!bankCats.isPending && (fallbackMode ? fallbackRows : rows).length === 0 && (
                  <p className="px-2 py-3 text-center text-xs font-medium text-tx4">Ничего не найдено</p>
                )}
              </div>
              <button
                type="button"
                onClick={() => setAdding(true)}
                className="mt-2 flex w-full items-center gap-2 rounded-[10px] border border-dashed border-dash px-2 py-2 text-left text-[12.5px] font-semibold text-tx3 hover:bg-srf2"
              >
                <span className="text-tx4">＋</span>
                {query.trim() && (fallbackMode ? fallbackRows : rows).length === 0
                  ? `Добавить «${query.trim()}»`
                  : "Добавить свою категорию"}
              </button>
            </>
          )}
        </div>
      )}
    </div>
  );
}

// The escape hatch: the bank shows a category the base doesn't know yet.
// Creates a bank_category row (is_custom); the canonical mapping is
// optional — an unmapped row keeps the «не попадёт в „Какой картой?"»
// warning downstream. Creating a brand-new canonical inline is still
// possible (the emoji then goes to the canonical and is inherited).
function AddCustomForm({
  bankID,
  periodID,
  initialTitle,
  onCancel,
  onCreated,
}: {
  bankID: number;
  periodID?: number;
  initialTitle: string;
  onCancel: () => void;
  onCreated: (r: BankCategory) => void;
}) {
  const qc = useQueryClient();
  const categories = useCategories();
  const [title, setTitle] = useState(initialTitle.trim());
  const [emoji, setEmoji] = useState("");
  const [canonicalID, setCanonicalID] = useState("");
  const [canonicalTouched, setCanonicalTouched] = useState(false);
  const [newCat, setNewCat] = useState(false);
  const [newSlug, setNewSlug] = useState("");
  const [newTitle, setNewTitle] = useState("");

  // Debounced alias pre-suggestion (S1) — a typed title the alias table
  // already knows pre-fills the canonical select.
  useEffect(() => {
    if (periodID == null || title.trim() === "" || canonicalTouched) return;
    const t = setTimeout(async () => {
      try {
        const res = unwrap(
          await api.GET("/api/v1/cashback/alias-suggestion", {
            params: { query: { offer_period_id: periodID, raw_title: title } },
          }),
        );
        if (res.suggestion) setCanonicalID(String(res.suggestion.id));
      } catch {
        // suggestion is best-effort; entry must never block on it
      }
    }, 400);
    return () => clearTimeout(t);
  }, [title, periodID, canonicalTouched]);

  const create = useMutation({
    mutationFn: async () => {
      let catID = canonicalID ? Number(canonicalID) : undefined;
      if (newCat && newSlug && newTitle) {
        const created = unwrap(
          await api.POST("/api/v1/cashback/canonical-categories", {
            body: { slug: newSlug, title_ru: newTitle, ...(emoji.trim() ? { emoji: emoji.trim() } : {}) },
          }),
        );
        catID = created.id;
        qc.invalidateQueries({ queryKey: ["categories"] });
      }
      // Catalog rows are always kind=regular — canonical-less ones are
      // ordinary categories without a cross-bank identity, not «спец»
      // (owner 2026-07-21; special is for granted bonus mechanics).
      return unwrap(
        await api.POST("/api/v1/cashback/bank-categories", {
          body: {
            bank_id: bankID,
            title: title.trim(),
            kind: "regular",
            ...(catID != null ? { canonical_category_id: catID } : {}),
            // A new canonical already carries the emoji — the row inherits it.
            ...(emoji.trim() && !newCat ? { emoji: emoji.trim() } : {}),
          },
        }),
      );
    },
    onSuccess: (r) => {
      qc.invalidateQueries({ queryKey: ["bank-categories", bankID] });
      onCreated(r);
    },
  });

  return (
    <div className="space-y-2.5">
      <Field label="Название — как в приложении банка">
        <Input required autoFocus value={title} onChange={(e) => setTitle(e.target.value)} placeholder="Кофейни" className="!py-2" />
      </Field>
      <div className="grid grid-cols-[64px_1fr] gap-2.5">
        <Field label="Emoji">
          <Input value={emoji} onChange={(e) => setEmoji(e.target.value)} placeholder="☕" className="!py-2 text-center" />
        </Field>
        <Field label="Каноническая категория">
          <Select
            value={canonicalID}
            onChange={(e) => {
              setCanonicalID(e.target.value);
              setCanonicalTouched(true);
            }}
            disabled={newCat}
            className="!py-2"
          >
            <option value="">— без категории —</option>
            {(categories.data ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {c.title_ru}
              </option>
            ))}
          </Select>
        </Field>
      </div>
      <div className="flex flex-wrap items-center gap-3 text-[11px] font-medium text-tx2">
        <label className="flex items-center gap-2">
          <input type="checkbox" checked={newCat} onChange={(e) => setNewCat(e.target.checked)} />
          новая каноническая
        </label>
      </div>
      {newCat && (
        <div className="grid grid-cols-2 gap-2.5">
          <Field label="Slug (латиницей)">
            <Input value={newSlug} onChange={(e) => setNewSlug(e.target.value)} pattern="[a-z0-9-]+" placeholder="coffee-shops" className="!py-2" />
          </Field>
          <Field label="Название (по-русски)">
            <Input value={newTitle} onChange={(e) => setNewTitle(e.target.value)} placeholder="Кофейни" className="!py-2" />
          </Field>
        </div>
      )}
      <div className="flex gap-2">
        <Btn type="button" className="!px-3 !py-1.5 text-xs" disabled={create.isPending || !title.trim()} onClick={() => create.mutate()}>
          Добавить
        </Btn>
        <Btn type="button" variant="ghost" className="!px-3 !py-1.5 text-xs" onClick={onCancel}>
          Назад
        </Btn>
      </div>
      <ErrMsg error={create.error} />
    </div>
  );
}
