// MD-01 Модерация (roles-moderation.md), actualized to the redesign3
// canvas language: заявка-rows per 4e (accent-bordered pending rows, the
// Принять/✕ action pair), точка anatomy per 2b (PosTypeIcon lead, name /
// mono merchant title / address, MCC in mono accl on the right), uppercase
// group labels with counters. The «Сервисы» card renders this route only
// for role ≠ user — navigation sugar; the server re-checks the role on
// every operation, so a demoted moderator's next click answers 403.
// The rows are ANONYMOUS: the API never carries the submitter.
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "../api/client";
import { BackButton, Btn, Empty, ErrMsg, PosTypeIcon, SegTabs, Spinner } from "../components/ui";

type Tab = "pending" | "published";

const ORIGIN_LABEL: Record<string, string> = {
  user_manual: "добавлена вручную",
  user_transaction: "из операции",
  admin: "оператор",
};

export default function Moderation() {
  const [tab, setTab] = useState<Tab>("pending");
  const qc = useQueryClient();
  const list = useQuery({
    queryKey: ["moderation", tab],
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/moderation/pos", { params: { query: { state: tab, limit: 100 } } })),
  });
  const act = useMutation({
    mutationFn: async ({ id, verb }: { id: string; verb: "approve" | "reject" }) =>
      unwrap(
        verb === "approve"
          ? await api.POST("/api/v1/moderation/pos/{id}/approve", { params: { path: { id } } })
          : await api.POST("/api/v1/moderation/pos/{id}/reject", { params: { path: { id } } }),
      ),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["moderation"] }),
  });

  const items = list.data?.items ?? [];
  const total = list.data?.total ?? 0;

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/services" />
        <h1 className="min-w-0 flex-1 truncate text-xl font-extrabold tracking-tight">Модерация</h1>
      </div>

      <SegTabs
        sid="MD-01.a"
        value={tab}
        onChange={(t) => setTab(t)}
        options={[
          { value: "pending", label: "Очередь" },
          { value: "published", label: "Опубликованные" },
        ]}
      />

      {list.isPending && <Spinner />}
      <ErrMsg error={list.error} />
      {list.data && items.length === 0 && (
        <Empty>{tab === "pending" ? "Очередь пуста — все заявки разобраны." : "Опубликованных точек пока нет."}</Empty>
      )}

      {items.length > 0 && (
        <div className="space-y-1.5" data-sid="MD-01.b">
          <p className="mx-0.5 pt-1 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">
            {tab === "pending" ? "Очередь" : "Опубликованные"} · {total}
          </p>
          {items.map((p) => (
            <div
              key={p.id}
              className={`flex items-start gap-2.5 rounded-2xl border bg-srf px-3 py-2.5 ${
                tab === "pending" ? "border-acc/35" : "border-brd"
              }`}
            >
              <PosTypeIcon type={p.type} />
              {/* Three fixed roles, one per line — the 2b точка anatomy —
                  then the meta line and the action pair. */}
              <div className="min-w-0 flex-1 space-y-[3px]">
                <p className="truncate text-[12.5px] leading-tight font-semibold text-tx2">{p.name}</p>
                {p.merchant_title && (
                  <p className="truncate font-mono text-[10px] leading-tight font-semibold tracking-wide text-tx3">
                    {p.merchant_title}
                  </p>
                )}
                {p.address && <p className="truncate text-[10px] leading-tight font-medium text-tx4">{p.address}</p>}
                <p className="text-[10px] leading-tight font-medium text-tx4">
                  {p.created_at}
                  {tab === "published" && p.origin !== "user_manual" && ` · ${ORIGIN_LABEL[p.origin] ?? p.origin}`}
                </p>
                <div className="flex gap-1.5 pt-1.5">
                  {tab === "pending" ? (
                    <>
                      <Btn
                        variant="soft"
                        className="!px-2.5 !py-1.5 text-xs"
                        disabled={act.isPending}
                        onClick={() => act.mutate({ id: p.id, verb: "approve" })}
                      >
                        Одобрить
                      </Btn>
                      <Btn
                        variant="ghost"
                        className="!px-2.5 !py-1.5 text-xs"
                        disabled={act.isPending}
                        onClick={() => act.mutate({ id: p.id, verb: "reject" })}
                      >
                        Отклонить
                      </Btn>
                    </>
                  ) : (
                    <Btn
                      variant="ghost"
                      className="!px-2.5 !py-1.5 text-xs"
                      disabled={act.isPending}
                      onClick={() => {
                        if (!confirm(`Отозвать «${p.name}» из каталога?`)) return;
                        act.mutate({ id: p.id, verb: "reject" });
                      }}
                    >
                      Отозвать
                    </Btn>
                  )}
                </div>
              </div>
              {p.mcc && <span className="font-mono text-[12px] leading-tight font-bold text-accl">{p.mcc}</span>}
            </div>
          ))}
        </div>
      )}
      <ErrMsg error={act.error} />
    </>
  );
}
