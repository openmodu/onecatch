export const WHITEBOARD_MIN_SCALE = 0.55;
export const WHITEBOARD_MAX_SCALE = 1.65;
export const WHITEBOARD_SCHEMA_VERSION = 6;

const clone = (value) => JSON.parse(JSON.stringify(value));

export const initialWhiteboard = {
  schemaVersion: WHITEBOARD_SCHEMA_VERSION,
  viewport: { x: 0, y: 0, scale: 1 },
  objects: [
    { id: "board-title", kind: "handwriting", x: 32, y: 49, width: 280, height: 54, title: "本地 Agent 调度优化", body: "" },
    { id: "goal-cold-start", kind: "sticky", x: 48, y: 129, width: 116, height: 98, title: "缩短执行\n冷启动时间", body: "" },
    { id: "goal-recovery", kind: "sticky", x: 48, y: 245, width: 116, height: 98, title: "提升中断后\n恢复成功率", body: "" },
    { id: "goal-cost", kind: "sticky", x: 48, y: 361, width: 116, height: 98, title: "降低重复\n请求成本", body: "" },
    { id: "goal-copy", kind: "handwriting-note", x: 274, y: 101, width: 240, height: 100, title: "目标", body: "让 Agent 执行更快、\n更稳、更省。" },
    { id: "priority-list", kind: "handwriting-note", x: 220, y: 300, width: 300, height: 156, title: "优先级（粗排）", body: "1. 恢复流程可靠性  ☆☆☆\n2. 冷启动优化      ☆☆☆\n3. 成本节流        ☆☆☆" },
    { id: "failure-trend", kind: "image", x: 38, y: 542, width: 264, height: 162, title: "当前失败率趋势（近 7 天）", src: "/assets/whiteboard/agent-failure-trend.png" },
    { id: "trend-note", kind: "handwriting-note", x: 98, y: 712, width: 230, height: 42, title: "", body: "稳定在 5% 以内 ☆" },
    { id: "open-question", kind: "question", x: 348, y: 536, width: 280, height: 152, title: "开放问题", body: "多机并发恢复是否需要\n分布式锁……？" },
  ],
  connections: [
    { id: "sticky-goal", from: "goal-cold-start", to: "goal-copy", tone: "human" },
  ],
  acceptedChangeIDs: [],
  activity: [
    { id: "activity-create", actor: "human", at: "10:21", text: "创建框架：本地 Agent 调度优化" },
    { id: "activity-order", actor: "agent", at: "10:28", text: "在任务序列（建议）中新增 5 项任务" },
    { id: "activity-risk", actor: "agent", at: "10:29", text: "新增风险提示 2" },
    { id: "activity-accept", actor: "human", at: "10:31", text: "审阅并接受 1 项任务" },
    { id: "activity-file", actor: "agent", at: "10:32", text: "建议关联 design/recovery-flow.md" },
  ],
};

export const demoWhiteboardProposal = {
  runtime: "codex",
  sessionId: "demo-whiteboard-session",
  summary: "已基于当前框架提出 5 项可审阅变更",
  changes: [
    { id: "test-baseline", action: "add", objectType: "test", category: "new", title: "测试截图（建议保留）", content: "$ go test ./internal/usecase/workflows\n\nok  internal/...  0.542s\nok  pkg/...       0.126s\n\nPASS", x: 510, y: 84, width: 276, height: 150, requiresConfirmation: false },
    { id: "recovery-order", action: "add", objectType: "checklist", category: "new", title: "任务序列（建议）", content: "诊断中断恢复失败原因\n优化恢复流程幂等性\n缓存冷启动依赖\n引入限流与重试策略\n验证并压测", x: 440, y: 270, width: 252, height: 248, requiresConfirmation: false },
    { id: "risk-disk", action: "add", objectType: "risk", category: "new", title: "风险提示 1", content: "磁盘写入高峰可能导致恢复延迟抖动。", x: 694, y: 518, width: 194, height: 92, requiresConfirmation: false },
    { id: "risk-lock", action: "add", objectType: "risk", category: "confirm", title: "风险提示 2", content: "过度缓存可能导致内存占用上升。", x: 694, y: 604, width: 194, height: 92, requiresConfirmation: true },
    { id: "recovery-file", action: "link", objectType: "file", category: "linked", title: "关联文件（建议新增）", content: "design/recovery-flow.md\n在 docs/ 下创建并关联", targetId: "open-question", x: 590, y: 694, width: 298, height: 86, requiresConfirmation: false },
  ],
};

export function createInitialWhiteboard() {
  return clone(initialWhiteboard);
}

export function createDemoWhiteboardProposal() {
  return clone(demoWhiteboardProposal);
}

export function clampWhiteboardScale(scale) {
  return Math.min(WHITEBOARD_MAX_SCALE, Math.max(WHITEBOARD_MIN_SCALE, scale));
}

export function screenToWorld(point, viewport) {
  return {
    x: (point.x - viewport.x) / viewport.scale,
    y: (point.y - viewport.y) / viewport.scale,
  };
}

export function zoomWhiteboardAt(viewport, scale, anchor) {
  const nextScale = clampWhiteboardScale(scale);
  const world = screenToWorld(anchor, viewport);
  return {
    x: anchor.x - world.x * nextScale,
    y: anchor.y - world.y * nextScale,
    scale: nextScale,
  };
}

export function moveWhiteboardObject(objects, objectID, delta) {
  return objects.map((object) => object.id === objectID
    ? { ...object, x: object.x + delta.x, y: object.y + delta.y }
    : object);
}

export function updateWhiteboardObject(objects, objectID, patch) {
  return objects.map((object) => object.id === objectID ? { ...object, ...patch } : object);
}

export function whiteboardConnectionPath(connection, objectByID) {
  const from = objectByID.get(connection.from);
  const to = objectByID.get(connection.to);
  if (!from || !to) return "";
  const x1 = from.x + from.width;
  const y1 = from.y + from.height / 2;
  const x2 = to.x;
  const y2 = to.y + to.height / 2;
  const bend = Math.max(44, Math.abs(x2 - x1) * 0.38);
  return `M ${x1} ${y1} C ${x1 + bend} ${y1}, ${x2 - bend} ${y2}, ${x2} ${y2}`;
}

export function normalizeWhiteboardProposal(proposal) {
  const changes = Array.isArray(proposal?.changes) ? proposal.changes.slice(0, 12) : [];
  return {
    runtime: String(proposal?.runtime || "codex"),
    sessionId: String(proposal?.sessionId || ""),
    summary: String(proposal?.summary || "Agent 已生成画布提案"),
    changes: changes.map((change, index) => ({
      id: String(change?.id || `agent-change-${index + 1}`),
      action: ["add", "update", "link"].includes(change?.action) ? change.action : "add",
      objectType: ["card", "checklist", "risk", "file", "test", "note"].includes(change?.objectType) ? change.objectType : "card",
      category: ["new", "linked", "confirm"].includes(change?.category) ? change.category : "new",
      title: String(change?.title || `Agent 提案 ${index + 1}`),
      content: String(change?.content || ""),
      targetId: String(change?.targetId || ""),
      x: Number.isFinite(change?.x) ? Math.min(980, Math.max(420, change.x)) : 520 + (index % 2) * 220,
      y: Number.isFinite(change?.y) ? Math.min(700, Math.max(70, change.y)) : 90 + index * 116,
      width: Number.isFinite(change?.width) ? Math.min(360, Math.max(180, change.width)) : 240,
      height: Number.isFinite(change?.height) ? Math.min(280, Math.max(82, change.height)) : 120,
      requiresConfirmation: Boolean(change?.requiresConfirmation || change?.category === "confirm"),
      state: "pending",
    })),
  };
}

export function mergeWhiteboardProposal(current, incoming) {
  const next = normalizeWhiteboardProposal(incoming);
  const changes = [...(current?.changes || [])];
  for (const operation of next.changes) {
    const index = changes.findIndex((change) => change.id === operation.id);
    if (index >= 0) changes[index] = { ...changes[index], ...operation, state: "pending" };
    else changes.push(operation);
  }
  return { ...next, changes: changes.slice(-12) };
}

function acceptedObjectKind(change) {
  if (change.objectType === "checklist") return "checklist";
  if (change.objectType === "file") return "file";
  if (change.objectType === "risk") return "note";
  if (change.objectType === "test") return "test";
  return "note";
}

export function applyWhiteboardChange(board, change, at = "刚刚") {
  if (!change || change.state === "accepted") return board;
  const objectID = change.action === "update" && change.targetId ? change.targetId : `accepted-${change.id}`;
  let objects = board.objects;
  let connections = board.connections;
  if (change.action === "update" && change.targetId && objects.some((object) => object.id === change.targetId)) {
    objects = updateWhiteboardObject(objects, change.targetId, { title: change.title, body: change.content });
  } else {
    const object = {
      id: objectID,
      kind: acceptedObjectKind(change),
      x: change.x,
      y: change.y,
      width: change.width,
      height: change.height,
      title: change.title,
      body: change.content,
      author: "Agent · 已接受",
    };
    objects = [...objects.filter((item) => item.id !== objectID), object];
  }
  if (change.action === "link" && change.targetId && objects.some((object) => object.id === change.targetId)) {
    const connectionID = `${change.targetId}-${objectID}`;
    if (!connections.some((connection) => connection.id === connectionID)) {
      connections = [...connections, { id: connectionID, from: change.targetId, to: objectID, tone: "accepted" }];
    }
  }
  return {
    ...board,
    objects,
    connections,
    acceptedChangeIDs: [...new Set([...(board.acceptedChangeIDs || []), change.id])],
    activity: [...(board.activity || []), { id: `activity-${change.id}-${Date.now()}`, actor: "human", at, text: `接受：${change.title}` }],
  };
}

export function serializeWhiteboardForAgent(board, selectedID = "", proposal = null, focusedChangeID = "") {
  return {
    canvas: { width: 1120, height: 800 },
    selectedObjectId: selectedID,
    objects: board.objects.map(({ id, kind, x, y, width, height, title, body, author }) => ({ id, kind, x, y, width, height, title, body, author })),
    connections: board.connections,
    acceptedChangeIds: board.acceptedChangeIDs || [],
    review: proposal ? {
      summary: String(proposal.summary || ""),
      focusedChangeId: String(focusedChangeID || ""),
      pendingChanges: (proposal.changes || []).filter((change) => change.state === "pending").map(({ id, action, objectType, category, title, content, targetId, x, y, width, height, requiresConfirmation }) => ({ id, action, objectType, category, title, content, targetId, x, y, width, height, requiresConfirmation })),
    } : { summary: "", focusedChangeId: "", pendingChanges: [] },
  };
}

export function parseStoredWhiteboard(value) {
  try {
    const parsed = JSON.parse(value);
    if (!parsed || parsed.schemaVersion !== WHITEBOARD_SCHEMA_VERSION || !Array.isArray(parsed.objects) || !Array.isArray(parsed.connections)) return null;
    return {
      schemaVersion: WHITEBOARD_SCHEMA_VERSION,
      viewport: parsed.viewport && Number.isFinite(parsed.viewport.scale) ? parsed.viewport : { x: 0, y: 0, scale: 1 },
      objects: parsed.objects,
      connections: parsed.connections,
      acceptedChangeIDs: Array.isArray(parsed.acceptedChangeIDs) ? parsed.acceptedChangeIDs : [],
      activity: Array.isArray(parsed.activity) ? parsed.activity : [],
    };
  } catch {
    return null;
  }
}
