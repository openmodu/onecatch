import assert from "node:assert/strict";
import test from "node:test";
import { desktopPlatform, discoveryHint, mobilePlatform, primaryShortcutLabel, usesCompactAuxiliaryChrome } from "./app/platform.js";

test("desktop platform and primary shortcut labels follow Windows conventions", () => {
  assert.equal(desktopPlatform({ userAgentData: { platform: "Windows" } }), "windows");
  assert.equal(desktopPlatform({ userAgentData: { platform: "Linux x86_64" } }), "linux");
  assert.equal(primaryShortcutLabel(",", { platform: "Win32" }), "Ctrl+,");
  assert.equal(primaryShortcutLabel(",", { platform: "MacIntel" }), "⌘,");
});

test("compact auxiliary chrome is limited to Windows and Linux", () => {
  assert.equal(usesCompactAuxiliaryChrome({ platform: "Win32" }), true);
  assert.equal(usesCompactAuxiliaryChrome({ platform: "Linux x86_64" }), true);
  assert.equal(usesCompactAuxiliaryChrome({ platform: "MacIntel" }), false);
});

test("discovery hints only mention Local Network permission on iOS", () => {
  const android = { userAgent: "Mozilla/5.0 (Linux; Android 15; sdk_gphone64_arm64) AppleWebKit/537.36 Chrome/131 Mobile Safari/537.36" };
  const iphone = { userAgent: "Mozilla/5.0 (iPhone; CPU iPhone OS 18_0 like Mac OS X) AppleWebKit/605.1.15" };
  const ipad = { userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15", maxTouchPoints: 5 };
  assert.equal(mobilePlatform(android), "android");
  assert.equal(mobilePlatform(iphone), "ios");
  assert.equal(mobilePlatform(ipad), "ios");
  assert.equal(mobilePlatform({ userAgent: "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7)" }), "other");
  for (const failed of [true, false]) {
    assert.doesNotMatch(discoveryHint(failed, "android"), /本地网络/);
    assert.match(discoveryHint(failed, "ios"), /本地网络/);
  }
  assert.match(discoveryHint(true, "android"), /Wi-Fi/);
});
