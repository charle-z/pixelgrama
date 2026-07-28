"use strict";

const test = require("node:test");
const assert = require("node:assert/strict");
const {
  EditorModel,
  DRAFT_VERSION,
  HISTORY_LIMIT,
  PALETTE_CATALOG,
  paletteByID,
  validPaletteRef,
  normalizePostcard,
  normalizePublicStats,
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
  assert.ok(spanish.length > 0 && english.length > 0);
  assert.equal(formatPostcardDate("not-a-date", "es", "UTC"), "");
});

test("accepts only current, bounded and typed drafts", () => {
  const valid = {
    version: DRAFT_VERSION,
    pixels: new Array(256).fill(2),
    alias: "DRAFT_1",
    selected: 2,
    tool: "pencil",
    parentId: null,
    paletteId: "vga16",
    paletteVersion: 1
  };
  assert.deepEqual(validateDraft(valid), valid);
  assert.equal(validateDraft({ ...valid, version: DRAFT_VERSION + 1 }), null);
  assert.equal(validateDraft({ ...valid, pixels: [1, 2] }), null);
  assert.equal(validateDraft({ ...valid, alias: "<script>" }), null);
  assert.equal(validateDraft({ ...valid, selected: 16 }), null);
  assert.equal(validateDraft({ ...valid, tool: "spray" }), null);
  assert.equal(validateDraft({ ...valid, paletteId: "arbitrary" }), null);
  assert.equal(validateDraft({ ...valid, paletteVersion: 2 }), null);
});

test("uses only the closed versioned palette catalog", () => {
  assert.equal(PALETTE_CATALOG.catalogVersion, 1);
  assert.equal(PALETTE_CATALOG.palettes.length, 3);
  assert.deepEqual(
    PALETTE_CATALOG.palettes.map((palette) => palette.id),
    ["vga16", "grayscale16", "sunset16"]
  );
  assert.equal(validPaletteRef("vga16", 1), true);
  assert.equal(validPaletteRef("grayscale16", 1), true);
  assert.equal(validPaletteRef("sunset16", 1), true);
  assert.equal(validPaletteRef("vga16", 2), false);
  assert.equal(validPaletteRef("custom", 1), false);
  assert.equal(paletteByID("grayscale16", 1).colors.length, 16);

  const editor = new EditorModel();
  assert.equal(editor.setPalette("grayscale16", 1), true);
  assert.equal(editor.setPalette("custom", 1), false);
  const draft = editor.draft("PALETTE");
  assert.equal(draft.paletteId, "grayscale16");
  assert.equal(draft.paletteVersion, 1);
  assert.equal(validateDraft(draft).paletteId, "grayscale16");
});

test("normalizes postcards only when their palette identity is supported", () => {
  const valid = {
    id: 7,
    pixels: new Array(256).fill(2),
    alias: "GRAY",
    created_at: "2026-07-28T12:34:00Z",
    content_hash: "a".repeat(64),
    format_version: 1,
    palette_id: "grayscale16",
    palette_version: 1,
    parent_id: 3
  };
  const normalized = normalizePostcard(valid);
  assert.equal(normalized.paletteId, "grayscale16");
  assert.equal(normalized.paletteVersion, 1);
  assert.equal(normalized.parentId, 3);
  assert.equal(normalizePostcard({ ...valid, palette_id: "custom" }), null);
  assert.equal(normalizePostcard({ ...valid, palette_version: 2 }), null);
  assert.equal(normalizePostcard({ ...valid, content_hash: "A".repeat(64) }), null);
});

test("normalizes complete public statistics without accepting invented palettes", () => {
  const valid = {
    schema_version: 1,
    week_key: "2026-W31",
    total_postcards: 8,
    postcards_this_week: 3,
    remix_count: 2,
    palettes: [
      { palette_id: "vga16", palette_version: 1, postcards: 5 },
      { palette_id: "grayscale16", palette_version: 1, postcards: 2 },
      { palette_id: "sunset16", palette_version: 1, postcards: 1 }
    ]
  };
  const normalized = normalizePublicStats(valid);
  assert.equal(normalized.totalPostcards, 8);
  assert.equal(normalized.palettes[1].paletteId, "grayscale16");
  assert.equal(normalizePublicStats({ ...valid, palettes: valid.palettes.slice(0, 2) }), null);
  assert.equal(normalizePublicStats({
    ...valid,
    palettes: valid.palettes.concat({ palette_id: "custom", palette_version: 1, postcards: 0 })
  }), null);
  assert.equal(normalizePublicStats({
    ...valid,
    palettes: [valid.palettes[0], valid.palettes[0], valid.palettes[2]]
  }), null);
});
