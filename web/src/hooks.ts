import { useQuery } from "@tanstack/react-query";
import { api, unwrap, type Program, type Tier } from "./api/client";

export function useBanks() {
  return useQuery({
    queryKey: ["banks"],
    queryFn: async () => unwrap(await api.GET("/api/v1/banks")) ?? [],
    staleTime: 5 * 60_000,
  });
}

// Bank clients (person × bank): держатель + tier live here; КБ selections
// are keyed by the client — all its cards share them.
export function useClients() {
  return useQuery({
    queryKey: ["clients"],
    queryFn: async () => unwrap(await api.GET("/api/v1/bank-clients")) ?? [],
  });
}

export function useCards() {
  return useQuery({
    queryKey: ["cards"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cards")) ?? [],
  });
}

export function useCategories() {
  return useQuery({
    queryKey: ["categories"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/canonical-categories")) ?? [],
    staleTime: 5 * 60_000,
  });
}

export function usePrograms() {
  return useQuery({
    queryKey: ["programs"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/programs")) ?? [],
    staleTime: 5 * 60_000,
  });
}

export type TierInfo = { tier: Tier; program: Program };

// tier id → {tier, program}: one cached join of the seeded reference data —
// resolves a bank client's caps/slots/currency for defaults and display.
export function useTierMap() {
  return useQuery({
    queryKey: ["tiermap"],
    queryFn: async () => {
      const programs = unwrap(await api.GET("/api/v1/cashback/programs")) ?? [];
      const map = new Map<number, TierInfo>();
      await Promise.all(
        programs.map(async (p) => {
          const tiers =
            unwrap(
              await api.GET("/api/v1/cashback/programs/{id}/tiers", {
                params: { path: { id: p.id } },
              }),
            ) ?? [];
          for (const t of tiers) map.set(t.id, { tier: t, program: p });
        }),
      );
      return map;
    },
    staleTime: 5 * 60_000,
  });
}

export function usePeriods() {
  return useQuery({
    queryKey: ["periods"],
    queryFn: async () => unwrap(await api.GET("/api/v1/cashback/offer-periods")) ?? [],
  });
}
