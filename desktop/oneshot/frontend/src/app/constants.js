export const statusLabel = {
  queued: "排队中",
  ready: "等待运行",
  running: "运行中",
  paused: "等待介入",
  completed: "已完成",
  failed: "失败",
  cancelled: "已终止",
  succeeded: "成功",
  interrupted: "已打断",
};

export const runStatusOptions = [
  { value: "", label: "全部状态" },
  { value: "queued", label: "排队中" },
  { value: "running", label: "运行中" },
  { value: "paused", label: "等待介入" },
  { value: "completed", label: "已完成" },
  { value: "failed", label: "失败" },
  { value: "cancelled", label: "已终止" },
];
