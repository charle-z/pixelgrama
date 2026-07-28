"use strict";

(() => {
  const {
    WIDTH,
    HEIGHT,
    DRAFT_VERSION,
    EditorModel,
    validateDraft,
    validPixels,
    formatPostcardDate,
    wallRequestPath,
    normalizeWallPage
  } = globalThis.PixelgramaEditor;
  const palette = [
    "#000000", "#0000AA", "#00AA00", "#00AAAA",
    "#AA0000", "#AA00AA", "#AA5500", "#AAAAAA",
    "#555555", "#5555FF", "#55FF55", "#55FFFF",
    "#FF5555", "#FF55FF", "#FFFF55", "#FFFFFF"
  ];
  const DRAFT_KEY = "pixelgrama:draft";
  const LANGUAGE_KEY = "pixelgrama:language";
  const editor = document.getElementById("editor");
  const context = editor.getContext("2d", { alpha: false });
  const paletteNode = document.getElementById("palette");
  const aliasNode = document.getElementById("alias");
  const statusNode = document.getElementById("status");
  const wallNode = document.getElementById("wall");
  const wallStateNode = document.getElementById("wall-state");
  const loadMoreNode = document.getElementById("load-more");
  const publishNode = document.getElementById("publish");
  const undoNode = document.getElementById("undo");
  const redoNode = document.getElementById("redo");
  const toolNodes = Array.from(document.querySelectorAll("[data-tool]"));
  let nextBeforeID = null;
  const renderedPostcardIDs = new Set();
  let drawing = false;
  let publishing = false;
  let currentStatus = "ready";
  let currentWallStatus = "loading";
  let remixParentID = null;

  const messages = {
    ready: { es: "LISTO", en: "READY" },
    draftRestored: { es: "BORRADOR RESTAURADO", en: "DRAFT RESTORED" },
    draftDiscarded: { es: "BORRADOR INVÁLIDO DESCARTADO", en: "INVALID DRAFT DISCARDED" },
    remixLoaded: { es: "REMIX CARGADO", en: "REMIX LOADED" },
    remixError: { es: "REMIX NO DISPONIBLE", en: "REMIX UNAVAILABLE" },
    invalidParent: { es: "POSTAL PADRE NO DISPONIBLE", en: "PARENT POSTCARD UNAVAILABLE" },
    publishing: { es: "PUBLICANDO...", en: "PUBLISHING..." },
    published: { es: "POSTAL PUBLICADA", en: "POSTCARD PUBLISHED" },
    invalidAlias: { es: "ALIAS INVÁLIDO", en: "INVALID ALIAS" },
    publishError: { es: "NO SE PUDO PUBLICAR", en: "POSTCARD COULD NOT BE PUBLISHED" },
    duplicate: { es: "POSTAL DUPLICADA", en: "DUPLICATE POSTCARD" },
    rateLimited: { es: "LÍMITE DE PUBLICACIÓN ALCANZADO", en: "PUBLISH RATE LIMIT REACHED" },
    networkError: { es: "ERROR DE RED", en: "NETWORK ERROR" },
    wallError: { es: "ERROR AL CARGAR EL MURO", en: "WALL LOAD ERROR" },
    empty: { es: "AÚN NO HAY POSTALES", en: "NO POSTCARDS YET" },
    loaded: { es: "MURO ACTUALIZADO", en: "WALL UPDATED" },
    loading: { es: "CARGANDO", en: "LOADING" }
  };

  function storageGet(key) {
    try {
      return localStorage.getItem(key);
    } catch (error) {
      return null;
    }
  }

  function storageSet(key, value) {
    try {
      localStorage.setItem(key, value);
    } catch (error) {
      return;
    }
  }

  function storageRemove(key) {
    try {
      localStorage.removeItem(key);
    } catch (error) {
      return;
    }
  }

  function loadDraft() {
    const raw = storageGet(DRAFT_KEY);
    if (raw === null) {
      return { draft: null, invalid: false };
    }
    try {
      const draft = validateDraft(JSON.parse(raw));
      if (draft !== null) {
        return { draft, invalid: false };
      }
    } catch (error) {
      storageRemove(DRAFT_KEY);
      return { draft: null, invalid: true };
    }
    storageRemove(DRAFT_KEY);
    return { draft: null, invalid: true };
  }

  const savedDraft = loadDraft();
  let model = new EditorModel(savedDraft.draft || {});
  if (savedDraft.draft !== null) {
    aliasNode.value = savedDraft.draft.alias;
    remixParentID = savedDraft.draft.parentId;
    currentStatus = "draftRestored";
  } else if (savedDraft.invalid) {
    currentStatus = "draftDiscarded";
  }
  const storedLanguage = storageGet(LANGUAGE_KEY);
  let language = storedLanguage === "en" ? "en" : "es";

  function translated(key) {
    return messages[key][language];
  }

  function setStatus(key) {
    currentStatus = key;
    statusNode.textContent = translated(key);
  }

  function setWallStatus(key) {
    currentWallStatus = key;
    wallStateNode.textContent = translated(key);
  }

  function applyLanguage(nextLanguage, persist) {
    language = nextLanguage === "en" ? "en" : "es";
    if (persist) {
      storageSet(LANGUAGE_KEY, language);
    }
    document.documentElement.lang = language;
    document.querySelectorAll("[data-es][data-en]").forEach((node) => {
      node.textContent = node.dataset[language];
    });
    editor.setAttribute("aria-label", language === "es" ? editor.dataset.labelEs : editor.dataset.labelEn);
    document.getElementById("lang-es").setAttribute("aria-pressed", String(language === "es"));
    document.getElementById("lang-en").setAttribute("aria-pressed", String(language === "en"));
    statusNode.textContent = translated(currentStatus);
    wallStateNode.textContent = translated(currentWallStatus);
  }

  function persistDraft() {
    storageSet(DRAFT_KEY, JSON.stringify(model.draft(aliasNode.value, remixParentID)));
  }

  function updateControls() {
    paletteNode.querySelectorAll("button").forEach((item, index) => {
      item.dataset.selected = String(index === model.selected);
    });
    toolNodes.forEach((node) => {
      node.setAttribute("aria-pressed", String(node.dataset.tool === model.tool));
    });
    undoNode.disabled = !model.canUndo;
    redoNode.disabled = !model.canRedo;
    publishNode.disabled = publishing;
  }

  function drawEditor() {
    const cell = editor.width / WIDTH;
    for (let index = 0; index < model.pixels.length; index += 1) {
      const x = (index % WIDTH) * cell;
      const y = Math.floor(index / WIDTH) * cell;
      context.fillStyle = palette[model.pixels[index]];
      context.fillRect(x, y, cell, cell);
    }
    context.strokeStyle = "#555555";
    context.lineWidth = 1;
    for (let coordinate = 0; coordinate <= WIDTH; coordinate += 1) {
      const position = coordinate * cell + 0.5;
      context.beginPath();
      context.moveTo(position, 0);
      context.lineTo(position, editor.height);
      context.stroke();
      context.beginPath();
      context.moveTo(0, position);
      context.lineTo(editor.width, position);
      context.stroke();
    }
    if (document.activeElement === editor) {
      context.strokeStyle = "#FFFF55";
      context.lineWidth = 3;
      context.strokeRect(model.cursor.x * cell + 2, model.cursor.y * cell + 2, cell - 4, cell - 4);
    }
  }

  function editorChanged(save) {
    drawEditor();
    updateControls();
    if (save) {
      persistDraft();
    }
  }

  function pointFromEvent(event) {
    const bounds = editor.getBoundingClientRect();
    const x = Math.floor(((event.clientX - bounds.left) / bounds.width) * WIDTH);
    const y = Math.floor(((event.clientY - bounds.top) / bounds.height) * HEIGHT);
    if (x < 0 || x >= WIDTH || y < 0 || y >= HEIGHT) {
      return null;
    }
    return { x, y };
  }

  function buildPalette() {
    palette.forEach((color, index) => {
      const button = document.createElement("button");
      button.type = "button";
      button.style.backgroundColor = color;
      button.dataset.selected = String(index === model.selected);
      button.setAttribute("aria-label", "VGA " + index + ": " + color);
      button.addEventListener("click", () => {
        model.setSelected(index);
        updateControls();
        persistDraft();
      });
      paletteNode.append(button);
    });
  }

  function drawPostcard(item) {
    const id = item && Number(item.id);
    if (!item || !validPixels(item.pixels) || !Number.isSafeInteger(id) || id < 1 || renderedPostcardIDs.has(id)) {
      return false;
    }
    renderedPostcardIDs.add(id);
    const article = document.createElement("article");
    article.className = "postcard";
    const canvas = document.createElement("canvas");
    canvas.width = 256;
    canvas.height = 256;
    const postcardContext = canvas.getContext("2d", { alpha: false });
    const cell = 16;
    item.pixels.forEach((value, index) => {
      postcardContext.fillStyle = palette[value];
      postcardContext.fillRect((index % 16) * cell, Math.floor(index / 16) * cell, cell, cell);
    });
    const alias = document.createElement("p");
    alias.className = "alias";
    alias.textContent = typeof item.alias === "string" && item.alias.length > 0 ? item.alias : "ANON";
    const meta = document.createElement("p");
    meta.className = "meta";
    meta.textContent = formatPostcardDate(item.created_at, language);
    const share = document.createElement("a");
    share.className = "share-link";
    share.href = "/p/" + id;
    share.dataset.es = "ABRIR / REMIX";
    share.dataset.en = "OPEN / REMIX";
    share.textContent = share.dataset[language];
    article.append(canvas, alias, meta, share);
    wallNode.append(article);
    return true;
  }

  async function loadWall(reset) {
    if (reset) {
      nextBeforeID = null;
      renderedPostcardIDs.clear();
      wallNode.replaceChildren();
    }
    setWallStatus("loading");
    try {
      const response = await fetch(wallRequestPath(nextBeforeID, 24), {
        headers: { Accept: "application/json" }
      });
      if (!response.ok) {
        throw new Error("wall");
      }
      const payload = normalizeWallPage(await response.json());
      if (payload === null) {
        throw new Error("wall payload");
      }
      payload.postcards.forEach(drawPostcard);
      nextBeforeID = payload.nextBeforeID;
      setWallStatus(renderedPostcardIDs.size === 0 ? "empty" : "loaded");
      loadMoreNode.hidden = nextBeforeID === null;
    } catch (error) {
      setWallStatus("wallError");
      loadMoreNode.hidden = true;
    }
  }

  async function publish() {
    if (publishing) {
      return;
    }
    const alias = aliasNode.value;
    if (!/^[A-Za-z0-9 _-]{0,16}$/.test(alias)) {
      setStatus("invalidAlias");
      return;
    }
    publishing = true;
    updateControls();
    setStatus("publishing");
    const payload = { pixels: model.pixels.slice() };
    if (alias.length > 0) {
      payload.alias = alias;
    }
    if (remixParentID !== null) {
      payload.parent_id = remixParentID;
    }
    try {
      const response = await fetch("/postcard", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify(payload)
      });
      const result = await response.json().catch(() => ({}));
      if (!response.ok) {
        if (result.error === "duplicate_postcard") {
          setStatus("duplicate");
        } else if (result.error === "rate_limited") {
          setStatus("rateLimited");
        } else if (result.error === "invalid_parent") {
          setStatus("invalidParent");
        } else {
          setStatus("publishError");
        }
        return;
      }
      storageRemove(DRAFT_KEY);
      remixParentID = null;
      window.history.replaceState(null, "", "/wall");
      setStatus("published");
      await loadWall(true);
    } catch (error) {
      setStatus("networkError");
    } finally {
      publishing = false;
      updateControls();
    }
  }

  async function loadRemix() {
    const parameters = new URLSearchParams(window.location.search);
    const rawID = parameters.get("remix");
    if (rawID === null) {
      return;
    }
    if (!/^[1-9][0-9]*$/.test(rawID)) {
      remixParentID = null;
      setStatus("remixError");
      return;
    }
    const remixID = Number(rawID);
    if (!Number.isSafeInteger(remixID)) {
      remixParentID = null;
      setStatus("remixError");
      return;
    }
    try {
      const response = await fetch("/p/" + remixID + ".json", {
        headers: { Accept: "application/json" }
      });
      if (!response.ok) {
        throw new Error("remix");
      }
      const item = await response.json();
      if (!item || item.id !== remixID || !validPixels(item.pixels)
        || item.format_version !== 1 || item.palette_id !== "vga16"
        || typeof item.content_hash !== "string" || !/^[0-9a-f]{64}$/.test(item.content_hash)) {
        throw new Error("remix");
      }
      model = new EditorModel({ pixels: item.pixels, selected: model.selected, tool: "pencil" });
      aliasNode.value = "";
      remixParentID = remixID;
      setStatus("remixLoaded");
      editorChanged(true);
    } catch (error) {
      remixParentID = null;
      setStatus("remixError");
    }
  }

  editor.addEventListener("pointerdown", (event) => {
    const point = pointFromEvent(event);
    if (point === null) {
      return;
    }
    editor.focus();
    model.beginStroke(point.x, point.y);
    drawing = model.tool === "pencil" || model.tool === "eraser";
    if (drawing) {
      editor.setPointerCapture(event.pointerId);
    }
    editorChanged(!drawing);
  });
  editor.addEventListener("pointermove", (event) => {
    if (!drawing) {
      return;
    }
    const point = pointFromEvent(event);
    if (point !== null) {
      model.continueStroke(point.x, point.y);
      editorChanged(false);
    }
  });
  function finishStroke() {
    if (!drawing) {
      return;
    }
    drawing = false;
    model.endStroke();
    editorChanged(true);
  }
  editor.addEventListener("pointerup", finishStroke);
  editor.addEventListener("pointercancel", finishStroke);
  editor.addEventListener("focus", drawEditor);
  editor.addEventListener("blur", drawEditor);
  editor.addEventListener("keydown", (event) => {
    const key = event.key.toLowerCase();
    if ((event.ctrlKey || event.metaKey) && key === "z") {
      event.preventDefault();
      if (event.shiftKey) model.redo(); else model.undo();
      editorChanged(true);
      return;
    }
    if ((event.ctrlKey || event.metaKey) && key === "y") {
      event.preventDefault();
      model.redo();
      editorChanged(true);
      return;
    }
    const moves = {
      arrowleft: [-1, 0],
      arrowright: [1, 0],
      arrowup: [0, -1],
      arrowdown: [0, 1]
    };
    if (moves[key]) {
      event.preventDefault();
      model.moveCursor(moves[key][0], moves[key][1]);
      drawEditor();
      return;
    }
    const tools = { p: "pencil", e: "eraser", f: "fill", i: "eyedropper" };
    if (tools[key]) {
      event.preventDefault();
      model.setTool(tools[key]);
      editorChanged(true);
      return;
    }
    if (/^[0-9a-f]$/.test(key) && !event.ctrlKey && !event.metaKey) {
      event.preventDefault();
      model.setSelected(parseInt(key, 16));
      editorChanged(true);
      return;
    }
    if (event.key === "[" || event.key === "]") {
      event.preventDefault();
      const delta = event.key === "[" ? -1 : 1;
      model.setSelected((model.selected + delta + palette.length) % palette.length);
      editorChanged(true);
      return;
    }
    if (event.key === " " || event.key === "Enter") {
      event.preventDefault();
      model.applyAt(model.cursor.x, model.cursor.y);
      editorChanged(true);
    }
  });

  toolNodes.forEach((node) => {
    node.addEventListener("click", () => {
      model.setTool(node.dataset.tool);
      editorChanged(true);
    });
  });
  undoNode.addEventListener("click", () => {
    model.undo();
    editorChanged(true);
  });
  redoNode.addEventListener("click", () => {
    model.redo();
    editorChanged(true);
  });
  document.getElementById("flip-horizontal").addEventListener("click", () => {
    model.flipHorizontal();
    editorChanged(true);
  });
  document.getElementById("flip-vertical").addEventListener("click", () => {
    model.flipVertical();
    editorChanged(true);
  });
  document.getElementById("clear").addEventListener("click", () => {
    model.clear();
    editorChanged(true);
    setStatus("ready");
  });
  publishNode.addEventListener("click", publish);
  aliasNode.addEventListener("input", persistDraft);
  document.getElementById("lang-es").addEventListener("click", async () => {
    applyLanguage("es", true);
    await loadWall(true);
  });
  document.getElementById("lang-en").addEventListener("click", async () => {
    applyLanguage("en", true);
    await loadWall(true);
  });
  loadMoreNode.addEventListener("click", async () => {
    if (nextBeforeID !== null) {
      await loadWall(false);
    }
  });

  buildPalette();
  applyLanguage(language, false);
  drawEditor();
  updateControls();
  loadRemix();
  loadWall(true);
})();
