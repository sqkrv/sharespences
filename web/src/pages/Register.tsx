import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useLocation, useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { purgeResponseCaches } from "../auth";
import { Btn, Card, Field, Input, ErrMsg, validityProps } from "../components/ui";
import { USERNAME_HINT, USERNAME_MAX, USERNAME_MIN, USERNAME_PATTERN } from "../lib";
import { BrandMark, LegalFooter } from "./Login";
import DevChip from "../dev/DevChip";

export default function Register() {
  const [form, setForm] = useState({ username: "", display_name: "", email: "", password: "" });
  const qc = useQueryClient();
  const navigate = useNavigate();
  const location = useLocation();
  const from = (location.state as { from?: string } | null)?.from ?? "/";
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));
  // The логин field lowercases as it is typed rather than rejecting capitals:
  // the server folds case anyway, and mobile keyboards capitalize the first
  // letter — a field stricter than the server would fail on its own default.
  const setUsername = (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, username: e.target.value.toLowerCase() }));

  const register = useMutation({
    mutationFn: async () => unwrap(await api.POST("/api/v1/auth/register", { body: form })),
    onSuccess: async (user) => {
      // Registration signs in (server sets the session cookie). Same reason
      // as sign-in: whoever used this browser last may not have signed out.
      await purgeResponseCaches();
      // Whatever the previous account left in the query cache would otherwise
      // render for this one until each query refetched.
      qc.clear();
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
            register.mutate();
          }}
        >
          <Field label="Логин">
            <Input
              required
              value={form.username}
              onChange={setUsername}
              autoComplete="username"
              autoCapitalize="none"
              autoCorrect="off"
              spellCheck={false}
              minLength={USERNAME_MIN}
              maxLength={USERNAME_MAX}
              pattern={USERNAME_PATTERN}
              // validityProps turns a pattern miss into this title (the rule
              // means nothing to the user without its explanation).
              title={USERNAME_HINT}
            />
            <p className="mt-1 text-[11px] font-medium text-tx4">{USERNAME_HINT}. По нему друзья найдут вас.</p>
          </Field>
          <Field label="Имя">
            <Input required maxLength={64} value={form.display_name} onChange={set("display_name")} />
          </Field>
          <Field label="Email">
            <Input type="email" required value={form.email} onChange={set("email")} autoComplete="email" />
          </Field>
          <Field label="Пароль (мин. 8 символов)">
            <Input type="password" required minLength={8} value={form.password} onChange={set("password")} autoComplete="new-password" />
          </Field>
          {/*
            Consent is collected as an affirmative act, unticked by default:
            the primary legal basis is п. 5 ч. 1 ст. 6 ФЗ-152 (processing needed
            to perform the Соглашение), and ст. 9's consent sits on top of it.
            `required` lets the browser block the submit — no state, no wiring.
          */}
          <label className="flex items-start gap-2.5 pt-1 text-[12px] leading-snug font-medium text-tx3">
            {/*
              Not the shared <Input> — that one carries the text-field styling.
              It only borrows the validity handlers, so the browser's own
              «Please check this box…» is replaced by Russian here too.
            */}
            <input
              type="checkbox"
              required
              {...validityProps<HTMLInputElement>()}
              className="mt-0.5 h-4 w-4 flex-none accent-[var(--t-acc)]"
            />
            <span>
              Принимаю{" "}
              <a className="font-semibold text-accl" href="/terms" target="_blank" rel="noreferrer">
                Пользовательское соглашение
              </a>{" "}
              и согласие на обработку персональных данных на условиях{" "}
              <a className="font-semibold text-accl" href="/privacy" target="_blank" rel="noreferrer">
                Политики
              </a>
            </span>
          </label>
          <Btn type="submit" className="w-full" disabled={register.isPending}>
            Создать аккаунт
          </Btn>
          <ErrMsg error={register.error} />
          <p className="text-center text-sm font-medium text-tx3">
            Уже есть аккаунт?{" "}
            <Link className="font-semibold text-accl" to="/login" state={location.state}>
              Войти
            </Link>
          </p>
        </form>
      </Card>
      <LegalFooter />
      <DevChip />
    </div>
  );
}
