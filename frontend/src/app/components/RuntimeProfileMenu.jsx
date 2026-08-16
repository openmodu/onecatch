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
import { runtimeHarness } from "../runtimeHarnesses.js";

const DEFAULT_VALUE = "__runtime_default__";

const unique = (values) => [...new Set(values.filter(Boolean))];

function optionValue(value) {
  return value || DEFAULT_VALUE;
}

function storedValue(value) {
  return value === DEFAULT_VALUE ? "" : value;
}

function modelLabel(configuration, value, fallback) {
  const model = selectedCodexModel(configuration, value || fallback);
  return model?.displayName || model?.model || value || fallback || "";
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
  const capability = runtimeHarness(harness);
  const configuredModel = value.model || runtimeSettings.defaultModel || configuration?.model || "";
  const selectedModel = selectedCodexModel(configuration, configuredModel);
  return {
    harness,
    model: value.model || configuredModel || selectedModel?.model || "",
    modelLabel: selectedModel?.displayName || selectedModel?.model || configuredModel || capability.label,
    reasoningEffort: capability.supportsReasoning ? value.reasoningEffort || runtimeSettings.reasoningEffort || configuration?.reasoningEffort || selectedModel?.defaultReasoningEffort || "" : "",
    serviceTier: capability.supportsSpeed ? value.serviceTier || runtimeSettings.serviceTier || configuration?.serviceTier || "standard" : "",
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
  const capability = runtimeHarness(profile.harness);
  const models = configuration?.models || [];
  const efforts = profile.harness === "codex"
    ? codexEffortValues(configuration, value?.model || runtimeSettings?.defaultModel, value?.reasoningEffort || runtimeSettings?.reasoningEffort)
    : capability.supportsReasoning ? unique([...(configuration?.efforts || []), runtimeSettings?.reasoningEffort, value?.reasoningEffort]) : [];
  const tiers = capability.supportsSpeed ? codexServiceTierValues(configuration, value?.model || runtimeSettings?.defaultModel, value?.serviceTier || runtimeSettings?.serviceTier) : [];
  const summary = [
    profile.modelLabel,
    capability.supportsReasoning ? effortLabel(t, profile.reasoningEffort) : "",
    capability.supportsSpeed ? speedLabel(t, profile.serviceTier) : "",
  ].filter(Boolean).join(" · ");
  const inheritedModelLabel = profile.harness === "codex" ? t("settings.useCodexConfig") : profile.harness === "claude" ? t("settings.useClaudeConfig") : t("settings.runtimeDefault");
  const loadingLabel = profile.harness === "claude" ? t("settings.readingClaudeModels") : t("settings.readingCodexConfig");
  const update = (patch) => onChange?.((current) => ({ ...current, ...patch }));
  const selectModel = (nextValue) => {
    const model = storedValue(nextValue);
    const nextModel = selectedCodexModel(configuration, model || runtimeSettings?.defaultModel);
    const supportedEfforts = profile.harness === "codex" ? nextModel?.reasoningEfforts || [] : configuration?.efforts || [];
    const supportedTiers = capability.supportsSpeed ? ["standard", ...(nextModel?.serviceTiers || []).map((tier) => tier.id)] : [];
    update({
      model,
      reasoningEffort: capability.supportsReasoning && (!value?.reasoningEffort || supportedEfforts.includes(value.reasoningEffort)) ? value?.reasoningEffort || "" : capability.supportsReasoning ? nextModel?.defaultReasoningEffort || "" : "",
      serviceTier: capability.supportsSpeed && (!value?.serviceTier || supportedTiers.includes(value.serviceTier)) ? value?.serviceTier || "" : capability.supportsSpeed ? "standard" : "",
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
        <ReadOnlyRow label={t("task.model")} value={profile.modelLabel} />
        {capability.supportsReasoning && <ReadOnlyRow label={t("settings.reasoningEffort")} value={effortLabel(t, profile.reasoningEffort)} />}
        {capability.supportsSpeed && <ReadOnlyRow label={t("settings.speed")} value={speedLabel(t, profile.serviceTier)} />}
      </> : <>
        <DropdownMenuSub>
          <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || (!models.length && !profile.model)}><RuntimeRow label={t("task.model")} value={modelLabel(configuration, value?.model, runtimeSettings?.defaultModel) || profile.modelLabel} /></DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="runtime-profile-submenu" sideOffset={6}>
            <DropdownMenuRadioGroup value={optionValue(value?.model)} onValueChange={selectModel}>
              <DropdownMenuRadioItem value={DEFAULT_VALUE}><span className="runtime-profile-option"><strong>{inheritedModelLabel}</strong><small>{modelLabel(configuration, "", runtimeSettings?.defaultModel) || capability.label}</small></span></DropdownMenuRadioItem>
              {models.map((model) => <DropdownMenuRadioItem value={model.model || model.id} key={model.id || model.model}><span className="runtime-profile-option"><strong>{model.displayName || model.model || model.id}</strong>{model.description && <small>{model.description}</small>}</span></DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>
        {capability.supportsReasoning && <DropdownMenuSub>
          <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || !efforts.length}><RuntimeRow label={t("settings.reasoningEffort")} value={effortLabel(t, profile.reasoningEffort)} /></DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="runtime-profile-submenu compact" sideOffset={6}>
            <DropdownMenuRadioGroup value={optionValue(value?.reasoningEffort)} onValueChange={(reasoningEffort) => update({ reasoningEffort: storedValue(reasoningEffort) })}>
              <DropdownMenuRadioItem value={DEFAULT_VALUE}>{t("settings.runtimeDefault")}</DropdownMenuRadioItem>
              {efforts.map((effort) => <DropdownMenuRadioItem value={effort} key={effort}>{effortLabel(t, effort)}</DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>}
        {capability.supportsSpeed && <DropdownMenuSub>
          <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || !tiers.length}><RuntimeRow label={t("settings.speed")} value={speedLabel(t, profile.serviceTier)} /></DropdownMenuSubTrigger>
          <DropdownMenuSubContent className="runtime-profile-submenu compact" sideOffset={6}>
            <DropdownMenuRadioGroup value={optionValue(value?.serviceTier)} onValueChange={(serviceTier) => update({ serviceTier: storedValue(serviceTier) })}>
              <DropdownMenuRadioItem value={DEFAULT_VALUE}>{t("settings.runtimeDefault")}</DropdownMenuRadioItem>
              {tiers.map((tier) => <DropdownMenuRadioItem value={tier} key={tier}>{speedLabel(t, tier)}</DropdownMenuRadioItem>)}
            </DropdownMenuRadioGroup>
          </DropdownMenuSubContent>
        </DropdownMenuSub>}
      </>}
      {(loading || error) && <><DropdownMenuSeparator /><div className={`runtime-profile-message ${error ? "error" : ""}`} role={error ? "alert" : "status"}>{loading ? loadingLabel : error}</div></>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
