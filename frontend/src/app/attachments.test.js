import assert from "node:assert/strict";
import test from "node:test";
import { attachmentName, attachmentPreviewURL, isImageAttachment, pastedImageFiles, pastedImageName } from "./attachments.js";

test("attachment metadata produces safe image previews and readable names", () => {
  const attachment = { name: "界面截图.png", storedPath: "/tmp/task/attachment_abcd1234-reference.png", mimeType: "image/png" };
  assert.equal(isImageAttachment(attachment), true);
  assert.equal(attachmentName(attachment), "界面截图.png");
  assert.equal(attachmentPreviewURL(attachment), "/attachment-preview?path=%2Ftmp%2Ftask%2Fattachment_abcd1234-reference.png");
  assert.equal(attachmentName("/tmp/attachment_abcd1234-reference.png"), "reference.png");
});

test("clipboard handling consumes supported images without intercepting text", () => {
  let prevented = false;
  const png = { name: "image.png", type: "image/png" };
  const files = pastedImageFiles({
    preventDefault: () => { prevented = true; },
    clipboardData: { items: [
      { kind: "string", type: "text/plain", getAsFile: () => null },
      { kind: "file", type: "image/png", getAsFile: () => png },
      { kind: "file", type: "image/svg+xml", getAsFile: () => ({}) },
    ] },
  });
  assert.deepEqual(files, [png]);
  assert.equal(prevented, true);
  assert.match(pastedImageName(png, 0, new Date("2026-09-08T03:04:05Z")), /^Screenshot-20260908T030405Z\.png$/);

  prevented = false;
  assert.deepEqual(pastedImageFiles({ preventDefault: () => { prevented = true; }, clipboardData: { items: [] } }), []);
  assert.equal(prevented, false);
});
