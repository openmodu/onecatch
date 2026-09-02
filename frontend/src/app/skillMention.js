export function findSkillTrigger(value, caret = value.length) {
  const safeCaret = Math.max(0, Math.min(Number.isFinite(caret) ? caret : value.length, value.length));
  const match = value.slice(0, safeCaret).match(/(^|\s)\$([A-Za-z0-9._:-]*)$/);
  if (!match) return null;
  return {
    start: safeCaret - match[2].length - 1,
    caret: safeCaret,
    query: match[2],
  };
}

export function filterSkills(skills = [], query = "", limit = 8) {
  const needle = query.trim().toLocaleLowerCase();
  const ranked = skills.map((skill, index) => {
    const name = String(skill.name || "").toLocaleLowerCase();
    const displayName = String(skill.displayName || "").toLocaleLowerCase();
    const description = String(skill.shortDescription || skill.description || "").toLocaleLowerCase();
    let rank = 4;
    if (!needle) rank = 0;
    else if (name.startsWith(needle)) rank = 0;
    else if (name.includes(needle)) rank = 1;
    else if (displayName.includes(needle)) rank = 2;
    else if (description.includes(needle)) rank = 3;
    return { skill, index, rank };
  }).filter((item) => item.rank < 4);
  ranked.sort((left, right) => left.rank - right.rank || left.index - right.index);
  return ranked.slice(0, limit).map((item) => item.skill);
}

export function insertSkill(value, trigger, name) {
  let suffix = value.slice(trigger.caret);
  let separator = " ";
  if (/^[ \t]/.test(suffix)) suffix = suffix.slice(1);
  else if (/^[\r\n]/.test(suffix)) separator = "";
  const token = `$${name}${separator}`;
  return {
    value: value.slice(0, trigger.start) + token + suffix,
    caret: trigger.start + token.length,
  };
}

function isSkillNameCharacter(value) {
  return /[A-Za-z0-9._:-]/.test(value);
}

export function splitSkillMentions(value = "") {
  const text = String(value);
  const parts = [];
  let plainStart = 0;

  for (let index = 0; index < text.length; index += 1) {
    if (text[index] !== "$" || (index > 0 && !/\s/.test(text[index - 1]))) continue;
    let end = index + 1;
    while (end < text.length && isSkillNameCharacter(text[end])) end += 1;
    if (end === index + 1) continue;
    if (plainStart < index) parts.push({ type: "text", value: text.slice(plainStart, index) });
    parts.push({ type: "skill", name: text.slice(index + 1, end), value: text.slice(index, end) });
    plainStart = end;
    index = end - 1;
  }

  if (plainStart < text.length) parts.push({ type: "text", value: text.slice(plainStart) });
  return parts;
}

function skillMentionNodes(value) {
  const parts = splitSkillMentions(value);
  if (!parts.some((part) => part.type === "skill")) return null;
  return parts.map((part) => part.type === "skill" ? {
    type: "link",
    url: `#onecatch-skill:${encodeURIComponent(part.name)}`,
    children: [{ type: "text", value: part.value }],
  } : { type: "text", value: part.value });
}

// Turn Skill mentions into inert links so Streamdown can render them through
// the existing safe link component. Code and existing links stay untouched.
export function remarkSkillMentions() {
  const visit = (node) => {
    if (!node || !Array.isArray(node.children) || ["code", "inlineCode", "link", "linkReference"].includes(node.type)) return;
    const children = [];
    node.children.forEach((child) => {
      if (child?.type === "text") {
        children.push(...(skillMentionNodes(child.value) || [child]));
      } else {
        visit(child);
        children.push(child);
      }
    });
    node.children = children;
  };
  return visit;
}
