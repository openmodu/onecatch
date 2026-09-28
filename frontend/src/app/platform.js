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

// Only iOS asks the user for Local Network access; Android has no such switch,
// so pointing Android users at it sends them looking for a setting that isn't there.
export function mobilePlatform(navigatorValue = globalThis.navigator) {
  const value = navigatorValue?.userAgent || "";
  if (/android/i.test(value)) return "android";
  if (/iphone|ipad|ipod/i.test(value) || (/macintosh/i.test(value) && navigatorValue?.maxTouchPoints > 1)) return "ios";
  return "other";
}

const discoveryHints = {
  ios: {
    failed: "搜索暂不可用。请在系统「设置 › OneCatch」中允许「本地网络」，或手动输入电脑地址。",
    empty: "未发现电脑。请确认电脑已开启「手机连接」、两端在同一局域网，并已允许本地网络访问。",
  },
  android: {
    failed: "搜索暂不可用。请确认手机已连接 Wi-Fi，或手动输入电脑地址。",
    empty: "未发现电脑。请确认电脑已开启「手机连接」，且手机和电脑连接的是同一个 Wi-Fi 或局域网。",
  },
  other: {
    failed: "搜索暂不可用。请手动输入电脑地址。",
    empty: "未发现电脑。请确认电脑已开启「手机连接」，且两端在同一局域网。",
  },
};

export function discoveryHint(failed, platform = mobilePlatform()) {
  const hints = discoveryHints[platform] || discoveryHints.other;
  return failed ? hints.failed : hints.empty;
}
