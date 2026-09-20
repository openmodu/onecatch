import { createContext, useContext, useState } from "react";
import { ImageOff, RotateCw } from "lucide-react";
import { Dialog, DialogContent, DialogTitle } from "@/components/ui/dialog";
import { conversationImageURL } from "../conversationImages.js";
import { attachmentName, attachmentPath, isImageAttachment } from "../attachments.js";

export const ConversationImageContext = createContext("");

export function ConversationImage({ src, alt = "图片" }) {
  const [open, setOpen] = useState(false);
  const [failed, setFailed] = useState(false);
  const [attempt, setAttempt] = useState(0);
  if (!src) return <span className="markdown-image-placeholder">图片：{alt}</span>;
  if (failed) return <button type="button" className="conversation-image-error" onClick={() => { setAttempt((value) => value + 1); setFailed(false); }}>
    <ImageOff size={16} /><span>{alt} · 加载失败，点按重试</span><RotateCw size={14} />
  </button>;
  return <>
    <button type="button" className="conversation-inline-image" aria-label={`查看图片：${alt}`} onClick={() => setOpen(true)}>
      <img key={attempt} src={src} alt={alt} loading="lazy" referrerPolicy="no-referrer" onError={() => setFailed(true)} />
    </button>
    <Dialog open={open} onOpenChange={setOpen}>
      <DialogContent className="conversation-image-dialog" aria-describedby={undefined}>
        <DialogTitle className="sr-only">{alt}</DialogTitle>
        <img src={src} alt={alt} referrerPolicy="no-referrer" />
        <p>{alt}</p>
      </DialogContent>
    </Dialog>
  </>;
}

export function MobileMessageAttachments({ attachments = [] }) {
  const runID = useContext(ConversationImageContext);
  if (!attachments.length) return null;
  return <div className="mobile-message-attachments">{attachments.map((attachment, index) => {
    const name = attachmentName(attachment);
    const path = attachmentPath(attachment);
    return isImageAttachment(attachment)
      ? <ConversationImage key={`${path}-${index}`} src={conversationImageURL(path, runID)} alt={name} />
      : <span className="mobile-message-file" key={`${path}-${index}`}>{name}</span>;
  })}</div>;
}
