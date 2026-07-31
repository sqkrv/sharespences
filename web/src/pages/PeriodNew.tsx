import { useEffect, useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate, useSearchParams } from "react-router-dom";
import { ApiError, api, unwrap, uploadAttachment } from "../api/client";
import { useBankCategories, useCategories, useClients, useTierMap } from "../hooks";
import { Badge, Btn, Card, ErrMsg, errorText, Field, Input, Select, Spinner } from "../components/ui";
import { CategoryPicker, type PickedCategory } from "../components/CategoryPicker";
import {
  clearJob,
  loadJob,
  phaseCaption,
  saveJob,
  startJob,
  useRecognition,
  useRecognitionPoll,
  type JobState,
  type ReviewRow,
} from "../recognition";
import ProgressRing from "../components/ProgressRing";
import { fmtRange, monthRange, quarterRange } from "../lib";

// S1 step 1, design screen 07 header: «Новый период» — pick the bank client
// (person × bank; all its cards share the selection), the range defaults
// from the program's period_type (МКБ → quarter), screenshots are optional
// evidence.
//
// Recognize mode (spec cashback-recognizer.md, CB-02): the same screen is
// also the recognizer flow — form → recognizing → review. The job id lives
// in ?job=…, and the draft itself in the localStorage store (../recognition)
// so it survives leaving the screen, closing the app, and a refresh
// mid-commit; the shell chip is what brings you back.

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
  // backfills THAT month, not today (2026-07-15). Mid-month day-15
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
      startJob(job.id, { clientID, start, end, attachmentIDs });
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
          {/* Shown even with no files picked (disabled): the button IS how
              one learns the screenshots can be read automatically — hidden
              until a file was chosen, it was only ever found by accident. */}
          <Btn
            type="button"
            variant="soft"
            disabled={create.isPending || recognize.isPending || !clientID || files.length === 0}
            className="w-full"
            onClick={() => recognize.mutate()}
            title={files.length === 0 ? "Сначала приложи скрины меню выше" : undefined}
          >
            {recognize.isPending ? "Загрузка скриншотов…" : "Распознать со скриншотов"}
          </Btn>
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
  // The store is the single source of truth — the shell chip reads the
  // same entry, so what you edit here and what it reports never diverge.
  const job = useRecognition(jobID);
  const state = job?.state ?? null;
  const persist = (s: JobState) => saveJob(jobID, s);
  const poll = useRecognitionPoll(job);

  // Leaving KEEPS the draft — the chip is what brings you back. Discarding
  // is a separate, explicit act.
  const backToForm = () => {
    const next = new URLSearchParams(params);
    next.delete("job");
    setParams(next, { replace: true });
  };
  const discard = () => {
    clearJob(jobID);
    backToForm();
  };

  // What is being recognized — the wait is minutes long and the chip can
  // bring you back to it from anywhere, so the screen has to say which bank
  // client and which period the job belongs to. Read-only here; both are
  // editable one step later, on review.
  const client = (clients.data ?? []).find((c) => String(c.id) === state?.clientID);

  const header = (
    <div className="flex items-center gap-2.5">
      <Link to="/" className="flex h-8 w-8 flex-none items-center justify-center rounded-[10px] border border-brd bg-srf">
        <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-tx2">
          <path d="M14.5 5 8 12l6.5 7" />
        </svg>
      </Link>
      <h1 className="min-w-0 flex-1 truncate text-lg font-extrabold tracking-tight">Распознавание</h1>
      {client && <Badge tone="indigo">{client.bank_name}</Badge>}
    </div>
  );

  if (!state) {
    return (
      <>
        {header}
        <Card className="p-4" data-sid="CB-02.b">
          <p className="text-sm font-semibold">Черновик не найден</p>
          <p className="mt-1 text-[12px] font-medium text-tx3">
            Задание отменено или ему больше суток. Начни заново — скриншоты придётся выбрать ещё раз.
          </p>
          <Btn variant="soft" className="mt-3 w-full" onClick={backToForm}>
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
                  : (poll.data?.error ?? errorText(poll.error))}
              </p>
              <p className="mt-1 text-[12px] font-medium text-tx3">Период всегда можно заполнить вручную — скриншоты уже загружены.</p>
              <Btn variant="soft" className="mt-3 w-full" onClick={discard}>
                Заполнить вручную
              </Btn>
            </>
          ) : (
            <div className="flex items-start gap-3">
              <ProgressRing done={poll.data?.done ?? 0} total={poll.data?.total ?? state.attachmentIDs.length} active />
              <div className="min-w-0 flex-1">
                <p className="text-sm font-semibold">Распознаём скриншоты</p>
                <p className="mt-0.5 text-[11px] font-medium text-tx3">
                  {[client && [client.bank_name, client.label].filter(Boolean).join(" · "), fmtRange(state.start, state.end)]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
                <p className="mt-0.5 text-[12px] font-semibold text-acc">{phaseCaption(poll.data)}</p>
                <p className="mt-0.5 text-[12px] font-medium text-tx3">
                  Локальная модель читает меню ≈2–3 минуты на скриншот. Можно уйти с экрана — плашка внизу покажет, когда будет
                  готово. Если закрыть приложение совсем, результат ждёт на сервере 30 минут.
                </p>
                <button type="button" className="mt-1.5 text-[11.5px] font-semibold text-tx4 underline" onClick={discard}>
                  Отменить и заполнить вручную
                </button>
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
      discard={discard}
      client={(clients.data ?? []).find((c) => String(c.id) === state.clientID)}
      onCommitted={(periodID) => {
        clearJob(jobID);
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
  discard,
  client,
  onCommitted,
}: {
  jobID: string;
  state: JobState;
  persist: (s: JobState) => void;
  discard: () => void;
  client?: { bank_id: number; bank_name?: string; label?: string | null };
  onCommitted: (periodID: number) => void;
}) {
  const rows = state.rows ?? [];
  const meta = state.meta;
  const setRows = (next: ReviewRow[]) => persist({ ...state, rows: next });
  const patchRow = (key: number, patch: Partial<ReviewRow>) => setRows(rows.map((r) => (r.key === key ? { ...r, ...patch } : r)));

  // The draft carries ids, not presentation — resolve the emoji and the
  // canonical title from the catalog at render time, the same way the
  // period screen does. Resolving here (rather than storing it on the row)
  // is what keeps the icon correct after a manual re-pick too, since the
  // picker's onChange only ever writes ids back.
  const bankCats = useBankCategories(client?.bank_id);
  const canonicals = useCategories();
  const mappingOf = (row: ReviewRow) => {
    // The API already resolves a catalog row's emoji (own, else canonical).
    const bc = row.bankCategoryID != null ? (bankCats.data ?? []).find((r) => r.id === row.bankCategoryID) : undefined;
    const canonID = bc?.canonical_category_id ?? row.canonicalID;
    const canon = canonID != null ? (canonicals.data ?? []).find((c) => c.id === canonID) : undefined;
    return {
      emoji: bc?.emoji ?? canon?.emoji ?? null,
      canonicalTitle: bc?.canonical_title_ru ?? canon?.title_ru ?? null,
    };
  };

  // Distinct slot counts across the screenshots: one value means they
  // agree (the server already prefilled it), more means a real conflict.
  const slotDisagreement = useMemo(
    () => [...new Set((state.meta?.slotCandidates ?? []).map((c) => c.value))],
    [state.meta],
  );

  // A selection dated today would fall outside a backfilled period —
  // mirror the Period screen's «задним числом» switch automatically.
  const backfill = useMemo(() => {
    const today = new Date().toISOString().slice(0, 10);
    return !(state.start <= today && today <= state.end);
  }, [state.start, state.end]);

  // Commit replays the four existing endpoints (the recognizer has no
  // write path of its own). Every step records itself in the store, so a
  // failure or refresh mid-commit RESUMES instead of duplicating.
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
              ...(row.subtitle.trim() ? { notes: row.subtitle.trim() } : {}),
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
        {/* Leaving keeps the draft — the shell chip brings you back. */}
        <Link to="/" className="flex h-8 w-8 flex-none items-center justify-center rounded-[10px] border border-brd bg-srf">
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" className="text-tx2">
            <path d="M14.5 5 8 12l6.5 7" />
          </svg>
        </Link>
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
          {/* Only when the screenshots actually DISAGREE — several shots of
              one menu normally report the same number, and calling that
              «расходятся» was crying wolf. */}
          {slotDisagreement.length > 1 && (
            <div className="flex flex-wrap items-center gap-1.5 text-[11px] font-medium text-warn">
              <span>Скриншоты расходятся:</span>
              {slotDisagreement.map((v) => (
                <button
                  key={v}
                  type="button"
                  onClick={() => persist({ ...state, slots: v })}
                  className={`rounded-full border px-2 py-0.5 ${state.slots === v ? "border-acc font-bold text-acc" : "border-warn/50"}`}
                >
                  {v} ({(meta?.slotCandidates ?? [])
                    .filter((c) => c.value === v)
                    .map((c) => `скрин ${c.source_image}`)
                    .join(", ")})
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

            {/* The bank's own subtitle: the only thing distinguishing two
                rows it lists under one title, so it is shown even though
                nothing here edits it. */}
            {row.subtitle && <p className="mt-1 pl-7 text-[11px] text-tx3">{row.subtitle}</p>}

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
                        ? {
                            bankCategoryID: row.bankCategoryID ?? undefined,
                            title: row.mappedTitle ?? row.title,
                            canonicalID: row.canonicalID,
                            kind: row.kind,
                            ...mappingOf(row),
                          }
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
                subtitle: "",
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
          Ничего не сохранится, пока не нажмёшь. Черновик ждёт, даже если закрыть приложение.
        </p>
        {partial && commit.error != null && (
          <p className="mt-1 text-center text-[11px] font-medium text-warn">Часть уже записана — повторная попытка продолжит с места остановки.</p>
        )}
        <ErrMsg error={commit.error} />
        {/* Explicit discard: the only thing that throws the draft away —
            «назад» and closing the app both keep it. Blocked once a period
            exists, since abandoning then would strand a half-filled one. */}
        {!partial && (
          <button
            type="button"
            className="mt-3 w-full text-center text-[11.5px] font-semibold text-tx4 underline"
            onClick={() => {
              if (window.confirm("Удалить распознанный черновик? Скриншоты останутся загруженными.")) discard();
            }}
          >
            Удалить черновик
          </button>
        )}
      </Card>
    </>
  );
}
