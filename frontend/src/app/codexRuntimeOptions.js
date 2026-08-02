const unique = (values) => [...new Set(values.filter(Boolean))];

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
