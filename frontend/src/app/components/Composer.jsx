import { useLayoutEffect, useRef, useState } from "react";
import { ArrowUp, Paperclip, Pause, Workflow } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import { Action } from "../../ui/primitives.jsx";
import { preserveComposerFocus } from "../composerInteraction.js";
import { shouldSubmitComposer } from "../composerKeyboard.js";
import { autosizeComposerTextarea, WORKBENCH_TEXTAREA_MIN_HEIGHT } from "../composerTextarea.js";
import { fileName } from "../format.js";
import { directAgentWorkflowID, supportsRuntimeProfile } from "../runtimeHarnesses.js";
import HarnessSelector from "./HarnessSelector.jsx";
import ContextGauge from "./ContextGauge.jsx";
import RuntimeProfileMenu from "./RuntimeProfileMenu.jsx";
import { useSkillPicker } from "./SkillPicker.jsx";
import SkillTextarea from "./SkillTextarea.jsx";
import TaskPermissionSelector from "./TaskPermissionSelector.jsx";
import WorkspaceComposerMeta from "./WorkspaceComposerMeta.jsx";

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
  onSubmit,
  runtimeProfile,
  onRuntimeProfileChange,
  runtimes,
  runtimeConfiguration,
  runtimeSettings,
  workflowId,
  workflowName,
  permission,
  contextWindow,
  mode,
  workspace,
  onEditWorkspace,
}) {
  const { t } = useTranslation();
  const [draft, setDraft] = useState("");
  const composing = useRef(false);
  const draftRef = useRef(null);
  const editable = ["running", "paused", "completed"].includes(runStatus);
  const canSend = Boolean(draft.trim() || attachments.length);
  const directAgent = !workflowId || workflowId === directAgentWorkflowID;
  const workflowLabel = workflowName || t("task.workflowMode");
  const showRuntimeProfile = Boolean(runtimeProfile && supportsRuntimeProfile(runtimeProfile.harness));

  useLayoutEffect(() => {
    autosizeComposerTextarea(draftRef.current, WORKBENCH_TEXTAREA_MIN_HEIGHT);
  }, [draft]);

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
  const skillRuntime = directAgent && ["codex", "claude"].includes(runtimeProfile?.harness) ? runtimeProfile.harness : "";
  const skillPicker = useSkillPicker({
    enabled: Boolean(skillRuntime) && editable,
    mode,
    runtime: skillRuntime,
    workspacePath: workspace?.path || "",
    value: draft,
    onValueChange: setDraft,
    textareaRef: draftRef,
    onKeyDown: submitFromComposer,
  });
  const skillHighlight = Boolean(skillRuntime);

  return <div className="workbench-composer shrink-0" onMouseDown={preserveComposerFocus}>
    <div className="workbench-composer-inner">
      <div className={`workbench-composer-shell ${editable ? "" : "disabled"}`.trim()}>
        {pendingInstructions.length > 0 && <div className="instruction-queue grid gap-1.5 rounded-md bg-muted/50 p-2 text-xs text-muted-foreground"><span>{t("composer.nextInstructions", { count: pendingInstructions.length })}</span>{pendingInstructions.map((instruction, index) => <div className={`instruction-item flex items-center gap-2 rounded-sm px-1 py-0.5 ${instruction.priority ? "priority text-warning" : ""}`} key={instruction.id}><b>{instruction.priority ? t("composer.priority") : `#${index + 1}`}</b><span>{instruction.content || t("common.attachmentsCount", { count: instruction.attachments?.length || 0 })}</span><Action size="compact" tone="danger" onClick={() => onRemoveInstruction(instruction.id)}>{t("common.remove")}</Action></div>)}</div>}
        {attachments.length > 0 && <div className="composer-attachments flex flex-wrap gap-1.5">{attachments.map((path) => <span className="attachment-chip inline-flex items-center gap-1.5 rounded-md bg-muted px-2 py-1 text-xs text-muted-foreground" key={path} title={path}>{fileName(path)}<Action size="compact" tone="danger" onClick={() => onRemoveAttachment(path)}>{t("common.remove")}</Action></span>)}</div>}
        <div className="workbench-composer-input"><div className={`codex-skill-field ${skillHighlight ? "has-skill-highlight" : ""}`.trim()}><SkillTextarea ref={draftRef} highlight={skillHighlight} aria-label={t("composer.aria")} value={draft} disabled={!editable} onCompositionStart={() => { composing.current = true; }} onCompositionEnd={() => { window.setTimeout(() => { composing.current = false; }, 100); }} placeholder={runStatus === "running" ? t(directAgent ? "composer.directAgentRunningPlaceholder" : "composer.runningPlaceholder") : runStatus === "paused" ? t("composer.pausedPlaceholder") : runStatus === "completed" ? t("composer.continuePlaceholder") : t("composer.finishedPlaceholder")} {...skillPicker.inputProps} />{skillPicker.menu}</div></div>
        <div className={`workbench-composer-actions ${showRuntimeProfile ? "profile-visible" : ""}`.trim()}>
          {onChooseAttachments && <Button type="button" variant="ghost" size="icon-sm" className="attachment-action" disabled={!editable} aria-label={t("composer.attachment")} title={t("composer.attachment")} onClick={onChooseAttachments}><Paperclip size={16} aria-hidden="true" /></Button>}
          {runtimeProfile && <div className="workbench-runtime-controls">
            {directAgent ? <HarnessSelector value={runtimeProfile} runtimes={runtimes} readOnly agentLabel /> : <span className="new-task-select executor is-read-only" aria-label={t("task.workflowTargetLabel", { name: workflowLabel })}><Workflow size={14} aria-hidden="true" /><span>{workflowLabel}</span></span>}
            <TaskPermissionSelector value={permission} readOnly />
            {/* Context pressure belongs where the context is about to be
                spent, not in the inspector: the moment it changes what you do
                is the moment you are deciding what to type next. */}
            {contextWindow?.known && <ContextGauge {...contextWindow} />}
          </div>}
          {showRuntimeProfile && <RuntimeProfileMenu className="workbench-runtime-profile" value={runtimeProfile} onChange={onRuntimeProfileChange} configuration={runtimeConfiguration?.data} runtimeSettings={runtimeSettings} loading={runtimeConfiguration?.loading} error={runtimeConfiguration?.error} readOnly={runStatus === "running"} />}
          <div className="workbench-composer-submit">{directAgent ? <>
            {runStatus === "running" ? <Button type="button" variant="secondary" size="icon-sm" className="composer-pause-action" disabled={busy === "interrupt"} aria-label={t("composer.pauseRun")} title={t("composer.pauseRun")} onClick={onInterrupt}><Pause size={15} aria-hidden="true" /></Button> : ["paused", "completed"].includes(runStatus) && <Button type="button" size="icon-sm" className="composer-send-action" disabled={busy === "queue" || active || (!canSend && runStatus !== "paused")} aria-label={t("composer.sendMessage")} title={t("composer.sendMessage")} onClick={() => send("queue")}><ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" /></Button>}
          </> : <>
            {runStatus === "running" && <><Action disabled={busy === "interrupt"} onClick={onInterrupt}>{t("composer.pause")}</Action><Action disabled={!canSend || busy === "queue"} onClick={() => send("queue")}>{t("composer.queueNext")}</Action><Action tone="primary" disabled={!canSend || busy === "insert"} onClick={() => send("insert")}>{t("composer.interruptInsert")}</Action></>}
            {runStatus === "paused" && <Button type="button" size="icon-sm" className="composer-send-action" disabled={busy === "queue" || active} aria-label={t("composer.resume")} title={t("composer.resume")} onClick={() => send("queue")}><ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" /></Button>}
            {runStatus === "completed" && <Button type="button" size="icon-sm" className="composer-send-action" disabled={!canSend || busy === "queue" || active} aria-label={t("composer.continue")} title={t("composer.continue")} onClick={() => send("queue")}><ArrowUp size={15} strokeWidth={2.4} aria-hidden="true" /></Button>}
          </>}</div>
        </div>
      </div>
      <WorkspaceComposerMeta mode={mode} workspace={workspace} onEdit={onEditWorkspace} />
    </div>
  </div>;
}
