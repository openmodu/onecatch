import { useTranslation } from "react-i18next";

import { Input } from "@/components/ui/input";
import { SettingsField } from "../settings/SettingsControls.jsx";

export default function WorkflowIdentityFields({ editor, setEditor, validation = [] }) {
  const { t } = useTranslation();
  const invalid = (path) => validation.some((issue) => issue.path === path);
  return <div className="grid min-w-0 gap-4 sm:grid-cols-2" aria-label={t("workflow.identity")}>
    <SettingsField label={t("workflow.name")}>
      <Input aria-label={t("workflow.name")} aria-invalid={invalid("name")} value={editor.name} onChange={(event) => setEditor((current) => ({ ...current, name: event.target.value }))} />
    </SettingsField>
    <SettingsField label={t("workflow.id")}>
      <Input className="font-mono" aria-label={t("workflow.id")} aria-invalid={invalid("id")} spellCheck="false" value={editor.id} onChange={(event) => setEditor((current) => ({ ...current, id: event.target.value }))} />
    </SettingsField>
  </div>;
}
