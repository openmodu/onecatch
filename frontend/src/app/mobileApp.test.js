import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import test from "node:test";

const sourceURL = new URL("./MobileApp.jsx", import.meta.url);

// A component or hook that is used but never imported compiles cleanly and
// blanks the whole app at runtime — twice now. The suite cannot mount the
// shell (no DOM here), so it checks the one thing that failed: every name the
// file renders or calls as a hook has to come from somewhere.
test("every component and hook MobileApp uses is declared or imported", async () => {
  const source = await readFile(sourceURL, "utf8");
  const declared = new Set();
  for (const [, name] of source.matchAll(/(?:^|\n)(?:export default |export )?function ([A-Z]\w*|use[A-Z]\w*)\s*\(/g)) declared.add(name);
  for (const [, name] of source.matchAll(/(?:const|let) ([A-Z]\w*|use[A-Z]\w*)\s*=/g)) declared.add(name);
  for (const [, names] of source.matchAll(/import\s+\{([^}]+)\}\s+from/g)) {
    for (const entry of names.split(",")) {
      const name = entry.trim().split(/\s+as\s+/).pop().trim();
      if (name) declared.add(name);
    }
  }
  for (const [, name] of source.matchAll(/import\s+([A-Za-z_$][\w$]*)\s*(?:,|from)/g)) declared.add(name);

  const used = new Set();
  for (const [, name] of source.matchAll(/<([A-Z]\w*)/g)) used.add(name);
  for (const [, name] of source.matchAll(/\b(use[A-Z]\w*)\s*\(/g)) used.add(name);
  const missing = [...used].filter((name) => !declared.has(name)).sort();
  assert.deepEqual(missing, [], `used but never declared or imported: ${missing.join(", ")}`);
});
