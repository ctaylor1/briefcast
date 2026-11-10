import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { usePlayer } from "./usePlayer.ts";
import { makeAudio, type FakeAudio } from "../test-support/audio.ts";
import { makeEpisode } from "../test-support/fixtures.ts";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http.ts";

const ID = "aaaaaaaa-0000-4000-8000-0000000000aa";
const NEXT = "aaaaaaaa-0000-4000-8000-0000000000bb";
let server: FakeServer;
let audio: FakeAudio;
let detach: () => void;
let saved: Array<[string, number]> = [];
const player = usePlayer();

function serve() {
  saved = [];
  const record = (id: string) => (request: { data?: unknown }) => {
    saved.push([id, (request.data as { positionSeconds: number }).positionSeconds]);
    return null;
  };
  return fakeServer({
    [`GET /podcastitems/${ID}/chapters`]: () => ({ source: "", chapters: [] }),
    [`GET /podcastitems/${NEXT}/chapters`]: () => ({ source: "", chapters: [] }),
    [`PATCH /podcastitems/${ID}/position`]: record(ID),
    [`PATCH /podcastitems/${NEXT}/position`]: record(NEXT),
    [`PATCH /podcastitems/${ID}/markPlayed`]: () => null,
  });
}

beforeEach(() => {
  audio = makeAudio();
  detach = player.attach(audio);
  server = serve();
});

afterEach(() => {
  player.stop();
  detach();
  server.restore();
});

async function start(item: ReturnType<typeof makeEpisode>, startSeconds?: number) {
  const started = player.playQueue([item], 0, startSeconds);
  await flushPromises(1);
  audio.setDuration(item.Duration);
  await started;
}

describe("resume position", () => {
  it("resumes from a saved position", async () => {
    await start(makeEpisode({ ID, Duration: 3600, PlaybackPositionSeconds: 754 }));
    assert.equal(audio.currentTime, 754);
  });

  it("starts over when the saved position is at the very start or end", async () => {
    await start(makeEpisode({ ID, Duration: 3600, PlaybackPositionSeconds: 4 }));
    assert.equal(audio.currentTime, 0);
    player.stop();
    await start(makeEpisode({ ID, Duration: 3600, PlaybackPositionSeconds: 3590 }));
    assert.equal(audio.currentTime, 0);
  });

  it("lets an explicit start time win over the saved one", async () => {
    await start(makeEpisode({ ID, Duration: 3600, PlaybackPositionSeconds: 754 }), 60);
    assert.equal(audio.currentTime, 60);
  });

  it("saves the position on pause and when switching episodes", async () => {
    const first = makeEpisode({ ID, Duration: 3600 });
    const second = makeEpisode({ ID: NEXT, Duration: 1200 });
    const started = player.playQueue([first, second]);
    await started;
    audio.currentTime = 321.7;
    audio.fire("timeupdate");
    player.pause();
    await flushPromises();
    assert.deepEqual(saved.at(-1), [ID, 321]);
    assert.equal(first.PlaybackPositionSeconds, 321, "the local copy is updated too");
    audio.currentTime = 400;
    audio.fire("timeupdate");
    await player.next();
    await flushPromises();
    assert.ok(saved.some(([id, s]) => id === ID && s === 400), JSON.stringify(saved));
  });

  it("does not save the same position twice", async () => {
    await start(makeEpisode({ ID, Duration: 3600 }));
    audio.currentTime = 100;
    audio.fire("timeupdate");
    player.pause();
    player.pause();
    await flushPromises();
    assert.equal(saved.filter(([, s]) => s === 100).length, 1);
  });

  it("resets the position to 0 when the episode finishes", async () => {
    await start(makeEpisode({ ID, Duration: 600, IsPlayed: false }));
    audio.currentTime = 600;
    audio.fire("timeupdate");
    audio.fire("ended");
    await flushPromises();
    assert.deepEqual(saved.at(-1), [ID, 0]);
  });
});
