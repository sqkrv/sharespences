import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { Btn, Field, Input, ErrMsg } from "../components/ui";

export default function Login() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const qc = useQueryClient();
  const navigate = useNavigate();

  const login = useMutation({
    mutationFn: async () =>
      unwrap(await api.POST("/api/v1/auth/login", { body: { email, password } })),
    onSuccess: (user) => {
      qc.setQueryData(["me"], user);
      navigate("/");
    },
  });

  return (
    <div className="mx-auto flex min-h-screen max-w-sm flex-col justify-center p-4">
      <h1 className="mb-6 text-center text-2xl font-bold text-indigo-700 dark:text-indigo-400">Sharespences</h1>
      <form
        className="space-y-3 rounded-xl bg-white dark:bg-slate-900 p-5 shadow-sm"
        onSubmit={(e) => {
          e.preventDefault();
          login.mutate();
        }}
      >
        <Field label="Email">
          <Input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" />
        </Field>
        <Field label="Пароль">
          <Input
            type="password"
            required
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            autoComplete="current-password"
          />
        </Field>
        <Btn type="submit" className="w-full" disabled={login.isPending}>
          Войти
        </Btn>
        <ErrMsg error={login.error} />
        <p className="text-center text-sm text-slate-500 dark:text-slate-400">
          Нет аккаунта?{" "}
          <Link className="font-medium text-indigo-600 dark:text-indigo-400" to="/register">
            Регистрация
          </Link>
        </p>
      </form>
    </div>
  );
}
