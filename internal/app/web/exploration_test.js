"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  validCursor,
  wallRequestPath,
  normalizeWallPage
} = require("./editor.js");

test("builds cursor wall requests without offset pages", () => {
  assert.equal(wallRequestPath(null, 24), "/wall?format=json&limit=24");
  assert.equal(wallRequestPath(42, 24), "/wall?format=json&limit=24&before_id=42");
  assert.equal(wallRequestPath(null), "/wall?format=json&limit=24");
  assert.throws(() => wallRequestPath(0, 24), /beforeID/);
  assert.throws(() => wallRequestPath(1.5, 24), /beforeID/);
});

test("accepts only positive safe cursor values", () => {
  assert.equal(validCursor(1), true);
  assert.equal(validCursor(Number.MAX_SAFE_INTEGER), true);
  assert.equal(validCursor(0), false);
  assert.equal(validCursor(-1), false);
  assert.equal(validCursor(1.5), false);
  assert.equal(validCursor("1"), false);
});

test("normalizes wall cursor payloads and rejects malformed cursors", () => {
  const postcards = [{ id: 3 }, { id: 2 }];
  assert.deepEqual(normalizeWallPage({ postcards, next_before_id: 2 }), {
    postcards,
    nextBeforeID: 2
  });
  assert.deepEqual(normalizeWallPage({ postcards }), {
    postcards,
    nextBeforeID: null
  });
  assert.equal(normalizeWallPage({ postcards, next_before_id: 0 }), null);
  assert.equal(normalizeWallPage({ postcards, next_before_id: "2" }), null);
  assert.equal(normalizeWallPage({ postcards: null }), null);
});
