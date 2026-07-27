import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, uploadAttachment, type PartnerOffer } from "../api/client";
import { useBanks, useClients } from "../hooks";
import { Btn, Card, ErrMsg, Field, Input, Select, Spinner } from "../components/ui";
import { fmtDate, fmtPercent, todayISO } from "../lib";

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

  const [form, setForm] = useState({
    bank_id: "",
    bank_client_id: "",
    merchant_title: "",
    percent: "",
    valid_from: "",
    valid_to: "",
    cap_value: "",
    notes: "",
  });
  const [files, setFiles] = useState<File[]>([]);
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  const create = useMutation({
    mutationFn: async () => {
      const attachmentIDs: string[] = [];
      for (const f of files) attachmentIDs.push((await uploadAttachment(f)).id);
      return unwrap(
        await api.POST("/api/v1/cashback/partner-offers", {
          body: {
            bank_id: Number(form.bank_id),
            merchant_title: form.merchant_title,
            ...(form.bank_client_id ? { bank_client_id: Number(form.bank_client_id) } : {}),
            ...(form.percent ? { percent: form.percent } : {}),
            ...(form.valid_from ? { valid_from: form.valid_from } : {}),
            ...(form.valid_to ? { valid_to: form.valid_to } : {}),
            ...(form.cap_value ? { cap_value: form.cap_value } : {}),
            ...(form.notes ? { notes: form.notes } : {}),
            ...(attachmentIDs.length ? { attachment_ids: attachmentIDs } : {}),
          },
        }),
      );
    },
    onSuccess: () => {
      setForm({ bank_id: "", bank_client_id: "", merchant_title: "", percent: "", valid_from: "", valid_to: "", cap_value: "", notes: "" });
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
    <div className={`flex items-center gap-2.5 rounded-2xl border border-brd bg-srf px-3 py-2.5 ${muted ? "opacity-55" : ""}`}>
      <span
        className="flex h-[38px] w-[38px] flex-none items-center justify-center rounded-xl text-sm font-bold text-accl"
        style={{ background: "repeating-linear-gradient(120deg, var(--t-inset) 0 6px, var(--t-srf2) 6px 12px)" }}
      >
        {o.merchant_title[0]?.toUpperCase()}
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
        onClick={() => {
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
              <Field label="Лимит (если есть)">
                <Input inputMode="decimal" value={form.cap_value} onChange={set("cap_value")} />
              </Field>
              <Field label="Заметки">
                <Input value={form.notes} onChange={set("notes")} />
              </Field>
            </div>
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
    </>
  );
}
