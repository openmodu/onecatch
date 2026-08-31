export function newSkillTemplate(name, description) {
  const summary = String(description || "").replace(/\s+/g, " ").trim();
  const title = String(name || "")
    .split("-")
    .filter(Boolean)
    .map((part) => part[0]?.toUpperCase() + part.slice(1))
    .join(" ");
  return `---\nname: ${name}\ndescription: ${summary}\n---\n\n# ${title || name}\n\nDescribe when and how the agent should use this skill.\n`;
}

export function syncStatusTone(status) {
  if (status === "synced") return "good";
  if (status === "partial" || status === "out-of-sync") return "warn";
  if (status === "error" || status === "rsync-unavailable") return "danger";
  return "accent";
}

export function formatSkillBytes(value) {
  const bytes = Math.max(0, Number(value) || 0);
  if (bytes < 1024) return `${bytes} B`;
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(bytes < 10 * 1024 ? 1 : 0)} KB`;
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}
