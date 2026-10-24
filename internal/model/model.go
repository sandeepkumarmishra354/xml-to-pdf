// Package model defines the typed document tree built from the parsed XML.
package model

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/style"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/units"
)

// Size is a page size in points.
type Size struct{ W, H float64 }

// Document is the root of a typed document.
type Document struct {
	Title, Author, Subject, Keywords string

	Size   Size       // default page size (already oriented)
	Margin [4]float64 // top, right, bottom, left
	Props  style.Props

	Styles     *style.Registry
	Images     map[string]string // resource id -> file path
	Watermarks []*Watermark
	Header     *Band
	Footer     *Band
	Pages      []*Page
}

// ImagePaths returns every image file referenced anywhere in the document.
func (d *Document) ImagePaths() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	var walk func([]Block)
	walk = func(bs []Block) {
		for _, b := range bs {
			switch b := b.(type) {
			case *Image:
				add(b.Path)
			case *Table:
				for _, r := range b.Rows {
					for _, c := range r.Cells {
						walk(c.Blocks)
					}
				}
			}
		}
	}
	for _, p := range d.Images {
		add(p)
	}
	for _, w := range d.Watermarks {
		add(w.Image)
	}
	if d.Header != nil {
		walk(d.Header.Blocks)
	}
	if d.Footer != nil {
		walk(d.Footer.Blocks)
	}
	for _, p := range d.Pages {
		walk(p.Blocks)
	}
	return out
}

// Page is one <page> element. Content that overflows continues on further
// physical pages with the same settings.
type Page struct {
	Line        int
	Size        Size
	Margin      [4]float64
	Props       style.Props
	Blocks      []Block
	NoHeader    bool
	NoFooter    bool
	NoWatermark bool
}

// Band is a <header> or <footer>, repeated on every selected page.
// Text inside may use {page} and {pages}.
type Band struct {
	Line   int
	Props  style.Props
	Blocks []Block
	Height *float64 // nil: measured from content
	Gap    float64  // space between band and body content
	Pages  PageSel
}

// Watermark is a text or image drawn on selected pages.
type Watermark struct {
	Line    int
	Text    string
	Image   string // file path
	Props   style.Props
	Opacity float64
	Rotate  float64
	Front   bool
	Pages   PageSel
	X, Y    *units.Length // center point, default page center
	W, H    *units.Length // image size
}

// Block is a flow element.
type Block interface{ Base() *Common }

// Common holds attributes shared by all blocks.
type Common struct {
	Line  int
	Props style.Props
	X, Y  *units.Length // absolute position (page coordinates) when set
	Width *units.Length // box width for text/table, image width for img
}

// Base implements Block.
func (c *Common) Base() *Common { return c }

// Absolute reports whether the block is positioned absolutely.
func (c *Common) Absolute() bool { return c.X != nil || c.Y != nil }

// Text is a paragraph of inline content.
type Text struct {
	Common
	Inlines []Inline
}

// Inline is a run of text, a line break, or a styled span with children.
type Inline struct {
	Text     string
	Br       bool
	Props    *style.Props // set for styled spans
	Children []Inline
}

// Image is an <img>.
type Image struct {
	Common
	Path   string
	Height *units.Length
}

// Table is a <table>.
type Table struct {
	Common
	Cols         []ColWidth // nil: equal widths
	Border       float64
	BorderColor  style.Color
	Padding      [4]float64
	HeaderBg     *style.Color
	Stripe       *style.Color
	RepeatHeader bool
	Rows         []*Row
}

// ColWidth is a column width: fractional ("2*"), fixed ("80", "30mm") or a
// percentage of the table width ("25%").
type ColWidth struct {
	Frac   float64
	Len    units.Length
	IsFrac bool
}

// ParseColWidths parses a comma separated list of column widths.
func ParseColWidths(s string) ([]ColWidth, error) {
	var out []ColWidth
	for _, p := range strings.Split(s, ",") {
		p = strings.TrimSpace(p)
		if strings.HasSuffix(p, "*") {
			f := 1.0
			if num := strings.TrimSuffix(p, "*"); num != "" {
				v, err := strconv.ParseFloat(num, 64)
				if err != nil || v <= 0 {
					return nil, fmt.Errorf("invalid column width %q", p)
				}
				f = v
			}
			out = append(out, ColWidth{Frac: f, IsFrac: true})
			continue
		}
		l, err := units.Parse(p)
		if err != nil || l.Value < 0 {
			return nil, fmt.Errorf("invalid column width %q", p)
		}
		out = append(out, ColWidth{Len: l})
	}
	return out, nil
}

// ResolveColWidths computes column widths in points for a table of the given
// width: fixed and percentage columns first, then fractions share the rest.
func ResolveColWidths(cols []ColWidth, tableW float64) []float64 {
	out := make([]float64, len(cols))
	used, fracs := 0.0, 0.0
	for i, c := range cols {
		if c.IsFrac {
			fracs += c.Frac
			continue
		}
		out[i] = c.Len.Resolve(tableW)
		used += out[i]
	}
	if rest := tableW - used; fracs > 0 && rest > 0 {
		for i, c := range cols {
			if c.IsFrac {
				out[i] = rest * c.Frac / fracs
			}
		}
	}
	return out
}

// Row is a table <row>.
type Row struct {
	Line      int
	Props     style.Props
	Header    bool
	MinHeight float64
	Cells     []*Cell
}

// Cell is a table <cell>.
type Cell struct {
	Line    int
	Props   style.Props
	Colspan int
	VAlign  string // top, middle, bottom
	Blocks  []Block
}

// Spacer is vertical empty space.
type Spacer struct {
	Common
	Height float64
}

// PageBreak forces a new page.
type PageBreak struct{ Common }

// PageSel selects page numbers: "all", "first", "last", "odd", "even",
// "3", "2-5", "2-" (open ended), comma separated.
type PageSel struct{ items []selItem }

type selItem struct {
	kind     string // all, first, last, odd, even, range
	from, to int    // to == 0: open ended
}

// ParsePageSel parses a page selector. An empty string selects all pages.
func ParsePageSel(s string) (PageSel, error) {
	var ps PageSel
	for _, p := range strings.Split(s, ",") {
		p = strings.ToLower(strings.TrimSpace(p))
		switch p {
		case "", "all", "first", "last", "odd", "even":
			if p == "" {
				p = "all"
			}
			ps.items = append(ps.items, selItem{kind: p})
			continue
		}
		from, to, isRange := strings.Cut(p, "-")
		a, err := strconv.Atoi(strings.TrimSpace(from))
		if err != nil || a < 1 {
			return ps, fmt.Errorf("invalid page selector %q", s)
		}
		b := a
		if isRange {
			b = 0
			if t := strings.TrimSpace(to); t != "" {
				b, err = strconv.Atoi(t)
				if err != nil || b < a {
					return ps, fmt.Errorf("invalid page selector %q", s)
				}
			}
		}
		ps.items = append(ps.items, selItem{kind: "range", from: a, to: b})
	}
	return ps, nil
}

// Match reports whether page n (1-based) of total is selected.
func (ps PageSel) Match(n, total int) bool {
	if len(ps.items) == 0 {
		return true
	}
	for _, it := range ps.items {
		switch it.kind {
		case "all":
			return true
		case "first":
			if n == 1 {
				return true
			}
		case "last":
			if n == total {
				return true
			}
		case "odd":
			if n%2 == 1 {
				return true
			}
		case "even":
			if n%2 == 0 {
				return true
			}
		case "range":
			if n >= it.from && (it.to == 0 || n <= it.to) {
				return true
			}
		}
	}
	return false
}
