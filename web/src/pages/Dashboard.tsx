import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { useBanks, useCards, usePeriods, usePrograms, useTierMap } from "../hooks";
import { Btn, Empty, ErrMsg, Field, Input, Section, Select, Spinner } from "../components/ui";
import { coversToday, fmtRange } from "../lib";

const PAYMENT_SYSTEMS = ["mir", "visa", "mastercard", "unionpay", "american_express"] as const;
type PaySystem = (typeof PAYMENT_SYSTEMS)[number];

function AddCardForm({ onDone }: { onDone: () => void }) {
  const banks = useBanks();
  const programs = usePrograms();
  const tierMap = useTierMap();
  const qc = useQueryClient();
  const [bankID, setBankID] = useState("");
  const [tierID, setTierID] = useState("");
  const [last4, setLast4] = useState("");
  const [paySystem, setPaySystem] = useState<PaySystem>("mir");

  const program = (programs.data ?? []).find((p) => String(p.bank_id) === bankID);
  const tiers = program
    ? [...(tierMap.data?.values() ?? [])].filter((ti) => ti.program.id === program.id)
    : [];

  const create = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/api/v1/cards", {
          body: {
            bank_id: Number(bankID),
            last_4_digits: Number(last4),
            payment_system: paySystem,
            ...(tierID ? { program_tier_id: Number(tierID) } : {}),
          },
        }),
      ),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["cards"] });
      onDone();
    },
  });

  return (
    <form
      className="mt-3 space-y-3 border-t border-slate-100 pt-3"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      <Field label="Банк">
        <Select
          required
          value={bankID}
          onChange={(e) => {
            setBankID(e.target.value);
            setTierID("");
          }}
        >
          <option value="">— выберите банк —</option>
          {(banks.data ?? []).map((b) => (
            <option key={b.id} value={b.id}>
              {b.name}
            </option>
          ))}
        </Select>
      </Field>
      {tiers.length > 0 && (
        <Field label="Уровень (тариф КБ-программы)">
          <Select value={tierID} onChange={(e) => setTierID(e.target.value)}>
            <option value="">— не указан —</option>
            {tiers.map(({ tier }) => (
              <option key={tier.id} value={tier.id}>
                {tier.name}
                {tier.is_paid_subscription ? " (подписка)" : ""}
              </option>
            ))}
          </Select>
        </Field>
      )}
      <div className="grid grid-cols-2 gap-3">
        <Field label="Последние 4 цифры">
          <Input
            required
            inputMode="numeric"
            pattern="\d{4}"
            maxLength={4}
            value={last4}
            onChange={(e) => setLast4(e.target.value)}
            placeholder="1234"
          />
        </Field>
        <Field label="Платёжная система">
          <Select value={paySystem} onChange={(e) => setPaySystem(e.target.value as PaySystem)}>
            {PAYMENT_SYSTEMS.map((ps) => (
              <option key={ps} value={ps}>
                {ps}
              </option>
            ))}
          </Select>
        </Field>
      </div>
      <Btn type="submit" disabled={create.isPending}>
        Добавить карту
      </Btn>
      <ErrMsg error={create.error} />
    </form>
  );
}

export default function Dashboard() {
  const cards = useCards();
  const periods = usePeriods();
  const tierMap = useTierMap();
  const [adding, setAdding] = useState(false);

  if (cards.isPending) return <Spinner />;
  if (cards.isError) return <ErrMsg error={cards.error} />;

  return (
    <>
      <Section title="Мои карты">
        {cards.data.length === 0 && <Empty>Карт пока нет — добавьте первую.</Empty>}
        <ul className="space-y-3">
          {cards.data.map((card) => {
            const tierInfo = card.program_tier_id != null ? tierMap.data?.get(card.program_tier_id) : undefined;
            const current = (periods.data ?? []).find(
              (p) => p.card_id === card.id && coversToday(p.period_start, p.period_end),
            );
            return (
              <li key={card.id} className="rounded-lg border border-slate-200 p-3">
                <div className="flex items-center justify-between gap-2">
                  <div>
                    <p className="font-medium">
                      {card.bank_name} <span className="text-slate-400">··{String(card.last_4_digits).padStart(4, "0")}</span>
                    </p>
                    {tierInfo && (
                      <p className="text-xs text-slate-500">
                        {tierInfo.tier.name}
                        {tierInfo.tier.is_paid_subscription && " · подписка"}
                        {tierInfo.tier.max_categories != null && ` · ${tierInfo.tier.max_categories} кат.`}
                      </p>
                    )}
                  </div>
                  {current ? (
                    <Link to={`/periods/${current.id}`}>
                      <Btn variant="ghost">Период {fmtRange(current.period_start, current.period_end)}</Btn>
                    </Link>
                  ) : (
                    <Link to={`/periods/new?card=${card.id}`}>
                      <Btn variant="ghost">+ Новый период</Btn>
                    </Link>
                  )}
                </div>
              </li>
            );
          })}
        </ul>
        {adding ? (
          <AddCardForm onDone={() => setAdding(false)} />
        ) : (
          <Btn variant="ghost" className="mt-3" onClick={() => setAdding(true)}>
            + Добавить карту
          </Btn>
        )}
      </Section>

      <Section title="Все периоды">
        {(periods.data ?? []).length === 0 ? (
          <Empty>Периодов пока нет. Откройте период на карте — это шаг 1 ежемесячного ритуала.</Empty>
        ) : (
          <ul className="divide-y divide-slate-100">
            {(periods.data ?? []).map((p) => (
              <li key={p.id}>
                <Link to={`/periods/${p.id}`} className="flex justify-between py-2 text-sm hover:bg-slate-50">
                  <span>{p.bank_name}</span>
                  <span className="text-slate-500">{fmtRange(p.period_start, p.period_end)}</span>
                </Link>
              </li>
            ))}
          </ul>
        )}
      </Section>
    </>
  );
}
