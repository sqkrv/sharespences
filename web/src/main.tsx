import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, Outlet, Navigate, useSearchParams } from "react-router-dom";
import { ApiError } from "./api/client";
import { RequireAuth } from "./auth";
import { OfflineChip, recheckNetwork, ReloadPrompt, usePrefetchOffline } from "./pwa";
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
import Mcc from "./pages/Mcc";
import PosNew from "./pages/PosNew";
import PartnerNew from "./pages/PartnerNew";
import Friends from "./pages/Friends";
import FriendsSettings from "./pages/FriendsSettings";
import FriendJoin from "./pages/FriendJoin";
import Perks from "./pages/Perks";
import Perk from "./pages/Perk";
import Services from "./pages/Services";
import Moderation from "./pages/Moderation";
import Stub from "./pages/Stub";
import "./index.css";

// Client errors (4xx) are answers, not glitches — don't retry them.
// networkMode offlineFirst: with the default 'online', navigator.onLine=false
// pauses queries and no fetch ever reaches the service worker — offline read
// (docs/specs/pwa.md) depends on this. Mutations keep the 'online' default:
// paused-not-lost is the right behavior for writes.
// A request that dies before reaching the server is first-hand evidence the
// probe has not caught up with yet (it re-runs on foreground return, or every
// 15s once already offline). Re-probing on such a failure is what makes the
// chip — and with it the link to the status page — appear at the moment the
// user hits the outage, instead of on their next app switch.
const queryClient = new QueryClient({
  queryCache: new QueryCache({ onError: (err) => recheckNetwork(err) }),
  mutationCache: new MutationCache({ onError: (err) => recheckNetwork(err) }),
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
            <Route path="/pos/new" element={<PosNew />} />
            <Route path="/pos" element={<Pos />} />
            <Route path="/mcc/:code" element={<Mcc />} />
            {/* The old CB-04 address — cached PWA shells and bookmarks still
                open it: a category deep link becomes the POS view, the rest
                lands on the search screen. */}
            <Route path="/lookup" element={<LegacyLookup />} />
            {/* Партнёрки live on the bank cards since v2; the old list
                address forwards there. */}
            <Route path="/partners" element={<Navigate replace to="/banks" />} />
            <Route path="/partners/new" element={<PartnerNew />} />
            <Route path="/friends" element={<Friends />} />
            <Route path="/friends/settings" element={<FriendsSettings />} />
            {/* /join is the short form printed on the invite (4e); the old
                /friends/join links keep working. */}
            <Route path="/join/:token" element={<FriendJoin />} />
            <Route path="/friends/join/:token" element={<FriendJoin />} />
            <Route path="/perks" element={<Perks />} />
            <Route path="/perks/:perkId" element={<Perk />} />
            <Route path="/services" element={<Services />} />
            <Route path="/moderation" element={<Moderation />} />
            <Route path="/home" element={<Stub title="Главная" />} />
            <Route path="/groups" element={<Stub title="Группы" />} />
            <Route path="/history" element={<Stub title="История" />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  </StrictMode>,
);
