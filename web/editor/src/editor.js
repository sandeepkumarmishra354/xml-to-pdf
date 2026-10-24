// CodeMirror 6 editor for the xml2pdf playground. Bundled by esbuild into
// docs/playground/editor.js; see web/editor/package.json.
import { basicSetup } from "codemirror";
import { EditorState, Prec } from "@codemirror/state";
import { EditorView, keymap, Decoration, MatchDecorator, ViewPlugin } from "@codemirror/view";
import { indentWithTab } from "@codemirror/commands";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags as t } from "@lezer/highlight";
import { xml, xmlLanguage } from "@codemirror/lang-xml";
import { json, jsonParseLinter } from "@codemirror/lang-json";
import { linter, lintGutter, setDiagnostics } from "@codemirror/lint";
import schema from "../schema.json";

// ------------------------------------------------------------------ schema

function attrSpecs(map) {
  return Object.entries(map).map(([name, values]) => (values.length ? { name, values } : { name }));
}

function xmlConfig() {
  const style = attrSpecs(schema.styleAttributes);
  const elements = Object.entries(schema.elements).map(([name, el]) => {
    const spec = { name, attributes: [...attrSpecs(el.attributes), ...(el.style ? style : [])] };
    if (el.children) spec.children = el.children;
    if (el.top) spec.top = true;
    return spec;
  });
  return { elements };
}

// lang-xml offers no value suggestions while the value is still empty
// (cursor between the quotes right after typing align="), which is exactly
// when they're most useful. This source covers that case from the schema;
// once a character is typed, lang-xml's own completion takes over.
function attrValuesFor(element, attr) {
  const el = schema.elements[element];
  if (!el) return null;
  return el.attributes[attr] || (el.style ? schema.styleAttributes[attr] : null) || null;
}

function emptyValueCompletion(context) {
  if (context.state.sliceDoc(context.pos, context.pos + 1) !== '"') return null;
  const before = context.state.sliceDoc(Math.max(0, context.pos - 500), context.pos);
  const m = /<([\w-]+)\b[^<>]*?\s([\w-]+)\s*=\s*"$/.exec(before);
  if (!m) return null;
  const values = attrValuesFor(m[1], m[2]);
  if (!values || !values.length) return null;
  return { from: context.pos, options: values.map((v) => ({ label: v, type: "enum" })) };
}

// --------------------------------------------------------------- variables

// Dotted paths into a JSON value, e.g. customer.name, items, items.0.price.
function jsonPaths(value, prefix = "", out = [], depth = 0) {
  if (depth > 5 || value === null || typeof value !== "object") return out;
  const entries = Array.isArray(value) ? (value.length ? [["0", value[0]]] : []) : Object.entries(value);
  for (const [k, v] of entries) {
    const path = prefix ? `${prefix}.${k}` : k;
    const kind = Array.isArray(v) ? "array" : v !== null && typeof v === "object" ? "object" : "value";
    out.push({ name: path, detail: kind === "value" ? String(v).slice(0, 30) : kind });
    jsonPaths(v, path, out, depth + 1);
  }
  return out;
}

export function variablesFromJSON(text) {
  try {
    return jsonPaths(JSON.parse(text || "{}"));
  } catch (e) {
    return [];
  }
}

function variablesFromXML(text) {
  const out = [];
  for (const m of text.matchAll(/<const\b[^>]*\bname\s*=\s*"([^"]+)"/g)) out.push({ name: m[1], detail: "constant" });
  for (const m of text.matchAll(/<for-each\b([^>]*)>/g)) {
    const attrs = m[1];
    const v = /\bvar\s*=\s*"([^"]+)"/.exec(attrs);
    out.push({ name: v ? v[1] : "item", detail: "loop variable" });
    for (const k of ["index", "number"]) {
      const a = new RegExp(`\\b${k}\\s*=\\s*"([^"]+)"`).exec(attrs);
      if (a) out.push({ name: a[1], detail: `loop ${k}` });
    }
  }
  return out;
}

function variableCompletion(getVariables) {
  return (context) => {
    const m = context.matchBefore(/\$\{[\w.\-]*/);
    if (!m) return null;
    const seen = new Set();
    const vars = [...variablesFromXML(context.state.doc.toString()), ...(getVariables ? getVariables() : [])]
      .filter((v) => !seen.has(v.name) && seen.add(v.name));
    return {
      from: m.from + 2,
      validFor: /^[\w.\-]*$/,
      options: vars.map((v) => ({
        label: v.name,
        detail: v.detail,
        type: "variable",
        apply: (view, completion, from, to) => {
          const close = view.state.sliceDoc(to, to + 1) === "}" ? "" : "}";
          view.dispatch({
            changes: { from, to, insert: completion.label + close },
            selection: { anchor: from + completion.label.length + close.length },
          });
        },
      })),
    };
  };
}

// -------------------------------------------------------------- appearance

const highlight = HighlightStyle.define([
  { tag: [t.tagName, t.angleBracket], color: "var(--tag)" },
  { tag: t.attributeName, color: "var(--attr)" },
  { tag: [t.attributeValue, t.string], color: "var(--str)" },
  { tag: [t.comment, t.processingInstruction, t.meta], color: "var(--comment)", fontStyle: "italic" },
  { tag: t.propertyName, color: "var(--attr)" },
  { tag: [t.number, t.bool, t.null], color: "var(--var)" },
  { tag: t.invalid, color: "#d1242f" },
]);

// Emphasize ${variables} and {page}/{pages} wherever they appear.
const varMarker = new MatchDecorator({
  regexp: /\$\{[^}\n]*\}|\{pages?\}/g,
  decoration: Decoration.mark({ class: "cm-x2p-var" }),
});
const varHighlighter = ViewPlugin.fromClass(
  class {
    constructor(view) { this.decorations = varMarker.createDeco(view); }
    update(u) { this.decorations = varMarker.updateDeco(u, this.decorations); }
  },
  { decorations: (v) => v.decorations },
);

const theme = EditorView.theme({
  "&": { height: "100%", color: "var(--fg)", backgroundColor: "var(--bg-code)", fontSize: "13px" },
  ".cm-scroller": {
    fontFamily: 'ui-monospace, SFMono-Regular, Menlo, Consolas, "Liberation Mono", monospace',
    lineHeight: "20px",
  },
  ".cm-content": { caretColor: "var(--fg)", padding: "8px 0" },
  "&.cm-focused": { outline: "none" },
  "&.cm-focused .cm-cursor": { borderLeftColor: "var(--fg)" },
  ".cm-gutters": { backgroundColor: "var(--bg-code)", color: "var(--comment)", borderRight: "1px solid var(--border)" },
  ".cm-activeLine": { backgroundColor: "color-mix(in srgb, var(--accent) 7%, transparent)" },
  ".cm-activeLineGutter": { backgroundColor: "color-mix(in srgb, var(--accent) 12%, transparent)", color: "var(--fg)" },
  "&.cm-focused > .cm-scroller > .cm-selectionLayer .cm-selectionBackground, .cm-selectionBackground, ::selection": {
    backgroundColor: "color-mix(in srgb, var(--accent) 28%, transparent) !important",
  },
  ".cm-matchingBracket, .cm-nonmatchingBracket": { backgroundColor: "color-mix(in srgb, var(--accent) 20%, transparent)", outline: "none" },
  ".cm-x2p-var": { color: "var(--var)", fontWeight: "600" },
  ".cm-tooltip": { backgroundColor: "var(--bg)", color: "var(--fg)", border: "1px solid var(--border)", borderRadius: "6px" },
  ".cm-tooltip-autocomplete > ul": { fontFamily: "inherit", maxHeight: "16em" },
  ".cm-tooltip-autocomplete > ul > li[aria-selected]": { backgroundColor: "var(--accent)", color: "var(--bg)" },
  ".cm-completionDetail": { color: "var(--fg-muted)", fontStyle: "normal", marginLeft: "1em" },
  ".cm-tooltip-autocomplete > ul > li[aria-selected] .cm-completionDetail": { color: "inherit", opacity: "0.8" },
  ".cm-panels": { backgroundColor: "var(--bg-soft)", color: "var(--fg)" },
  ".cm-panels.cm-panels-bottom": { borderTop: "1px solid var(--border)" },
  ".cm-panel input, .cm-panel button": { color: "var(--fg)" },
  ".cm-searchMatch": { backgroundColor: "color-mix(in srgb, #f0c36d 45%, transparent)" },
  ".cm-foldPlaceholder": { backgroundColor: "var(--bg)", border: "1px solid var(--border)", color: "var(--fg-muted)" },
  ".cm-lintRange-error": { backgroundImage: "none", textDecoration: "underline wavy #d1242f", textUnderlineOffset: "3px" },
});

// ----------------------------------------------------------------- editor

/**
 * Creates an editor inside parent.
 * options: { doc, language: "xml" | "json", onChange, onRun, getVariables }
 */
export function createEditor(parent, options) {
  const { language = "xml", onChange, onRun, getVariables } = options;
  const extensions = [
    basicSetup,
    // Highest precedence so Mod-Enter renders instead of the default
    // keymap's "insert blank line".
    Prec.highest(keymap.of([
      { key: "Mod-Enter", preventDefault: true, run: () => { if (onRun) onRun(); return true; } },
    ])),
    keymap.of([indentWithTab]),
    EditorState.tabSize.of(2),
    theme,
    syntaxHighlighting(highlight),
    varHighlighter,
    lintGutter(),
    EditorView.updateListener.of((u) => { if (u.docChanged && onChange) onChange(); }),
    EditorView.contentAttributes.of({ "aria-label": language === "xml" ? "XML document" : "JSON data" }),
  ];
  if (language === "xml") {
    extensions.push(
      xml(xmlConfig()),
      xmlLanguage.data.of({ autocomplete: variableCompletion(getVariables) }),
      xmlLanguage.data.of({ autocomplete: emptyValueCompletion }),
    );
  } else {
    extensions.push(json(), linter(jsonParseLinter(), { delay: 300 }));
  }

  const view = new EditorView({
    parent,
    state: EditorState.create({ doc: options.doc || "", extensions }),
  });

  return {
    view,
    getValue: () => view.state.doc.toString(),
    setValue(text) {
      // A fresh state resets undo history, folds and scroll position.
      view.setState(EditorState.create({ doc: text, extensions }));
    },
    focus: () => view.focus(),
    selectLine(n) {
      const line = view.state.doc.line(Math.max(1, Math.min(n, view.state.doc.lines)));
      view.dispatch({
        selection: { anchor: line.from, head: line.to },
        effects: EditorView.scrollIntoView(line.from, { y: "center" }),
      });
      view.focus();
    },
    setError(lineNo, message) {
      const diagnostics = [];
      if (lineNo) {
        const line = view.state.doc.line(Math.max(1, Math.min(lineNo, view.state.doc.lines)));
        diagnostics.push({ from: line.from, to: Math.max(line.to, line.from), severity: "error", message });
      }
      view.dispatch(setDiagnostics(view.state, diagnostics));
    },
  };
}
