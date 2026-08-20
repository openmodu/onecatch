import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { SettingsButton, SettingsKicker } from "./SettingsControls.jsx";

// Every window asks the user to confirm something destructive, so this lives
// next to the settings controls it borrows rather than inside the settings
// screen — importing it must not pull that screen along.
export function ConfirmDialog({ dialog, busy = false, onCancel, onConfirm }) {
  const { t } = useTranslation();
  if (!dialog) return null;
  return <Dialog open onOpenChange={(open) => !open && !busy && onCancel()}>
    <DialogContent
      showCloseButton={false}
      className="sm:max-w-md"
      onEscapeKeyDown={(event) => busy && event.preventDefault()}
      onPointerDownOutside={(event) => busy && event.preventDefault()}
    >
      <DialogHeader>
        <SettingsKicker>{dialog.eyebrow || t(dialog.dangerous ? "modal.dangerous" : "modal.confirmChange")}</SettingsKicker>
        <DialogTitle>{dialog.title}</DialogTitle>
        <DialogDescription>{dialog.description}</DialogDescription>
      </DialogHeader>
      {dialog.detail && <div className="rounded-md bg-muted px-3 py-2 text-sm text-muted-foreground">{dialog.detail}</div>}
      <DialogFooter>
        <SettingsButton tone="muted" disabled={busy} onClick={onCancel}>{dialog.cancelLabel || t("common.cancel")}</SettingsButton>
        <SettingsButton autoFocus tone={dialog.dangerous ? "danger" : "primary"} disabled={busy} onClick={onConfirm}>{busy ? t("common.processing") : dialog.confirmLabel || t("common.confirm")}</SettingsButton>
      </DialogFooter>
    </DialogContent>
  </Dialog>;
}
