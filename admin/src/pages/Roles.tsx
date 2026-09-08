// AD-08 Роли (roles-moderation.md): exact-username promote/demote across
// user | moderator | admin. Deliberately NO user listing — appointment is
// operator work, and the sidecar never grows an enumeration surface.
import { useState } from "react";
import { request } from "../api";
import { Btn, Card, ErrMsg, Field, inputCls } from "../ui";

interface UserRole {
  username: string;
  role: "user" | "moderator" | "admin";
}

const ROLE_LABEL: Record<string, string> = {
  user: "пользователь",
  moderator: "модератор",
  admin: "админ",
};

export default function Roles() {
  const [username, setUsername] = useState("");
  const [found, setFound] = useState<UserRole | null>(null);
  const [role, setRole] = useState<UserRole["role"]>("user");
  const [saved, setSaved] = useState(false);
  const [err, setErr] = useState<unknown>(null);

  return (
    <>
      <Card title="Роли — по точному логину">
        <p className="mb-3 text-xs text-tx3">
          Модераторы проверяют заявки на точки продаж в приложении (MD-01). Список пользователей здесь не показывается
          намеренно — только точный логин.
        </p>
        <form
          className="flex items-end gap-2"
          onSubmit={async (e) => {
            e.preventDefault();
            setErr(null);
            setSaved(false);
            setFound(null);
            try {
              const u = await request<UserRole>("GET", `/api/users/${encodeURIComponent(username.trim())}/role`);
              setFound(u);
              setRole(u.role);
            } catch (e2) {
              setErr(e2);
            }
          }}
        >
          <Field label="Логин">
            <input
              className={`${inputCls} min-w-64`}
              required
              placeholder="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
            />
          </Field>
          <Btn kind="primary" type="submit">
            Найти
          </Btn>
        </form>
        <ErrMsg error={err} />
      </Card>

      {found && (
        <Card title={`@${found.username}`}>
          <form
            className="flex items-end gap-2"
            onSubmit={async (e) => {
              e.preventDefault();
              setErr(null);
              setSaved(false);
              try {
                const u = await request<UserRole>(
                  "PUT",
                  `/api/users/${encodeURIComponent(found.username)}/role`,
                  { role },
                );
                setFound(u);
                setRole(u.role);
                setSaved(true);
              } catch (e2) {
                setErr(e2);
              }
            }}
          >
            <Field label="Роль">
              <select className={inputCls} value={role} onChange={(e) => setRole(e.target.value as UserRole["role"])}>
                {(["user", "moderator", "admin"] as const).map((r) => (
                  <option key={r} value={r}>
                    {ROLE_LABEL[r]}
                  </option>
                ))}
              </select>
            </Field>
            <Btn kind="primary" type="submit" disabled={role === found.role}>
              Сохранить
            </Btn>
            {saved && <span className="pb-2 text-sm text-mint">Сохранено ✓</span>}
          </form>
          <p className="mt-2 text-xs text-tx3">
            Текущая роль: <b>{ROLE_LABEL[found.role]}</b>. Понижение действует со следующего запроса — перелогин не
            нужен.
          </p>
        </Card>
      )}
    </>
  );
}
