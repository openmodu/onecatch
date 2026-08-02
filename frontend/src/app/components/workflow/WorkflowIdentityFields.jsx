import { useTranslation } from "react-i18next";

export default function WorkflowIdentityFields({ editor, setEditor, validation = [] }) {
  const { t } = useTranslation();
  const invalid = (path) => validation.some((issue) => issue.path === path);
  return <div className="workflow-identity" aria-label={t("workflow.identity")}>
    <label className="workflow-identity__name">
      <span>{t("workflow.name")}</span>
      <input aria-label={t("workflow.name")} aria-invalid={invalid("name")} value={editor.name} onChange={(event) => setEditor((current) => ({ ...current, name: event.target.value }))} />
    </label>
    <label className="workflow-identity__id">
      <span>{t("workflow.id")}</span>
      <input aria-label={t("workflow.id")} aria-invalid={invalid("id")} spellCheck="false" value={editor.id} onChange={(event) => setEditor((current) => ({ ...current, id: event.target.value }))} />
    </label>
  </div>;
}
