import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { MutationCache, QueryCache, QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { BrowserRouter, Routes, Route, Outlet } from "react-router-dom";
import { ApiError } from "./api/client";
import { RequireAuth } from "./auth";
import { OfflineChip, recheckNetwork, ReloadPrompt, usePrefetchOffline } from "./pwa";
import DevChip from "./dev/DevChip";
import RecognitionChip from "./components/RecognitionChip";
import NavBar from "./components/NavBar";
import Login from "./pages/Login";
import Register from "./pages/Register";
import Overview from "./pages/Overview";
import PeriodNew from "./pages/PeriodNew";
import Period from "./pages/Period";
import Lookup from "./pages/Lookup";
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
            <Route path="/periods/new" element={<PeriodNew />} />
            <Route path="/periods/:id" element={<Period />} />
            <Route path="/lookup" element={<Lookup />} />
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
