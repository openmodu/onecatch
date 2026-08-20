const stripPrefix = (value = "") => {
  const unquoted = value.startsWith('"') && value.endsWith('"') ? value.slice(1, -1) : value;
  return unquoted === "/dev/null" ? "" : unquoted.replace(/^[ab]\//, "");
};

function lineKind(line) {
  if (line.startsWith("+")) return "add";
  if (line.startsWith("-")) return "delete";
  if (line.startsWith("\\")) return "meta";
  return "context";
}

export function parseUnifiedDiff(value = "", scope = "worktree") {
  const files = [];
  let file = null;
  let hunk = null;
  let oldLine = 0;
  let newLine = 0;

  for (const rawLine of String(value).split("\n")) {
    const header = rawLine.match(/^diff --git "?a\/(.+?)"? "?b\/(.+?)"?$/);
    if (header) {
      file = { path: stripPrefix(header[2]), oldPath: stripPrefix(header[1]), scope, hunks: [], additions: 0, deletions: 0 };
      files.push(file);
      hunk = null;
      continue;
    }
    if (!file) continue;
    // Only the preamble carries "+++"/"---" headers. Inside a hunk those are
    // ordinary added or deleted lines whose own text starts with "++"/"--".
    if (!hunk && rawLine.startsWith("+++ ")) {
      file.path = stripPrefix(rawLine.slice(4)) || file.path;
      continue;
    }
    if (!hunk && rawLine.startsWith("--- ")) {
      file.oldPath = stripPrefix(rawLine.slice(4)) || file.oldPath;
      continue;
    }
    const hunkHeader = rawLine.match(/^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@(.*)$/);
    if (hunkHeader) {
      oldLine = Number(hunkHeader[1]);
      newLine = Number(hunkHeader[3]);
      hunk = { header: rawLine, label: hunkHeader[5].trim(), scope, lines: [] };
      file.hunks.push(hunk);
      continue;
    }
    if (!hunk || rawLine === "" || /^(?:index |new file mode |deleted file mode |similarity index |rename from |rename to )/.test(rawLine)) continue;
    const kind = lineKind(rawLine);
    const line = { kind, text: kind === "meta" ? rawLine : rawLine.slice(1), oldNumber: null, newNumber: null };
    if (kind === "add") {
      line.newNumber = newLine++;
      file.additions += 1;
    } else if (kind === "delete") {
      line.oldNumber = oldLine++;
      file.deletions += 1;
    } else if (kind === "context") {
      line.oldNumber = oldLine++;
      line.newNumber = newLine++;
    }
    hunk.lines.push(line);
  }
  return files;
}

function untrackedFile(path, content) {
  const lines = String(content).split("\n");
  if (lines.at(-1) === "") lines.pop();
  return {
    path,
    oldPath: "",
    scope: "untracked",
    additions: lines.length,
    deletions: 0,
    hunks: [{
      header: `@@ -0,0 +1,${lines.length} @@`,
      label: "",
      scope: "untracked",
      lines: lines.map((text, index) => ({ kind: "add", text, oldNumber: null, newNumber: index + 1 })),
    }],
  };
}

export function buildGitReview(snapshot = {}, stagedDiff = "", worktreeDiff = "", untracked = {}) {
  const parsed = [...parseUnifiedDiff(stagedDiff, "staged"), ...parseUnifiedDiff(worktreeDiff, "worktree")];
  const byPath = new Map();
  for (const item of parsed) {
    const current = byPath.get(item.path);
    if (current) {
      current.hunks.push(...item.hunks);
      current.additions += item.additions;
      current.deletions += item.deletions;
      current.scopes.add(item.scope);
    } else {
      byPath.set(item.path, { ...item, scopes: new Set([item.scope]) });
    }
  }
  for (const status of snapshot.files || []) {
    if (!byPath.has(status.path) && Object.hasOwn(untracked, status.path)) {
      const item = untrackedFile(status.path, untracked[status.path]);
      byPath.set(status.path, { ...item, scopes: new Set([item.scope]), status });
      continue;
    }
    const item = byPath.get(status.path) || { path: status.path, oldPath: status.path, hunks: [], additions: 0, deletions: 0, scopes: new Set() };
    item.status = status;
    byPath.set(status.path, item);
  }
  const files = [...byPath.values()].map((file) => ({ ...file, scopes: [...file.scopes] }));
  return {
    files,
    additions: files.reduce((total, file) => total + file.additions, 0),
    deletions: files.reduce((total, file) => total + file.deletions, 0),
  };
}
