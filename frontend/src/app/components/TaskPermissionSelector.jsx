import { Eye, Shield, ShieldCheck } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";

const permissionOptions = [
  { value: "read-only", label: "task.permissionReadOnly", description: "task.permissionReadOnlyDescription", icon: Eye },
  { value: "workspace-write", label: "task.permissionWorkspace", description: "task.permissionWorkspaceDescription", icon: Shield },
  { value: "full", label: "task.permissionFull", description: "task.permissionFullDescription", icon: ShieldCheck },
];

export default function TaskPermissionSelector({ value, allowFull = false, onChange, readOnly = false, className = "" }) {
  const { t } = useTranslation();
  const selected = permissionOptions.find((option) => option.value === value) || permissionOptions[1];
  const SelectedIcon = selected.icon;

  if (readOnly) {
    return <span className={`new-task-select permission is-read-only ${className}`.trim()} aria-label={`${t("task.permission")}: ${t(selected.label)}`} title={t("task.permissionDescription")}>
      <SelectedIcon size={14} aria-hidden="true" />
      <span>{t(selected.label)}</span>
    </span>;
  }

  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" className={`new-task-select permission ${className}`.trim()} aria-label={t("task.permission")} title={t("task.permissionDescription")}>
        <SelectedIcon size={14} aria-hidden="true" />
        <span>{t(selected.label)}</span>
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent className="task-permission-menu" side="top" align="start" sideOffset={8}>
      <DropdownMenuLabel className="task-executor-heading">{t("task.permissionDescription")}</DropdownMenuLabel>
      <DropdownMenuRadioGroup value={value || "workspace-write"} onValueChange={(sandbox) => onChange?.((current) => ({ ...current, sandbox }))}>
        {permissionOptions.map((option) => {
          const Icon = option.icon;
          return <DropdownMenuRadioItem className="task-executor-option" value={option.value} disabled={option.value === "full" && !allowFull} key={option.value}>
            <Icon size={14} aria-hidden="true" />
            <span><strong>{t(option.label)}</strong><small>{t(option.description)}</small></span>
          </DropdownMenuRadioItem>;
        })}
      </DropdownMenuRadioGroup>
    </DropdownMenuContent>
  </DropdownMenu>;
}
