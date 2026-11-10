import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { createLatest } from "./latest.ts";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("createLatest", () => {
  it("marks an older call stale when a newer one starts", async () => {
    const latest = createLatest();
    const slow = deferred<string>();
    const fast = deferred<string>();
    const first = latest.run(() => slow.promise);
    const second = latest.run(() => fast.promise);
    fast.resolve("new");
    slow.resolve("old");
    assert.deepEqual(await second, { stale: false, value: "new" });
    assert.equal((await first).stale, true);
  });

  it("swallows failures of stale calls but rethrows current ones", async () => {
    const latest = createLatest();
    const old = deferred<string>();
    const first = latest.run(() => old.promise);
    const second = latest.run(() => Promise.reject(new Error("boom")));
    old.reject(new Error("ignored"));
    assert.equal((await first).stale, true);
    await assert.rejects(second, /boom/);
  });

  it("invalidate makes the call in flight stale", async () => {
    const latest = createLatest();
    const pending = latest.run(() => Promise.resolve(1));
    latest.invalidate();
    assert.equal((await pending).stale, true);
  });
});
