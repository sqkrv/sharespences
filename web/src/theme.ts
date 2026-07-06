// Theme per docs/design/ui-preferences.md (meta-repo): dark-first, system-
// dependent, persistent manual override. Three-state System/Light/Dark;
// System resolves to light ONLY on an explicit OS light preference — dark
// is the fallback. The initial class is set by an inline script in
// index.html before first paint; this module keeps it in sync afterwards.
import { useEffect, useState } from "react";

export type ThemeSetting = "system" | "light" | "dark";

const KEY = "theme";

export function currentSetting(): ThemeSetting {
  const v = localStorage.getItem(KEY);
  return v === "light" || v === "dark" ? v : "system";
}

function systemPrefersLight(): boolean {
  return window.matchMedia("(prefers-color-scheme: light)").matches;
}

function syncClass(setting: ThemeSetting) {
  const dark = setting === "dark" || (setting === "system" && !systemPrefersLight());
  document.documentElement.classList.toggle("dark", dark);
}

export function useTheme(): [ThemeSetting, (s: ThemeSetting) => void] {
  const [setting, setSetting] = useState<ThemeSetting>(currentSetting);

  useEffect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: light)");
    const onChange = () => syncClass(currentSetting());
    mq.addEventListener("change", onChange);
    return () => mq.removeEventListener("change", onChange);
  }, []);

  const set = (s: ThemeSetting) => {
    if (s === "system") localStorage.removeItem(KEY);
    else localStorage.setItem(KEY, s);
    syncClass(s);
    setSetting(s);
  };
  return [setting, set];
}
