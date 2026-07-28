import { useEffect, useMemo, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { ApiError, api, unwrap, uploadAttachment, type RecognitionDraft } from "../api/client";
import { useClients, useTierMap } from "../hooks";
import { Badge, Btn, Card, ErrMsg, Field, Input, Select, Spinner } from "../components/ui";
import { CategoryPicker, type PickedCategory } from "../components/CategoryPicker";
import { monthRange, quarterRange } from "../lib";

// S1 step 1, design screen 07 header: «Новый период» — pick the bank client
// (person × bank; all its cards share the selection), the range defaults
// from the program's period_type (МКБ → quarter), screenshots are optional
// evidence.
//
// Recognize mode (spec cashback-recognizer.md, CB-02): the same screen is
// also the recognizer flow — form → recognizing → review. The job id lives
// in ?job=…; everything the user edits on review persists to
// sessionStorage keyed by that id, so a refresh loses nothing and a
// refresh mid-commit resumes instead of stranding a half-filled period.

type ReviewRow = {
  key: number;
  title: string;
  percent: string;
  cap: string;
  kind: "regular" | "super" | "special";
  picked: boolean;
  bankCategoryID: number | null;
  canonicalID: number | null;
  mappedTitle: string | null;
  needsReview: boolean;
  notes: string[];
  conflictPercents: string[];
  conflictCaps: string[];
};

type JobMeta = {
  notes: string[];
  periodTexts: string[];
  slotCandidates: { value: number; source_image: number }[];
  images: { screenType: string; skipped: boolean; note: string }[];
};

type JobState = {
  clientID: string;
  start: string;
  end: string;
  attachmentIDs: string[];
  rows?: ReviewRow[];
  slots?: number | null;
  meta?: JobMeta;
  createdID?: number;
  slotsDone?: boolean;
  offersDone?: Record<number, number>; // row key → created offer id
  selectedDone?: number[]; // row keys whose selection is written
};

const jobKey = (id: string) => `recognize-${id}`;
function loadJob(id: string): JobState | null {
  try {
    const raw = sessionStorage.getItem(jobKey(id));
    return raw ? (JSON.parse(raw) as JobState) : null;
  } catch {
    return null;
  }
}
const saveJob = (id: string, s: JobState) => sessionStorage.setItem(jobKey(id), JSON.stringify(s));

function draftRows(d: RecognitionDraft): ReviewRow[] {
  return (d.rows ?? []).map((r, i) => ({
    key: i,
    title: r.raw_title,
    percent: r.percent ?? "",
    cap: r.cap_value ?? "",
    kind: (r.kind as ReviewRow["kind"]) || "regular",
    picked: r.checked,
    bankCategoryID: r.bank_category_id ?? null,
    canonicalID: r.canonical_category_id ?? null,
    mappedTitle: r.catalog_title ?? null,
    needsReview: r.needs_review,
    notes: r.review_notes ?? [],
    conflictPercents: r.conflict_percents ?? [],
    conflictCaps: r.conflict_caps ?? [],
  }));
}

export default function PeriodNew() {
  const [params] = useSearchParams();
  const jobID = params.get("job");
  if (jobID) return <RecognizeFlow jobID={jobID} />;
  return <PeriodForm />;
}

function PeriodForm() {
  const clients = useClients();
  const tierMap = useTierMap();
  const [params, setParams] = useSearchParams();
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

  // Upload the same screenshots, but hand them to the recognizer first —
  // the period itself is created later, on review commit.
  const recognize = useMutation({
    mutationFn: async () => {
      const attachmentIDs: string[] = [];
      for (const f of files) {
        attachmentIDs.push((await uploadAttachment(f)).id);
      }
      const job = unwrap(
        await api.POST("/api/v1/cashback/recognitions", {
          body: { bank_client_id: Number(clientID), attachment_ids: attachmentIDs },
        }),
      );
      return { job, attachmentIDs };
    },
    onSuccess: ({ job, attachmentIDs }) => {
      saveJob(job.id, { clientID, start, end, attachmentIDs });
      const next = new URLSearchParams(params);
      next.set("job", job.id);
      setParams(next);
    },
  });

  // Vision absent is honest degradation, not a fault — manual entry is
  // always the fallback path.
  const recognizeError =
    recognize.error instanceof ApiError && recognize.error.status === 503
      ? new Error("Распознавание сейчас недоступно — создай период вручную, скриншоты всё равно приложатся")
      : recognize.error;

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

      <Card className="p-4" data-sid="CB-02.a">
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
            {/* The recognizer decodes PNG/JPEG/WebP; HEIC and PDF would only
                skip with a note, so the picker doesn't offer them. */}
            <input type="file" accept="image/png,image/jpeg,image/webp" multiple className="hidden" onChange={(e) => setFiles([...(e.target.files ?? [])])} />
          </label>
          <Btn type="submit" disabled={create.isPending || recognize.isPending || !clientID} className="w-full">
            {create.isPending ? "Создание…" : "Открыть период"}
          </Btn>
          {files.length > 0 && (
            <Btn
              type="button"
              variant="soft"
              disabled={create.isPending || recognize.isPending || !clientID}
              className="w-full"
              onClick={() => recognize.mutate()}
            >
              {recognize.isPending ? "Загрузка скриншотов…" : "Распознать со скриншотов"}
            </Btn>
          )}
          <ErrMsg error={create.error} />
          <ErrMsg error={recognizeError} />
        </form>
      </Card>
    </>
  );
}

function RecognizeFlow({ jobID }: { jobID: string }) {
  const [params, setParams] = useSearchParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const clients = useClients();
  const [state, setState] = useState<JobState | null>(() => loadJob(jobID));
  const persist = (s: JobState) => {
    saveJob(jobID, s);
    setState(s);
  };

  // Poll only until the draft is captured into sessionStorage — after
  // that the review survives the job's server-side TTL and restarts.
  const poll = useQuery({
    queryKey: ["recognition", jobID],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/recognitions/{id}", { params: { path: { id: jobID } } })),
    enabled: state != null && state.rows == null,
    refetchInterval: (q) => (q.state.data?.status === "running" ? 2000 : false),
    staleTime: 0,
    retry: false,
  });

  // Seed the editable review from the finished draft, exactly once.
  useEffect(() => {
    if (!state || state.rows != null || poll.data?.status !== "done" || !poll.data.draft) return;
    const d = poll.data.draft;
    persist({
      ...state,
      rows: draftRows(d),
      slots: d.slot_count ?? null,
      meta: {
        notes: d.notes ?? [],
        periodTexts: d.period_texts ?? [],
        slotCandidates: (d.slot_candidates ?? []).map((c) => ({ value: c.value, source_image: c.source_image })),
        images: (d.images ?? []).map((im) => ({ screenType: im.screen_type ?? "", skipped: im.skipped ?? false, note: im.note ?? "" })),
      },
    });
  }, [poll.data, state]); // eslint-disable-line react-hooks/exhaustive-deps

  const leave = () => {
    sessionStorage.removeItem(jobKey(jobID));
    const next = new URLSearchParams(params);
    next.delete("job");
    setParams(next, { replace: true });
  };

  const header = (
    <div className="flex items-center gap-2.5">
      <Link to="/" className="flex h-8 w-8 flex-none items-center justify-center rounded-[10px] border border-brd bg-srf">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-tx2">
          <path d="M14.5 5 8 12l6.5 7" />
        </svg>
      </Link>
      <h1 className="min-w-0 flex-1 truncate text-lg font-extrabold tracking-tight">Распознавание</h1>
    </div>
  );

  if (!state) {
    return (
      <>
        {header}
        <Card className="p-4" data-sid="CB-02.b">
          <p className="text-sm font-semibold">Черновик не найден</p>
          <p className="mt-1 text-[12px] font-medium text-tx3">Задание истекло или страница открыта в новой сессии. Начни заново — скриншоты придётся выбрать ещё раз.</p>
          <Btn variant="soft" className="mt-3 w-full" onClick={leave}>
            К форме периода
          </Btn>
        </Card>
      </>
    );
  }

  if (state.rows == null) {
    const failed = poll.data?.status === "failed";
    const notFound = poll.error instanceof ApiError && poll.error.status === 404;
    return (
      <>
        {header}
        <Card className="p-4" data-sid="CB-02.b">
          {failed || notFound || poll.error ? (
            <>
              <p className="text-sm font-semibold">Распознать не получилось</p>
              <p className="mt-1 text-[12px] font-medium text-tx3">
                {notFound
                  ? "Задание не найдено — истёк срок хранения или сервер перезапускался."
                  : (poll.data?.error ?? (poll.error instanceof ApiError ? poll.error.message : String(poll.error ?? "")))}
              </p>
              <p className="mt-1 text-[12px] font-medium text-tx3">Период всегда можно заполнить вручную — скриншоты уже загружены.</p>
              <Btn variant="soft" className="mt-3 w-full" onClick={leave}>
                Заполнить вручную
              </Btn>
            </>
          ) : (
            <div className="flex items-center gap-3">
              <Spinner />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold">
                  Распознаём скриншоты{poll.data ? ` · ${poll.data.done} из ${poll.data.total}` : "…"}
                </p>
                <p className="mt-0.5 text-[12px] font-medium text-tx3">Локальная модель читает меню ≈2–3 минуты на скриншот. Можно уйти с экрана — задание идёт на сервере.</p>
              </div>
            </div>
          )}
        </Card>
      </>
    );
  }

  return (
    <RecognizeReview
      jobID={jobID}
      state={state}
      persist={persist}
      leave={leave}
      client={(clients.data ?? []).find((c) => String(c.id) === state.clientID)}
      onCommitted={(periodID) => {
        sessionStorage.removeItem(jobKey(jobID));
        qc.invalidateQueries({ queryKey: ["overview"] });
        qc.invalidateQueries({ queryKey: ["periods"] });
        navigate(`/periods/${periodID}`);
      }}
    />
  );
}

function RecognizeReview({
  jobID,
  state,
  persist,
  leave,
  client,
  onCommitted,
}: {
  jobID: string;
  state: JobState;
  persist: (s: JobState) => void;
  leave: () => void;
  client?: { bank_id: number; bank_name?: string; label?: string | null };
  onCommitted: (periodID: number) => void;
}) {
  const rows = state.rows ?? [];
  const meta = state.meta;
  const setRows = (next: ReviewRow[]) => persist({ ...state, rows: next });
  const patchRow = (key: number, patch: Partial<ReviewRow>) => setRows(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));

  // A selection dated today would fall outside a backfilled period —
  // mirror the Period screen's «задним числом» switch automatically.
  const backfill = useMemo(() => {
    const today = new Date().toISOString().slice(0, 10);
    return !(state.start <= today && today <= state.end);
  }, [state.start, state.end]);

  // Commit replays the four existing endpoints (the recognizer has no
  // write path of its own). Every step records itself in sessionStorage,
  // so a failure or refresh mid-commit RESUMES instead of duplicating.
  const commit = useMutation({
    mutationFn: async () => {
      let s = loadJob(jobID) ?? state;
      let periodID = s.createdID;
      if (periodID == null) {
        const p = unwrap(
          await api.POST("/api/v1/cashback/offer-periods", {
            body: {
              bank_client_id: Number(s.clientID),
              period_start: s.start,
              period_end: s.end,
              ...(s.attachmentIDs.length ? { attachment_ids: s.attachmentIDs } : {}),
            },
          }),
        );
        periodID = p.id;
        s = { ...s, createdID: periodID };
        saveJob(jobID, s);
      }
      if (s.slots != null && !s.slotsDone) {
        unwrap(
          await api.PUT("/api/v1/cashback/offer-periods/{id}/max-categories", {
            params: { path: { id: periodID } },
            body: { value: s.slots },
          }),
        );
        s = { ...s, slotsDone: true };
        saveJob(jobID, s);
      }
      const offersDone = { ...(s.offersDone ?? {}) };
      for (const row of s.rows ?? []) {
        if (offersDone[row.key] != null || !row.title.trim()) continue;
        const offer = unwrap(
          await api.POST("/api/v1/cashback/category-offers", {
            body: {
              offer_period_id: periodID,
              raw_title: row.title.trim(),
              ...(row.percent.trim() ? { percent: row.percent.trim() } : {}),
              ...(row.cap.trim() ? { cap_value: row.cap.trim() } : {}),
              kind: row.kind,
              ...(row.bankCategoryID != null ? { bank_category_id: row.bankCategoryID } : {}),
              ...(row.canonicalID != null ? { canonical_category_id: row.canonicalID } : {}),
            },
          }),
        );
        offersDone[row.key] = offer.id;
        s = { ...s, offersDone };
        saveJob(jobID, s);
      }
      const selectedDone = new Set(s.selectedDone ?? []);
      for (const row of s.rows ?? []) {
        const offerID = offersDone[row.key];
        if (!row.picked || offerID == null || selectedDone.has(row.key)) continue;
        unwrap(
          await api.POST("/api/v1/cashback/selections", {
            body: { category_offer_id: offerID, ...(backfill ? { backfill_override: true } : {}) },
          }),
        );
        selectedDone.add(row.key);
        s = { ...s, selectedDone: [...selectedDone] };
        saveJob(jobID, s);
      }
      return periodID;
    },
    onSuccess: onCommitted,
  });

  const pickedCount = rows.filter((r) => r.picked).length;
  const committable = rows.some((r) => r.title.trim() !== "");
  const partial = state.createdID != null;

  return (
    <>
      <div className="flex items-center gap-2.5">
        <button type="button" onClick={leave} className="flex h-8 w-8 flex-none items-center justify-center rounded-[10px] border border-brd bg-srf">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-tx2">
            <path d="M14.5 5 8 12l6.5 7" />
          </svg>
        </button>
        <h1 className="min-w-0 flex-1 truncate text-lg font-extrabold tracking-tight">Проверь распознанное</h1>
        {client && <Badge tone="indigo">{client.bank_name}</Badge>}
      </div>

      <Card className="p-4" data-sid="CB-02.c">
        <div className="space-y-3">
          <div className="grid grid-cols-2 gap-3">
            <Field label="Начало">
              <Input type="date" value={state.start} onChange={(e) => persist({ ...state, start: e.target.value })} />
            </Field>
            <Field label="Конец">
              <Input type="date" value={state.end} onChange={(e) => persist({ ...state, end: e.target.value })} />
            </Field>
          </div>
          {meta != null && meta.periodTexts.length > 0 && (
            <p className="text-[11px] font-medium text-tx3">На скриншотах: {meta.periodTexts.join(" · ")}</p>
          )}

          <Field label="Категорий можно выбрать">
            <Input
              type="number"
              min={1}
              inputMode="numeric"
              placeholder="как в тарифе"
              value={state.slots ?? ""}
              onChange={(e) => persist({ ...state, slots: e.target.value === "" ? null : Number(e.target.value) })}
            />
          </Field>
          {meta != null && meta.slotCandidates.length > 1 && (
            <div className="flex flex-wrap items-center gap-1.5 text-[11px] font-medium text-tx3">
              <span>Скриншоты расходятся:</span>
              {meta.slotCandidates.map((c, i) => (
                <button
                  key={i}
                  type="button"
                  onClick={() => persist({ ...state, slots: c.value })}
                  className={`rounded-full border px-2 py-0.5 ${state.slots === c.value ? "border-acc font-bold text-acc" : "border-brd2 text-tx3"}`}
                >
                  {c.value} (скрин {c.source_image})
                </button>
              ))}
            </div>
          )}
        </div>
      </Card>

      {meta != null && (meta.notes.length > 0 || meta.images.some((im) => im.skipped)) && (
        <Card className="p-3" data-sid="CB-02.d">
          {meta.notes.map((n, i) => (
            <p key={i} className="text-[11.5px] font-medium text-warn">
              ⚠ {n}
            </p>
          ))}
          <p className="mt-1 text-[10.5px] font-medium text-tx4">
            {meta.images
              .map((im, i) => `скрин ${i + 1} — ${im.skipped ? `пропущен${im.note ? ` (${im.note})` : ""}` : im.screenType || "прочитан"}`)
              .join(" · ")}
          </p>
        </Card>
      )}

      <div className="space-y-2" data-sid="CB-02.e">
        {rows.map((row) => (
          <div key={row.key} className={`rounded-xl border p-2.5 ${row.needsReview ? "border-warn/60 bg-warn/5" : "border-brd bg-srf"}`}>
            <div className="flex items-center gap-2">
              <input
                type="checkbox"
                checked={row.picked}
                onChange={(e) => patchRow(row.key, { picked: e.target.checked })}
                className="h-5 w-5 flex-none accent-[var(--t-acc)]"
                aria-label="выбрана в банке"
              />
              <Input
                value={row.title}
                placeholder="Название как в банке"
                // A new title invalidates the old title's mapping — the user
                // re-picks if they still want one.
                onChange={(e) => patchRow(row.key, { title: e.target.value, bankCategoryID: null, canonicalID: null, mappedTitle: null })}
                className="min-w-0 flex-1 !px-2.5 !py-1.5 text-[13px]"
              />
              <div className="relative w-[64px] flex-none">
                <Input
                  inputMode="decimal"
                  value={row.percent}
                  onChange={(e) => patchRow(row.key, { percent: e.target.value })}
                  className="!py-1.5 !pl-2 !pr-5 text-right text-[13px]"
                />
                <span className="pointer-events-none absolute inset-y-0 right-2 flex items-center text-[11px] text-tx4">%</span>
              </div>
              <button
                type="button"
                onClick={() => setRows(rows.filter((r) => r.key !== row.key))}
                className="flex h-7 w-7 flex-none items-center justify-center rounded-lg border border-brd text-[13px] text-tx3"
                aria-label="убрать строку"
              >
                ✕
              </button>
            </div>

            {row.conflictPercents.length > 0 && (
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11px] font-medium text-warn">
                <span>процент на скриншотах:</span>
                {row.conflictPercents.map((p) => (
                  <button
                    key={p}
                    type="button"
                    onClick={() => patchRow(row.key, { percent: p })}
                    className={`rounded-full border px-2 py-0.5 ${row.percent === p ? "border-acc font-bold text-acc" : "border-warn/50"}`}
                  >
                    {p}%
                  </button>
                ))}
              </div>
            )}
            {row.conflictCaps.length > 0 && (
              <div className="mt-1.5 flex flex-wrap items-center gap-1.5 text-[11px] font-medium text-warn">
                <span>лимит на скриншотах:</span>
                {row.conflictCaps.map((c) => (
                  <button
                    key={c}
                    type="button"
                    onClick={() => patchRow(row.key, { cap: c })}
                    className={`rounded-full border px-2 py-0.5 ${row.cap === c ? "border-acc font-bold text-acc" : "border-warn/50"}`}
                  >
                    до {c} ₽
                  </button>
                ))}
              </div>
            )}

            <div className="mt-2 flex items-center gap-2">
              {row.kind !== "regular" && <Badge tone="amber">{row.kind === "super" ? "барабан" : "спец"}</Badge>}
              <div className="min-w-0 flex-1">
                {client ? (
                  <CategoryPicker
                    bankID={client.bank_id}
                    bankName={client.bank_name ?? ""}
                    value={
                      row.bankCategoryID != null || row.canonicalID != null
                        ? { bankCategoryID: row.bankCategoryID ?? undefined, title: row.mappedTitle ?? row.title, canonicalID: row.canonicalID }
                        : null
                    }
                    onChange={(v: PickedCategory) =>
                      patchRow(row.key, {
                        bankCategoryID: v.bankCategoryID ?? null,
                        canonicalID: v.canonicalID ?? null,
                        mappedTitle: v.title,
                        ...(row.kind === "regular" && v.kind && v.kind !== "regular" ? { kind: v.kind } : {}),
                      })
                    }
                  />
                ) : null}
              </div>
              <div className="relative w-[86px] flex-none">
                <Input
                  inputMode="numeric"
                  placeholder="лимит"
                  value={row.cap}
                  onChange={(e) => patchRow(row.key, { cap: e.target.value })}
                  className="!py-1.5 !pl-2 !pr-5 text-right text-[12px]"
                />
                <span className="pointer-events-none absolute inset-y-0 right-2 flex items-center text-[11px] text-tx4">₽</span>
              </div>
            </div>

            {row.notes.map((n, i) => (
              <p key={i} className="mt-1 text-[11px] font-medium text-warn">
                ⚠ {n}
              </p>
            ))}
          </div>
        ))}

        <Btn
          type="button"
          variant="soft"
          className="w-full"
          onClick={() =>
            setRows([
              ...rows,
              {
                key: rows.reduce((m, r) => Math.max(m, r.key), 0) + 1,
                title: "",
                percent: "",
                cap: "",
                kind: "regular",
                picked: false,
                bankCategoryID: null,
                canonicalID: null,
                mappedTitle: null,
                needsReview: false,
                notes: [],
                conflictPercents: [],
                conflictCaps: [],
              },
            ])
          }
        >
          + Добавить строку
        </Btn>
      </div>

      <Card className="p-4" data-sid="CB-02.f">
        <Btn type="button" disabled={commit.isPending || !committable} className="w-full" onClick={() => commit.mutate()}>
          {commit.isPending
            ? "Сохранение…"
            : partial
              ? "Продолжить сохранение"
              : `Создать период · ${rows.filter((r) => r.title.trim()).length} категорий, ${pickedCount} выбрано`}
        </Btn>
        <p className="mt-1.5 text-center text-[10.5px] font-medium text-tx4">
          {state.attachmentIDs.length > 0 ? `Скриншоты (${state.attachmentIDs.length}) приложатся к периоду. ` : ""}
          {backfill ? "Отметки запишутся задним числом. " : ""}
          Ничего не сохранится, пока не нажмёшь.
        </p>
        {partial && commit.error != null && (
          <p className="mt-1 text-center text-[11px] font-medium text-warn">Часть уже записана — повторная попытка продолжит с места остановки.</p>
        )}
        <ErrMsg error={commit.error} />
      </Card>
    </>
  );
}
