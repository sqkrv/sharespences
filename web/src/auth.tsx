import type { ReactNode } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Navigate, useNavigate } from "react-router-dom";
import { api, unwrap, ApiError } from "./api/client";
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
// HttpOnly; the API answer is the only source of truth).
export function RequireAuth({ children }: { children: ReactNode }) {
  const me = useMe();
  if (me.isPending) return <Spinner />;
  if (me.isError) {
    if (me.error instanceof ApiError && me.error.status === 401) {
      return <Navigate to="/login" replace />;
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
