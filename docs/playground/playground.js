// xml2pdf playground: edits XML/JSON, renders with the Go engine compiled to
// WebAssembly (xml2pdf.wasm), and previews the PDF with PDF.js.

const PDFJS = "https://cdnjs.cloudflare.com/ajax/libs/pdf.js/4.10.38";
const STORAGE_KEY = "xml2pdf-playground";
const LONG_LINK = 8000;

const EXAMPLES = {
  hello: { xml: "examples/hello.xml" },
  invoice: { xml: "examples/invoice.xml", json: "examples/invoice.json" },
  loop: { xml: "examples/loop.xml", json: "examples/loop.json" },
  report: { xml: "examples/report.xml" },
};

const $ = (id) => document.getElementById(id);
const els = {
  xml: $("xml"), json: $("json"), status: $("status"), pages: $("pages"),
  error: $("error"), errorMsg: $("error-msg"), errorLine: $("error-line"),
  download: $("download"), share: $("share"), render: $("render"),
  example: $("example"), files: $("files"), filesCount: $("files-count"),
  upload: $("upload"), toast: $("toast"),
};

const files = new Map(); // name -> Uint8Array
let pdfjs = null;        // PDF.js module, or null if it failed to load
let lastPdf = null;      // Uint8Array of the latest good render
let renderSeq = 0;

// ---------------------------------------------------------------- storage

function save() {
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify({ xml: editors.xml.get(), json: editors.json.get() }));
  } catch (e) { /* storage unavailable: ignore */ }
}

function restore() {
  try {
    const s = JSON.parse(localStorage.getItem(STORAGE_KEY) || "null");
    if (s && typeof s.xml === "string") return s;
  } catch (e) { /* ignore */ }
  return null;
}

// ----------------------------------------------------------------- editor

// The playground talks to editors through this small interface so the
// CodeMirror upgrade (editor.js) and the plain-textarea fallback are
// interchangeable: get(), set(text), jump(line), setError(line, msg).
function textareaEditor(textarea) {
  return {
    get: () => textarea.value,
    set: (text) => setText(textarea, text),
    jump: (line) => jumpToLine(textarea, line),
    setError() {},
    focus: () => textarea.focus(),
  };
}
const editors = { xml: textareaEditor(els.xml), json: textareaEditor(els.json) };
globalThis.xml2pdfPlayground = editors;

// Replaces the textareas with CodeMirror. On failure the textareas stay.
async function upgradeEditors() {
  let mod;
  try {
    mod = await import("./editor.js");
  } catch (e) {
    console.warn("Code editor unavailable, using plain text areas", e);
    return;
  }
  for (const kind of ["xml", "json"]) {
    const textarea = els[kind];
    const box = textarea.parentNode;
    const cm = mod.createEditor(box, {
      doc: textarea.value,
      language: kind,
      onChange: scheduleRender,
      onRun: render,
      getVariables: kind === "xml" ? () => mod.variablesFromJSON(editors.json.get()) : undefined,
    });
    box.classList.add("cm-active");
    editors[kind] = {
      get: cm.getValue,
      set: cm.setValue,
      jump: (line) => { showTab(kind); cm.selectLine(line); },
      setError: cm.setError,
      focus: cm.focus,
      view: cm.view,
    };
  }
}

function updateGutter(textarea) {
  const gutter = textarea.parentNode.querySelector(".pg-gutter");
  const lines = textarea.value.split("\n").length;
  if (gutter.dataset.lines !== String(lines)) {
    let s = "";
    for (let i = 1; i <= lines; i++) s += i + "\n";
    gutter.textContent = s;
    gutter.dataset.lines = String(lines);
  }
  gutter.scrollTop = textarea.scrollTop;
}

function setupEditor(textarea) {
  textarea.addEventListener("scroll", () => updateGutter(textarea));
  textarea.addEventListener("input", () => { updateGutter(textarea); scheduleRender(); });
  textarea.addEventListener("keydown", (e) => {
    if (e.key === "Tab" && !e.shiftKey && !e.ctrlKey && !e.metaKey) {
      e.preventDefault();
      const { selectionStart: a, selectionEnd: b, value } = textarea;
      textarea.value = value.slice(0, a) + "  " + value.slice(b);
      textarea.selectionStart = textarea.selectionEnd = a + 2;
      textarea.dispatchEvent(new Event("input"));
    } else if (e.key === "Enter" && (e.ctrlKey || e.metaKey)) {
      e.preventDefault();
      render();
    }
  });
}

function setText(textarea, text) {
  textarea.value = text;
  textarea.scrollTop = 0;
  updateGutter(textarea);
}

function jumpToLine(textarea, line) {
  showTab(textarea === els.json ? "json" : "xml");
  const lines = textarea.value.split("\n");
  const n = Math.max(1, Math.min(line, lines.length));
  let start = 0;
  for (let i = 0; i < n - 1; i++) start += lines[i].length + 1;
  textarea.focus();
  textarea.setSelectionRange(start, start + lines[n - 1].length);
  const lineHeight = parseFloat(getComputedStyle(textarea).lineHeight) || 20;
  textarea.scrollTop = Math.max(0, (n - 4) * lineHeight);
  updateGutter(textarea);
}

function showTab(name) {
  for (const t of ["xml", "json", "files"]) {
    $("tab-" + t).setAttribute("aria-selected", String(t === name));
    $("panel-" + t).hidden = t !== name;
  }
  if (name !== "files" && !els[name].parentNode.classList.contains("cm-active")) updateGutter(els[name]);
}

// ----------------------------------------------------------------- images

function listFiles() {
  els.files.textContent = "";
  els.filesCount.textContent = files.size ? `(${files.size})` : "";
  if (!files.size) {
    const li = document.createElement("li");
    li.className = "empty";
    li.textContent = "No images yet.";
    els.files.append(li);
    return;
  }
  for (const [name, data] of [...files].sort()) {
    const li = document.createElement("li");
    const img = document.createElement("img");
    img.alt = "";
    img.src = URL.createObjectURL(new Blob([data]));
    img.onload = () => URL.revokeObjectURL(img.src);
    const code = document.createElement("code");
    code.textContent = name;
    const size = document.createElement("small");
    size.textContent = `${(data.length / 1024).toFixed(1)} KB`;
    const rm = document.createElement("button");
    rm.type = "button";
    rm.textContent = "Remove";
    rm.setAttribute("aria-label", `Remove ${name}`);
    rm.onclick = () => { files.delete(name); listFiles(); render(); };
    li.append(img, code, size, rm);
    els.files.append(li);
  }
}

async function addUploads(list) {
  for (const f of list) {
    const name = f.name.split(/[\\/]/).pop().replace(/[^\w.\-]/g, "_");
    files.set(name, new Uint8Array(await f.arrayBuffer()));
  }
  listFiles();
  toast(list.length === 1 ? `Added ${list[0].name}` : `Added ${list.length} images`);
  render();
}

// ------------------------------------------------------------------ render

let timer = 0;
function scheduleRender() {
  clearTimeout(timer);
  timer = setTimeout(render, 300);
}

function showError(message, line) {
  els.error.hidden = false;
  els.errorMsg.textContent = message;
  const isData = message.startsWith("data:");
  if (line && !isData) {
    els.errorLine.hidden = false;
    els.errorLine.textContent = `Go to line ${line}`;
    els.errorLine.onclick = () => editors.xml.jump(line);
  } else if (isData) {
    els.errorLine.hidden = false;
    els.errorLine.textContent = "Open data";
    els.errorLine.onclick = () => showTab("json");
  } else {
    els.errorLine.hidden = true;
  }
  editors.xml.setError(line && !isData ? line : null, message);
  els.pages.classList.add("stale");
  els.status.textContent = lastPdf ? "Showing the last successful render." : "Nothing rendered yet.";
}

function render() {
  clearTimeout(timer);
  save();
  if (!globalThis.xml2pdf) return;
  const seq = ++renderSeq;
  const res = globalThis.xml2pdf.render(editors.xml.get(), editors.json.get(), Object.fromEntries(files));
  if (!res.ok) {
    showError(res.error, res.line);
    return;
  }
  els.error.hidden = true;
  editors.xml.setError(null);
  lastPdf = res.pdf;
  els.download.disabled = false;
  showPdf(res.pdf, res.ms, seq);
}

async function showPdf(bytes, ms, seq) {
  const took = ms < 1 ? "under 1 ms" : `${Math.round(ms)} ms`;
  if (!pdfjs) {
    const frame = document.createElement("iframe");
    frame.title = "PDF preview";
    frame.src = URL.createObjectURL(new Blob([bytes], { type: "application/pdf" }));
    els.pages.replaceChildren(frame);
    els.pages.classList.remove("stale");
    els.status.textContent = `Rendered in ${took}.`;
    return;
  }
  try {
    const doc = await pdfjs.getDocument({ data: bytes.slice() }).promise;
    const width = Math.min(els.pages.clientWidth - 32, 820);
    const dpr = window.devicePixelRatio || 1;
    const canvases = [];
    for (let i = 1; i <= doc.numPages; i++) {
      const page = await doc.getPage(i);
      const base = page.getViewport({ scale: 1 });
      const scale = Math.max(width, 200) / base.width;
      const vp = page.getViewport({ scale: scale * dpr });
      const canvas = document.createElement("canvas");
      canvas.width = Math.floor(vp.width);
      canvas.height = Math.floor(vp.height);
      canvas.style.width = `${Math.floor(vp.width / dpr)}px`;
      canvas.setAttribute("role", "img");
      canvas.setAttribute("aria-label", `Page ${i} of ${doc.numPages}`);
      await page.render({ canvasContext: canvas.getContext("2d"), viewport: vp }).promise;
      if (seq !== renderSeq) return; // a newer render started
      canvases.push(canvas);
    }
    els.pages.replaceChildren(...canvases);
    els.pages.classList.remove("stale");
    els.status.textContent = `${doc.numPages} page${doc.numPages === 1 ? "" : "s"} · rendered in ${took}`;
    doc.destroy();
  } catch (e) {
    els.status.textContent = `Preview failed: ${e.message}`;
  }
}

// --------------------------------------------------------------- examples

async function fetchText(url) {
  const r = await fetch(url);
  if (!r.ok) throw new Error(`${url}: ${r.status}`);
  return r.text();
}

async function loadExample(name) {
  const ex = EXAMPLES[name];
  if (!ex) return;
  const [xml, json] = await Promise.all([fetchText(ex.xml), ex.json ? fetchText(ex.json) : ""]);
  editors.xml.set(xml);
  editors.json.set(json);
  showTab("xml");
  render();
}

// ------------------------------------------------------------------ share

function toBase64Url(bytes) {
  let s = "";
  for (let i = 0; i < bytes.length; i += 0x8000) s += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(s).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");
}

function fromBase64Url(s) {
  const bin = atob(s.replace(/-/g, "+").replace(/_/g, "/"));
  return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

async function streamBytes(bytes, transform) {
  const stream = new Blob([bytes]).stream().pipeThrough(transform);
  return new Uint8Array(await new Response(stream).arrayBuffer());
}

async function encodeState() {
  const raw = new TextEncoder().encode(JSON.stringify({ x: editors.xml.get(), j: editors.json.get() }));
  if (typeof CompressionStream === "function") {
    return "c=" + toBase64Url(await streamBytes(raw, new CompressionStream("deflate-raw")));
  }
  return "u=" + toBase64Url(raw);
}

async function decodeState(hash) {
  const m = /^#([cu])=(.+)$/.exec(hash);
  if (!m) return null;
  let bytes = fromBase64Url(m[2]);
  if (m[1] === "c") bytes = await streamBytes(bytes, new DecompressionStream("deflate-raw"));
  const s = JSON.parse(new TextDecoder().decode(bytes));
  return { xml: String(s.x ?? ""), json: String(s.j ?? "") };
}

async function share() {
  const hash = await encodeState();
  const url = `${location.origin}${location.pathname}#${hash}`;
  history.replaceState(null, "", "#" + hash);
  let copied = false;
  try {
    await navigator.clipboard.writeText(url);
    copied = true;
  } catch (e) { /* clipboard unavailable */ }
  let msg = copied ? "Link copied to clipboard." : "Link is in the address bar.";
  if (url.length > LONG_LINK) msg += " It's long, so some sites may cut it off.";
  if (files.size > 1 || (files.size === 1 && !files.has("logo.png"))) msg += " Uploaded images aren't included.";
  toast(msg);
}

let toastTimer = 0;
function toast(msg) {
  els.toast.textContent = msg;
  els.toast.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { els.toast.hidden = true; }, 3500);
}

// ---------------------------------------------------------------- startup

function download() {
  if (!lastPdf) return;
  const a = document.createElement("a");
  a.href = URL.createObjectURL(new Blob([lastPdf], { type: "application/pdf" }));
  a.download = "document.pdf";
  document.body.append(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(a.href), 1000);
}

async function startWasm() {
  const ready = new Promise((resolve) => { globalThis.onXml2pdfReady = resolve; });
  const go = new Go();
  let result;
  try {
    result = await WebAssembly.instantiateStreaming(fetch("xml2pdf.wasm"), go.importObject);
  } catch (e) {
    // Servers that don't send application/wasm need the slower path.
    const buf = await (await fetch("xml2pdf.wasm")).arrayBuffer();
    result = await WebAssembly.instantiate(buf, go.importObject);
  }
  go.run(result.instance);
  await ready;
}

async function loadPdfjs() {
  try {
    const mod = await import(`${PDFJS}/pdf.min.mjs`);
    mod.GlobalWorkerOptions.workerSrc = `${PDFJS}/pdf.worker.min.mjs`;
    return mod;
  } catch (e) {
    console.warn("PDF.js unavailable, using the browser's PDF viewer", e);
    return null;
  }
}

async function main() {
  setupEditor(els.xml);
  setupEditor(els.json);
  for (const t of ["xml", "json", "files"]) $("tab-" + t).onclick = () => showTab(t);
  els.render.onclick = render;
  els.download.onclick = download;
  els.share.onclick = () => share().catch((e) => toast(`Couldn't create a link: ${e.message}`));
  els.example.onchange = () => {
    const v = els.example.value;
    els.example.value = "";
    if (v) loadExample(v).catch((e) => toast(e.message));
  };
  els.upload.onchange = () => {
    if (els.upload.files.length) addUploads([...els.upload.files]);
    els.upload.value = "";
  };

  // Bundled logo used by the examples.
  const logo = fetch("examples/logo.png").then((r) => (r.ok ? r.arrayBuffer() : null)).then((b) => {
    if (b) files.set("logo.png", new Uint8Array(b));
    listFiles();
  }).catch(() => listFiles());

  // Initial document: shared link, then last session, then the hello example.
  let initial = null;
  if (location.hash.length > 3) {
    try { initial = await decodeState(location.hash); } catch (e) { toast("That share link is damaged."); }
  }
  if (!initial) initial = restore();
  if (initial) {
    editors.xml.set(initial.xml);
    editors.json.set(initial.json);
  } else {
    const [xml] = await Promise.all([fetchText(EXAMPLES.hello.xml).catch(() => "<document>\n  <page>Hello!</page>\n</document>\n")]);
    editors.xml.set(xml);
    editors.json.set("");
  }

  try {
    [pdfjs] = await Promise.all([loadPdfjs(), startWasm(), logo, upgradeEditors()]);
  } catch (e) {
    els.status.textContent = `Couldn't load the renderer: ${e.message}`;
    return;
  }
  els.status.textContent = `Renderer ready (xml2pdf ${globalThis.xml2pdf.version}).`;
  render();
}

main();
