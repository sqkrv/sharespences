// AD-02 Банки: banks + programs + tiers, read-only (seed-managed).
import { useState } from "react";
import { useFetch, type Bank, type Program, type Tier } from "../api";
import { Badge, Card, ErrMsg, SeedBanner, Spinner, TableWrap, Td, Th } from "../ui";

function Tiers({ programID }: { programID: number }) {
  const { data, error } = useFetch<Tier[]>(`/api/programs/${programID}/tiers`);
  if (error) return <ErrMsg error={error} />;
  if (!data) return <Spinner />;
  return (
    <TableWrap>
      <thead>
        <tr>
          <Th>Тариф</Th>
          <Th>Подписка</Th>
          <Th>Лимит</Th>
          <Th>Лимит/категория</Th>
          <Th>Слоты</Th>
          <Th>Заметки</Th>
        </tr>
      </thead>
      <tbody>
        {data.map((t) => (
          <tr key={t.id}>
            <Td>{t.name}</Td>
            <Td>{t.is_paid_subscription ? <Badge tone="amber">платная</Badge> : "—"}</Td>
            <Td>{t.cap_value ?? "?"}</Td>
            <Td>{t.cap_per_category ?? "—"}</Td>
            <Td>{t.max_categories ?? "—"}</Td>
            <Td className="text-tx3">{t.notes ?? ""}</Td>
          </tr>
        ))}
      </tbody>
    </TableWrap>
  );
}

export default function Banks() {
  const { data: banks, error: bErr } = useFetch<Bank[]>("/api/banks");
  const { data: programs, error: pErr } = useFetch<Program[]>("/api/programs");
  const [open, setOpen] = useState<number | null>(null);
  if (bErr || pErr) return <ErrMsg error={bErr ?? pErr} />;
  if (!banks || !programs) return <Spinner />;
  return (
    <>
      <SeedBanner what="Банки, программы и тарифы" />
      {banks.map((b) => (
        <Card key={b.id}>
          <div className="flex items-center gap-2">
            <span
              className="inline-block h-3 w-3 rounded-full border border-brd"
              style={{ background: b.color_hex ?? "transparent" }}
            />
            <span className="font-semibold">{b.name}</span>
            <span className="text-xs text-tx4">#{b.id}</span>
          </div>
          {programs
            .filter((p) => p.bank_id === b.id)
            .map((p) => (
              <div key={p.id} className="mt-3 rounded-xl border border-brd2 bg-srf2 p-3">
                <button
                  type="button"
                  className="flex w-full items-center gap-2 text-left text-sm"
                  onClick={() => setOpen(open === p.id ? null : p.id)}
                >
                  <span className="font-medium">{p.name}</span>
                  <Badge tone="slate">{p.period_type}</Badge>
                  <Badge tone="slate">{p.selection_mode}</Badge>
                  <Badge tone={p.currency_kind === "rub" ? "green" : "slate"}>
                    {p.currency_kind === "rub" ? "₽" : (p.points_label ?? "баллы")}
                  </Badge>
                  {p.selection_opens_day && <span className="text-xs text-tx3">выбор с {p.selection_opens_day}-го</span>}
                  <span className="ml-auto text-tx3">{open === p.id ? "▴" : "▾"}</span>
                </button>
                {open === p.id && (
                  <div className="mt-2">
                    <Tiers programID={p.id} />
                  </div>
                )}
              </div>
            ))}
          {programs.every((p) => p.bank_id !== b.id) && (
            <p className="mt-2 text-sm text-tx3">Без программы (справочный банк)</p>
          )}
        </Card>
      ))}
    </>
  );
}
