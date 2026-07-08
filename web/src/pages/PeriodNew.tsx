import { useEffect, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { api, unwrap, uploadAttachment } from "../api/client";
import { useCards, useTierMap } from "../hooks";
import { Badge, Btn, Card, ErrMsg, Field, Input, Select, Spinner } from "../components/ui";
import { monthRange, quarterRange } from "../lib";

// S1 step 1, design screen 07 header: «Новый период» — pick the card, the
// range defaults from the program's period_type (МКБ → quarter),
// screenshots are optional evidence.
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

  const card = (cards.data ?? []).find((c) => String(c.id) === cardID);

  // Default the range from the selected card's program period type.
  useEffect(() => {
    const info = card?.program_tier_id != null ? tierMap.data?.get(card.program_tier_id) : undefined;
    const range = info?.program.period_type === "quarter" ? quarterRange() : monthRange();
    setStart(range.start);
    setEnd(range.end);
  }, [cardID, cards.data, tierMap.data]); // eslint-disable-line react-hooks/exhaustive-deps

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
      qc.invalidateQueries({ queryKey: ["overview"] });
      navigate(`/periods/${p.id}`);
    },
  });

  if (cards.isPending) return <Spinner />;

  return (
    <>
      <div className="flex items-center gap-2.5">
        <Link to="/" className="flex h-8 w-8 flex-none items-center justify-center rounded-[10px] border border-brd bg-srf">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-tx2">
            <path d="M14.5 5 8 12l6.5 7" />
          </svg>
        </Link>
        <h1 className="min-w-0 flex-1 truncate text-lg font-extrabold tracking-tight">Новый период</h1>
        {card && <Badge tone="indigo">{card.bank_name}</Badge>}
      </div>

      <Card className="p-4">
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
          <label className="flex cursor-pointer items-center gap-2.5 rounded-xl border border-dashed border-dash px-3 py-2.5">
            <span className="h-[26px] w-[26px] flex-none rounded-md" style={{ background: "repeating-linear-gradient(120deg, var(--t-inset) 0 5px, var(--t-srf2) 5px 10px)" }} />
            <span className="min-w-0 flex-1">
              <span className="block text-[11px] font-semibold text-tx2">Скрин меню из банка</span>
              <span className="block text-[9px] font-medium text-tx4">{files.length > 0 ? `${files.length} фото` : "необязательно"}</span>
            </span>
            <input type="file" accept="image/*" multiple className="hidden" onChange={(e) => setFiles([...(e.target.files ?? [])])} />
          </label>
          <Btn type="submit" disabled={create.isPending || !cardID} className="w-full">
            {create.isPending ? "Создание…" : "Открыть период"}
          </Btn>
          <ErrMsg error={create.error} />
        </form>
      </Card>
    </>
  );
}
