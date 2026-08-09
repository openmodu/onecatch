export const THEME_STORAGE_KEY = "oneshot.appearance.theme";
export const ACCENT_STORAGE_KEY = "oneshot.appearance.accent";

export const themeModes = ["system", "light", "dark"];
export const accentThemes = ["forest", "ocean", "violet", "amber"];

export function normalizeThemeMode(value) {
  return themeModes.includes(value) ? value : "system";
}

export function normalizeAccentTheme(value) {
  return accentThemes.includes(value) ? value : "forest";
}

function read(storage, key) {
  try { return storage?.getItem(key); } catch { return null; }
}

export function readAppearance(storage = typeof localStorage === "undefined" ? null : localStorage) {
  return {
    theme: normalizeThemeMode(read(storage, THEME_STORAGE_KEY)),
    accent: normalizeAccentTheme(read(storage, ACCENT_STORAGE_KEY)),
  };
}

export function applyAppearance(appearance, root = typeof document === "undefined" ? null : document.documentElement) {
  if (!root) return;
  const theme = normalizeThemeMode(appearance?.theme);
  const accent = normalizeAccentTheme(appearance?.accent);
  if (theme === "system") delete root.dataset.theme;
  else root.dataset.theme = theme;
  root.dataset.accent = accent;
  root.style.colorScheme = theme === "system" ? "light dark" : theme;
  globalThis.webkit?.messageHandlers?.oneshotSidebar?.postMessage({ theme });
}

export function saveAppearance(appearance, storage = typeof localStorage === "undefined" ? null : localStorage) {
  const normalized = {
    theme: normalizeThemeMode(appearance?.theme),
    accent: normalizeAccentTheme(appearance?.accent),
  };
  try {
    storage?.setItem(THEME_STORAGE_KEY, normalized.theme);
    storage?.setItem(ACCENT_STORAGE_KEY, normalized.accent);
  } catch { /* best effort */ }
  applyAppearance(normalized);
  return normalized;
}
