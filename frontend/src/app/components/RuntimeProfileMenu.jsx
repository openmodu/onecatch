import { ChevronDown, LoaderCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { codexEffortValues, codexServiceTierValues, selectedCodexModel } from "../codexRuntimeOptions.js";

const DEFAULT_VALUE = "__runtime_default__";
const HARNESS_LABELS = { codex: "Codex", claude: "Claude Code", modu: "Modu Code" };

function optionValue(value) {
  return value || DEFAULT_VALUE;
}

function storedValue(value) {
  return value === DEFAULT_VALUE ? "" : value;
}

function modelLabel(configuration, value, fallback) {
  const model = selectedCodexModel(configuration, value || fallback);
  return model?.displayName || model?.model || value || fallback || "Codex";
}

function effortLabel(t, effort) {
  return effort ? t(`settings.reasoningEffort.${effort}`, { defaultValue: effort }) : t("settings.runtimeDefault");
}

function speedLabel(t, tier) {
  return t(`settings.speed.${tier || "standard"}`, { defaultValue: tier || "standard" });
}

function RuntimeRow({ label, value }) {
  return <span className="runtime-profile-row-copy"><strong>{label}</strong><span>{value}</span></span>;
}

function ReadOnlyRow({ label, value }) {
  return <DropdownMenuItem className="runtime-profile-row read-only" onSelect={(event) => event.preventDefault()}><RuntimeRow label={label} value={value} /></DropdownMenuItem>;
}

export function resolvedRuntimeProfile(value = {}, configuration, runtimeSettings = {}) {
  const harness = value.harness || "codex";
  const configuredModel = value.model || runtimeSettings.defaultModel || configuration?.model || "";
  const selectedModel = selectedCodexModel(configuration, configuredModel);
  return {
    harness,
    model: value.model || configuredModel || selectedModel?.model || "",
    modelLabel: selectedModel?.displayName || selectedModel?.model || configuredModel || HARNESS_LABELS[harness] || harness,
    reasoningEffort: value.reasoningEffort || runtimeSettings.reasoningEffort || configuration?.reasoningEffort || selectedModel?.defaultReasoningEffort || "",
    serviceTier: value.serviceTier || runtimeSettings.serviceTier || configuration?.serviceTier || "standard",
  };
}

export default function RuntimeProfileMenu({
  value,
  onChange,
  configuration,
  runtimeSettings,
  loading = false,
  error = "",
  readOnly = false,
  className = "",
}) {
  const { t } = useTranslation();
  const profile = resolvedRuntimeProfile(value, configuration, runtimeSettings);
  const harnessLabel = HARNESS_LABELS[profile.harness] || profile.harness;
  const selectedModel = selectedCodexModel(configuration, value?.model || runtimeSettings?.defaultModel);
  const models = configuration?.models || [];
  const efforts = codexEffortValues(configuration, value?.model || runtimeSettings?.defaultModel, value?.reasoningEffort || runtimeSettings?.reasoningEffort);
  const tiers = codexServiceTierValues(configuration, value?.model || runtimeSettings?.defaultModel, value?.serviceTier || runtimeSettings?.serviceTier);
  const summary = [profile.modelLabel, effortLabel(t, profile.reasoningEffort), speedLabel(t, profile.serviceTier)].filter(Boolean).join(" · ");
  const update = (patch) => onChange?.((current) => ({ ...current, ...patch }));
  const selectModel = (nextValue) => {
    const model = storedValue(nextValue);
    const nextModel = selectedCodexModel(configuration, model || runtimeSettings?.defaultModel);
    const supportedEfforts = nextModel?.reasoningEfforts || [];
    const supportedTiers = ["standard", ...(nextModel?.serviceTiers || []).map((tier) => tier.id)];
    update({
      model,
      reasoningEffort: !value?.reasoningEffort || supportedEfforts.includes(value.reasoningEffort) ? value?.reasoningEffort || "" : nextModel?.defaultReasoningEffort || "",
      serviceTier: !value?.serviceTier || supportedTiers.includes(value.serviceTier) ? value?.serviceTier || "" : "standard",
    });
  };

  return <DropdownMenu>
    <DropdownMenuTrigger asChild>
      <Button type="button" variant="ghost" className={`runtime-profile-trigger ${className}`.trim()} aria-label={t("task.runtimeProfile")} title={t("task.runtimeProfile")}>
        {loading && <LoaderCircle className="animate-spin" size={13} aria-hidden="true" />}
        <span>{summary}</span>
        <ChevronDown size={14} aria-hidden="true" />
      </Button>
    </DropdownMenuTrigger>
    <DropdownMenuContent className="runtime-profile-menu" side="top" align="end" sideOffset={8}>
      {readOnly ? <>
        <ReadOnlyRow label={t("task.harness")} value={harnessLabel} />
        <ReadOnlyRow label={t("task.model")} value={profile.modelLabel} />
        <ReadOnlyRow label={t("settings.reasoningEffort")} value={effortLabel(t, profile.reasoningEffort)} />
        <ReadOnlyRow label={t("settings.speed")} value={speedLabel(t, profile.serviceTier)} />
      </> : <>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger className="runtime-profile-row"><RuntimeRow label={t("task.harness")} value="Codex" /></DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="runtime-profile-submenu" sideOffset={6}>
            <DropdownMenuRadioGroup value="codex" onValueChange={() => update({ harness: "codex" })}>
              <DropdownMenuRadioItem value="codex"><span className="runtime-profile-option"><strong>Codex</strong><small>{t("task.codexHarnessDescription")}</small></span></DropdownMenuRadioItem>
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || (!models.length && !profile.model)}><RuntimeRow label={t("task.model")} value={modelLabel(configuration, value?.model, runtimeSettings?.defaultModel)} /></DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="runtime-profile-submenu" sideOffset={6}>
            <DropdownMenuRadioGroup value={optionValue(value?.model)} onValueChange={selectModel}>
              <DropdownMenuRadioItem value={DEFAULT_VALUE}><span className="runtime-profile-option"><strong>{t("settings.useCodexConfig")}</strong><small>{modelLabel(configuration, "", runtimeSettings?.defaultModel)}</small></span></DropdownMenuRadioItem>
              {models.map((model) => <DropdownMenuRadioItem value={model.model || model.id} key={model.id || model.model}><span className="runtime-profile-option"><strong>{model.displayName || model.model || model.id}</strong>{model.description && <small>{model.description}</small>}</span></DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || !efforts.length}><RuntimeRow label={t("settings.reasoningEffort")} value={effortLabel(t, profile.reasoningEffort)} /></DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="runtime-profile-submenu compact" sideOffset={6}>
            <DropdownMenuRadioGroup value={optionValue(value?.reasoningEffort)} onValueChange={(reasoningEffort) => update({ reasoningEffort: storedValue(reasoningEffort) })}>
              <DropdownMenuRadioItem value={DEFAULT_VALUE}>{t("settings.runtimeDefault")}</DropdownMenuRadioItem>
              {efforts.map((effort) => <DropdownMenuRadioItem value={effort} key={effort}>{effortLabel(t, effort)}</DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || !tiers.length}><RuntimeRow label={t("settings.speed")} value={speedLabel(t, profile.serviceTier)} /></DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="runtime-profile-submenu compact" sideOffset={6}>
            <DropdownMenuRadioGroup value={optionValue(value?.serviceTier)} onValueChange={(serviceTier) => update({ serviceTier: storedValue(serviceTier) })}>
              <DropdownMenuRadioItem value={DEFAULT_VALUE}>{t("settings.runtimeDefault")}</DropdownMenuRadioItem>
              {tiers.map((tier) => <DropdownMenuRadioItem value={tier} key={tier}>{speedLabel(t, tier)}</DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
      </>}
      {(loading || error) && <><DropdownMenuSeparator /><div className={`runtime-profile-message ${error ? "error" : ""}`} role={error ? "alert" : "status"}>{loading ? t("settings.readingCodexConfig") : error}</div></>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
