// MD-01 Модерация (roles-moderation.md): очередь заявок 5e и поток недавно
// опубликованных точек. The «Сервисы» card renders this route only for
// role ≠ user; that is navigation sugar — the server re-checks the role on
// every operation, so a demoted moderator's next click answers 403.
// The rows are ANONYMOUS: the API never carries the submitter.
import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { api, unwrap } from "../api/client";
import { Badge, Btn, Card, Empty, ErrMsg, SegTabs, Spinner } from "../components/ui";

type Tab = "pending" | "published";

const ORIGIN_LABEL: Record<string, string> = {
  user_manual: "добавлена вручную",
  user_transaction: "из операции",
  admin: "оператор",
};
const TYPE_LABEL: Record<string, string> = {
  offline: "офлайн",
  online: "онлайн",
  app: "приложение",
  other: "другое",
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

  return (
    <>
      <h1 className="text-[25px] font-extrabold tracking-tight">Модерация</h1>

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
        <Card className="divide-y divide-brd2 p-0" data-sid="MD-01.b">
          {items.map((p) => (
            <div key={p.id} className="p-4">
              <div className="flex items-start justify-between gap-3">
                <div className="min-w-0">
                  <p className="text-sm font-bold">{p.name}</p>
                  {p.merchant_title && <p className="text-[12px] font-medium text-tx3">{p.merchant_title}</p>}
                  <p className="mt-1 text-[12px] font-medium text-tx3">
                    {p.mcc && <span className="font-mono">MCC {p.mcc}</span>}
                    {p.type && <> · {TYPE_LABEL[p.type] ?? p.type}</>}
                  </p>
                  {p.address && <p className="mt-0.5 text-[12px] font-medium text-tx4">{p.address}</p>}
                  <p className="mt-1 text-[11px] font-medium text-tx4">
                    {p.created_at}
                    {tab === "published" && p.origin !== "user_manual" && (
                      <>
                        {" "}
                        <Badge tone="slate">{ORIGIN_LABEL[p.origin] ?? p.origin}</Badge>
                      </>
                    )}
                  </p>
                </div>
                <div className="flex flex-none flex-col gap-1.5">
                  {tab === "pending" && (
                    <Btn disabled={act.isPending} onClick={() => act.mutate({ id: p.id, verb: "approve" })}>
                      Одобрить
                    </Btn>
                  )}
                  <Btn
                    variant="danger"
                    disabled={act.isPending}
                    onClick={() => {
                      if (tab === "published" && !confirm(`Отозвать «${p.name}» из каталога?`)) return;
                      act.mutate({ id: p.id, verb: "reject" });
                    }}
                  >
                    {tab === "pending" ? "Отклонить" : "Отозвать"}
                  </Btn>
                </div>
              </div>
            </div>
          ))}
        </Card>
      )}
      <ErrMsg error={act.error} />
    </>
  );
}
