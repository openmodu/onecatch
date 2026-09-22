import test from "node:test";
import assert from "node:assert/strict";
import { appendPrompt, builtinPromptActions, capturePromptSelection, readPromptLibrary, renderPromptTemplate, templateArguments, templateContextNames, validatePromptLibrary, writePromptLibrary } from "./promptActions.js";
import { translationResources } from "../i18n.js";

const a = { id: "custom-a", name: "A", body: "{{project}} {{arg:focus}} {{template:custom-b}}" };
const b = { id: "custom-b", name: "B", body: "{{arg:focus}} {{arg:language}} {{selection}}" };
const code = (expected) => (error) => error.code === expected;

test("nested templates collect unique parameters in order and expand context", () => {
  assert.deepEqual(templateArguments(a, [a, b]), ["focus", "language"]);
  assert.equal(renderPromptTemplate(a, [a, b], { project: "demo", selection: "code" }, { focus: "edge cases", language: "Go" }), "demo edge cases edge cases Go code");
});

test("context and arguments are literal and cannot expand further template references", () => {
  assert.equal(renderPromptTemplate(b, [a, b], { selection: "{{template:missing}}" }, { focus: "{{arg:secret}}", language: "Go" }), "{{arg:secret}} Go {{template:missing}}");
});

test("cycles, missing references, unknown variables and absent parameters fail explicitly", () => {
  assert.throws(() => templateArguments({ ...b, body: "{{template:custom-a}}" }, [a, { ...b, body: "{{template:custom-a}}" }]), code("cycle"));
  assert.throws(() => templateArguments(a, [a]), code("missingTemplate"));
  assert.throws(() => templateArguments({ ...a, body: "{{typo}}" }, []), code("unknownToken"));
  assert.throws(() => renderPromptTemplate(b, [b], {}, {}), code("requiredArgument"));
  assert.throws(() => renderPromptTemplate({ ...b, body: "{{selection}}" }, [], {}, {}), code("requiredSelection"));
  assert.throws(() => renderPromptTemplate({ ...b, body: "{{arg:toString}}" }, [], {}, {}), code("requiredArgument"));
});

test("expansion caps nesting, repeated references and output size", () => {
  const library = Array.from({ length: 9 }, (_, i) => ({ id: `custom-${i}`, name: `${i}`, body: i === 8 ? "end" : `{{template:custom-${i + 1}}}` }));
  assert.throws(() => templateArguments(library[0], library), code("depth"));
  assert.throws(() => renderPromptTemplate({ ...b, body: "{{selection}}" }, [], { selection: "x".repeat(128001) }, {}), code("size"));
  assert.throws(() => templateArguments({ ...b, body: "{{date}}".repeat(1001) }, []), code("size"));
});

test("library persistence validates data and does not store context or arbitrary properties", () => {
  let raw;
  const storage = { getItem: () => raw, setItem: (_, value) => { raw = value; } };
  assert.deepEqual(readPromptLibrary(storage), []);
  writePromptLibrary(storage, [{ ...a, selection: "private log", builtin: true }]);
  assert.deepEqual(readPromptLibrary(storage), [a]);
  assert.ok(!raw.includes("private log"));
  assert.throws(() => validatePromptLibrary([a, a]), code("invalidLibrary"));
  assert.throws(() => validatePromptLibrary([{ ...a, id: "builtin-review" }]), code("invalidLibrary"));
  raw = '{"version":2,"templates":[]}';
  assert.throws(() => readPromptLibrary(storage), code("invalidLibrary"));
  raw = 'not json';
  assert.throws(() => readPromptLibrary(storage));
  assert.throws(() => writePromptLibrary({ setItem() { throw Error("quota"); } }, [a]), /quota/);
});

test("all built-in actions render in both languages with material and project context", () => {
  for (const translations of Object.values(translationResources)) {
    const library = builtinPromptActions((key) => translations[key]);
    for (const item of library) {
      const args = Object.fromEntries(templateArguments(item, library).map((name) => [name, "edge cases"]));
      const output = renderPromptTemplate(item, library, { selection: "source", project: "demo", path: "/demo", task: "fix" }, args);
      assert.match(output, /source/);
      assert.match(output, /\/demo/);
      assert.doesNotMatch(output, /\{\{/);
    }
  }
});

test("selection capture prefers textarea selection and keeps existing drafts intact", () => {
  assert.equal(capturePromptSelection({ activeElement: { tagName: "TEXTAREA", value: "abcde", selectionStart: 1, selectionEnd: 4 }, getSelection: () => "other" }), "bcd");
  assert.equal(capturePromptSelection({ activeElement: { tagName: "INPUT", type: "password", value: "secret", selectionStart: 0, selectionEnd: 6 }, getSelection: () => "" }), "");
  assert.equal(capturePromptSelection({ activeElement: null, getSelection: () => "diff" }), "diff");
  assert.equal(appendPrompt("existing draft", "action"), "existing draft\n\naction");
});


test("quick picker only requests context referenced by the template or its nested templates", () => {
  assert.deepEqual(templateContextNames(a, [a, b]), ["project", "selection"]);
  assert.deepEqual(templateContextNames({ id: "plain", body: "Review project {{project}}" }, []), ["project"]);
  assert.deepEqual(templateContextNames({ id: "plain", body: "Run the tests" }, []), []);
});
