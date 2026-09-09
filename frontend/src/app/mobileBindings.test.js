import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const bindingURL = new URL("../../bindings/github.com/openmodu/onecatch/internal/transport/wails/mobilebinding.js", import.meta.url);
const FQN = "github.com/openmodu/onecatch/internal/transport/wails.MobileBinding.";

// Wails derives a bound method's id from an FNV-1a hash of its fully qualified
// name. A wrong id fails only at runtime, as a call that answers nothing, so
// the ids in the generated file are checked here against the same arithmetic.
function fnv1a(value) {
  let hash = 0x811c9dc5;
  for (const byte of new TextEncoder().encode(value)) {
    hash ^= byte;
    hash = Math.imul(hash, 0x01000193) >>> 0;
  }
  return hash;
}

test("every mobile binding calls the id the Go side answers to", async () => {
  const source = await readFile(bindingURL, "utf8");
  const calls = [...source.matchAll(/export function (\w+)\([^)]*\)\s*\{\s*return \$Call\.ByID\((\d+)/g)];
  assert.ok(calls.length >= 15, `found ${calls.length} bound methods`);
  for (const [, name, id] of calls) {
    assert.equal(Number(id), fnv1a(FQN + name), `${name} is bound to the wrong id`);
  }
  assert.ok(calls.some(([, name]) => name === "RefreshRuns"), "pull to refresh needs its own binding");
});
