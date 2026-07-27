import { useState } from "react";
import { useLocation } from "react-router-dom";
import { BUILD, fetchServerBuild, isStale, useDevMode, useServerBuild } from "./devmode";
import { screenFor } from "./screens";

// The dev-mode chip: the screen's ID, always in the same corner, and a tap
// away from a paste-ready description of exactly where the user is standing.
// Everything in the payload is derived from the registry, the router, a DOM
// sweep for `data-sid` and localStorage — no page component has to hand it
// anything, which is what keeps dev mode out of the app's own code.

function uiState(): string {
  const out: string[] = [];
  for (let i = 0; i < localStorage.length; i++) {
    const k = localStorage.key(i);
    if (k == null) continue;
    const v = localStorage.getItem(k) ?? "";
    // Only UI preferences live here (auth is a cookie), but truncate anyway
    // rather than let a future key paste a blob into a chat.
    out.push(`${k}=${v.length > 32 ? `<${v.length} chars>` : v}`);
  }
  return out.sort().join(" ") || "—";
}

function context(pathname: string, search: string, serverBuild: string | null | undefined): string {
  const screen = screenFor(pathname);
  const regions = [...document.querySelectorAll("[data-sid]")]
    .map((el) => el.getAttribute("data-sid") ?? "")
    .filter(Boolean)
    .sort();
  const dark = document.documentElement.classList.contains("dark");
  const standalone = window.matchMedia("(display-mode: standalone)").matches;
  const server = serverBuild == null ? "—" : serverBuild === BUILD ? "same" : `${serverBuild} ⚠️ stale bundle`;

  return [
    screen ? `${screen.id} · ${screen.title}` : "?? · экран не в реестре",
    `route:   ${screen?.path ?? "—"}`,
    `url:     ${pathname}${search}`,
    `file:    ${screen?.file ?? "—"}`,
    `regions: ${[...new Set(regions)].join(", ") || "—"}`,
    `build:   ${BUILD} (server ${server})`,
    `ui:      ${uiState()}`,
    `env:     ${window.innerWidth}×${window.innerHeight} theme=${dark ? "dark" : "light"} standalone=${standalone ? "yes" : "no"}`,
  ].join("\n");
}

export default function DevChip() {
  const [dev] = useDevMode();
  const { pathname, search } = useLocation();
  const serverBuild = useServerBuild(dev);
  const [copied, setCopied] = useState(false);
  // navigator.clipboard needs a secure context — prod may well be plain HTTP.
  // Falling back to a selectable panel keeps the feature usable there.
  const [fallback, setFallback] = useState<string | null>(null);

  if (!dev) return null;

  const screen = screenFor(pathname);
  const stale = isStale(serverBuild);

  const copy = async () => {
    // Read the server stamp fresh: the payload gets pasted into a bug report
    // and «server same» must not be a claim inherited from page load.
    const text = context(pathname, search, await fetchServerBuild());
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    } catch {
      setFallback(text);
    }
  };

  return (
    <>
      <button
        type="button"
        onClick={() => void copy()}
        title="Скопировать контекст экрана"
        className={`fixed bottom-24 left-3 z-40 rounded-lg border border-dashed bg-bg/90 px-2 py-1 font-mono text-[10.5px] font-bold backdrop-blur ${
          stale ? "border-warn text-warn" : "border-dash text-tx3"
        }`}
      >
        {copied ? "скопировано" : `${screen?.id ?? "??"}${stale ? " ⚠️" : ""}`}
      </button>

      {fallback != null && (
        <div className="fixed inset-0 z-50 flex items-center justify-center bg-black/60 p-4">
          <div className="max-h-full w-full max-w-md overflow-auto rounded-2xl border border-brd bg-srf p-4">
            <pre className="font-mono text-[10.5px] leading-relaxed whitespace-pre-wrap text-tx2 select-all">
              {fallback}
            </pre>
            <button
              type="button"
              className="mt-3 w-full rounded-xl border border-brd2 bg-srf2 py-2 text-sm font-semibold text-tx3"
              onClick={() => setFallback(null)}
            >
              Закрыть
            </button>
          </div>
        </div>
      )}
    </>
  );
}
