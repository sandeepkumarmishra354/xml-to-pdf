// Package render paints laid out pages into a PDF using fpdf.
package render

import (
	"fmt"
	"io"
	"io/fs"
	"path"
	"strings"
	"time"

	"github.com/go-pdf/fpdf"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/layout"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/model"
)

// Options controls PDF output.
type Options struct {
	// CreationDate is stored in the PDF metadata; zero means now.
	CreationDate time.Time
	// FS, when set, holds the image files referenced by the document.
	FS fs.FS
}

type measurer struct {
	pdf *fpdf.Fpdf
	tr  func(string) string
}

func (m *measurer) Encode(s string) string { return m.tr(s) }

func (m *measurer) Width(font, style string, size float64, s string) float64 {
	m.pdf.SetFont(font, style, size)
	return m.pdf.GetStringWidth(s)
}

func (m *measurer) ImageSize(path string) (float64, float64) {
	info := m.pdf.GetImageInfo(path)
	if info == nil {
		return 0, 0
	}
	return info.Extent()
}

// Render lays out doc and writes the PDF to w.
func Render(doc *model.Document, w io.Writer, opts Options) error {
	pdf := fpdf.NewCustom(&fpdf.InitType{UnitStr: "pt", Size: fpdf.SizeType{Wd: doc.Size.W, Ht: doc.Size.H}})
	pdf.SetAutoPageBreak(false, 0)
	pdf.SetMargins(0, 0, 0)
	pdf.SetCellMargin(0)
	pdf.SetTitle(doc.Title, true)
	pdf.SetAuthor(doc.Author, true)
	pdf.SetSubject(doc.Subject, true)
	pdf.SetKeywords(doc.Keywords, true)
	pdf.SetCreator("xml2pdf", true)
	// Order internal catalogs (fonts, images) consistently so identical
	// input produces identical bytes.
	pdf.SetCatalogSort(true)
	if !opts.CreationDate.IsZero() {
		pdf.SetCreationDate(opts.CreationDate)
		pdf.SetModificationDate(opts.CreationDate)
	}

	for _, p := range doc.ImagePaths() {
		if opts.FS != nil {
			if err := registerFromFS(pdf, opts.FS, p); err != nil {
				return fmt.Errorf("image %s: %w", p, err)
			}
			continue
		}
		pdf.RegisterImageOptions(p, fpdf.ImageOptions{ReadDpi: true})
		if err := pdf.Error(); err != nil {
			return fmt.Errorf("image %s: %w", p, err)
		}
	}

	m := &measurer{pdf: pdf, tr: pdf.UnicodeTranslatorFromDescriptor("")}
	pages, err := layout.Layout(doc, m)
	if err != nil {
		return err
	}
	for _, p := range pages {
		pdf.AddPageFormat("P", fpdf.SizeType{Wd: p.W, Ht: p.H})
		paint(pdf, p.Ops)
	}
	if err := pdf.Error(); err != nil {
		return err
	}
	return pdf.Output(w)
}

func registerFromFS(pdf *fpdf.Fpdf, fsys fs.FS, name string) error {
	f, err := fsys.Open(name)
	if err != nil {
		return err
	}
	defer f.Close()
	ext := strings.TrimPrefix(strings.ToLower(path.Ext(name)), ".")
	if ext == "" {
		return fmt.Errorf("image file has no extension")
	}
	pdf.RegisterImageOptionsReader(name, fpdf.ImageOptions{ImageType: ext, ReadDpi: true}, f)
	return pdf.Error()
}

func paint(pdf *fpdf.Fpdf, ops []layout.Op) {
	for _, op := range ops {
		switch o := op.(type) {
		case layout.TextOp:
			pdf.SetFont(o.Font, o.Style, o.Size)
			pdf.SetTextColor(o.Color.R, o.Color.G, o.Color.B)
			pdf.Text(o.X, o.Y, o.S)
		case layout.ImageOp:
			pdf.ImageOptions(o.Path, o.X, o.Y, o.W, o.H, false, fpdf.ImageOptions{}, 0, "")
		case layout.RectOp:
			style := ""
			if o.Fill != nil {
				pdf.SetFillColor(o.Fill.R, o.Fill.G, o.Fill.B)
				style += "F"
			}
			if o.Stroke != nil {
				pdf.SetDrawColor(o.Stroke.R, o.Stroke.G, o.Stroke.B)
				pdf.SetLineWidth(o.LineWidth)
				style += "D"
			}
			if style != "" {
				pdf.Rect(o.X, o.Y, o.W, o.H, style)
			}
		case layout.GroupOp:
			pdf.TransformBegin()
			if o.Rotate != 0 {
				pdf.TransformRotate(o.Rotate, o.Cx, o.Cy)
			}
			if o.Alpha < 1 {
				pdf.SetAlpha(o.Alpha, "Normal")
			}
			paint(pdf, o.Ops)
			if o.Alpha < 1 {
				pdf.SetAlpha(1, "Normal")
			}
			pdf.TransformEnd()
		}
	}
}
