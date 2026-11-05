import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { userAgentSourceLabel } from "./userAgent.ts";

describe("userAgentSourceLabel", () => {
  it("names the configuration source", () => {
    assert.match(userAgentSourceLabel("env"), /^Set by BRIEFCAST_USER_AGENT/);
    assert.match(userAgentSourceLabel("legacy_setting"), /legacy settings page/);
    assert.match(userAgentSourceLabel("default"), /^Briefcast default/);
    assert.match(userAgentSourceLabel(undefined), /^Briefcast default/);
  });
});
