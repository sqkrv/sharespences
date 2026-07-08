import { useState } from "react";
import { useMutation, useQueryClient } from "@tanstack/react-query";
import { Link, useNavigate } from "react-router-dom";
import { api, unwrap } from "../api/client";
import { Btn, Card, Field, Input, ErrMsg } from "../components/ui";

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

export default function Login() {
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const qc = useQueryClient();
  const navigate = useNavigate();

  const login = useMutation({
    mutationFn: async () => unwrap(await api.POST("/api/v1/auth/login", { body: { email, password } })),
    onSuccess: (user) => {
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
            login.mutate();
          }}
        >
          <Field label="Email">
            <Input type="email" required value={email} onChange={(e) => setEmail(e.target.value)} autoComplete="email" />
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
            <Link className="font-semibold text-accl" to="/register">
              Регистрация
            </Link>
          </p>
        </form>
      </Card>
    </div>
  );
}
