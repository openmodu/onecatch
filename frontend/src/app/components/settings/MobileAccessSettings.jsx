import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, Clipboard as ClipboardIcon, Smartphone } from "lucide-react";
import { Clipboard } from "@wailsio/runtime";
import { WorkerBinding } from "../../../../bindings/github.com/openmodu/onecatch/internal/transport/wails/index.js";
import { Badge } from "@/components/ui/badge";
import { SettingsButton, SettingsKicker, SettingsSection, SettingsSwitchRow } from "./SettingsControls.jsx";
import { errorMessage } from "../../format.js";
import { demoHostedWorker, pairingCountdown } from "../../mobileAccess.js";

async function copyText(value) {
  try {
    await Clipboard.SetText(value);
  } catch (wailsError) {
    if (!navigator.clipboard?.writeText) throw wailsError;
    await navigator.clipboard.writeText(value);
  }
}

function CopyRow({ value, label }) {
  const [copied, setCopied] = useState(false);
  useEffect(() => {
    if (!copied) return undefined;
    const timer = window.setTimeout(() => setCopied(false), 1600);
    return () => window.clearTimeout(timer);
  }, [copied]);
  return <div className="flex items-center gap-2 rounded-lg bg-background/75 px-3 py-2">
    <code className="min-w-0 flex-1 truncate select-text text-xs text-foreground">{value}</code>
    <SettingsButton tone="ghost" compact aria-label={label} onClick={() => { void copyText(value).then(() => setCopied(true)); }}>
      {copied ? <Check /> : <ClipboardIcon />}
    </SettingsButton>
  </div>;
}

export default function MobileAccessSettings({ mode, notify }) {
  const { t } = useTranslation();
  const [status, setStatus] = useState(mode === "demo" ? demoHostedWorker : null);
  const [busy, setBusy] = useState("");
  const [tick, setTick] = useState(0);

  const load = useCallback(async () => {
    if (mode === "demo") return;
    try {
      setStatus(await WorkerBinding.HostedWorker());
    } catch (error) {
      notify("error", errorMessage(error));
    }
  }, [mode, notify]);
  useEffect(() => { void load(); }, [load]);

  // A code is only usable while it lasts, so the panel counts it down rather
  // than leaving a dead code on screen.
  const pairing = status?.pairing;
  useEffect(() => {
    if (!pairing) return undefined;
    const timer = window.setInterval(() => setTick((value) => value + 1), 1000);
    return () => window.clearInterval(timer);
  }, [pairing?.code]);
  const countdown = useMemo(() => pairing ? pairingCountdown(pairing.expiresAt) : "", [pairing, tick]);

  const run = async (action, key) => {
    if (mode === "demo") return;
    setBusy(key);
    try {
      setStatus(await action());
    } catch (error) {
      notify("error", errorMessage(error));
      void load();
    } finally {
      setBusy("");
    }
  };
  const toggle = (next) => run(() => next ? WorkerBinding.StartHostedWorker() : WorkerBinding.StopHostedWorker(), "toggle");
  const pair = () => run(() => WorkerBinding.PairHostedWorker(), "pair");

  const running = Boolean(status?.running);
  const addresses = status?.addresses || [];
  return <>
    <SettingsSection title={t("settings.mobileAccessTitle")} description={t("settings.mobileAccessDescription")}
      aside={running ? <Badge variant="outline" className="gap-1.5"><Smartphone className="size-3" />{t("settings.mobileAccessOn")}</Badge> : null}>
      <SettingsSwitchRow
        checked={running}
        disabled={Boolean(busy) || mode === "demo"}
        onChange={toggle}
        label={t("settings.mobileAccessToggle")}
        description={t("settings.mobileAccessToggleHint", { port: status?.port || 9232 })}
      />
    </SettingsSection>

    {running && <SettingsSection title={t("settings.mobileAccessPairTitle")} description={t("settings.mobileAccessPairDescription")}>
      <div className="grid gap-3">
        <div className="grid gap-1.5">
          <SettingsKicker>{t("settings.mobileAccessAddress")}</SettingsKicker>
          {addresses.length
            ? addresses.map((address) => <CopyRow key={address} value={address} label={t("settings.mobileAccessCopyAddress")} />)
            : <p className="m-0 text-xs text-muted-foreground">{t("settings.mobileAccessNoAddress")}</p>}
        </div>
        <div className="grid gap-1.5">
          <SettingsKicker>{t("settings.mobileAccessCode")}</SettingsKicker>
          {pairing && countdown
            ? <div className="flex items-center gap-3 rounded-lg bg-muted/45 px-3 py-2.5">
                <code className="flex-1 select-text font-mono text-xl font-semibold tracking-[0.18em] text-foreground">{pairing.code}</code>
                <span className="text-xs tabular-nums text-muted-foreground">{t("settings.mobileAccessExpiresIn", { countdown })}</span>
                <SettingsButton tone="ghost" compact aria-label={t("settings.mobileAccessCopyCode")} onClick={() => { void copyText(pairing.code); }}><ClipboardIcon /></SettingsButton>
              </div>
            : <p className="m-0 text-xs text-muted-foreground">{t("settings.mobileAccessCodeHint")}</p>}
          <div><SettingsButton disabled={busy === "pair"} onClick={pair}>{pairing && countdown ? t("settings.mobileAccessNewCode") : t("settings.mobileAccessCreateCode")}</SettingsButton></div>
        </div>
        {status?.fingerprint && <p className="m-0 text-xs leading-relaxed text-muted-foreground">
          {t("settings.mobileAccessFingerprint", { fingerprint: status.fingerprint.slice(0, 16).toUpperCase() })}
        </p>}
      </div>
    </SettingsSection>}
  </>;
}
