import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Action, Field, Kicker, ToggleRow } from "../../ui/primitives.jsx";
import Modal from "./Modal.jsx";

function WorkerSection({ kicker, description, children }) {
  return <section className="worker-form-section">
    <header className="worker-form-section-head">
      <Kicker>{kicker}</Kicker>
      <p>{description}</p>
    </header>
    {children}
  </section>;
}

export default function WorkerModal({ form, setForm, busy, onClose, onUpdate, onPair }) {
  const { t } = useTranslation();
  const [pairingCode, setPairingCode] = useState("");
  const creating = !form.id;
  const update = (field, value) => setForm((current) => ({ ...current, [field]: value }));

  return <Modal className="worker-modal" title={t("worker.modalTitle")} subtitle={t("worker.modalSubtitle")} onClose={onClose}>
    <form className="worker-form" onSubmit={(event) => { event.preventDefault(); if (creating) onPair(form.baseUrl, pairingCode); else onUpdate(); }}>
      <div className="worker-form-scroll">
        {creating && <WorkerSection kicker={t("worker.pairingSection")} description={t("worker.pairingSectionDescription")}>
          <div className="worker-pairing-grid">
            <Field label={t("worker.baseUrl")}>
              <input autoFocus value={form.baseUrl} onChange={(event) => update("baseUrl", event.target.value)} placeholder="https://192.168.1.20:9231" />
            </Field>
            <Field label={t("worker.pairingCode")}>
              <input value={pairingCode} onChange={(event) => setPairingCode(event.target.value.toUpperCase())} placeholder="ABCD-2345" />
            </Field>
            <Action type="submit" tone="primary" disabled={busy === "worker-pair" || !form.baseUrl.trim() || !pairingCode.trim()}>
              {busy === "worker-pair" ? t("worker.pairing") : t("worker.pair")}
            </Action>
          </div>
        </WorkerSection>}

        {!creating && <><WorkerSection kicker={t("worker.connectionSection")} description={t("worker.connectionSectionDescription")}>
          <div className="worker-form-grid">
            <Field label={t("worker.id")}>
              <input autoFocus value={form.id} disabled />
            </Field>
            <Field label={t("worker.name")}>
              <input value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="Build Mac mini" />
            </Field>
            <Field className="worker-form-wide" label={t("worker.baseUrl")}>
              <input value={form.baseUrl} onChange={(event) => update("baseUrl", event.target.value)} placeholder="https://192.168.1.20:9231" />
            </Field>
          </div>
        </WorkerSection>

        <WorkerSection kicker={t("worker.tlsSection")} description={t("worker.tlsSectionDescription")}>
          <div className="worker-form-grid">
            <Field className="worker-form-wide" label={t("worker.serverFingerprint")}>
              <input value={form.serverCertificateSha256} onChange={(event) => update("serverCertificateSha256", event.target.value)} placeholder={t("worker.serverFingerprintPlaceholder")} />
            </Field>
            <Field label={t("worker.caFile")}>
              <input value={form.caFile} onChange={(event) => update("caFile", event.target.value)} placeholder="/path/to/ca.pem" />
            </Field>
            <Field label={t("worker.serverName")}>
              <input value={form.serverName} onChange={(event) => update("serverName", event.target.value)} placeholder="worker.example.internal" />
            </Field>
            <Field label={t("worker.clientCertFile")}>
              <input value={form.clientCertFile} onChange={(event) => update("clientCertFile", event.target.value)} placeholder="/path/to/client.pem" />
            </Field>
            <Field label={t("worker.clientKeyFile")}>
              <input value={form.clientKeyFile} onChange={(event) => update("clientKeyFile", event.target.value)} placeholder="/path/to/client-key.pem" />
            </Field>
          </div>
        </WorkerSection>

        <WorkerSection kicker={t("worker.schedulingSection")} description={t("worker.schedulingSectionDescription")}>
          <ToggleRow checked={form.enabled} onChange={(enabled) => update("enabled", enabled)} label={t("worker.enableScheduling")} description={t("worker.enableSchedulingDescription")} />
        </WorkerSection></>}
      </div>

      <div className="modal-actions worker-form-actions">
        <Action tone="muted" onClick={onClose}>{t("common.cancel")}</Action>
        {!creating && <Action type="submit" tone="primary" disabled={busy === "worker"}>{t("worker.saveChanges")}</Action>}
      </div>
    </form>
  </Modal>;
}
