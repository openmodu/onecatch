import { Check, ChevronDown, LoaderCircle } from "lucide-react";
import { useTranslation } from "react-i18next";
import { Button } from "@/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import {
  claudeModelDisplayLabel,
  codexServiceTierValues,
  defaultClaudeModel,
  groupedClaudeModels,
  runtimeDefaultEffort,
  runtimeEffortValues,
  selectedCodexModel,
} from "../codexRuntimeOptions.js";
import { runtimeHarness } from "../runtimeHarnesses.js";

const DEFAULT_VALUE = "__runtime_default__";

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

function reasoningDescription(t, effort) {
  return t(`settings.reasoningEffort.${effort}.description`, { defaultValue: "" });
}

function speedDescription(t, tier, model) {
  const localized = t(`settings.speed.${tier || "standard"}.description`, { defaultValue: "" });
  if (localized) return localized;
  return model?.serviceTiers?.find((option) => option.id === tier)?.description || "";
}

export function compactRuntimeModelLabel(label = "") {
  return String(label).replace(/^gpt-/i, "").replaceAll("-", " ");
}

function RuntimeRow({ label, value }) {
  return <span className="runtime-profile-row-copy"><strong>{label}</strong><span>{value}</span></span>;
}

function RuntimeSubmenuOption({ value, label, description = "", selected = false }) {
  return <DropdownMenuRadioItem value={value} className="runtime-profile-submenu-option">
    <span className="runtime-profile-submenu-option-copy">
      <strong>{label}</strong>
      {description && <small>{description}</small>}
    </span>
    {selected && <Check className="runtime-profile-submenu-check" size={17} strokeWidth={2} aria-hidden="true" />}
  </DropdownMenuRadioItem>;
}

function ClaudeModelOption({ model, selected = false, isDefault = false }) {
  const value = model.model || model.id;
  return <DropdownMenuRadioItem value={value} className="runtime-profile-submenu-option claude-model-option">
    <span className="claude-model-option-copy">
      <strong>{claudeModelDisplayLabel(model)}</strong>
      {isDefault && <small>{"Default"}</small>}
    </span>
    {selected && <Check className="runtime-profile-submenu-check" size={17} strokeWidth={2} aria-hidden="true" />}
  </DropdownMenuRadioItem>;
}

function ReadOnlyRow({ label, value }) {
  return <DropdownMenuItem className="runtime-profile-row read-only" onSelect={(event) => event.preventDefault()}><RuntimeRow label={label} value={value} /></DropdownMenuItem>;
}

export function resolvedRuntimeProfile(value = {}, configuration, runtimeSettings = {}) {
  const harness = value.harness || "codex";
  const capability = runtimeHarness(harness);
  const configuredModel = value.model || runtimeSettings.defaultModel || configuration?.model || "";
  const models = configuration?.models || [];
  const selectedModel = harness === "claude"
    ? models.find((model) => (model.model || model.id) === defaultClaudeModel(models, configuredModel)) || null
    : selectedCodexModel(configuration, configuredModel);
  const resolvedModel = configuredModel || selectedModel?.model || selectedModel?.id || "";
  return {
    harness,
    model: value.model || resolvedModel,
    modelLabel: harness === "claude"
      ? claudeModelDisplayLabel(selectedModel || { model: resolvedModel, displayName: resolvedModel }) || capability.label
      : selectedModel?.displayName || selectedModel?.model || configuredModel || capability.label,
    reasoningEffort: capability.supportsReasoning ? value.reasoningEffort || runtimeSettings.reasoningEffort || runtimeDefaultEffort(configuration, configuredModel) : "",
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
  const selectedModel = selectedCodexModel(configuration, value?.model || runtimeSettings?.defaultModel);
  const claudeModels = groupedClaudeModels(models);
  const efforts = capability.supportsReasoning
    ? runtimeEffortValues(configuration, value?.model || runtimeSettings?.defaultModel, value?.reasoningEffort || runtimeSettings?.reasoningEffort)
    : [];
  const tiers = capability.supportsSpeed ? codexServiceTierValues(configuration, value?.model || runtimeSettings?.defaultModel, value?.serviceTier || runtimeSettings?.serviceTier) : [];
  const displayModelLabel = profile.harness === "codex" ? compactRuntimeModelLabel(profile.modelLabel) : profile.modelLabel;
  const selectedModelLabel = modelLabel(configuration, value?.model, runtimeSettings?.defaultModel) || profile.modelLabel;
  const displaySelectedModelLabel = profile.harness === "codex" ? compactRuntimeModelLabel(selectedModelLabel) : selectedModelLabel;
  const summary = profile.harness === "claude" ? displayModelLabel : [
    displayModelLabel,
    capability.supportsReasoning ? effortLabel(t, profile.reasoningEffort) : "",
  ].filter(Boolean).join(" ");
  const inheritedModelLabel = profile.harness === "codex" ? t("settings.useCodexConfig") : t("settings.runtimeDefault");
  const loadingLabel = profile.harness === "claude"
    ? t("settings.readingClaudeModels")
    : profile.harness === "codex"
      ? t("settings.readingCodexConfig")
      : t("settings.readingHarnessConfig", { harness: capability.label });
  const inheritedReasoningEffort = runtimeSettings?.reasoningEffort || runtimeDefaultEffort(configuration, value?.model || runtimeSettings?.defaultModel);
  const inheritedServiceTier = runtimeSettings?.serviceTier || configuration?.serviceTier || "standard";
  const inheritedClaudeModel = defaultClaudeModel(models, runtimeSettings?.defaultModel || configuration?.model || "");
  const claudeModelValue = profile.model || inheritedClaudeModel;
  const reasoningMenuValue = profile.reasoningEffort || inheritedReasoningEffort;
  const update = (patch) => onChange?.((current) => ({ ...current, ...patch }));
  const selectClaudeModel = (model) => update({ model: model === inheritedClaudeModel ? "" : model });
  const selectReasoningEffort = (reasoningEffort) => update({ reasoningEffort: reasoningEffort === inheritedReasoningEffort ? "" : reasoningEffort });
  const selectModel = (nextValue) => {
    const model = storedValue(nextValue);
    const nextModel = selectedCodexModel(configuration, model || runtimeSettings?.defaultModel);
    const supportedEfforts = runtimeEffortValues(configuration, model || runtimeSettings?.defaultModel);
    const supportedTiers = capability.supportsSpeed ? ["standard", ...(nextModel?.serviceTiers || []).map((tier) => tier.id)] : [];
    update({
      model,
      reasoningEffort: capability.supportsReasoning && (!value?.reasoningEffort || supportedEfforts.includes(value.reasoningEffort)) ? value?.reasoningEffort || "" : capability.supportsReasoning ? nextModel?.defaultReasoningEffort || nextModel?.defaultEffort || "" : "",
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
    <DropdownMenuContent className={`runtime-profile-menu ${profile.harness === "claude" ? "claude-profile-menu" : ""}`.trim()} side="top" align="end" sideOffset={8}>
      {profile.harness === "claude" ? <>
        <DropdownMenuLabel className="runtime-profile-submenu-heading">{t("task.models")}</DropdownMenuLabel>
        {readOnly
          ? <ReadOnlyRow label={t("task.model")} value={displayModelLabel} />
          : <DropdownMenuRadioGroup value={claudeModelValue} onValueChange={selectClaudeModel}>
            {claudeModels.primary.map((model) => <ClaudeModelOption
              model={model}
              selected={(model.model || model.id) === claudeModelValue}
              isDefault={(model.model || model.id) === inheritedClaudeModel}
              key={model.id || model.model}
            />)}
            {claudeModels.more.length > 0 && <>
              <DropdownMenuSeparator />
              <DropdownMenuSub>
                <DropdownMenuSubTrigger className="claude-more-models-trigger">{t("task.moreModels")}</DropdownMenuSubTrigger>
                <DropdownMenuSubContent className="runtime-profile-submenu claude-more-models" sideOffset={6}>
                  <DropdownMenuRadioGroup value={claudeModelValue} onValueChange={selectClaudeModel}>
                    {claudeModels.more.map((model) => <ClaudeModelOption
                      model={model}
                      selected={(model.model || model.id) === claudeModelValue}
                      key={model.id || model.model}
                    />)}
                  </DropdownMenuRadioGroup>
                </DropdownMenuSubContent>
              </DropdownMenuSub>
            </>}
          </DropdownMenuRadioGroup>}
        {/* The Claude branch rendered models only, so --effort was reachable
            from Settings but not per run — even though the adapter has always
            passed it and the CLI advertises every level. */}
        {capability.supportsReasoning && <>
          <DropdownMenuSeparator />
          {readOnly
            ? <ReadOnlyRow label={t("settings.reasoningEffort")} value={effortLabel(t, profile.reasoningEffort)} />
            : <DropdownMenuSub>
              <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || !efforts.length}><RuntimeRow label={t("settings.reasoningEffort")} value={effortLabel(t, profile.reasoningEffort)} /></DropdownMenuSubTrigger>
              <DropdownMenuSubContent className="runtime-profile-submenu compact" sideOffset={6}>
                <DropdownMenuLabel className="runtime-profile-submenu-heading">{t("settings.reasoningEffort")}</DropdownMenuLabel>
                <DropdownMenuRadioGroup value={reasoningMenuValue} onValueChange={selectReasoningEffort}>
                  {efforts.map((effort) => <RuntimeSubmenuOption
                    value={effort}
                    label={effortLabel(t, effort)}
                    description={reasoningDescription(t, effort)}
                    selected={effort === reasoningMenuValue}
                    key={effort}
                  />)}
                </DropdownMenuRadioGroup>
              </DropdownMenuSubContent>
            </DropdownMenuSub>}
        </>}
        {(loading || error) && <><DropdownMenuSeparator /><div className={`runtime-profile-message ${error ? "error" : ""}`} role={error ? "alert" : "status"}>{loading ? loadingLabel : error}</div></>}
      </> : <>
        {readOnly ? <>
          <ReadOnlyRow label={t("task.model")} value={displayModelLabel} />
          {capability.supportsReasoning && <ReadOnlyRow label={t("settings.reasoningEffort")} value={effortLabel(t, profile.reasoningEffort)} />}
          {capability.supportsSpeed && <ReadOnlyRow label={t("settings.speed")} value={speedLabel(t, profile.serviceTier)} />}
        </> : <>
          <DropdownMenuSub>
            <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || (!models.length && !profile.model)}><RuntimeRow label={t("task.model")} value={displaySelectedModelLabel} /></DropdownMenuSubTrigger>
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
              <DropdownMenuLabel className="runtime-profile-submenu-heading">{t("settings.reasoningEffort")}</DropdownMenuLabel>
              <DropdownMenuRadioGroup value={reasoningMenuValue} onValueChange={selectReasoningEffort}>
                {efforts.map((effort) => <RuntimeSubmenuOption
                  value={effort}
                  label={effortLabel(t, effort)}
                  description={reasoningDescription(t, effort)}
                  selected={effort === reasoningMenuValue}
                  key={effort}
                />)}
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>}
          {capability.supportsSpeed && <DropdownMenuSub>
            <DropdownMenuSubTrigger className="runtime-profile-row" disabled={loading || !tiers.length}><RuntimeRow label={t("settings.speed")} value={speedLabel(t, profile.serviceTier)} /></DropdownMenuSubTrigger>
            <DropdownMenuSubContent className="runtime-profile-submenu compact" sideOffset={6}>
              <DropdownMenuLabel className="runtime-profile-submenu-heading">{t("settings.speed")}</DropdownMenuLabel>
              <DropdownMenuRadioGroup value={profile.serviceTier || inheritedServiceTier} onValueChange={(serviceTier) => update({ serviceTier: serviceTier === inheritedServiceTier ? "" : serviceTier })}>
                {tiers.map((tier) => <RuntimeSubmenuOption
                  value={tier}
                  label={speedLabel(t, tier)}
                  description={speedDescription(t, tier, selectedModel)}
                  selected={tier === (profile.serviceTier || inheritedServiceTier)}
                  key={tier}
                />)}
              </DropdownMenuRadioGroup>
            </DropdownMenuSubContent>
          </DropdownMenuSub>}
        </>}
        {(loading || error) && <><DropdownMenuSeparator /><div className={`runtime-profile-message ${error ? "error" : ""}`} role={error ? "alert" : "status"}>{loading ? loadingLabel : error}</div></>}
      </>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
