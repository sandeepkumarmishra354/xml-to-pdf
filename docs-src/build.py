#!/usr/bin/env python3
"""Builds the static documentation site in docs/ from the fragments in docs-src/.

Each fragment is plain HTML for the page body, with two conveniences:

  ```xml            fenced code blocks (xml, sh, go, json, text); the content
  <tag>...</tag>    is HTML-escaped, so code can be pasted verbatim
  ```

  {{file:examples/invoice.xml}}   inside a fence, inserts that file's contents

Run `make docs` (or `python3 docs-src/build.py`) after editing.
"""
import html
import pathlib
import re

ROOT = pathlib.Path(__file__).resolve().parent.parent
SRC = ROOT / "docs-src"
OUT = ROOT / "docs"
REPO = "https://github.com/sandeepkumarmishra354/xml-to-pdf"

# (file, title, sidebar sub-links)
PAGES = [
    ("index.html", "Overview", []),
    ("guide.html", "Guide", [
        ("structure", "Document structure"),
        ("units", "Units"),
        ("pages", "Page setup"),
        ("layout", "Layout & pagination"),
        ("text", "Text & fonts"),
        ("styles", "Styles"),
        ("colors", "Colors"),
        ("variables", "Variables"),
        ("loops", "Loops"),
        ("bands", "Headers & footers"),
        ("watermarks", "Watermarks"),
        ("page-selectors", "Page selectors"),
        ("errors", "Errors"),
        ("limitations", "Limitations"),
    ]),
    ("reference.html", "Element reference", [
        ("document", "<document>"),
        ("constants", "<constants>"),
        ("resources", "<resources>"),
        ("page", "<page>"),
        ("header-footer", "<header> / <footer>"),
        ("watermark", "<watermark>"),
        ("text", "<text>"),
        ("inline", "Inline tags"),
        ("img", "<img>"),
        ("table", "<table>"),
        ("row", "<row>"),
        ("cell", "<cell>"),
        ("spacer", "<spacer>"),
        ("page-break", "<page-break>"),
        ("for-each", "<for-each>"),
        ("style-attributes", "Style attributes"),
    ]),
    ("cli.html", "Command line", []),
    ("library.html", "Go library", []),
    ("examples.html", "Examples", []),
]

GROUPS = [
    ("Getting started", ["index.html", "cli.html", "library.html"]),
    ("Writing documents", ["guide.html", "reference.html"]),
    ("More", ["examples.html"]),
]

FENCE = re.compile(r"^```(\w+)\n(.*?)^```\s*$", re.S | re.M)
INCLUDE = re.compile(r"\{\{file:([^}]+)\}\}")


def render_fences(body: str) -> str:
    def repl(m):
        lang, code = m.group(1), m.group(2)
        code = INCLUDE.sub(lambda f: (ROOT / f.group(1)).read_text().rstrip("\n"), code)
        return f'<pre><code class="lang-{lang}">{html.escape(code.rstrip(chr(10)), quote=False)}</code></pre>'
    return FENCE.sub(repl, body)


def sidebar() -> str:
    titles = {f: (t, subs) for f, t, subs in PAGES}
    out = []
    for group, files in GROUPS:
        out.append(f"<h4>{group}</h4>\n<ul>")
        for f in files:
            title, subs = titles[f]
            out.append(f'<li><a href="{f}">{html.escape(title)}</a>')
            if subs:
                out.append("<ul>")
                for anchor, label in subs:
                    out.append(f'<li><a href="{f}#{anchor}">{html.escape(label)}</a></li>')
                out.append("</ul>")
            out.append("</li>")
        out.append("</ul>")
    return "\n".join(out)


def pager(i: int) -> str:
    order = [f for _, files in GROUPS for f in files]
    titles = {f: t for f, t, _ in PAGES}
    k = order.index(PAGES[i][0])
    parts = ['<nav class="pager">']
    if k > 0:
        p = order[k - 1]
        parts.append(f'<a class="prev" href="{p}"><small>Previous</small>{html.escape(titles[p])}</a>')
    else:
        parts.append("<span></span>")
    if k < len(order) - 1:
        n = order[k + 1]
        parts.append(f'<a class="next" href="{n}"><small>Next</small>{html.escape(titles[n])}</a>')
    parts.append("</nav>")
    return "".join(parts)


TEMPLATE = """<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{title}</title>
<meta name="description" content="xml2pdf: generate PDFs from declarative XML. Documentation for the XML tags, attributes, command line tool and Go library.">
<link rel="icon" href="data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' viewBox='0 0 32 32'%3E%3Crect width='32' height='32' rx='7' fill='%231f4e9c'/%3E%3Ctext x='16' y='21' font-size='13' text-anchor='middle' fill='white' font-family='monospace'%3E%3C/%3E%3C/text%3E%3C/svg%3E">
<link rel="stylesheet" href="assets/style.css">
<script src="assets/site.js"></script>
</head>
<body>
<header class="topbar">
  <button class="icon-btn menu-btn" id="menu-btn" type="button" aria-label="Toggle navigation"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" aria-hidden="true"><path d="M3 6h18M3 12h18M3 18h18"/></svg></button>
  <a class="brand" href="index.html"><span class="logo">&lt;/&gt;</span>xml2pdf</a>
  <span class="spacer"></span>
  <button class="icon-btn" id="theme-toggle" type="button" aria-label="Toggle dark mode" title="Toggle dark mode"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><circle cx="12" cy="12" r="9"/><path d="M12 3a9 9 0 0 0 0 18z" fill="currentColor"/></svg></button>
  <a class="gh" href="{repo}"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M16 18l6-6-6-6"/><path d="M8 6l-6 6 6 6"/></svg><span class="txt">Source</span></a>
</header>
<div class="layout">
<nav class="sidebar" aria-label="Documentation">
{sidebar}
</nav>
<main>
{body}
{pager}
<footer class="site">xml2pdf is open source. <a href="{repo}">Source on GitHub</a> · <a href="{repo}/issues">Report an issue</a></footer>
</main>
</div>
</body>
</html>
"""


def main():
    side = sidebar()
    for i, (fname, title, _) in enumerate(PAGES):
        body = render_fences((SRC / fname).read_text())
        page_title = "xml2pdf: generate PDFs from XML" if fname == "index.html" else f"{title} · xml2pdf"
        (OUT / fname).write_text(TEMPLATE.format(
            title=html.escape(page_title), repo=REPO, sidebar=side, body=body, pager=pager(i)))
        print("wrote", OUT / fname)
    (OUT / ".nojekyll").write_text("")


if __name__ == "__main__":
    main()
