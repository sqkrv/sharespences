import type { ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { api, unwrap, ApiError } from "./api/client";
import { clearAllRecognitions } from "./recognition";
import { Spinner, ErrMsg } from "./components/ui";

// purgeResponseCaches drops every Cache Storage entry. The service worker
// caches read endpoints NetworkFirst with ignoreVary (scs stamps Vary: Cookie,
// which would otherwise defeat the cache — see docs/specs/pwa.md), so entries
// are keyed by URL alone and carry no notion of who they belong to. Logout
// clears them; sign-in has to clear them too, because the previous user may
// simply have closed the tab, and then a slow network on the next sign-in
// would answer from their cached data.
//
// Recognition drafts are deliberately left to logout: they are the user's own
// unfinished work, and a returning user would lose it.
export async function purgeResponseCaches() {
  if (!("caches" in window)) return;
  const keys = await caches.keys();
  await Promise.all(keys.map((k) => caches.delete(k)));
}

export function useMe() {
  return useQuery({
    queryKey: ["me"],
    queryFn: async () => unwrap(await api.GET("/api/v1/auth/me")),
    retry: false,
    staleTime: 5 * 60_000,
  });
}

// RequireAuth gates the app shell: 401 → /login (the session cookie is
// HttpOnly; the API answer is the only source of truth). The interrupted
// location rides along as router state — in memory only, never storage —
// so an invite link (/friends/join/:token) survives the login round-trip.
export function RequireAuth({ children }: { children: ReactNode }) {
  const me = useMe();
  const location = useLocation();
  if (me.isPending) return <Spinner />;
  if (me.isError) {
    if (me.error instanceof ApiError && me.error.status === 401) {
      return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
    }
    return <ErrMsg error={me.error} />;
  }
  return children;
}

export function useLogout() {
  const qc = useQueryClient();
  const navigate = useNavigate();
  return useMutation({
    mutationFn: async () => unwrap(await api.POST("/api/v1/auth/logout")),
    onSuccess: async () => {
      qc.clear();
      // Same rule for recognition drafts: they hold the bank client, the
      // screenshot ids and the read menu itself.
      clearAllRecognitions();
      // Offline-read caches hold personal data (держатели, last-4) — they
      // must not survive logout on a shared device (docs/specs/pwa.md).
      await purgeResponseCaches();
      navigate("/login");
    },
  });
}
