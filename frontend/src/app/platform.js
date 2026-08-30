export function desktopPlatform(navigatorValue = globalThis.navigator) {
  const value = navigatorValue?.userAgentData?.platform || navigatorValue?.platform || navigatorValue?.userAgent || "";
  if (/win/i.test(value)) return "windows";
  if (/mac/i.test(value)) return "macos";
  if (/linux/i.test(value)) return "linux";
  return "other";
}

export function usesCompactAuxiliaryChrome(navigatorValue = globalThis.navigator) {
  return ["windows", "linux"].includes(desktopPlatform(navigatorValue));
}

export function primaryShortcutLabel(key, navigatorValue = globalThis.navigator) {
  return `${desktopPlatform(navigatorValue) === "macos" ? "⌘" : "Ctrl+"}${key}`;
}
