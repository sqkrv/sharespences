// AD-04 Каталог: per-bank bank_category rows. Custom rows editable; the
// MCC-links drawer is editable only when the seed CSV doesn't cover the
// category (seed_managed from the API — the UI never guesses).
import { useState } from "react";
import { request, useFetch, type Bank, type BankCategory, type Canonical, type MCCLinks } from "../api";
import { Badge, Btn, Card, ErrMsg, Field, Pager, SeedBanner, Spinner, TableWrap, Td, Th, inputCls } from "../ui";

function LinksDrawer({ row, onDone }: { row: BankCategory; onDone: () => void }) {
  const { data, error, reload } = useFetch<MCCLinks>(`/api/bank-categories/${row.id}/mcc`);
  const [form, setForm] = useState({ code: "", note: "" });
  const [err, setErr] = useState<unknown>(null);
  if (error) return <ErrMsg error={error} />;
  if (!data) return <Spinner />;
  const write = async (fn: () => Promise<unknown>) => {
    setErr(null);
    try {
      await fn();
      reload();
      onDone();
    } catch (e) {
      setErr(e);
    }
  };
  return (
    <div className="space-y-2 rounded-xl border border-brd2 bg-srf2 p-3">
      {data.seed_managed && <SeedBanner what="Набор MCC этой категории" />}
      {data.links.length === 0 && <p className="text-sm text-tx3">Связок нет</p>}
      {data.links.map((l) => (
        <div key={l.mcc_code} className="flex items-center gap-2 text-sm">
          <span className="font-mono tabular-nums">{String(l.mcc_code).padStart(4, "0")}</span>
          <span>{l.mcc_name}</span>
          {l.note && <span className="text-xs text-tx3">{l.note}</span>}
          {!data.seed_managed && (
            <Btn
              kind="danger"
              className="ml-auto"
              onClick={() =>
                confirm(`Убрать MCC ${l.mcc_code}?`) &&
                write(() => request("DELETE", `/api/bank-categories/${row.id}/mcc/${l.mcc_code}`))
              }
            >
              убрать
            </Btn>
          )}
        </div>
      ))}
      {!data.seed_managed && (
        <form
          className="flex items-end gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            const code = Number(form.code);
            if (!code) return;
            write(() => request("PUT", `/api/bank-categories/${row.id}/mcc/${code}`, { note: form.note || undefined }));
            setForm({ code: "", note: "" });
          }}
        >
          <Field label="MCC">
            <input
              className={`${inputCls} w-24`}
              required
              pattern="\d{1,4}"
              value={form.code}
              onChange={(e) => setForm((f) => ({ ...f, code: e.target.value }))}
            />
          </Field>
          <Field label="Заметка">
            <input
              className={inputCls}
              value={form.note}
              onChange={(e) => setForm((f) => ({ ...f, note: e.target.value }))}
            />
          </Field>
          <Btn kind="primary" type="submit" className="whitespace-nowrap">
            Добавить
          </Btn>
        </form>
      )}
      <ErrMsg error={err} />
    </div>
  );
}

function EditForm({ row, canonicals, onDone }: { row: BankCategory; canonicals: Canonical[]; onDone: () => void }) {
  const [form, setForm] = useState({
    title: row.title,
    canonical: row.canonical_category_id ? String(row.canonical_category_id) : "",
    kind: row.kind,
    emoji: row.emoji ?? "",
    active: row.active,
  });
  const [err, setErr] = useState<unknown>(null);
  return (
    <form
      className="flex flex-wrap items-end gap-2 rounded-xl border border-brd2 bg-srf2 p-3"
      onSubmit={async (e) => {
        e.preventDefault();
        setErr(null);
        try {
          await request("PUT", `/api/bank-categories/${row.id}`, {
            title: form.title,
            canonical_category_id: form.canonical ? Number(form.canonical) : undefined,
            kind: form.kind,
            emoji: form.emoji || undefined,
            active: form.active,
          });
          onDone();
        } catch (e2) {
          setErr(e2);
        }
      }}
    >
      <Field label="Название">
        <input
          className={inputCls}
          required
          value={form.title}
          onChange={(e) => setForm((f) => ({ ...f, title: e.target.value }))}
        />
      </Field>
      <Field label="Каноническая">
        <select
          className={inputCls}
          value={form.canonical}
          onChange={(e) => setForm((f) => ({ ...f, canonical: e.target.value }))}
        >
          <option value="">— без канонической —</option>
          {canonicals.map((c) => (
            <option key={c.id} value={c.id}>
              {c.title_ru}
            </option>
          ))}
        </select>
      </Field>
      <Field label="Вид">
        <select className={inputCls} value={form.kind} onChange={(e) => setForm((f) => ({ ...f, kind: e.target.value }))}>
          <option value="regular">regular</option>
          <option value="super">super</option>
          <option value="special">special</option>
        </select>
      </Field>
      <Field label="Эмодзи">
        <input
          className={`${inputCls} w-16`}
          value={form.emoji}
          onChange={(e) => setForm((f) => ({ ...f, emoji: e.target.value }))}
        />
      </Field>
      <label className="flex items-center gap-1.5 pb-2 text-sm">
        <input
          type="checkbox"
          className="accent-[var(--t-acc)]"
          checked={form.active}
          onChange={(e) => setForm((f) => ({ ...f, active: e.target.checked }))}
        />
        активна
      </label>
      <Btn kind="primary" type="submit">
        Сохранить
      </Btn>
      <ErrMsg error={err} />
    </form>
  );
}

export default function Catalog() {
  const { data: banks, error: bErr } = useFetch<Bank[]>("/api/banks");
  const { data: canonicals } = useFetch<Canonical[]>("/api/canonical-categories");
  const [bankID, setBankID] = useState("");
  const { data: rows, error, reload } = useFetch<BankCategory[]>(
    `/api/bank-categories${bankID ? `?bank_id=${bankID}` : ""}`,
  );
  const [drawer, setDrawer] = useState<number | null>(null);
  const [editing, setEditing] = useState<number | null>(null);
  const [offset, setOffset] = useState(0);
  const [delErr, setDelErr] = useState<unknown>(null);
  const PAGE = 50;
  if (bErr) return <ErrMsg error={bErr} />;
  if (!banks) return <Spinner />;
  const visible = rows?.slice(offset, offset + PAGE) ?? [];
  return (
    <>
      <Card>
        <div className="flex items-center gap-3">
          <Field label="Банк">
            <select
              className={inputCls}
              value={bankID}
              onChange={(e) => {
                setBankID(e.target.value);
                setOffset(0);
              }}
            >
              <option value="">— все банки —</option>
              {banks.map((b) => (
                <option key={b.id} value={b.id}>
                  {b.name}
                </option>
              ))}
            </select>
          </Field>
          {rows && <p className="pt-4 text-sm text-tx3">{rows.length} строк</p>}
        </div>
      </Card>
      <Card>
        {error ? <ErrMsg error={error} /> : null}
        {!rows && !error && <Spinner />}
        {rows && (
          <>
            <TableWrap>
              <thead>
                <tr>
                  <Th></Th>
                  <Th>Банк</Th>
                  <Th>Название</Th>
                  <Th>Каноническая</Th>
                  <Th>Вид</Th>
                  <Th>MCC</Th>
                  <Th>Статус</Th>
                  <Th></Th>
                </tr>
              </thead>
              <tbody>
                {visible.map((r) => (
                  <>
                    <tr key={r.id}>
                      <Td>{r.emoji ?? ""}</Td>
                      <Td className="text-tx3">{r.bank_name}</Td>
                      <Td>{r.title}</Td>
                      <Td className="text-tx3">{r.canonical_title ?? "—"}</Td>
                      <Td>
                        {r.kind !== "regular" ? <Badge tone={r.kind === "super" ? "green" : "amber"}>{r.kind}</Badge> : ""}
                      </Td>
                      <Td>
                        <button type="button" className="text-accl hover:underline" onClick={() => setDrawer(drawer === r.id ? null : r.id)}>
                          {r.mcc_count}
                        </button>
                      </Td>
                      <Td>
                        {r.seed_managed ? <Badge tone="slate">в сиде</Badge> : <Badge tone="green">custom</Badge>}{" "}
                        {!r.active && <Badge tone="warn">неактивна</Badge>}
                      </Td>
                      <Td>
                        {!r.seed_managed && (
                          <span className="flex gap-1">
                            <Btn onClick={() => setEditing(editing === r.id ? null : r.id)}>изменить</Btn>
                            <Btn
                              kind="danger"
                              onClick={async () => {
                                if (!confirm(`Удалить «${r.title}»?`)) return;
                                setDelErr(null);
                                try {
                                  await request("DELETE", `/api/bank-categories/${r.id}`);
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
                    {editing === r.id && canonicals && (
                      <tr key={`e${r.id}`}>
                        <td colSpan={8} className="p-2">
                          <EditForm
                            row={r}
                            canonicals={canonicals}
                            onDone={() => {
                              setEditing(null);
                              reload();
                            }}
                          />
                        </td>
                      </tr>
                    )}
                    {drawer === r.id && (
                      <tr key={`d${r.id}`}>
                        <td colSpan={8} className="p-2">
                          <LinksDrawer row={r} onDone={reload} />
                        </td>
                      </tr>
                    )}
                  </>
                ))}
              </tbody>
            </TableWrap>
            <ErrMsg error={delErr} />
            <Pager total={rows.length} limit={PAGE} offset={offset} onOffset={setOffset} />
          </>
        )}
      </Card>
    </>
  );
}
