import { matchPath } from "react-router-dom";

// Screen IDs (docs/design/ui-preferences.md §Dev mode). A shared vocabulary:
// «поправь CB-03» beats describing a screen in
// prose. Prefix = module (CB кешбек, GR группы, HM главная, HS история,
// SYS системные, MD модерация), so the numbers never collide the way the
// specs' old S<n> labels did across cashback.md and group-expenses.md.
//
// This array is the ONLY list of screen IDs — it is what renders the chip,
// so it cannot rot silently. Sub-region IDs (CB-01.a…) are deliberately not
// here: they live as `data-sid` literals in the JSX, one `grep -rn CB-01.a
// web/src` away, so there is no second inventory to keep in sync.

export type Screen = { id: string; path: string; title: string; file: string };

export const SCREENS: Screen[] = [
  { id: "SYS-01", path: "/login", title: "Вход", file: "web/src/pages/Login.tsx" },
  { id: "SYS-02", path: "/register", title: "Регистрация", file: "web/src/pages/Register.tsx" },
  { id: "SYS-03", path: "/services", title: "Сервисы", file: "web/src/pages/Services.tsx" },
  { id: "MD-01", path: "/moderation", title: "Модерация", file: "web/src/pages/Moderation.tsx" },
  { id: "CB-01", path: "/", title: "Кешбек — лента", file: "web/src/pages/Overview.tsx" },
  { id: "CB-02", path: "/periods/new", title: "Меню месяца", file: "web/src/pages/PeriodNew.tsx" },
  { id: "CB-03", path: "/periods/:id", title: "Меню банка", file: "web/src/pages/Period.tsx" },
  { id: "CB-04", path: "/search", title: "Поиск", file: "web/src/pages/Search.tsx" },
  { id: "CB-09", path: "/banks", title: "Банки и карты", file: "web/src/pages/Banks.tsx" },
  { id: "CB-10", path: "/banks/new", title: "Новый банк", file: "web/src/pages/BankNew.tsx" },
  // /pos/new before /pos: the router matches in order and so does this table.
  { id: "CB-13", path: "/pos/new", title: "Новая точка", file: "web/src/pages/PosNew.tsx" },
  { id: "CB-11", path: "/pos", title: "Точка продаж", file: "web/src/pages/Pos.tsx" },
  // CB-05 (the partner-offer list screen) dissolved into the bank cards on
  // CB-09 + the CB-12 form (партнёрки v2, 2026-08-06); /partners redirects.
  { id: "CB-12", path: "/partners/new", title: "Партнёрское предложение", file: "web/src/pages/PartnerNew.tsx" },
  { id: "CB-06", path: "/friends", title: "Кешбек друзей", file: "web/src/pages/Friends.tsx" },
  { id: "CB-07", path: "/friends/settings", title: "Друзья и шэринг", file: "web/src/pages/FriendsSettings.tsx" },
  // /join is the short form printed on invites (4e); /friends/join is the
  // pre-00025 spelling old links still carry.
  { id: "CB-08", path: "/join/:token", title: "Приглашение в друзья", file: "web/src/pages/FriendJoin.tsx" },
  { id: "CB-08", path: "/friends/join/:token", title: "Приглашение в друзья", file: "web/src/pages/FriendJoin.tsx" },
  { id: "HM-01", path: "/home", title: "Главная (заглушка)", file: "web/src/pages/Stub.tsx" },
  { id: "GR-01", path: "/groups", title: "Группы (заглушка)", file: "web/src/pages/Stub.tsx" },
  { id: "HS-01", path: "/history", title: "История (заглушка)", file: "web/src/pages/Stub.tsx" },
];

// Shared widgets keep one ID wherever they appear, so «W-01» always means the
// month picker. That is what spares the components a `sid` prop from every
// parent screen; they carry their `data-sid` literal themselves.
//   W-01 components/MonthPicker.tsx (v2: bottom sheet with fill logos)
//   W-02 components/CategoryPicker.tsx
//   W-03 components/NavBar.tsx
//   W-04 components/Lightbox.tsx
//   W-05 components/Sheet.tsx (shared bottom sheet)
//
// Static pages live outside the SPA entirely — plain HTML in web/public/,
// served by internal/web/web.go at their extensionless URLs. React never
// mounts on them, so they cannot carry a `data-sid` or render the chip and
// they are deliberately absent from SCREENS above; the IDs exist only so the
// vocabulary covers every screen a user can reach.
//   SYS-04 /privacy  web/public/privacy.html
//   SYS-05 /terms    web/public/terms.html

// `/periods/new` must win over `/periods/:id` — the router matches in order
// and so does this table (SCREENS lists the literal first).
export function screenFor(pathname: string): Screen | undefined {
  return SCREENS.find((s) => matchPath({ path: s.path, end: true }, pathname));
}
