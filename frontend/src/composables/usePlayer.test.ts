import { afterEach, beforeEach, describe, it } from "node:test";
import assert from "node:assert/strict";
import { nextTick } from "vue";
import { usePlayer } from "./usePlayer.ts";
import { makeAudio, type FakeAudio } from "../test-support/audio.ts";
import { makeEpisode } from "../test-support/fixtures.ts";
import { fakeServer, flushPromises, type FakeServer } from "../test-support/http.ts";

const A = makeEpisode({ ID: "aaaaaaaa-0000-4000-8000-000000000001", Title: "First", Duration: 600 });
const B = makeEpisode({ ID: "aaaaaaaa-0000-4000-8000-000000000002", Title: "Second", Duration: 900 });

let server: FakeServer;
let audio: FakeAudio;
let detach: () => void;
const player = usePlayer();

function routes(extra: Record<string, () => unknown> = {}) {
  return fakeServer({
    [`GET /podcastitems/${A.ID}/chapters`]: () => ({ source: "", chapters: [] }),
    [`GET /podcastitems/${B.ID}/chapters`]: () => ({ source: "", chapters: [] }),
    ...extra,
  });
}

beforeEach(() => {
  // The player writes resume positions onto the queued items; start each test clean.
  A.PlaybackPositionSeconds = null;
  B.PlaybackPositionSeconds = null;
  audio = makeAudio();
  detach = player.attach(audio);
});

afterEach(() => {
  player.stop();
  detach();
  server?.restore();
  try {
    localStorage.clear();
  } catch {
    // ignore
  }
});

describe("usePlayer", () => {
  it("plays a queue from the first episode, with an encoded audio URL", async () => {
    server = routes();
    await player.playQueue([A, B]);
    assert.equal(player.current.value?.ID, A.ID);
    assert.equal(audio.getAttribute("src"), `/podcastitems/${A.ID}/file`);
    assert.equal(audio.playCalls, 1);
    assert.equal(player.state.playing, true);
    assert.equal(player.hasNext.value, true);
    assert.ok(document.documentElement.classList.contains("has-player"));
  });

  it("moves through the queue; Previous restarts the episode once it has played a while", async () => {
    server = routes();
    await player.playQueue([A, B]);
    await player.next();
    assert.equal(player.current.value?.ID, B.ID);
    audio.currentTime = 42;
    audio.fire("timeupdate");
    await player.previous();
    assert.equal(player.current.value?.ID, B.ID, "restarted, did not go back");
    assert.equal(audio.currentTime, 0);
    await player.previous();
    assert.equal(player.current.value?.ID, A.ID);
  });

  it("seeks within the episode and skips by the given amount", async () => {
    server = routes();
    await player.playQueue([A]);
    audio.setDuration(600);
    player.seek(700);
    assert.equal(audio.currentTime, 600, "clamped to the duration");
    player.seek(100);
    player.skip(30);
    assert.equal(audio.currentTime, 130);
    player.skip(-500);
    assert.equal(audio.currentTime, 0);
  });

  it("starts at a requested time once metadata has loaded", async () => {
    server = routes();
    const started = player.playQueue([A], 0, 125);
    await flushPromises(1);
    assert.equal(audio.playCalls, 0, "waits for metadata before playing");
    audio.setDuration(600);
    await started;
    assert.equal(audio.currentTime, 125);
    assert.equal(audio.playCalls, 1);
  });

  it("lets the newest request win when two start at once", async () => {
    server = routes();
    const first = player.playQueue([A], 0, 100);
    const second = player.playQueue([B], 0, 200);
    audio.setDuration(900);
    await Promise.all([first, second]);
    assert.equal(player.current.value?.ID, B.ID);
    assert.equal(audio.currentTime, 200);
    assert.equal(audio.playCalls, 1);
  });

  it("marks an episode played when it ends and continues with the next", async () => {
    let marked = "";
    server = routes({ [`PATCH /podcastitems/${A.ID}/markPlayed`]: () => (marked = A.ID) });
    await player.playQueue([{ ...A, IsPlayed: false }, B]);
    audio.fire("ended");
    await flushPromises();
    assert.equal(marked, A.ID);
    assert.equal(player.current.value?.ID, B.ID);
  });

  it("validates and remembers the playback speed", async () => {
    server = routes();
    await player.playQueue([A]);
    player.setRate(1.5);
    assert.equal(audio.playbackRate, 1.5);
    assert.equal(localStorage.getItem("briefcast-playback-rate"), "1.5");
    player.setRate(9);
    assert.equal(player.state.rate, 1.5, "unknown speeds are ignored");
  });

  it("skips sponsor chapters automatically when the podcast asks for it, once per segment", async () => {
    const item = { ...A, Podcast: { ID: "p", Title: "Show", AutoSkipSponsorChapters: true } };
    server = fakeServer({
      [`GET /podcastitems/${A.ID}/chapters`]: () => ({
        source: "file",
        chapters: [
          { title: "Intro", startSeconds: 0 },
          { title: "Sponsor: Example", startSeconds: 60 },
          { title: "Main", startSeconds: 120 },
        ],
      }),
    });
    await player.playQueue([item]);
    audio.setDuration(600);
    await flushPromises();
    audio.currentTime = 70;
    audio.fire("timeupdate");
    assert.equal(audio.currentTime, 120);
    audio.currentTime = 70;
    audio.fire("timeupdate");
    assert.equal(audio.currentTime, 70, "a manual seek back into the segment is respected");
  });

  it("reports blocked autoplay so the user can press Play", async () => {
    server = routes();
    audio.rejectPlayWith = new DOMException("blocked", "NotAllowedError");
    await player.playQueue([A]);
    assert.equal(player.state.autoplayBlocked, true);
    assert.equal(player.state.playing, false);
  });

  it("clears the bar's space when stopped", async () => {
    server = routes();
    await player.playQueue([A]);
    player.stop();
    await nextTick();
    assert.equal(player.current.value, null);
    assert.ok(!document.documentElement.classList.contains("has-player"));
  });
});
