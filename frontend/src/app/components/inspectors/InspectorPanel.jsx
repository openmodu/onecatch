import { useEffect, useState } from "react";
import { Activity, FileCode2, FileDiff, GitBranch, ListTree } from "lucide-react";
import { useTranslation } from "react-i18next";
import StatusInspector from "./StatusInspector.jsx";
import GitInspector from "./GitInspector.jsx";
import EventInspector from "./EventInspector.jsx";
import FileInspector from "./FileInspector.jsx";
import ReviewPanel from "./ReviewPanel.jsx";
import SkillFilesInspector from "./SkillFilesInspector.jsx";

// The tab strip, its body and the file editor's mount bookkeeping, shared by
// the workbench dock and the detached inspector window so the two surfaces can
// never drift apart. Everything window-specific — the frame, and which buttons
// sit on the right of the toolbar — is supplied by the caller.
export default function InspectorPanel({ className = "", scope = "task", mode, workspaceID, remoteFS = null, detail, queuedTask, queuePosition = 0, draft = false, runWorkerID = "", reviewRequest = 0, notify, onOpenTerminal, onDirtyChange, actions = null }) {
  const { t } = useTranslation();
  const [tab, setTab] = useState("status");
  const [fileInspectorMounted, setFileInspectorMounted] = useState(false);

  useEffect(() => { if (scope === "task" && reviewRequest > 0) setTab("review"); }, [reviewRequest, scope]);

  if (scope === "skills") return <aside className={className} aria-label={t("skill.files")}>
    <div className="workbench-inspector-toolbar">
      <strong className="workbench-inspector-title pl-3">{t("skill.files")}</strong>
      <div className="workbench-inspector-window-actions ml-auto">{actions}</div>
    </div>
    <div className="workbench-inspector-body min-h-0 overflow-hidden" id="workbench-inspector-content">
      <SkillFilesInspector mode={mode} />
    </div>
  </aside>;

  const tabs = [
    { value: "status", label: t("inspector.status"), icon: Activity },
    { value: "files", label: t("inspector.files"), icon: FileCode2 },
    { value: "git", label: t("inspector.git"), icon: GitBranch },
    { value: "review", label: t("review.title"), icon: FileDiff },
    { value: "events", label: t("inspector.events"), icon: ListTree },
  ];
  const activeTab = tabs.find((item) => item.value === tab) || tabs[0];

  return <aside className={className} aria-label={t("inspector.aria")}>
    <div className="workbench-inspector-toolbar">
      <div className="workbench-inspector-tabs" role="tablist" aria-label={t("inspector.aria")}>
        {tabs.map(({ value, label, icon: Icon }) => <button
          type="button"
          role="tab"
          className={tab === value ? "active" : ""}
          aria-selected={tab === value}
          aria-controls="workbench-inspector-content"
          title={label}
          key={value}
          onClick={() => { setTab(value); if (value === "files") setFileInspectorMounted(true); }}
        ><Icon size={15} strokeWidth={2} aria-hidden="true" /><span className="sr-only">{label}</span></button>)}
      </div>
      <strong className="workbench-inspector-title">{activeTab.label}</strong>
      <div className="workbench-inspector-window-actions">{actions}</div>
    </div>
    <div className={`workbench-inspector-body min-h-0 ${tab === "files" || tab === "review" ? "overflow-hidden" : "overflow-y-auto"}`} id="workbench-inspector-content">
      {fileInspectorMounted && <div className={tab === "files" ? "h-full" : "hidden"}><FileInspector mode={mode} workspaceID={workspaceID} active={tab === "files"} notify={notify} onDirtyChange={onDirtyChange} /></div>}
      {tab === "status" ? <StatusInspector detail={draft ? null : detail} queuedTask={draft ? null : queuedTask} queuePosition={draft ? 0 : queuePosition} draft={draft} notify={notify} onOpenTerminal={onOpenTerminal} />
        : tab === "git" ? <GitInspector mode={mode} workspaceID={workspaceID} remoteFS={remoteFS} runWorkerID={draft ? "" : runWorkerID} notify={notify} />
          : tab === "review" ? <ReviewPanel mode={mode} workspaceID={workspaceID} active notify={notify} onClose={() => setTab("status")} />
          : tab === "events" ? <EventInspector detail={draft ? null : detail} />
            : null}
    </div>
  </aside>;
}
