import { NavLink } from "react-router-dom";

// The design's five-tab bottom navbar. Only «Кешбек» is a built module;
// the rest route to honest «в разработке» stubs (2026-07-09).
const TABS: { to: string; label: string; icon: React.ReactNode }[] = [
  {
    to: "/home",
    label: "Главная",
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
        <path d="M3.5 10.6 12 4l8.5 6.6" />
        <path d="M5.6 9.5V20h12.8V9.5" />
      </svg>
    ),
  },
  {
    to: "/",
    label: "Кешбек",
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round">
        <line x1="6.5" y1="17.5" x2="17.5" y2="6.5" />
        <circle cx="8" cy="8" r="1.9" />
        <circle cx="16" cy="16" r="1.9" />
      </svg>
    ),
  },
  {
    to: "/groups",
    label: "Группы",
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="8.5" cy="8.5" r="3.1" />
        <circle cx="16.5" cy="9.5" r="2.5" />
        <path d="M2.6 19c0-3.2 2.7-5 5.9-5 1.7 0 3.2.5 4.2 1.5" />
        <path d="M14.4 15.2c.6-.3 1.4-.5 2.3-.5 2.7 0 4.7 1.6 4.7 4.3" />
      </svg>
    ),
  },
  {
    to: "/services",
    label: "Сервисы",
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
        <rect x="4" y="4" width="6.5" height="6.5" rx="1.8" />
        <rect x="13.5" y="4" width="6.5" height="6.5" rx="1.8" />
        <rect x="4" y="13.5" width="6.5" height="6.5" rx="1.8" />
        <rect x="13.5" y="13.5" width="6.5" height="6.5" rx="1.8" />
      </svg>
    ),
  },
  {
    to: "/history",
    label: "История",
    icon: (
      <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.7" strokeLinecap="round" strokeLinejoin="round">
        <circle cx="12" cy="12" r="8.4" />
        <path d="M12 7.2v5l3.4 2" />
      </svg>
    ),
  },
];

export default function NavBar() {
  return (
    <nav className="fixed inset-x-0 bottom-0 z-20 mx-auto max-w-md border-t border-brd bg-bg/85 pb-[max(env(safe-area-inset-bottom),10px)] backdrop-blur">
      <div data-sid="W-03" className="flex items-end gap-0.5 px-1.5 pt-2">
        {TABS.map((t) => (
          <NavLink
            key={t.to}
            to={t.to}
            end={t.to === "/"}
            className={({ isActive }) =>
              `flex flex-1 flex-col items-center gap-1 ${isActive ? "text-accl" : "text-tx4"}`
            }
          >
            {({ isActive }) => (
              <>
                <span className={`h-1 w-1 rounded-full ${isActive ? "bg-acc" : "bg-transparent"}`} />
                {t.icon}
                <span className={`text-[9.5px] ${isActive ? "font-bold" : "font-semibold"}`}>{t.label}</span>
              </>
            )}
          </NavLink>
        ))}
      </div>
    </nav>
  );
}
