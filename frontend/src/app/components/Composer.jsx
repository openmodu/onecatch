import { useState } from "react";
import { Paperclip } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Action } from "../../ui/primitives.jsx";
import { preserveComposerFocus } from "../composerInteraction.js";
import { fileName } from "../format.js";
import HarnessSelector from "./HarnessSelector.jsx";
import RuntimeProfileMenu from "./RuntimeProfileMenu.jsx";

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
  runtimeProfile,
  onRuntimeProfileChange,
  runtimes,
  runtimeConfiguration,
  runtimeSettings,
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState("");
  const editable = ["running", "paused", "completed"].includes(runStatus);
  const canSend = Boolean(draft.trim() || attachments.length);

  const send = async (modeName) => {
    const accepted = await onSubmit(modeName, draft.trim(), runtimeProfile);
    if (accepted) setDraft("");
  };
  const submitFromComposer = (event) => {
    if (event.key !== "Enter" || event.shiftKey || event.nativeEvent.isComposing || (!canSend && runStatus !== "paused")) return;
    event.preventDefault();
    void send("queue");
  };

  return <div className="workbench-composer shrink-0" onMouseDown={preserveComposerFocus}>
    <div className="workbench-composer-inner">
      <div className={`workbench-composer-shell ${editable ? "" : "disabled"}`.trim()}>
        {pendingInstructions.length > 0 && <div className="instruction-queue grid gap-1.5 rounded-md bg-muted/50 p-2 text-xs text-muted-foreground"><span>{t("composer.nextInstructions", { count: pendingInstructions.length })}</span>{pendingInstructions.map((instruction, index) => <div className={`instruction-item flex items-center gap-2 rounded-sm px-1 py-0.5 ${instruction.priority ? "priority text-warning" : ""}`} key={instruction.id}><b>{instruction.priority ? t("composer.priority") : `#${index + 1}`}</b><span>{instruction.content || t("common.attachmentsCount", { count: instruction.attachments?.length || 0 })}</span><Action size="compact" tone="danger" onClick={() => onRemoveInstruction(instruction.id)}>{t("common.remove")}</Action></div>)}</div>}
        {attachments.length > 0 && <div className="composer-attachments flex flex-wrap gap-1.5">{attachments.map((path) => <span className="attachment-chip inline-flex items-center gap-1.5 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground" key={path} title={path}>{fileName(path)}<Action size="compact" tone="danger" onClick={() => onRemoveAttachment(path)}>{t("common.remove")}</Action></span>)}</div>}
        <div className="workbench-composer-input"><textarea aria-label={t("composer.aria")} value={draft} disabled={!editable} onChange={(event) => setDraft(event.target.value)} onKeyDown={submitFromComposer} placeholder={runStatus === "running" ? t("composer.runningPlaceholder") : runStatus === "paused" ? t("composer.pausedPlaceholder") : runStatus === "completed" ? t("composer.continuePlaceholder") : t("composer.finishedPlaceholder")} /></div>
        <div className="workbench-composer-actions"><Button type="button" variant="ghost" size="icon-sm" className="attachment-action" disabled={!editable} aria-label={t("composer.attachment")} title={t("composer.attachment")} onClick={onChooseAttachments}><Paperclip size={16} aria-hidden="true" /></Button>{runtimeProfile && <div className="workbench-runtime-controls"><HarnessSelector value={runtimeProfile} onChange={onRuntimeProfileChange} runtimes={runtimes} readOnly={runStatus === "running"} /><RuntimeProfileMenu className="workbench-runtime-profile" value={runtimeProfile} onChange={onRuntimeProfileChange} configuration={runtimeConfiguration?.data} runtimeSettings={runtimeSettings} loading={runtimeConfiguration?.loading} error={runtimeConfiguration?.error} readOnly={runStatus === "running"} /></div>}<div className="workbench-composer-submit">{runStatus === "running" && <><Action disabled={busy === "interrupt"} onClick={onInterrupt}>{t("composer.pause")}</Action><Action disabled={!canSend || busy === "queue"} onClick={() => send("queue")}>{t("composer.queueNext")}</Action><Action tone="primary" disabled={!canSend || busy === "insert"} onClick={() => send("insert")}>{t("composer.interruptInsert")}</Action></>}{runStatus === "paused" && <><Action tone="danger" disabled={busy === "cancel" || active} onClick={onCancel}>{t("composer.terminate")}</Action><Action tone="primary" disabled={busy === "queue" || active} onClick={() => send("queue")}>{t("composer.resume")}</Action></>}{runStatus === "completed" && <Action tone="primary" disabled={!canSend || busy === "queue" || active} onClick={() => send("queue")}>{t("composer.continue")}</Action>}</div></div>
      </div>
    </div>
  </div>;
}
