import test from "node:test";
import assert from "node:assert/strict";
import { codeLanguageFromClassName, highlightCode, highlightSource, syntaxLanguageForPath } from "./syntaxHighlight.js";

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

test("fenced code languages accept common aliases and class lists", () => {
  assert.equal(codeLanguageFromClassName("code language-YAML extra"), "yaml");
  assert.equal(codeLanguageFromClassName("not-language-js"), "");
  assert.equal(codeLanguageFromClassName(), "");
  for (const [alias, language] of Object.entries({ js: "javascript", ts: "typescript", yml: "yaml", sh: "bash", shell: "bash", py: "python", html: "markup", "c++": "cpp", "c#": "csharp", jsonc: "json5" })) {
    assert.equal(highlightCode("", alias.toUpperCase()).language, language);
  }
});

test("YAML code highlights keys and strings and preserves source whitespace", () => {
  const source = 'common:\n  runtime_envs:\n    LOG_LEVEL: info\n    ENABLE_APMPLUS: "true"  \n\n';
  const { html, language } = highlightCode(source, "yaml");
  assert.equal(language, "yaml");
  assert.match(html, /class="token key atrule">common<\/span>/);
  assert.match(html, /class="token string">"true"<\/span>/);
  assert.equal(html.replace(/<[^>]+>/g, ""), source);
});

test("unknown and unsafe language names fall back to escaped plain text", () => {
  for (const name of ["", "text", "unknown", "__proto__", "constructor", "extend", "insertBefore"]) {
    assert.deepEqual(highlightCode('<script>alert("x")</script>&', name), { html: '&lt;script&gt;alert("x")&lt;/script&gt;&amp;', language: "plain" });
  }
});

test("highlighted HTML and incomplete streaming code cannot inject source markup", () => {
  for (const language of ["html", "js", "yaml", "bash"]) {
    const source = '<img src=x onerror="alert(1)">\n  & unfinished';
    const { html } = highlightCode(source, language);
    assert.doesNotMatch(html, /<img\b|<script\b/);
    const restored = html.replace(/<[^>]+>/g, "").replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&amp;", "&");
    assert.equal(restored, source);
  }
});
