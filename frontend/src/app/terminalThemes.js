export const TERMINAL_THEME_IDS = ["system", "paper", "midnight", "contrast"];

const fixedThemes = {
  paper: { background: "#f4f0e6", foreground: "#28251f", cursor: "#785a2b", cursorAccent: "#fffaf0", selectionBackground: "#d9ceb9", black: "#28251f", red: "#a33f35", green: "#507245", yellow: "#9a6a22", blue: "#396c83", magenta: "#765384", cyan: "#39756e", white: "#e9e2d4", brightBlack: "#756f65", brightWhite: "#fffaf0" },
  midnight: { background: "#111821", foreground: "#d8dee8", cursor: "#89b4a6", cursorAccent: "#111821", selectionBackground: "#344453", black: "#111821", red: "#d47770", green: "#8fb38a", yellow: "#d0aa6d", blue: "#78a6c8", magenta: "#aa8abd", cyan: "#74b8b0", white: "#d8dee8", brightBlack: "#64717f", brightWhite: "#f2f5f8" },
  contrast: { background: "#000000", foreground: "#ffffff", cursor: "#ffd75f", cursorAccent: "#000000", selectionBackground: "#365f91", black: "#000000", red: "#ff6b6b", green: "#7dff8a", yellow: "#ffe66d", blue: "#7db7ff", magenta: "#d99cff", cyan: "#71f6e5", white: "#eeeeee", brightBlack: "#8a8a8a", brightWhite: "#ffffff" },
};

export function normalizeTerminalPreferences(value = {}) {
  return {
    shell: String(value.shell || "").trim(),
    arguments: Array.isArray(value.arguments) ? value.arguments.map((item) => String(item).trim()).filter(Boolean) : [],
    theme: TERMINAL_THEME_IDS.includes(value.theme) ? value.theme : "system",
  };
}

export function resolveTerminalTheme(id = "system") {
  if (fixedThemes[id]) return fixedThemes[id];
  const style = getComputedStyle(document.documentElement);
  const token = (name, fallback) => style.getPropertyValue(name).trim() || fallback;
  return {
    background: token("--background", "#f5f5f0"), foreground: token("--foreground", "#1a1a1a"),
    cursor: token("--primary", "#694d1f"), cursorAccent: token("--primary-foreground", "#f7f4ec"),
    selectionBackground: token("--accent", "#e4e1d5"), black: token("--foreground", "#1a1a1a"),
    brightBlack: token("--muted-foreground", "#60605e"), white: token("--background", "#f5f5f0"), brightWhite: token("--card", "#fcfcfa"),
  };
}
