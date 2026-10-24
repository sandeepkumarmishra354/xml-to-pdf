// Package style defines text styling properties, named reusable styles and
// the cascade that combines them (document -> page -> element -> inline).
package style

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/units"
)

// Color is an RGB color.
type Color struct{ R, G, B int }

var named = map[string]Color{
	"black":     {0, 0, 0},
	"white":     {255, 255, 255},
	"red":       {255, 0, 0},
	"green":     {0, 128, 0},
	"blue":      {0, 0, 255},
	"yellow":    {255, 255, 0},
	"orange":    {255, 165, 0},
	"purple":    {128, 0, 128},
	"gray":      {128, 128, 128},
	"grey":      {128, 128, 128},
	"lightgray": {211, 211, 211},
	"lightgrey": {211, 211, 211},
	"darkgray":  {64, 64, 64},
	"darkgrey":  {64, 64, 64},
	"navy":      {0, 0, 128},
	"teal":      {0, 128, 128},
	"maroon":    {128, 0, 0},
}

// ParseColor parses "#rgb", "#rrggbb", "rgb(r,g,b)" or a basic color name.
func ParseColor(s string) (Color, error) {
	t := strings.ToLower(strings.TrimSpace(s))
	if c, ok := named[t]; ok {
		return c, nil
	}
	if strings.HasPrefix(t, "#") {
		h := t[1:]
		if len(h) == 3 {
			h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
		}
		if len(h) == 6 {
			v, err := strconv.ParseUint(h, 16, 32)
			if err == nil {
				return Color{int(v >> 16 & 0xff), int(v >> 8 & 0xff), int(v & 0xff)}, nil
			}
		}
	}
	if strings.HasPrefix(t, "rgb(") && strings.HasSuffix(t, ")") {
		parts := strings.Split(t[4:len(t)-1], ",")
		if len(parts) == 3 {
			var c [3]int
			ok := true
			for i, p := range parts {
				v, err := strconv.Atoi(strings.TrimSpace(p))
				if err != nil || v < 0 || v > 255 {
					ok = false
					break
				}
				c[i] = v
			}
			if ok {
				return Color{c[0], c[1], c[2]}, nil
			}
		}
	}
	return Color{}, fmt.Errorf("invalid color %q", s)
}

// Fonts maps accepted font names to the PDF core font family.
var Fonts = map[string]string{
	"helvetica":       "helvetica",
	"arial":           "helvetica",
	"sans":            "helvetica",
	"sans-serif":      "helvetica",
	"times":           "times",
	"times-roman":     "times",
	"times new roman": "times",
	"serif":           "times",
	"courier":         "courier",
	"courier new":     "courier",
	"monospace":       "courier",
	"zapfdingbats":    "zapfdingbats",
}

// Alignments accepted by the align property.
var Alignments = map[string]bool{"left": true, "center": true, "right": true, "justify": true}

// Props is a set of optionally specified style properties.
type Props struct {
	Refs       []string // named styles applied first, in order
	Font       *string
	Size       *float64
	Bold       *bool
	Italic     *bool
	Underline  *bool
	Color      *Color
	Align      *string
	LineHeight *float64
	// Non-inherited properties.
	MarginTop    *float64
	MarginBottom *float64
	Bg           *Color
}

// PropNames lists the attribute names understood by ParseProps.
var PropNames = []string{
	"style", "font", "size", "bold", "italic", "underline", "color", "align",
	"line-height", "margin-top", "margin-bottom", "bg",
}

// ParseProps reads style attributes using get, which returns an attribute
// value and whether it is present.
func ParseProps(get func(string) (string, bool)) (Props, error) {
	var p Props
	if v, ok := get("style"); ok {
		p.Refs = strings.Fields(v)
	}
	if v, ok := get("font"); ok {
		f, ok := Fonts[strings.ToLower(strings.TrimSpace(v))]
		if !ok {
			return p, fmt.Errorf("unknown font %q (available: helvetica, times, courier, zapfdingbats)", v)
		}
		p.Font = &f
	}
	if v, ok := get("size"); ok {
		f, err := units.ParseAbs(v)
		if err != nil || f <= 0 {
			return p, fmt.Errorf("invalid size %q", v)
		}
		p.Size = &f
	}
	for _, b := range []struct {
		name string
		dst  **bool
	}{{"bold", &p.Bold}, {"italic", &p.Italic}, {"underline", &p.Underline}} {
		if v, ok := get(b.name); ok {
			x, err := ParseBool(v)
			if err != nil {
				return p, fmt.Errorf("%s: %v", b.name, err)
			}
			*b.dst = &x
		}
	}
	for _, c := range []struct {
		name string
		dst  **Color
	}{{"color", &p.Color}, {"bg", &p.Bg}} {
		if v, ok := get(c.name); ok {
			col, err := ParseColor(v)
			if err != nil {
				return p, fmt.Errorf("%s: %v", c.name, err)
			}
			*c.dst = &col
		}
	}
	if v, ok := get("align"); ok {
		a := strings.ToLower(strings.TrimSpace(v))
		if !Alignments[a] {
			return p, fmt.Errorf("invalid align %q (left, center, right, justify)", v)
		}
		p.Align = &a
	}
	if v, ok := get("line-height"); ok {
		f, err := strconv.ParseFloat(strings.TrimSpace(v), 64)
		if err != nil || f <= 0 {
			return p, fmt.Errorf("invalid line-height %q (a multiple of the font size, e.g. 1.2)", v)
		}
		p.LineHeight = &f
	}
	for _, m := range []struct {
		name string
		dst  **float64
	}{{"margin-top", &p.MarginTop}, {"margin-bottom", &p.MarginBottom}} {
		if v, ok := get(m.name); ok {
			f, err := units.ParseAbs(v)
			if err != nil {
				return p, fmt.Errorf("%s: %v", m.name, err)
			}
			*m.dst = &f
		}
	}
	return p, nil
}

// ParseBool accepts true/false, yes/no, 1/0.
func ParseBool(v string) (bool, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "yes", "1":
		return true, nil
	case "false", "no", "0":
		return false, nil
	}
	return false, fmt.Errorf("invalid boolean %q", v)
}

// Over returns p with every property set in q overriding p's.
func (p Props) Over(q Props) Props {
	r := p
	r.Refs = append(append([]string(nil), p.Refs...), q.Refs...)
	if q.Font != nil {
		r.Font = q.Font
	}
	if q.Size != nil {
		r.Size = q.Size
	}
	if q.Bold != nil {
		r.Bold = q.Bold
	}
	if q.Italic != nil {
		r.Italic = q.Italic
	}
	if q.Underline != nil {
		r.Underline = q.Underline
	}
	if q.Color != nil {
		r.Color = q.Color
	}
	if q.Align != nil {
		r.Align = q.Align
	}
	if q.LineHeight != nil {
		r.LineHeight = q.LineHeight
	}
	if q.MarginTop != nil {
		r.MarginTop = q.MarginTop
	}
	if q.MarginBottom != nil {
		r.MarginBottom = q.MarginBottom
	}
	if q.Bg != nil {
		r.Bg = q.Bg
	}
	return r
}

// Computed is a fully resolved style.
type Computed struct {
	Font       string
	Size       float64
	Bold       bool
	Italic     bool
	Underline  bool
	Color      Color
	Align      string
	LineHeight float64

	MarginTop    float64
	MarginBottom float64
	Bg           *Color
}

// Default is the root style used when the document sets nothing.
var Default = Computed{Font: "helvetica", Size: 11, Align: "left", LineHeight: 1.2}

// FontStyle returns the PDF font style string ("", "B", "I" or "BI").
func (c Computed) FontStyle() string {
	s := ""
	if c.Bold {
		s += "B"
	}
	if c.Italic {
		s += "I"
	}
	return s
}

// LineH returns the line height in points.
func (c Computed) LineH() float64 { return c.Size * c.LineHeight }

func (c *Computed) apply(p Props) {
	if p.Font != nil {
		c.Font = *p.Font
	}
	if p.Size != nil {
		c.Size = *p.Size
	}
	if p.Bold != nil {
		c.Bold = *p.Bold
	}
	if p.Italic != nil {
		c.Italic = *p.Italic
	}
	if p.Underline != nil {
		c.Underline = *p.Underline
	}
	if p.Color != nil {
		c.Color = *p.Color
	}
	if p.Align != nil {
		c.Align = *p.Align
	}
	if p.LineHeight != nil {
		c.LineHeight = *p.LineHeight
	}
	if p.MarginTop != nil {
		c.MarginTop = *p.MarginTop
	}
	if p.MarginBottom != nil {
		c.MarginBottom = *p.MarginBottom
	}
	if p.Bg != nil {
		c.Bg = p.Bg
	}
}

// Def is a named style declared in <resources>.
type Def struct {
	ID      string
	Extends string
	Props   Props
	Line    int

	flat     *Props
	visiting bool
}

// Registry holds named styles.
type Registry struct {
	defs map[string]*Def
}

// NewRegistry creates an empty registry.
func NewRegistry() *Registry { return &Registry{defs: map[string]*Def{}} }

// Add registers a style definition.
func (r *Registry) Add(d *Def) error {
	if _, dup := r.defs[d.ID]; dup {
		return fmt.Errorf("style %q defined twice", d.ID)
	}
	r.defs[d.ID] = d
	return nil
}

// Has reports whether a style is defined.
func (r *Registry) Has(id string) bool { _, ok := r.defs[id]; return ok }

// Flatten resolves `extends` chains and style references inside definitions,
// reporting unknown parents and cycles.
func (r *Registry) Flatten() error {
	for _, d := range r.defs {
		if _, err := r.flat(d); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) flat(d *Def) (Props, error) {
	if d.flat != nil {
		return *d.flat, nil
	}
	if d.visiting {
		return Props{}, fmt.Errorf("style %q: circular extends", d.ID)
	}
	d.visiting = true
	defer func() { d.visiting = false }()
	var base Props
	parents := d.Props.Refs
	if d.Extends != "" {
		parents = append(strings.Fields(d.Extends), parents...)
	}
	for _, id := range parents {
		pd, ok := r.defs[id]
		if !ok {
			return Props{}, fmt.Errorf("style %q extends unknown style %q", d.ID, id)
		}
		pp, err := r.flat(pd)
		if err != nil {
			return Props{}, err
		}
		base = base.Over(pp)
	}
	own := d.Props
	own.Refs = nil
	f := base.Over(own)
	f.Refs = nil
	d.flat = &f
	return f, nil
}

// Resolve computes the style of an element whose parent style is parent and
// whose own properties are p. Inheritable properties (font, color, ...) come
// from the parent; margins and background do not inherit.
func (r *Registry) Resolve(parent Computed, p Props) (Computed, error) {
	c := parent
	c.MarginTop, c.MarginBottom, c.Bg = 0, 0, nil
	for _, id := range p.Refs {
		d, ok := r.defs[id]
		if !ok {
			return c, fmt.Errorf("unknown style %q", id)
		}
		fp, err := r.flat(d)
		if err != nil {
			return c, err
		}
		c.apply(fp)
	}
	c.apply(p)
	return c, nil
}
