import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { Btn, Card, Field, Input, ErrMsg, validityProps } from "../components/ui";
import { BrandMark, LegalFooter } from "./Login";
import DevChip from "../dev/DevChip";

export default function Register() {
  const [form, setForm] = useState({ username: "", display_name: "", email: "", password: "" });
  const qc = useQueryClient();
  const navigate = useNavigate();
  const set = (k: keyof typeof form) => (e: React.ChangeEvent<HTMLInputElement>) =>
    setForm((f) => ({ ...f, [k]: e.target.value }));

  const register = useMutation({
    mutationFn: async () => unwrap(await api.POST("/api/v1/auth/register", { body: form })),
    onSuccess: (user) => {
      // Registration signs in (server sets the session cookie).
      qc.setQueryData(["me"], user);
      navigate("/");
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
            <Input required value={form.username} onChange={set("username")} autoComplete="username" />
          </Field>
          <Field label="Имя">
            <Input required value={form.display_name} onChange={set("display_name")} />
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
            <Link className="font-semibold text-accl" to="/login">
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
