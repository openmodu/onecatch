// This catalog and template engine are independent implementations for OneCatch.
export const PROMPT_LIBRARY_KEY = "onecatch.promptTemplates.v1";
const MAX_BODY = 32000;
const MAX_OUTPUT = 128000;
const tokenPattern = /\{\{\s*([^{}]+?)\s*\}\}/g;
const contextNames = new Set(["selection", "project", "path", "task", "date"]);

export class PromptTemplateError extends Error {
  constructor(code, detail = "") {
    super(code);
    this.code = code;
    this.detail = detail;
  }
}

export function builtinPromptActions(t) {
  return ["explain", "tests", "review"].map((id) => ({
    id: `builtin-${id}`, name: t(`promptActions.${id}`), body: t(`promptActions.${id}Body`, { skipInterpolation: true }), builtin: true,
  }));
}

function expandTemplate(template, library, resolve, stack = [], budget = { tokens: 0 }) {
  if (stack.includes(template.id)) throw new PromptTemplateError("cycle", template.name);
  if (stack.length >= 8) throw new PromptTemplateError("depth");
  if (typeof template.body !== "string" || template.body.length > MAX_BODY) throw new PromptTemplateError("size");
  const output = template.body.replace(tokenPattern, (_, raw) => {
    if (++budget.tokens > 1000) throw new PromptTemplateError("size");
    const token = raw.trim();
    if (token.startsWith("template:")) {
      const id = token.slice(9).trim();
      const nested = library.find((item) => item.id === id);
      if (!nested) throw new PromptTemplateError("missingTemplate", id);
      return expandTemplate(nested, library, resolve, [...stack, template.id], budget);
    }
    if (token.startsWith("arg:") && token.slice(4).trim()) return resolve("argument", token.slice(4).trim());
    if (contextNames.has(token)) return resolve("context", token);
    throw new PromptTemplateError("unknownToken", token);
  });
  if (output.length > MAX_OUTPUT) throw new PromptTemplateError("size");
  return output;
}

export function templateArguments(template, library) {
  const names = new Set();
  expandTemplate(template, library, (kind, name) => {
    if (kind === "argument") names.add(name);
    return "";
  });
  return [...names];
}

export function renderPromptTemplate(template, library, context, args) {
  return expandTemplate(template, library, (kind, name) => {
    const values = kind === "argument" ? args : context;
    const value = String(values && Object.hasOwn(values, name) ? values[name] : "");
    if (kind === "argument" && !value.trim()) throw new PromptTemplateError("requiredArgument", name);
    if (name === "selection" && kind === "context" && !value.trim()) throw new PromptTemplateError("requiredSelection");
    return value;
  });
}

export function validatePromptLibrary(value) {
  if (!Array.isArray(value) || value.length > 100) throw new PromptTemplateError("invalidLibrary");
  const ids = new Set();
  return value.map((item) => {
    if (!item || typeof item.id !== "string" || !/^custom-[a-zA-Z0-9-]+$/.test(item.id) || ids.has(item.id)
      || typeof item.name !== "string" || !item.name.trim() || item.name.length > 120
      || typeof item.body !== "string" || !item.body.trim() || item.body.length > MAX_BODY) {
      throw new PromptTemplateError("invalidLibrary");
    }
    ids.add(item.id);
    return { id: item.id, name: item.name.trim(), body: item.body };
  });
}

export function readPromptLibrary(storage) {
  const raw = storage.getItem(PROMPT_LIBRARY_KEY);
  if (!raw) return [];
  const data = JSON.parse(raw);
  if (data.version !== 1) throw new PromptTemplateError("invalidLibrary");
  return validatePromptLibrary(data.templates);
}

export function writePromptLibrary(storage, templates) {
  storage.setItem(PROMPT_LIBRARY_KEY, JSON.stringify({ version: 1, templates: validatePromptLibrary(templates) }));
}

export function capturePromptSelection(doc) {
  const element = doc.activeElement;
  if (element && ["TEXTAREA", "INPUT"].includes(element.tagName) && element.type !== "password") {
    if (typeof element.selectionStart === "number" && element.selectionEnd > element.selectionStart) {
      return element.value.slice(element.selectionStart, element.selectionEnd);
    }
  }
  return doc.getSelection()?.toString() || "";
}

export function appendPrompt(draft, prompt) {
  return draft ? `${draft}\n\n${prompt}` : prompt;
}

export function templateContextNames(template, library) {
  const names = new Set();
  expandTemplate(template, library, (kind, name) => {
    if (kind === "context") names.add(name);
    return "";
  });
  return [...names];
}
