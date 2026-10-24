# xml2pdf

[![CI](https://github.com/sandeepkumarmishra354/xml-to-pdf/actions/workflows/ci.yml/badge.svg)](https://github.com/sandeepkumarmishra354/xml-to-pdf/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/sandeepkumarmishra354/xml-to-pdf)](https://github.com/sandeepkumarmishra354/xml-to-pdf/releases/latest)
[![Go Reference](https://pkg.go.dev/badge/github.com/sandeepkumarmishra354/xml-to-pdf.svg)](https://pkg.go.dev/github.com/sandeepkumarmishra354/xml-to-pdf/pkg/xml2pdf)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

Generate PDFs from a declarative XML description. You describe pages, text, images, tables, watermarks, headers and footers in XML, and `xml2pdf` lays them out (wrapping text, breaking pages, repeating table headers) and writes a PDF.

- **Single static binary.** It's written in Go with no cgo and no runtime dependencies.
- **Fast.** A multi-page report renders in about 20 ms.
- **Templating.** Constants live in the document, and values can be supplied from a JSON file or the command line with `${name}`. `<for-each>` repeats content once for each element of a data array.

**Documentation:** https://sandeepkumarmishra354.github.io/xml-to-pdf/ (sources in `docs-src/`, rebuilt with `make docs`)

```xml
<document title="Hello" page-size="A4" margin="20mm">
  <constants><const name="who" value="World"/></constants>
  <page>
    <text size="24" bold="true">Hello, ${who}!</text>
  </page>
</document>
```

## Install

**Linux and macOS** (one line):

```sh
curl -fsSL https://raw.githubusercontent.com/sandeepkumarmishra354/xml-to-pdf/main/install.sh | sh
```

The script downloads the right binary for your system from the latest release, verifies its SHA-256 checksum, and installs it to `/usr/local/bin`. If that directory isn't writable, it uses `sudo` or falls back to `~/.local/bin`. Set `XML2PDF_VERSION=0.1.0` to pin a version, or `XML2PDF_INSTALL_DIR=~/bin` to choose the directory. Installing this way also avoids the macOS "could not verify" warning, because macOS only flags files downloaded through a browser.

**Windows, or manual download:** get the archive for your platform from the [latest release](https://github.com/sandeepkumarmishra354/xml-to-pdf/releases/latest), unpack it, and put `xml2pdf` on your `PATH`. On macOS, a binary downloaded through a browser is blocked the first time it runs; clear the flag with `xattr -d com.apple.quarantine xml2pdf`.

**With Go** (1.24+):

```sh
go install github.com/sandeepkumarmishra354/xml-to-pdf/cmd/xml2pdf@latest
```

**From source:**

```sh
make build             # bin/xml2pdf (static)
make dist              # cross-compiled binaries for linux/darwin/windows × amd64/arm64 in dist/
make test              # run the test suite
make examples          # render examples/*.xml into bin/
```


## Command line

```sh
xml2pdf invoice.xml                                  # writes invoice.pdf
xml2pdf invoice.xml -o out.pdf --data data.json      # values from a JSON file
xml2pdf invoice.xml --json '{"customer":{"name":"Globex"}}'   # inline JSON
some-api | xml2pdf invoice.xml --data -              # JSON from stdin
xml2pdf invoice.xml --data base.json --json '{"status":"PAID"}'  # merged, later wins
xml2pdf invoice.xml --set status=PAID --set n=42     # override single values
cat doc.xml | xml2pdf - -o - > doc.pdf               # stdin / stdout
```

| Flag | Meaning |
|---|---|
| `-o FILE` | Output file. It defaults to the input name with `.pdf`. Use `-` for stdout. |
| `--data FILE` | A JSON file containing an object. Its values are available as `${key}`, `${a.b}` or `${list.0.c}`, and its arrays work with `<for-each>`. Use `-` to read the JSON from stdin. |
| `--json JSON` | The same as `--data`, but takes the JSON object inline, e.g. `--json '{"name":"Alice"}'`. |
| `--base-dir DIR` | The directory that relative image paths resolve against. It defaults to the input file's directory, or the working directory when reading from stdin. |
| `--set k=v` | Sets a variable. It can be repeated and takes priority over `--data`, `--json` and constants. |
| `--version` | Prints the version. |

`--data` and `--json` can be repeated and mixed. They are deep-merged from left to right: nested objects combine key by key, and anything else (strings, numbers, arrays) is replaced by the later value. So `--data invoice.json --json '{"customer":{"name":"Initech"}}'` changes only the customer's name and keeps the rest of the file.

## Go library

```go
import "github.com/sandeepkumarmishra354/xml-to-pdf/pkg/xml2pdf"

data, _ := xml2pdf.LoadData("data.json")
err := xml2pdf.RenderFile("invoice.xml", "invoice.pdf", xml2pdf.Options{
    Data: data,
    Vars: map[string]string{"status": "PAID"},
})
// or stream: xml2pdf.Render(r io.Reader, w io.Writer, opts)
```

## Concepts

### Units

Lengths are in **points** (1/72 inch) by default. You can also use `mm`, `cm`, `in` and `pt` suffixes (`20mm`, `1.5in`). Many attributes accept `%`:

- widths are a percentage of the available width;
- `x` is a percentage of the page width and `y` of the page height.

### Variables

Values are looked up in this order, and the first match wins:

1. `--set key=value` on the command line (or `Options.Vars`)
2. JSON data from `--data` and `--json` (or `Options.Data`); nested keys use dots, e.g. `${customer.name}` or `${items.0.price}`
3. `<constants>` in the document

Variables expand in every attribute and in every piece of text. Constants can reference other variables. An undefined variable is an error that reports its line number. Write `$${` to output a literal `${`.

```xml
<constants>
  <const name="company" value="Acme Corp"/>
  <const name="footer">© ${year} ${company}</const>   <!-- value may also be the element text -->
</constants>
```

### Loops: `<for-each>`

`<for-each>` repeats its content once for each element of an array from the data. It's handy for table rows:

```xml
<table widths="0.5in,4*,1*">
  <row header="true"><cell>#</cell><cell>Item</cell><cell>Price</cell></row>
  <for-each items="${items}" var="item" number="n">
    <row><cell>${n}</cell><cell>${item.name}</cell><cell>${item.price}</cell></row>
  </for-each>
</table>
```

| Attribute | Meaning |
|---|---|
| `items` | The array to loop over: a variable path such as `items`, `${items}` or `${order.lines}`. It's required. |
| `var` | The name for the current element (default `item`). Use `${item}` for a scalar element, or `${item.field}` for one field of an object. |
| `index` | Optional name for the 0-based position, e.g. `index="i"` gives `${i}`. |
| `number` | Optional name for the 1-based position, e.g. `number="n"` gives `${n}`. |

Loop variables shadow every other variable with the same name. An empty array produces nothing.

A loop can sit anywhere: around table rows, blocks inside a page, inline text, or whole `<page>` elements (one page per record, for mail-merge). Loops can be nested:

```xml
<for-each items="orders" var="order">
  <text style="h2">Order ${order.id}</text>
  <for-each items="order.lines" var="line"><text>${line.qty} × ${line.sku}</text></for-each>
</for-each>
```

When you use the Go library, `Options.Data` can also hold Go slices and maps, such as `[]map[string]any`, not only decoded JSON.

### Layout

Content **flows** from top to bottom inside the page margins and continues on new pages as needed. Paragraphs break between lines and tables break between rows; a table's header rows repeat on each new page. Give any block an `x` and/or `y` to **position it absolutely**, measured from the page's top-left corner. Absolute blocks don't affect the flow.

Each `<page>` element starts a new physical page, and its overflow pages share its settings.

### Styles

Text properties cascade from document to page to block to inline span:

| Attribute | Values | Inherited |
|---|---|---|
| `style` | one or more named styles, e.g. `style="h1 muted"` | – |
| `font` | `helvetica` (also `arial`, `sans`), `times` (`serif`), `courier` (`monospace`), `zapfdingbats` | yes |
| `size` / `font-size` | font size, e.g. `11` | yes |
| `bold`, `italic`, `underline` | `true` / `false` | yes |
| `color` | `#rgb`, `#rrggbb`, `rgb(r,g,b)` or a name (`red`, `gray`, `navy`, …) | yes |
| `align` | `left`, `center`, `right`, `justify` | yes |
| `line-height` | a multiple of the font size (default `1.2`) | yes |
| `margin-top`, `margin-bottom` | space above and below a block | no |
| `bg` | background color (rows and cells) | no |

Named styles are declared in `<resources>`. A style can `extends` another one, and the attributes written on the element itself win over the named style.

## Element reference

### `<document>`

The root element. Its attributes are `title`, `author`, `subject`, `keywords` (PDF metadata), `page-size` (`A3`, `A4`, `A5`, `Letter`, `Legal`, `Tabloid`, or `WxH` such as `100mmx150mm`), `orientation` (`portrait` or `landscape`), `margin` (1–4 values, CSS order: top right bottom left; default `0.5in`), and any style attribute.

Its children are `<constants>`, `<resources>`, `<watermark>`, `<header>`, `<footer>` and `<page>`.

### `<resources>`

```xml
<resources>
  <image id="logo" src="images/logo.png"/>            <!-- PNG, JPEG or GIF; path relative to the XML file -->
  <style id="h1" size="20" bold="true" color="#1f4e9c" margin-bottom="6"/>
  <style id="h2" extends="h1" size="14"/>
</resources>
```

### `<page>`

Optional overrides: `page-size`, `orientation`, `margin`. Use `header="false"`, `footer="false"` or `watermark="false"` to hide those on this page and its overflow pages. Style attributes cascade to the content.

Its children are any blocks. Loose text placed directly inside a page becomes a paragraph.

### Blocks

**`<text>`** (or `<p>`) is a paragraph. Whitespace is collapsed and lines wrap automatically. Inline markup: `<b>`/`<strong>`, `<i>`/`<em>`, `<u>`, `<span …style attributes…>`, `<br/>`.

```xml
<text align="justify" margin-bottom="8">Plain, <b>bold</b>, <span color="red" size="14">big red</span><br/>new line</text>
<text x="120mm" y="20mm" width="70mm" align="right">Absolutely placed</text>
```

**`<img>`** (or `<image>`) takes `ref="id"` or `src="file.png"`. Set `width` and/or `height`; if you set only one, the aspect ratio is kept. Images never grow beyond the available space. `align` places the image horizontally, and `x`/`y` positions it absolutely.

**`<table>`**

```xml
<table widths="3*,1*,80,20%" border="0.5" border-color="#bbb" padding="4 6"
       header-bg="#eee" stripe="#f7f7f7" repeat-header="true">
  <row header="true"><cell>Item</cell><cell align="right">Qty</cell>…</row>
  <row><cell colspan="2">…</cell><cell valign="middle">…</cell>…</row>
</table>
```

| Attribute | Meaning |
|---|---|
| `widths` | Column widths. `N*` takes a share of the remaining space, a plain length is a fixed width, and `%` is a percentage of the table width. Without `widths`, all columns are equal. |
| `width` | Table width (default `100%`). |
| `border`, `border-color` | Cell border width (default `0.5`; `0` means none) and color. |
| `padding` | Cell padding, 1–4 values (default `4`). |
| `header-bg`, `stripe` | Background of header rows, and of every other body row. |
| `repeat-header` | Repeat header rows after a page break (default `true`). |

`<row>` takes `header="true"` (bold, and repeated across pages), `height` (minimum height), `bg`, and style attributes. `<cell>` takes `colspan`, `valign` (`top`, `middle` or `bottom`), `bg`, and style attributes. A cell can hold inline text or blocks (`<text>`, `<img>`, even a nested `<table>`). Rows are never split across pages.

**`<spacer height="10mm"/>`** adds vertical space. **`<page-break/>`** starts a new page.

### `<header>` / `<footer>`

Content repeated on each page, inside the top or bottom margin box. The body content area shrinks to make room.

- `{page}` and `{pages}` in their text become the current page number and the total page count.
- `height` sets a fixed height; by default the height is measured from the content.
- `gap` is the space between the band and the body (default `6`).
- `pages` selects which pages show the band.

```xml
<footer pages="all"><text align="center" size="9">Page {page} of {pages}</text></footer>
<header pages="2-"><text align="right">Annual report</text></header>
```

### `<watermark>`

A text or image watermark. You can declare several.

```xml
<watermark text="DRAFT" opacity="0.15" rotate="45" size="80" color="red"/>
<watermark image="logo" width="60%" opacity="0.06" layer="back" pages="2-"/>
```

| Attribute | Meaning |
|---|---|
| `text` / `image` / `src` | The content: text, an image resource id, or an image file. |
| `opacity` | `0`–`1` (default `0.15`). |
| `rotate` | Degrees counter-clockwise (default `45`). |
| `layer` | `front` (default) draws over the content; `back` draws beneath it. |
| `x`, `y` | Center point (default: page center). |
| `width`, `height` | Image size (default: 50% of the page width). |
| `pages` | Page selector (see below). |
| style attributes | `font`, `size` (default 60), `bold` (default true), `color` (default gray). |

### Page selectors

The `pages` attribute on header, footer and watermark is a comma-separated list of `all`, `first`, `last`, `odd`, `even`, `3`, `2-5` or `2-` (page 2 onwards). Page numbers count physical pages in the output.

## Limitations

- Only the 14 built-in PDF fonts are available, so text is limited to Windows-1252 (Latin-1 plus €, curly quotes and dashes). Custom TTF fonts with full Unicode are planned.
- There are no conditionals (`<if>`) yet.
- There are no lines or shapes, lists, links or bookmarks.
- Table rows taller than a page are not split.

## Project layout

```
cmd/xml2pdf        CLI
pkg/xml2pdf        public Go API
internal/parser    XML → node tree (keeps mixed content and line numbers)
internal/vars      ${...} resolution
internal/model     typed document model + validation
internal/style     colors, fonts, named styles and the cascade
internal/units     length parsing
internal/layout    text wrapping, tables, pagination → drawing ops
internal/render    drawing ops → PDF (go-pdf/fpdf)
examples/          invoice.xml (+ invoice.json), report.xml
```

## Releasing

Releases are built by [GoReleaser](https://goreleaser.com) in GitHub Actions. To publish one, push a version tag:

```sh
git tag v0.1.0
git push origin v0.1.0
```

The release workflow runs the tests, cross-compiles the binaries, and publishes a GitHub Release with archives, checksums and a changelog. Run `make snapshot` to try the release build locally; it needs `goreleaser` installed and writes to `dist/`.

## License

[MIT](LICENSE): free for personal and commercial use.
