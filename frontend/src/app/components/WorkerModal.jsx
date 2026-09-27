import { buildWorkerCommand, isVolcengineSandboxURL } from "../workerWorkspace.js";

import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { SettingsButton, SettingsField, SettingsSelect, SettingsSwitchRow } from "./settings/SettingsControls.jsx";

export default function WorkerModal({ form, setForm, busy, onClose, onUpdate, onPair }) {
  const { t } = useTranslation();
  const [returnFocus] = useState(() => document.activeElement);
  const [pairingCode, setPairingCode] = useState("");
  const creating = !form.id;
  const [connectionType, setConnectionType] = useState("sandbox");
  const sandbox = creating ? connectionType === "sandbox" : form.provider === "volcengine-sandbox";
  const submitting = busy === "worker-pair" || busy === "worker";
  const [remotePath, setRemotePath] = useState("/home/gem");
  const canPair = sandbox ? isVolcengineSandboxURL(form.baseUrl) && remotePath.trim().startsWith("/") : Boolean(form.baseUrl.trim() && pairingCode.trim());
  const update = (field, value) => setForm((current) => ({ ...current, [field]: value }));

  return <Dialog open onOpenChange={(open) => !open && onClose()}>
    <DialogContent className="flex max-h-[calc(100dvh-3rem)] flex-col gap-0 overflow-hidden p-0 sm:max-w-xl" showCloseButton={false} onCloseAutoFocus={(event) => { event.preventDefault(); returnFocus?.focus?.(); }}>
      <DialogHeader className="px-6 pt-6 pb-4">
        <DialogTitle>{t(creating ? "worker.connectTitle" : "worker.modalTitle")}</DialogTitle>
        <DialogDescription className="sr-only">{t("worker.connectTitle")}</DialogDescription>
      </DialogHeader>
      <form className="flex min-h-0 flex-col" onSubmit={(event) => { event.preventDefault(); if (submitting || (creating && !canPair)) return; if (creating) onPair(form.baseUrl, sandbox ? remotePath : pairingCode); else onUpdate(); }}>
        <div className="min-h-0 overflow-y-auto px-6">
          <div className="grid gap-4 pb-4">
            {creating && <div className="grid gap-5">
              <SettingsField label={t("worker.connectionType")}>
                <SettingsSelect ariaLabel={t("worker.connectionType")} value={connectionType} disabled={submitting} onChange={setConnectionType} options={[{ value: "sandbox", label: t("worker.volcengineSandbox") }, { value: "worker", label: t("worker.standardWorker") }]} />
              </SettingsField>
              <SettingsField label={t(sandbox ? "worker.sandboxUrl" : "worker.baseUrl")} hint={sandbox ? t("worker.sandboxUrlHint") : undefined}>
                <Input autoFocus value={form.baseUrl} disabled={submitting} onChange={(event) => { update("baseUrl", event.target.value); if (isVolcengineSandboxURL(event.target.value)) setConnectionType("sandbox"); }} placeholder={sandbox ? "https://…volceapi.com/?faasInstanceName=…&Authorization=…" : "https://192.168.1.20:9231"} autoComplete="off" spellCheck={false} />
              </SettingsField>
              {sandbox ? <SettingsField label={t("worker.sandboxPath")} hint={t("worker.sandboxPathHint")}><Input value={remotePath} disabled={submitting} onChange={(event) => setRemotePath(event.target.value)} placeholder="/home/gem" /></SettingsField> : <SettingsField label={t("worker.pairingCode")} hint={t("worker.pairingSectionDescription")}><Input value={pairingCode} disabled={submitting} onChange={(event) => setPairingCode(event.target.value.toUpperCase())} placeholder="ABCD-2345" /></SettingsField>}
            </div>}

            {!creating && <>
              <div>
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 [&_.wide]:col-span-full">
                  <SettingsField className="wide" label={t("worker.name")}><Input autoFocus value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="Build Mac mini" /></SettingsField>
                  <SettingsField className="wide" label={t("worker.baseUrl")}><Input value={form.baseUrl} onChange={(event) => update("baseUrl", event.target.value)} placeholder="https://192.168.1.20:9231" /></SettingsField>
                </div>
              </div>

              {!sandbox && <details><summary className="cursor-pointer text-sm text-muted-foreground">{t("worker.tlsSection")}</summary><div className="pt-3">
                <div className="grid grid-cols-1 gap-4 sm:grid-cols-2 [&_.wide]:col-span-full">
                  <SettingsField className="wide" label={t("worker.serverFingerprint")}><Input value={form.serverCertificateSha256} onChange={(event) => update("serverCertificateSha256", event.target.value)} placeholder={t("worker.serverFingerprintPlaceholder")} /></SettingsField>
                  <SettingsField label={t("worker.caFile")}><Input value={form.caFile} onChange={(event) => update("caFile", event.target.value)} placeholder="/path/to/ca.pem" /></SettingsField>
                  <SettingsField label={t("worker.serverName")}><Input value={form.serverName} onChange={(event) => update("serverName", event.target.value)} placeholder="worker.example.internal" /></SettingsField>
                  <SettingsField label={t("worker.clientCertFile")}><Input value={form.clientCertFile} onChange={(event) => update("clientCertFile", event.target.value)} placeholder="/path/to/client.pem" /></SettingsField>
                  <SettingsField label={t("worker.clientKeyFile")}><Input value={form.clientKeyFile} onChange={(event) => update("clientKeyFile", event.target.value)} placeholder="/path/to/client-key.pem" /></SettingsField>
                </div>
              </div></details>}

              {sandbox && <SettingsField label={t("worker.sandboxPath")}><Input value={form.remotePath || "/home/gem"} onChange={(event) => update("remotePath", event.target.value)} /></SettingsField>}

              <div>
                <SettingsSwitchRow checked={form.enabled} onChange={(enabled) => update("enabled", enabled)} label={t("worker.enableScheduling")} />
              </div>
            </>}
            {!sandbox && <details className="text-sm"><summary className="cursor-pointer text-muted-foreground">{t("worker.deploymentInstructions")}</summary><pre className="mt-3 overflow-x-auto rounded-lg bg-muted p-3 text-xs select-text"><code>{buildWorkerCommand({ workerID: form.id || "remote-worker" })}</code></pre></details>}
          </div>
        </div>
        <DialogFooter className="shrink-0 border-t border-border bg-muted/40 px-6 py-4">
          <SettingsButton tone="muted" onClick={onClose}>{t("common.cancel")}</SettingsButton>
          {creating && <SettingsButton type="submit" tone="primary" disabled={submitting || !canPair}>{t(submitting ? "worker.connecting" : sandbox ? "worker.sandboxConnect" : "worker.pair")}</SettingsButton>}
          {!creating && <SettingsButton type="submit" tone="primary" disabled={busy === "worker"}>{t("worker.saveChanges")}</SettingsButton>}
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
