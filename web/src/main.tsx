import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, NavLink, Outlet } from "react-router-dom";
import { ApiError } from "./api/client";
import { RequireAuth, useMe, useLogout } from "./auth";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Dashboard from "./pages/Dashboard";
import PeriodNew from "./pages/PeriodNew";
import Period from "./pages/Period";
import Lookup from "./pages/Lookup";
import Partners from "./pages/Partners";
import "./index.css";

// Client errors (4xx) are answers, not glitches — don't retry them.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
    },
  },
});

function Shell() {
  const me = useMe();
  const logout = useLogout();
  const link = ({ isActive }: { isActive: boolean }) =>
    `rounded-lg px-3 py-1.5 text-sm font-medium ${isActive ? "bg-indigo-600 text-white" : "text-slate-600 hover:bg-slate-200"}`;
  return (
    <div className="mx-auto min-h-screen max-w-3xl">
      <header className="sticky top-0 z-10 border-b border-slate-200 bg-white/90 px-4 py-3 backdrop-blur">
        <div className="flex items-center justify-between gap-2">
          <span className="text-base font-bold text-indigo-700">Sharespences</span>
          <div className="flex items-center gap-2 text-sm text-slate-500">
            <span>{me.data?.display_name}</span>
            <button
              onClick={() => logout.mutate()}
              className="rounded-lg px-2 py-1 text-xs text-slate-400 hover:bg-slate-100"
            >
              Выйти
            </button>
          </div>
        </div>
        <nav className="mt-2 flex gap-1 overflow-x-auto">
          <NavLink to="/" end className={link}>
            Карты
          </NavLink>
          <NavLink to="/lookup" className={link}>
            Какой картой?
          </NavLink>
          <NavLink to="/partners" className={link}>
            Партнёрские
          </NavLink>
        </nav>
      </header>
      <main className="space-y-4 p-4">
        <Outlet />
      </main>
    </div>
  );
}

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route path="/login" element={<Login />} />
          <Route path="/register" element={<Register />} />
          <Route
            element={
              <RequireAuth>
                <Shell />
              </RequireAuth>
            }
          >
            <Route path="/" element={<Dashboard />} />
            <Route path="/periods/new" element={<PeriodNew />} />
            <Route path="/periods/:id" element={<Period />} />
            <Route path="/lookup" element={<Lookup />} />
            <Route path="/partners" element={<Partners />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
