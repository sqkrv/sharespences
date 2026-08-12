import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { useNavigate, useSearchParams } from "react-router-dom";
import { ApiError, api, unwrap } from "../api/client";
import { useCategories } from "../hooks";
import { BackButton, Btn, Card, ErrMsg, Field, Input } from "../components/ui";
import { FALLBACK_EMOJI } from "../lib";

// CB-13 «Новая точка» (redesign 5e): the zero-results tail of «Поиск», по
// модели mcc-codes — the MCC is the main field, the category is derived
// from the code, never picked. The name arrives from the query; the
// name+MCC pair nets duplicates and offers to open the existing точка
// instead of creating a copy. The form's tail depends on the payment type.
// A saved точка is pending until moderated; its author sees it in search
// right away, the общий каталог after approval.

const TYPES = [
  ["offline", "📍 Офлайн"],
  ["online", "🌐 Онлайн"],
  ["app", "📱 Приложение"],
  ["other", "✏️ Другое"],
] as const;
type PosType = (typeof TYPES)[number][0];

const TAIL: Record<PosType, { label: string; placeholder: string }> = {
  offline: { label: "Адрес оплаты", placeholder: "город, улица и дом" },
  online: { label: "Адрес сайта", placeholder: "https://…" },
  app: { label: "Название приложения", placeholder: "как в магазине приложений" },
  other: { label: "Описание", placeholder: "терминал у курьера, в транспорте…" },
};

export default function PosNew() {
  const [params] = useSearchParams();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const categories = useCategories();
  const [mcc, setMcc] = useState("");
  const [name, setName] = useState(params.get("query") ?? "");
  const [merchant, setMerchant] = useState("");
  const [type, setType] = useState<PosType>("offline");
  const [tail, setTail] = useState("");

  const codeReady = /^\d{3,4}$/.test(mcc);
  const resolve = useQuery({
    queryKey: ["mcc-resolve", mcc],
    enabled: codeReady,
    retry: false,
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/resolve", { params: { query: { code: mcc } } })),
  });
  const unknownCode = resolve.isError && resolve.error instanceof ApiError && resolve.error.status === 404;
  const canonRef = (resolve.data?.canonicals ?? [])[0];
  const canon = canonRef && (categories.data ?? []).find((c) => c.slug === canonRef.slug);

  // The duplicate net: same MCC + similar name → open the existing карточка,
  // считать will the same one.
  const similar = useQuery({
    queryKey: ["pos-similar", mcc, name.trim()],
    enabled: codeReady && name.trim().length >= 2,
    queryFn: async () =>
      unwrap(
        await api.GET("/api/v1/mcc/points-of-sale/similar", {
          params: { query: { mcc, name: name.trim() } },
        }),
      ) ?? [],
  });
  const dupe = (similar.data ?? [])[0];

  const save = useMutation({
    mutationFn: async () =>
      unwrap(
        await api.POST("/api/v1/mcc/points-of-sale", {
          body: {
            mcc,
            name: name.trim(),
            ...(merchant.trim() ? { merchant_title: merchant.trim() } : {}),
            type,
            ...(tail.trim() ? { address: tail.trim() } : {}),
          },
        }),
      ),
    onSuccess: (p) => {
      qc.invalidateQueries({ queryKey: ["pos-similar"] });
      // The saved точка opens as CB-11 — расчёт карт живёт там.
      navigate(`/pos?mcc=${p.mcc}&merchant=${encodeURIComponent(p.name)}`, { replace: true });
    },
  });

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/search" />
        <h1 className="min-w-0 flex-1 truncate text-xl font-extrabold tracking-tight">Новая точка</h1>
        <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[10.5px] font-semibold text-tx3">точка продаж</span>
      </div>

      <Card className="p-4" data-sid="CB-13.a">
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <Field label="MCC">
            <Input
              required
              inputMode="numeric"
              pattern="\d{3,4}"
              maxLength={4}
              title="3–4 цифры"
              value={mcc}
              onChange={(e) => setMcc(e.target.value.trim())}
              placeholder="5462"
            />
          </Field>
          <p className="text-[10px] font-medium text-tx4">4 цифры · из истории транзакций или чека</p>
          {resolve.data && (
            <div className="flex items-center gap-2 rounded-xl border border-mint/25 bg-mint/5 px-3 py-2">
              <span className="min-w-0 flex-1 text-[12px] font-semibold text-tx2">
                {resolve.data.code.name}
                {canon && (
                  <span className="font-medium text-tx4"> · {canon.emoji || FALLBACK_EMOJI} {canon.title_ru}</span>
                )}
              </span>
              <span className="flex-none text-[10.5px] font-bold text-mint">код узнан</span>
            </div>
          )}
          {unknownCode && (
            <p className="rounded-xl bg-warn/10 px-3 py-2 text-[11.5px] font-medium text-warn">
              Кода {mcc} нет в справочнике — проверь цифры по чеку или выписке.
            </p>
          )}

          <Field label="Название точки">
            <Input required minLength={2} value={name} onChange={(e) => setName(e.target.value)} placeholder="Пекарня Хлебник" />
          </Field>

          {dupe && (
            <div className="space-y-1.5 rounded-xl border border-gold/30 bg-gold/5 p-3" data-sid="CB-13.b">
              <div className="flex items-center gap-2">
                <span className="min-w-0 flex-1 text-[12.5px] font-semibold text-gold">
                  Похожая точка уже есть
                  <span className="block text-[11px] font-medium text-tx3">«{dupe.name}» · MCC {dupe.mcc}</span>
                </span>
                <Btn
                  type="button"
                  variant="soft"
                  className="!px-2.5 !py-1.5 text-xs"
                  onClick={() => navigate(`/pos?mcc=${dupe.mcc}&merchant=${encodeURIComponent(dupe.name)}`)}
                >
                  Открыть
                </Btn>
              </div>
              <p className="text-[10px] font-medium text-tx4">Если это она — не создавай копию: считать будет та же карточка.</p>
            </div>
          )}

          <Field label="Мерчант в выписке — не обязательно">
            <Input
              value={merchant}
              onChange={(e) => setMerchant(e.target.value)}
              placeholder="HLEBNIK PEKARNYA"
              autoCapitalize="characters"
              spellCheck={false}
            />
          </Field>
          <p className="text-[10px] font-medium text-tx4">Латиницей, как в выписке или SMS — по нему точку найдут другие.</p>

          <span className="inline-flex flex-wrap overflow-hidden rounded-lg border border-brd2">
            {TYPES.map(([t, label]) => (
              <button
                key={t}
                type="button"
                onClick={() => setType(t)}
                className={`px-2.5 py-1.5 text-[11px] font-semibold ${type === t ? "grad-acc text-white" : "bg-srf2 text-tx3"}`}
              >
                {label}
              </button>
            ))}
          </span>

          <Field label={TAIL[type].label}>
            <Input value={tail} onChange={(e) => setTail(e.target.value)} placeholder={TAIL[type].placeholder} />
          </Field>
          {type === "offline" && (
            <p className="text-[10px] font-medium text-tx4">Если терминал у курьера или в транспорте — выбери «Другое» и так и напиши.</p>
          )}

          <Btn type="submit" className="w-full" disabled={save.isPending || !codeReady || unknownCode}>
            Сохранить точку
          </Btn>
          <p className="text-center text-[10px] font-medium text-tx4">
            После модерации точка попадёт в общий каталог — её найдут все.
          </p>
          <ErrMsg error={save.error} />
        </form>
      </Card>
    </>
  );
}
