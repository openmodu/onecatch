// The Skills library renders in two React trees that never share a parent: the
// detail card sits in the workbench's main column, while the file tree and its
// editor live in the inspector aside. Threading state between them would mean
// teaching TaskWorkbench about skills, which it otherwise knows nothing about,
// so they stay in step over these window events instead.
//
// The skills inspector is never detached into its own window, so a plain
// CustomEvent on `window` is enough — no Wails event bridge is involved.

export const SKILL_SELECTED_EVENT = "onecatch:skill-selected";
export const SKILL_FILE_OPEN_EVENT = "onecatch:skill-file-open";
export const SKILL_FILE_DRAFT_EVENT = "onecatch:skill-file-draft";

// The inspector starts collapsed and may mount long after a skill was picked,
// so the last selection is retained for whoever subscribes late.
let selection = { name: "", path: "" };

export function publishSkillSelection(value) {
  selection = { name: value?.name || "", path: value?.path || "" };
  window.dispatchEvent(new CustomEvent(SKILL_SELECTED_EVENT, { detail: selection }));
}

export function currentSkillSelection() {
  return selection;
}

// Asks the inspector to open one file for editing. Expanding a collapsed aside
// mounts the inspector in the same render that dispatches this, so the request
// is also retained: the event alone would land before anything is listening.
// The token lets a late subscriber tell a request it has not seen from the one
// it already opened and the user then closed.
let fileRequest = { path: "", token: 0 };

export function requestSkillFile(path) {
  fileRequest = { path: path || "", token: fileRequest.token + 1 };
  window.dispatchEvent(new CustomEvent(SKILL_FILE_OPEN_EVENT, { detail: fileRequest }));
}

export function currentSkillFileRequest() {
  return fileRequest;
}

// `saved` separates a keystroke from a write that reached disk. The detail card
// re-renders its preview on every draft frame — that is the live rendering the
// editor is for — but only a save is allowed to move the on-disk size and
// timestamp it prints.
export function publishSkillFileDraft({ path, content, saved = false }) {
  window.dispatchEvent(new CustomEvent(SKILL_FILE_DRAFT_EVENT, { detail: { path: path || "", content: String(content ?? ""), saved: Boolean(saved) } }));
}

export function subscribeSkillWorkspace(event, handler) {
  window.addEventListener(event, handler);
  return () => window.removeEventListener(event, handler);
}

// `<skill>/SKILL.md` is the one path both sides derive rather than pass around.
export function skillDocumentPath(name) {
  return name ? `${name}/SKILL.md` : "";
}
