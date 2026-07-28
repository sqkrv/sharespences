import { useEffect, useState, useSyncExternalStore } from "react";

// Dev mode per docs/design/ui-preferences.md: a persisted toggle under
// «Сервисы», OFF by default, and deliberately alive in the PRODUCTION build —
// feedback comes from the installed PWA, so a dev-server-only
// flag would be off exactly where the reports are written.
//
// Storage mirrors theme.ts (localStorage + a class on <html>), but the flag
// is a module-level store read through useSyncExternalStore (the pwa.tsx
// idiom) rather than theme.ts's per-instance useState: the switch lives in
// «Сервисы» and the chip lives in the Shell — two subtrees, so it must
// broadcast. The <html> class is what drives the region outlines in CSS,
// which is why tagging a block costs one attribute and no React at all.

const KEY = "dev-mode";

let on = localStorage.getItem(KEY) === "1";
const subs = new Set<() => void>();

function syncClass() {
  document.documentElement.classList.toggle("devmode", on);
}
syncClass();

function subscribe(cb: () => void) {
  subs.add(cb);
  return () => subs.delete(cb);
}

export function setDevMode(next: boolean) {
  if (next === on) return;
  on = next;
  if (next) localStorage.setItem(KEY, "1");
  else localStorage.removeItem(KEY);
  syncClass();
  subs.forEach((cb) => cb());
}

export function useDevMode(): [boolean, (next: boolean) => void] {
  return [
    useSyncExternalStore(subscribe, () => on),
    setDevMode,
  ];
}

// Build stamp: injected by the `define` in vite.config.ts, so it identifies
// the bundle currently RUNNING.
declare const __BUILD__: string;
export const BUILD: string = __BUILD__;

// …and the stamp the server currently ships, fetched fresh. The two differ
// exactly when a service worker is still serving an old bundle — the failure
// class behind the 2026-07-24 «кнопка установки не появляется» report, which
// took a session to identify. `build.json` is outside the precache (workbox
// globPatterns lists no .json) and the Go handler sends `no-cache` for
// everything outside assets/, so this genuinely reaches the network.
// Returns null when unreachable (offline, or the Vite dev server, which builds
// no build.json).
export async function fetchServerBuild(): Promise<string | null> {
  try {
    const res = await fetch(`/build.json?t=${Date.now()}`, { cache: "no-store" });
    if (res.ok) return ((await res.json()) as { build?: string }).build ?? null;
  } catch {
    // Offline or no build.json — reported as «—», never thrown.
  }
  return null;
}

// Undefined while unknown. Re-read on every return to foreground: a deploy
// lands while the app sits open, and the installed PWA's normal lifecycle is
// resume, not reload — a mount-only read would report «сервер тот же» straight
// through the deploy it exists to catch. Same trigger pwa.tsx uses for the
// offline probe.
export function useServerBuild(enabled: boolean): string | null | undefined {
  const [stamp, setStamp] = useState<string | null>();

  useEffect(() => {
    if (!enabled) return;
    let alive = true;
    const read = () =>
      void fetchServerBuild().then((s) => {
        if (alive) setStamp(s);
      });
    read();
    const onVisible = () => {
      if (document.visibilityState === "visible") read();
    };
    document.addEventListener("visibilitychange", onVisible);
    return () => {
      alive = false;
      document.removeEventListener("visibilitychange", onVisible);
    };
  }, [enabled]);

  return stamp;
}

// ISO-8601 stamps compare lexicographically.
export function isStale(serverBuild: string | null | undefined): boolean {
  return serverBuild != null && serverBuild > BUILD;
}
