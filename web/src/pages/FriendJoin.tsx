import { useMutation } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { useInvalidateFriends } from "../hooks";
import { Btn, Card, ErrMsg } from "../components/ui";

// CB-08 «Приглашение в друзья»: the landing for an invite link. Since 4e
// (2026-08-12) the link is multi-use and a claim files a friend REQUEST —
// «переход сам по себе не делает другом», the inviter confirms it in their
// входящие. Claim still happens on an explicit tap — no auto-fire on mount
// and no unauthenticated preflight. RequireAuth already parked this URL
// through the login/registration round-trip.
export default function FriendJoin() {
  const { token = "" } = useParams();
  const invalidate = useInvalidateFriends();

  const claim = useMutation({
    mutationFn: async () => unwrap(await api.POST("/api/v1/friends/invites/claim", { body: { token } })),
    onSuccess: invalidate,
  });

  const res = claim.data;
  const who = res && (
    <>
      {res.inviter.display_name} <span className="font-medium text-tx4">@{res.inviter.username}</span>
    </>
  );

  return (
    <div className="pt-6">
      <Card className="space-y-3 p-5 text-center" data-sid="CB-08.a">
        {res ? (
          <>
            <p className="text-3xl">{res.status === "accepted" || res.status === "already_friends" ? "🤝" : "💌"}</p>
            {res.status === "accepted" ? (
              <>
                <p className="text-base font-bold">Теперь вы друзья с {who}</p>
                <p className="text-[12px] font-medium text-tx3">
                  Встречная заявка уже ждала. По умолчанию ничего не расшарено — отметь, какие банки открыть, и
                  попроси о том же в ответ.
                </p>
              </>
            ) : res.status === "already_friends" ? (
              <>
                <p className="text-base font-bold">Вы уже друзья с {who}</p>
                <p className="text-[12px] font-medium text-tx3">Шэринг настраивается в «Друзья и шэринг».</p>
              </>
            ) : res.status === "already_requested" ? (
              <>
                <p className="text-base font-bold">Заявка уже отправлена {who}</p>
                <p className="text-[12px] font-medium text-tx3">Осталось дождаться подтверждения.</p>
              </>
            ) : (
              <>
                <p className="text-base font-bold">Заявка отправлена {who}</p>
                <p className="text-[12px] font-medium text-tx3">
                  Вы станете друзьями, когда её подтвердят. По умолчанию ничего не расшарено — открывать банки
                  каждый решает сам.
                </p>
              </>
            )}
            <div className="flex flex-col gap-2">
              <Link to="/friends/settings" className="block">
                <Btn className="w-full">Друзья и шэринг</Btn>
              </Link>
              <Link to="/friends" className="block">
                <Btn variant="ghost" className="w-full">
                  Кешбэк друзей
                </Btn>
              </Link>
            </div>
          </>
        ) : (
          <>
            <p className="text-3xl">💌</p>
            <p className="text-base font-bold">Приглашение в друзья</p>
            <p className="text-[12px] font-medium text-tx3">
              По этой ссылке вы отправите заявку в друзья — когда её подтвердят, сможете открыть друг другу свои
              категории кешбэка.
            </p>
            <Btn className="w-full" disabled={claim.isPending || token === ""} onClick={() => claim.mutate()}>
              Отправить заявку
            </Btn>
            <ErrMsg error={claim.error} />
          </>
        )}
      </Card>
    </div>
  );
}
