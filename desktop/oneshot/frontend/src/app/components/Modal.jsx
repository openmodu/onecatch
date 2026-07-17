import { useTranslation } from "react-i18next";
import { Action, Toolbar } from "../../ui/primitives.jsx";

export default function Modal({ title, subtitle, onClose, children, wide = false }) {
  const { t } = useTranslation();
  if (wide) return <section className="workflow-editor-surface legacy-wide-editor"><Toolbar className="modal-header editor-toolbar"><Action onClick={onClose}>&lt; {t("common.back")}</Action><div><h2>{title}</h2><p>{subtitle}</p></div></Toolbar>{children}</section>;
  return <div className="modal-backdrop"><div className="modal"><div className="modal-header"><div><h2>{title}</h2><p>{subtitle}</p></div><Action size="compact" tone="muted" onClick={onClose}>{t("common.close")}</Action></div>{children}</div></div>;
}
