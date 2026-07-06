import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { api, unwrap, uploadAttachment } from "../api/client";
import { useCards, useTierMap } from "../hooks";
import { Btn, ErrMsg, Field, Input, Section, Select, Spinner } from "../components/ui";
import { monthRange, quarterRange } from "../lib";

// S1 step 1: «Новый период» — pick the card, the range defaults from the
// program's period_type (МКБ → quarter), screenshots are optional evidence.
export default function PeriodNew() {
  const cards = useCards();
  const tierMap = useTierMap();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const qc = useQueryClient();

  const [cardID, setCardID] = useState(params.get("card") ?? "");
  const [start, setStart] = useState(monthRange().start);
  const [end, setEnd] = useState(monthRange().end);
  const [files, setFiles] = useState<File[]>([]);

  // Default the range from the selected card's program period type.
  useEffect(() => {
    const card = (cards.data ?? []).find((c) => String(c.id) === cardID);
    const info = card?.program_tier_id != null ? tierMap.data?.get(card.program_tier_id) : undefined;
    const range = info?.program.period_type === "quarter" ? quarterRange() : monthRange();
    setStart(range.start);
    setEnd(range.end);
  }, [cardID, cards.data, tierMap.data]);

  const create = useMutation({
    mutationFn: async () => {
      const attachmentIDs: string[] = [];
      for (const f of files) {
        attachmentIDs.push((await uploadAttachment(f)).id);
      }
      return unwrap(
        await api.POST("/api/v1/cashback/offer-periods", {
          body: {
            card_id: Number(cardID),
            period_start: start,
            period_end: end,
            ...(attachmentIDs.length ? { attachment_ids: attachmentIDs } : {}),
          },
        }),
      );
    },
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: ["periods"] });
      navigate(`/periods/${p.id}`);
    },
  });

  if (cards.isPending) return <Spinner />;

  return (
    <Section title="Новый период выбора КБ">
      <form
        className="space-y-3"
        onSubmit={(e) => {
          e.preventDefault();
          create.mutate();
        }}
      >
        <Field label="Карта">
          <Select required value={cardID} onChange={(e) => setCardID(e.target.value)}>
            <option value="">— выберите карту —</option>
            {(cards.data ?? []).map((c) => (
              <option key={c.id} value={c.id}>
                {c.bank_name} ··{String(c.last_4_digits).padStart(4, "0")}
              </option>
            ))}
          </Select>
        </Field>
        <div className="grid grid-cols-2 gap-3">
          <Field label="Начало">
            <Input type="date" required value={start} onChange={(e) => setStart(e.target.value)} />
          </Field>
          <Field label="Конец">
            <Input type="date" required value={end} onChange={(e) => setEnd(e.target.value)} />
          </Field>
        </div>
        <Field label="Скриншоты меню из банка (необязательно)">
          <Input
            type="file"
            accept="image/*"
            multiple
            onChange={(e) => setFiles([...(e.target.files ?? [])])}
          />
        </Field>
        {files.length > 0 && <p className="text-xs text-slate-500 dark:text-slate-400">{files.length} файл(ов) будет загружено</p>}
        <Btn type="submit" disabled={create.isPending || !cardID}>
          {create.isPending ? "Создание…" : "Открыть период"}
        </Btn>
        <ErrMsg error={create.error} />
      </form>
    </Section>
  );
}
