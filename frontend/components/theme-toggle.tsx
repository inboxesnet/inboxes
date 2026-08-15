"use client";

import { useTheme } from "next-themes";
import { Sun, Moon, Monitor } from "lucide-react";

// Theme cycles light → dark → system. "System" follows the OS setting;
// without it one click permanently locked the user out of system-follow.
const THEME_ORDER = ["light", "dark", "system"] as const;

export function nextTheme(theme: string | undefined): string {
  const current = THEME_ORDER.indexOf((theme ?? "system") as (typeof THEME_ORDER)[number]);
  return THEME_ORDER[(current + 1) % THEME_ORDER.length];
}

export function themeLabel(theme: string | undefined): string {
  switch (theme) {
    case "light":
      return "Theme: Light";
    case "dark":
      return "Theme: Dark";
    default:
      return "Theme: System";
  }
}

export function ThemeIcon({ theme, className }: { theme: string | undefined; className?: string }) {
  if (theme === "dark") return <Moon className={className} />;
  if (theme === "light") return <Sun className={className} />;
  return <Monitor className={className} />;
}

export function ThemeToggle() {
  const { theme, setTheme } = useTheme();

  return (
    <button
      onClick={() => setTheme(nextTheme(theme))}
      className="flex items-center justify-center h-8 w-8 rounded-full bg-muted text-muted-foreground hover:bg-accent transition-colors"
      title={themeLabel(theme)}
    >
      <ThemeIcon theme={theme} className="h-4 w-4" />
    </button>
  );
}
