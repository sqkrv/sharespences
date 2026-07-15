import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { api, unwrap, uploadAttachment } from "../api/client";
import { useClients, useTierMap } from "../hooks";
import { Badge, Btn, Card, ErrMsg, Field, Input, Select, Spinner } from "../components/ui";
import { monthRange, quarterRange } from "../lib";

// S1 step 1, design screen 07 header: «Новый период» — pick the bank client
// (person × bank; all its cards share the selection), the range defaults
// from the program's period_type (МКБ → quarter), screenshots are optional
// evidence.
export default function PeriodNew() {
  const clients = useClients();
  const tierMap = useTierMap();
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const qc = useQueryClient();

  // The month being viewed on the overview (?month=YYYY-MM), so «Добавить»
  // backfills THAT month, not today (owner 2026-07-15). Mid-month day-15
  // dodges any timezone edge; monthRange/quarterRange only read year+month.
  const monthParam = params.get("month");
  const baseDate = useMemo(
    () => (monthParam ? new Date(Number(monthParam.slice(0, 4)), Number(monthParam.slice(5, 7)) - 1, 15) : new Date()),
    [monthParam],
  );

  const [clientID, setClientID] = useState(params.get("client") ?? "");
  const [start, setStart] = useState(monthRange(baseDate).start);
  const [end, setEnd] = useState(monthRange(baseDate).end);
  const [files, setFiles] = useState<File[]>([]);

  const client = (clients.data ?? []).find((c) => String(c.id) === clientID);

  // Default the range from the viewed month + the client's program period type.
  useEffect(() => {
    const info = client?.program_tier_id != null ? tierMap.data?.get(client.program_tier_id) : undefined;
    const range = info?.program.period_type === "quarter" ? quarterRange(baseDate) : monthRange(baseDate);
    setStart(range.start);
    setEnd(range.end);
  }, [clientID, clients.data, tierMap.data, baseDate]); // eslint-disable-line react-hooks/exhaustive-deps

  const create = useMutation({
    mutationFn: async () => {
      const attachmentIDs: string[] = [];
      for (const f of files) {
        attachmentIDs.push((await uploadAttachment(f)).id);
      }
      return unwrap(
        await api.POST("/api/v1/cashback/offer-periods", {
          body: {
            bank_client_id: Number(clientID),
            period_start: start,
            period_end: end,
            ...(attachmentIDs.length ? { attachment_ids: attachmentIDs } : {}),
          },
        }),
      );
    },
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: ["overview"] });
      // The month picker's dots come from ["periods"] — refresh so the new
      // month is marked immediately, not after staleness kicks in.
      qc.invalidateQueries({ queryKey: ["periods"] });
      navigate(`/periods/${p.id}`);
    },
  });

  if (clients.isPending) return <Spinner />;

  return (
    <>
      <div className="flex items-center gap-2.5">
        <Link to="/" className="flex h-8 w-8 flex-none items-center justify-center rounded-[10px] border border-brd bg-srf">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-tx2">
            <path d="M14.5 5 8 12l6.5 7" />
          </svg>
        </Link>
        <h1 className="min-w-0 flex-1 truncate text-lg font-extrabold tracking-tight">Новый период</h1>
        {client && <Badge tone="indigo">{client.bank_name}</Badge>}
      </div>

      <Card className="p-4">
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            create.mutate();
          }}
        >
          <Field label="Банк · держатель">
            <Select required value={clientID} onChange={(e) => setClientID(e.target.value)}>
              <option value="">— выберите —</option>
              {(clients.data ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {c.bank_name}
                  {c.label ? ` · ${c.label}` : ""}
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
          <Btn type="submit" disabled={create.isPending || !clientID} className="w-full">
            {create.isPending ? "Создание…" : "Открыть период"}
          </Btn>
          <ErrMsg error={create.error} />
        </form>
      </Card>
    </>
  );
}
