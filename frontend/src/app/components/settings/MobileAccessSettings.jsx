import { useCallback, useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { Check, Clipboard as ClipboardIcon, Fingerprint, RefreshCw, Smartphone } from "lucide-react";
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
  return <div className="flex min-h-10 items-center gap-3 px-3 py-2.5">
    <code className="min-w-0 flex-1 truncate select-text text-[13px] text-foreground">{value}</code>
    <SettingsButton tone="ghost" compact className="size-7 p-0 text-muted-foreground" aria-label={label} onClick={() => { void copyText(value).then(() => setCopied(true)); }}>
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

    {running && <SettingsSection title={t("settings.mobileAccessPairTitle")} description={t("settings.mobileAccessPairDescription")} contentClassName="p-4">
      <div className="grid gap-5">
        <div className="grid gap-2">
          <SettingsKicker>{t("settings.mobileAccessAddress")}</SettingsKicker>
          {addresses.length ? <div className="divide-y divide-border/65 overflow-hidden rounded-lg border border-border/70 bg-background/55">
            {addresses.map((address) => <CopyRow key={address} value={address} label={t("settings.mobileAccessCopyAddress")} />)}
          </div> : <p className="m-0 text-xs text-muted-foreground">{t("settings.mobileAccessNoAddress")}</p>}
        </div>
        <div className="grid gap-2">
          <div className="flex items-center justify-between gap-3">
            <SettingsKicker>{t("settings.mobileAccessCode")}</SettingsKicker>
            <SettingsButton tone="muted" compact disabled={busy === "pair"} onClick={pair}>
              <RefreshCw className={busy === "pair" ? "animate-spin" : ""} />
              {pairing && countdown ? t("settings.mobileAccessNewCode") : t("settings.mobileAccessCreateCode")}
            </SettingsButton>
          </div>
          {pairing && countdown
            ? <div className="flex min-h-14 items-center gap-3 rounded-lg border border-border/70 bg-muted/30 px-3.5 py-3">
                <code className="flex-1 select-text font-mono text-xl font-semibold tracking-[0.18em] text-foreground">{pairing.code}</code>
                <span className="text-xs tabular-nums text-muted-foreground">{t("settings.mobileAccessExpiresIn", { countdown })}</span>
                <SettingsButton tone="ghost" compact className="size-7 p-0 text-muted-foreground" aria-label={t("settings.mobileAccessCopyCode")} onClick={() => { void copyText(pairing.code); }}><ClipboardIcon /></SettingsButton>
              </div>
            : <p className="m-0 text-xs text-muted-foreground">{t("settings.mobileAccessCodeHint")}</p>}
        </div>
        {status?.fingerprint && <div className="flex items-center gap-2 border-t border-border/60 pt-3 text-xs leading-relaxed text-muted-foreground">
          <Fingerprint className="size-3.5 shrink-0" />
          <span>{t("settings.mobileAccessFingerprint", { fingerprint: status.fingerprint.slice(0, 16).toUpperCase() })}</span>
        </div>}
      </div>
    </SettingsSection>}
  </>;
}
