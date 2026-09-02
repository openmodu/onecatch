import assert from "node:assert/strict";
import test from "node:test";

import { androidVersionCode, parseChangelog } from "./release-info.mjs";

test("reads the first version section and its notes", () => {
  const release = parseChangelog(`# Changelog

## 1.2.3

- First change
- Second change

## 1.2.2

- Older change
`);

  assert.deepEqual(release, {
    version: "1.2.3",
    notes: "- First change\n- Second change",
    androidVersionCode: 1_002_003,
  });
});

test("requires release notes", () => {
  assert.throws(
    () => parseChangelog("# Changelog\n\n## 1.2.3\n"),
    /must include release notes/,
  );
});

test("rejects a malformed latest section instead of falling back", () => {
  assert.throws(
    () => parseChangelog("# Changelog\n\n## 1.2\n\n- New\n\n## 1.1.0\n\n- Old\n"),
    /first CHANGELOG.md section/,
  );
});

test("rejects Android versionCode collisions", () => {
  assert.throws(() => androidVersionCode("1.1000.0"), /below 1000/);
  assert.throws(() => androidVersionCode("1.0.1000"), /below 1000/);
});
