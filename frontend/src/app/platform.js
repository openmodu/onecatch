export function desktopPlatform(navigatorValue = globalThis.navigator) {
  const value = navigatorValue?.userAgentData?.platform || navigatorValue?.platform || navigatorValue?.userAgent || "";
  if (/win/i.test(value)) return "windows";
  if (/mac/i.test(value)) return "macos";
  return "other";
}

export function primaryShortcutLabel(key, navigatorValue = globalThis.navigator) {
  return `${desktopPlatform(navigatorValue) === "macos" ? "⌘" : "Ctrl+"}${key}`;
}
