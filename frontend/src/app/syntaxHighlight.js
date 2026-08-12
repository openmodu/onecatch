import Prism from "prismjs";
import "prismjs/components/prism-bash.js";
import "prismjs/components/prism-c.js";
import "prismjs/components/prism-cpp.js";
import "prismjs/components/prism-csharp.js";
import "prismjs/components/prism-docker.js";
import "prismjs/components/prism-go.js";
import "prismjs/components/prism-ini.js";
import "prismjs/components/prism-java.js";
import "prismjs/components/prism-json.js";
import "prismjs/components/prism-jsx.js";
import "prismjs/components/prism-kotlin.js";
import "prismjs/components/prism-markdown.js";
import "prismjs/components/prism-json5.js";
import "prismjs/components/prism-makefile.js";
import "prismjs/components/prism-python.js";
import "prismjs/components/prism-ruby.js";
import "prismjs/components/prism-rust.js";
import "prismjs/components/prism-scss.js";
import "prismjs/components/prism-sql.js";
import "prismjs/components/prism-toml.js";
import "prismjs/components/prism-typescript.js";
import "prismjs/components/prism-tsx.js";
import "prismjs/components/prism-yaml.js";

const LANGUAGE_BY_EXTENSION = {
  ".bash": "bash",
  ".c": "c",
  ".cc": "cpp",
  ".cpp": "cpp",
  ".cs": "csharp",
  ".css": "css",
  ".go": "go",
  ".h": "c",
  ".hpp": "cpp",
  ".htm": "markup",
  ".html": "markup",
  ".ini": "ini",
  ".java": "java",
  ".js": "javascript",
  ".json": "json",
  ".jsonc": "json5",
  ".jsx": "jsx",
  ".kt": "kotlin",
  ".kts": "kotlin",
  ".md": "markdown",
  ".mdx": "markdown",
  ".py": "python",
  ".rs": "rust",
  ".scss": "scss",
  ".sh": "bash",
  ".sql": "sql",
  ".toml": "toml",
  ".ts": "typescript",
  ".tsx": "tsx",
  ".xml": "markup",
  ".yaml": "yaml",
  ".yml": "yaml",
  ".zsh": "bash",
};

const LANGUAGE_BY_FILENAME = {
  dockerfile: "docker",
  gemfile: "ruby",
  makefile: "makefile",
};

function escapeHTML(source) {
  return source.replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;");
}

export function syntaxLanguageForPath(path = "") {
  const filename = path.slice(path.lastIndexOf("/") + 1).toLowerCase();
  if (LANGUAGE_BY_FILENAME[filename]) return LANGUAGE_BY_FILENAME[filename];
  const extensionIndex = filename.lastIndexOf(".");
  return extensionIndex >= 0 ? LANGUAGE_BY_EXTENSION[filename.slice(extensionIndex)] || "plain" : "plain";
}

export function highlightSource(source = "", path = "") {
  const language = syntaxLanguageForPath(path);
  const grammar = language === "plain" ? null : Prism.languages[language];
  return {
    html: grammar ? Prism.highlight(source, grammar, language) : escapeHTML(source),
    language,
  };
}
