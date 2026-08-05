import { NavLink, Navigate, Route, Routes } from "react-router-dom";
import { useFetch } from "./api";
import Dashboard from "./pages/Dashboard";
import Banks from "./pages/Banks";
import Canonicals from "./pages/Canonicals";
import Catalog from "./pages/Catalog";
import MCCPage from "./pages/MCC";
import Journal from "./pages/Journal";
import Pos from "./pages/Pos";

const NAV = [
  { to: "/", label: "Дашборд" },
  { to: "/banks", label: "Банки" },
  { to: "/canonicals", label: "Категории" },
  { to: "/catalog", label: "Каталог" },
  { to: "/mcc", label: "MCC" },
  { to: "/journal", label: "Журнал" },
  { to: "/pos", label: "Точки продаж" },
];

export default function App() {
  const { data: version } = useFetch<{ version: string }>("/api/version");
  return (
    <div className="mx-auto flex min-h-dvh max-w-6xl">
      <aside className="flex w-52 shrink-0 flex-col gap-1 border-r border-brd p-4">
        <p className="mb-3 text-sm font-bold">
          Sharespences <span className="text-tx3">· Админ</span>
        </p>
        {NAV.map((n) => (
          <NavLink
            key={n.to}
            to={n.to}
            end={n.to === "/"}
            className={({ isActive }) =>
              `rounded-lg px-3 py-1.5 text-sm ${isActive ? "bg-inset font-semibold text-accl" : "text-tx2 hover:text-tx"}`
            }
          >
            {n.label}
          </NavLink>
        ))}
        <div className="mt-auto pt-4 text-[11px] text-tx4">
          <p>{version?.version ?? "…"}</p>
          <a href="/docs" className="hover:text-tx2">
            API-документация
          </a>
        </div>
      </aside>
      <main className="min-w-0 flex-1 space-y-4 p-5">
        <Routes>
          <Route path="/" element={<Dashboard />} />
          <Route path="/banks" element={<Banks />} />
          <Route path="/canonicals" element={<Canonicals />} />
          <Route path="/catalog" element={<Catalog />} />
          <Route path="/mcc" element={<MCCPage />} />
          <Route path="/journal" element={<Journal />} />
          <Route path="/pos" element={<Pos />} />
          <Route path="*" element={<Navigate to="/" replace />} />
        </Routes>
      </main>
    </div>
  );
}
