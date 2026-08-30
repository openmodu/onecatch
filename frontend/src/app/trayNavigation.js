export const TRAY_ACTION_EVENT = "onecatch:tray-action";

export function newestTaskRun(runs = []) {
  return [...runs].sort((left, right) => {
    const byActivity = String(right?.updatedAt || "").localeCompare(String(left?.updatedAt || ""));
    if (byActivity) return byActivity;
    return String(left?.id || "").localeCompare(String(right?.id || ""));
  })[0] || null;
}
