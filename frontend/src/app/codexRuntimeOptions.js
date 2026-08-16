const unique = (values) => [...new Set(values.filter(Boolean))];

export const demoCodexConfiguration = {
  model: "gpt-5.6-sol",
  reasoningEffort: "medium",
  serviceTier: "",
  models: [
    {
      id: "gpt-5.6-sol",
      model: "gpt-5.6-sol",
      displayName: "GPT-5.6-Sol",
      description: "Complex, open-ended work",
      defaultReasoningEffort: "low",
      reasoningEfforts: ["low", "medium", "high", "xhigh", "max", "ultra"],
      serviceTiers: [{ id: "fast", name: "Fast", description: "1.5× speed" }],
      isDefault: true,
    },
    {
      id: "gpt-5.6-terra",
      model: "gpt-5.6-terra",
      displayName: "GPT-5.6-Terra",
      description: "Everyday workhorse",
      defaultReasoningEffort: "medium",
      reasoningEfforts: ["low", "medium", "high", "xhigh", "max", "ultra"],
      serviceTiers: [{ id: "fast", name: "Fast", description: "1.5× speed" }],
    },
  ],
};

export const demoClaudeConfiguration = {
  efforts: ["low", "medium", "high", "xhigh", "max"],
  models: [
    { model: "fable", displayName: "Fable", alias: true },
    { model: "opus", displayName: "Opus", alias: true },
    { model: "sonnet", displayName: "Sonnet", alias: true },
    { model: "claude-fable-5", displayName: "claude-fable-5", alias: false },
  ],
};

export function selectedCodexModel(configuration, selected = "") {
  const models = configuration?.models || [];
  const target = selected || configuration?.model || "";
  return models.find((model) => model.model === target || model.id === target)
    || models.find((model) => model.isDefault)
    || models[0]
    || null;
}

export function codexEffortValues(configuration, selected = "", current = "") {
  const model = selectedCodexModel(configuration, selected);
  return unique([
    ...(model?.reasoningEfforts || []),
    !selected ? configuration?.reasoningEffort : "",
    current,
  ]);
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
