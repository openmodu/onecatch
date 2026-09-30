import test from "node:test";
import assert from "node:assert/strict";
import { normalizeSidebarItems, readSidebarItems, writeSidebarItems } from "./sidebarPreferences.js";

test("sidebar preferences recover defaults from missing, malformed or unavailable storage", () => {
  for (const saved of [null, "broken", "{}"] ) {
    assert.deepEqual(readSidebarItems({ getItem: () => saved }), ["templates", "skills"]);
  }
  assert.deepEqual(readSidebarItems({ getItem() { throw new Error("blocked"); } }), ["templates", "skills"]);
});

test("sidebar preferences retain an empty selection and normalize unknown or duplicate entries", () => {
  assert.deepEqual(normalizeSidebarItems([]), []);
  assert.deepEqual(normalizeSidebarItems(["usage", "unknown", "skills", "usage"]), ["skills", "usage"]);
  let saved;
  const storage = { setItem: (_, value) => { saved = value; }, getItem: () => saved };
  writeSidebarItems(storage, ["usage", "workflows"]);
  assert.deepEqual(readSidebarItems(storage), ["usage", "workflows"]);
  writeSidebarItems({ setItem() { throw new Error("blocked"); } }, []);
});
