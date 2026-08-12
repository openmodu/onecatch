import { useEffect, useMemo, useRef, useState } from "react";
import { Events } from "@wailsio/runtime";
import {
  Check,
  ChevronDown,
  ChevronRight,
  FileText,
  Grid2X2,
  Info,
  Link2,
  Lock,
  MoreHorizontal,
  Paperclip,
  Pencil,
  Send,
  Sparkles,
  Square,
  TestTube2,
  X,
} from "lucide-react";
import { WhiteboardBinding } from "../../../../bindings/github.com/openmodu/oneshot/internal/transport/wails/index.js";
import {
  applyWhiteboardChange,
  createDemoWhiteboardProposal,
  createInitialWhiteboard,
  mergeWhiteboardProposal,
  moveWhiteboardObject,
  normalizeWhiteboardProposal,
  parseStoredWhiteboard,
  serializeWhiteboardForAgent,
  updateWhiteboardObject,
  whiteboardConnectionPath,
  zoomWhiteboardAt,
} from "../../whiteboardModel.js";

const canvasInteractive = "button, input, textarea, select, a";
const whiteboardRuntimeEvent = "whiteboard:runtime-event";

const wait = (milliseconds) => new Promise((resolve) => window.setTimeout(resolve, milliseconds));

function requestID() {
  return globalThis.crypto?.randomUUID?.() || `whiteboard-${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function runtimeName(runtime) {
  if (runtime === "codex") return "Codex";
  if (runtime === "claude") return "Claude Code";
  if (runtime === "modu") return "Modu Code";
  return runtime;
}

function changeOperation(change) {
  const verb = change.action === "update" ? "更新" : change.action === "link" ? "关联" : "新增";
  return `${verb}提案：${change.title}`;
}

function readBoard(storageKey) {
  try {
    return parseStoredWhiteboard(window.localStorage.getItem(storageKey)) || createInitialWhiteboard();
  } catch {
    return createInitialWhiteboard();
  }
}

function timeLabel() {
  return new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date());
}

function errorText(error) {
  if (typeof error === "string") return error;
  return error?.message || error?.Message || "Agent 运行失败";
}

function EditableText({ value, multiline = false, className = "", label, onChange }) {
  if (multiline) return <textarea aria-label={label} className={className} value={value} spellCheck="false" onChange={(event) => onChange(event.target.value)} />;
  return <input aria-label={label} className={className} value={value} spellCheck="false" onChange={(event) => onChange(event.target.value)} />;
}

function HumanObject({ object, selected, onPointerDown, onChange }) {
  const common = {
    className: `agent-board-object agent-board-object--${object.kind} ${selected ? "selected" : ""}`,
    style: { left: object.x, top: object.y, width: object.width, minHeight: object.height },
    "data-canvas-object": object.id,
    role: "group",
    tabIndex: 0,
    "aria-label": object.title || object.body || "画布对象",
    onPointerDown: (event) => onPointerDown(event, object, "board"),
  };

  if (object.kind === "image") return <figure {...common}><img src={object.src} alt={object.title} draggable="false" /></figure>;

  if (object.kind === "sticky") return <article {...common}>
    <EditableText multiline label="便签内容" className="agent-sticky-copy" value={object.title} onChange={(value) => onChange(object.id, { title: value })} />
  </article>;

  if (object.kind === "handwriting" || object.kind === "handwriting-note") return <article {...common}>
    {object.title && <EditableText label="对象标题" className="agent-hand-title" value={object.title} onChange={(value) => onChange(object.id, { title: value })} />}
    {object.body && <EditableText multiline label="对象内容" className="agent-hand-body" value={object.body} onChange={(value) => onChange(object.id, { body: value })} />}
  </article>;

  if (object.kind === "question") return <article {...common}>
    <EditableText label="开放问题标题" className="agent-question-title" value={object.title} onChange={(value) => onChange(object.id, { title: value })} />
    <EditableText multiline label="开放问题内容" className="agent-question-body" value={object.body} onChange={(value) => onChange(object.id, { body: value })} />
  </article>;

  if (object.kind === "test") return <article {...common}>
    <header><strong>{object.title}</strong><span>已接受</span></header>
    <pre>{object.body}</pre>
    <small>{object.author}</small>
  </article>;

  if (object.kind === "checklist") return <article {...common}>
    <header><strong>{object.title}</strong><span>已接受</span></header>
    <ol>{String(object.body || "").split("\n").filter(Boolean).map((item, index) => <li key={`${object.id}-${index}`}><i>{index + 1}</i>{item}</li>)}</ol>
    <small>{object.author}</small>
  </article>;

  if (object.kind === "file") return <article {...common}>
    <header><FileText size={17} aria-hidden="true" /><strong>{object.title}</strong></header>
    <p>{object.body}</p><small>{object.author}</small>
  </article>;

  return <article {...common}>
    <EditableText label="对象标题" className="agent-native-title" value={object.title} onChange={(value) => onChange(object.id, { title: value })} />
    <EditableText multiline label="对象内容" className="agent-native-body" value={object.body || ""} onChange={(value) => onChange(object.id, { body: value })} />
    {object.author && <small>{object.author}</small>}
  </article>;
}

function ProposalObject({ change, selected, editing, live, opacity, runtime, onPointerDown, onSelect, onUpdate }) {
  if (change.state !== "pending") return null;
  const lines = String(change.content || "").split("\n").filter(Boolean);
  const common = {
    className: `agent-proposal-object agent-proposal-object--${change.objectType} ${selected ? "selected" : ""} ${live ? "agent-proposal-object--live" : ""}`,
    style: { left: change.x, top: change.y, width: change.width, minHeight: change.height, opacity },
    "data-proposal-object": change.id,
    role: "group",
    tabIndex: 0,
    "aria-label": change.title,
    onClick: (event) => { event.stopPropagation(); onSelect(change.id); },
    onPointerDown: (event) => onPointerDown(event, change, "proposal"),
  };

  return <article {...common}>
    <header>
      {editing ? <EditableText label="提案标题" value={change.title} onChange={(value) => onUpdate(change.id, { title: value })} /> : <strong>{change.title}</strong>}
      <span>提案 · {change.objectType === "risk" ? "风险" : change.objectType === "file" ? "文件" : change.objectType === "test" ? "截图" : "结构化"}</span>
    </header>
    {change.objectType === "test" ? <pre>{change.content}</pre>
      : change.objectType === "checklist" ? <ol>{lines.map((line, index) => <li key={`${change.id}-${index}`}><i>{index + 1}</i>{line}</li>)}</ol>
        : change.objectType === "file" ? <div className="agent-proposal-file"><FileText size={20} aria-hidden="true" />{editing ? <EditableText multiline label="提案内容" value={change.content} onChange={(value) => onUpdate(change.id, { content: value })} /> : <p>{change.content}</p>}<Link2 size={16} aria-hidden="true" /></div>
          : editing ? <EditableText multiline label="提案内容" value={change.content} onChange={(value) => onUpdate(change.id, { content: value })} /> : <p>{change.content}</p>}
    <span className="agent-proposal-author">Agent · {runtime === "codex" ? "Codex" : runtime}</span>
  </article>;
}

function AgentOperationOverlay({ busy, runtime, operation, trace, appliedCount }) {
  if (!busy) return null;
  const visibleTrace = trace.slice(-3);
  return <section className="agent-operation-overlay" aria-live="polite">
    <header><i><Sparkles size={14} aria-hidden="true" /></i><p><strong>{runtimeName(runtime)} 正在操作白板</strong><span>实时操作 · 已落板 {appliedCount} 项</span></p></header>
    <ol>
      {visibleTrace.map((item, index) => <li key={item.id} className={index === visibleTrace.length - 1 ? "active" : ""}><i>{item.kind === "canvas_change" ? <Check size={10} aria-hidden="true" /> : <span />}</i><span>{item.text}</span></li>)}
      {!visibleTrace.length && <li className="active"><i><span /></i><span>{operation}</span></li>}
    </ol>
  </section>;
}

const groupOrder = [
  { id: "new", label: "新增" },
  { id: "linked", label: "关联" },
  { id: "confirm", label: "需要确认" },
];

function ChangePanel({ proposal, selectedIDs, focusedID, agentBusy, liveOperation, onToggle, onClearSelected, onFocus, onAcceptSelected }) {
  const pending = proposal.changes.filter((change) => change.state === "pending");
  const selectedCount = pending.filter((change) => selectedIDs.has(change.id)).length;
  return <aside className="agent-change-panel" aria-label="Agent 变更清单">
    <header className="agent-change-panel-header"><strong>变更清单 ({pending.length})</strong><button type="button" onClick={onClearSelected}>全部不选</button><MoreHorizontal size={18} aria-hidden="true" /></header>
    <div className="agent-change-scroll">
      {agentBusy && <div className="agent-live-operation" role="status"><Sparkles size={14} aria-hidden="true" /><p><strong>Agent 正在操作白板</strong><span>{liveOperation}</span></p></div>}
      {groupOrder.map((group) => {
        const changes = proposal.changes.filter((change) => change.category === group.id && change.state === "pending");
        if (!changes.length) return null;
        return <section key={group.id} className="agent-change-group">
          <h3><ChevronDown size={14} aria-hidden="true" />{group.label} ({changes.length})</h3>
          {changes.map((change) => <button key={change.id} type="button" className={`agent-change-card ${focusedID === change.id ? "active" : ""} ${change.state}`} onClick={() => onFocus(change.id)}>
            <span className={`agent-change-check ${selectedIDs.has(change.id) ? "checked" : ""}`} role="checkbox" aria-checked={selectedIDs.has(change.id)} onClick={(event) => { event.stopPropagation(); onToggle(change.id); }}>{selectedIDs.has(change.id) && <Check size={12} aria-hidden="true" />}</span>
            <span className="agent-change-copy"><strong>{change.title}</strong><small>提案 · {change.objectType === "risk" ? "风险" : change.objectType === "file" ? "文件" : change.objectType === "test" ? "截图" : "结构化"}</small><p>{change.content.split("\n")[0]}</p>{change.state === "accepted" && <em>已接受</em>}</span>
          </button>)}
        </section>;
      })}
    </div>
    <footer><p><i><Check size={11} aria-hidden="true" /></i>已选择 {selectedCount} 项变更</p><button type="button" disabled={!selectedCount} onClick={onAcceptSelected}>接受选中项 ({selectedCount})</button></footer>
  </aside>;
}

function ActivityRail({ activity }) {
  return <section className="agent-activity" aria-label="变更记录">
    <header><strong>变更记录</strong><button type="button">查看全部 <ChevronRight size={15} aria-hidden="true" /></button></header>
    <div>{activity.slice(-5).map((item) => <article key={item.id}><i className={item.actor}><span>{item.actor === "agent" ? <Sparkles size={11} /> : <Check size={11} />}</span></i><p><strong>{item.actor === "agent" ? "Agent · Codex" : "你"}</strong><time>{item.at}</time><span>{item.text}</span></p></article>)}</div>
  </section>;
}

export default function WhiteboardPage({ workspace, mode = "demo", runtimes = [], onClose }) {
  const storageKey = `oneshot.whiteboard.agent.v7.${workspace?.id || "demo"}`;
  const [board, setBoard] = useState(() => mode === "demo" ? createInitialWhiteboard() : readBoard(storageKey));
  const [proposal, setProposal] = useState(() => normalizeWhiteboardProposal(mode === "demo" ? createDemoWhiteboardProposal() : { runtime: "codex", summary: "", changes: [] }));
  const [selectedIDs, setSelectedIDs] = useState(() => new Set(mode === "demo" ? ["recovery-order", "recovery-file"] : []));
  const [focusedID, setFocusedID] = useState(mode === "demo" ? "recovery-order" : "");
  const [selectedObjectID, setSelectedObjectID] = useState("");
  const [editingID, setEditingID] = useState("");
  const [viewMode, setViewMode] = useState("proposal");
  const [compare, setCompare] = useState(false);
  const [proposalOpacity, setProposalOpacity] = useState(.6);
  const [instruction, setInstruction] = useState("让 Agent 基于这个框架继续优化…");
  const [runtime, setRuntime] = useState("codex");
  const [agentState, setAgentState] = useState("ready");
  const [agentError, setAgentError] = useState("");
  const [agentSessionID, setAgentSessionID] = useState("");
  const [liveOperation, setLiveOperation] = useState("等待指令");
  const [agentTrace, setAgentTrace] = useState([]);
  const [liveChangeID, setLiveChangeID] = useState("");
  const rootRef = useRef(null);
  const gestureRef = useRef(null);
  const activeRequestRef = useRef("");
  const streamedChangeIDsRef = useRef(new Set());
  const liveChangeTimerRef = useRef(0);
  const objectByID = useMemo(() => new Map(board.objects.map((object) => [object.id, object])), [board.objects]);
  const proposalByID = useMemo(() => new Map(proposal.changes.map((change) => [change.id, change])), [proposal.changes]);
  const focusedChange = proposalByID.get(focusedID);
  const availableRuntimes = useMemo(() => runtimes.filter((item) => item.available !== false), [runtimes]);

  useEffect(() => {
    const next = availableRuntimes.find((item) => item.id === runtime) || availableRuntimes[0];
    if (next && next.id !== runtime) setRuntime(next.id);
  }, [availableRuntimes, runtime]);

  useEffect(() => {
    if (mode === "demo") return undefined;
    const timer = window.setTimeout(() => {
      try { window.localStorage.setItem(storageKey, JSON.stringify(board)); } catch { /* best effort */ }
    }, 140);
    return () => window.clearTimeout(timer);
  }, [board, mode, storageKey]);

  useEffect(() => Events.On(whiteboardRuntimeEvent, (event) => {
    const frame = event?.data;
    if (!frame || frame.requestId !== activeRequestRef.current) return;
    const text = String(frame.text || "Agent 正在处理白板");
    setLiveOperation(text);
    setAgentState(frame.kind === "canvas_change" ? "applying" : frame.kind === "tool_use" || frame.kind === "tool_result" ? "working" : "reading");
    setAgentTrace((current) => [...current, { id: `${frame.requestId}-${frame.seq}`, kind: frame.kind, text }].slice(-8));
    if (frame.kind === "canvas_change" && frame.change?.id) {
      streamedChangeIDsRef.current.add(frame.change.id);
      setViewMode("proposal");
      setProposal((current) => mergeWhiteboardProposal(current, { runtime: current.runtime || "codex", summary: current.summary, changes: [frame.change] }));
      setFocusedID(frame.change.id);
      setLiveChangeID(frame.change.id);
      window.clearTimeout(liveChangeTimerRef.current);
      liveChangeTimerRef.current = window.setTimeout(() => setLiveChangeID(""), 700);
    }
  }), []);

  useEffect(() => () => window.clearTimeout(liveChangeTimerRef.current), []);

  const updateBoardObject = (objectID, patch) => setBoard((current) => ({ ...current, objects: updateWhiteboardObject(current.objects, objectID, patch) }));
  const updateProposal = (changeID, patch) => setProposal((current) => ({ ...current, changes: current.changes.map((change) => change.id === changeID ? { ...change, ...patch } : change) }));

  const beginObjectGesture = (event, object, source) => {
    setSelectedObjectID(source === "board" ? object.id : "");
    if (source === "proposal") setFocusedID(object.id);
    if (event.button !== 0 || event.target.closest(canvasInteractive)) return;
    event.preventDefault();
    event.stopPropagation();
    rootRef.current?.setPointerCapture(event.pointerId);
    gestureRef.current = { type: "drag", source, id: object.id, startX: event.clientX, startY: event.clientY, lastX: 0, lastY: 0 };
  };

  const beginCanvasGesture = (event) => {
    if (event.target.closest?.(`[data-canvas-object], [data-proposal-object], .agent-canvas-controls, ${canvasInteractive}`)) return;
    if (event.button !== 0 && event.button !== 1) return;
    setSelectedObjectID("");
    setFocusedID("");
    if (event.button === 1 || event.altKey || event.shiftKey) {
      gestureRef.current = { type: "pan", startX: event.clientX, startY: event.clientY, originX: board.viewport.x, originY: board.viewport.y };
      rootRef.current?.setPointerCapture(event.pointerId);
    }
  };

  const moveGesture = (event) => {
    const gesture = gestureRef.current;
    if (!gesture) return;
    if (gesture.type === "pan") {
      setBoard((current) => ({ ...current, viewport: { ...current.viewport, x: gesture.originX + event.clientX - gesture.startX, y: gesture.originY + event.clientY - gesture.startY } }));
      return;
    }
    const nextX = (event.clientX - gesture.startX) / board.viewport.scale;
    const nextY = (event.clientY - gesture.startY) / board.viewport.scale;
    const delta = { x: nextX - gesture.lastX, y: nextY - gesture.lastY };
    gesture.lastX = nextX;
    gesture.lastY = nextY;
    if (gesture.source === "proposal") updateProposal(gesture.id, { x: proposalByID.get(gesture.id).x + delta.x, y: proposalByID.get(gesture.id).y + delta.y });
    else setBoard((current) => ({ ...current, objects: moveWhiteboardObject(current.objects, gesture.id, delta) }));
  };

  const finishGesture = (event) => {
    gestureRef.current = null;
    try { rootRef.current?.releasePointerCapture(event.pointerId); } catch { /* capture may be gone */ }
  };

  const handleWheel = (event) => {
    event.preventDefault();
    const rect = rootRef.current.getBoundingClientRect();
    if (event.ctrlKey || event.metaKey) {
      const anchor = { x: event.clientX - rect.left, y: event.clientY - rect.top };
      setBoard((current) => ({ ...current, viewport: zoomWhiteboardAt(current.viewport, current.viewport.scale * Math.exp(-event.deltaY * .0035), anchor) }));
    } else {
      setBoard((current) => ({ ...current, viewport: { ...current.viewport, x: current.viewport.x - event.deltaX, y: current.viewport.y - event.deltaY } }));
    }
  };

  const toggleSelected = (changeID) => setSelectedIDs((current) => {
    const next = new Set(current);
    if (next.has(changeID)) next.delete(changeID); else next.add(changeID);
    return next;
  });

  const acceptChanges = (changeIDs) => {
    const accepted = proposal.changes.filter((change) => changeIDs.has(change.id) && change.state === "pending");
    if (!accepted.length) return;
    setBoard((current) => accepted.reduce((next, change) => applyWhiteboardChange(next, change, timeLabel()), current));
    setProposal((current) => ({ ...current, changes: current.changes.map((change) => changeIDs.has(change.id) ? { ...change, state: "accepted" } : change) }));
    setSelectedIDs((current) => new Set([...current].filter((id) => !changeIDs.has(id))));
    setFocusedID("");
    setEditingID("");
  };

  const ignoreChange = (changeID) => {
    updateProposal(changeID, { state: "ignored" });
    setBoard((current) => ({ ...current, activity: [...current.activity, { id: `ignored-${changeID}-${Date.now()}`, actor: "human", at: timeLabel(), text: `忽略：${proposalByID.get(changeID)?.title || changeID}` }] }));
    setFocusedID("");
  };

  const stageAgentProposal = async (next, currentRequestID) => {
    setAgentState("applying");
    setSelectedIDs(new Set());
    setFocusedID("");
    const remainingChanges = next.changes.filter((change) => !streamedChangeIDsRef.current.has(change.id));
    for (const change of remainingChanges) {
      await wait(180);
      if (activeRequestRef.current !== currentRequestID) return false;
      const operation = changeOperation(change);
      setLiveOperation(operation);
      setAgentTrace((current) => [...current, { id: `${currentRequestID}-canvas-${change.id}`, kind: "canvas_change", text: operation }].slice(-8));
      setProposal((current) => mergeWhiteboardProposal(current, { ...next, changes: [change] }));
      setFocusedID(change.id);
      setLiveChangeID(change.id);
      setBoard((current) => ({ ...current, activity: [...current.activity, { id: `agent-op-${change.id}-${Date.now()}`, actor: "agent", at: timeLabel(), text: operation }] }));
    }
    setProposal((current) => mergeWhiteboardProposal(current, next));
    return true;
  };

  const stopAgent = async () => {
    const currentRequestID = activeRequestRef.current;
    activeRequestRef.current = "";
    if (mode !== "demo" && currentRequestID) await WhiteboardBinding.Cancel(currentRequestID);
    setAgentState("ready");
    setLiveOperation("已停止本轮白板操作");
    setAgentTrace((current) => [...current, { id: `stopped-${Date.now()}`, kind: "stopped", text: "你停止了本轮白板操作" }].slice(-8));
  };

  const submitAgent = async (event) => {
    event.preventDefault();
    const prompt = instruction.trim();
    const agentBusy = ["reading", "working", "applying"].includes(agentState);
    if (!prompt || agentBusy) return;
    const currentRequestID = requestID();
    activeRequestRef.current = currentRequestID;
    streamedChangeIDsRef.current = new Set();
    setLiveChangeID("");
    setAgentState("reading");
    setLiveOperation(`读取 ${board.objects.length} 个正式对象和 ${proposal.changes.filter((change) => change.state === "pending").length} 项待审提案`);
    setAgentTrace([{ id: `${currentRequestID}-read`, kind: "canvas_read", text: "读取当前画布和选中上下文" }]);
    setAgentError("");
    try {
      let result;
      if (mode === "demo") {
        await wait(260);
        setAgentState("working");
        setLiveOperation("检查工作区中的恢复流程与测试基线");
        setAgentTrace((current) => [...current, { id: `${currentRequestID}-tool`, kind: "tool_use", text: "检查工作区中的恢复流程与测试基线" }]);
        await wait(360);
        result = createDemoWhiteboardProposal();
      } else {
        result = await WhiteboardBinding.ProposeChanges({
          workspaceId: workspace?.id || "",
          runtime,
          instruction: prompt,
          canvasJson: JSON.stringify(serializeWhiteboardForAgent(board, selectedObjectID, proposal, focusedID)),
          requestId: currentRequestID,
          sessionId: agentSessionID,
        });
      }
      const next = normalizeWhiteboardProposal(result);
      setAgentSessionID(next.sessionId || agentSessionID);
      const staged = await stageAgentProposal(next, currentRequestID);
      if (!staged) return;
      setSelectedIDs(new Set(next.changes.filter((change) => !change.requiresConfirmation).slice(0, 2).map((change) => change.id)));
      setFocusedID(next.changes[0]?.id || "");
      setBoard((current) => ({ ...current, activity: [...current.activity, { id: `agent-${Date.now()}`, actor: "agent", at: timeLabel(), text: next.summary }] }));
      setInstruction("");
      setLiveOperation(`完成：${next.summary}`);
      setAgentState("ready");
    } catch (error) {
      if (activeRequestRef.current !== currentRequestID) return;
      setAgentError(errorText(error));
      setLiveOperation("Agent 操作失败");
      setAgentState("error");
    }
  };

  const proposalObjects = proposal.changes.filter((change) => change.state === "pending");
  const proposalMap = new Map(proposalObjects.map((change) => [change.id, change]));
  const canvasAndProposalMap = new Map([...board.objects, ...proposalObjects].map((object) => [object.id, object]));
  const proposalConnections = proposalObjects.filter((change) => change.action === "link" && change.targetId && canvasAndProposalMap.has(change.targetId));
  const proposalAnchor = proposalMap.get("recovery-order") || proposalObjects.find((change) => change.objectType === "checklist") || proposalObjects[0];
  const agentBusy = ["reading", "working", "applying"].includes(agentState);
  const cursorAnchor = focusedChange?.state === "pending" ? focusedChange : proposalAnchor;

  return <section className="agent-whiteboard-page">
    <header className="agent-whiteboard-topbar">
      <button type="button" className="agent-project-back" onClick={onClose}>{workspace?.name || "oneshot"}</button>
      <span>{workspace?.path || "~/Code/openmodu/oneshot"}</span><Lock size={14} aria-hidden="true" />
      <button type="button" className={`agent-scope-chip ${agentState}`} title={agentSessionID ? `继续会话 ${agentSessionID}` : "新白板会话"}>（{runtimeName(runtime)} · {agentSessionID ? "继续会话" : "仅此框架"}）<Info size={14} aria-hidden="true" /></button>
    </header>

    <div className="agent-whiteboard-workspace">
      <div className="agent-whiteboard-primary">
        <div className="agent-canvas-shell">
          <div className="agent-canvas-controls">
            <div className="agent-mode-switch"><button type="button" className={viewMode === "mine" ? "active" : ""} onClick={() => setViewMode("mine")}>我的内容</button><button type="button" className={viewMode === "proposal" ? "active" : ""} onClick={() => setViewMode("proposal")}>Agent 提案</button></div>
            <button type="button" className={compare ? "active" : ""} aria-pressed={compare} onClick={() => setCompare((value) => !value)}><Grid2X2 size={15} aria-hidden="true" />对比</button>
            <label>透明度<input aria-label="Agent 提案透明度" type="range" min="35" max="100" value={Math.round(proposalOpacity * 100)} onChange={(event) => setProposalOpacity(Number(event.target.value) / 100)} /><span>{Math.round(proposalOpacity * 100)}%</span></label>
          </div>

          <div
            ref={rootRef}
            className={`agent-canvas-viewport ${compare ? "compare" : ""}`}
            onPointerDown={beginCanvasGesture}
            onPointerMove={moveGesture}
            onPointerUp={finishGesture}
            onPointerCancel={finishGesture}
            onWheel={handleWheel}
          >
            <AgentOperationOverlay busy={agentBusy} runtime={runtime} operation={liveOperation} trace={agentTrace} appliedCount={streamedChangeIDsRef.current.size} />
            <div className="agent-canvas-world" style={{ transform: `translate3d(${board.viewport.x}px, ${board.viewport.y}px, 0) scale(${board.viewport.scale})` }}>
              <svg className="agent-canvas-edges" width="1120" height="800" viewBox="0 0 1120 800" aria-hidden="true">
                {board.connections.map((connection) => <path key={connection.id} className={connection.tone} d={whiteboardConnectionPath(connection, objectByID)} />)}
                {viewMode === "proposal" && proposalConnections.map((change) => <path key={`proposal-${change.id}`} className="proposal" d={whiteboardConnectionPath({ from: change.targetId, to: change.id }, canvasAndProposalMap)} />)}
              </svg>
              {board.objects.map((object) => <HumanObject key={object.id} object={object} selected={selectedObjectID === object.id} onPointerDown={beginObjectGesture} onChange={updateBoardObject} />)}
              {viewMode === "proposal" && proposalObjects.map((change) => <ProposalObject key={change.id} change={change} selected={focusedID === change.id} editing={editingID === change.id} live={liveChangeID === change.id} opacity={.7 + proposalOpacity * .3} runtime={proposal.runtime} onPointerDown={beginObjectGesture} onSelect={setFocusedID} onUpdate={updateProposal} />)}
              {viewMode === "proposal" && cursorAnchor && <div className={`agent-canvas-cursor ${agentBusy ? "working" : ""}`} style={{ left: cursorAnchor.x + cursorAnchor.width - 18, top: cursorAnchor.y + cursorAnchor.height * .63 }}><Send size={24} aria-hidden="true" /><span>Agent · {runtimeName(proposal.runtime)}{agentBusy ? " · 操作中" : ""}</span></div>}
              {viewMode === "proposal" && focusedChange?.state === "pending" && <div className="agent-proposal-actions" style={{ left: Math.min(870, focusedChange.x + focusedChange.width + 12), top: focusedChange.y + 42 }}>
                <button type="button" className="accept" onClick={() => acceptChanges(new Set([focusedChange.id]))}><Check size={15} aria-hidden="true" />接受此项</button>
                <button type="button" onClick={() => setEditingID((id) => id === focusedChange.id ? "" : focusedChange.id)}><Pencil size={14} aria-hidden="true" />调整</button>
                <button type="button" onClick={() => ignoreChange(focusedChange.id)}><X size={15} aria-hidden="true" />忽略</button>
              </div>}
            </div>
          </div>
        </div>

        <form className="agent-composer" onSubmit={submitAgent}>
          <textarea aria-label="给 Agent 的白板指令" value={instruction} placeholder="让 Agent 基于这个框架继续…" onChange={(event) => setInstruction(event.target.value)} onKeyDown={(event) => { if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) { event.preventDefault(); event.currentTarget.form?.requestSubmit(); } }} />
          <div className="agent-composer-meta"><span>@ 框架</span><span># {selectedObjectID ? "1 个选中项" : "选中项"}</span><button type="button"><Paperclip size={14} aria-hidden="true" />附件</button><small>Enter 提交</small></div>
          <select aria-label="Agent 运行时" value={runtime} onChange={(event) => { setRuntime(event.target.value); setAgentSessionID(""); }}>{(availableRuntimes.length ? availableRuntimes : [{ id: "codex", name: "Codex" }]).map((item) => <option key={item.id} value={item.id}>{item.name || item.id}</option>)}</select>
          <button className="agent-composer-submit" type={agentBusy ? "button" : "submit"} aria-label={agentBusy ? "停止 Agent 白板操作" : "提交给 Agent"} disabled={!agentBusy && !instruction.trim()} onClick={agentBusy ? stopAgent : undefined}>{agentBusy ? <X size={18} aria-hidden="true" /> : <Send size={18} aria-hidden="true" />}</button>
          {agentBusy && <p className="agent-running"><Sparkles size={13} aria-hidden="true" />{liveOperation}</p>}
          {agentError && <p className="agent-error">{agentError}</p>}
        </form>
        <ActivityRail activity={board.activity} />
      </div>
      <ChangePanel proposal={proposal} selectedIDs={selectedIDs} focusedID={focusedID} agentBusy={agentBusy} liveOperation={agentTrace.at(-1)?.text || liveOperation} onToggle={toggleSelected} onClearSelected={() => setSelectedIDs(new Set())} onFocus={setFocusedID} onAcceptSelected={() => acceptChanges(selectedIDs)} />
    </div>
  </section>;
}
