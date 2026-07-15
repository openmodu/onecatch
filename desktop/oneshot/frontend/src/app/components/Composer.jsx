import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Action } from "../../ui/primitives.jsx";
import { preserveComposerFocus } from "../composerInteraction.js";
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
  const { t } = useTranslation();
  const [draft, setDraft] = useState("");
  const editable = ["running", "paused"].includes(runStatus);
  const canSend = Boolean(draft.trim() || attachments.length);

  const send = async (modeName) => {
    const accepted = await onSubmit(modeName, draft.trim());
    if (accepted) setDraft("");
  };

  return <div className="workbench-composer" onMouseDown={preserveComposerFocus}>
    {pendingInstructions.length > 0 && <div className="instruction-queue"><span>{t("composer.nextInstructions", { count: pendingInstructions.length })}</span>{pendingInstructions.map((instruction, index) => <div className={`instruction-item ${instruction.priority ? "priority" : ""}`} key={instruction.id}><b>{instruction.priority ? t("composer.priority") : `#${index + 1}`}</b><span>{instruction.content || t("common.attachmentsCount", { count: instruction.attachments?.length || 0 })}</span><button type="button" onClick={() => onRemoveInstruction(instruction.id)}>{t("common.remove")}</button></div>)}</div>}
    {attachments.length > 0 && <div className="composer-attachments">{attachments.map((path) => <span className="attachment-chip" key={path} title={path}>{fileName(path)}<button type="button" onClick={() => onRemoveAttachment(path)}>×</button></span>)}</div>}
    <div className={`workbench-composer-input ${editable ? "" : "disabled"}`.trim()}><span aria-hidden="true">&gt;</span><textarea aria-label={t("composer.aria")} value={draft} disabled={!editable} onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && (canSend || runStatus === "paused")) send("queue"); }} placeholder={runStatus === "running" ? t("composer.runningPlaceholder") : runStatus === "paused" ? t("composer.pausedPlaceholder") : t("composer.finishedPlaceholder")} /></div>
    <div className="workbench-composer-actions"><button type="button" className="attachment-action" disabled={!editable} onClick={onChooseAttachments}>[ + {t("composer.attachment")} ]</button><span>{t("composer.submitHint")}</span><div>{runStatus === "running" && <><Action disabled={busy === "interrupt"} onClick={onInterrupt}>{t("composer.pause")}</Action><Action disabled={!canSend || busy === "queue"} onClick={() => send("queue")}>{t("composer.queueNext")}</Action><Action tone="primary" disabled={!canSend || busy === "insert"} onClick={() => send("insert")}>{t("composer.interruptInsert")}</Action></>}{runStatus === "paused" && <><Action tone="danger" disabled={busy === "cancel" || active} onClick={onCancel}>{t("composer.terminate")}</Action><Action tone="primary" disabled={busy === "queue" || active} onClick={() => send("queue")}>{t("composer.resume")}</Action></>}</div></div>
  </div>;
}
