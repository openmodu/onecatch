import { useState } from "react";
import { X } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { attachmentName, attachmentPreviewURL, isImageAttachment } from "../attachments.js";
import { fileName } from "../format.js";

export function AttachmentLightbox({ attachment, open, onOpenChange }) {
  const name = attachmentName(attachment);
  return <Dialog open={open} onOpenChange={onOpenChange}>
    <DialogContent className="conversation-image-dialog" aria-describedby={undefined}>
      <DialogTitle className="sr-only">{name}</DialogTitle>
      <img src={attachmentPreviewURL(attachment)} alt={name} />
      <p title={name}>{name}</p>
    </DialogContent>
  </Dialog>;
}

export default function ComposerAttachmentPreview({ path, onRemove, chipClassName = "attachment-chip", removeIconSize = 13 }) {
  const { t } = useTranslation();
  const [previewFailed, setPreviewFailed] = useState(false);
  const [previewOpen, setPreviewOpen] = useState(false);
  const name = fileName(path);
  if (!isImageAttachment(path) || previewFailed) {
    return <span className={chipClassName} title={path}>
      <span>{name}</span>
      <button type="button" aria-label={`${t("common.remove")} ${name}`} title={t("common.remove")} onClick={() => onRemove?.(path)}><X size={removeIconSize} aria-hidden="true" /></button>
    </span>;
  }
  return <div className="composer-image-attachment" title={name}>
    <button type="button" className="composer-image-preview" aria-label={t("timeline.openImage", { name })} onClick={() => setPreviewOpen(true)}>
      <img src={attachmentPreviewURL(path)} alt={name} onError={() => setPreviewFailed(true)} />
    </button>
    <button type="button" className="composer-image-remove" aria-label={`${t("common.remove")} ${name}`} title={t("common.remove")} onClick={() => onRemove?.(path)}><X size={12} aria-hidden="true" /></button>
    <AttachmentLightbox attachment={path} open={previewOpen} onOpenChange={setPreviewOpen} />
  </div>;
}
