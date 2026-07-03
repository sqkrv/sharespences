import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { Btn, Field, Input, ErrMsg } from "../components/ui";

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
    <div className="mx-auto flex min-h-screen max-w-sm flex-col justify-center p-4">
      <h1 className="mb-6 text-center text-2xl font-bold text-indigo-700">Регистрация</h1>
      <form
        className="space-y-3 rounded-xl bg-white p-5 shadow-sm"
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
          <Input
            type="password"
            required
            minLength={8}
            value={form.password}
            onChange={set("password")}
            autoComplete="new-password"
          />
        </Field>
        <Btn type="submit" className="w-full" disabled={register.isPending}>
          Создать аккаунт
        </Btn>
        <ErrMsg error={register.error} />
        <p className="text-center text-sm text-slate-500">
          Уже есть аккаунт?{" "}
          <Link className="font-medium text-indigo-600" to="/login">
            Войти
          </Link>
        </p>
      </form>
    </div>
  );
}
