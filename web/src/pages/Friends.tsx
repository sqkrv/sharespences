import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api, unwrap, type FriendCashback, type FriendOffer, type FriendSharedClient } from "../api/client";
import { BankBadge, Btn, Card, Empty, ErrMsg, Pct, Spinner } from "../components/ui";
import { currencyBadge, fmtRange } from "../lib";

// CB-06 «Кешбек друзей» (docs/specs/friends-sharing.md FR-S3): a read-only
// window into each friend's shared clients — selected chips, gold chips for
// granted барабан/спец rows, the unselected menu collapsed. Caps are absent
// by API shape (invariant 4), so there is no ProgressRing here on purpose.

function OfferChip({ o, gold = false }: { o: FriendOffer; gold?: boolean }) {
  return (
    <span
      className={`inline-flex items-center gap-1.5 rounded-xl border px-2.5 py-1.5 text-[12px] font-semibold ${
        gold ? "border-gold/30 bg-gold/5" : "border-brd bg-srf2"
      }`}
    >
      {o.raw_title}
      {gold && (
        <span className="rounded bg-gold/10 px-1 py-[1px] text-[9px] font-bold text-gold">
          {o.kind === "super" ? "барабан" : "спец"}
        </span>
      )}
      <Pct percent={o.percent} currency={o.currency_kind} className="text-[12px]" />
    </span>
  );
}

function ClientCard({ c }: { c: FriendSharedClient }) {
  const [menuOpen, setMenuOpen] = useState(false);
  const menu = c.menu ?? [];
  const selected = c.selected ?? [];
  const granted = c.granted ?? [];
  return (
    <div className="space-y-2 rounded-xl border border-brd2 bg-srf2/50 p-3">
      <div className="flex items-center gap-2.5">
        <BankBadge name={c.bank_name} size={26} />
        <div className="min-w-0 flex-1">
          <p className="text-[13px] font-bold">
            {c.bank_name}
            {c.holder_label && <span className="font-medium text-tx4"> · {c.holder_label}</span>}
          </p>
          <p className="text-[10px] font-medium text-tx4">
            {c.period_start && c.period_end ? fmtRange(c.period_start, c.period_end) : "нет периода на эту дату"}
          </p>
        </div>
      </div>
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
        c.period_start && <p className="text-[11px] font-medium text-tx4">Категории пока не выбраны</p>
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

function FriendCard({ f }: { f: FriendCashback }) {
  const clients = f.clients ?? [];
  return (
    <Card className="space-y-2.5 p-3.5" data-sid="CB-06.a">
      <p className="text-sm font-bold">
        {f.display_name} <span className="font-medium text-tx4">@{f.username}</span>
      </p>
      {clients.length > 0 ? (
        clients.map((c) => <ClientCard key={c.bank_client_id} c={c} />)
      ) : (
        <p className="text-[11.5px] font-medium text-tx4">Пока ничего не расшарено тебе</p>
      )}
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
      <div className="flex items-center justify-between">
        <h1 className="text-[22px] font-extrabold tracking-tight">Кешбек друзей</h1>
        <Link to="/friends/settings">
          <Btn variant="ghost" className="!px-3 !py-1.5 text-xs">
            Друзья и шэринг
          </Btn>
        </Link>
      </div>

      {friends.isPending && <Spinner />}
      <ErrMsg error={friends.error} />

      {friends.data &&
        ((friends.data.friends ?? []).length > 0 ? (
          <div className="space-y-2.5" data-sid="CB-06.b">
            {(friends.data.friends ?? []).map((f) => (
              <FriendCard key={f.user_id} f={f} />
            ))}
          </div>
        ) : (
          <div className="space-y-3">
            <Empty>Друзей пока нет</Empty>
            <Card className="space-y-2 p-4 text-[12px] font-medium text-tx3">
              <p>
                Друзья видят выбранные категории друг друга — и «Какой картой платить?» сможет ответить «картой
                друга». По умолчанию ничего не расшарено: каждый сам отмечает, какие банки видны кому.
              </p>
              <Link to="/friends/settings" className="block">
                <Btn variant="soft" className="w-full">
                  Добавить друга
                </Btn>
              </Link>
            </Card>
          </div>
        ))}
    </>
  );
}
