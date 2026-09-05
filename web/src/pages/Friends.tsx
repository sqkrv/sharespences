import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api, unwrap, type FriendCashback, type FriendOffer, type FriendPeriod, type FriendSharedClient } from "../api/client";
import { BackButton, BankBadge, Card, Empty, ErrMsg, Pct, Spinner } from "../components/ui";
import { FALLBACK_EMOJI, coversToday, currencyBadge, fmtRange } from "../lib";

// CB-06 «Кешбек друзей» v2 (redesign 4a): each friend's picture in chips
// tinted by the currency legend — мята рубли, сирень баллы, золото спец —
// without limits and without history (invariants 4 and 8). The unselected
// menu stays collapsed; caps are absent by API shape, so no ProgressRing.

// The 4a chip: its wash is the currency legend, so «чем платят» reads
// before the числа do.
function OfferChip({ o, gold = false }: { o: FriendOffer; gold?: boolean }) {
  const wash = gold
    ? "border border-gold/30 bg-gold/10"
    : o.currency_kind === "points"
      ? "bg-accl/10"
      : "bg-mint/10";
  return (
    <span className={`inline-flex items-center gap-1.5 rounded-lg px-2 py-1 text-[11.5px] font-semibold text-tx2 ${wash}`}>
      <span className="text-[12px] leading-none">{o.emoji || FALLBACK_EMOJI}</span>
      {o.raw_title}
      {gold && (
        <span className="rounded bg-gold/10 px-1 py-[1px] text-[9px] font-bold text-gold">
          {o.kind === "super" ? "барабан" : "спец"}
        </span>
      )}
      <Pct percent={o.percent} currency={o.currency_kind} className="text-[11.5px]" />
    </span>
  );
}

// Monogram avatar — the friend's first letter on a soft accent tile.
function Monogram({ name }: { name: string }) {
  return (
    <span className="flex h-[34px] w-[34px] flex-none items-center justify-center rounded-xl bg-acc/15 text-sm font-extrabold text-accl">
      {(name.trim()[0] ?? "?").toUpperCase()}
    </span>
  );
}

// PeriodBlock renders one period of the shared window: chips + the
// collapsible unselected menu. Друзья see the current period and next
// month's, nothing further (invariant 8) — the future one is marked.
function PeriodBlock({ p }: { p: FriendPeriod }) {
  const [menuOpen, setMenuOpen] = useState(false);
  const menu = p.menu ?? [];
  const selected = p.selected ?? [];
  const granted = p.granted ?? [];
  const current = coversToday(p.period_start, p.period_end);
  return (
    <div className="space-y-1.5">
      <p className="text-[10px] font-medium text-tx4">
        {fmtRange(p.period_start, p.period_end)}
        {!current && (
          <span className="ml-1.5 rounded bg-acc/15 px-1 py-[1px] text-[9px] font-bold text-accl">следующий</span>
        )}
      </p>
      {selected.length + granted.length > 0 ? (
        <div className="flex flex-wrap gap-1.5">
          {selected.map((o, i) => (
            <OfferChip key={`s${i}`} o={o} />
          ))}
          {granted.map((o, i) => (
            <OfferChip key={`g${i}`} o={o} gold />
          ))}
        </div>
      ) : (
        <p className="text-[11px] font-medium text-tx4">Категории пока не выбраны</p>
      )}
      {menu.length > 0 && (
        <div>
          <button
            type="button"
            onClick={() => setMenuOpen((v) => !v)}
            className="text-[11px] font-semibold text-accl"
          >
            {menuOpen ? "Скрыть меню" : `Всё меню · ещё ${menu.length}`}
          </button>
          {menuOpen && (
            <div className="mt-1.5 space-y-1">
              {menu.map((o, i) => (
                <div key={i} className="flex items-center justify-between rounded-lg bg-inset px-2.5 py-1.5">
                  <span className="min-w-0 flex-1 truncate text-[12px] font-medium text-tx2">
                    <span className="mr-1">{o.emoji || FALLBACK_EMOJI}</span>
                    {o.raw_title}
                    <span className="ml-1 text-[9.5px] text-tx4">{currencyBadge(o.currency_kind, o.points_label)}</span>
                  </span>
                  <Pct percent={o.percent} currency={o.currency_kind} className="text-[12px]" />
                </div>
              ))}
            </div>
          )}
        </div>
      )}
    </div>
  );
}

function ClientCard({ c }: { c: FriendSharedClient }) {
  const periods = c.periods ?? [];
  return (
    <div className="space-y-2 rounded-xl border border-brd2 bg-srf2/50 p-3">
      <div className="flex items-center gap-2.5">
        <BankBadge name={c.bank_name} size={26} />
        <p className="min-w-0 flex-1 text-[13px] font-bold">
          {c.bank_name}
          {c.holder_label && <span className="font-medium text-tx4"> · {c.holder_label}</span>}
        </p>
      </div>
      {periods.length > 0 ? (
        periods.map((p, i) => <PeriodBlock key={i} p={p} />)
      ) : (
        <p className="text-[11px] font-medium text-tx4">Периоды пока не внесены</p>
      )}
    </div>
  );
}

function FriendCard({ f }: { f: FriendCashback }) {
  const clients = f.clients ?? [];
  const empty = clients.length === 0;
  return (
    <Card className={`space-y-2.5 p-3.5 ${empty ? "border-dashed border-dash bg-srf/50" : ""}`} data-sid="CB-06.a">
      <div className="flex items-center gap-2.5">
        <Monogram name={f.display_name} />
        <div className="min-w-0 flex-1">
          <p className={`text-sm font-bold ${empty ? "text-tx3" : ""}`}>{f.display_name}</p>
          <p className="text-[10.5px] font-medium text-tx4">
            @{f.username} ·{" "}
            {empty
              ? "ничего не расшарено — так по умолчанию"
              : `${clients.length} ${clients.length === 1 ? "клиент" : clients.length < 5 ? "клиента" : "клиентов"}`}
          </p>
        </div>
      </div>
      {clients.map((c) => (
        <ClientCard key={c.bank_client_id} c={c} />
      ))}
    </Card>
  );
}

export default function Friends() {
  const friends = useQuery({
    queryKey: ["cashback-friends"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/friends")),
  });

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton />
        <h1 className="min-w-0 flex-1 text-xl font-extrabold tracking-tight">Кешбек друзей</h1>
        <Link
          to="/friends/settings"
          title="Друзья и шэринг"
          className="flex h-[33px] w-[33px] flex-none items-center justify-center rounded-[11px] bg-inset"
        >
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="var(--t-accl)" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
            <circle cx="12" cy="12" r="3.2" />
            <path d="M19 12a7 7 0 0 0-.14-1.4l2-1.55-2-3.46-2.36.95a7 7 0 0 0-2.42-1.4L13.7 2.6h-3.4l-.38 2.54a7 7 0 0 0-2.42 1.4l-2.36-.95-2 3.46 2 1.55A7 7 0 0 0 5 12c0 .48.05.94.14 1.4l-2 1.55 2 3.46 2.36-.95a7 7 0 0 0 2.42 1.4l.38 2.54h3.4l.38-2.54a7 7 0 0 0 2.42-1.4l2.36.95 2-3.46-2-1.55c.09-.46.14-.92.14-1.4Z" />
          </svg>
        </Link>
      </div>

      {/* Invariant 8 in one line: today through the end of next month —
          no history, and never the friend's limits (invariant 4). */}
      <div className="flex items-center gap-2 rounded-xl border border-acc/25 bg-acc/10 px-3 py-2">
        <span className="h-1.5 w-1.5 flex-none rounded-full bg-acc" />
        <span className="text-[11px] font-medium text-tx2">Видно текущий и следующий месяц — без истории и без лимитов</span>
      </div>

      {friends.isPending && <Spinner />}
      <ErrMsg error={friends.error} />

      {friends.data &&
        ((friends.data.friends ?? []).length > 0 ? (
          <>
            <div className="space-y-2.5" data-sid="CB-06.b">
              {(friends.data.friends ?? []).map((f) => (
                <FriendCard key={f.user_id} f={f} />
              ))}
            </div>
            <p className="mx-0.5 text-[10.5px] leading-snug font-medium text-tx4">
              Карты друзей уже ранжируются в твоей ленте и точке продаж — чип «друг». При равном проценте своя карта выше.
            </p>
          </>
        ) : (
          <div className="space-y-3">
            <Empty>Друзей пока нет</Empty>
            <Card className="space-y-2 p-4 text-[12px] font-medium text-tx3">
              <p>
                Друзья видят выбранные категории друг друга — и лента сможет ответить «картой друга». По умолчанию ничего
                не расшарено: каждый сам отмечает, какие банки видны кому.
              </p>
              <Link to="/friends/settings" className="block">
                <span className="block w-full rounded-xl bg-acc/15 py-2 text-center text-sm font-semibold text-accl">Добавить друга</span>
              </Link>
            </Card>
          </div>
        ))}
    </>
  );
}
