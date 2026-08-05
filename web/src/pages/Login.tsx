import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { purgeResponseCaches } from "../auth";
import { Btn, Card, Field, Input, ErrMsg } from "../components/ui";
import DevChip from "../dev/DevChip";

export function BrandMark() {
  return (
    <div className="mb-6 flex items-center justify-center gap-2.5">
      <span className="grad-acc flex h-[30px] w-[30px] items-center justify-center rounded-[9px] text-[15px] font-extrabold text-white shadow-[0_6px_18px_-6px_rgba(139,111,255,.8)]">
        S
      </span>
      <span className="text-[13px] font-semibold tracking-[.34em] text-tx3 uppercase">Sharespences</span>
    </div>
  );
}

// Login/Register are the only screens without the bottom NavBar, so they are
// the only ones a footer can sit under without fighting it — and the only ones
// a stranger sees before deciding to register. Everywhere else the same links
// live in the «О приложении» card on SYS-03.
//
// Plain <a>, never <Link>: /privacy and /terms are static documents served
// outside the SPA (web/public/*.html), so a client-side navigation would find
// no route and render nothing. target=_blank because a form is always on
// screen here — reading the policy in place would discard what was typed.
// (The same links in SYS-03's «О приложении» card navigate in place: no form
// is at risk there, and staying in the tab keeps an installed PWA in the app.)
export function LegalFooter() {
  return (
    <footer className="mt-6 text-center text-[11.5px] leading-relaxed font-medium text-tx4">
      <p>
        Разработал{" "}
        <a className="font-semibold text-tx3" href="https://t.me/sharespences" target="_blank" rel="noreferrer">
          @sharespences
        </a>
      </p>
      <p className="mt-1">
        <a className="font-semibold text-tx3" href="/privacy" target="_blank" rel="noreferrer">
          Политика конфиденциальности
        </a>
        <span className="px-1.5">·</span>
        <a className="font-semibold text-tx3" href="/terms" target="_blank" rel="noreferrer">
          Пользовательское соглашение
        </a>
      </p>
    </footer>
  );
}

export default function Login() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const qc = useQueryClient();
  const navigate = useNavigate();
  // RequireAuth parked the interrupted location here (an invite link must
  // survive the login round-trip).
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? "/";

  const login = useMutation({
    mutationFn: async () => unwrap(await api.POST("/api/v1/auth/login", { body: { email, password } })),
    onSuccess: async (user) => {
      // A previous user may have closed the tab without signing out, leaving
      // their responses in the offline cache (keyed by URL only) — drop them
      // before this session can be served from them.
      await purgeResponseCaches();
      qc.setQueryData(["me"], user);
      navigate(from, { replace: true });
    },
  });

  return (
    <div className="mx-auto flex min-h-dvh max-w-sm flex-col justify-center p-4">
      <BrandMark />
      <Card className="p-5">
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            login.mutate();
          }}
        >
          <Field label="Email">
            {/* Case is folded server-side; this only stops the mobile keyboard
                from capitalizing an address the user then has to fix. */}
            <Input
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              autoComplete="email"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
            />
          </Field>
          <Field label="Пароль">
            <Input type="password" required value={password} onChange={(e) => setPassword(e.target.value)} autoComplete="current-password" />
          </Field>
          <Btn type="submit" className="w-full" disabled={login.isPending}>
            Войти
          </Btn>
          <ErrMsg error={login.error} />
          <p className="text-center text-sm font-medium text-tx3">
            Нет аккаунта?{" "}
            <Link className="font-semibold text-accl" to="/register" state={location.state}>
              Регистрация
            </Link>
          </p>
        </form>
      </Card>
      <LegalFooter />
      {/* Login/Register render outside the Shell, so they mount the chip themselves. */}
      <DevChip />
    </div>
  );
}
