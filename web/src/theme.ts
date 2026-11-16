// The theme: follow the system, or a choice the reader made. The choice is a
// per-browser convenience, so it lives in localStorage and the page works the
// same when storage is blocked.

export type ThemeChoice = "system" | "light" | "dark";

const THEME_KEY = "eznit.theme";

// THEME_EVENT tells every toggle on the page that the choice changed.
export const THEME_EVENT = "eznit-theme";

export function getTheme(): ThemeChoice {
  try {
    const stored = window.localStorage.getItem(THEME_KEY);
    if (stored === "light" || stored === "dark") {
      return stored;
    }
  } catch {
    // storage blocked: the system decides
  }
  return "system";
}

export function applyTheme(choice: ThemeChoice): void {
  const root = document.documentElement;
  if (choice === "system") {
    delete root.dataset.theme;
  } else {
    root.dataset.theme = choice;
  }
  try {
    if (choice === "system") {
      window.localStorage.removeItem(THEME_KEY);
    } else {
      window.localStorage.setItem(THEME_KEY, choice);
    }
  } catch {
    // the choice lasts as long as the page
  }
  window.dispatchEvent(new Event(THEME_EVENT));
}
