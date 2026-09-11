const extensionOf = (path = "") => {
  const filename = path.slice(path.lastIndexOf("/") + 1).toLowerCase();
  const dot = filename.lastIndexOf(".");
  return dot >= 0 ? filename.slice(dot + 1) : filename;
};

export async function loadEditorLanguage(path) {
  const extension = extensionOf(path);
  switch (extension) {
  case "go": return import("@codemirror/lang-go").then(({ go }) => go());
  case "c":
  case "cc":
  case "cpp":
  case "h":
  case "hpp": return import("@codemirror/lang-cpp").then(({ cpp }) => cpp());
  case "css": return import("@codemirror/lang-css").then(({ css }) => css());
  case "htm":
  case "html": return import("@codemirror/lang-html").then(({ html }) => html());
  case "java": return import("@codemirror/lang-java").then(({ java }) => java());
  case "js": return import("@codemirror/lang-javascript").then(({ javascript }) => javascript());
  case "jsx": return import("@codemirror/lang-javascript").then(({ javascript }) => javascript({ jsx: true }));
  case "ts": return import("@codemirror/lang-javascript").then(({ javascript }) => javascript({ typescript: true }));
  case "tsx": return import("@codemirror/lang-javascript").then(({ javascript }) => javascript({ jsx: true, typescript: true }));
  case "json":
  case "jsonc": return import("@codemirror/lang-json").then(({ json }) => json());
  case "md":
  case "mdx": return import("@codemirror/lang-markdown").then(({ markdown }) => markdown());
  case "py": return import("@codemirror/lang-python").then(({ python }) => python());
  case "rs": return import("@codemirror/lang-rust").then(({ rust }) => rust());
  case "sql": return import("@codemirror/lang-sql").then(({ sql }) => sql());
  case "xml": return import("@codemirror/lang-xml").then(({ xml }) => xml());
  case "yaml":
  case "yml": return import("@codemirror/lang-yaml").then(({ yaml }) => yaml());
  default: return null;
  }
}
