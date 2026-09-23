import test from "node:test";
import assert from "node:assert/strict";
import { paletteCommandResults, readCommandHistory, rememberCommand } from "./paletteCommands.js";
const t = (key) => ({ "sidebar.templates": "模板", "sidebar.settings": "设置", "sidebar.usage": "用量" })[key] || key;
const ids = (query, history) => paletteCommandResults(t, query, history).map((command) => command.id);

test("commands match Chinese, English, abbreviations and multiple terms", () => {
  for (const query of ["模板", "TEMPLATES", "mb", "tmplt", "prompt snippet"]) assert.equal(ids(query)[0], "templates");
  assert.equal(ids("quota")[0], "usage");
  assert.equal(ids("设置")[0], "settings");
  assert.deepEqual(ids("unknown nonexistent command"), []);
  assert.deepEqual(ids("template quota"), []);
});

test("recent commands rank first without hiding commands or overriding relevance", () => {
  const commands = ids("", ["usage", "templates"]);
  assert.deepEqual(commands.slice(0, 2), ["usage", "templates"]);
  assert.equal(commands.length, 6);
  assert.equal(ids("templates", ["usage"])[0], "templates");
});

test("command history deduplicates and rejects invalid persisted entries", () => {
  let value = '["usage","unknown","usage",null,"templates"]';
  const storage = { getItem: () => value, setItem: (_, next) => { value = next; } };
  assert.deepEqual(readCommandHistory(storage), ["usage", "templates"]);
  rememberCommand(storage, "templates");
  assert.deepEqual(readCommandHistory(storage), ["templates", "usage"]);
  rememberCommand(storage, "not-a-command");
  assert.deepEqual(readCommandHistory(storage), ["templates", "usage"]);
  value = '{"version": 2}';
  assert.deepEqual(readCommandHistory(storage), []);
  value = 'broken';
  assert.deepEqual(readCommandHistory(storage), []);
  const unavailable = { getItem() { throw Error("blocked"); }, setItem() { throw Error("quota"); } };
  assert.deepEqual(readCommandHistory(unavailable), []);
  assert.doesNotThrow(() => rememberCommand(unavailable, "usage"));
});
