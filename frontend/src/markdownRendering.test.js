import assert from "node:assert/strict";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import test from "node:test";
import { build } from "esbuild";

// Exercise the real Streamdown -> pre -> code renderer, without a native
// WebView. Only Wails is stubbed; React, Prism and the Markdown parser are real.
const result = await build({
  stdin: {
    contents: `
      import React from "react";
      import { renderToStaticMarkup } from "react-dom/server";
      import MarkdownContent from "./app/components/MarkdownContent.jsx";
      import i18n from "./i18n.js";
      export function render(content, streaming = false) {
        return renderToStaticMarkup(React.createElement(MarkdownContent, { content, streaming }));
      }
    `,
    resolveDir: fileURLToPath(new URL(".", import.meta.url)),
  },
  alias: { "@": fileURLToPath(new URL(".", import.meta.url)) },
  bundle: true,
  platform: "node",
  format: "cjs",
  jsx: "automatic",
  write: false,
  logLevel: "silent",
  plugins: [{
    name: "native-runtime-stub",
    setup(builder) {
      builder.onResolve({ filter: /^@wailsio\/runtime$/ }, () => ({ path: "runtime", namespace: "test" }));
      builder.onLoad({ filter: /.*/, namespace: "test" }, () => ({ contents: "export const Browser = { OpenURL: async () => {} }; export const Clipboard = { SetText: async () => {} };" }));
    },
  }],
});
const module = { exports: {} };
new Function("require", "module", "exports", result.outputFiles[0].text)(createRequire(import.meta.url), module, module.exports);
const { render } = module.exports;

function codeText(html) {
  const code = /<pre\b[^>]*><code\b[^>]*>([\s\S]*?)<\/code><\/pre>/.exec(html)?.[1];
  return code?.replace(/<[^>]+>/g, "").replaceAll("&lt;", "<").replaceAll("&gt;", ">").replaceAll("&quot;", '"').replaceAll("&amp;", "&");
}

test("rendered YAML contains a language header, two actions, and highlighted code only", () => {
  const source = 'common:\n  LOG_LEVEL: info\n  ENABLE_APMPLUS: "true"  \n\n';
  const html = render(`\x60\x60\x60yaml\n${source}\x60\x60\x60`);
  assert.match(html, /title="YAML">YAML<\/span>/);
  assert.equal((html.match(/<button\b/g) || []).length, 2);
  assert.match(html, /aria-pressed="false"/);
  assert.match(html, /class="token key atrule">common<\/span>/);
  assert.match(html, /class="token string">"true"<\/span>/);
  assert.match(html, /data-language="yaml"/);
  assert.equal(codeText(html), source);
});

test("inline code has no toolbar or syntax markup", () => {
  const html = render("Inline `const x = 1`.");
  assert.doesNotMatch(html, /markdown-code-toolbar|class="token /);
  assert.match(html, /<code[^>]*>const x = 1<\/code>/);
});

test("streaming fences highlight safely before a closing fence arrives", () => {
  const html = render('```js\nconst value = "<img src=x>";', true);
  assert.match(html, /class="token keyword">const<\/span>/);
  assert.doesNotMatch(html, /<img\b/);
  assert.match(codeText(html), /const value = "<img src=x>";/);
});

test("unknown-language and indented blocks remain copyable plain text", () => {
  const unknown = render("```unknown\n<script>&\n```");
  assert.equal(codeText(unknown), "<script>&\n");
  assert.doesNotMatch(unknown, /<script\b|class="token /);
  const indented = render("    first\n    second\n");
  assert.equal(codeText(indented), "first\nsecond\n");
  assert.match(indented, /markdown-code-toolbar/);
});
