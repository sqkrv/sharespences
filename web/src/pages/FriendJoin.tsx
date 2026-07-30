import { useMutation } from "@tanstack/react-query";
import { Link, useParams } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { useInvalidateFriends } from "../hooks";
import { Btn, Card, ErrMsg } from "../components/ui";

// CB-08 «Приглашение в друзья» (docs/specs/friends-sharing.md FR-S2): the
// landing for a one-shot invite link. Claim happens on an explicit tap —
// no auto-fire on mount (StrictMode double-mounts would burn the token and
// then show its own 409) and no unauthenticated preflight (the token stays
// unspent until the holder decides). RequireAuth already parked this URL
// through the login/registration round-trip.
export default function FriendJoin() {
  const { token = "" } = useParams();
  const invalidate = useInvalidateFriends();

  const claim = useMutation({
    mutationFn: async () => unwrap(await api.POST("/api/v1/friends/invites/claim", { body: { token } })),
    onSuccess: invalidate,
  });

  return (
    <div className="pt-6">
      <Card className="space-y-3 p-5 text-center" data-sid="CB-08.a">
        {claim.isSuccess ? (
          <>
            <p className="text-3xl">🤝</p>
            <p className="text-base font-bold">
              Теперь вы друзья с {claim.data.display_name}{" "}
              <span className="font-medium text-tx4">@{claim.data.username}</span>
            </p>
            <p className="text-[12px] font-medium text-tx3">
              По умолчанию ничего не расшарено — отметь, какие банки открыть, и попроси о том же в ответ.
            </p>
            <div className="flex flex-col gap-2">
              <Link to="/friends/settings" className="block">
                <Btn className="w-full">Настроить шэринг</Btn>
              </Link>
              <Link to="/friends" className="block">
                <Btn variant="ghost" className="w-full">
                  Кешбек друзей
                </Btn>
              </Link>
            </div>
          </>
        ) : (
          <>
            <p className="text-3xl">💌</p>
            <p className="text-base font-bold">Приглашение в друзья</p>
            <p className="text-[12px] font-medium text-tx3">
              Ссылка одноразовая: приняв её, вы станете друзьями и сможете открыть друг другу свои категории
              кешбека.
            </p>
            <Btn className="w-full" disabled={claim.isPending || token === ""} onClick={() => claim.mutate()}>
              Принять приглашение
            </Btn>
            <ErrMsg error={claim.error} />
          </>
        )}
      </Card>
    </div>
  );
}
