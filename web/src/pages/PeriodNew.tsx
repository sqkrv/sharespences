import { useMemo, useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ApiError, api, attachmentURL, unwrap, uploadAttachment } from "../api/client";
import { useBankCategories, useCategories, useClients, useTierMap } from "../hooks";
import { BackButton, Badge, Btn, Card, ErrMsg, errorText, Field, Input, Select, Spinner } from "../components/ui";
import { CategoryPicker, type PickedCategory } from "../components/CategoryPicker";
import { Lightbox } from "../components/Lightbox";
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
import { isoDate, monthGenOf, monthNameOf, monthRange, parseMonthHints, plural, quarterRange } from "../lib";

// CB-02 «Меню месяца» (redesign 2h): dates are gone from the UI — the month
// is known from context (?month=, else today) and the program's period_type
// derives the range silently (МКБ quarter: filling August fills the whole
// quarter, one offer_period spanning it). The API still takes explicit
// dates; this screen just stops asking the user to retype the calendar.
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
  const [files, setFiles] = useState<File[]>([]);

  const client = (clients.data ?? []).find((c) => String(c.id) === clientID);

  // The range never shows as dates: month + the program's period_type derive
  // it (МКБ → the month's whole quarter, one period covering all three).
  const info = client?.program_tier_id != null ? tierMap.data?.get(client.program_tier_id) : undefined;
  const isQuarter = info?.program.period_type === "quarter";
  const range = isQuarter ? quarterRange(baseDate) : monthRange(baseDate);
  const { start, end } = range;
  const monthISO = isoDate(baseDate);

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
      // replace: the spent form must not sit in history — «назад» from the
      // created period returns to where the flow started, not to the form.
      navigate(`/periods/${p.id}`, { replace: true });
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
        <BackButton small />
        <h1 className="min-w-0 flex-1 truncate text-lg font-extrabold tracking-tight">Меню месяца</h1>
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
          <div className="flex items-center gap-2.5">
            <div className="min-w-0 flex-1">
              <p className="text-[17px] font-extrabold tracking-tight">Меню {monthGenOf(monthISO)}</p>
              <p className="mt-0.5 text-[11px] font-medium text-tx4">
                {client ? "слоты и лимит из каталога" : "выбери банк — месяц уже известен"}
              </p>
            </div>
            {isQuarter && (
              <Badge tone="amber">
                квартал · {monthNameOf(start)}–{monthNameOf(end)}
              </Badge>
            )}
          </div>
          <Field label="Банк · держатель">
            <Select required value={clientID} onChange={(e) => setClientID(e.target.value)}>
              <option value="">— выбери —</option>
              {(clients.data ?? []).map((c) => (
                <option key={c.id} value={c.id}>
                  {c.bank_name}
                  {c.label ? ` · ${c.label}` : ""}
                </option>
              ))}
            </Select>
          </Field>
          {isQuarter && (
            <p className="flex items-center gap-2 rounded-xl border border-acc/25 bg-acc/10 px-3 py-2 text-[11px] leading-snug font-medium text-tx2">
              <span className="h-1.5 w-1.5 flex-none rounded-full bg-acc" />
              У {client?.bank_name} меню квартальное: заполни {monthNameOf(monthISO)} — весь квартал заполнится тем же меню
            </p>
          )}
          <label className="flex cursor-pointer items-center gap-2.5 rounded-xl border border-dashed border-dash px-3 py-2.5">
            <span className="h-[26px] w-[26px] flex-none rounded-md" style={{ background: "repeating-linear-gradient(120deg, var(--t-inset) 0 5px, var(--t-srf2) 5px 10px)" }} />
            <span className="min-w-0 flex-1">
              <span className="block text-[11px] font-semibold text-tx2">Скрины меню из банка</span>
              <span className="block text-[9px] font-medium text-tx4">{files.length > 0 ? `${files.length} фото` : "пока нет"}</span>
            </span>
            {/* The recognizer decodes PNG/JPEG/WebP; HEIC and PDF would only
                skip with a note, so the picker doesn't offer them. */}
            <input type="file" accept="image/png,image/jpeg,image/webp" multiple className="hidden" onChange={(e) => setFiles([...(e.target.files ?? [])])} />
          </label>
          {/* Recognize leads (the ritual's main path); manual is the honest
              fallback and creates the period straight away. The recognize
              button shows even with no files picked (disabled): it IS how
              one learns the screenshots can be read automatically. */}
          <Btn
            type="button"
            disabled={create.isPending || recognize.isPending || !clientID || files.length === 0}
            className="w-full"
            onClick={() => recognize.mutate()}
            title={files.length === 0 ? "Сначала приложи скрины меню выше" : undefined}
          >
            {recognize.isPending ? "Загрузка скриншотов…" : "Распознать со скриншотов"}
          </Btn>
          <Btn type="submit" variant="soft" disabled={create.isPending || recognize.isPending || !clientID} className="w-full">
            {create.isPending ? "Создание…" : "Заполнить вручную"}
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
      <BackButton small />
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
                  {[client && [client.bank_name, client.label].filter(Boolean).join(" · "), `меню ${monthGenOf(state.start)}`]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
                <p className="mt-0.5 text-[12px] font-semibold text-acc">{phaseCaption(poll.data)}</p>
                <p className="mt-0.5 text-[12px] font-medium text-tx3">
                  Локальная модель читает меню ≈30 секунд на скриншот. Можно уйти с экрана — плашка внизу покажет, когда будет
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

  // 7a: the partial-result gate — some screenshots were skipped, say so in
  // the same card before review. Nothing is written until review confirms;
  // the draft survives reload. Re-recognizing with extra screens stays out
  // (recognizer decision: no «дораспознать») — the re-shoot path starts
  // over from the form.
  const skippedIdx = (state.meta?.images ?? [])
    .map((im, i) => (im.skipped ? i : -1))
    .filter((i) => i >= 0);
  if (skippedIdx.length > 0 && !state.ackPartial) {
    const readCount = (state.rows ?? []).filter((r) => r.title.trim()).length;
    const list = skippedIdx.map((i) => i + 1).join(" и ");
    return (
      <>
        {header}
        <Card className="space-y-3 p-4" data-sid="CB-02.d">
          <div className="flex items-center gap-2.5">
            <div className="min-w-0 flex-1">
              <p className="text-[16px] font-extrabold tracking-tight">Меню {monthGenOf(state.start)}</p>
              <p className="mt-0.5 text-[11px] font-semibold text-warn">распознавание · не всё получилось</p>
            </div>
            <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[11px] font-bold text-tx3">
              {state.attachmentIDs.length} {plural(state.attachmentIDs.length, "скрин", "скрина", "скринов")}
            </span>
          </div>
          <div className="flex gap-2 overflow-x-auto">
            {state.attachmentIDs.map((aid, i) => (
              <div key={aid} className="relative flex-none">
                <img
                  src={attachmentURL(aid)}
                  alt={`скрин ${i + 1}`}
                  className={`h-20 rounded-xl border object-cover ${skippedIdx.includes(i) ? "border-warn/40 opacity-50" : "border-brd"}`}
                />
                {skippedIdx.includes(i) && (
                  <span className="absolute -top-1.5 -right-1.5 flex h-5 w-5 items-center justify-center rounded-full border border-warn/40 bg-srf text-[10px] font-bold text-warn">
                    ✕
                  </span>
                )}
              </div>
            ))}
          </div>
          <p className="text-[12px] leading-snug font-medium text-tx2">
            {skippedIdx.length === 1 ? `Скрин ${list} не похож` : `Скрины ${list} не похожи`} на меню банка — возможно, это не
            тот экран или текст размыт.{" "}
            {readCount > 0
              ? `Из остальных ${readCount === 1 ? "прочитана 1 категория" : `прочитано ${readCount} ${plural(readCount, "категория", "категории", "категорий")}`}, черновик сохранён.`
              : "Прочитать не удалось ничего."}
          </p>
          {readCount > 0 ? (
            <>
              <Btn className="w-full" onClick={() => persist({ ...state, ackPartial: true })}>
                Продолжить с {readCount} {plural(readCount, "категорией", "категориями", "категориями")}
              </Btn>
              <Btn variant="ghost" className="w-full" onClick={discard}>
                Переснять заново
              </Btn>
            </>
          ) : (
            <Btn variant="soft" className="w-full" onClick={discard}>
              К форме — переснять или заполнить вручную
            </Btn>
          )}
          <p className="text-[10px] leading-snug font-medium text-tx4">
            Ничего не записано, пока не пройдена проверка распознанного.
          </p>
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
        navigate(`/periods/${periodID}`, { replace: true });
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
  const [viewing, setViewing] = useState<number | null>(null);
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

  // «На скринах — июль, записываем в август»: month names hinted by the
  // screenshots vs the months the period covers (a quarter covers three).
  // Warn-only; the target month never changes silently.
  const monthMismatch = useMemo(() => {
    const hints = parseMonthHints(state.meta?.periodTexts ?? []);
    if (hints.length === 0) return null;
    const covered = new Set<number>();
    let d = new Date(Number(state.start.slice(0, 4)), Number(state.start.slice(5, 7)) - 1, 1);
    const endD = new Date(Number(state.end.slice(0, 4)), Number(state.end.slice(5, 7)) - 1, 1);
    while (d <= endD) {
      covered.add(d.getMonth());
      d = new Date(d.getFullYear(), d.getMonth() + 1, 1);
    }
    const foreign = hints.filter((m) => !covered.has(m));
    if (foreign.length === 0 || foreign.length < hints.length) return null; // some hint agrees → no alarm
    return foreign.map((m) => monthNameOf(`0000-${String(m + 1).padStart(2, "0")}-01`)).join(", ");
  }, [state.meta, state.start, state.end]);

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
        <BackButton small />
        <h1 className="min-w-0 flex-1 truncate text-lg font-extrabold tracking-tight">Проверь распознанное</h1>
        {client && <Badge tone="indigo">{client.bank_name}</Badge>}
      </div>

      <Card className="p-4" data-sid="CB-02.c">
        <div className="space-y-3">
          <div className="flex items-center gap-2.5">
            <div className="min-w-0 flex-1">
              <p className="text-[16px] font-extrabold tracking-tight">Меню {monthGenOf(state.start)}</p>
              <p className="mt-0.5 text-[11px] font-medium text-tx4">
                {[client && [client.bank_name, client.label].filter(Boolean).join(" · "), "слоты и лимит из каталога"]
                  .filter(Boolean)
                  .join(" · ")}
              </p>
            </div>
            <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[11px] font-bold text-tx3">
              {state.attachmentIDs.length} {plural(state.attachmentIDs.length, "скрин", "скрина", "скринов")}
            </span>
          </div>
          {/* Month check without date fields (3b): the verbatim screenshot
              hints are compared against the month being written — a warning
              to read, never a silent change of the target month. */}
          {monthMismatch && (
            <p className="flex items-center gap-2 rounded-[10px] border border-warn/35 bg-warn/5 px-2.5 py-2 text-[11px] leading-snug font-medium text-warn">
              <span className="h-[5px] w-[5px] flex-none rounded-full bg-warn" />
              На скринах — {monthMismatch}. Записываем в {monthNameOf(state.start)}: проверь, то ли это меню
            </p>
          )}
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

      {/* The screenshots themselves, which this screen never showed: every
          row below is a claim about one of these pictures, and the notes
          address them by number («скрин 2»), so the strip is numbered to
          match. Reviewing a prefill without the source is guesswork. */}
      {state.attachmentIDs.length > 0 && (
        <Card className="p-3" data-sid="CB-02.g">
          <p className="mb-1.5 text-[10.5px] font-semibold tracking-[.06em] text-tx4 uppercase">
            Скриншоты
          </p>
          <div className="flex gap-2 overflow-x-auto">
            {state.attachmentIDs.map((aid, n) => (
              <button
                key={aid}
                type="button"
                onClick={() => setViewing(n)}
                className="relative flex-none"
                aria-label={`Открыть скриншот ${n + 1}`}
              >
                <img
                  src={attachmentURL(aid)}
                  alt={`скриншот ${n + 1}`}
                  className="h-20 rounded-xl border border-brd object-cover"
                />
                <span className="absolute bottom-1 left-1 rounded bg-black/60 px-1 text-[9px] font-bold text-white">
                  {n + 1}
                </span>
              </button>
            ))}
          </div>
        </Card>
      )}
      {viewing != null && (
        <Lightbox
          ids={state.attachmentIDs}
          startIndex={viewing}
          alt="скриншот меню"
          onClose={() => setViewing(null)}
        />
      )}

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
