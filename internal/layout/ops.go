// Package layout turns a model.Document into positioned drawing operations,
// one list per physical page. Coordinates are in points with the origin at
// the top-left corner of the page and y growing downwards.
package layout

import "github.com/sandeepkumarmishra354/xml-to-pdf/internal/style"

// Op is a drawing operation.
type Op interface {
	shift(dx, dy float64) Op
}

// TextOp draws a single run of already-encoded text with its baseline at Y.
type TextOp struct {
	X, Y  float64
	S     string
	Font  string
	Style string // "", "B", "I", "BI"
	Size  float64
	Color style.Color
}

// ImageOp draws an image file into a box.
type ImageOp struct {
	Path       string
	X, Y, W, H float64
}

// RectOp draws a filled and/or stroked rectangle.
type RectOp struct {
	X, Y, W, H float64
	Fill       *style.Color
	Stroke     *style.Color
	LineWidth  float64
}

// GroupOp draws child ops with opacity and rotation (degrees,
// counter-clockwise) around (Cx, Cy).
type GroupOp struct {
	Ops            []Op
	Alpha          float64
	Rotate, Cx, Cy float64
}

func (o TextOp) shift(dx, dy float64) Op  { o.X += dx; o.Y += dy; return o }
func (o ImageOp) shift(dx, dy float64) Op { o.X += dx; o.Y += dy; return o }
func (o RectOp) shift(dx, dy float64) Op  { o.X += dx; o.Y += dy; return o }
func (o GroupOp) shift(dx, dy float64) Op {
	o.Ops = shiftAll(o.Ops, dx, dy)
	o.Cx += dx
	o.Cy += dy
	return o
}

func shiftAll(ops []Op, dx, dy float64) []Op {
	if dx == 0 && dy == 0 {
		return ops
	}
	out := make([]Op, len(ops))
	for i, o := range ops {
		out[i] = o.shift(dx, dy)
	}
	return out
}

// Page is a laid out physical page.
type Page struct {
	W, H float64
	Ops  []Op
}

// Measurer provides font metrics and image sizes from the PDF backend.
type Measurer interface {
	// Encode converts UTF-8 text to the encoding used by the fonts.
	Encode(s string) string
	// Width returns the width in points of encoded text.
	Width(font, style string, size float64, s string) float64
	// ImageSize returns the natural size of an image in points.
	ImageSize(path string) (w, h float64)
}
