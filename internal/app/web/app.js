"use strict";

(() => {
  const palette = [
    "#000000", "#0000AA", "#00AA00", "#00AAAA",
    "#AA0000", "#AA00AA", "#AA5500", "#AAAAAA",
    "#555555", "#5555FF", "#55FF55", "#55FFFF",
    "#FF5555", "#FF55FF", "#FFFF55", "#FFFFFF"
  ];
  const editor = document.getElementById("editor");
  const context = editor.getContext("2d", { alpha: false });
  const paletteNode = document.getElementById("palette");
  const aliasNode = document.getElementById("alias");
  const statusNode = document.getElementById("status");
  const wallNode = document.getElementById("wall");
  const wallStateNode = document.getElementById("wall-state");
  const loadMoreNode = document.getElementById("load-more");
  const pixels = new Array(256).fill(0);
  let selected = 15;
  let drawing = false;
  let page = 1;
  let language = "es";

  const messages = {
    ready: { es: "LISTO", en: "READY" },
    publishing: { es: "PUBLICANDO...", en: "PUBLISHING..." },
    published: { es: "POSTAL PUBLICADA", en: "POSTCARD PUBLISHED" },
    invalidAlias: { es: "ALIAS INVÁLIDO", en: "INVALID ALIAS" },
    wallError: { es: "ERROR AL CARGAR EL MURO", en: "WALL LOAD ERROR" },
    empty: { es: "AÚN NO HAY POSTALES", en: "NO POSTCARDS YET" },
    loaded: { es: "MURO ACTUALIZADO", en: "WALL UPDATED" }
  };

  function translated(key) {
    return messages[key][language];
  }

  function setStatus(key) {
    statusNode.textContent = translated(key);
  }

  function applyLanguage(nextLanguage) {
    language = nextLanguage;
    document.documentElement.lang = language;
    document.querySelectorAll("[data-es][data-en]").forEach((node) => {
      node.textContent = node.dataset[language];
    });
    document.getElementById("lang-es").setAttribute("aria-pressed", String(language === "es"));
    document.getElementById("lang-en").setAttribute("aria-pressed", String(language === "en"));
  }

  function drawEditor() {
    const cell = editor.width / 16;
    for (let index = 0; index < pixels.length; index += 1) {
      const x = (index % 16) * cell;
      const y = Math.floor(index / 16) * cell;
      context.fillStyle = palette[pixels[index]];
      context.fillRect(x, y, cell, cell);
    }
    context.strokeStyle = "#555555";
    context.lineWidth = 1;
    for (let coordinate = 0; coordinate <= 16; coordinate += 1) {
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
  }

  function paint(event) {
    const bounds = editor.getBoundingClientRect();
    const x = Math.floor(((event.clientX - bounds.left) / bounds.width) * 16);
    const y = Math.floor(((event.clientY - bounds.top) / bounds.height) * 16);
    if (x < 0 || x > 15 || y < 0 || y > 15) {
      return;
    }
    pixels[y * 16 + x] = selected;
    drawEditor();
  }

  function buildPalette() {
    palette.forEach((color, index) => {
      const button = document.createElement("button");
      button.type = "button";
      button.style.backgroundColor = color;
      button.dataset.selected = String(index === selected);
      button.setAttribute("aria-label", `VGA ${index}: ${color}`);
      button.addEventListener("click", () => {
        selected = index;
        paletteNode.querySelectorAll("button").forEach((item, itemIndex) => {
          item.dataset.selected = String(itemIndex === selected);
        });
      });
      paletteNode.append(button);
    });
  }

  function validPixels(value) {
    return Array.isArray(value) && value.length === 256 && value.every((item) => Number.isInteger(item) && item >= 0 && item <= 15);
  }

  function drawPostcard(item) {
    if (!item || !validPixels(item.pixels)) {
      return;
    }
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
    const commit = typeof item.commit === "string" ? item.commit.slice(0, 12) : "unknown";
    const created = typeof item.created_at === "string" ? item.created_at : "";
    meta.textContent = `#${Number(item.id) || 0} · ${commit} · ${created}`;
    article.append(canvas, alias, meta);
    wallNode.append(article);
  }

  async function loadWall(reset) {
    if (reset) {
      page = 1;
      wallNode.replaceChildren();
    }
    wallStateNode.textContent = language === "es" ? "CARGANDO" : "LOADING";
    try {
      const response = await fetch(`/wall?format=json&page=${page}&limit=24`, {
        headers: { Accept: "application/json" }
      });
      if (!response.ok) {
        throw new Error("wall");
      }
      const payload = await response.json();
      const postcards = Array.isArray(payload.postcards) ? payload.postcards : [];
      postcards.forEach(drawPostcard);
      wallStateNode.textContent = postcards.length === 0 && page === 1 ? translated("empty") : translated("loaded");
      loadMoreNode.hidden = postcards.length < 24;
    } catch (error) {
      wallStateNode.textContent = translated("wallError");
      loadMoreNode.hidden = true;
    }
  }

  async function publish() {
    const alias = aliasNode.value;
    if (!/^[A-Za-z0-9 _-]{0,16}$/.test(alias)) {
      setStatus("invalidAlias");
      return;
    }
    setStatus("publishing");
    const payload = { pixels: pixels.slice() };
    if (alias.length > 0) {
      payload.alias = alias;
    }
    try {
      const response = await fetch("/postcard", {
        method: "POST",
        headers: { "Content-Type": "application/json", Accept: "application/json" },
        body: JSON.stringify(payload)
      });
      const result = await response.json();
      if (!response.ok) {
        statusNode.textContent = typeof result.message === "string" ? result.message : `HTTP ${response.status}`;
        return;
      }
      setStatus("published");
      await loadWall(true);
    } catch (error) {
      statusNode.textContent = "NETWORK ERROR";
    }
  }

  editor.addEventListener("pointerdown", (event) => {
    drawing = true;
    editor.setPointerCapture(event.pointerId);
    paint(event);
  });
  editor.addEventListener("pointermove", (event) => {
    if (drawing) {
      paint(event);
    }
  });
  editor.addEventListener("pointerup", () => {
    drawing = false;
  });
  editor.addEventListener("pointercancel", () => {
    drawing = false;
  });
  document.getElementById("clear").addEventListener("click", () => {
    pixels.fill(0);
    drawEditor();
    setStatus("ready");
  });
  document.getElementById("publish").addEventListener("click", publish);
  document.getElementById("lang-es").addEventListener("click", () => applyLanguage("es"));
  document.getElementById("lang-en").addEventListener("click", () => applyLanguage("en"));
  loadMoreNode.addEventListener("click", async () => {
    page += 1;
    await loadWall(false);
  });

  buildPalette();
  drawEditor();
  applyLanguage("es");
  loadWall(true);
})();
