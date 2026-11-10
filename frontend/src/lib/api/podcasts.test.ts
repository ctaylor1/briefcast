import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { podcastsApi } from "./podcasts.ts";
import { fakeServer, type FakeServer } from "../../test-support/http.ts";

let server: FakeServer | null = null;
afterEach(() => server?.restore());

describe("podcastsApi.list", () => {
  it("returns an empty list when the server sends null for an empty library", async () => {
    server = fakeServer({ "GET /podcasts": () => null });
    assert.deepEqual(await podcastsApi.list(), []);
  });

  it("returns the podcasts the server sends", async () => {
    server = fakeServer({ "GET /podcasts": () => [{ ID: "p1", Title: "One" }] });
    assert.deepEqual(await podcastsApi.list(), [{ ID: "p1", Title: "One" }]);
  });
});
