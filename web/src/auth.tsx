import type { ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useLocation, useNavigate } from "react-router-dom";
import { api, unwrap, ApiError } from "./api/client";
import { clearAllRecognitions } from "./recognition";
import { Spinner, ErrMsg } from "./components/ui";

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
      if ("caches" in window) {
        const keys = await caches.keys();
        await Promise.all(keys.map((k) => caches.delete(k)));
      }
      navigate("/login");
    },
  });
}
