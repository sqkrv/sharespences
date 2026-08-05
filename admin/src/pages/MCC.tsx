// AD-05 MCC: searchable paginated dictionary. Seed-CSV codes are read-only
// («в сиде»); codes outside the CSV are creatable/editable/deletable.
import { useState } from "react";
import { request, useFetch, type MCC, type Page } from "../api";
import { Badge, Btn, Card, ErrMsg, Field, Pager, SearchInput, Spinner, TableWrap, Td, Th, inputCls } from "../ui";

const PAGE = 50;

function RowForm({ initial, onDone }: { initial?: MCC; onDone: () => void }) {
  const [form, setForm] = useState({
    code: initial ? String(initial.code) : "",
    name: initial?.name ?? "",
    description: initial?.description ?? "",
  });
  const [err, setErr] = useState<unknown>(null);
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={async (e) => {
        e.preventDefault();
        setErr(null);
        const body = { name: form.name, description: form.description || undefined };
        try {
          if (initial) {
            await request("PUT", `/api/mcc/${initial.code}`, body);
          } else {
            await request("POST", "/api/mcc", { code: Number(form.code), ...body });
          }
          onDone();
        } catch (e2) {
          setErr(e2);
        }
      }}
    >
      <Field label="Код">
        <input
          className={`${inputCls} w-24`}
          required
          pattern="\d{1,4}"
          disabled={!!initial}
          value={form.code}
          onChange={(e) => setForm((f) => ({ ...f, code: e.target.value }))}
        />
      </Field>
      <Field label="Название">
        <input
          className={`${inputCls} min-w-64`}
          required
          value={form.name}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
        />
      </Field>
      <Field label="Описание">
        <input
          className={`${inputCls} min-w-64`}
          value={form.description}
          onChange={(e) => setForm((f) => ({ ...f, description: e.target.value }))}
        />
      </Field>
      <Btn kind="primary" type="submit">
        {initial ? "Сохранить" : "Добавить"}
      </Btn>
      <ErrMsg error={err} />
    </form>
  );
}

export default function MCCPage() {
  const [query, setQuery] = useState("");
  const [offset, setOffset] = useState(0);
  const [creating, setCreating] = useState(false);
  const [editing, setEditing] = useState<number | null>(null);
  const [delErr, setDelErr] = useState<unknown>(null);
  const { data, error, reload } = useFetch<Page<MCC>>(
    `/api/mcc?query=${encodeURIComponent(query)}&limit=${PAGE}&offset=${offset}`,
  );
  return (
    <>
      <Card>
        <div className="flex items-center gap-3">
          <SearchInput
            placeholder="Название или префикс кода…"
            onQuery={(q) => {
              setQuery(q);
              setOffset(0);
            }}
          />
          <Btn onClick={() => setCreating(!creating)}>{creating ? "Скрыть форму" : "Новый код"}</Btn>
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
                  <Th>Код</Th>
                  <Th>Название</Th>
                  <Th>Описание</Th>
                  <Th></Th>
                  <Th></Th>
                </tr>
              </thead>
              <tbody>
                {data.items.map((m) => (
                  <>
                    <tr key={m.code}>
                      <Td className="font-mono tabular-nums">{String(m.code).padStart(4, "0")}</Td>
                      <Td>{m.name}</Td>
                      <Td className="text-tx3">{m.description ?? ""}</Td>
                      <Td>{m.seed_managed && <Badge tone="slate">в сиде</Badge>}</Td>
                      <Td>
                        {!m.seed_managed && (
                          <span className="flex gap-1">
                            <Btn onClick={() => setEditing(editing === m.code ? null : m.code)}>изменить</Btn>
                            <Btn
                              kind="danger"
                              onClick={async () => {
                                if (!confirm(`Удалить MCC ${m.code}?`)) return;
                                setDelErr(null);
                                try {
                                  await request("DELETE", `/api/mcc/${m.code}`);
                                  reload();
                                } catch (e) {
                                  setDelErr(e);
                                }
                              }}
                            >
                              удалить
                            </Btn>
                          </span>
                        )}
                      </Td>
                    </tr>
                    {editing === m.code && (
                      <tr key={`e${m.code}`}>
                        <td colSpan={5} className="p-2">
                          <RowForm
                            initial={m}
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
