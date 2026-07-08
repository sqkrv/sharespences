import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, Outlet } from "react-router-dom";
import { ApiError } from "./api/client";
import { RequireAuth } from "./auth";
import NavBar from "./components/NavBar";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Overview from "./pages/Overview";
import PeriodNew from "./pages/PeriodNew";
import Period from "./pages/Period";
import Lookup from "./pages/Lookup";
import Partners from "./pages/Partners";
import Services from "./pages/Services";
import Stub from "./pages/Stub";
import "./index.css";

// Client errors (4xx) are answers, not glitches — don't retry them.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
    },
  },
});

// Phone-shaped shell per the design: content column + fixed bottom navbar.
function Shell() {
  return (
    <div className="mx-auto min-h-dvh max-w-md">
      <main className="space-y-4 px-4 pt-4 pb-28">
        <Outlet />
      </main>
      <NavBar />
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
            <Route path="/" element={<Overview />} />
            <Route path="/periods/new" element={<PeriodNew />} />
            <Route path="/periods/:id" element={<Period />} />
            <Route path="/lookup" element={<Lookup />} />
            <Route path="/partners" element={<Partners />} />
            <Route path="/services" element={<Services />} />
            <Route path="/home" element={<Stub title="Главная" />} />
            <Route path="/groups" element={<Stub title="Группы" />} />
            <Route path="/history" element={<Stub title="История" />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
