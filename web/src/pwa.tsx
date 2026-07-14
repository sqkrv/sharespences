import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useRegisterSW } from "virtual:pwa-register/react";
import { api, unwrap } from "./api/client";
import { midMonthISO } from "./lib";

// PWA glue per docs/specs/pwa.md (meta-repo): offline indicator, prompt-style
// update toast, install affordances, and the offline-read cache warm-up.

function subscribeOnline(cb: () => void) {
  window.addEventListener("online", cb);
  window.addEventListener("offline", cb);
  return () => {
    window.removeEventListener("online", cb);
    window.removeEventListener("offline", cb);
  };
}

export function useOnline(): boolean {
  return useSyncExternalStore(subscribeOnline, () => navigator.onLine);
}

// «нет сети» pill under the status bar; data on screen is cache-served.
export function OfflineChip() {
  const online = useOnline();
  if (online) return null;
  return (
    <div className="pointer-events-none fixed inset-x-0 top-[max(env(safe-area-inset-top),8px)] z-40 flex justify-center">
      <span className="rounded-full border border-gold/25 bg-bg/90 px-3 py-1 text-[10.5px] font-semibold text-gold backdrop-blur">
        нет сети — данные из кэша
      </span>
    </div>
  );
}

// New deploy → waiting SW → toast; reload applies it (registerType: prompt).
export function ReloadPrompt() {
  const {
    needRefresh: [needRefresh, setNeedRefresh],
    updateServiceWorker,
  } = useRegisterSW();
  if (!needRefresh) return null;
  return (
    <div className="fixed inset-x-0 bottom-24 z-40 flex justify-center px-4">
      <div className="flex items-center gap-3 rounded-2xl border border-brd bg-srf px-4 py-2.5 shadow-[0_14px_40px_-12px_rgba(0,0,0,.55)]">
        <span className="text-sm font-semibold">Доступно обновление</span>
        <button
          type="button"
          className="grad-acc rounded-xl px-3 py-1.5 text-[12.5px] font-bold text-white"
          onClick={() => updateServiceWorker(true)}
        >
          Обновить
        </button>
        <button type="button" className="px-1 text-tx4" onClick={() => setNeedRefresh(false)}>
          ✕
        </button>
      </div>
    </div>
  );
}

type BeforeInstallPromptEvent = Event & { prompt: () => Promise<void> };

// Chromium fires beforeinstallprompt; iOS Safari never does — there the UX
// is a static «Поделиться → На экран „Домой"» hint (Services screen).
export function useInstallPrompt() {
  const [deferred, setDeferred] = useState<BeforeInstallPromptEvent | null>(null);

  useEffect(() => {
    const onPrompt = (e: Event) => {
      e.preventDefault();
      setDeferred(e as BeforeInstallPromptEvent);
    };
    window.addEventListener("beforeinstallprompt", onPrompt);
    return () => window.removeEventListener("beforeinstallprompt", onPrompt);
  }, []);

  const isStandalone =
    window.matchMedia("(display-mode: standalone)").matches ||
    ("standalone" in navigator && (navigator as { standalone?: boolean }).standalone === true);
  const isIOS = /iphone|ipad|ipod/i.test(navigator.userAgent);

  return {
    isStandalone,
    isIOS,
    canInstall: deferred != null,
    install: async () => {
      await deferred?.prompt();
      setDeferred(null);
    },
  };
}

// Offline-read warm-up: the checkout scenario must work for categories the
// user never opened this session. Prefetching routes the responses through
// the service worker, which is what actually persists them. Best-effort,
// sequential (throttled), once per session, skipped offline.
export function usePrefetchOffline() {
  const qc = useQueryClient();
  const ran = useRef(false);

  useEffect(() => {
    if (ran.current || !navigator.onLine || !("serviceWorker" in navigator)) return;
    ran.current = true;
    void (async () => {
      try {
        // Only fetches the SW intercepts get cached. On the very first visit
        // the page starts uncontrolled; clientsClaim hands it over right
        // after activation — wait for that (bounded), else skip quietly.
        if (!navigator.serviceWorker.controller) {
          await new Promise<void>((resolve) => {
            const timer = setTimeout(resolve, 10_000);
            navigator.serviceWorker.addEventListener(
              "controllerchange",
              () => {
                clearTimeout(timer);
                resolve();
              },
              { once: true },
            );
          });
          if (!navigator.serviceWorker.controller) return;
        }
        // RequireAuth fetched /auth/me BEFORE the SW controlled the page —
        // re-fetch it through the SW or offline start hangs on the gate
        // (docs/specs/pwa.md trap 2).
        await fetch("/api/v1/auth/me");
        const now = new Date();
        const date = midMonthISO(now.getFullYear(), now.getMonth());
        await qc.prefetchQuery({
          queryKey: ["overview", date],
          queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview", { params: { query: { date } } })),
        });
        const ov = await qc.fetchQuery({
          queryKey: ["overview"],
          queryFn: async () => unwrap(await api.GET("/api/v1/cashback/overview")),
        });
        const slugs = (ov.categories ?? []).map((c) => c.slug);
        if (ov.base) slugs.push("all-purchases");
        for (const slug of slugs) {
          await qc.prefetchQuery({
            queryKey: ["lookup", slug],
            queryFn: async () =>
              unwrap(await api.GET("/api/v1/cashback/lookup", { params: { query: { category: slug } } })),
          });
        }
      } catch {
        // Warm-up is opportunistic; the app works without it.
      }
    })();
  }, [qc]);
}
