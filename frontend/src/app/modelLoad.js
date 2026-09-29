// A missing queue size is unknown, not zero. Load above 100% signals pressure
// but cannot tell us an exact queue length or waiting time.
export function modelLoadStatus(model) {
  const load = model?.load;
  const percent = Number.isFinite(load?.percent) && load.percent >= 0 ? load.percent : null;
  const queueSize = Number.isFinite(load?.queueSize) && load.queueSize >= 0 ? load.queueSize : null;
  if (percent === null && queueSize === null) return null;
  return { percent, queueSize, busy: queueSize > 0 || percent >= 100 };
}

export function modelLoadLabel(model, t) {
  const status = modelLoadStatus(model);
  if (!status) return "";
  const parts = [];
  if (status.queueSize > 0) parts.push(t("settings.modelQueueSize", { count: status.queueSize }));
  else if (status.queueSize === 0) parts.push(t("settings.modelQueueEmpty"));
  else if (status.busy) parts.push(t("settings.modelMayQueue"));
  if (status.percent !== null) parts.push(t("settings.modelLoadPercent", { percent: status.percent }));
  return parts.join(" · ");
}
