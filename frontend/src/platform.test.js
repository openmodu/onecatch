import assert from "node:assert/strict";
import test from "node:test";
import { desktopPlatform, primaryShortcutLabel } from "./app/platform.js";

test("desktop platform and primary shortcut labels follow Windows conventions", () => {
  assert.equal(desktopPlatform({ userAgentData: { platform: "Windows" } }), "windows");
  assert.equal(desktopPlatform({ userAgentData: { platform: "Linux x86_64" } }), "linux");
  assert.equal(primaryShortcutLabel(",", { platform: "Win32" }), "Ctrl+,");
  assert.equal(primaryShortcutLabel(",", { platform: "MacIntel" }), "⌘,");
});
