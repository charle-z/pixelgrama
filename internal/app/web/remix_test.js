"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  EditorModel,
  DRAFT_VERSION,
  validateDraft
} = require("./editor.js");

test("migrates version one drafts without a remix parent", () => {
  const legacy = {
    version: 1,
    pixels: new Array(256).fill(3),
    alias: "LEGACY",
    selected: 3,
    tool: "pencil"
  };
  const migrated = validateDraft(legacy);
  assert.equal(migrated.version, DRAFT_VERSION);
  assert.equal(migrated.parentId, null);
  assert.deepEqual(migrated.pixels, legacy.pixels);
});

test("persists only valid remix parent identifiers", () => {
  const editor = new EditorModel();
  const valid = editor.draft("REMIX", 42);
  assert.equal(valid.parentId, 42);
  assert.equal(validateDraft(valid).parentId, 42);

  assert.equal(editor.draft("REMIX", 0).parentId, null);
  assert.equal(editor.draft("REMIX", -1).parentId, null);
  assert.equal(editor.draft("REMIX", 1.5).parentId, null);
  assert.equal(validateDraft({ ...valid, parentId: 0 }), null);
  assert.equal(validateDraft({ ...valid, parentId: "42" }), null);
});
