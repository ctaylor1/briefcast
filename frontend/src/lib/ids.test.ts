import { describe, it } from "node:test";
import assert from "node:assert/strict";
import { isRecordId, parseRecordIds } from "./ids.ts";

const A = "3f2a8c1e-1b2c-4d5e-8f90-0123456789ab";
const B = "AAAAAAAA-BBBB-4CCC-8DDD-EEEEEEEEEEEE";

describe("record IDs", () => {
  it("accepts UUIDs only", () => {
    assert.equal(isRecordId(A), true);
    assert.equal(isRecordId(B), true);
    for (const bad of ["", "1", "a/delete", `${A}/delete`, `${A}?x=1`, " " + A, null, 5]) {
      assert.equal(isRecordId(bad), false, String(bad));
    }
  });

  it("parses comma lists and arrays, dropping invalid and duplicate IDs", () => {
    assert.deepEqual(parseRecordIds(`${A}, ${B}`, undefined), [A, B]);
    assert.deepEqual(parseRecordIds([`${A},../../x`, A], B), [A, B]);
    assert.deepEqual(parseRecordIds("x,y", ["z"]), []);
  });
});
