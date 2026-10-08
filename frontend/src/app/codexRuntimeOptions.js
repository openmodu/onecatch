const unique = (values) => [...new Set(values.filter(Boolean))];

export const demoClaudeConfiguration = {
  efforts: ["low", "medium", "high", "xhigh", "max"],
  models: [
    { model: "fable", displayName: "Fable", alias: true },
    { model: "opus", displayName: "Opus", alias: true },
    { model: "sonnet", displayName: "Sonnet", alias: true },
    { model: "claude-fable-5", displayName: "claude-fable-5", alias: false },
  ],
};

export function claudeModelDisplayLabel(model = {}) {
  const value = String(model.model || model.id || "");
  if (model.displayName && model.displayName !== value) return model.displayName;
  const versioned = value.toLowerCase().match(/^claude-([a-z]+)-(\d+)(?:[-.](\d+))?/);
  if (versioned) {
    const family = `${versioned[1][0].toUpperCase()}${versioned[1].slice(1)}`;
    return `${family} ${versioned[2]}${versioned[3] ? `.${versioned[3]}` : ""}`;
  }
  return model.displayName || value;
}

export function groupedClaudeModels(models = []) {
  const aliases = models.filter((model) => model.alias);
  return aliases.length
    ? { primary: aliases, more: models.filter((model) => !model.alias) }
    : { primary: models, more: [] };
}

export function defaultClaudeModel(models = [], configured = "") {
  return configured
    || models.find((model) => model.isDefault)?.model
    || "";
}

export function selectedCodexModel(configuration, selected = "") {
  const models = configuration?.models || [];
  const target = selected || configuration?.model || "";
  if (target) {
    return models.find((model) => model.model === target || model.id === target)
      || { model: target, displayName: target };
  }
  return models.find((model) => model.isDefault)
    || null;
}

// HarnessConfiguration uses `efforts`/`defaultEffort` for generic adapters
// such as Pi and Grok, while Codex's richer app-server response uses
// `reasoningEfforts`/`defaultReasoningEffort`. Keep that wire-format
// difference out of the menu so every direct Agent gets the controls its
// adapter advertised.
export function runtimeEffortValues(configuration, selected = "", current = "") {
  const model = selectedCodexModel(configuration, selected);
  const modelEfforts = model?.reasoningEfforts?.length
    ? model.reasoningEfforts
    : model?.efforts || [];
  const supported = modelEfforts.length ? modelEfforts : configuration?.efforts || [];
  return unique([
    ...supported,
    !selected ? configuration?.reasoningEffort : "",
    current,
  ]);
}

export function runtimeDefaultEffort(configuration, selected = "") {
  const model = selectedCodexModel(configuration, selected);
  return configuration?.reasoningEffort
    || model?.defaultReasoningEffort
    || model?.defaultEffort
    || "";
}

export function codexEffortValues(configuration, selected = "", current = "") {
  return runtimeEffortValues(configuration, selected, current);
}

export function codexServiceTierValues(configuration, selected = "", current = "") {
  const model = selectedCodexModel(configuration, selected);
  return unique([
    "standard",
    ...(model?.serviceTiers || []).map((tier) => tier.id),
    !selected ? configuration?.serviceTier : "",
    current,
  ]);
}
