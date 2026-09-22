import { useTranslation } from "react-i18next";
import { PromptTemplateLibrary } from "./components/PromptActions.jsx";

export default function TemplateManagerPage({ context, onInsert, canInsert, targetLabel }) {
  const { t } = useTranslation();
  return <section className="template-manager-page" aria-label={t("sidebar.templates")}>
    <PromptTemplateLibrary management context={context} onInsert={onInsert} insertDisabled={!canInsert} targetLabel={targetLabel} />
  </section>;
}
