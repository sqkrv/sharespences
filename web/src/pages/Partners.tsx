import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap, uploadAttachment } from "../api/client";
import { useBanks, useCards } from "../hooks";
import { Btn, Empty, ErrMsg, Field, Input, Section, Select, Spinner } from "../components/ui";
import { fmtDate, fmtPercent } from "../lib";

// S4: partner offers are record-only — a form and a list, never helper math.
export default function Partners() {
  const banks = useBanks();
  const cards = useCards();
  const qc = useQueryClient();

  const offers = useQuery({
    queryKey: ["partner-offers"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/partner-offers")) ?? [],
  });

  const [form, setForm] = useState({
    bank_id: "",
    card_id: "",
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
            ...(form.card_id ? { card_id: Number(form.card_id) } : {}),
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
      setForm({ bank_id: "", card_id: "", merchant_title: "", percent: "", valid_from: "", valid_to: "", cap_value: "", notes: "" });
      setFiles([]);
      qc.invalidateQueries({ queryKey: ["partner-offers"] });
    },
  });

  const remove = useMutation({
    mutationFn: async (id: number) =>
      unwrap(await api.DELETE("/api/v1/cashback/partner-offers/{id}", { params: { path: { id } } })),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["partner-offers"] }),
  });

  const bankCards = (cards.data ?? []).filter((c) => String(c.bank_id) === form.bank_id);

  return (
    <>
      <Section title="Партнёрские предложения">
        {offers.isPending && <Spinner />}
        {offers.isError && <ErrMsg error={offers.error} />}
        {offers.data && offers.data.length === 0 && <Empty>Пока не записано ни одного предложения.</Empty>}
        <ul className="space-y-2">
          {(offers.data ?? []).map((o) => (
            <li key={o.id} className="flex items-center justify-between gap-3 rounded-lg border border-slate-200 p-3">
              <div>
                <p className="font-medium">
                  {o.merchant_title} <span className="text-indigo-700">{fmtPercent(o.percent)}</span>
                </p>
                <p className="text-xs text-slate-500">
                  {o.bank_name}
                  {o.valid_to && ` · до ${fmtDate(o.valid_to)}`}
                  {o.notes && ` · ${o.notes}`}
                </p>
              </div>
              <Btn variant="danger" onClick={() => remove.mutate(o.id)} disabled={remove.isPending}>
                Удалить
              </Btn>
            </li>
          ))}
        </ul>
      </Section>

      <Section title="Записать предложение">
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
            <Field label="Карта (необязательно)">
              <Select value={form.card_id} onChange={set("card_id")}>
                <option value="">— любая —</option>
                {bankCards.map((c) => (
                  <option key={c.id} value={c.id}>
                    ··{String(c.last_4_digits).padStart(4, "0")}
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
      </Section>
    </>
  );
}
