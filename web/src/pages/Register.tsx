import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { Btn, Card, Field, Input, ErrMsg } from "../components/ui";
import { BrandMark } from "./Login";

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
    </div>
  );
}
