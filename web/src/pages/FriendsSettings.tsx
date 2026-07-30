import { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "react-router-dom";
import { api, unwrap, type Friend, type FriendRequest } from "../api/client";
import { useClients, useInvalidateFriends } from "../hooks";
import { BankBadge, Btn, Card, Empty, ErrMsg, Input, SegTabs, Spinner } from "../components/ui";

// CB-07 «Друзья и шэринг» (docs/specs/friends-sharing.md FR-S1/S2/S3): the
// graph management screen. Three cuts: друзья (per-friend grant toggles +
// unfriend), заявки (exact-username search, inbox), приглашения (one-shot
// links; the token is shown exactly once — the server stores only a hash).

type Cut = "friends" | "requests" | "invites";

function useFriends() {
  return useQuery({
    queryKey: ["friends"],
    queryFn: async () => unwrap(await api.GET("/api/v1/friends")) ?? [],
  });
}

function FriendRow({ f }: { f: Friend }) {
  const [open, setOpen] = useState(false);
  const [confirmRemove, setConfirmRemove] = useState(false);
  const clients = useClients();
  const invalidate = useInvalidateFriends();

  const sharing = useQuery({
    queryKey: ["friend-sharing"],
    queryFn: async () => unwrap(await api.GET("/api/v1/friends/sharing")) ?? [],
  });
  const sharedSet = new Set(
    (sharing.data ?? []).filter((s) => s.friend_user_id === f.user_id).map((s) => s.bank_client_id),
  );

  const toggle = useMutation({
    mutationFn: async ({ clientID, shared }: { clientID: number; shared: boolean }) =>
      unwrap(
        await api.PUT("/api/v1/friends/sharing", {
          body: { bank_client_id: clientID, friend_user_id: f.user_id, shared },
        }),
      ),
    onSuccess: invalidate,
  });

  const remove = useMutation({
    mutationFn: async () =>
      unwrap(await api.DELETE("/api/v1/friends/{userId}", { params: { path: { userId: f.user_id } } })),
    onSuccess: invalidate,
  });

  return (
    <Card className="p-3.5">
      <div className="flex items-center gap-2.5">
        <button type="button" className="min-w-0 flex-1 text-left" onClick={() => setOpen((v) => !v)}>
          <p className="text-sm font-bold">
            {f.display_name} <span className="font-medium text-tx4">@{f.username}</span>
          </p>
          <p className="text-[10.5px] font-medium text-tx4">
            {sharedSet.size > 0 ? `видит ${sharedSet.size} из твоих банков` : "ничего не видит"}
          </p>
        </button>
        <span className="text-tx4">{open ? "▴" : "▾"}</span>
      </div>

      {open && (
        <div className="mt-3 space-y-2 border-t border-brd pt-3">
          <p className="text-[10.5px] font-semibold tracking-wide text-tx3">ЧТО ВИДИТ {f.display_name.toUpperCase()}</p>
          {(clients.data ?? []).length === 0 && (
            <p className="text-[11.5px] font-medium text-tx4">У тебя пока нет банков — добавь их на обзоре.</p>
          )}
          {(clients.data ?? []).map((c) => {
            const shared = sharedSet.has(c.id);
            return (
              <button
                key={c.id}
                type="button"
                disabled={toggle.isPending}
                onClick={() => toggle.mutate({ clientID: c.id, shared: !shared })}
                className={`flex w-full items-center gap-2.5 rounded-xl border px-3 py-2 text-left transition ${
                  shared ? "border-acc/40 bg-acc/10" : "border-brd2 bg-srf2"
                }`}
              >
                <BankBadge name={c.bank_name ?? ""} size={24} />
                <span className="min-w-0 flex-1 text-[13px] font-semibold">
                  {c.bank_name}
                  {c.label && <span className="font-medium text-tx4"> · {c.label}</span>}
                </span>
                <span
                  className={`h-5 w-9 flex-none rounded-full p-0.5 transition ${shared ? "bg-acc" : "bg-inset"}`}
                >
                  <span
                    className={`block h-4 w-4 rounded-full bg-white transition ${shared ? "translate-x-4" : ""}`}
                  />
                </span>
              </button>
            );
          })}
          <ErrMsg error={toggle.error} />

          {confirmRemove ? (
            <div className="space-y-2 rounded-xl bg-warn/5 p-3">
              <p className="text-[12px] font-medium text-warn">
                Удалить из друзей? Доступ к кешбеку отзовётся в обе стороны.
              </p>
              <div className="flex gap-2">
                <Btn variant="danger" disabled={remove.isPending} onClick={() => remove.mutate()}>
                  Удалить
                </Btn>
                <Btn variant="ghost" onClick={() => setConfirmRemove(false)}>
                  Отмена
                </Btn>
              </div>
              <ErrMsg error={remove.error} />
            </div>
          ) : (
            <Btn variant="danger" className="w-full" onClick={() => setConfirmRemove(true)}>
              Удалить из друзей
            </Btn>
          )}
        </div>
      )}
    </Card>
  );
}

function FriendsCut() {
  const friends = useFriends();
  return (
    <div className="space-y-2.5" data-sid="CB-07.a">
      {friends.isPending && <Spinner />}
      <ErrMsg error={friends.error} />
      {friends.data &&
        (friends.data.length > 0 ? (
          <>
            <p className="mx-0.5 text-[11px] font-medium text-tx4">
              По умолчанию друг не видит ничего — отметь, какие банки ему открыть.
            </p>
            {friends.data.map((f) => (
              <FriendRow key={f.user_id} f={f} />
            ))}
          </>
        ) : (
          <Empty>Друзей пока нет — отправь заявку или ссылку-приглашение</Empty>
        ))}
    </div>
  );
}

function RequestsCut() {
  const [username, setUsername] = useState("");
  const [submitted, setSubmitted] = useState("");
  const invalidate = useInvalidateFriends();

  const requests = useQuery({
    queryKey: ["friend-requests"],
    queryFn: async () => unwrap(await api.GET("/api/v1/friends/requests")),
  });

  const search = useQuery({
    queryKey: ["friend-search", submitted],
    enabled: submitted !== "",
    retry: false,
    queryFn: async () =>
      unwrap(await api.GET("/api/v1/friends/search", { params: { query: { username: submitted } } })),
  });

  const [notice, setNotice] = useState("");
  const send = useMutation({
    mutationFn: async (name: string) =>
      unwrap(await api.POST("/api/v1/friends/requests", { body: { username: name } })),
    onSuccess: (r) => {
      setUsername("");
      setSubmitted("");
      invalidate();
      if (r.status === "accepted") setNotice("Встречная заявка уже ждала — вы теперь друзья");
      else setNotice("Заявка отправлена");
    },
  });

  const respond = useMutation({
    mutationFn: async ({ id, action }: { id: number; action: "accept" | "decline" | "cancel" }) => {
      const path = { params: { path: { id } } };
      if (action === "cancel") return unwrap(await api.DELETE("/api/v1/friends/requests/{id}", path));
      if (action === "accept") return unwrap(await api.POST("/api/v1/friends/requests/{id}/accept", path));
      return unwrap(await api.POST("/api/v1/friends/requests/{id}/decline", path));
    },
    onSuccess: invalidate,
  });

  const reqRow = (r: FriendRequest, actions: React.ReactNode) => (
    <div key={r.id} className="flex items-center gap-2.5 rounded-xl border border-brd bg-srf px-3 py-2.5">
      <div className="min-w-0 flex-1">
        <p className="text-[13px] font-semibold">
          {r.display_name} <span className="font-medium text-tx4">@{r.username}</span>
        </p>
      </div>
      {actions}
    </div>
  );

  return (
    <div className="space-y-4" data-sid="CB-07.b">
      <Card className="space-y-2.5 p-4">
        <p className="text-[11px] font-semibold tracking-wide text-tx3">НАЙТИ ПО ЛОГИНУ</p>
        <form
          className="flex gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            setNotice("");
            setSubmitted(username.trim());
          }}
        >
          <Input
            placeholder="логин целиком"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            required
          />
          <Btn type="submit" variant="soft" disabled={search.isFetching}>
            Найти
          </Btn>
        </form>
        <p className="text-[10px] font-medium text-tx4">Поиск только по точному логину — по кусочку не найдёт.</p>
        {search.isError && <Empty>Никого не нашлось</Empty>}
        {search.data && (
          <div className="flex items-center gap-2.5 rounded-xl border border-acc/30 bg-acc/5 px-3 py-2.5">
            <div className="min-w-0 flex-1">
              <p className="text-[13px] font-semibold">
                {search.data.display_name} <span className="font-medium text-tx4">@{search.data.username}</span>
              </p>
            </div>
            <Btn
              variant="soft"
              className="!px-2.5 !py-1.5 text-xs"
              disabled={send.isPending}
              onClick={() => send.mutate(search.data.username)}
            >
              Отправить заявку
            </Btn>
          </div>
        )}
        <ErrMsg error={send.error} />
        {notice && <p className="rounded-xl bg-mint/10 px-3 py-2 text-sm font-medium text-mint">{notice}</p>}
      </Card>

      {requests.isPending && <Spinner />}
      <ErrMsg error={requests.error} />
      {requests.data && (
        <>
          {(requests.data.incoming ?? []).length > 0 && (
            <div className="space-y-1.5">
              <p className="mx-0.5 text-[11px] font-semibold text-accl">Входящие</p>
              {(requests.data.incoming ?? []).map((r) =>
                reqRow(
                  r,
                  <div className="flex gap-1.5">
                    <Btn
                      variant="soft"
                      className="!px-2.5 !py-1.5 text-xs"
                      disabled={respond.isPending}
                      onClick={() => respond.mutate({ id: r.id, action: "accept" })}
                    >
                      Принять
                    </Btn>
                    <Btn
                      variant="ghost"
                      className="!px-2.5 !py-1.5 text-xs"
                      disabled={respond.isPending}
                      onClick={() => respond.mutate({ id: r.id, action: "decline" })}
                    >
                      Отклонить
                    </Btn>
                  </div>,
                ),
              )}
            </div>
          )}
          {(requests.data.outgoing ?? []).length > 0 && (
            <div className="space-y-1.5">
              <p className="mx-0.5 text-[11px] font-semibold text-tx3">Исходящие</p>
              {(requests.data.outgoing ?? []).map((r) =>
                reqRow(
                  r,
                  <Btn
                    variant="ghost"
                    className="!px-2.5 !py-1.5 text-xs"
                    disabled={respond.isPending}
                    onClick={() => respond.mutate({ id: r.id, action: "cancel" })}
                  >
                    Отменить
                  </Btn>,
                ),
              )}
            </div>
          )}
          {(requests.data.incoming ?? []).length + (requests.data.outgoing ?? []).length === 0 && (
            <Empty>Заявок нет</Empty>
          )}
          <ErrMsg error={respond.error} />
        </>
      )}
    </div>
  );
}

function InvitesCut() {
  const invalidate = useInvalidateFriends();
  const qc = useQueryClient();
  // The plaintext token exists only in the create response — kept in memory
  // for the copy button, never in storage.
  const [fresh, setFresh] = useState<{ id: string; url: string } | null>(null);
  const [copied, setCopied] = useState(false);

  const invites = useQuery({
    queryKey: ["friend-invites"],
    queryFn: async () => unwrap(await api.GET("/api/v1/friends/invites")) ?? [],
  });

  const create = useMutation({
    mutationFn: async () => unwrap(await api.POST("/api/v1/friends/invites")),
    onSuccess: (inv) => {
      setFresh({ id: inv.id, url: window.location.origin + inv.url });
      setCopied(false);
      qc.invalidateQueries({ queryKey: ["friend-invites"] });
    },
  });

  const revoke = useMutation({
    mutationFn: async (id: string) =>
      unwrap(await api.DELETE("/api/v1/friends/invites/{id}", { params: { path: { id } } })),
    onSuccess: (_, id) => {
      if (fresh?.id === id) setFresh(null);
      qc.invalidateQueries({ queryKey: ["friend-invites"] });
      invalidate();
    },
  });

  return (
    <div className="space-y-3" data-sid="CB-07.c">
      <Card className="space-y-2.5 p-4">
        <p className="text-[12px] font-medium text-tx3">
          Одноразовая ссылка: отправь её в любом мессенджере — кто откроет, тот и станет другом. Действует 7 дней.
        </p>
        <Btn className="w-full" disabled={create.isPending} onClick={() => create.mutate()}>
          Создать ссылку
        </Btn>
        <ErrMsg error={create.error} />
        {fresh && (
          <div className="space-y-2 rounded-xl border border-acc/30 bg-acc/5 p-3">
            <p className="text-[10.5px] font-semibold text-accl">
              Ссылка показывается только сейчас — скопируй её.
            </p>
            <p className="break-all rounded-lg bg-inset px-2.5 py-2 font-mono text-[11px] text-tx2">{fresh.url}</p>
            <Btn
              variant="soft"
              className="w-full"
              onClick={async () => {
                await navigator.clipboard.writeText(fresh.url);
                setCopied(true);
              }}
            >
              {copied ? "Скопировано ✓" : "Скопировать"}
            </Btn>
          </div>
        )}
      </Card>

      {invites.isPending && <Spinner />}
      <ErrMsg error={invites.error} />
      {invites.data &&
        (invites.data.length > 0 ? (
          <div className="space-y-1.5">
            <p className="mx-0.5 text-[11px] font-semibold text-tx3">Активные приглашения</p>
            {invites.data.map((inv) => (
              <div key={inv.id} className="flex items-center gap-2.5 rounded-xl border border-brd bg-srf px-3 py-2.5">
                <div className="min-w-0 flex-1">
                  <p className="text-[12px] font-semibold text-tx2">
                    до {new Date(inv.expires_at).toLocaleDateString("ru-RU")}
                  </p>
                  <p className="text-[10px] font-medium text-tx4">ссылку можно увидеть только при создании</p>
                </div>
                <Btn
                  variant="danger"
                  className="!px-2.5 !py-1.5 text-xs"
                  disabled={revoke.isPending}
                  onClick={() => revoke.mutate(inv.id)}
                >
                  Отозвать
                </Btn>
              </div>
            ))}
            <ErrMsg error={revoke.error} />
          </div>
        ) : (
          <Empty>Активных приглашений нет</Empty>
        ))}
    </div>
  );
}

export default function FriendsSettings() {
  const [cut, setCut] = useState<Cut>("friends");
  return (
    <>
      <div className="flex items-center justify-between">
        <h1 className="text-[22px] font-extrabold tracking-tight">Друзья и шэринг</h1>
        <Link to="/friends" className="text-[12px] font-semibold text-accl">
          Кешбек друзей ›
        </Link>
      </div>
      <SegTabs
        sid="CB-07.d"
        value={cut}
        onChange={setCut}
        options={[
          { value: "friends", label: "Друзья" },
          { value: "requests", label: "Заявки" },
          { value: "invites", label: "Приглашения" },
        ]}
      />
      {cut === "friends" && <FriendsCut />}
      {cut === "requests" && <RequestsCut />}
      {cut === "invites" && <InvitesCut />}
    </>
  );
}
