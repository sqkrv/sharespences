// Hand-written client for the sidecar API (deliberately no codegen: the
// admin API is outside the public contract — docs/specs/admin.md).
import { useCallback, useEffect, useState } from "react";

export class ApiError extends Error {
  status: number;
  constructor(status: number, body: unknown) {
    const b = body as { detail?: string; title?: string } | null;
    super(b?.detail || b?.title || `Ошибка ${status}`);
    this.status = status;
  }
}

export async function request<T>(method: string, path: string, body?: unknown): Promise<T> {
  const res = await fetch(path, {
    method,
    headers: body !== undefined ? { "Content-Type": "application/json" } : undefined,
    body: body !== undefined ? JSON.stringify(body) : undefined,
  });
  if (!res.ok) {
    let parsed: unknown = null;
    try {
      parsed = await res.json();
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(res.status, parsed);
  }
  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export function errorText(e: unknown): string {
  if (e instanceof ApiError) return e.message;
  if (e instanceof TypeError) return "Нет связи с сервером — проверь туннель";
  return "Что-то пошло не так — попробуй ещё раз";
}

/** Minimal read hook: refetches when `path` changes; `reload` after writes. */
export function useFetch<T>(path: string) {
  const [data, setData] = useState<T | null>(null);
  const [error, setError] = useState<unknown>(null);
  const [tick, setTick] = useState(0);
  const reload = useCallback(() => setTick((t) => t + 1), []);
  useEffect(() => {
    let live = true;
    setError(null);
    request<T>("GET", path)
      .then((d) => live && setData(d))
      .catch((e) => live && setError(e));
    return () => {
      live = false;
    };
  }, [path, tick]);
  return { data, error, reload };
}

// --- DTOs (hand-typed mirrors of internal/admin/http.go) ---

export interface Bank {
  id: number;
  name: string;
  color_hex?: string;
}

export interface Canonical {
  id: number;
  slug: string;
  title_ru: string;
  emoji?: string;
}

export interface Alias {
  bank_id: number;
  bank_name: string;
  raw_title: string;
  canonical_category_id: number;
  canonical_slug: string;
  canonical_title: string;
}

export interface Program {
  id: number;
  bank_id: number;
  bank_name: string;
  name: string;
  period_type: string;
  selection_mode: string;
  currency_kind: string;
  points_label?: string;
  selection_opens_day?: number;
  notes?: string;
}

export interface Tier {
  id: number;
  name: string;
  is_paid_subscription: boolean;
  cap_value?: string;
  cap_scope: string;
  cap_per_category?: string;
  max_categories?: number;
  notes?: string;
}

export interface BankCategory {
  id: number;
  bank_id: number;
  bank_name: string;
  title: string;
  canonical_category_id?: number;
  canonical_slug?: string;
  canonical_title?: string;
  kind: string;
  emoji?: string;
  is_custom: boolean;
  active: boolean;
  mcc_count: number;
  seed_managed: boolean;
}

export interface MCC {
  code: number;
  name: string;
  description?: string;
  seed_managed: boolean;
}

export interface MCCLink {
  mcc_code: number;
  mcc_name: string;
  note?: string;
}

export interface MCCLinks {
  seed_managed: boolean;
  links: MCCLink[];
}

export interface MCCChange {
  id: number;
  bank_id: number;
  bank_name: string;
  bank_category_id?: number;
  category_title: string;
  mcc_code?: number;
  action: string;
  noted_at: string;
  source: string;
  note?: string;
}

export interface POS {
  id: string;
  name: string;
  merchant_title?: string;
  mcc_code?: number;
  type?: string;
  address?: string;
  confirmations?: number;
  created_at: string;
  last_confirmed_at?: string;
}

export interface Page<T> {
  total: number;
  items: T[];
}

export interface Dashboard {
  db_size_bytes: number;
  counts: Record<string, number>;
  tables: { table: string; rows: number }[];
  migrations: { version: number; source: string; applied_at?: string }[];
}
