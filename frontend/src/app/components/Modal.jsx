import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { cn } from "@/lib/utils";
import { Action, Toolbar } from "../../ui/primitives.jsx";

export default function Modal({ title, subtitle, onClose, children, wide = false, className = "" }) {
  const { t } = useTranslation();

  /* `wide` isn't a dialog at all — it's the full-surface workflow editor that
     replaces the main area, so it stays a plain section. */
  if (wide) {
    return (
      <section className="workflow-editor-surface legacy-wide-editor">
        <Toolbar className="modal-header editor-toolbar">
          <Action onClick={onClose}>&lt; {t("common.back")}</Action>
          <div><h2>{title}</h2><p>{subtitle}</p></div>
        </Toolbar>
        {children}
      </section>
    );
  }

  /* Callers mount this conditionally, so it is open whenever it exists.
     Routing close through onOpenChange picks up Esc and overlay clicks, which
     the hand-rolled backdrop never handled. */
  return (
    <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
      <DialogContent className={cn("sm:max-w-xl", className)}>
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {subtitle ? <DialogDescription>{subtitle}</DialogDescription> : null}
        </DialogHeader>
        {children}
      </DialogContent>
    </Dialog>
  );
}
