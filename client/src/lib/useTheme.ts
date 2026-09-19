import { useEffect, useState } from "react";

/** Workbench = light, Cyberdeck Night = dark. Persisted locally. */
export type ThemeKey = "workbench" | "cyberdeck";

const THEME_STORAGE_KEY = "serverhub.theme";

function initial(): ThemeKey {
  try {
    return localStorage.getItem(THEME_STORAGE_KEY) === "cyberdeck"
      ? "cyberdeck"
      : "workbench";
  } catch {
    return "workbench";
  }
}

export function useTheme() {
  const [theme, setTheme] = useState<ThemeKey>(initial);

  useEffect(() => {
    const dark = theme === "cyberdeck";
    document.documentElement.classList.toggle("dark", dark);
    document.documentElement.style.colorScheme = dark ? "dark" : "light";
    try {
      localStorage.setItem(THEME_STORAGE_KEY, theme);
    } catch {
      /* private mode — theme just won't persist */
    }
  }, [theme]);

  return { theme, setTheme, isDark: theme === "cyberdeck" };
}
