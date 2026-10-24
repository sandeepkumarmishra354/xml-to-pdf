package model

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/style"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/units"
)

// PageSizes lists the named page sizes (portrait, points).
var PageSizes = map[string]Size{
	"a3":      {841.89, 1190.55},
	"a4":      {595.28, 841.89},
	"a5":      {419.53, 595.28},
	"letter":  {612, 792},
	"legal":   {612, 1008},
	"tabloid": {792, 1224},
}

// DefaultMargin is used when the document does not set a margin (0.5in).
const DefaultMargin = 36.0

var inlineTags = map[string]bool{"b": true, "strong": true, "i": true, "em": true, "u": true, "span": true, "br": true}

// Build converts a parsed (and variable-expanded) tree into a Document.
// Relative file paths are resolved against baseDir.
func Build(root *parser.Node, baseDir string) (*Document, error) {
	b := &builder{baseDir: baseDir}
	return b.document(root)
}

type builder struct {
	baseDir string
	doc     *Document
	// Default page geometry before orientation, used by <page> overrides.
	baseSize    Size
	orientation string
}

// attrs tracks which attributes of a node have been consumed so unknown
// (likely misspelled) attributes can be reported.
type attrs struct {
	n    *parser.Node
	used map[string]bool
}

func newAttrs(n *parser.Node) *attrs { return &attrs{n: n, used: map[string]bool{}} }

func (a *attrs) get(k string) (string, bool) {
	a.used[k] = true
	return a.n.Attr(k)
}

func (a *attrs) props() (style.Props, error) {
	p, err := style.ParseProps(a.get)
	if err != nil {
		return p, a.errf("%v", err)
	}
	if v, ok := a.get("font-size"); ok && p.Size == nil {
		f, err := units.ParseAbs(v)
		if err != nil || f <= 0 {
			return p, a.errf("invalid font-size %q", v)
		}
		p.Size = &f
	}
	return p, nil
}

func (a *attrs) length(k string) (*units.Length, error) {
	v, ok := a.get(k)
	if !ok {
		return nil, nil
	}
	l, err := units.Parse(v)
	if err != nil {
		return nil, a.errf("%s: %v", k, err)
	}
	return &l, nil
}

func (a *attrs) abs(k string) (*float64, error) {
	v, ok := a.get(k)
	if !ok {
		return nil, nil
	}
	f, err := units.ParseAbs(v)
	if err != nil {
		return nil, a.errf("%s: %v", k, err)
	}
	return &f, nil
}

func (a *attrs) boolean(k string, def bool) (bool, error) {
	v, ok := a.get(k)
	if !ok {
		return def, nil
	}
	b, err := style.ParseBool(v)
	if err != nil {
		return false, a.errf("%s: %v", k, err)
	}
	return b, nil
}

func (a *attrs) float(k string, def float64) (float64, error) {
	v, ok := a.get(k)
	if !ok {
		return def, nil
	}
	f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
	if err != nil {
		return 0, a.errf("%s: invalid number %q", k, v)
	}
	return f, nil
}

func (a *attrs) color(k string) (*style.Color, error) {
	v, ok := a.get(k)
	if !ok {
		return nil, nil
	}
	c, err := style.ParseColor(v)
	if err != nil {
		return nil, a.errf("%s: %v", k, err)
	}
	return &c, nil
}

func (a *attrs) errf(format string, args ...any) error {
	return parser.Errorf(a.n.Line, "<%s>: %s", a.n.Name, fmt.Sprintf(format, args...))
}

// done reports the first attribute that was never consumed.
func (a *attrs) done() error {
	for _, at := range a.n.Attrs {
		if !a.used[at.Name] {
			return a.errf("unknown attribute %q", at.Name)
		}
	}
	return nil
}

func (b *builder) path(a *attrs, p string) (string, error) {
	if !filepath.IsAbs(p) {
		p = filepath.Join(b.baseDir, p)
	}
	if _, err := os.Stat(p); err != nil {
		return "", a.errf("image not found: %s", p)
	}
	return p, nil
}

func (b *builder) document(n *parser.Node) (*Document, error) {
	if n.Name != "document" {
		return nil, parser.Errorf(n.Line, "root element must be <document>, got <%s>", n.Name)
	}
	doc := &Document{Styles: style.NewRegistry(), Images: map[string]string{}}
	b.doc = doc
	a := newAttrs(n)
	doc.Title, _ = a.get("title")
	doc.Author, _ = a.get("author")
	doc.Subject, _ = a.get("subject")
	doc.Keywords, _ = a.get("keywords")
	var err error
	if doc.Props, err = a.props(); err != nil {
		return nil, err
	}
	b.baseSize = PageSizes["a4"]
	if v, ok := a.get("page-size"); ok {
		if b.baseSize, err = parsePageSize(v); err != nil {
			return nil, a.errf("%v", err)
		}
	}
	b.orientation = "portrait"
	if v, ok := a.get("orientation"); ok {
		if b.orientation, err = parseOrientation(v); err != nil {
			return nil, a.errf("%v", err)
		}
	}
	doc.Size = orient(b.baseSize, b.orientation)
	doc.Margin = [4]float64{DefaultMargin, DefaultMargin, DefaultMargin, DefaultMargin}
	if v, ok := a.get("margin"); ok {
		if doc.Margin, err = units.ParseBox(v); err != nil {
			return nil, a.errf("margin: %v", err)
		}
	}
	if err := a.done(); err != nil {
		return nil, err
	}

	// Resources first, so styles and images can be referenced from anywhere.
	for _, c := range n.Elements() {
		if c.Name == "resources" {
			if err := b.resources(c); err != nil {
				return nil, err
			}
		}
	}
	if err := doc.Styles.Flatten(); err != nil {
		return nil, parser.Errorf(n.Line, "%v", err)
	}

	for _, c := range n.Children {
		if c.IsText() {
			if strings.TrimSpace(c.Text) != "" {
				return nil, parser.Errorf(c.Line, "unexpected text directly inside <document>; put it in a <page>")
			}
			continue
		}
		switch c.Name {
		case "constants", "resources":
		case "watermark":
			w, err := b.watermark(c)
			if err != nil {
				return nil, err
			}
			doc.Watermarks = append(doc.Watermarks, w)
		case "header", "footer":
			band, err := b.band(c)
			if err != nil {
				return nil, err
			}
			if c.Name == "header" {
				if doc.Header != nil {
					return nil, parser.Errorf(c.Line, "only one <header> allowed")
				}
				doc.Header = band
			} else {
				if doc.Footer != nil {
					return nil, parser.Errorf(c.Line, "only one <footer> allowed")
				}
				doc.Footer = band
			}
		case "page":
			p, err := b.page(c)
			if err != nil {
				return nil, err
			}
			doc.Pages = append(doc.Pages, p)
		default:
			return nil, parser.Errorf(c.Line, "unknown element <%s> in <document>", c.Name)
		}
	}
	if len(doc.Pages) == 0 {
		return nil, parser.Errorf(n.Line, "document has no <page>")
	}
	if err := b.checkStyleRefs(); err != nil {
		return nil, err
	}
	return doc, nil
}

func parsePageSize(v string) (Size, error) {
	t := strings.ToLower(strings.TrimSpace(v))
	if s, ok := PageSizes[t]; ok {
		return s, nil
	}
	w, h, ok := strings.Cut(t, "x")
	if ok {
		wv, err1 := units.ParseAbs(w)
		hv, err2 := units.ParseAbs(h)
		if err1 == nil && err2 == nil && wv > 0 && hv > 0 {
			return Size{wv, hv}, nil
		}
	}
	return Size{}, fmt.Errorf("invalid page-size %q (A3, A4, A5, Letter, Legal, Tabloid or WIDTHxHEIGHT, e.g. 100mmx150mm)", v)
}

func parseOrientation(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "portrait", "p":
		return "portrait", nil
	case "landscape", "l":
		return "landscape", nil
	}
	return "", fmt.Errorf("invalid orientation %q (portrait or landscape)", v)
}

func orient(s Size, o string) Size {
	if (o == "landscape") != (s.W > s.H) {
		return Size{s.H, s.W}
	}
	return s
}

func (b *builder) resources(n *parser.Node) error {
	if err := newAttrs(n).done(); err != nil {
		return err
	}
	for _, c := range n.Children {
		if c.IsText() {
			if strings.TrimSpace(c.Text) != "" {
				return parser.Errorf(c.Line, "unexpected text in <resources>")
			}
			continue
		}
		a := newAttrs(c)
		id, ok := a.get("id")
		if !ok || strings.TrimSpace(id) == "" {
			return a.errf("missing id attribute")
		}
		switch c.Name {
		case "image":
			src, ok := a.get("src")
			if !ok {
				return a.errf("missing src attribute")
			}
			p, err := b.path(a, src)
			if err != nil {
				return err
			}
			if _, dup := b.doc.Images[id]; dup {
				return a.errf("image %q defined twice", id)
			}
			b.doc.Images[id] = p
		case "style":
			ext, _ := a.get("extends")
			p, err := a.props()
			if err != nil {
				return err
			}
			if err := b.doc.Styles.Add(&style.Def{ID: id, Extends: ext, Props: p, Line: c.Line}); err != nil {
				return a.errf("%v", err)
			}
		default:
			return parser.Errorf(c.Line, "unknown resource <%s> (expected <image> or <style>)", c.Name)
		}
		if err := a.done(); err != nil {
			return err
		}
	}
	return nil
}

// imageRef resolves an image given as ref="id" or src="path".
func (b *builder) imageRef(a *attrs, refAttr string) (string, error) {
	ref, hasRef := a.get(refAttr)
	src, hasSrc := a.get("src")
	switch {
	case hasRef && hasSrc:
		return "", a.errf("use either %s or src, not both", refAttr)
	case hasRef:
		p, ok := b.doc.Images[ref]
		if !ok {
			return "", a.errf("unknown image resource %q", ref)
		}
		return p, nil
	case hasSrc:
		return b.path(a, src)
	}
	return "", nil
}

func (b *builder) watermark(n *parser.Node) (*Watermark, error) {
	a := newAttrs(n)
	w := &Watermark{Line: n.Line, Front: true}
	var err error
	if w.Props, err = a.props(); err != nil {
		return nil, err
	}
	w.Text, _ = a.get("text")
	if w.Text == "" {
		w.Text = strings.TrimSpace(n.InnerText())
	}
	if w.Image, err = b.imageRef(a, "image"); err != nil {
		return nil, err
	}
	if (w.Text == "") == (w.Image == "") {
		return nil, a.errf("needs exactly one of text, image or src")
	}
	if w.Opacity, err = a.float("opacity", 0.15); err != nil {
		return nil, err
	}
	if w.Opacity < 0 || w.Opacity > 1 {
		return nil, a.errf("opacity must be between 0 and 1")
	}
	if w.Rotate, err = a.float("rotate", 45); err != nil {
		return nil, err
	}
	if v, ok := a.get("layer"); ok {
		switch strings.ToLower(v) {
		case "back", "behind":
			w.Front = false
		case "front", "over":
			w.Front = true
		default:
			return nil, a.errf("layer must be back or front")
		}
	}
	if w.Pages, err = b.pageSel(a); err != nil {
		return nil, err
	}
	for _, l := range []struct {
		k   string
		dst **units.Length
	}{{"x", &w.X}, {"y", &w.Y}, {"width", &w.W}, {"height", &w.H}} {
		if *l.dst, err = a.length(l.k); err != nil {
			return nil, err
		}
	}
	return w, a.done()
}

func (b *builder) pageSel(a *attrs) (PageSel, error) {
	v, _ := a.get("pages")
	ps, err := ParsePageSel(v)
	if err != nil {
		return ps, a.errf("%v", err)
	}
	return ps, nil
}

func (b *builder) band(n *parser.Node) (*Band, error) {
	a := newAttrs(n)
	band := &Band{Line: n.Line, Gap: 6}
	var err error
	if band.Props, err = a.props(); err != nil {
		return nil, err
	}
	if band.Height, err = a.abs("height"); err != nil {
		return nil, err
	}
	if g, err := a.abs("gap"); err != nil {
		return nil, err
	} else if g != nil {
		band.Gap = *g
	}
	if band.Pages, err = b.pageSel(a); err != nil {
		return nil, err
	}
	if err := a.done(); err != nil {
		return nil, err
	}
	band.Blocks, err = b.flow(n, false)
	return band, err
}

func (b *builder) page(n *parser.Node) (*Page, error) {
	a := newAttrs(n)
	p := &Page{Line: n.Line, Size: b.doc.Size, Margin: b.doc.Margin}
	var err error
	if p.Props, err = a.props(); err != nil {
		return nil, err
	}
	size, o := b.baseSize, b.orientation
	if v, ok := a.get("page-size"); ok {
		if size, err = parsePageSize(v); err != nil {
			return nil, a.errf("%v", err)
		}
	}
	if v, ok := a.get("orientation"); ok {
		if o, err = parseOrientation(v); err != nil {
			return nil, a.errf("%v", err)
		}
	}
	p.Size = orient(size, o)
	if v, ok := a.get("margin"); ok {
		if p.Margin, err = units.ParseBox(v); err != nil {
			return nil, a.errf("margin: %v", err)
		}
	}
	var show bool
	if show, err = a.boolean("header", true); err != nil {
		return nil, err
	}
	p.NoHeader = !show
	if show, err = a.boolean("footer", true); err != nil {
		return nil, err
	}
	p.NoFooter = !show
	if show, err = a.boolean("watermark", true); err != nil {
		return nil, err
	}
	p.NoWatermark = !show
	if err := a.done(); err != nil {
		return nil, err
	}
	p.Blocks, err = b.flow(n, false)
	return p, err
}

// flow parses block children. Loose inline content (text, <b>, <span>, ...)
// is gathered into implicit <text> paragraphs.
func (b *builder) flow(n *parser.Node, inCell bool) ([]Block, error) {
	var out []Block
	var pending []*parser.Node
	flush := func() error {
		if len(pending) == 0 {
			return nil
		}
		blank := true
		for _, p := range pending {
			if !p.IsText() || strings.TrimSpace(p.Text) != "" {
				blank = false
			}
		}
		if !blank {
			inl, err := b.inlines(pending)
			if err != nil {
				return err
			}
			out = append(out, &Text{Common: Common{Line: pending[0].Line}, Inlines: inl})
		}
		pending = nil
		return nil
	}
	for _, c := range n.Children {
		if c.IsText() || inlineTags[c.Name] {
			pending = append(pending, c)
			continue
		}
		if err := flush(); err != nil {
			return nil, err
		}
		blk, err := b.block(c)
		if err != nil {
			return nil, err
		}
		if inCell && blk.Base().Absolute() {
			return nil, parser.Errorf(c.Line, "<%s>: x/y positioning is not allowed inside table cells", c.Name)
		}
		out = append(out, blk)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	return out, nil
}

func (b *builder) common(a *attrs, c *Common) error {
	c.Line = a.n.Line
	var err error
	if c.Props, err = a.props(); err != nil {
		return err
	}
	if c.X, err = a.length("x"); err != nil {
		return err
	}
	if c.Y, err = a.length("y"); err != nil {
		return err
	}
	c.Width, err = a.length("width")
	return err
}

func (b *builder) block(n *parser.Node) (Block, error) {
	a := newAttrs(n)
	switch n.Name {
	case "text", "p":
		t := &Text{}
		if err := b.common(a, &t.Common); err != nil {
			return nil, err
		}
		if err := a.done(); err != nil {
			return nil, err
		}
		inl, err := b.inlines(n.Children)
		t.Inlines = inl
		return t, err
	case "img", "image":
		img := &Image{}
		if err := b.common(a, &img.Common); err != nil {
			return nil, err
		}
		var err error
		if img.Height, err = a.length("height"); err != nil {
			return nil, err
		}
		if img.Path, err = b.imageRef(a, "ref"); err != nil {
			return nil, err
		}
		if img.Path == "" {
			return nil, a.errf("needs ref or src")
		}
		if err := a.done(); err != nil {
			return nil, err
		}
		if len(n.Elements()) > 0 {
			return nil, a.errf("must be empty")
		}
		return img, nil
	case "table":
		return b.table(n, a)
	case "spacer":
		s := &Spacer{}
		if err := b.common(a, &s.Common); err != nil {
			return nil, err
		}
		h, err := a.abs("height")
		if err != nil {
			return nil, err
		}
		if h == nil {
			return nil, a.errf("missing height attribute")
		}
		s.Height = *h
		return s, a.done()
	case "page-break":
		pb := &PageBreak{Common: Common{Line: n.Line}}
		return pb, a.done()
	}
	return nil, parser.Errorf(n.Line, "unknown element <%s>", n.Name)
}

func (b *builder) inlines(nodes []*parser.Node) ([]Inline, error) {
	var out []Inline
	for _, c := range nodes {
		if c.IsText() {
			out = append(out, Inline{Text: c.Text})
			continue
		}
		a := newAttrs(c)
		var p style.Props
		switch c.Name {
		case "br":
			if err := a.done(); err != nil {
				return nil, err
			}
			out = append(out, Inline{Br: true})
			continue
		case "b", "strong":
			t := true
			p.Bold = &t
		case "i", "em":
			t := true
			p.Italic = &t
		case "u":
			t := true
			p.Underline = &t
		case "span":
		default:
			return nil, parser.Errorf(c.Line, "<%s> is not allowed inside text (allowed: b, i, u, span, br)", c.Name)
		}
		own, err := a.props()
		if err != nil {
			return nil, err
		}
		p = p.Over(own)
		if err := a.done(); err != nil {
			return nil, err
		}
		kids, err := b.inlines(c.Children)
		if err != nil {
			return nil, err
		}
		out = append(out, Inline{Props: &p, Children: kids})
	}
	return out, nil
}

func (b *builder) table(n *parser.Node, a *attrs) (*Table, error) {
	t := &Table{Border: 0.5, Padding: [4]float64{4, 4, 4, 4}, RepeatHeader: true}
	if err := b.common(a, &t.Common); err != nil {
		return nil, err
	}
	var err error
	if v, ok := a.get("widths"); ok {
		if t.Cols, err = ParseColWidths(v); err != nil {
			return nil, a.errf("%v", err)
		}
	}
	if bw, err := a.abs("border"); err != nil {
		return nil, err
	} else if bw != nil {
		t.Border = *bw
	}
	if c, err := a.color("border-color"); err != nil {
		return nil, err
	} else if c != nil {
		t.BorderColor = *c
	}
	if v, ok := a.get("padding"); ok {
		if t.Padding, err = units.ParseBox(v); err != nil {
			return nil, a.errf("padding: %v", err)
		}
	}
	if t.HeaderBg, err = a.color("header-bg"); err != nil {
		return nil, err
	}
	if t.Stripe, err = a.color("stripe"); err != nil {
		return nil, err
	}
	if t.RepeatHeader, err = a.boolean("repeat-header", true); err != nil {
		return nil, err
	}
	if err := a.done(); err != nil {
		return nil, err
	}
	for _, c := range n.Children {
		if c.IsText() {
			if strings.TrimSpace(c.Text) != "" {
				return nil, parser.Errorf(c.Line, "unexpected text in <table>; put it in a <cell>")
			}
			continue
		}
		if c.Name != "row" {
			return nil, parser.Errorf(c.Line, "unexpected <%s> in <table>, expected <row>", c.Name)
		}
		r, err := b.row(c)
		if err != nil {
			return nil, err
		}
		t.Rows = append(t.Rows, r)
	}
	ncols := len(t.Cols)
	for _, r := range t.Rows {
		span := 0
		for _, c := range r.Cells {
			span += c.Colspan
		}
		if len(t.Cols) > 0 && span > ncols {
			return nil, parser.Errorf(r.Line, "row spans %d columns but table has %d", span, ncols)
		}
		if len(t.Cols) == 0 && span > ncols {
			ncols = span
		}
	}
	if len(t.Cols) == 0 {
		for i := 0; i < ncols; i++ {
			t.Cols = append(t.Cols, ColWidth{Frac: 1, IsFrac: true})
		}
	}
	return t, nil
}

func (b *builder) row(n *parser.Node) (*Row, error) {
	a := newAttrs(n)
	r := &Row{Line: n.Line}
	var err error
	if r.Props, err = a.props(); err != nil {
		return nil, err
	}
	if r.Header, err = a.boolean("header", false); err != nil {
		return nil, err
	}
	if h, err := a.abs("height"); err != nil {
		return nil, err
	} else if h != nil {
		r.MinHeight = *h
	}
	if err := a.done(); err != nil {
		return nil, err
	}
	for _, c := range n.Children {
		if c.IsText() {
			if strings.TrimSpace(c.Text) != "" {
				return nil, parser.Errorf(c.Line, "unexpected text in <row>; put it in a <cell>")
			}
			continue
		}
		if c.Name != "cell" {
			return nil, parser.Errorf(c.Line, "unexpected <%s> in <row>, expected <cell>", c.Name)
		}
		ca := newAttrs(c)
		cell := &Cell{Line: c.Line, Colspan: 1, VAlign: "top"}
		if cell.Props, err = ca.props(); err != nil {
			return nil, err
		}
		if v, ok := ca.get("colspan"); ok {
			if cell.Colspan, err = strconv.Atoi(strings.TrimSpace(v)); err != nil || cell.Colspan < 1 {
				return nil, ca.errf("invalid colspan %q", v)
			}
		}
		if v, ok := ca.get("valign"); ok {
			switch v = strings.ToLower(strings.TrimSpace(v)); v {
			case "top", "middle", "bottom":
				cell.VAlign = v
			default:
				return nil, ca.errf("valign must be top, middle or bottom")
			}
		}
		if err := ca.done(); err != nil {
			return nil, err
		}
		if cell.Blocks, err = b.flow(c, true); err != nil {
			return nil, err
		}
		r.Cells = append(r.Cells, cell)
	}
	return r, nil
}

// checkStyleRefs verifies that every style="..." reference exists, so typos
// are reported with a line number before layout.
func (b *builder) checkStyleRefs() error {
	reg := b.doc.Styles
	check := func(line int, p style.Props) error {
		for _, r := range p.Refs {
			if !reg.Has(r) {
				return parser.Errorf(line, "unknown style %q", r)
			}
		}
		return nil
	}
	var inl func(int, []Inline) error
	inl = func(line int, xs []Inline) error {
		for _, x := range xs {
			if x.Props != nil {
				if err := check(line, *x.Props); err != nil {
					return err
				}
			}
			if err := inl(line, x.Children); err != nil {
				return err
			}
		}
		return nil
	}
	var blocks func([]Block) error
	blocks = func(bs []Block) error {
		for _, blk := range bs {
			c := blk.Base()
			if err := check(c.Line, c.Props); err != nil {
				return err
			}
			switch blk := blk.(type) {
			case *Text:
				if err := inl(c.Line, blk.Inlines); err != nil {
					return err
				}
			case *Table:
				for _, r := range blk.Rows {
					if err := check(r.Line, r.Props); err != nil {
						return err
					}
					for _, cell := range r.Cells {
						if err := check(cell.Line, cell.Props); err != nil {
							return err
						}
						if err := blocks(cell.Blocks); err != nil {
							return err
						}
					}
				}
			}
		}
		return nil
	}
	d := b.doc
	if err := check(0, d.Props); err != nil {
		return err
	}
	for _, w := range d.Watermarks {
		if err := check(w.Line, w.Props); err != nil {
			return err
		}
	}
	for _, band := range []*Band{d.Header, d.Footer} {
		if band == nil {
			continue
		}
		if err := check(band.Line, band.Props); err != nil {
			return err
		}
		if err := blocks(band.Blocks); err != nil {
			return err
		}
	}
	for _, p := range d.Pages {
		if err := check(p.Line, p.Props); err != nil {
			return err
		}
		if err := blocks(p.Blocks); err != nil {
			return err
		}
	}
	return nil
}
