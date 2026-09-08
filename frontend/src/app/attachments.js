import { fileName } from "./format.js";

const imageExtensions = /\.(?:gif|jpe?g|png|webp)$/i;
const supportedClipboardImageTypes = new Set(["image/gif", "image/jpeg", "image/png", "image/webp"]);

export function isImageAttachment(attachment) {
  const mimeType = typeof attachment === "object" ? attachment?.mimeType : "";
  const path = typeof attachment === "object" ? attachment?.storedPath || attachment?.path || "" : attachment;
  return String(mimeType).startsWith("image/") || imageExtensions.test(String(path || ""));
}

export function attachmentPath(attachment) {
  return typeof attachment === "object" ? attachment?.storedPath || attachment?.path || "" : String(attachment || "");
}

export function attachmentName(attachment) {
  if (typeof attachment === "object" && attachment?.name) return attachment.name;
  return fileName(attachmentPath(attachment)).replace(/^attachment_[a-f0-9]+-/, "");
}

export function attachmentPreviewURL(attachment) {
  return `/attachment-preview?path=${encodeURIComponent(attachmentPath(attachment))}`;
}

export function pastedImageFiles(event) {
  const files = Array.from(event.clipboardData?.items || [])
    .filter((item) => item.kind === "file" && supportedClipboardImageTypes.has(item.type))
    .map((item) => item.getAsFile())
    .filter(Boolean);
  if (files.length) event.preventDefault();
  return files;
}

export function blobBase64(blob) {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onerror = () => reject(reader.error || new Error("Unable to read pasted image"));
    reader.onload = () => resolve(String(reader.result || "").split(",", 2)[1] || "");
    reader.readAsDataURL(blob);
  });
}

export function pastedImageName(file, index = 0, now = new Date()) {
  const supplied = String(file?.name || "").trim();
  if (supplied && supplied !== "image.png") return supplied;
  const stamp = now.toISOString().replace(/[-:]/g, "").replace(/\.\d{3}Z$/, "Z");
  const extension = file?.type === "image/jpeg" ? "jpg" : file?.type?.split("/")[1] || "png";
  return `Screenshot-${stamp}${index ? `-${index + 1}` : ""}.${extension}`;
}
