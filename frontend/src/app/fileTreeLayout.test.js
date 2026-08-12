import test from "node:test";
import assert from "node:assert/strict";
import {
  FILE_TREE_DEFAULT_RATIO,
  FILE_TREE_MIN_WIDTH,
  FILE_TREE_RATIO_STORAGE_KEY,
  clampFileTreeWidth,
  readFileTreeRatio,
  writeFileTreeRatio,
} from "./fileTreeLayout.js";

test("file tree width stays usable for both panes", () => {
  assert.equal(clampFileTreeWidth(40, 800), FILE_TREE_MIN_WIDTH);
  assert.equal(clampFileTreeWidth(300, 800), 300);
  assert.equal(clampFileTreeWidth(900, 800), 573);
});

test("file tree ratio is persisted and invalid values use the default", () => {
  const values = new Map();
  const storage = {
    getItem: (key) => values.get(key),
    setItem: (key, value) => values.set(key, value),
  };

  writeFileTreeRatio(0.42, storage);
  assert.equal(values.get(FILE_TREE_RATIO_STORAGE_KEY), "0.42");
  assert.equal(readFileTreeRatio(storage), 0.42);

  values.set(FILE_TREE_RATIO_STORAGE_KEY, "2");
  assert.equal(readFileTreeRatio(storage), FILE_TREE_DEFAULT_RATIO);
});

test("file tree ratio gracefully handles unavailable storage", () => {
  const storage = {
    getItem: () => { throw new Error("unavailable"); },
    setItem: () => { throw new Error("unavailable"); },
  };

  assert.equal(readFileTreeRatio(storage), FILE_TREE_DEFAULT_RATIO);
  assert.doesNotThrow(() => writeFileTreeRatio(0.5, storage));
});
