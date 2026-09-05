import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import test from "node:test";

import { parseRGB } from "./mobileChrome.js";

const workbench = readFileSync(new URL("./MobileApp.jsx", import.meta.url), "utf8");
const mobileShell = readFileSync(new URL("../../../internal/app/mobile/mobile.go", import.meta.url), "utf8");

test("parseRGB reads the colours a browser actually reports", () => {
  assert.deepEqual(parseRGB("rgb(245, 245, 240)"), { red: 245, green: 245, blue: 240 });
  assert.deepEqual(parseRGB("rgba(28, 28, 28, 1)"), { red: 28, green: 28, blue: 28 });
  assert.deepEqual(parseRGB("rgb(28 28 28 / 1)"), { red: 28, green: 28, blue: 28 });
  assert.equal(parseRGB("transparent"), null);
  assert.equal(parseRGB(""), null);
});

test("the safe-area strips follow the page theme rather than the launch colour", () => {
  // Go fixes one colour at launch, which is right for the splash and wrong the
  // moment the phone is in dark mode.
  assert.match(mobileShell, /BackgroundColour:\s+application\.NewRGB\(/);
  assert.match(workbench, /useNativeChrome\(\);/);
});
