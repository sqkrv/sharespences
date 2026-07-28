import { useEffect, useSyncExternalStore } from "react";
import { useQuery } from "@tanstack/react-query";
import { api, unwrap, type RecognitionDraft } from "./api/client";

// Client-side state for screenshot-recognition jobs (spec:
// cashback-recognizer.md). Two jobs of its own:
//
//  1. **Survive leaving the screen.** The server job is a goroutine that
//     runs whether or not anyone is looking, but its result is held in
//     memory for 30 minutes only. Storage is therefore localStorage, not
//     sessionStorage: closing the installed PWA must not orphan a job
//     you are still waiting for.
//  2. **Be findable without the URL.** The job id lives in ?job=…, which
//     is gone the moment you navigate away. This module keeps an index
//     so the shell chip (components/RecognitionChip) can surface a
//     running or finished job from anywhere in the app.
//
// Once a finished draft is captured here, the server job is irrelevant —
// commit replays the four ordinary write endpoints and never touches it.

export type ReviewRow = {
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

export type JobMeta = {
  notes: string[];
  periodTexts: string[];
  slotCandidates: { value: number; source_image: number }[];
  images: { screenType: string; skipped: boolean; note: string }[];
};

export type JobState = {
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

export type Recognition = { id: string; startedAt: number; state: JobState };

const INDEX_KEY = "recognize-jobs";
const stateKey = (id: string) => `recognize-${id}`;
// An uncommitted draft older than this is abandoned, not pending — drop it
// rather than let the chip nag forever.
const MAX_AGE_MS = 24 * 60 * 60 * 1000;

type IndexEntry = { id: string; startedAt: number };

function readIndex(): IndexEntry[] {
  try {
    const parsed = JSON.parse(localStorage.getItem(INDEX_KEY) ?? "[]");
    return Array.isArray(parsed) ? parsed : [];
  } catch {
    return [];
  }
}

const writeIndex = (entries: IndexEntry[]) => localStorage.setItem(INDEX_KEY, JSON.stringify(entries));

export function loadJob(id: string): JobState | null {
  try {
    const raw = localStorage.getItem(stateKey(id));
    return raw ? (JSON.parse(raw) as JobState) : null;
  } catch {
    return null;
  }
}

// useSyncExternalStore compares snapshots by reference — recompute only on
// an actual write, never inside the getter.
let snapshot: Recognition[] = [];
const subs = new Set<() => void>();

function refresh() {
  const now = Date.now();
  snapshot = readIndex()
    .map((e) => {
      const state = loadJob(e.id);
      return state && now - e.startedAt < MAX_AGE_MS ? { ...e, state } : null;
    })
    .filter((e): e is Recognition => e != null);
  subs.forEach((cb) => cb());
}

// Drop expired / orphaned entries from storage. Deliberately NOT part of
// refresh(): a write inside a `storage`-event handler would ping-pong
// between tabs.
function prune() {
  writeIndex(
    readIndex().filter((e) => {
      const live = Date.now() - e.startedAt < MAX_AGE_MS && loadJob(e.id) != null;
      if (!live) localStorage.removeItem(stateKey(e.id));
      return live;
    }),
  );
}

export function startJob(id: string, state: JobState) {
  localStorage.setItem(stateKey(id), JSON.stringify(state));
  writeIndex([{ id, startedAt: Date.now() }, ...readIndex().filter((e) => e.id !== id)]);
  refresh();
}

export function saveJob(id: string, state: JobState) {
  localStorage.setItem(stateKey(id), JSON.stringify(state));
  refresh();
}

export function clearJob(id: string) {
  localStorage.removeItem(stateKey(id));
  writeIndex(readIndex().filter((e) => e.id !== id));
  refresh();
}

// Drafts carry personal data (bank client, screenshot ids, the menu
// itself) — they follow the same shared-device rule as the offline caches
// (docs/specs/pwa.md) and must not survive logout.
export function clearAllRecognitions() {
  for (const e of readIndex()) localStorage.removeItem(stateKey(e.id));
  localStorage.removeItem(INDEX_KEY);
  refresh();
}

prune();
refresh();

// Another tab committed or cancelled — don't leave a stale chip behind.
window.addEventListener("storage", (e) => {
  if (e.key == null || e.key === INDEX_KEY || e.key.startsWith("recognize-")) refresh();
});

function subscribe(cb: () => void) {
  subs.add(cb);
  return () => {
    subs.delete(cb);
  };
}

export function useRecognitions(): Recognition[] {
  return useSyncExternalStore(subscribe, () => snapshot);
}

// Newest first — the server allows one running job per user, so this is
// «the» job in practice; older entries are uncommitted drafts.
export function useActiveRecognition(): Recognition | null {
  return useRecognitions()[0] ?? null;
}

export function useRecognition(id: string): Recognition | null {
  return useRecognitions().find((r) => r.id === id) ?? null;
}

export type JobPhase = {
  done?: number;
  total?: number;
  image?: number;
  pass?: string;
  attempt?: number;
  reduced?: boolean;
};

// What the server is doing right now, in one line. Kept honest: a rung
// past the first means the model failed to answer in the expected shape
// and the request is being escalated — that is the reason a long wait got
// longer, and hiding it would make the job look stuck for no reason.
export function phaseCaption(job: JobPhase | undefined): string {
  if (!job?.image) return "готовим скриншоты…";
  const total = job.total ?? 0;
  const which = total > 1 ? `скрин ${job.image} из ${total} · ` : "";
  const what = job.pass === "slots" ? "считаем, сколько категорий можно выбрать" : "читаем меню";
  const extra = [
    (job.attempt ?? 1) > 1 ? `попытка ${job.attempt} из 3` : "",
    job.reduced ? "картинка уменьшена" : "",
  ].filter(Boolean);
  return which + what + (extra.length ? ` · ${extra.join(" · ")}` : "");
}

export function draftRows(d: RecognitionDraft): ReviewRow[] {
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

// Poll a job until its draft is captured. Shared by the review screen and
// the shell chip: identical query keys mean TanStack runs ONE poll no
// matter how many of them are mounted, and whichever is alive captures
// the draft — so a job that finishes while you are on «Обзор» is saved
// locally before the server's 30-minute window can expire.
export function useRecognitionPoll(job: Recognition | null) {
  const id = job?.id;
  const captured = job?.state.rows != null;

  const poll = useQuery({
    queryKey: ["recognition", id],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/recognitions/{id}", { params: { path: { id: id! } } })),
    enabled: id != null && !captured,
    refetchInterval: (q) => (q.state.data?.status === "running" ? 2000 : false),
    staleTime: 0,
    retry: false,
  });

  useEffect(() => {
    if (job == null || captured || poll.data?.status !== "done" || !poll.data.draft) return;
    const d = poll.data.draft;
    saveJob(job.id, {
      ...job.state,
      rows: draftRows(d),
      slots: d.slot_count ?? null,
      meta: {
        notes: d.notes ?? [],
        periodTexts: d.period_texts ?? [],
        slotCandidates: (d.slot_candidates ?? []).map((c) => ({ value: c.value, source_image: c.source_image })),
        images: (d.images ?? []).map((im) => ({
          screenType: im.screen_type ?? "",
          skipped: im.skipped ?? false,
          note: im.note ?? "",
        })),
      },
    });
  }, [poll.data, job, captured]);

  return poll;
}
