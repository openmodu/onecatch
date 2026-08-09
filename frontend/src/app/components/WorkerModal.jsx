import { useState } from "react";
import { useTranslation } from "react-i18next";

import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { ScrollArea } from "@/components/ui/scroll-area";
import { SettingsButton, SettingsField, SettingsSwitchRow } from "./settings/SettingsControls.jsx";

function WorkerSection({ title, description, children }) {
  return <Card className="gap-4 py-4 shadow-none">
    <CardHeader className="gap-1 px-4">
      <CardTitle className="text-sm">{title}</CardTitle>
      <CardDescription className="text-xs leading-relaxed">{description}</CardDescription>
    </CardHeader>
    <CardContent className="px-4">{children}</CardContent>
  </Card>;
}

export default function WorkerModal({ form, setForm, busy, onClose, onUpdate, onPair }) {
  const { t } = useTranslation();
  const [pairingCode, setPairingCode] = useState("");
  const creating = !form.id;
  const update = (field, value) => setForm((current) => ({ ...current, [field]: value }));

  return <Dialog open onOpenChange={(open) => !open && onClose()}>
    <DialogContent className="max-h-[calc(100vh-3rem)] gap-0 overflow-hidden p-0 sm:max-w-2xl" showCloseButton={false}>
      <DialogHeader className="px-6 pt-6 pb-4">
        <DialogTitle>{t("worker.modalTitle")}</DialogTitle>
        <DialogDescription>{t("worker.modalSubtitle")}</DialogDescription>
      </DialogHeader>
      <form className="min-h-0" onSubmit={(event) => { event.preventDefault(); if (creating) onPair(form.baseUrl, pairingCode); else onUpdate(); }}>
        <ScrollArea className="h-[min(62vh,620px)] px-6">
          <div className="grid gap-4 pb-4">
            {creating && <WorkerSection title={t("worker.pairingSection")} description={t("worker.pairingSectionDescription")}>
              <div className="grid grid-cols-2 items-end gap-4">
                <SettingsField label={t("worker.baseUrl")}><Input autoFocus value={form.baseUrl} onChange={(event) => update("baseUrl", event.target.value)} placeholder="https://192.168.1.20:9231" /></SettingsField>
                <SettingsField label={t("worker.pairingCode")}><Input value={pairingCode} onChange={(event) => setPairingCode(event.target.value.toUpperCase())} placeholder="ABCD-2345" /></SettingsField>
                <SettingsButton className="col-span-full justify-self-end" type="submit" tone="primary" disabled={busy === "worker-pair" || !form.baseUrl.trim() || !pairingCode.trim()}>{busy === "worker-pair" ? t("worker.pairing") : t("worker.pair")}</SettingsButton>
              </div>
            </WorkerSection>}

            {!creating && <>
              <WorkerSection title={t("worker.connectionSection")} description={t("worker.connectionSectionDescription")}>
                <div className="grid grid-cols-2 gap-4 [&_.wide]:col-span-full">
                  <SettingsField label={t("worker.id")}><Input autoFocus value={form.id} disabled /></SettingsField>
                  <SettingsField label={t("worker.name")}><Input value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="Build Mac mini" /></SettingsField>
                  <SettingsField className="wide" label={t("worker.baseUrl")}><Input value={form.baseUrl} onChange={(event) => update("baseUrl", event.target.value)} placeholder="https://192.168.1.20:9231" /></SettingsField>
                </div>
              </WorkerSection>

              <WorkerSection title={t("worker.tlsSection")} description={t("worker.tlsSectionDescription")}>
                <div className="grid grid-cols-2 gap-4 [&_.wide]:col-span-full">
                  <SettingsField className="wide" label={t("worker.serverFingerprint")}><Input value={form.serverCertificateSha256} onChange={(event) => update("serverCertificateSha256", event.target.value)} placeholder={t("worker.serverFingerprintPlaceholder")} /></SettingsField>
                  <SettingsField label={t("worker.caFile")}><Input value={form.caFile} onChange={(event) => update("caFile", event.target.value)} placeholder="/path/to/ca.pem" /></SettingsField>
                  <SettingsField label={t("worker.serverName")}><Input value={form.serverName} onChange={(event) => update("serverName", event.target.value)} placeholder="worker.example.internal" /></SettingsField>
                  <SettingsField label={t("worker.clientCertFile")}><Input value={form.clientCertFile} onChange={(event) => update("clientCertFile", event.target.value)} placeholder="/path/to/client.pem" /></SettingsField>
                  <SettingsField label={t("worker.clientKeyFile")}><Input value={form.clientKeyFile} onChange={(event) => update("clientKeyFile", event.target.value)} placeholder="/path/to/client-key.pem" /></SettingsField>
                </div>
              </WorkerSection>

              <WorkerSection title={t("worker.schedulingSection")} description={t("worker.schedulingSectionDescription")}>
                <SettingsSwitchRow checked={form.enabled} onChange={(enabled) => update("enabled", enabled)} label={t("worker.enableScheduling")} description={t("worker.enableSchedulingDescription")} />
              </WorkerSection>
            </>}
          </div>
        </ScrollArea>
        <DialogFooter className="bg-muted/40 px-6 py-4">
          <SettingsButton tone="muted" onClick={onClose}>{t("common.cancel")}</SettingsButton>
          {!creating && <SettingsButton type="submit" tone="primary" disabled={busy === "worker"}>{t("worker.saveChanges")}</SettingsButton>}
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
