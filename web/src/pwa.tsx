import { useEffect, useRef, useSyncExternalStore } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useRegisterSW } from "virtual:pwa-register/react";
import { api, unwrap } from "./api/client";
import { midMonthISO, STATUS_URL } from "./lib";

// PWA glue per docs/specs/pwa.md (meta-repo): offline indicator, prompt-style
// update toast, install affordances, and the offline-read cache warm-up.

// «Offline» = the server didn't answer a probe. navigator.onLine alone can't
// carry the chip: it tracks the OS network interface, so a dead server,
// WiFi-without-internet, and iOS standalone (onLine stuck true, WebKit bug)
// all read «online» while every response is silently cache-served. The probe
// is HEAD (the SW caches match GET only, so it always reaches the network)
// + no-store (skips the HTTP cache); any HTTP status counts as reachable.
//
// The two failures are not the same answer, which is why this is a state and
// not a boolean: with no network at all there is nothing to send the user to
// (the status page is a web page too), while a device that has internet but
// cannot reach us is exactly the case the status page exists for.
export type NetState = "online" | "device-offline" | "server-unreachable";
let net: NetState = navigator.onLine ? "online" : "device-offline";
const netSubs = new Set<() => void>();
let recoverTimer: ReturnType<typeof setInterval> | undefined;

function setNet(next: NetState) {
  // While offline, re-probe periodically — recovery can't wait for the
  // `online` event (it may never fire in iOS standalone).
  if (next !== "online" && recoverTimer == null) recoverTimer = setInterval(() => void probe(), 15_000);
  if (next === "online" && recoverTimer != null) {
    clearInterval(recoverTimer);
    recoverTimer = undefined;
  }
  if (next === net) return;
  net = next;
  netSubs.forEach((cb) => cb());
}

async function probe() {
  try {
    await fetch("/manifest.webmanifest", {
      method: "HEAD",
      cache: "no-store",
      signal: AbortSignal.timeout(4_000), // hung ≈ offline; the SW falls back to cache at 3s anyway
    });
    setNet("online");
  } catch {
    // A failed probe with the interface up means the server is the problem —
    // the OS's «offline» is trustworthy in the negative direction only.
    setNet(navigator.onLine ? "server-unreachable" : "device-offline");
  }
}

window.addEventListener("offline", () => setNet("device-offline")); // OS says down — trust it
window.addEventListener("online", () => void probe()); // OS says up — verify first
document.addEventListener("visibilitychange", () => {
  // The checkout moment: the app is re-opened exactly when connectivity is
  // in doubt — re-check on every return to foreground.
  if (document.visibilityState === "visible") void probe();
});
void probe();

// recheckNetwork re-probes after a request failed the way a network failure
// looks (a thrown TypeError — an HTTP status means the server answered, so it
// is not one). Wired into the query/mutation caches in main.tsx.
export function recheckNetwork(err: unknown) {
  if (err instanceof TypeError) void probe();
}

function subscribeNet(cb: () => void) {
  netSubs.add(cb);
  return () => netSubs.delete(cb);
}

export function useNetState(): NetState {
  return useSyncExternalStore(subscribeNet, () => net);
}

export function useOnline(): boolean {
  return useNetState() === "online";
}

// «нет сети» pill under the status bar; data on screen is cache-served. When
// the device has internet and only we are unreachable, the pill also carries
// the way to find out why — the status page lives on separate infrastructure,
// so it answers exactly when the app does not.
export function OfflineChip() {
  const state = useNetState();
  if (state === "online") return null;
  const serverDown = state === "server-unreachable";
  return (
    <div className="pointer-events-none fixed inset-x-0 top-[max(env(safe-area-inset-top),8px)] z-40 flex justify-center">
      <span className="flex items-center gap-1.5 rounded-full border border-gold/25 bg-bg/90 px-3 py-1 text-[10.5px] font-semibold text-gold backdrop-blur">
        {serverDown ? "сервер не отвечает" : "нет сети — данные из кэша"}
        {serverDown && (
          <a
            href={STATUS_URL}
            target="_blank"
            rel="noopener noreferrer"
            className="pointer-events-auto rounded-full bg-gold/15 px-1.5 py-px underline underline-offset-2"
          >
            статус
          </a>
        )}
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

// Chromium fires beforeinstallprompt ONCE, shortly after load — long before
// the user SPA-navigates to «Сервисы». A listener added on Services mount
// misses it every time, so it lives at module scope (main.tsx imports this
// module at startup). iOS Safari never fires it — there the UX is a static
// «Поделиться → На экран „Домой"» hint (Services screen).
let deferredInstall: BeforeInstallPromptEvent | null = null;
const installSubs = new Set<() => void>();

function setDeferredInstall(e: BeforeInstallPromptEvent | null) {
  deferredInstall = e;
  installSubs.forEach((cb) => cb());
}

window.addEventListener("beforeinstallprompt", (e) => {
  e.preventDefault();
  setDeferredInstall(e as BeforeInstallPromptEvent);
});
// Installed via the in-app button or the browser's own UI — the affordance goes.
window.addEventListener("appinstalled", () => setDeferredInstall(null));

function subscribeInstall(cb: () => void) {
  installSubs.add(cb);
  return () => installSubs.delete(cb);
}

export function useInstallPrompt() {
  const canInstall = useSyncExternalStore(subscribeInstall, () => deferredInstall != null);
  const isStandalone =
    window.matchMedia("(display-mode: standalone)").matches ||
    ("standalone" in navigator && (navigator as { standalone?: boolean }).standalone === true);
  const isIOS = /iphone|ipad|ipod/i.test(navigator.userAgent);

  return {
    isStandalone,
    isIOS,
    canInstall,
    install: async () => {
      await deferredInstall?.prompt();
      setDeferredInstall(null);
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
