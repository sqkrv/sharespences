import type { ReactNode } from "react";
import { ApiError } from "../api/client";

export function Section({ title, children }: { title?: string; children: ReactNode }) {
  return (
    <section className="rounded-xl bg-white p-4 shadow-sm">
      {title && <h2 className="mb-3 text-sm font-semibold uppercase tracking-wide text-slate-500">{title}</h2>}
      {children}
    </section>
  );
}

export function Btn({
  children,
  variant = "primary",
  ...props
}: React.ButtonHTMLAttributes<HTMLButtonElement> & { variant?: "primary" | "ghost" | "danger" }) {
  const styles = {
    primary: "bg-indigo-600 text-white hover:bg-indigo-700 disabled:bg-slate-300",
    ghost: "bg-slate-100 text-slate-700 hover:bg-slate-200 disabled:text-slate-400",
    danger: "bg-rose-50 text-rose-700 hover:bg-rose-100 disabled:text-slate-400",
  }[variant];
  return (
    <button
      {...props}
      className={`rounded-lg px-3 py-2 text-sm font-medium transition disabled:cursor-not-allowed ${styles} ${props.className ?? ""}`}
    >
      {children}
    </button>
  );
}

export function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <label className="block">
      <span className="mb-1 block text-xs font-medium text-slate-500">{label}</span>
      {children}
    </label>
  );
}

const inputCls =
  "w-full rounded-lg border border-slate-300 bg-white px-3 py-2 text-sm focus:border-indigo-500 focus:outline-none";

export function Input(props: React.InputHTMLAttributes<HTMLInputElement>) {
  return <input {...props} className={`${inputCls} ${props.className ?? ""}`} />;
}

export function Select(props: React.SelectHTMLAttributes<HTMLSelectElement>) {
  return <select {...props} className={`${inputCls} ${props.className ?? ""}`} />;
}

export function Badge({ children, tone = "slate" }: { children: ReactNode; tone?: "slate" | "amber" | "green" | "indigo" }) {
  const tones = {
    slate: "bg-slate-100 text-slate-600",
    amber: "bg-amber-100 text-amber-800",
    green: "bg-emerald-100 text-emerald-800",
    indigo: "bg-indigo-100 text-indigo-800",
  }[tone];
  return <span className={`inline-block rounded-full px-2 py-0.5 text-xs font-medium ${tones}`}>{children}</span>;
}

export function ErrMsg({ error }: { error: unknown }) {
  if (!error) return null;
  const msg = error instanceof ApiError ? error.message : error instanceof Error ? error.message : String(error);
  return <p className="mt-2 rounded-lg bg-rose-50 px-3 py-2 text-sm text-rose-700">{msg}</p>;
}

export function Spinner() {
  return <p className="p-6 text-center text-sm text-slate-400">Загрузка…</p>;
}

export function Empty({ children }: { children: ReactNode }) {
  return <p className="rounded-lg bg-slate-50 px-3 py-4 text-center text-sm text-slate-500">{children}</p>;
}
