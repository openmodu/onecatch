import test from "node:test";
import assert from "node:assert/strict";
import { commandPaletteShortcutIndex, commandPaletteWorkspaceResults, moveCommandPaletteIndex } from "./commandPaletteNavigation.js";

test("command palette keyboard navigation wraps and supports boundaries", () => {
  assert.equal(moveCommandPaletteIndex(-1, 4, "ArrowDown"), 0);
  assert.equal(moveCommandPaletteIndex(3, 4, "ArrowDown"), 0);
  assert.equal(moveCommandPaletteIndex(0, 4, "ArrowUp"), 3);
  assert.equal(moveCommandPaletteIndex(2, 4, "Home"), 0);
  assert.equal(moveCommandPaletteIndex(2, 4, "End"), 3);
  assert.equal(moveCommandPaletteIndex(2, 0, "ArrowDown"), -1);
});

test("command palette shortcuts resolve numbered results and commands", () => {
  const items = [
    { shortcutKey: "", shortcutLabel: "⌘1" },
    { shortcutKey: "", shortcutLabel: "⌘2" },
    { shortcutKey: "n", shortcutLabel: "⌘N" },
    { shortcutKey: "o", shortcutLabel: "⌘O" },
    { shortcutKey: ",", shortcutLabel: "⌘," },
  ];
  assert.equal(commandPaletteShortcutIndex(items, "1"), 0);
  assert.equal(commandPaletteShortcutIndex(items, "2"), 1);
  assert.equal(commandPaletteShortcutIndex(items, "N"), 2);
  assert.equal(commandPaletteShortcutIndex(items, "o"), 3);
  assert.equal(commandPaletteShortcutIndex(items, ","), 4);
  assert.equal(commandPaletteShortcutIndex(items, "5"), -1);
});

test("command palette project results filter names and paths with stable shortcuts", () => {
  const workspaces = [
    { id: "one", name: "onecatch", path: "/code/openmodu/onecatch" },
    { id: "two", name: "learning", path: "/documents/children" },
    { id: "three", name: "backend", path: "/srv/backend", remoteFs: { host: "devbox", root: "/srv/backend" } },
  ];
  assert.deepEqual(commandPaletteWorkspaceResults(workspaces, "ONECATCH", 2), [
    { workspace: workspaces[0], shortcutLabel: "⌘3" },
  ]);
  assert.deepEqual(commandPaletteWorkspaceResults(workspaces, "documents", 0), [
    { workspace: workspaces[1], shortcutLabel: "⌘1" },
  ]);
  assert.deepEqual(commandPaletteWorkspaceResults(workspaces, "DEVBOX", 1), [
    { workspace: workspaces[2], shortcutLabel: "⌘2" },
  ]);
  assert.deepEqual(commandPaletteWorkspaceResults(workspaces, "", 0), []);
});
