// AD-07 Точки продаж: debounced search over the 47K-row merchant base,
// server-side pagination with a windowed total; create/edit/delete without
// the location geometry (v1 has no map widget).
import { useState } from "react";
import { request, useFetch, type POS, type Page } from "../api";
import { Btn, Card, ErrMsg, Field, Pager, SearchInput, Spinner, TableWrap, Td, Th, inputCls } from "../ui";

const PAGE = 50;
const TYPES = ["offline", "online", "app", "other"] as const;

function RowForm({ initial, onDone }: { initial?: POS; onDone: () => void }) {
  const [form, setForm] = useState({
    name: initial?.name ?? "",
    merchant: initial?.merchant_title ?? "",
    code: initial?.mcc_code != null ? String(initial.mcc_code) : "",
    type: initial?.type ?? "",
    address: initial?.address ?? "",
  });
  const [err, setErr] = useState<unknown>(null);
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={async (e) => {
        e.preventDefault();
        setErr(null);
        const body = {
          name: form.name,
          merchant_title: form.merchant || undefined,
          mcc_code: form.code ? Number(form.code) : undefined,
          type: form.type || undefined,
          address: form.address || undefined,
        };
        try {
          if (initial) {
            await request("PUT", `/api/pos/${initial.id}`, body);
          } else {
            await request("POST", "/api/pos", body);
          }
          onDone();
        } catch (e2) {
          setErr(e2);
        }
      }}
    >
      <Field label="Название">
        <input
          className={`${inputCls} min-w-56`}
          required
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
        />
      </Field>
      <Field label="Мерчант">
        <input
          className={inputCls}
          value={form.merchant}
          onChange={(e) => setForm((f) => ({ ...f, merchant: e.target.value }))}
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
      <Field label="Тип">
        <select className={inputCls} value={form.type} onChange={(e) => setForm((f) => ({ ...f, type: e.target.value }))}>
          <option value="">—</option>
          {TYPES.map((t) => (
            <option key={t} value={t}>
              {t}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Адрес">
        <input
          className={`${inputCls} min-w-56`}
          value={form.address}
          onChange={(e) => setForm((f) => ({ ...f, address: e.target.value }))}
        />
      </Field>
      <Btn kind="primary" type="submit">
        {initial ? "Сохранить" : "Добавить"}
      </Btn>
      <ErrMsg error={err} />
    </form>
  );
}

export default function Pos() {
  const [query, setQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<string | null>(null);
  const [delErr, setDelErr] = useState<unknown>(null);
  const { data, error, reload } = useFetch<Page<POS>>(
    `/api/pos?query=${encodeURIComponent(query)}&limit=${PAGE}&offset=${offset}`,
  );
  return (
    <>
      <Card>
        <div className="flex items-center gap-3">
          <SearchInput
            placeholder="Название или мерчант…"
            onQuery={(q) => {
              setQuery(q);
              setOffset(0);
            }}
          />
          <Btn onClick={() => setCreating(!creating)}>{creating ? "Скрыть форму" : "Новая точка"}</Btn>
          {data && <p className="text-sm text-tx3">всего {data.total.toLocaleString("ru")}</p>}
        </div>
        {creating && (
          <div className="mt-3">
            <RowForm
              onDone={() => {
                setCreating(false);
                reload();
              }}
            />
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
                  <Th>Название</Th>
                  <Th>Мерчант</Th>
                  <Th>MCC</Th>
                  <Th>Тип</Th>
                  <Th>Адрес</Th>
                  <Th>Подтв.</Th>
                  <Th></Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((p) => (
                  <>
                    <tr key={p.id}>
                      <Td>{p.name}</Td>
                      <Td className="text-tx3">{p.merchant_title ?? ""}</Td>
                      <Td className="font-mono tabular-nums">
                        {p.mcc_code != null ? String(p.mcc_code).padStart(4, "0") : "—"}
                      </Td>
                      <Td className="text-tx3">{p.type ?? ""}</Td>
                      <Td className="max-w-72 truncate text-tx3">{p.address ?? ""}</Td>
                      <Td className="tabular-nums text-tx3">{p.confirmations ?? ""}</Td>
                      <Td>
                        <span className="flex gap-1">
                          <Btn onClick={() => setEditing(editing === p.id ? null : p.id)}>изменить</Btn>
                          <Btn
                            kind="danger"
                            onClick={async () => {
                              if (!confirm(`Удалить «${p.name}»?`)) return;
                              setDelErr(null);
                              try {
                                await request("DELETE", `/api/pos/${p.id}`);
                                reload();
                              } catch (e) {
                                setDelErr(e);
                              }
                            }}
                          >
                            удалить
                          </Btn>
                        </span>
                      </Td>
                    </tr>
                    {editing === p.id && (
                      <tr key={`e${p.id}`}>
                        <td colSpan={7} className="p-2">
                          <RowForm
                            initial={p}
                            onDone={() => {
                              setEditing(null);
                              reload();
                            }}
                          />
                        </td>
                      </tr>
                    )}
                  </>
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
