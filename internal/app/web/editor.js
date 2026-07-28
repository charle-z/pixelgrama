"use strict";

((root, factory) => {
  const editor = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = editor;
  }
  root.PixelgramaEditor = editor;
})(typeof globalThis === "object" ? globalThis : this, () => {
  const WIDTH = 16;
  const HEIGHT = 16;
  const PIXEL_COUNT = WIDTH * HEIGHT;
  const COLOR_COUNT = 16;
  const HISTORY_LIMIT = 64;
  const DRAFT_VERSION = 2;
  const TOOLS = Object.freeze(["pencil", "eraser", "fill", "eyedropper"]);

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

  function validateDraft(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) {
      return null;
    }
    if ((value.version !== 1 && value.version !== DRAFT_VERSION) || !validPixels(value.pixels)) {
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
    if (value.version === DRAFT_VERSION && value.parentId !== null && value.parentId !== undefined) {
      if (!Number.isSafeInteger(value.parentId) || value.parentId < 1) {
        return null;
      }
      parentId = value.parentId;
    }
    return {
      version: DRAFT_VERSION,
      pixels: value.pixels.slice(),
      alias: value.alias,
      selected: value.selected,
      tool: value.tool,
      parentId
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
        parentId: safeParentId
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
    EditorModel,
    validPixels,
    validateDraft,
    formatPostcardDate,
    linePoints,
    floodFill,
    flippedHorizontally,
    flippedVertically
  });
});
