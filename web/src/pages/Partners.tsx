import { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, attachmentURL, unwrap, uploadAttachment, type PartnerOffer } from "../api/client";
import { useBanks, useClients } from "../hooks";
import { Btn, Card, ErrMsg, Field, Input, Select, Spinner } from "../components/ui";
import { fmtDate, fmtPercent, merchantMonogram, todayISO } from "../lib";

const emptyForm = {
  bank_id: "",
  bank_client_id: "",
  merchant_title: "",
  percent: "",
  valid_from: "",
  valid_to: "",
  cap_value: "",
  min_amount: "",
  notes: "",
};
type OfferForm = typeof emptyForm;

// Prefill the edit form from a recorded offer. Numbers arrive as strings
// already (the API keeps decimals exact), so they go straight into inputs.
function formOf(o: PartnerOffer): OfferForm {
  return {
    bank_id: String(o.bank_id),
    bank_client_id: o.bank_client_id != null ? String(o.bank_client_id) : "",
    merchant_title: o.merchant_title,
    percent: o.percent ?? "",
    valid_from: o.valid_from ?? "",
    valid_to: o.valid_to ?? "",
    cap_value: o.cap_value ?? "",
    min_amount: o.min_amount ?? "",
    notes: o.notes ?? "",
  };
}

// Only send what was filled in: an empty string means «not recorded», which
// is a NULL rather than a zero.
function bodyOf(f: OfferForm) {
  return {
    bank_id: Number(f.bank_id),
    merchant_title: f.merchant_title,
    ...(f.bank_client_id ? { bank_client_id: Number(f.bank_client_id) } : {}),
    ...(f.percent ? { percent: f.percent } : {}),
    ...(f.valid_from ? { valid_from: f.valid_from } : {}),
    ...(f.valid_to ? { valid_to: f.valid_to } : {}),
    ...(f.cap_value ? { cap_value: f.cap_value } : {}),
    ...(f.min_amount ? { min_amount: f.min_amount } : {}),
    ...(f.notes ? { notes: f.notes } : {}),
  };
}

// Design screen 08. Partner offers are record-only (S4) — sections by
// urgency, never in helper math.
export default function Partners() {
  const banks = useBanks();
  const clients = useClients();
  const qc = useQueryClient();
  const [adding, setAdding] = useState(false);

  const offers = useQuery({
    queryKey: ["partner-offers"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/partner-offers")) ?? [],
  });

  const [form, setForm] = useState<OfferForm>(emptyForm);
  const [files, setFiles] = useState<File[]>([]);
  // Which offer is open in the detail sheet, and whether that sheet is in
  // edit mode. null = sheet closed.
  const [openID, setOpenID] = useState<number | null>(null);
  const [editing, setEditing] = useState(false);
  const set = (k: keyof OfferForm) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  const create = useMutation({
    mutationFn: async () => {
      const attachmentIDs: string[] = [];
      for (const f of files) attachmentIDs.push((await uploadAttachment(f)).id);
      return unwrap(
        await api.POST("/api/v1/cashback/partner-offers", {
          body: { ...bodyOf(form), ...(attachmentIDs.length ? { attachment_ids: attachmentIDs } : {}) },
        }),
      );
    },
    onSuccess: () => {
      setForm(emptyForm);
      setFiles([]);
      setAdding(false);
      qc.invalidateQueries({ queryKey: ["partner-offers"] });
    },
  });

  const remove = useMutation({
    mutationFn: async (id: number) =>
      unwrap(await api.DELETE("/api/v1/cashback/partner-offers/{id}", { params: { path: { id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["partner-offers"] }),
  });

  const bankClients = (clients.data ?? []).filter((c) => String(c.bank_id) === form.bank_id);

  const today = todayISO();
  const soonEdge = (() => {
    const d = new Date();
    d.setDate(d.getDate() + 14);
    return d.toISOString().slice(0, 10);
  })();
  const all = offers.data ?? [];
  const expired = all.filter((o) => o.valid_to != null && o.valid_to < today);
  const expiring = all.filter((o) => o.valid_to != null && o.valid_to >= today && o.valid_to <= soonEdge);
  const active = all.filter((o) => !expired.includes(o) && !expiring.includes(o));

  const daysLeft = (to: string) => Math.max(0, Math.round((new Date(to).getTime() - new Date(today).getTime()) / 86400000));

  const clientOf = (o: PartnerOffer) => (clients.data ?? []).find((c) => c.id === o.bank_client_id);

  const Row = ({ o, urgent, muted }: { o: PartnerOffer; urgent?: boolean; muted?: boolean }) => (
    <div
      role="button"
      tabIndex={0}
      onClick={() => {
        setOpenID(o.id);
        setEditing(false);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter" || e.key === " ") {
          e.preventDefault();
          setOpenID(o.id);
          setEditing(false);
        }
      }}
      className={`flex cursor-pointer items-center gap-2.5 rounded-2xl border border-brd bg-srf px-3 py-2.5 hover:border-dash ${muted ? "opacity-55" : ""}`}
      data-sid="CB-05.b"
    >
      <span
        className="flex h-[38px] w-[38px] flex-none items-center justify-center rounded-xl text-sm font-bold text-accl"
        style={{ background: "repeating-linear-gradient(120deg, var(--t-inset) 0 6px, var(--t-srf2) 6px 12px)" }}
      >
        {merchantMonogram(o.merchant_title)}
      </span>
      <div className="min-w-0 flex-1">
        <p className="truncate text-[13.5px] font-bold">{o.merchant_title}</p>
        <p className="mt-0.5 flex items-center gap-1.5 text-[10px] font-medium text-tx4">
          {o.valid_to && (
            <span className={urgent ? "font-semibold text-warn" : ""}>
              до {fmtDate(o.valid_to)}
              {urgent && ` · ${daysLeft(o.valid_to)} дн.`}
            </span>
          )}
          {o.valid_to && <span className="h-[3px] w-[3px] rounded-full bg-dash" />}
          <span className="truncate">
            {o.bank_name}
            {clientOf(o)?.label && ` · ${clientOf(o)!.label}`}
          </span>
        </p>
      </div>
      <div className="flex-none text-right">
        <p className="text-[16px] font-extrabold text-accl">{fmtPercent(o.percent)}</p>
        <p className="text-[8.5px] font-medium text-tx4">разовое</p>
      </div>
      <button
        type="button"
        className="px-1 text-tx4 hover:text-warn"
        title="Удалить"
        onClick={(e) => {
          e.stopPropagation(); // the row itself opens the offer
          if (window.confirm(`Удалить «${o.merchant_title}»?`)) remove.mutate(o.id);
        }}
      >
        ✕
      </button>
    </div>
  );

  return (
    <>
      <div className="flex items-start justify-between gap-2.5">
        <h1 className="text-[22px] leading-tight font-extrabold tracking-tight">
          Партнёрские
          <br />
          предложения
        </h1>
        <button
          type="button"
          onClick={() => setAdding((v) => !v)}
          className="flex h-[34px] w-[34px] flex-none items-center justify-center rounded-xl bg-acc/15 text-xl leading-none font-normal text-accl"
        >
          {adding ? "×" : "+"}
        </button>
      </div>

      {adding && (
        <Card className="p-4" data-sid="CB-05.a">
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              create.mutate();
            }}
          >
            <div className="grid grid-cols-2 gap-3">
              <Field label="Банк">
                <Select required value={form.bank_id} onChange={set("bank_id")}>
                  <option value="">— банк —</option>
                  {(banks.data ?? []).map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.name}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label="Держатель (необязательно)">
                <Select value={form.bank_client_id} onChange={set("bank_client_id")}>
                  <option value="">— любой —</option>
                  {bankClients.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.label ?? "Я"}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
            <Field label="Мерчант / предложение">
              <Input required value={form.merchant_title} onChange={set("merchant_title")} placeholder="10% в Летуаль" />
            </Field>
            <div className="grid grid-cols-3 gap-3">
              <Field label="Процент">
                <Input inputMode="decimal" value={form.percent} onChange={set("percent")} placeholder="10" />
              </Field>
              <Field label="С даты">
                <Input type="date" value={form.valid_from} onChange={set("valid_from")} />
              </Field>
              <Field label="По дату">
                <Input type="date" value={form.valid_to} onChange={set("valid_to")} />
              </Field>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Мин. сумма покупки">
                <Input inputMode="decimal" value={form.min_amount} onChange={set("min_amount")} placeholder="2000" />
              </Field>
              <Field label="Лимит (если есть)">
                <Input inputMode="decimal" value={form.cap_value} onChange={set("cap_value")} />
              </Field>
            </div>
            <Field label="Заметки">
              <Input value={form.notes} onChange={set("notes")} />
            </Field>
            <Field label="Скриншот (необязательно)">
              <Input type="file" accept="image/*" multiple onChange={(e) => setFiles([...(e.target.files ?? [])])} />
            </Field>
            <Btn type="submit" disabled={create.isPending || !form.bank_id || !form.merchant_title}>
              Записать
            </Btn>
            <ErrMsg error={create.error} />
          </form>
        </Card>
      )}

      {offers.isPending && <Spinner />}
      {offers.isError && <ErrMsg error={offers.error} />}
      {all.length === 0 && !offers.isPending && (
        <p className="rounded-xl border border-brd bg-srf px-3 py-4 text-center text-sm font-medium text-tx3">
          Пока не записано ни одного предложения.
        </p>
      )}

      {expiring.length > 0 && (
        <>
          <p className="mx-0.5 text-[10.5px] font-semibold tracking-[.06em] text-tx4 uppercase">Скоро истекают</p>
          <div className="space-y-1.5">
            {expiring.map((o) => (
              <Row key={o.id} o={o} urgent />
            ))}
          </div>
        </>
      )}

      {active.length > 0 && (
        <>
          <p className="mx-0.5 text-[10.5px] font-semibold tracking-[.06em] text-tx4 uppercase">Активные</p>
          <div className="space-y-1.5">
            {active.map((o) => (
              <Row key={o.id} o={o} />
            ))}
          </div>
        </>
      )}

      {expired.length > 0 && (
        <>
          <p className="mx-0.5 text-[10.5px] font-semibold tracking-[.06em] text-tx4 uppercase">Прошедшие</p>
          <div className="space-y-1.5">
            {expired.map((o) => (
              <Row key={o.id} o={o} muted />
            ))}
          </div>
        </>
      )}

      {openID != null && (
        <OfferSheet
          id={openID}
          editing={editing}
          onEdit={setEditing}
          onClose={() => {
            setOpenID(null);
            setEditing(false);
          }}
        />
      )}
    </>
  );
}

// OfferSheet is CB-05's detail view: the recorded fields, the screenshots
// that were attached when it was entered, and an edit form over the same
// fields. Fetched per-offer because the list response carries no
// attachments — they only matter once you open one.
function OfferSheet({
  id,
  editing,
  onEdit,
  onClose,
}: {
  id: number;
  editing: boolean;
  onEdit: (v: boolean) => void;
  onClose: () => void;
}) {
  const qc = useQueryClient();
  const banks = useBanks();
  const clients = useClients();
  const [form, setForm] = useState<OfferForm>(emptyForm);

  const offer = useQuery({
    queryKey: ["partner-offer", id],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/partner-offers/{id}", { params: { path: { id } } })),
  });

  // Refill the form whenever the sheet flips into edit mode, so «отмена»
  // followed by «изменить» starts from the saved values, not stale edits.
  useEffect(() => {
    if (editing && offer.data) setForm(formOf(offer.data));
  }, [editing, offer.data]);

  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["partner-offer", id] });
    qc.invalidateQueries({ queryKey: ["partner-offers"] });
  };

  const save = useMutation({
    mutationFn: async () =>
      unwrap(await api.PUT("/api/v1/cashback/partner-offers/{id}", { params: { path: { id } }, body: bodyOf(form) })),
    onSuccess: () => {
      invalidate();
      onEdit(false);
    },
  });

  const addShot = useMutation({
    mutationFn: async (files: File[]) => {
      for (const f of files) {
        const a = await uploadAttachment(f);
        unwrap(
          await api.POST("/api/v1/cashback/partner-offers/{id}/attachments", {
            params: { path: { id } },
            body: { attachment_id: a.id },
          }),
        );
      }
    },
    onSuccess: invalidate,
  });

  const dropShot = useMutation({
    mutationFn: async (attachmentID: string) =>
      unwrap(
        await api.DELETE("/api/v1/cashback/partner-offers/{id}/attachments/{attachment_id}", {
          params: { path: { id, attachment_id: attachmentID } },
        }),
      ),
    onSuccess: invalidate,
  });

  const o = offer.data;
  const bankClients = (clients.data ?? []).filter((c) => String(c.bank_id) === form.bank_id);
  const set = (k: keyof OfferForm) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  return (
    <div
      className="fixed inset-0 z-40 flex items-end justify-center bg-black/45 p-0 sm:items-center sm:p-4"
      onClick={onClose}
      data-sid="CB-05.c"
    >
      <Card
        className="max-h-[88vh] w-full max-w-md overflow-y-auto rounded-b-none p-4 sm:rounded-b-2xl"
        onClick={(e: React.MouseEvent) => e.stopPropagation()}
      >
        {offer.isPending && <Spinner />}
        {offer.isError && <ErrMsg error={offer.error} />}
        {o && !editing && (
          <div className="space-y-3">
            <div className="flex items-start justify-between gap-3">
              <div className="min-w-0">
                <p className="text-[17px] leading-tight font-extrabold">{o.merchant_title}</p>
                <p className="mt-1 text-[11px] font-medium text-tx4">{o.bank_name}</p>
              </div>
              <p className="flex-none text-[22px] leading-none font-extrabold text-accl">{fmtPercent(o.percent)}</p>
            </div>

            <dl className="grid grid-cols-2 gap-x-3 gap-y-2 text-[11.5px]">
              <Detail label="Действует" value={[o.valid_from && fmtDate(o.valid_from), o.valid_to && fmtDate(o.valid_to)].filter(Boolean).join(" — ")} />
              <Detail label="Мин. сумма" value={o.min_amount ? `от ${o.min_amount} ₽` : ""} />
              <Detail label="Лимит" value={o.cap_value ? `до ${o.cap_value} ₽` : ""} />
              <Detail label="Заметки" value={o.notes ?? ""} />
            </dl>

            <div>
              <p className="mb-1.5 text-[10.5px] font-semibold tracking-[.06em] text-tx4 uppercase">Скриншоты</p>
              <div className="flex gap-2 overflow-x-auto">
                {(o.attachment_ids ?? []).map((aid) => (
                  <div key={aid} className="relative flex-none">
                    <a href={attachmentURL(aid)} target="_blank" rel="noreferrer">
                      <img src={attachmentURL(aid)} alt="скриншот предложения" className="h-24 rounded-xl border border-brd object-cover" />
                    </a>
                    <button
                      type="button"
                      title="Убрать скриншот"
                      className="absolute -top-1.5 -right-1.5 flex h-5 w-5 items-center justify-center rounded-full border border-brd bg-srf text-[10px] font-bold text-tx3 hover:text-warn"
                      onClick={() => {
                        if (window.confirm("Убрать скриншот?")) dropShot.mutate(aid);
                      }}
                    >
                      ✕
                    </button>
                  </div>
                ))}
                <label className="flex h-24 w-16 flex-none cursor-pointer flex-col items-center justify-center gap-1 rounded-xl border border-dashed border-dash text-tx4">
                  <span className="text-lg leading-none">+</span>
                  <span className="text-[9px] font-semibold">скрин</span>
                  <input
                    type="file"
                    accept="image/*"
                    multiple
                    className="hidden"
                    onChange={(e) => e.target.files?.length && addShot.mutate([...e.target.files])}
                  />
                </label>
              </div>
              <ErrMsg error={addShot.error ?? dropShot.error} />
            </div>

            <div className="flex gap-2">
              <Btn onClick={() => onEdit(true)}>Изменить</Btn>
              <Btn variant="ghost" onClick={onClose}>
                Закрыть
              </Btn>
            </div>
          </div>
        )}

        {o && editing && (
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate();
            }}
          >
            <div className="grid grid-cols-2 gap-3">
              <Field label="Банк">
                <Select required value={form.bank_id} onChange={set("bank_id")}>
                  <option value="">— банк —</option>
                  {(banks.data ?? []).map((b) => (
                    <option key={b.id} value={b.id}>
                      {b.name}
                    </option>
                  ))}
                </Select>
              </Field>
              <Field label="Держатель (необязательно)">
                <Select value={form.bank_client_id} onChange={set("bank_client_id")}>
                  <option value="">— любой —</option>
                  {bankClients.map((c) => (
                    <option key={c.id} value={c.id}>
                      {c.label ?? "Я"}
                    </option>
                  ))}
                </Select>
              </Field>
            </div>
            <Field label="Мерчант / предложение">
              <Input required value={form.merchant_title} onChange={set("merchant_title")} />
            </Field>
            <div className="grid grid-cols-3 gap-3">
              <Field label="Процент">
                <Input inputMode="decimal" value={form.percent} onChange={set("percent")} />
              </Field>
              <Field label="С даты">
                <Input type="date" value={form.valid_from} onChange={set("valid_from")} />
              </Field>
              <Field label="По дату">
                <Input type="date" value={form.valid_to} onChange={set("valid_to")} />
              </Field>
            </div>
            <div className="grid grid-cols-2 gap-3">
              <Field label="Мин. сумма покупки">
                <Input inputMode="decimal" value={form.min_amount} onChange={set("min_amount")} placeholder="2000" />
              </Field>
              <Field label="Лимит (если есть)">
                <Input inputMode="decimal" value={form.cap_value} onChange={set("cap_value")} />
              </Field>
            </div>
            <Field label="Заметки">
              <Input value={form.notes} onChange={set("notes")} />
            </Field>
            <div className="flex gap-2">
              <Btn type="submit" disabled={save.isPending || !form.bank_id || !form.merchant_title}>
                Сохранить
              </Btn>
              <Btn type="button" variant="ghost" onClick={() => onEdit(false)}>
                Отмена
              </Btn>
            </div>
            <ErrMsg error={save.error} />
          </form>
        )}
      </Card>
    </div>
  );
}

function Detail({ label, value }: { label: string; value: string }) {
  return (
    <div>
      <dt className="text-[9.5px] font-semibold tracking-[.06em] text-tx4 uppercase">{label}</dt>
      <dd className={`mt-0.5 font-semibold ${value ? "" : "text-tx4"}`}>{value || "—"}</dd>
    </div>
  );
}
