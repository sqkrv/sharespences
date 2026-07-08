import { useMe, useLogout } from "../auth";
import { useTheme, type ThemeSetting } from "../theme";
import { Btn, Card } from "../components/ui";

// «Сервисы» hosts what the design's shell has no other place for:
// profile, the three-state theme control, docs, logout.
export default function Services() {
  const me = useMe();
  const logout = useLogout();
  const [setting, setSetting] = useTheme();

  const options: { value: ThemeSetting; label: string }[] = [
    { value: "system", label: "◐ Системная" },
    { value: "light", label: "☀ Светлая" },
    { value: "dark", label: "☾ Тёмная" },
  ];

  return (
    <>
      <h1 className="text-[25px] font-extrabold tracking-tight">Сервисы</h1>

      <Card className="p-4">
        <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">Аккаунт</p>
        <p className="mt-2 text-base font-bold">{me.data?.display_name}</p>
        <p className="text-sm font-medium text-tx3">{me.data?.email}</p>
        <Btn variant="danger" className="mt-4" onClick={() => logout.mutate()}>
          Выйти
        </Btn>
      </Card>

      <Card className="p-4">
        <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">Оформление</p>
        <div className="mt-3 flex gap-1.5">
          {options.map((o) => (
            <button
              key={o.value}
              type="button"
              onClick={() => setSetting(o.value)}
              className={`flex-1 rounded-xl px-2 py-2.5 text-[12px] transition ${
                setting === o.value ? "grad-acc font-bold text-white" : "border border-brd2 bg-srf2 font-semibold text-tx3"
              }`}
            >
              {o.label}
            </button>
          ))}
        </div>
      </Card>

      <Card className="p-4">
        <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">Для разработчиков</p>
        <a href="/docs" className="mt-2 block text-sm font-semibold text-accl">
          OpenAPI-документация →
        </a>
      </Card>
    </>
  );
}
