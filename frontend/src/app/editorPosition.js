export function lspPositionAt(document, offset) {
  const bounded = Math.max(0, Math.min(offset, document.length));
  const line = document.lineAt(bounded);
  return { line: line.number - 1, character: bounded - line.from };
}

export function editorOffsetAt(document, position = {}) {
  const lineNumber = Math.max(1, Math.min((position.line || 0) + 1, document.lines));
  const line = document.line(lineNumber);
  return line.from + Math.max(0, Math.min(position.character || 0, line.length));
}
