export function availableModelOptions(
  defaultModel: string,
  selectedModel: string,
  usedModels: string[],
  recommendedIds: string[],
): string[] {
  const recommended = new Set(recommendedIds);
  return [...new Set([defaultModel, selectedModel, ...usedModels].filter(Boolean))]
    .filter((model) => !recommended.has(model))
    .sort();
}

export function formatPricePerMillion(pricePerToken: string): string {
  const price = Number(pricePerToken);
  if (pricePerToken.trim() === "" || !Number.isFinite(price) || price < 0) {
    return "Price unavailable";
  }
  if (price === 0) return "Free";
  return `$${(price * 1_000_000).toFixed(2)} / 1M tokens`;
}

export function formatTokenCapacity(tokens: number): string {
  if (!Number.isFinite(tokens) || tokens <= 0) return "Unknown context";
  if (tokens >= 1_000_000) return `${(tokens / 1_000_000).toFixed(2)}M tokens`;
  if (tokens >= 1_000) return `${Math.round(tokens / 1_000)}K tokens`;
  return `${tokens} tokens`;
}
