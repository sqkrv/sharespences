// MD-01 Модерация (design4 «Moderation - Module», boards 1b–1e): the queue
// holds user_manual submissions with the top card expanded — exactly the 5e
// form's fields plus the resolved category and the duplicate net; decisions
// are one tap. Reject opens the 1c sheet: an optional reason FOR THE
// OPERATOR, never returned to the author, and the sheet says so plainly.
// «Опубликованные» (1d) is the after-the-fact supervision of everything
// people wrote — вручную, из транзакций, админом — where «Снять» pulls a
// row out of search. The rows are ANONYMOUS: the API never carries the
// submitter. Role knowledge in the SPA is cosmetic; the server re-checks
// on every request.
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { api, unwrap, type Schemas } from "../api/client";
import { useMe } from "../auth";
import { useCategories } from "../hooks";
import { BackButton, Btn, Empty, ErrMsg, SegTabs, Spinner } from "../components/ui";
import { Sheet } from "../components/Sheet";
import { FALLBACK_EMOJI } from "../lib";

type Tab = "pending" | "published";
type Row = Schemas["ModerationRowDTO"];

const ORIGIN_LABEL: Record<string, string> = {
  user_manual: "вручную",
  user_transaction: "из транзакций",
  admin: "админ",
};
const TYPE_EMOJI: Record<string, string> = { offline: "📍", online: "🌐", app: "📱", other: "✏️" };
const TYPE_LABEL: Record<string, string> = { offline: "Офлайн", online: "Онлайн", app: "Приложение", other: "Другое" };

// «10.08», or «сегодня» for today — the boards' date register.
function fmtDay(stamp?: string | null): string {
  if (!stamp) return "";
  const day = stamp.slice(0, 10);
  if (day === new Date().toISOString().slice(0, 10)) return "сегодня";
  return `${day.slice(8, 10)}.${day.slice(5, 7)}`;
}

// The expanded queue card (1b): chips, labeled rows, the duplicate net, and
// the verdict pair. Resolve and similar are fetched only here — one card is
// open at a time, so the queue costs two extra requests, not 2N.
function ExpandedCard({ row, onApprove, onReject, busy }: { row: Row; onApprove: () => void; onReject: () => void; busy: boolean }) {
  const navigate = useNavigate();
  const categories = useCategories();
  const resolve = useQuery({
    queryKey: ["mcc-resolve", row.mcc],
    enabled: !!row.mcc,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/resolve", { params: { query: { code: row.mcc ?? "" } } })),
  });
  const canonRef = (resolve.data?.canonicals ?? [])[0];
  const canon = canonRef && (categories.data ?? []).find((c) => c.slug === canonRef.slug);
  const similar = useQuery({
    queryKey: ["pos-similar", row.mcc, row.name],
    enabled: !!row.mcc && row.name.length >= 2,
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/mcc/points-of-sale/similar", { params: { query: { mcc: row.mcc ?? "", name: row.name } } })) ?? [],
  });
  const hits = (similar.data ?? []).filter((s) => s.id !== row.id).slice(0, 2);

  const label = "w-[82px] flex-none text-[11.5px] font-semibold text-tx4";
  const chip = "rounded-[9px] border border-brd bg-srf px-2 py-1 text-[11px] font-semibold";
  return (
    <div className="rounded-[18px] border border-acc bg-srf2 p-3.5 shadow-[0_10px_26px_-16px_rgba(139,111,255,.5)]" data-sid="MD-01.c">
      <div className="flex items-center gap-2">
        <p className="min-w-0 flex-1 truncate text-base font-extrabold tracking-tight">{row.name}</p>
        <span className="flex-none text-[10.5px] font-medium text-tx4">{fmtDay(row.created_at)}</span>
      </div>
      <div className="mt-2 flex flex-wrap gap-1.5">
        {row.mcc && <span className={`${chip} font-mono font-bold text-accl`}>MCC {row.mcc}</span>}
        <span className={`${chip} text-tx2`}>
          {canon ? `${canon.title_ru} · ${canon.emoji || FALLBACK_EMOJI}` : (row.mcc_name ?? "категория неизвестна")}
        </span>
        {row.type && (
          <span className={`${chip} text-tx2`}>
            {TYPE_EMOJI[row.type]} {TYPE_LABEL[row.type]}
          </span>
        )}
      </div>
      <div className="mt-2.5 space-y-1.5">
        {row.address && (
          <div className="flex gap-2 text-[11.5px]">
            <span className={label}>Адрес</span>
            <span className="min-w-0 flex-1 font-semibold text-tx">{row.address}</span>
          </div>
        )}
        {row.merchant_title && (
          <div className="flex gap-2 text-[11.5px]">
            <span className={label}>В выписке</span>
            <span className="min-w-0 flex-1 font-mono font-semibold text-tx">{row.merchant_title}</span>
          </div>
        )}
        {hits.length > 0 && (
          <div className="flex gap-2 text-[11.5px]">
            <span className={label}>Похожие</span>
            <span className="min-w-0 flex-1 space-y-0.5">
              {hits.map((h) => (
                <button
                  key={h.id}
                  type="button"
                  className="block max-w-full truncate text-left font-semibold text-gold"
                  onClick={() => navigate(`/pos?mcc=${h.mcc}&merchant=${encodeURIComponent(h.name)}&pos=${h.id}`)}
                >
                  «{h.name}» · MCC {h.mcc} — открыть
                </button>
              ))}
            </span>
          </div>
        )}
      </div>
      <div className="mt-3 flex gap-2">
        <button
          type="button"
          disabled={busy}
          onClick={onApprove}
          className="flex-1 rounded-xl border border-mint/50 bg-mint/15 py-2.5 text-[12.5px] font-bold text-mint transition active:scale-[.98] disabled:opacity-40"
        >
          Одобрить
        </button>
        <button
          type="button"
          disabled={busy}
          onClick={onReject}
          className="flex-1 rounded-xl border border-warn/40 bg-warn/10 py-2.5 text-[12.5px] font-bold text-warn transition active:scale-[.98] disabled:opacity-40"
        >
          Отклонить
        </button>
      </div>
    </div>
  );
}

// The 1c sheet: reject with an optional operator note. Doubles as 1d's
// «Снять» confirmation — same endpoint, same note semantics.
function RejectSheet({ row, published, busy, onConfirm, onClose }: {
  row: Row;
  published: boolean;
  busy: boolean;
  onConfirm: (note: string) => void;
  onClose: () => void;
}) {
  const [note, setNote] = useState("");
  return (
    <Sheet onClose={onClose} sid="MD-01.d">
      <p className="text-lg font-extrabold tracking-tight">
        {published ? `Снять «${row.name}» из каталога?` : `Отклонить «${row.name}»?`}
      </p>
      <p className="mt-1.5 text-xs leading-relaxed font-medium text-tx3">
        Точка {published ? "перестанет отвечать поиску" : "не попадёт в каталог"}. Заявка сохранится — повторная с тем же
        именем и MCC будет поймана как дубль.
      </p>
      <p className="mx-0.5 mt-3.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">
        Причина <span className="font-semibold tracking-normal normal-case text-tx4">— не обязательно</span>
      </p>
      <textarea
        className="mt-1.5 min-h-[66px] w-full rounded-xl border border-brd2 bg-srf2 px-3 py-2.5 text-sm font-medium text-tx placeholder:text-tx4 focus:border-acc focus:outline-none"
        maxLength={500}
        value={note}
        onChange={(e) => setNote(e.target.value)}
        placeholder="Опечатка в MCC, дубль, не существует…"
      />
      <p className="mx-0.5 mt-1 text-[10.5px] font-medium text-tx4">Заметка видна только оператору. Автор заявки её не получит.</p>
      <div className="mt-3.5 flex gap-2">
        <Btn variant="ghost" className="flex-1" onClick={onClose}>
          Отмена
        </Btn>
        {/* The confirm carries the destructive tone from the theme, not a
            pair of literal pinks: those two hex values were the only
            hardcoded colours in the SPA and stayed the same in both themes. */}
        <Btn variant="danger" className="flex-1" disabled={busy} onClick={() => onConfirm(note.trim())}>
          {published ? "Снять" : "Отклонить"}
        </Btn>
      </div>
    </Sheet>
  );
}

export default function Moderation() {
  const [tab, setTab] = useState<Tab>("pending");
  const [expanded, setExpanded] = useState<string | null>(null);
  const [rejecting, setRejecting] = useState<Row | null>(null);
  const qc = useQueryClient();
  const me = useMe();

  const list = useQuery({
    queryKey: ["moderation", tab],
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/moderation/pos", { params: { query: { state: tab, limit: 100 } } })),
  });
  const pendingCount = useQuery({
    queryKey: ["moderation", "badge"],
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/moderation/pos", { params: { query: { state: "pending", limit: 1 } } })),
    staleTime: 30_000,
  });
  const act = useMutation({
    mutationFn: async ({ id, verb, note }: { id: string; verb: "approve" | "reject"; note?: string }) =>
      unwrap(
        verb === "approve"
          ? await api.POST("/api/v1/moderation/pos/{id}/approve", { params: { path: { id } } })
          : await api.POST("/api/v1/moderation/pos/{id}/reject", {
              params: { path: { id } },
              body: note ? { note } : {},
            }),
      ),
    onSuccess: () => {
      setRejecting(null);
      qc.invalidateQueries({ queryKey: ["moderation"] });
    },
  });

  const items = list.data?.items ?? [];
  const queueTotal = pendingCount.data?.total ?? 0;
  // The top card opens itself (1b: «верхняя карточка раскрыта») unless the
  // moderator collapsed or opened another.
  const openID = tab === "pending" ? (expanded ?? items[0]?.id ?? null) : null;

  // Collapsed meta, the boards' register: MCC · категория · тип · хвост.
  const metaLine = (p: Row) =>
    [
      p.mcc && `MCC ${p.mcc}`,
      p.mcc_name,
      p.type && `${TYPE_EMOJI[p.type]} ${TYPE_LABEL[p.type]}`,
      p.address,
    ]
      .filter(Boolean)
      .join(" · ");

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/services" />
        <h1 className="min-w-0 flex-1 truncate text-xl font-extrabold tracking-tight">Модерация</h1>
        {me.data && me.data.role !== "user" && (
          <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[10.5px] font-semibold text-tx3">
            {me.data.role === "admin" ? "админ" : "модератор"}
          </span>
        )}
      </div>

      <SegTabs
        sid="MD-01.a"
        value={tab}
        onChange={(t) => {
          setTab(t);
          setExpanded(null);
        }}
        options={[
          { value: "pending", label: queueTotal > 0 ? `Очередь · ${queueTotal}` : "Очередь" },
          { value: "published", label: "Опубликованные" },
        ]}
      />

      {list.isPending && <Spinner />}
      <ErrMsg error={list.error} />

      {/* 1e: the empty queue is the normal state of a small base. */}
      {list.data && items.length === 0 && tab === "pending" && (
        <div className="flex flex-col items-center gap-2.5 rounded-2xl border border-brd bg-srf px-5 py-7 text-center" data-sid="MD-01.e">
          <span className="flex h-11 w-11 items-center justify-center rounded-[14px] bg-inset text-xl">🛡️</span>
          <p className="text-sm font-bold">Очередь пуста</p>
          <p className="text-[11.5px] leading-relaxed font-medium text-tx3">
            Новые ручные точки появятся здесь до публикации. Строки из транзакций публикуются сразу — их видно во
            вкладке «Опубликованные».
          </p>
          <button type="button" className="text-[12.5px] font-bold text-accl" onClick={() => setTab("published")}>
            К опубликованным
          </button>
        </div>
      )}
      {list.data && items.length === 0 && tab === "published" && <Empty>Опубликованных точек пока нет.</Empty>}

      {items.length > 0 && tab === "pending" && (
        <div className="space-y-2.5" data-sid="MD-01.b">
          {items.map((p) =>
            p.id === openID ? (
              <ExpandedCard
                key={p.id}
                row={p}
                busy={act.isPending}
                onApprove={() => act.mutate({ id: p.id, verb: "approve" })}
                onReject={() => setRejecting(p)}
              />
            ) : (
              <button
                key={p.id}
                type="button"
                onClick={() => setExpanded(p.id)}
                className="block w-full rounded-[18px] border border-brd bg-srf px-3.5 py-3 text-left"
              >
                <span className="flex items-center gap-2">
                  <span className="min-w-0 flex-1 truncate text-sm font-bold">{p.name}</span>
                  <span className="flex-none text-[10.5px] font-medium text-tx4">{fmtDay(p.created_at)}</span>
                </span>
                <span className="mt-1 block truncate text-[11px] font-medium text-tx3">{metaLine(p)}</span>
              </button>
            ),
          )}
          <p className="mx-0.5 text-[10.5px] leading-relaxed font-medium text-tx4">
            Кто предложил точку — не видно и не запрашивается: submitted_by остаётся в базе только для оператора.
            Одобренная точка сразу отвечает поиску, отклонённая хранится для дедупа повторных заявок.
          </p>
        </div>
      )}

      {items.length > 0 && tab === "published" && (
        <div className="space-y-2.5" data-sid="MD-01.f">
          {items.map((p) => (
            <div key={p.id} className="flex items-center gap-2 rounded-[18px] border border-brd bg-srf px-3.5 py-3">
              <div className="min-w-0 flex-1">
                <p className="truncate text-sm font-bold">{p.name}</p>
                <p className="mt-0.5 truncate text-[11px] font-medium text-tx3">
                  {[
                    p.mcc && `MCC ${p.mcc}`,
                    p.mcc_name,
                    p.origin === "user_manual" && p.moderated_at ? `одобрена ${fmtDay(p.moderated_at)}` : fmtDay(p.created_at),
                  ]
                    .filter(Boolean)
                    .join(" · ")}
                </p>
              </div>
              <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[10px] font-semibold text-tx3">
                {ORIGIN_LABEL[p.origin] ?? p.origin}
              </span>
              <button
                type="button"
                disabled={act.isPending}
                onClick={() => setRejecting(p)}
                className="flex-none rounded-[9px] border border-warn/40 px-2.5 py-1.5 text-[11px] font-bold text-warn disabled:opacity-40"
              >
                Снять
              </button>
            </div>
          ))}
          <p className="mx-0.5 text-[10.5px] leading-relaxed font-medium text-tx4">
            Строки скрейпа mcc-codes.ru в поток не попадают — здесь только то, что написали люди: вручную, из
            транзакций, админом.
          </p>
        </div>
      )}
      <ErrMsg error={act.error} />

      {rejecting && (
        <RejectSheet
          row={rejecting}
          published={tab === "published"}
          busy={act.isPending}
          onConfirm={(note) => act.mutate({ id: rejecting.id, verb: "reject", note: note || undefined })}
          onClose={() => setRejecting(null)}
        />
      )}
    </>
  );
}
