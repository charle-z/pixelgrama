"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  EditorModel,
  DRAFT_VERSION,
  HISTORY_LIMIT,
  linePoints,
  validateDraft,
  formatPostcardDate,
  flippedHorizontally,
  flippedVertically
} = require("./editor.js");

test("interpolates fast strokes and groups them into one undo entry", () => {
  const editor = new EditorModel();
  editor.setSelected(3);
  editor.beginStroke(0, 0);
  editor.continueStroke(7, 0);
  editor.endStroke();
  assert.deepEqual(editor.pixels.slice(0, 8), new Array(8).fill(3));
  assert.equal(editor.undoStack.length, 1);
  assert.equal(editor.undo(), true);
  assert.deepEqual(editor.pixels.slice(0, 8), new Array(8).fill(0));
  assert.equal(editor.redo(), true);
  assert.deepEqual(editor.pixels.slice(0, 8), new Array(8).fill(3));
});

test("keeps history bounded", () => {
  const editor = new EditorModel({ historyLimit: 3 });
  for (let index = 0; index < 5; index += 1) {
    editor.setSelected(index + 1);
    editor.applyAt(index, 0);
  }
  assert.equal(editor.undoStack.length, 3);
  assert.equal(HISTORY_LIMIT, 64);
});

test("supports eraser, fill and eyedropper", () => {
  const editor = new EditorModel();
  editor.setSelected(4);
  editor.setTool("fill");
  assert.equal(editor.applyAt(0, 0), true);
  assert.ok(editor.pixels.every((value) => value === 4));
  editor.setTool("eraser");
  editor.applyAt(2, 2);
  assert.equal(editor.pixels[34], 0);
  editor.setTool("eyedropper");
  editor.applyAt(0, 0);
  assert.equal(editor.selected, 4);
});

test("flips pixels without losing values", () => {
  const pixels = new Array(256).fill(0);
  pixels[0] = 1;
  pixels[15] = 2;
  pixels[240] = 3;
  assert.equal(flippedHorizontally(pixels)[15], 1);
  assert.equal(flippedHorizontally(pixels)[0], 2);
  assert.equal(flippedVertically(pixels)[240], 1);
  assert.equal(flippedVertically(pixels)[0], 3);
});

test("uses Bresenham interpolation for diagonal strokes", () => {
  assert.deepEqual(linePoints(0, 0, 3, 3), [
    { x: 0, y: 0 },
    { x: 1, y: 1 },
    { x: 2, y: 2 },
    { x: 3, y: 3 }
  ]);
});

test("never persists an invalid alias with an otherwise valid draft", () => {
  const editor = new EditorModel();
  editor.setSelected(6);
  editor.applyAt(0, 0);
  const draft = editor.draft("<invalid>");
  assert.equal(draft.alias, "");
  assert.equal(draft.pixels[0], 6);
  assert.notEqual(validateDraft(draft), null);
});

test("localizes postcard dates and rejects invalid values", () => {
  const value = "2026-07-28T12:34:00Z";
  const spanish = formatPostcardDate(value, "es", "UTC");
  const english = formatPostcardDate(value, "en", "UTC");
  assert.match(spanish, /2026/);
  assert.match(english, /2026/);
  assert.notEqual(spanish, english);
  assert.equal(formatPostcardDate("not-a-date", "es", "UTC"), "");
});

test("accepts only current, bounded and typed drafts", () => {
  const valid = {
    version: DRAFT_VERSION,
    pixels: new Array(256).fill(2),
    alias: "DRAFT_1",
    selected: 2,
    tool: "pencil",
    parentId: null
  };
  assert.deepEqual(validateDraft(valid), valid);
  assert.equal(validateDraft({ ...valid, version: DRAFT_VERSION + 1 }), null);
  assert.equal(validateDraft({ ...valid, pixels: [1, 2] }), null);
  assert.equal(validateDraft({ ...valid, alias: "<script>" }), null);
  assert.equal(validateDraft({ ...valid, selected: 16 }), null);
  assert.equal(validateDraft({ ...valid, tool: "spray" }), null);
});
