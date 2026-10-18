import assert from "node:assert/strict";
import test from "node:test";
import { mkdtempSync, rmSync, writeFileSync } from "node:fs";
import os from "node:os";
import path from "node:path";
import { execFileSync } from "node:child_process";
import { fileURLToPath } from "node:url";

import { androidVersionCode, parseChangelog, validateVersionTemplates } from "./release-info.mjs";

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

test("validates the Keel template without removed Wails files", (t) => {
  const repoRoot = mkdtempSync(path.join(os.tmpdir(), "onecatch-release-"));
  t.after(() => rmSync(repoRoot, { recursive: true, force: true }));
  const configPath = path.join(repoRoot, "keel.json");
  writeFileSync(configPath, JSON.stringify({ version: "0.0.0" }));
  assert.doesNotThrow(() => validateVersionTemplates(repoRoot));
  for (const version of ["0.5.0", "", null, 0]) {
    writeFileSync(configPath, JSON.stringify({ version }));
    assert.throws(() => validateVersionTemplates(repoRoot), /keel.json version template/);
  }
  writeFileSync(configPath, "{}");
  assert.throws(() => validateVersionTemplates(repoRoot), /keel.json version template/);
});

test("release check succeeds against the current repository layout", () => {
  const output = execFileSync(process.execPath, [
    fileURLToPath(new URL("./release-info.mjs", import.meta.url)), "--check",
  ], { encoding: "utf8" });
  assert.match(output, /Release \d+\.\d+\.\d+ is valid \(Keel desktop\)/);
});
