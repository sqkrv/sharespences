import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, Outlet, Navigate, useSearchParams } from "react-router-dom";
import { ApiError } from "./api/client";
import { RequireAuth } from "./auth";
import { OfflineChip, ReloadPrompt, usePrefetchOffline } from "./pwa";
import DevChip from "./dev/DevChip";
import RecognitionChip from "./components/RecognitionChip";
import NavBar from "./components/NavBar";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Overview from "./pages/Overview";
import Banks from "./pages/Banks";
import BankNew from "./pages/BankNew";
import PeriodNew from "./pages/PeriodNew";
import Period from "./pages/Period";
import Search from "./pages/Search";
import Pos from "./pages/Pos";
import Partners from "./pages/Partners";
import Friends from "./pages/Friends";
import FriendsSettings from "./pages/FriendsSettings";
import FriendJoin from "./pages/FriendJoin";
import Services from "./pages/Services";
import Stub from "./pages/Stub";
import "./index.css";

// Client errors (4xx) are answers, not glitches — don't retry them.
// networkMode offlineFirst: with the default 'online', navigator.onLine=false
// pauses queries and no fetch ever reaches the service worker — offline read
// (docs/specs/pwa.md) depends on this. Mutations keep the 'online' default:
// paused-not-lost is the right behavior for writes.
const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      networkMode: "offlineFirst",
      retry: (count, err) => !(err instanceof ApiError && err.status < 500) && count < 2,
    },
  },
});

// The retired /lookup URL, kept as a redirect for cached shells/bookmarks.
function LegacyLookup() {
  const [params] = useSearchParams();
  const cat = params.get("cat");
  return <Navigate replace to={cat ? `/pos?cat=${cat}` : "/search"} />;
}

// Phone-shaped shell per the design: content column + fixed bottom navbar.
// Top padding honors the status bar in standalone PWA (viewport-fit=cover).
function Shell() {
  usePrefetchOffline();
  return (
    <div className="mx-auto min-h-dvh max-w-md">
      <main className="space-y-4 px-4 pt-[max(env(safe-area-inset-top),1rem)] pb-28">
        <Outlet />
      </main>
      <NavBar />
      <OfflineChip />
      <RecognitionChip />
      <ReloadPrompt />
      <DevChip />
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
            <Route path="/banks" element={<Banks />} />
            <Route path="/banks/new" element={<BankNew />} />
            <Route path="/periods/new" element={<PeriodNew />} />
            <Route path="/periods/:id" element={<Period />} />
            <Route path="/search" element={<Search />} />
            <Route path="/pos" element={<Pos />} />
            {/* The old CB-04 address — cached PWA shells and bookmarks still
                open it: a category deep link becomes the POS view, the rest
                lands on the search screen. */}
            <Route path="/lookup" element={<LegacyLookup />} />
            <Route path="/partners" element={<Partners />} />
            <Route path="/friends" element={<Friends />} />
            <Route path="/friends/settings" element={<FriendsSettings />} />
            <Route path="/friends/join/:token" element={<FriendJoin />} />
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
