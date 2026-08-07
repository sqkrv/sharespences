import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api, unwrap, uploadAttachment } from "../api/client";
import { useBanks, useCategories, useClients, usePrograms } from "../hooks";
import { BackButton, Btn, Card, ErrMsg, Field, Input, Select, Spinner } from "../components/ui";
import { FALLBACK_EMOJI } from "../lib";

// CB-12 «Партнёрское предложение» (redesign 2f) — a screen, not a form under
// a list. Corrected against the mock's gaps: min_amount, notes and
// screenshots stayed (the data model carries them); держатель is optional
// (bank-level offers are legal — the deal usually pays on any card of the
// bank); the limit's unit follows the offer's own currency; the activation
// pair distinguishes «не требует» / «требует, не активировано» /
// «активировано».

type Scope = "merchant" | "category";

export default function PartnerNew() {
  const [params] = useSearchParams();
  const editID = params.get("id");
  const banks = useBanks();
  const clients = useClients();
  const programs = usePrograms();
  const categories = useCategories();
  const qc = useQueryClient();
  const navigate = useNavigate();

  const existing = useQuery({
    queryKey: ["partner-offer", editID],
    enabled: editID != null,
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/cashback/partner-offers/{id}", { params: { path: { id: Number(editID) } } })),
  });

  // «Банк · держатель»: a client, or the whole bank (bank-level offer).
  const [subject, setSubject] = useState(params.get("client") ? `c${params.get("client")}` : "");
  const [title, setTitle] = useState("");
  const [scope, setScope] = useState<Scope>("merchant");
  const [canonicalID, setCanonicalID] = useState("");
  const [merchantKind, setMerchantKind] = useState("");
  const [percent, setPercent] = useState("");
  const [currency, setCurrency] = useState<"" | "rub" | "points">("");
  const [validFrom, setValidFrom] = useState("");
  const [validTo, setValidTo] = useState("");
  const [cap, setCap] = useState("");
  const [minAmount, setMinAmount] = useState("");
  const [requiresActivation, setRequiresActivation] = useState(false);
  const [activated, setActivated] = useState(false);
  const [notes, setNotes] = useState("");
  const [files, setFiles] = useState<File[]>([]);
  const [loaded, setLoaded] = useState(false);

  useEffect(() => {
    const o = existing.data;
    if (!o || loaded) return;
    setSubject(o.bank_client_id != null ? `c${o.bank_client_id}` : `b${o.bank_id}`);
    setTitle(o.merchant_title);
    setScope((o.scope_kind as Scope) ?? "merchant");
    setCanonicalID(o.canonical_category_id != null ? String(o.canonical_category_id) : "");
    setMerchantKind(o.merchant_kind ?? "");
    setPercent(o.percent ?? "");
    setCurrency((o.currency_kind as "rub" | "points" | undefined) ?? "");
    setValidFrom(o.valid_from ?? "");
    setValidTo(o.valid_to ?? "");
    setCap(o.cap_value ?? "");
    setMinAmount(o.min_amount ?? "");
    setRequiresActivation(o.requires_activation);
    setActivated(o.activated_at != null);
    setNotes(o.notes ?? "");
    setLoaded(true);
  }, [existing.data, loaded]);

  const bankID = subject.startsWith("b")
    ? Number(subject.slice(1))
    : (clients.data ?? []).find((c) => String(c.id) === subject.slice(1))?.bank_id;
  const clientID = subject.startsWith("c") ? Number(subject.slice(1)) : null;
  const program = bankID != null ? (programs.data ?? []).find((p) => p.bank_id === bankID) : undefined;

  // The currency toggle defaults from the chosen bank's program — the server
  // never guesses; an offer at a bank without a program stays unknown until
  // the user says what it pays in.
  useEffect(() => {
    if (currency === "" && program) setCurrency(program.currency_kind as "rub" | "points");
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [program?.id]);

  const pointsLabel = program?.points_label || "баллы";
  const unit = currency === "points" ? pointsLabel : "₽";

  const save = useMutation({
    mutationFn: async () => {
      const body = {
        bank_id: bankID!,
        ...(clientID != null ? { bank_client_id: clientID } : {}),
        merchant_title: title.trim(),
        scope_kind: scope,
        ...(canonicalID ? { canonical_category_id: Number(canonicalID) } : {}),
        ...(merchantKind ? { merchant_kind: merchantKind as "offline" | "online" | "app" | "other" } : {}),
        ...(percent.trim() ? { percent: percent.trim() } : {}),
        ...(currency ? { currency_kind: currency } : {}),
        ...(validFrom ? { valid_from: validFrom } : {}),
        ...(validTo ? { valid_to: validTo } : {}),
        ...(cap.trim() ? { cap_value: cap.trim() } : {}),
        ...(minAmount.trim() ? { min_amount: minAmount.trim() } : {}),
        requires_activation: requiresActivation,
        activated: requiresActivation && activated,
        ...(notes.trim() ? { notes: notes.trim() } : {}),
      };
      if (editID != null) {
        const updated = unwrap(
          await api.PUT("/api/v1/cashback/partner-offers/{id}", { params: { path: { id: Number(editID) } }, body }),
        );
        for (const f of files) {
          const a = await uploadAttachment(f);
          unwrap(
            await api.POST("/api/v1/cashback/partner-offers/{id}/attachments", {
              params: { path: { id: updated.id } },
              body: { attachment_id: a.id },
            }),
          );
        }
        return updated;
      }
      const ids: string[] = [];
      for (const f of files) ids.push((await uploadAttachment(f)).id);
      return unwrap(
        await api.POST("/api/v1/cashback/partner-offers", {
          body: { ...body, ...(ids.length ? { attachment_ids: ids } : {}) },
        }),
      );
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      qc.invalidateQueries({ queryKey: ["partner-offers"] });
      qc.invalidateQueries({ queryKey: ["partner-offer"] });
      qc.invalidateQueries({ queryKey: ["lookup"] });
      navigate("/banks");
    },
  });

  if (editID != null && existing.isPending) return <Spinner />;

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/banks" />
        <h1 className="min-w-0 flex-1 text-xl font-extrabold tracking-tight">Партнёрское предложение</h1>
      </div>

      <Card className="p-4" data-sid="CB-12.a">
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (bankID != null) save.mutate();
          }}
        >
          <Field label="Банк · держатель">
            <Select required value={subject} onChange={(e) => setSubject(e.target.value)}>
              <option value="">— выберите —</option>
              {(banks.data ?? [])
                .filter((b) => (clients.data ?? []).some((c) => c.bank_id === b.id))
                .map((b) => (
                  <option key={`b${b.id}`} value={`b${b.id}`}>
                    {b.name} — весь банк
                  </option>
                ))}
              {(clients.data ?? []).map((c) => (
                <option key={`c${c.id}`} value={`c${c.id}`}>
                  {c.bank_name} · {c.label ?? "Я"}
                </option>
              ))}
            </Select>
          </Field>

          <Field label="Название">
            <Input required value={title} onChange={(e) => setTitle(e.target.value)} placeholder="10% баллами в Лавке" />
          </Field>

          <Field label="Где действует">
            <div className="flex gap-2">
              <span className="flex flex-none overflow-hidden rounded-xl border border-brd2">
                {(
                  [
                    ["merchant", "магазин"],
                    ["category", "категория"],
                  ] as const
                ).map(([sVal, label]) => (
                  <button
                    key={sVal}
                    type="button"
                    onClick={() => setScope(sVal)}
                    className={`px-2.5 py-2 text-[11px] font-semibold ${scope === sVal ? "grad-acc text-white" : "bg-srf2 text-tx3"}`}
                  >
                    {label}
                  </button>
                ))}
              </span>
              <Select
                required={scope === "category"}
                value={canonicalID}
                onChange={(e) => setCanonicalID(e.target.value)}
                className="min-w-0 flex-1"
              >
                <option value="">{scope === "category" ? "— категория —" : "канон-подсказка (необязательно)"}</option>
                {(categories.data ?? []).map((c) => (
                  <option key={c.id} value={c.id}>
                    {c.emoji || FALLBACK_EMOJI} {c.title_ru}
                  </option>
                ))}
              </Select>
            </div>
            <span className="mt-1 block text-[10px] font-medium text-tx4">
              {scope === "merchant"
                ? "матчится по названию точки; канон добавит акцию в подбор по категории — с пометкой «только в …»"
                : "действует на всю категорию — ранжируется в ней без оговорок"}
            </span>
          </Field>

          {scope === "merchant" && (
            <Field label="Тип точки (необязательно)">
              <Select value={merchantKind} onChange={(e) => setMerchantKind(e.target.value)}>
                <option value="">— не указан —</option>
                <option value="offline">офлайн-точка</option>
                <option value="online">онлайн</option>
                <option value="app">приложение</option>
                <option value="other">другое</option>
              </Select>
            </Field>
          )}

          <div className="grid grid-cols-2 gap-3">
            <Field label="Ставка">
              <Input inputMode="decimal" value={percent} onChange={(e) => setPercent(e.target.value)} placeholder="10" />
            </Field>
            <Field label="Начисляется">
              <span className="flex overflow-hidden rounded-xl border border-brd2">
                {(
                  [
                    ["rub", "рубли"],
                    ["points", pointsLabel],
                  ] as const
                ).map(([cVal, label]) => (
                  <button
                    key={cVal}
                    type="button"
                    onClick={() => setCurrency(cVal)}
                    className={`flex-1 truncate px-1 py-2.5 text-[11px] font-semibold ${currency === cVal ? "grad-acc text-white" : "bg-srf2 text-tx3"}`}
                  >
                    {label}
                  </button>
                ))}
              </span>
            </Field>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <Field label="Действует с (необязательно)">
              <Input type="date" value={validFrom} onChange={(e) => setValidFrom(e.target.value)} />
            </Field>
            <Field label="Действует по">
              <Input type="date" value={validTo} onChange={(e) => setValidTo(e.target.value)} />
            </Field>
          </div>

          <div className="grid grid-cols-2 gap-3">
            <Field label={`Лимит (${unit})`}>
              <Input inputMode="decimal" value={cap} onChange={(e) => setCap(e.target.value)} placeholder="2000" />
            </Field>
            <Field label="Мин. сумма покупки (₽)">
              <Input inputMode="decimal" value={minAmount} onChange={(e) => setMinAmount(e.target.value)} placeholder="от 1500" />
            </Field>
          </div>

          <div className="space-y-2">
            <label className="flex items-center gap-2.5 text-[12.5px] font-semibold text-tx2">
              <input type="checkbox" checked={requiresActivation} onChange={(e) => setRequiresActivation(e.target.checked)} />
              Требует активации в банке
            </label>
            {requiresActivation && (
              <label className="ml-6 flex items-center gap-2.5 text-[12.5px] font-semibold text-tx2">
                <input type="checkbox" checked={activated} onChange={(e) => setActivated(e.target.checked)} />
                Уже активировано
              </label>
            )}
            {requiresActivation && !activated && (
              <p className="ml-6 text-[10px] font-medium text-warn">Будет ранжироваться с пометкой «требует активации».</p>
            )}
          </div>

          <Field label="Заметки (необязательно)">
            <Input value={notes} onChange={(e) => setNotes(e.target.value)} placeholder="условия, откуда узнал" />
          </Field>

          <label className="flex cursor-pointer items-center gap-2.5 rounded-xl border border-dashed border-dash px-3 py-2.5">
            <span className="h-[26px] w-[26px] flex-none rounded-md" style={{ background: "repeating-linear-gradient(120deg, var(--t-inset) 0 5px, var(--t-srf2) 5px 10px)" }} />
            <span className="min-w-0 flex-1">
              <span className="block text-[11px] font-semibold text-tx2">Скрин условий из банка</span>
              <span className="block text-[9px] font-medium text-tx4">{files.length > 0 ? `${files.length} фото` : "необязательно"}</span>
            </span>
            <input type="file" accept="image/png,image/jpeg,image/webp,image/heic,application/pdf" multiple className="hidden" onChange={(e) => setFiles([...(e.target.files ?? [])])} />
          </label>

          <Btn type="submit" className="w-full" disabled={save.isPending || bankID == null}>
            {save.isPending ? "Сохранение…" : "Сохранить предложение"}
          </Btn>
          <ErrMsg error={save.error} />
        </form>
      </Card>

      <p className="mx-0.5 text-[11px] leading-snug font-medium text-tx4">
        Разовые акции не занимают слоты и не трогают лимиты месяца — но ранжируются в ленте и в точке продаж наравне с категориями.
      </p>
    </>
  );
}
