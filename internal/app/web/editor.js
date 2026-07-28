"use strict";

((root, factory) => {
  const catalog = root.PixelgramaPaletteCatalog || (typeof module === "object" && module.exports
    ? require("../../core/palettes.json") : null);
  const editor = factory(catalog);
  if (typeof module === "object" && module.exports) {
    module.exports = editor;
  }
  root.PixelgramaEditor = editor;
})(typeof globalThis === "object" ? globalThis : this, (catalogValue) => {
  const WIDTH = 16;
  const HEIGHT = 16;
  const PIXEL_COUNT = WIDTH * HEIGHT;
  const COLOR_COUNT = 16;
  const HISTORY_LIMIT = 64;
  const DRAFT_VERSION = 3;
  const TOOLS = Object.freeze(["pencil", "eraser", "fill", "eyedropper"]);

  function plainCatalogText(value) {
    return typeof value === "string"
      && value.length > 0
      && Array.from(value).length <= 64
      && !/[\u0000-\u001f\u007f<>]/.test(value);
  }

  function normalizePaletteCatalog(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)
      || value.catalog_version !== 1 || !Array.isArray(value.palettes) || value.palettes.length < 1) {
      return null;
    }
    const seen = new Set();
    const palettes = [];
    for (const item of value.palettes) {
      if (!item || typeof item !== "object" || Array.isArray(item)
        || typeof item.id !== "string" || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(item.id)
        || !Number.isInteger(item.version) || item.version < 1
        || !plainCatalogText(item.name_es) || !plainCatalogText(item.name_en)
        || !Array.isArray(item.colors) || item.colors.length !== COLOR_COUNT
        || !item.colors.every((color) => typeof color === "string" && /^#[0-9A-Fa-f]{6}$/.test(color))) {
        return null;
      }
      const key = item.id + "@" + item.version;
      if (seen.has(key)) {
        return null;
      }
      seen.add(key);
      palettes.push(Object.freeze({
        id: item.id,
        version: item.version,
        nameES: item.name_es,
        nameEN: item.name_en,
        colors: Object.freeze(item.colors.map((color) => color.toUpperCase()))
      }));
    }
    return Object.freeze({
      catalogVersion: 1,
      palettes: Object.freeze(palettes)
    });
  }

  const PALETTE_CATALOG = normalizePaletteCatalog(catalogValue);
  if (PALETTE_CATALOG === null) {
    throw new Error("Pixelgrama palette catalog is invalid");
  }

  function paletteByID(id, version) {
    return PALETTE_CATALOG.palettes.find((palette) => palette.id === id && palette.version === version) || null;
  }

  function validPaletteRef(id, version) {
    return paletteByID(id, version) !== null;
  }

  const DEFAULT_PALETTE = paletteByID("vga16", 1);
  if (DEFAULT_PALETTE === null) {
    throw new Error("Pixelgrama default palette is missing");
  }

  function validPixels(value) {
    return Array.isArray(value) && value.length === PIXEL_COUNT && value.every((item) => (
      Number.isInteger(item) && item >= 0 && item < COLOR_COUNT
    ));
  }

  function validTool(value) {
    return TOOLS.includes(value);
  }

  function validCoordinate(value, maximum) {
    return Number.isInteger(value) && value >= 0 && value < maximum;
  }

  function indexFor(x, y) {
    return y * WIDTH + x;
  }

  function equalPixels(left, right) {
    return left.every((value, index) => value === right[index]);
  }

  function linePoints(x0, y0, x1, y1) {
    const points = [];
    let x = x0;
    let y = y0;
    const deltaX = Math.abs(x1 - x0);
    const stepX = x0 < x1 ? 1 : -1;
    const deltaY = -Math.abs(y1 - y0);
    const stepY = y0 < y1 ? 1 : -1;
    let error = deltaX + deltaY;

    while (true) {
      points.push({ x, y });
      if (x === x1 && y === y1) {
        break;
      }
      const doubled = error * 2;
      if (doubled >= deltaY) {
        error += deltaY;
        x += stepX;
      }
      if (doubled <= deltaX) {
        error += deltaX;
        y += stepY;
      }
    }
    return points;
  }

  function floodFill(pixels, x, y, replacement) {
    const start = indexFor(x, y);
    const target = pixels[start];
    if (target === replacement) {
      return false;
    }
    const pending = [start];
    pixels[start] = replacement;
    while (pending.length > 0) {
      const current = pending.pop();
      const currentX = current % WIDTH;
      const currentY = Math.floor(current / WIDTH);
      const neighbours = [];
      if (currentX > 0) neighbours.push(current - 1);
      if (currentX < WIDTH - 1) neighbours.push(current + 1);
      if (currentY > 0) neighbours.push(current - WIDTH);
      if (currentY < HEIGHT - 1) neighbours.push(current + WIDTH);
      neighbours.forEach((next) => {
        if (pixels[next] === target) {
          pixels[next] = replacement;
          pending.push(next);
        }
      });
    }
    return true;
  }

  function flippedHorizontally(pixels) {
    const result = new Array(PIXEL_COUNT);
    for (let y = 0; y < HEIGHT; y += 1) {
      for (let x = 0; x < WIDTH; x += 1) {
        result[indexFor(WIDTH - 1 - x, y)] = pixels[indexFor(x, y)];
      }
    }
    return result;
  }

  function flippedVertically(pixels) {
    const result = new Array(PIXEL_COUNT);
    for (let y = 0; y < HEIGHT; y += 1) {
      for (let x = 0; x < WIDTH; x += 1) {
        result[indexFor(x, HEIGHT - 1 - y)] = pixels[indexFor(x, y)];
      }
    }
    return result;
  }

  function formatPostcardDate(value, language, timeZone) {
    if (typeof value !== "string") {
      return "";
    }
    const date = new Date(value);
    if (Number.isNaN(date.getTime())) {
      return "";
    }
    const options = { dateStyle: "medium", timeStyle: "short" };
    if (typeof timeZone === "string" && timeZone !== "") {
      options.timeZone = timeZone;
    }
    return new Intl.DateTimeFormat(language === "en" ? "en-US" : "es-CO", options).format(date);
  }

  function validCursor(value) {
    return Number.isSafeInteger(value) && value > 0;
  }

  function wallRequestPath(beforeID, limit) {
    const pageLimit = Number.isInteger(limit) && limit > 0 ? limit : 24;
    let path = "/wall?format=json&limit=" + pageLimit;
    if (beforeID !== null) {
      if (!validCursor(beforeID)) {
        throw new TypeError("beforeID must be a positive safe integer or null");
      }
      path += "&before_id=" + beforeID;
    }
    return path;
  }

  function normalizeWallPage(value) {
    if (!value || typeof value !== "object" || Array.isArray(value) || !Array.isArray(value.postcards)) {
      return null;
    }
    const nextBeforeID = value.next_before_id === undefined || value.next_before_id === null
      ? null
      : value.next_before_id;
    if (nextBeforeID !== null && !validCursor(nextBeforeID)) {
      return null;
    }
    return {
      postcards: value.postcards,
      nextBeforeID
    };
  }

  function normalizePostcard(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)
      || !Number.isSafeInteger(value.id) || value.id < 1
      || !validPixels(value.pixels)
      || value.format_version !== 1
      || !validPaletteRef(value.palette_id, value.palette_version)
      || typeof value.content_hash !== "string" || !/^[0-9a-f]{64}$/.test(value.content_hash)
      || typeof value.created_at !== "string" || Number.isNaN(new Date(value.created_at).getTime())) {
      return null;
    }
    if (value.alias !== undefined && (typeof value.alias !== "string"
      || !/^[A-Za-z0-9 _-]{1,16}$/.test(value.alias))) {
      return null;
    }
    if (value.parent_id !== undefined && (!Number.isSafeInteger(value.parent_id) || value.parent_id < 1)) {
      return null;
    }
    return {
      id: value.id,
      pixels: value.pixels.slice(),
      alias: value.alias || "",
      createdAt: value.created_at,
      contentHash: value.content_hash,
      paletteId: value.palette_id,
      paletteVersion: value.palette_version,
      parentId: value.parent_id === undefined ? null : value.parent_id
    };
  }

  function nonNegativeInteger(value) {
    return Number.isSafeInteger(value) && value >= 0;
  }

  function normalizePublicStats(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)
      || value.schema_version !== 1
      || typeof value.week_key !== "string" || !/^\d{4}-W\d{2}$/.test(value.week_key)
      || !nonNegativeInteger(value.total_postcards)
      || !nonNegativeInteger(value.postcards_this_week)
      || !nonNegativeInteger(value.remix_count)
      || !Array.isArray(value.palettes)) {
      return null;
    }
    const counts = new Map();
    for (const item of value.palettes) {
      if (!item || typeof item !== "object" || Array.isArray(item)
        || !validPaletteRef(item.palette_id, item.palette_version)
        || !nonNegativeInteger(item.postcards)) {
        return null;
      }
      const key = item.palette_id + "@" + item.palette_version;
      if (counts.has(key)) {
        return null;
      }
      counts.set(key, item.postcards);
    }
    if (counts.size !== PALETTE_CATALOG.palettes.length) {
      return null;
    }
    const palettes = PALETTE_CATALOG.palettes.map((palette) => {
      const key = palette.id + "@" + palette.version;
      if (!counts.has(key)) {
        return null;
      }
      return {
        paletteId: palette.id,
        paletteVersion: palette.version,
        nameES: palette.nameES,
        nameEN: palette.nameEN,
        postcards: counts.get(key)
      };
    });
    if (palettes.some((palette) => palette === null)) {
      return null;
    }
    return {
      schemaVersion: 1,
      weekKey: value.week_key,
      totalPostcards: value.total_postcards,
      postcardsThisWeek: value.postcards_this_week,
      remixCount: value.remix_count,
      palettes
    };
  }

  function plainChallengeText(value) {
    return typeof value === "string"
      && value.length > 0
      && Array.from(value).length <= 96
      && !/[\u0000-\u001f\u007f<>]/.test(value);
  }

  function normalizeDailyChallenge(value) {
    if (!value || typeof value !== "object" || Array.isArray(value) || value.catalog_version !== 1) {
      return null;
    }
    if (typeof value.date !== "string" || !/^\d{4}-\d{2}-\d{2}$/.test(value.date)) {
      return null;
    }
    const date = new Date(value.date + "T00:00:00Z");
    if (Number.isNaN(date.getTime()) || date.toISOString().slice(0, 10) !== value.date) {
      return null;
    }
    if (typeof value.slug !== "string" || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(value.slug)) {
      return null;
    }
    if (!plainChallengeText(value.prompt_es) || !plainChallengeText(value.prompt_en)) {
      return null;
    }
    return {
      catalogVersion: 1,
      date: value.date,
      slug: value.slug,
      promptES: value.prompt_es,
      promptEN: value.prompt_en
    };
  }

  function dailyChallengePrompt(value, language) {
    if (!value) {
      return "";
    }
    return language === "en" ? value.promptEN : value.promptES;
  }

  function validateDraft(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return null;
    }
    if (![1, 2, DRAFT_VERSION].includes(value.version) || !validPixels(value.pixels)) {
      return null;
    }
    if (typeof value.alias !== "string" || !/^[A-Za-z0-9 _-]{0,16}$/.test(value.alias)) {
      return null;
    }
    if (!Number.isInteger(value.selected) || value.selected < 0 || value.selected >= COLOR_COUNT) {
      return null;
    }
    if (!validTool(value.tool)) {
      return null;
    }
    let parentId = null;
    if (value.version >= 2 && value.parentId !== null && value.parentId !== undefined) {
      if (!Number.isSafeInteger(value.parentId) || value.parentId < 1) {
        return null;
      }
      parentId = value.parentId;
    }
    let palette = DEFAULT_PALETTE;
    if (value.version === DRAFT_VERSION) {
      palette = paletteByID(value.paletteId, value.paletteVersion);
      if (palette === null) {
        return null;
      }
    }
    return {
      version: DRAFT_VERSION,
      pixels: value.pixels.slice(),
      alias: value.alias,
      selected: value.selected,
      tool: value.tool,
      parentId,
      paletteId: palette.id,
      paletteVersion: palette.version
    };
  }

  class EditorModel {
    constructor(options = {}) {
      this.historyLimit = Number.isInteger(options.historyLimit) && options.historyLimit > 0
        ? options.historyLimit
        : HISTORY_LIMIT;
      this.pixels = validPixels(options.pixels) ? options.pixels.slice() : new Array(PIXEL_COUNT).fill(0);
      this.selected = Number.isInteger(options.selected) && options.selected >= 0 && options.selected < COLOR_COUNT
        ? options.selected
        : 15;
      this.tool = validTool(options.tool) ? options.tool : "pencil";
      const palette = paletteByID(options.paletteId, options.paletteVersion) || DEFAULT_PALETTE;
      this.paletteId = palette.id;
      this.paletteVersion = palette.version;
      this.cursor = { x: 0, y: 0 };
      this.undoStack = [];
      this.redoStack = [];
      this.strokeBefore = null;
      this.lastPoint = null;
    }

    get canUndo() {
      return this.undoStack.length > 0;
    }

    get canRedo() {
      return this.redoStack.length > 0;
    }

    setSelected(value) {
      if (!Number.isInteger(value) || value < 0 || value >= COLOR_COUNT) {
        return false;
      }
      this.selected = value;
      return true;
    }

    setTool(value) {
      if (!validTool(value)) {
        return false;
      }
      this.tool = value;
      return true;
    }

    setPalette(id, version) {
      if (!validPaletteRef(id, version)) {
        return false;
      }
      this.paletteId = id;
      this.paletteVersion = version;
      return true;
    }

    setCursor(x, y) {
      if (!validCoordinate(x, WIDTH) || !validCoordinate(y, HEIGHT)) {
        return false;
      }
      this.cursor = { x, y };
      return true;
    }

    moveCursor(deltaX, deltaY) {
      const x = Math.max(0, Math.min(WIDTH - 1, this.cursor.x + deltaX));
      const y = Math.max(0, Math.min(HEIGHT - 1, this.cursor.y + deltaY));
      this.cursor = { x, y };
    }

    beginStroke(x, y) {
      if (!this.setCursor(x, y)) {
        return false;
      }
      if (this.tool === "eyedropper") {
        this.selected = this.pixels[indexFor(x, y)];
        return false;
      }
      if (this.tool === "fill") {
        const before = this.pixels.slice();
        floodFill(this.pixels, x, y, this.selected);
        return this.commit(before);
      }
      this.strokeBefore = this.pixels.slice();
      this.lastPoint = { x, y };
      this.paintLine(x, y, x, y);
      return true;
    }

    continueStroke(x, y) {
      if (this.strokeBefore === null || !this.setCursor(x, y)) {
        return false;
      }
      this.paintLine(this.lastPoint.x, this.lastPoint.y, x, y);
      this.lastPoint = { x, y };
      return true;
    }

    endStroke() {
      if (this.strokeBefore === null) {
        return false;
      }
      const before = this.strokeBefore;
      this.strokeBefore = null;
      this.lastPoint = null;
      return this.commit(before);
    }

    applyAt(x, y) {
      if (!this.setCursor(x, y)) {
        return false;
      }
      if (this.tool === "eyedropper") {
        this.selected = this.pixels[indexFor(x, y)];
        return false;
      }
      const before = this.pixels.slice();
      if (this.tool === "fill") {
        floodFill(this.pixels, x, y, this.selected);
      } else {
        this.pixels[indexFor(x, y)] = this.tool === "eraser" ? 0 : this.selected;
      }
      return this.commit(before);
    }

    paintLine(x0, y0, x1, y1) {
      const value = this.tool === "eraser" ? 0 : this.selected;
      linePoints(x0, y0, x1, y1).forEach((point) => {
        this.pixels[indexFor(point.x, point.y)] = value;
      });
    }

    clear() {
      const before = this.pixels.slice();
      this.pixels.fill(0);
      return this.commit(before);
    }

    flipHorizontal() {
      const before = this.pixels.slice();
      this.pixels = flippedHorizontally(this.pixels);
      return this.commit(before);
    }

    flipVertical() {
      const before = this.pixels.slice();
      this.pixels = flippedVertically(this.pixels);
      return this.commit(before);
    }

    undo() {
      if (!this.canUndo) {
        return false;
      }
      this.redoStack.push(this.pixels.slice());
      this.pixels = this.undoStack.pop();
      this.strokeBefore = null;
      this.lastPoint = null;
      return true;
    }

    redo() {
      if (!this.canRedo) {
        return false;
      }
      this.undoStack.push(this.pixels.slice());
      this.pixels = this.redoStack.pop();
      this.strokeBefore = null;
      this.lastPoint = null;
      return true;
    }

    commit(before) {
      if (equalPixels(before, this.pixels)) {
        return false;
      }
      this.undoStack.push(before);
      if (this.undoStack.length > this.historyLimit) {
        this.undoStack.shift();
      }
      this.redoStack = [];
      return true;
    }

    draft(alias, parentId) {
      const safeAlias = typeof alias === "string" && /^[A-Za-z0-9 _-]{0,16}$/.test(alias) ? alias : "";
      const safeParentId = Number.isSafeInteger(parentId) && parentId > 0 ? parentId : null;
      return {
        version: DRAFT_VERSION,
        pixels: this.pixels.slice(),
        alias: safeAlias,
        selected: this.selected,
        tool: this.tool,
        parentId: safeParentId,
        paletteId: this.paletteId,
        paletteVersion: this.paletteVersion
      };
    }
  }

  return Object.freeze({
    WIDTH,
    HEIGHT,
    PIXEL_COUNT,
    COLOR_COUNT,
    HISTORY_LIMIT,
    DRAFT_VERSION,
    TOOLS,
    PALETTE_CATALOG,
    DEFAULT_PALETTE,
    EditorModel,
    validPixels,
    validPaletteRef,
    paletteByID,
    normalizePaletteCatalog,
    normalizePostcard,
    normalizePublicStats,
    validateDraft,
    formatPostcardDate,
    validCursor,
    wallRequestPath,
    normalizeWallPage,
    normalizeDailyChallenge,
    dailyChallengePrompt,
    linePoints,
    floodFill,
    flippedHorizontally,
    flippedVertically
  });
});
