import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { Btn, ErrMsg, Pct, Spinner } from "./ui";
import { Sheet } from "./Sheet";
import { fmtDate } from "../lib";

// Партнёрки, extracted from CB-09 so the bank period screen (CB-03) can
// carry the same chips + sheet — the offers' home stays the bank card, but
// «где мои партнёрки, как их править?» must be answerable from the menu
// screen too (2026-08-07 feedback).

// The minimal chip shape both feeds satisfy: the overview's per-client chip
// DTO and the full partner-offer DTO.
export type PartnerChipOffer = {
  id: number;
  merchant_title: string;
  percent?: string;
  valid_to?: string;
  status?: string;
};

// Alive offers as gold chips, past ones folded behind a count (3c).
export function PartnerChips({ offers, onOpen }: { offers: PartnerChipOffer[]; onOpen: (id: number) => void }) {
  const [showPast, setShowPast] = useState(false);
  if (offers.length === 0) return null;
  const alive = offers.filter((p) => p.status === "active" || p.status === "scheduled");
  const past = offers.filter((p) => p.status === "ended" || p.status === "expired");
  const chip = (p: PartnerChipOffer, muted = false) => (
    <button
      key={p.id}
      type="button"
      onClick={(e) => {
        e.stopPropagation();
        onOpen(p.id);
      }}
      className={`rounded-lg border px-2 py-1 text-[10.5px] font-semibold whitespace-nowrap ${
        muted ? "border-brd2 text-tx4" : "border-gold/30 bg-gold/10 text-gold"
      }`}
    >
      ★ {p.merchant_title}
      {p.percent != null && ` ${p.percent}%`}
      {!muted && p.valid_to && ` · по ${fmtDate(p.valid_to)}`}
      {muted && ` · ${p.status === "ended" ? "завершена" : "истекла"}`}
    </button>
  );
  return (
    <div className="mt-1.5 flex flex-wrap gap-1.5">
      {alive.map((p) => chip(p))}
      {past.length > 0 && (
        <button
          type="button"
          onClick={(e) => {
            e.stopPropagation();
            setShowPast(!showPast);
          }}
          className="rounded-lg border border-dashed border-dash px-2 py-1 text-[10.5px] font-semibold text-tx4"
        >
          прошедшие · {past.length} {showPast ? "▲" : "▼"}
        </button>
      )}
      {showPast && past.map((p) => chip(p, true))}
    </div>
  );
}

// The expanded партнёрка card (3c): scope, term, limit, activation — with
// «Завершить» as an undoable event and edit/delete a screen away.
export function PartnerSheet({ id, onClose }: { id: number; onClose: () => void }) {
  const qc = useQueryClient();
  const navigate = useNavigate();
  const offer = useQuery({
    queryKey: ["partner-offer", String(id)],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/partner-offers/{id}", { params: { path: { id } } })),
  });
  const invalidate = () => {
    qc.invalidateQueries({ queryKey: ["overview"] });
    qc.invalidateQueries({ queryKey: ["partner-offer"] });
    qc.invalidateQueries({ queryKey: ["partner-offers"] });
    qc.invalidateQueries({ queryKey: ["lookup"] });
  };
  const end = useMutation({
    mutationFn: async () =>
      unwrap(await api.POST("/api/v1/cashback/partner-offers/{id}/end", { params: { path: { id } } })),
    onSuccess: invalidate,
  });
  const reopen = useMutation({
    mutationFn: async () =>
      unwrap(await api.POST("/api/v1/cashback/partner-offers/{id}/reopen", { params: { path: { id } } })),
    onSuccess: invalidate,
  });
  const remove = useMutation({
    mutationFn: async () =>
      unwrap(await api.DELETE("/api/v1/cashback/partner-offers/{id}", { params: { path: { id } } })),
    onSuccess: () => {
      invalidate();
      onClose();
    },
  });

  const o = offer.data;
  const daysLeft =
    o?.valid_to && o.status === "active"
      ? Math.max(0, Math.ceil((new Date(o.valid_to).getTime() - Date.now()) / 86_400_000))
      : null;
  const unit = o?.currency_kind === "points" ? o.points_label || "баллов" : "₽";
  const scopeText =
    o?.scope_kind === "category"
      ? `категория · ${o.canonical_title_ru ?? "—"}`
      : `магазин · ${o?.merchant_title ?? ""}${o?.canonical_title_ru ? ` (канон: ${o.canonical_title_ru})` : ""}`;

  return (
    <Sheet onClose={onClose} sid="CB-09.e">
      {offer.isPending && <Spinner />}
      {offer.isError && <ErrMsg error={offer.error} />}
      {o && (
        <div className="space-y-3 pb-1">
          <div className="flex items-start gap-2.5">
            <span className="flex h-[30px] w-[30px] flex-none items-center justify-center rounded-[9px] bg-gold/15 text-sm font-extrabold text-gold">★</span>
            <div className="min-w-0 flex-1">
              <p className="text-sm leading-tight font-bold text-gold">{o.merchant_title}</p>
              <p className="mt-0.5 text-[10.5px] font-medium text-tx4">
                {o.bank_name}
                {o.holder_label ? ` · ${o.holder_label}` : o.bank_client_id == null ? " · весь банк" : ""}
                {" · "}
                {o.status === "active" ? "действует" : o.status === "scheduled" ? "ещё не началась" : o.status === "ended" ? "завершена" : "истекла"}
              </p>
            </div>
            <Pct percent={o.percent} currency={o.currency_kind ?? "unknown"} className="text-lg" />
          </div>
          <dl className="space-y-1.5">
            {(
              [
                ["Где действует", scopeText],
                [
                  "Срок",
                  [
                    o.valid_from && `с ${fmtDate(o.valid_from)}`,
                    o.valid_to ? `по ${fmtDate(o.valid_to)}` : "бессрочно",
                    daysLeft != null && `осталось ${daysLeft} дн.`,
                  ]
                    .filter(Boolean)
                    .join(" · "),
                ],
                ["Лимит", [o.cap_value && `${o.cap_value} ${unit}`, o.min_amount && `покупка от ${o.min_amount} ₽`].filter(Boolean).join(" · ") || "—"],
                [
                  "Активация",
                  !o.requires_activation ? "не требуется" : o.activated_at != null ? "активировано ✓" : "требует активации в банке",
                ],
                ...(o.notes ? ([["Заметки", o.notes]] as const) : []),
              ] as const
            ).map(([label, value]) => (
              <div key={label} className="flex items-baseline gap-2">
                <dt className="w-[88px] flex-none text-[10px] font-medium tracking-[.06em] text-tx4 uppercase">{label}</dt>
                <dd className={`min-w-0 flex-1 text-[12.5px] font-semibold ${label === "Активация" && o.requires_activation && o.activated_at == null ? "text-warn" : "text-tx2"}`}>
                  {value}
                </dd>
              </div>
            ))}
          </dl>
          <p className="text-[10px] leading-snug font-medium text-tx4">
            Не занимает слот и не трогает лимит месяца — но ранжируется в ленте, поиске и точке продаж.
          </p>
          <div className="flex gap-2">
            <Btn variant="soft" className="flex-1" onClick={() => navigate(`/partners/new?id=${o.id}`)}>
              Редактировать
            </Btn>
            {o.status === "ended" ? (
              <Btn variant="ghost" className="flex-1" disabled={reopen.isPending} onClick={() => reopen.mutate()}>
                Вернуть
              </Btn>
            ) : (
              <Btn variant="ghost" className="flex-1" disabled={end.isPending} onClick={() => end.mutate()}>
                Завершить
              </Btn>
            )}
            <Btn
              variant="danger"
              disabled={remove.isPending}
              onClick={() => {
                if (window.confirm(`Удалить партнёрку «${o.merchant_title}» насовсем? «Завершить» мягче — её можно вернуть.`)) remove.mutate();
              }}
            >
              🗑
            </Btn>
          </div>
          <ErrMsg error={end.error ?? reopen.error ?? remove.error} />
        </div>
      )}
    </Sheet>
  );
}
