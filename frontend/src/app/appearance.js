export const THEME_STORAGE_KEY = "onecatch.appearance.theme";
export const ACCENT_STORAGE_KEY = "onecatch.appearance.accent";
export const CHAT_FONT_SIZE_STORAGE_KEY = "onecatch.appearance.chat-font-size";
export const APPEARANCE_CHANGED_EVENT = "onecatch:appearance-changed";

export const themeModes = ["system", "light", "dark"];
export const accentThemes = ["forest", "ocean", "violet", "amber"];
export const chatFontSizes = ["small", "standard", "large", "extra-large"];

export function normalizeThemeMode(value) {
  return themeModes.includes(value) ? value : "system";
}

export function normalizeAccentTheme(value) {
  return accentThemes.includes(value) ? value : "forest";
}

export function normalizeChatFontSize(value) {
  return chatFontSizes.includes(value) ? value : "standard";
}

function read(storage, key) {
  try { return storage?.getItem(key); } catch { return null; }
}

export function readAppearance(storage = typeof localStorage === "undefined" ? null : localStorage) {
  return {
    theme: normalizeThemeMode(read(storage, THEME_STORAGE_KEY)),
    accent: normalizeAccentTheme(read(storage, ACCENT_STORAGE_KEY)),
    chatFontSize: normalizeChatFontSize(read(storage, CHAT_FONT_SIZE_STORAGE_KEY)),
  };
}

export function applyAppearance(appearance, root = typeof document === "undefined" ? null : document.documentElement) {
  if (!root) return;
  const theme = normalizeThemeMode(appearance?.theme);
  const accent = normalizeAccentTheme(appearance?.accent);
  const chatFontSize = normalizeChatFontSize(appearance?.chatFontSize);
  if (theme === "system") delete root.dataset.theme;
  else root.dataset.theme = theme;
  root.dataset.accent = accent;
  root.dataset.chatFontSize = chatFontSize;
  root.style.colorScheme = theme === "system" ? "light dark" : theme;
  globalThis.webkit?.messageHandlers?.onecatchSidebar?.postMessage({ theme });
}

export function saveAppearance(appearance, storage = typeof localStorage === "undefined" ? null : localStorage) {
  const normalized = {
    theme: normalizeThemeMode(appearance?.theme),
    accent: normalizeAccentTheme(appearance?.accent),
    chatFontSize: normalizeChatFontSize(appearance?.chatFontSize),
  };
  try {
    storage?.setItem(THEME_STORAGE_KEY, normalized.theme);
    storage?.setItem(ACCENT_STORAGE_KEY, normalized.accent);
    storage?.setItem(CHAT_FONT_SIZE_STORAGE_KEY, normalized.chatFontSize);
  } catch { /* best effort */ }
  applyAppearance(normalized);
  return normalized;
}
