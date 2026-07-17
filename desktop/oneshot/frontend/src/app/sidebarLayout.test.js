import assert from "node:assert/strict";
import test from "node:test";
import {
  SIDEBAR_DEFAULT_WIDTH,
  SIDEBAR_MAX_WIDTH,
  SIDEBAR_MIN_WIDTH,
  SIDEBAR_WIDTH_STORAGE_KEY,
  clampSidebarWidth,
  parseSidebarWidth,
  readSidebarWidth,
  sidebarWidthBounds,
  writeSidebarWidth,
} from "./sidebarLayout.js";

test("sidebar width stays usable for both sidebar and main content", () => {
  assert.deepEqual(sidebarWidthBounds(1440), { min: SIDEBAR_MIN_WIDTH, max: SIDEBAR_MAX_WIDTH });
  assert.deepEqual(sidebarWidthBounds(900), { min: SIDEBAR_MIN_WIDTH, max: 340 });
  assert.equal(clampSidebarWidth(120, 1440), SIDEBAR_MIN_WIDTH);
  assert.equal(clampSidebarWidth(900, 1440), SIDEBAR_MAX_WIDTH);
  assert.equal(clampSidebarWidth(400, 900), 340);
  assert.equal(clampSidebarWidth("bad", 1440), SIDEBAR_DEFAULT_WIDTH);
});

test("sidebar width parser rejects malformed preferences", () => {
  assert.equal(parseSidebarWidth('{"width":320}', 1440), 320);
  assert.equal(parseSidebarWidth('{"width":"320"}', 1440), null);
  assert.equal(parseSidebarWidth("not-json", 1440), null);
  assert.equal(parseSidebarWidth("", 1440), null);
});

test("sidebar width preference survives storage failures", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key) || null,
    setItem: (key, value) => values.set(key, value),
  };
  assert.equal(readSidebarWidth(storage, 1440), null);
  assert.equal(writeSidebarWidth(storage, 336), true);
  assert.equal(values.get(SIDEBAR_WIDTH_STORAGE_KEY), '{"width":336}');
  assert.equal(readSidebarWidth(storage, 1440), 336);

  const denied = {
    getItem: () => { throw new Error("denied"); },
    setItem: () => { throw new Error("denied"); },
  };
  assert.equal(readSidebarWidth(denied, 1440), null);
  assert.equal(writeSidebarWidth(denied, 300), false);
});
