export function nextWorkflowItemID(prefix, steps) {
  const ids = new Set((steps || []).map((step) => step.id));
  const marker = `${prefix}_`;
  let next = 1;
  for (const id of ids) {
    if (!id.startsWith(marker)) continue;
    const suffix = id.slice(marker.length);
    if (!/^\d+$/.test(suffix)) continue;
    const value = Number(suffix);
    if (!Number.isSafeInteger(value)) continue;
    next = Math.max(next, value + 1);
  }
  while (ids.has(`${marker}${next}`)) next += 1;
  return `${marker}${next}`;
}

export function nextWorkflowDefinitionID(base, definitions) {
  const ids = new Set((definitions || []).map((definition) => definition.id));
  if (!ids.has(base)) return base;
  let suffix = 2;
  while (ids.has(`${base}_${suffix}`)) suffix += 1;
  return `${base}_${suffix}`;
}
