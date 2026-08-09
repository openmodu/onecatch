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

  return <div className="workbench-composer shrink-0 bg-background px-4 pt-2 pb-3" onMouseDown={preserveComposerFocus}>
    {pendingInstructions.length > 0 && <div className="instruction-queue mb-2 grid gap-1.5 rounded-md border bg-muted/50 p-2 text-xs text-muted-foreground"><span>{t("composer.nextInstructions", { count: pendingInstructions.length })}</span>{pendingInstructions.map((instruction, index) => <div className={`instruction-item flex items-center gap-2 rounded-sm px-1 py-0.5 ${instruction.priority ? "priority text-warning" : ""}`} key={instruction.id}><b>{instruction.priority ? t("composer.priority") : `#${index + 1}`}</b><span>{instruction.content || t("common.attachmentsCount", { count: instruction.attachments?.length || 0 })}</span><Action size="compact" tone="danger" onClick={() => onRemoveInstruction(instruction.id)}>{t("common.remove")}</Action></div>)}</div>}
    {attachments.length > 0 && <div className="composer-attachments mb-2 flex flex-wrap gap-1.5">{attachments.map((path) => <span className="attachment-chip inline-flex items-center gap-1.5 rounded-md border bg-muted px-2 py-1 text-xs text-muted-foreground" key={path} title={path}>{fileName(path)}<Action size="compact" tone="danger" onClick={() => onRemoveAttachment(path)}>{t("common.remove")}</Action></span>)}</div>}
    <div className={`workbench-composer-input flex rounded-md border bg-background px-3 py-2 transition-colors focus-within:border-ring ${editable ? "" : "disabled opacity-60"}`.trim()}><textarea className="min-h-14 w-full resize-none border-0 bg-transparent p-0 text-sm leading-relaxed outline-none focus:outline-none" aria-label={t("composer.aria")} value={draft} disabled={!editable} onChange={(event) => setDraft(event.target.value)} onKeyDown={(event) => { if ((event.metaKey || event.ctrlKey) && event.key === "Enter" && (canSend || runStatus === "paused")) send("queue"); }} placeholder={runStatus === "running" ? t("composer.runningPlaceholder") : runStatus === "paused" ? t("composer.pausedPlaceholder") : t("composer.finishedPlaceholder")} /></div>
    <div className="workbench-composer-actions mt-2 flex items-center gap-3"><Action size="compact" className="attachment-action" disabled={!editable} onClick={onChooseAttachments}>+ {t("composer.attachment")}</Action><span className="text-xs text-muted-foreground">{t("composer.submitHint")}</span><div className="ml-auto flex items-center gap-2">{runStatus === "running" && <><Action disabled={busy === "interrupt"} onClick={onInterrupt}>{t("composer.pause")}</Action><Action disabled={!canSend || busy === "queue"} onClick={() => send("queue")}>{t("composer.queueNext")}</Action><Action tone="primary" disabled={!canSend || busy === "insert"} onClick={() => send("insert")}>{t("composer.interruptInsert")}</Action></>}{runStatus === "paused" && <><Action tone="danger" disabled={busy === "cancel" || active} onClick={onCancel}>{t("composer.terminate")}</Action><Action tone="primary" disabled={busy === "queue" || active} onClick={() => send("queue")}>{t("composer.resume")}</Action></>}</div></div>
  </div>;
}
