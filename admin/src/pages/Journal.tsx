// AD-06 Журнал: the mcc_change news-digest precursor (ADR-0004) — view,
// hand-add observed changes, delete typos.
import { useState } from "react";
import { request, useFetch, type Bank, type MCCChange, type Page } from "../api";
import { Badge, Btn, Card, ErrMsg, Field, Pager, Spinner, TableWrap, Td, Th, inputCls } from "../ui";

const PAGE = 50;
const ACTIONS = ["added", "removed", "category_added", "category_removed"] as const;
const ACTION_LABEL: Record<string, string> = {
  imported: "импорт",
  added: "+MCC",
  removed: "−MCC",
  category_added: "+категория",
  category_removed: "−категория",
};

function AddForm({ banks, onDone }: { banks: Bank[]; onDone: () => void }) {
  const [form, setForm] = useState({ bank: "", title: "", code: "", action: "added", source: "", note: "" });
  const [err, setErr] = useState<unknown>(null);
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={async (e) => {
        e.preventDefault();
        setErr(null);
        try {
          await request("POST", "/api/mcc-changes", {
            bank_id: Number(form.bank),
            category_title: form.title,
            mcc_code: form.code ? Number(form.code) : undefined,
            action: form.action,
            source: form.source || undefined,
            note: form.note || undefined,
          });
          setForm({ bank: form.bank, title: "", code: "", action: form.action, source: form.source, note: "" });
          onDone();
        } catch (e2) {
          setErr(e2);
        }
      }}
    >
      <Field label="Банк">
        <select
          className={inputCls}
          required
          value={form.bank}
          onChange={(e) => setForm((f) => ({ ...f, bank: e.target.value }))}
        >
          <option value="">—</option>
          {banks.map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Категория">
        <input
          className={inputCls}
          required
          value={form.title}
          onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
        />
      </Field>
      <Field label="MCC">
        <input
          className={`${inputCls} w-24`}
          pattern="\d{1,4}"
          value={form.code}
          onChange={(e) => setForm((f) => ({ ...f, code: e.target.value }))}
        />
      </Field>
      <Field label="Событие">
        <select
          className={inputCls}
          value={form.action}
          onChange={(e) => setForm((f) => ({ ...f, action: e.target.value }))}
        >
          {ACTIONS.map((a) => (
            <option key={a} value={a}>
              {ACTION_LABEL[a]}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Источник">
        <input
          className={inputCls}
          placeholder="manual (admin)"
          value={form.source}
          onChange={(e) => setForm((f) => ({ ...f, source: e.target.value }))}
        />
      </Field>
      <Field label="Заметка">
        <input className={inputCls} value={form.note} onChange={(e) => setForm((f) => ({ ...f, note: e.target.value }))} />
      </Field>
      <Btn kind="primary" type="submit">
        Записать
      </Btn>
      <ErrMsg error={err} />
    </form>
  );
}

export default function Journal() {
  const { data: banks } = useFetch<Bank[]>("/api/banks");
  const [offset, setOffset] = useState(0);
  const [adding, setAdding] = useState(false);
  const [delErr, setDelErr] = useState<unknown>(null);
  const { data, error, reload } = useFetch<Page<MCCChange>>(`/api/mcc-changes?limit=${PAGE}&offset=${offset}`);
  return (
    <>
      <Card>
        <div className="flex items-center gap-3">
          <Btn onClick={() => setAdding(!adding)}>{adding ? "Скрыть форму" : "Записать изменение"}</Btn>
          {data && <p className="text-sm text-tx3">всего {data.total.toLocaleString("ru")}</p>}
        </div>
        {adding && banks && (
          <div className="mt-3">
            <AddForm banks={banks} onDone={reload} />
          </div>
        )}
      </Card>
      <Card>
        {error ? <ErrMsg error={error} /> : null}
        {!data && !error && <Spinner />}
        {data && (
          <>
            <TableWrap>
              <thead>
                <tr>
                  <Th>Когда</Th>
                  <Th>Банк</Th>
                  <Th>Категория</Th>
                  <Th>MCC</Th>
                  <Th>Событие</Th>
                  <Th>Источник</Th>
                  <Th></Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((c) => (
                  <tr key={c.id}>
                    <Td className="whitespace-nowrap text-tx3">{c.noted_at}</Td>
                    <Td>{c.bank_name}</Td>
                    <Td>
                      {c.category_title}
                      {c.note && <span className="block text-xs text-tx4">{c.note}</span>}
                    </Td>
                    <Td className="font-mono tabular-nums">{c.mcc_code != null ? String(c.mcc_code).padStart(4, "0") : "—"}</Td>
                    <Td>
                      <Badge tone={c.action === "removed" || c.action === "category_removed" ? "warn" : "green"}>
                        {ACTION_LABEL[c.action] ?? c.action}
                      </Badge>
                    </Td>
                    <Td className="max-w-64 truncate text-xs text-tx4" >{c.source}</Td>
                    <Td>
                      <Btn
                        kind="danger"
                        onClick={async () => {
                          if (!confirm("Удалить запись журнала?")) return;
                          setDelErr(null);
                          try {
                            await request("DELETE", `/api/mcc-changes/${c.id}`);
                            reload();
                          } catch (e) {
                            setDelErr(e);
                          }
                        }}
                      >
                        удалить
                      </Btn>
                    </Td>
                  </tr>
                ))}
              </tbody>
            </TableWrap>
            <ErrMsg error={delErr} />
            <Pager total={data.total} limit={PAGE} offset={offset} onOffset={setOffset} />
          </>
        )}
      </Card>
    </>
  );
}
