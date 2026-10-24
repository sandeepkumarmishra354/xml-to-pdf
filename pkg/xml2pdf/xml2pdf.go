// Package xml2pdf renders XML document descriptions into PDF files.
//
//	err := xml2pdf.RenderFile("invoice.xml", "invoice.pdf", xml2pdf.Options{
//		Vars: map[string]string{"invoice_no": "42"},
//	})
package xml2pdf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/model"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/render"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/vars"
)

// Options configures rendering.
type Options struct {
	// BaseDir resolves relative image paths. RenderFile defaults it to the
	// directory of the input file; Render defaults it to the working directory.
	BaseDir string
	// Data provides values for ${name} lookups, e.g. decoded JSON.
	// Nested values are addressed as ${a.b} and ${list.0.c}.
	Data map[string]any
	// Vars override Data and document constants.
	Vars map[string]string
	// CreationDate is written to the PDF metadata; zero means now.
	CreationDate time.Time
}

// Render reads an XML document from r and writes the PDF to w.
func Render(r io.Reader, w io.Writer, opts Options) error {
	root, err := parser.Parse(r)
	if err != nil {
		return err
	}
	consts, err := vars.CollectConstants(root)
	if err != nil {
		return err
	}
	if err := vars.ExpandTree(root, vars.New(opts.Vars, opts.Data, consts)); err != nil {
		return err
	}
	base := opts.BaseDir
	if base == "" {
		base = "."
	}
	doc, err := model.Build(root, base)
	if err != nil {
		return err
	}
	return render.Render(doc, w, render.Options{CreationDate: opts.CreationDate})
}

// RenderFile renders the XML file at in to the PDF file at out. The output
// file is only written if rendering succeeds.
func RenderFile(in, out string, opts Options) error {
	f, err := os.Open(in)
	if err != nil {
		return err
	}
	defer f.Close()
	if opts.BaseDir == "" {
		opts.BaseDir = filepath.Dir(in)
	}
	var buf bytes.Buffer
	if err := Render(f, &buf, opts); err != nil {
		return fmt.Errorf("%s: %w", in, err)
	}
	return os.WriteFile(out, buf.Bytes(), 0o644)
}

// LoadData reads a JSON object for use as Options.Data. Numbers keep their
// original formatting (12.50 stays "12.50").
func LoadData(path string) (map[string]any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}
