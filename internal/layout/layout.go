package layout

import (
	"strconv"
	"strings"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/model"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/style"
)

type fragKind int

const (
	fragNormal fragKind = iota
	fragSpacer          // vertical space, dropped at the top of an overflow page
	fragBreak           // forced page break
	fragAbs             // absolutely positioned; ops are in page coordinates
)

// frag is an unbreakable horizontal slice of content (a text line, an
// image, a table row). Ops are relative to the fragment's top-left corner.
type frag struct {
	kind         fragKind
	h            float64
	ops          []Op
	keepWithNext bool    // e.g. table header rows
	repeat       []*frag // emitted first if this fragment starts a new page
}

// geom describes a page and its content box.
type geom struct {
	W, H                     float64
	left, right, top, bottom float64
	headerTop, footerTop     float64
}

func (g geom) width() float64 { return g.right - g.left }

type engine struct {
	doc   *model.Document
	m     Measurer
	root  style.Computed
	subst func(string) string // {page}/{pages} replacement inside bands
}

// Layout lays out the whole document.
func Layout(doc *model.Document, m Measurer) ([]Page, error) {
	e := &engine{doc: doc, m: m}
	var err error
	if e.root, err = doc.Styles.Resolve(style.Default, doc.Props); err != nil {
		return nil, err
	}

	type phys struct {
		g   geom
		src *model.Page
		ops []Op
	}
	var pages []*phys
	for _, p := range doc.Pages {
		pc, err := doc.Styles.Resolve(e.root, p.Props)
		if err != nil {
			return nil, parser.Errorf(p.Line, "%v", err)
		}
		g, err := e.geometry(p)
		if err != nil {
			return nil, err
		}
		frags, err := e.blocks(p.Blocks, pc, g.width(), g)
		if err != nil {
			return nil, err
		}
		for _, ops := range paginate(frags, g) {
			pages = append(pages, &phys{g: g, src: p, ops: ops})
		}
	}

	total := len(pages)
	out := make([]Page, total)
	for i, pg := range pages {
		n := i + 1
		e.subst = strings.NewReplacer("{page}", strconv.Itoa(n), "{pages}", strconv.Itoa(total)).Replace
		var ops []Op
		wm := func(front bool) error {
			if pg.src.NoWatermark {
				return nil
			}
			for _, w := range doc.Watermarks {
				if w.Front == front && w.Pages.Match(n, total) {
					op, err := e.watermark(w, pg.g)
					if err != nil {
						return err
					}
					ops = append(ops, op)
				}
			}
			return nil
		}
		band := func(b *model.Band, hidden bool, top float64) error {
			if b == nil || hidden || !b.Pages.Match(n, total) {
				return nil
			}
			bops, abs, _, err := e.band(b, pg.g)
			if err != nil {
				return err
			}
			ops = append(ops, shiftAll(bops, pg.g.left, top)...)
			ops = append(ops, abs...)
			return nil
		}
		if err := wm(false); err != nil {
			return nil, err
		}
		if err := band(doc.Header, pg.src.NoHeader, pg.g.headerTop); err != nil {
			return nil, err
		}
		ops = append(ops, pg.ops...)
		if err := band(doc.Footer, pg.src.NoFooter, pg.g.footerTop); err != nil {
			return nil, err
		}
		if err := wm(true); err != nil {
			return nil, err
		}
		out[i] = Page{W: pg.g.W, H: pg.g.H, Ops: ops}
	}
	return out, nil
}

// geometry computes the content box of a page, reserving room for the
// header and footer bands.
func (e *engine) geometry(p *model.Page) (geom, error) {
	g := geom{W: p.Size.W, H: p.Size.H}
	g.left, g.right = p.Margin[3], p.Size.W-p.Margin[1]
	g.top, g.bottom = p.Margin[0], p.Size.H-p.Margin[2]
	// Measure bands with representative page numbers.
	e.subst = strings.NewReplacer("{page}", "99", "{pages}", "99").Replace
	defer func() { e.subst = nil }()
	if b := e.doc.Header; b != nil && !p.NoHeader {
		_, _, h, err := e.band(b, g)
		if err != nil {
			return g, err
		}
		g.headerTop = g.top
		g.top += h + b.Gap
	}
	if b := e.doc.Footer; b != nil && !p.NoFooter {
		_, _, h, err := e.band(b, g)
		if err != nil {
			return g, err
		}
		g.footerTop = g.bottom - h
		g.bottom -= h + b.Gap
	}
	if g.bottom-g.top < 10 || g.width() < 10 {
		return g, parser.Errorf(p.Line, "page content area is too small (check margin, header and footer sizes)")
	}
	return g, nil
}

// band lays out a header/footer. Returns flow ops (relative to the band's
// top-left), absolute ops, and the band height.
func (e *engine) band(b *model.Band, g geom) ([]Op, []Op, float64, error) {
	c, err := e.doc.Styles.Resolve(e.root, b.Props)
	if err != nil {
		return nil, nil, 0, parser.Errorf(b.Line, "%v", err)
	}
	frags, err := e.blocks(b.Blocks, c, g.width(), g)
	if err != nil {
		return nil, nil, 0, err
	}
	ops, abs, h := stack(frags)
	if b.Height != nil {
		h = *b.Height
	}
	return ops, abs, h, nil
}

// stack places fragments one below another without pagination.
func stack(frags []*frag) (ops, abs []Op, h float64) {
	for _, f := range frags {
		switch f.kind {
		case fragAbs:
			abs = append(abs, f.ops...)
		case fragNormal, fragSpacer:
			ops = append(ops, shiftAll(f.ops, 0, h)...)
			h += f.h
		}
	}
	return ops, abs, h
}

// paginate distributes fragments over as many pages as needed and returns
// the ops of each page in page coordinates.
func paginate(frags []*frag, g geom) [][]Op {
	var pages [][]Op
	var y float64
	empty, used, overflowed := true, false, false
	newPage := func(overflow bool) {
		pages = append(pages, nil)
		y = g.top
		empty, used, overflowed = true, false, overflow
	}
	place := func(f *frag) {
		k := len(pages) - 1
		pages[k] = append(pages[k], shiftAll(f.ops, g.left, y)...)
		y += f.h
		empty, used = false, true
	}
	newPage(false)
	for i, f := range frags {
		switch f.kind {
		case fragBreak:
			if used {
				newPage(false)
			}
		case fragAbs:
			k := len(pages) - 1
			pages[k] = append(pages[k], f.ops...)
			used = true
		case fragSpacer:
			if empty && overflowed {
				continue
			}
			if y+f.h > g.bottom+eps {
				newPage(true)
				continue
			}
			y += f.h
		default:
			need := f.h
			if f.keepWithNext && i+1 < len(frags) && frags[i+1].kind == fragNormal {
				need += frags[i+1].h
			}
			if y+need > g.bottom+eps && !empty {
				newPage(true)
				for _, hf := range f.repeat {
					place(hf)
				}
			}
			place(f)
		}
	}
	return pages
}

// blocks lays out a list of blocks at the given width.
func (e *engine) blocks(bs []model.Block, parent style.Computed, maxW float64, g geom) ([]*frag, error) {
	var out []*frag
	for _, b := range bs {
		cm := b.Base()
		c, err := e.doc.Styles.Resolve(parent, cm.Props)
		if err != nil {
			return nil, parser.Errorf(cm.Line, "%v", err)
		}
		if cm.Absolute() {
			x, y := g.left, g.top
			if cm.X != nil {
				x = cm.X.Resolve(g.W)
			}
			if cm.Y != nil {
				y = cm.Y.Resolve(g.H)
			}
			w := g.right - x
			if _, isImg := b.(*model.Image); !isImg && cm.Width != nil {
				w = cm.Width.Resolve(g.width())
			}
			frags, err := e.block(b, c, max(w, 1), g)
			if err != nil {
				return nil, err
			}
			ops, abs, _ := stack(frags)
			out = append(out, &frag{kind: fragAbs, ops: append(shiftAll(ops, x, y), abs...)})
			continue
		}
		w := maxW
		if _, isImg := b.(*model.Image); !isImg && cm.Width != nil {
			w = min(cm.Width.Resolve(maxW), maxW)
		}
		if c.MarginTop > 0 {
			out = append(out, &frag{kind: fragSpacer, h: c.MarginTop})
		}
		frags, err := e.block(b, c, max(w, 1), g)
		if err != nil {
			return nil, err
		}
		out = append(out, frags...)
		if c.MarginBottom > 0 {
			out = append(out, &frag{kind: fragSpacer, h: c.MarginBottom})
		}
	}
	return out, nil
}

func (e *engine) block(b model.Block, c style.Computed, maxW float64, g geom) ([]*frag, error) {
	switch b := b.(type) {
	case *model.Text:
		frags, err := e.text(b, c, maxW)
		if err != nil {
			return nil, parser.Errorf(b.Line, "%v", err)
		}
		return frags, nil
	case *model.Image:
		return e.image(b, c, maxW, g), nil
	case *model.Table:
		return e.table(b, c, maxW, g)
	case *model.Spacer:
		return []*frag{{kind: fragSpacer, h: b.Height}}, nil
	case *model.PageBreak:
		return []*frag{{kind: fragBreak}}, nil
	}
	return nil, nil
}

func (e *engine) image(img *model.Image, c style.Computed, maxW float64, g geom) []*frag {
	nw, nh := e.m.ImageSize(img.Path)
	if nw <= 0 || nh <= 0 {
		return nil
	}
	w, h := nw, nh
	switch {
	case img.Width != nil && img.Height != nil:
		w, h = img.Width.Resolve(maxW), img.Height.Resolve(g.bottom-g.top)
	case img.Width != nil:
		w = img.Width.Resolve(maxW)
		h = w * nh / nw
	case img.Height != nil:
		h = img.Height.Resolve(g.bottom - g.top)
		w = h * nw / nh
	}
	// Never let an image exceed the available box.
	if w > maxW {
		h, w = h*maxW/w, maxW
	}
	if maxH := g.bottom - g.top; h > maxH {
		w, h = w*maxH/h, maxH
	}
	x := 0.0
	switch c.Align {
	case "center":
		x = (maxW - w) / 2
	case "right":
		x = maxW - w
	}
	return []*frag{{h: h, ops: []Op{ImageOp{Path: img.Path, X: x, W: w, H: h}}}}
}

func (e *engine) table(t *model.Table, c style.Computed, maxW float64, g geom) ([]*frag, error) {
	colW := model.ResolveColWidths(t.Cols, maxW)
	pad := t.Padding
	var header []*frag
	var out []*frag
	body := 0
	for _, r := range t.Rows {
		parent := c
		if r.Header {
			parent.Bold = true
		}
		rc, err := e.doc.Styles.Resolve(parent, r.Props)
		if err != nil {
			return nil, parser.Errorf(r.Line, "%v", err)
		}
		rowBg := rc.Bg
		if rowBg == nil {
			if r.Header {
				rowBg = t.HeaderBg
			} else if body%2 == 1 {
				rowBg = t.Stripe
			}
		}

		type cellBox struct {
			x, w   float64
			ops    []Op
			h      float64
			bg     *style.Color
			valign string
		}
		var cells []cellBox
		col := 0
		for _, cell := range r.Cells {
			x := 0.0
			for _, w := range colW[:col] {
				x += w
			}
			w := 0.0
			for _, cw := range colW[col : col+cell.Colspan] {
				w += cw
			}
			col += cell.Colspan
			cc, err := e.doc.Styles.Resolve(rc, cell.Props)
			if err != nil {
				return nil, parser.Errorf(cell.Line, "%v", err)
			}
			frags, err := e.blocks(cell.Blocks, cc, max(w-pad[1]-pad[3], 1), g)
			if err != nil {
				return nil, err
			}
			ops, _, h := stack(frags)
			bg := cc.Bg
			if bg == nil {
				bg = rowBg
			}
			cells = append(cells, cellBox{x: x, w: w, ops: ops, h: h, bg: bg, valign: cell.VAlign})
		}
		// Fill missing trailing cells so the grid stays complete.
		for ; col < len(colW); col++ {
			x := 0.0
			for _, w := range colW[:col] {
				x += w
			}
			cells = append(cells, cellBox{x: x, w: colW[col], bg: rowBg})
		}

		rowH := r.MinHeight
		for _, cb := range cells {
			rowH = max(rowH, cb.h+pad[0]+pad[2])
		}
		var ops []Op
		for _, cb := range cells {
			if cb.bg != nil {
				ops = append(ops, RectOp{X: cb.x, W: cb.w, H: rowH, Fill: cb.bg})
			}
			dy := pad[0]
			switch cb.valign {
			case "middle":
				dy += (rowH - pad[0] - pad[2] - cb.h) / 2
			case "bottom":
				dy = rowH - pad[2] - cb.h
			}
			ops = append(ops, shiftAll(cb.ops, cb.x+pad[3], dy)...)
			if t.Border > 0 {
				bc := t.BorderColor
				ops = append(ops, RectOp{X: cb.x, W: cb.w, H: rowH, Stroke: &bc, LineWidth: t.Border})
			}
		}
		f := &frag{h: rowH, ops: ops}
		if r.Header {
			f.keepWithNext = true
			header = append(header, f)
		} else {
			if t.RepeatHeader {
				f.repeat = header
			}
			body++
		}
		out = append(out, f)
	}
	return out, nil
}

// watermark builds the op for one watermark on a page.
func (e *engine) watermark(w *model.Watermark, g geom) (Op, error) {
	size, bold, gray := 60.0, true, style.Color{R: 150, G: 150, B: 150}
	defaults := style.Props{Size: &size, Bold: &bold, Color: &gray}
	c, err := e.doc.Styles.Resolve(e.root, defaults.Over(w.Props))
	if err != nil {
		return nil, parser.Errorf(w.Line, "%v", err)
	}
	cx, cy := g.W/2, g.H/2
	if w.X != nil {
		cx = w.X.Resolve(g.W)
	}
	if w.Y != nil {
		cy = w.Y.Resolve(g.H)
	}
	var ops []Op
	if w.Text != "" {
		s := e.m.Encode(e.subst(w.Text))
		tw := e.width(&c, s)
		ops = append(ops, TextOp{X: cx - tw/2, Y: cy + 0.35*c.Size, S: s, Font: c.Font, Style: c.FontStyle(), Size: c.Size, Color: c.Color})
	} else {
		nw, nh := e.m.ImageSize(w.Image)
		if nw > 0 && nh > 0 {
			iw, ih := g.W*0.5, 0.0
			switch {
			case w.W != nil && w.H != nil:
				iw, ih = w.W.Resolve(g.W), w.H.Resolve(g.H)
			case w.H != nil:
				ih = w.H.Resolve(g.H)
				iw = ih * nw / nh
			default:
				if w.W != nil {
					iw = w.W.Resolve(g.W)
				}
				ih = iw * nh / nw
			}
			ops = append(ops, ImageOp{Path: w.Image, X: cx - iw/2, Y: cy - ih/2, W: iw, H: ih})
		}
	}
	return GroupOp{Ops: ops, Alpha: w.Opacity, Rotate: w.Rotate, Cx: cx, Cy: cy}, nil
}
