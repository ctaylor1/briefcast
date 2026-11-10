import { afterEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { apiPath, episodeMarkdownPackageUrl, podcastImageUrl } from "./paths.ts";
import { episodesApi } from "./episodes.ts";
import { podcastsApi } from "./podcasts.ts";
import { summariesApi } from "./summaries.ts";
import { downloadsApi } from "./downloads.ts";
import { settingsApi } from "./settings.ts";
import { fakeServer, type FakeServer } from "../../test-support/http.ts";

let server: FakeServer | null = null;
afterEach(() => server?.restore());

describe("apiPath", () => {
  it("encodes every interpolated value", () => {
    assert.equal(apiPath`/podcastitems/${"a/delete"}/summary`, "/podcastitems/a%2Fdelete/summary");
    assert.equal(apiPath`/x/${"a?b=c#d"}`, "/x/a%3Fb%3Dc%23d");
    assert.equal(apiPath`/x/${".."}/y`, "/x/%2E%2E/y");
    assert.equal(apiPath`/x/${"."}/y`, "/x/%2E/y");
    assert.equal(apiPath`/x/${"a.b"}/y`, "/x/a.b/y");
    assert.equal(podcastImageUrl("p/1"), "/podcasts/p%2F1/image");
    assert.equal(episodeMarkdownPackageUrl("e 1", "summary"), "/podcastitems/e%201/summary?format=markdown-package");
  });
});

describe("API modules", () => {
  it("send encoded IDs on every path", async () => {
    const hostile = "a/delete";
    server = fakeServer({
      "GET /podcastitems/a%2Fdelete": () => ({}),
      "PATCH /podcastitems/a%2Fdelete/markPlayed": () => null,
      "POST /podcastitems/a%2Fdelete/download": () => null,
      "DELETE /podcasts/a%2Fdelete": () => null,
      "POST /summaries/a%2Fdelete/favorite": () => null,
      "POST /podcastitems/a%2Fdelete/cancel": () => null,
      "POST /settings/prompt-versions/a%2Fdelete/restore": () => ({}),
    });
    await episodesApi.getById(hostile);
    await episodesApi.setPlayed(hostile, true);
    await episodesApi.queueDownload(hostile);
    await podcastsApi.deleteById(hostile);
    await summariesApi.favorite(hostile);
    await downloadsApi.cancelEpisode(hostile);
    await settingsApi.restorePromptVersion(hostile);
    assert.deepEqual(
      server.requests.map((r) => `${r.method} ${r.url}`),
      [
        "GET /podcastitems/a%2Fdelete",
        "PATCH /podcastitems/a%2Fdelete/markPlayed",
        "POST /podcastitems/a%2Fdelete/download",
        "DELETE /podcasts/a%2Fdelete",
        "POST /summaries/a%2Fdelete/favorite",
        "POST /podcastitems/a%2Fdelete/cancel",
        "POST /settings/prompt-versions/a%2Fdelete/restore",
      ],
    );
  });

  it("send query values as params rather than pasted into the path", async () => {
    server = fakeServer({
      "GET /settings/prompt-versions": () => [],
      "GET /settings/logs": () => ({ entries: [] }),
    });
    await settingsApi.getPromptVersions("system");
    await settingsApi.getLogs(5);
    assert.deepEqual(server.requests.map((r) => [r.url, r.params]), [
      ["/settings/prompt-versions", { type: "system" }],
      ["/settings/logs", { limit: 5 }],
    ]);
  });
});
