import { useState } from "react";
import { Action } from "../../ui/primitives.jsx";
import { fileName } from "../format.js";

// Draft text is intentionally local state: keystrokes re-render only this
// subtree instead of the whole workbench + polling tree, which is what made
// typing feel laggy. The parent still owns attachments and the async submit.
export default function Composer({
  runStatus,
  active,
  busy,
  attachments,
  pendingInstructions,
  onChooseAttachments,
  onRemoveAttachment,
  onRemoveInstruction,
  onInterrupt,
  onCancel,
  onSubmit,
}) {
  const [draft, setDraft] = useState("");
  const editable = ["running", "paused"].includes(runStatus);
  const canSend = Boolean(draft.trim() || attachments.length);

  const send = async (modeName) => {
    const accepted = await onSubmit(modeName, draft.trim());
    if (accepted) setDraft("");
  };

  return <div className="workbench-composer">
    {pendingInstructions.length > 0 && <div className="instruction-queue"><span>下一轮指令 · {pendingInstructions.length}</span>{pendingInstructions.map((instruction, index) => <div className={`instruction-item ${instruction.priority ? "priority" : ""}`} key={instruction.id}><b>{instruction.priority ? "优先" : `#${index + 1}`}</b><span>{instruction.content || `${instruction.attachments?.length || 0} 个附件`}</span><button type="button" onClick={() => onRemoveInstruction(instruction.id)}>移除</button></div>)}</div>}
    {attachments.length > 0 && <div className="composer-attachments">{attachments.map((path) => <span className="attachment-chip" key={path} title={path}>{fileName(path)}<button type="button" onClick={() => onRemoveAttachment(path)}>×</button></span>)}</div>}
    <textarea value={draft} disabled={!editable} onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && (canSend || runStatus === "paused")) send("queue"); }} placeholder={runStatus === "running" ? "补充下一轮指令，或打断当前轮次优先插入…" : runStatus === "paused" ? "补充指令并恢复运行…" : "这次运行已经结束"} />
    <div className="workbench-composer-actions"><button type="button" className="attachment-action" disabled={!editable} onClick={onChooseAttachments}>[ + 附件 ]</button><span>⌘ Enter 提交</span><div>{runStatus === "running" && <><Action disabled={busy === "interrupt"} onClick={onInterrupt}>仅暂停</Action><Action disabled={!canSend || busy === "queue"} onClick={() => send("queue")}>排到下一轮</Action><Action tone="primary" disabled={!canSend || busy === "insert"} onClick={() => send("insert")}>打断并插入</Action></>}{runStatus === "paused" && <><Action tone="danger" disabled={busy === "cancel" || active} onClick={onCancel}>终止</Action><Action tone="primary" disabled={busy === "queue" || active} onClick={() => send("queue")}>恢复运行</Action></>}</div></div>
  </div>;
}
