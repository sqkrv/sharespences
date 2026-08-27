import { useQuery } from "@tanstack/react-query";
import { ApiError, api, unwrap } from "../api/client";
import { useNavigate, useParams } from "react-router-dom";
import { useCategories } from "../hooks";
import { BackButton, Card, ErrMsg, Spinner } from "../components/ui";
import { FALLBACK_EMOJI } from "../lib";
import { Leaderboard } from "./Pos";

// CB-13 «Экран кода» (13a) — where a tap on an MCC lands: the official code
// name, its canonical categories, «Этот код в ваших банках» (the same exact
// per-bank board the точка продаж uses — each bank judged by its OWN
// category for the code), and the known points carrying it.

export default function Mcc() {
  const code = useParams().code ?? "";
  const navigate = useNavigate();
  const categories = useCategories();

  const resolve = useQuery({
    queryKey: ["mcc-resolve", code],
    retry: false,
    queryFn: async () => unwrap(await api.GET("/api/v1/mcc/resolve", { params: { query: { code } } })),
  });
  const board = useQuery({
    queryKey: ["mcc-board", code],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/mcc-board", { params: { query: { code: Number(code) } } })),
  });
  const merchants = useQuery({
    queryKey: ["mcc-code-merchants", code],
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/mcc/codes/{code}/merchants", { params: { path: { code: Number(code) } } })) ?? [],
  });

  const unknownCode = resolve.isError && resolve.error instanceof ApiError && resolve.error.status === 404;
  const canonicals = resolve.data?.canonicals ?? [];
  const emojiOf = (slug?: string | null) => (categories.data ?? []).find((c) => c.slug === slug)?.emoji;

  return (
    <>
      <div className="flex items-center gap-2.5">
        <BackButton fallback="/search" />
        <h1 className="min-w-0 flex-1 truncate text-xl font-extrabold tracking-tight">MCC {code}</h1>
        <span className="flex-none rounded-lg bg-inset px-2 py-1 text-[10.5px] font-semibold text-tx3">код категории</span>
      </div>

      {resolve.isPending && <Spinner />}
      {unknownCode && (
        <Card className="p-4 text-center">
          <p className="text-sm font-medium text-tx3">Код {code} не найден в справочнике MCC.</p>
        </Card>
      )}
      {resolve.isError && !unknownCode && <ErrMsg error={resolve.error} />}

      {resolve.data && (
        <Card className="p-3.5" data-sid="CB-13.a">
          <div className="flex items-center gap-3">
            <span className="flex-none font-mono text-[22px] font-extrabold text-accl">{resolve.data.code.code}</span>
            <div className="min-w-0 flex-1">
              <p className="text-[14px] font-bold">{resolve.data.code.name}</p>
              <p className="mt-0.5 text-[10px] font-medium text-tx4">официальное имя кода в платёжных сетях</p>
            </div>
          </div>
          {canonicals.length > 0 && (
            <div className="mt-2.5 border-t border-brd/60 pt-2.5">
              <p className="text-[10px] font-medium tracking-[.06em] text-tx4 uppercase">каноническая категория</p>
              <p className="mt-1 text-[12.5px] font-semibold text-tx2">
                {canonicals.map((c) => `${emojiOf(c.slug) || FALLBACK_EMOJI} ${c.title}`).join(" · ")}
              </p>
            </div>
          )}
        </Card>
      )}

      {board.isPending && <Spinner />}
      {board.isError && <ErrMsg error={board.error} />}
      {board.data && <Leaderboard board={board.data} matches={[]} sid="CB-13.b" />}

      {(merchants.data ?? []).length > 0 && (
        <div className="space-y-1.5" data-sid="CB-13.c">
          <p className="mx-0.5 text-[10.5px] font-extrabold tracking-[.14em] text-tx3 uppercase">Точки с кодом {code}</p>
          {(merchants.data ?? []).map((m) => (
            <button
              key={m.id}
              type="button"
              onClick={() => navigate(`/pos?mcc=${code}&merchant=${encodeURIComponent(m.name)}&pos=${m.id}`)}
              className="flex w-full items-center gap-2.5 rounded-2xl border border-brd bg-srf px-3 py-2.5 text-left hover:bg-srf2"
            >
              <span className="min-w-0 flex-1">
                <span className="block truncate text-[13px] font-bold">{m.name}</span>
                {m.address && <span className="block truncate text-[10.5px] font-medium text-tx4">{m.address}</span>}
              </span>
              <span className="text-tx4">›</span>
            </button>
          ))}
        </div>
      )}
    </>
  );
}
