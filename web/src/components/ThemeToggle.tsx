import { useEffect, useState } from "react";
import { applyTheme, getTheme, THEME_EVENT } from "../theme";
import type { ThemeChoice } from "../theme";
import Icon from "./Icon";
import type { IconName } from "./Icon";

const themeOptions: { value: ThemeChoice; icon: IconName; label: string }[] = [
  { value: "system", icon: "monitor", label: "System theme" },
  { value: "light", icon: "sun", label: "Light theme" },
  { value: "dark", icon: "moon", label: "Dark theme" },
];

export default function ThemeToggle({ compact }: { compact?: boolean }) {
  const [theme, setTheme] = useState<ThemeChoice>(getTheme);

  // Two toggles can be on screen (the sidebar and the phone bar): each one
  // follows a change the other made.
  useEffect(() => {
    const follow = (): void => setTheme(getTheme());
    window.addEventListener(THEME_EVENT, follow);
    return () => window.removeEventListener(THEME_EVENT, follow);
  }, []);

  function choose(choice: ThemeChoice): void {
    applyTheme(choice);
    setTheme(choice);
  }

  if (compact === true) {
    const next = theme === "dark" ? "light" : "dark";
    return (
      <button
        type="button"
        className="icon-btn"
        aria-label={`Switch to the ${next} theme`}
        onClick={() => choose(next)}
      >
        <Icon name={theme === "dark" ? "sun" : "moon"} />
      </button>
    );
  }

  return (
    <fieldset className="theme-toggle">
      <legend className="visually-hidden">Theme</legend>
      {themeOptions.map((option) => (
        <button
          key={option.value}
          type="button"
          aria-pressed={theme === option.value}
          aria-label={option.label}
          title={option.label}
          className={theme === option.value ? "active" : undefined}
          onClick={() => choose(option.value)}
        >
          <Icon name={option.icon} size={15} />
        </button>
      ))}
    </fieldset>
  );
}
