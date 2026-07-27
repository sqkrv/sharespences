import { useMe, useLogout } from "../auth";
import { useTheme, type ThemeSetting } from "../theme";
import { useInstallPrompt } from "../pwa";
import { Btn, Card } from "../components/ui";
import { BUILD, isStale, useDevMode, useServerBuild } from "../dev/devmode";

// «Сервисы» hosts what the design's shell has no other place for:
// profile, the three-state theme control, install, docs, logout, dev mode.
export default function Services() {
  const me = useMe();
  const logout = useLogout();
  const [setting, setSetting] = useTheme();
  const { isStandalone, isIOS, canInstall, install } = useInstallPrompt();
  const [dev, setDev] = useDevMode();
  const serverBuild = useServerBuild(dev);

  const options: { value: ThemeSetting; label: string }[] = [
    { value: "system", label: "◐ Системная" },
    { value: "light", label: "☀ Светлая" },
    { value: "dark", label: "☾ Тёмная" },
  ];

  return (
    <>
      <h1 className="text-[25px] font-extrabold tracking-tight">Сервисы</h1>

      <Card className="p-4" data-sid="SYS-03.a">
        <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">Аккаунт</p>
        <p className="mt-2 text-base font-bold">{me.data?.display_name}</p>
        <p className="text-sm font-medium text-tx3">{me.data?.email}</p>
        <Btn variant="danger" className="mt-4" onClick={() => logout.mutate()}>
          Выйти
        </Btn>
      </Card>

      <Card className="p-4" data-sid="SYS-03.b">
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

      {!isStandalone && (canInstall || isIOS) && (
        <Card className="p-4" data-sid="SYS-03.c">
          <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">Приложение</p>
          {canInstall ? (
            <>
              <p className="mt-2 text-sm font-medium text-tx3">
                Установите Sharespences на домашний экран — запуск без браузера, работает при плохой сети.
              </p>
              <Btn className="mt-3" onClick={() => void install()}>
                Установить приложение
              </Btn>
            </>
          ) : (
            // iOS Safari has no install event — the path is manual.
            <p className="mt-2 text-sm font-medium text-tx3">
              На iPhone: <b className="text-tx">Поделиться</b> → <b className="text-tx">На экран «Домой»</b> — приложение
              запустится без браузера и работает при плохой сети.
            </p>
          )}
        </Card>
      )}

      <Card className="p-4" data-sid="SYS-03.d">
        <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">Для разработчиков</p>

        <label className="mt-3 flex items-center justify-between gap-3">
          <span>
            <span className="block text-sm font-bold">Режим разработчика</span>
            <span className="block text-[12px] font-medium text-tx3">
              ID экранов поверх интерфейса и копирование контекста одним касанием
            </span>
          </span>
          <input
            type="checkbox"
            checked={dev}
            onChange={(e) => setDev(e.target.checked)}
            className="h-5 w-5 flex-none accent-[var(--t-acc)]"
          />
        </label>

        {dev && (
          <p className="mt-3 font-mono text-[10.5px] text-tx4">
            сборка {BUILD}
            {serverBuild === undefined ? "" : serverBuild === null ? " · сервер —" : serverBuild === BUILD ? " · сервер тот же" : ` · сервер ${serverBuild}`}
          </p>
        )}
        {dev && isStale(serverBuild) && (
          <p className="mt-1 text-[12px] font-semibold text-warn">
            ⚠️ приложение из кэша, на сервере новее — перезагрузите страницу
          </p>
        )}

        <a href="/docs" className="mt-3 block text-sm font-semibold text-accl">
          OpenAPI-документация →
        </a>
      </Card>
    </>
  );
}
