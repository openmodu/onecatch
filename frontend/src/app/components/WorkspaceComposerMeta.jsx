import { Folder, HardDrive } from "lucide-react";
import { useTranslation } from "react-i18next";
import { StatusBadge } from "../../ui/primitives.jsx";

function workspaceLocation(workspace) {
  return workspace?.remoteFs
    ? `${workspace.remoteFs.username ? `${workspace.remoteFs.username}@` : ""}${workspace.remoteFs.host}:${workspace.remoteFs.root}`
    : workspace?.path || "";
}

export default function WorkspaceComposerMeta({ mode, workspace, onEdit }) {
  const { t } = useTranslation();
  if (!workspace) return null;
  const location = workspaceLocation(workspace);
  return <div className="composer-workspace-meta">
    <button type="button" className="composer-workspace-button" aria-label={`${t("workspace.editProject")} ${workspace.name}`} title={`${t("workspace.editProject")} · ${location}`} onClick={onEdit}>
      <Folder size={14} strokeWidth={2.1} aria-hidden="true" />
      <span>{workspace.name}</span>
    </button>
    <StatusBadge status={mode === "wails" ? "good" : "warn"} className="composer-workspace-location" title={location}>
      <HardDrive size={12} strokeWidth={2.2} aria-hidden="true" />
      {mode === "wails" ? t(workspace.remoteFs ? "workspace.remoteFS" : "common.local") : t("common.preview")}
    </StatusBadge>
  </div>;
}
