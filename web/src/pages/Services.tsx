import { useState } from "react";
import { Link } from "react-router-dom";
import { useMe, useLogout } from "../auth";
import { useTheme, type ThemeSetting } from "../theme";
import { useInstallPrompt } from "../pwa";
import { useVersion } from "../hooks";
import { Btn, Card } from "../components/ui";
import { BUILD, isStale, useDevMode, useServerBuild } from "../dev/devmode";

// «О приложении» reports two different things on one line. The CalVer version
// (ADR-0006) comes from the SERVER and names the running binary — the answer
// to «which build hit this bug?» from someone who is not the author. BUILD is
// the compile-time stamp of the BUNDLE the browser is running, which can lag
// the server whenever a service worker is still serving an old one.
const built = new Date(BUILD);
const buildLabel = Number.isNaN(built.getTime())
  ? null
  : built.toLocaleDateString("ru-RU", { day: "numeric", month: "long", year: "numeric" });

// SYS-03.f «Модули» (design4 «Perks - Module», board C). The spec asks for one
// «Привилегии» item; the design widens it to a card, because Друзья is built
// and has had no entry from this screen at all. Both rows use the accent-row
// pattern the moderation card already established.
//
// The «нов» badge marks the first release of a module and comes off in the next
// one — it is a hand-edited literal, not stored state.
function ModuleRow({
  to,
  emoji,
  title,
  sub,
  fresh,
}: {
  to: string;
  emoji: string;
  title: string;
  sub: string;
  fresh?: boolean;
}) {
  return (
    <Link to={to} className="flex items-center gap-3 rounded-xl border border-brd bg-srf2 px-3 py-2.5">
      <span className="flex h-[34px] w-[34px] flex-none items-center justify-center rounded-[12px] bg-inset text-[16px]">
        {emoji}
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex items-center gap-1.5">
          <span className="truncate text-[13px] font-bold">{title}</span>
          {fresh && (
            <span className="flex-none rounded bg-acc/15 px-1 py-[1px] text-[9px] font-bold text-accl">нов</span>
          )}
        </span>
        <span className="block truncate text-[10.5px] font-medium text-tx3">{sub}</span>
      </span>
      <span className="flex-none text-[13px] font-bold text-tx4">›</span>
    </Link>
  );
}

// «Сервисы» hosts what the design's shell has no other place for: profile, the
// three-state theme control, install, docs, logout, dev mode, and the app's own
// identity card (author, legal documents, build).
export default function Services() {
  const me = useMe();
  const logout = useLogout();
  const [loginCopied, setLoginCopied] = useState(false);
  const [setting, setSetting] = useTheme();
  const { isStandalone, isIOS, canInstall, install } = useInstallPrompt();
  const [dev, setDev] = useDevMode();
  const version = useVersion();
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
        {/* The login is how friends find you (CB-07 searches the exact
            login and nothing else) — this is its one visible home. */}
        {me.data?.username && (
          <div className="mt-2 flex items-center gap-2">
            <span className="rounded-lg bg-inset px-2 py-1 font-mono text-[12.5px] font-semibold text-tx2">
              @{me.data.username}
            </span>
            <button
              type="button"
              className="text-[11px] font-semibold text-accl"
              onClick={async () => {
                await navigator.clipboard.writeText(me.data!.username);
                setLoginCopied(true);
              }}
            >
              {loginCopied ? "Скопировано ✓" : "Скопировать"}
            </button>
          </div>
        )}
        <p className="mt-1 text-[10px] font-medium text-tx4">По логину друзья находят тебя в кешбеке.</p>
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

      <Card className="p-4" data-sid="SYS-03.f">
        <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">Модули</p>
        <div className="mt-2.5 space-y-1.5">
          <ModuleRow
            to="/perks"
            emoji="🎟️"
            title="Привилегии"
            sub="Квоты банковских привилегий: такси, бизнес-залы, преференции"
            fresh
          />
          <ModuleRow to="/friends/settings" emoji="👥" title="Друзья" sub="Обмен кешбек-выборами и заявки" />
        </div>
      </Card>

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

        {/* The rendered /docs page is a development aid (it loads Stoplight
            from unpkg, which production must not do), so the link points at
            the document itself — always served, and what a client generator
            wants anyway. */}
        <a href="/openapi.json" className="mt-3 block text-sm font-semibold text-accl">
          OpenAPI-документация →
        </a>
      </Card>

      <Card className="p-4" data-sid="SYS-03.e">
        <p className="text-[10px] font-semibold uppercase tracking-[.1em] text-tx4">О приложении</p>
        <p className="mt-2 text-sm font-medium text-tx3">
          Sharespences — учёт банковского кешбека: какие категории выбраны и какой картой выгоднее платить.
        </p>
        <p className="mt-2 text-sm font-medium text-tx3">
          Разработал{" "}
          <a className="font-semibold text-accl" href="https://t.me/sharespences" target="_blank" rel="noreferrer">
            @sharespences
          </a>{" "}
          — туда же можно написать об ошибке или попросить удалить аккаунт.
        </p>
        {/*
          Same-tab, unlike the auth screens' LegalFooter: no form is at risk
          here, and an in-place navigation keeps an installed PWA in the app.
        */}
        <a href="/privacy" className="mt-3 block text-sm font-semibold text-accl">
          Политика обработки персональных данных →
        </a>
        <a href="/terms" className="mt-2 block text-sm font-semibold text-accl">
          Пользовательское соглашение →
        </a>
        <p className="mt-3 text-[11px] font-medium text-tx4">
          {version.data?.version && version.data.version !== "dev" ? `Версия ${version.data.version}` : "Версия для разработки"}
          {buildLabel && ` · сборка от ${buildLabel}`}
        </p>
      </Card>
    </>
  );
}
