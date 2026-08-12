import test from "node:test";
import assert from "node:assert/strict";
import { highlightSource, syntaxLanguageForPath } from "./syntaxHighlight.js";

test("syntax language follows common filenames and extensions", () => {
  assert.equal(syntaxLanguageForPath("internal/app/main.go"), "go");
  assert.equal(syntaxLanguageForPath("frontend/src/App.tsx"), "tsx");
  assert.equal(syntaxLanguageForPath("deploy/Dockerfile"), "docker");
  assert.equal(syntaxLanguageForPath("LICENSE"), "plain");
});

test("syntax highlighting emits Prism tokens for supported source", () => {
  const highlighted = highlightSource("package main\nfunc main() {}", "main.go");
  assert.equal(highlighted.language, "go");
  assert.match(highlighted.html, /token keyword/);
  assert.match(highlighted.html, /token function/);
});

test("plain text is escaped without injecting markup", () => {
  const highlighted = highlightSource("<script>&", "notes.txt");
  assert.equal(highlighted.language, "plain");
  assert.equal(highlighted.html, "&lt;script&gt;&amp;");
});
