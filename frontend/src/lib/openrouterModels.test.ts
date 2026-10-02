import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { availableModelOptions, formatPricePerMillion, formatTokenCapacity } from "./openrouterModels.ts";

describe("availableModelOptions", () => {
  it("keeps the selected custom model and historical models without repeating recommendations", () => {
    assert.deepEqual(
      availableModelOptions("openrouter/auto", "custom/model", ["old/model", "google/gemini-test"], ["google/gemini-test"]),
      ["custom/model", "old/model", "openrouter/auto"],
    );
  });
});

describe("OpenRouter model details", () => {
  it("converts API prices from dollars per token to dollars per million tokens", () => {
    assert.equal(formatPricePerMillion("0.0000003"), "$0.30 / 1M tokens");
    assert.equal(formatPricePerMillion("0"), "Free");
    assert.equal(formatPricePerMillion("unknown"), "Price unavailable");
  });

  it("shows token capacity at a readable scale", () => {
    assert.equal(formatTokenCapacity(128000), "128K tokens");
    assert.equal(formatTokenCapacity(1048576), "1.05M tokens");
  });
});
