import { matchPath } from "react-router-dom";

// Screen IDs (docs/design/ui-preferences.md §Dev mode). The vocabulary the
// owner and the agent share: «поправь CB-03» beats describing a screen in
// prose. Prefix = module (CB кешбек, GR группы, HM главная, HS история,
// SYS системные), so the numbers never collide the way the specs' old S<n>
// labels did across cashback.md and group-expenses.md.
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
  { id: "CB-01", path: "/", title: "Кешбек — обзор", file: "web/src/pages/Overview.tsx" },
  { id: "CB-02", path: "/periods/new", title: "Новый период", file: "web/src/pages/PeriodNew.tsx" },
  { id: "CB-03", path: "/periods/:id", title: "Период", file: "web/src/pages/Period.tsx" },
  { id: "CB-04", path: "/lookup", title: "Какой картой платить?", file: "web/src/pages/Lookup.tsx" },
  { id: "CB-05", path: "/partners", title: "Партнёрские предложения", file: "web/src/pages/Partners.tsx" },
  { id: "HM-01", path: "/home", title: "Главная (заглушка)", file: "web/src/pages/Stub.tsx" },
  { id: "GR-01", path: "/groups", title: "Группы (заглушка)", file: "web/src/pages/Stub.tsx" },
  { id: "HS-01", path: "/history", title: "История (заглушка)", file: "web/src/pages/Stub.tsx" },
];

// Shared widgets keep one ID wherever they appear, so «W-01» always means the
// month picker. That is what spares the components a `sid` prop from every
// parent screen; they carry their `data-sid` literal themselves.
//   W-01 components/MonthPicker.tsx
//   W-02 components/CategoryPicker.tsx
//   W-03 components/NavBar.tsx
//
// Static pages live outside the SPA entirely — plain HTML in web/public/,
// served by internal/web/web.go at their extensionless URLs. React never
// mounts on them, so they cannot carry a `data-sid` or render the chip and
// they are deliberately absent from SCREENS above; the IDs exist only so the
// vocabulary covers every screen a user can reach.
//   SYS-04 /privacy  web/public/privacy.html
//   SYS-05 /terms    web/public/terms.html

// `/periods/new` must win over `/periods/:id` — the router matches in order
// and so do we (SCREENS lists the literal first).
export function screenFor(pathname: string): Screen | undefined {
  return SCREENS.find((s) => matchPath({ path: s.path, end: true }, pathname));
}
