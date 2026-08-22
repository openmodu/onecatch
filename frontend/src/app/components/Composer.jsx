import { useRef, useState } from "react";
import { ChevronDown, CircleStop, LoaderCircle, Paperclip, Pause, Workflow } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@/components/ui/dropdown-menu";
import { Action } from "../../ui/primitives.jsx";
import { preserveComposerFocus } from "../composerInteraction.js";
import { shouldSubmitComposer } from "../composerKeyboard.js";
import { fileName } from "../format.js";
import { directAgentWorkflowID, supportsRuntimeProfile } from "../runtimeHarnesses.js";
import HarnessSelector from "./HarnessSelector.jsx";
import RuntimeProfileMenu from "./RuntimeProfileMenu.jsx";
import TaskPermissionSelector from "./TaskPermissionSelector.jsx";

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
  workflowId,
  workflowName,
  permission,
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState("");
  const composing = useRef(false);
  const editable = ["running", "paused", "completed"].includes(runStatus);
  const canSend = Boolean(draft.trim() || attachments.length);
  const directAgent = !workflowId || workflowId === directAgentWorkflowID;
  const workflowLabel = workflowName || t("task.workflowMode");
  const showRuntimeProfile = Boolean(runtimeProfile && supportsRuntimeProfile(runtimeProfile.harness));

  const send = async (modeName) => {
    const accepted = await onSubmit(modeName, draft.trim(), runtimeProfile);
    if (accepted) setDraft("");
  };
  const submitFromComposer = (event) => {
    if (directAgent && runStatus === "running") return;
    if (!shouldSubmitComposer(event, composing.current) || (!canSend && runStatus !== "paused")) return;
    event.preventDefault();
    void send("queue");
  };

  return <div className="workbench-composer shrink-0" onMouseDown={preserveComposerFocus}>
    <div className="workbench-composer-inner">
      <div className={`workbench-composer-shell ${editable ? "" : "disabled"}`.trim()}>
        {pendingInstructions.length > 0 && <div className="instruction-queue grid gap-1.5 rounded-md bg-muted/50 p-2 text-xs text-muted-foreground"><span>{t("composer.nextInstructions", { count: pendingInstructions.length })}</span>{pendingInstructions.map((instruction, index) => <div className={`instruction-item flex items-center gap-2 rounded-sm px-1 py-0.5 ${instruction.priority ? "priority text-warning" : ""}`} key={instruction.id}><b>{instruction.priority ? t("composer.priority") : `#${index + 1}`}</b><span>{instruction.content || t("common.attachmentsCount", { count: instruction.attachments?.length || 0 })}</span><Action size="compact" tone="danger" onClick={() => onRemoveInstruction(instruction.id)}>{t("common.remove")}</Action></div>)}</div>}
        {attachments.length > 0 && <div className="composer-attachments flex flex-wrap gap-1.5">{attachments.map((path) => <span className="attachment-chip inline-flex items-center gap-1.5 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground" key={path} title={path}>{fileName(path)}<Action size="compact" tone="danger" onClick={() => onRemoveAttachment(path)}>{t("common.remove")}</Action></span>)}</div>}
        <div className="workbench-composer-input"><textarea aria-label={t("composer.aria")} value={draft} disabled={!editable} onChange={(event) => setDraft(event.target.value)} onCompositionStart={() => { composing.current = true; }} onCompositionEnd={() => { window.setTimeout(() => { composing.current = false; }, 100); }} onKeyDown={submitFromComposer} placeholder={runStatus === "running" ? t(directAgent ? "composer.directAgentRunningPlaceholder" : "composer.runningPlaceholder") : runStatus === "paused" ? t("composer.pausedPlaceholder") : runStatus === "completed" ? t("composer.continuePlaceholder") : t("composer.finishedPlaceholder")} /></div>
        <div className={`workbench-composer-actions ${showRuntimeProfile ? "profile-visible" : ""}`.trim()}>
          {onChooseAttachments && <Button type="button" variant="ghost" size="icon-sm" className="attachment-action" disabled={!editable} aria-label={t("composer.attachment")} title={t("composer.attachment")} onClick={onChooseAttachments}><Paperclip size={16} aria-hidden="true" /></Button>}
          {runtimeProfile && <div className="workbench-runtime-controls">
            {directAgent ? <HarnessSelector value={runtimeProfile} runtimes={runtimes} readOnly agentLabel /> : <span className="new-task-select executor is-read-only" aria-label={t("task.workflowTargetLabel", { name: workflowLabel })}><Workflow size={14} aria-hidden="true" /><span>{workflowLabel}</span></span>}
            <TaskPermissionSelector value={permission} readOnly />
          </div>}
          {showRuntimeProfile && <RuntimeProfileMenu className="workbench-runtime-profile" value={runtimeProfile} onChange={onRuntimeProfileChange} configuration={runtimeConfiguration?.data} runtimeSettings={runtimeSettings} loading={runtimeConfiguration?.loading} error={runtimeConfiguration?.error} readOnly={runStatus === "running"} />}
          <div className="workbench-composer-submit">{directAgent ? <>
            {runStatus === "running" ? <DropdownMenu>
              <DropdownMenuTrigger asChild><Button type="button" variant="secondary" className="composer-running-trigger" disabled={busy === "interrupt" || busy === "cancel"} aria-label={t("composer.runningActions")}>
                <LoaderCircle className="composer-running-icon" size={14} aria-hidden="true" />
                {t("composer.running")}
                <ChevronDown size={13} aria-hidden="true" />
              </Button></DropdownMenuTrigger>
              <DropdownMenuContent align="end" className="min-w-36">
                <DropdownMenuItem onSelect={onInterrupt}><Pause size={14} aria-hidden="true" />{t("composer.pauseRun")}</DropdownMenuItem>
                <DropdownMenuSeparator />
                <DropdownMenuItem variant="destructive" onSelect={onCancel}><CircleStop size={14} aria-hidden="true" />{t("composer.stop")}</DropdownMenuItem>
              </DropdownMenuContent>
            </DropdownMenu> : ["paused", "completed"].includes(runStatus) && <Action tone="primary" disabled={busy === "queue" || active || (!canSend && runStatus !== "paused")} onClick={() => send("queue")}>{t("composer.sendMessage")}</Action>}
          </> : <>
            {runStatus === "running" && <><Action disabled={busy === "interrupt"} onClick={onInterrupt}>{t("composer.pause")}</Action><Action disabled={!canSend || busy === "queue"} onClick={() => send("queue")}>{t("composer.queueNext")}</Action><Action tone="primary" disabled={!canSend || busy === "insert"} onClick={() => send("insert")}>{t("composer.interruptInsert")}</Action></>}
            {runStatus === "paused" && <><Action tone="danger" disabled={busy === "cancel" || active} onClick={onCancel}>{t("composer.terminate")}</Action><Action tone="primary" disabled={busy === "queue" || active} onClick={() => send("queue")}>{t("composer.resume")}</Action></>}
            {runStatus === "completed" && <Action tone="primary" disabled={!canSend || busy === "queue" || active} onClick={() => send("queue")}>{t("composer.continue")}</Action>}
          </>}</div>
        </div>
      </div>
    </div>
  </div>;
}
