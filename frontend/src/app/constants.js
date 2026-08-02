export const statusKey = (status) => `status.${status || "ready"}`;

export const runStatusOptions = (t) => [
  { value: "", label: t("status.all") },
  ...["queued", "running", "paused", "completed", "failed", "cancelled"].map((value) => ({ value, label: t(statusKey(value)) })),
];
