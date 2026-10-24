// Package xml2pdf renders XML document descriptions into PDF files.
//
//	err := xml2pdf.RenderFile("invoice.xml", "invoice.pdf", xml2pdf.Options{
//		Vars: map[string]string{"invoice_no": "42"},
//	})
package xml2pdf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
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
	// FS, when set, is where image files are read from instead of the OS
	// filesystem, e.g. an embed.FS or fstest.MapFS. Image paths in the
	// document are then slash-separated and relative to BaseDir inside FS.
	FS fs.FS
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
	doc, err := model.Build(root, base, opts.FS)
	if err != nil {
		return err
	}
	return render.Render(doc, w, render.Options{CreationDate: opts.CreationDate, FS: opts.FS})
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

// LoadData reads a JSON object from a file for use as Options.Data.
// Numbers keep their original formatting (12.50 stays "12.50").
func LoadData(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	m, err := ParseData(f)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return m, nil
}

// ParseData decodes a single JSON object for use as Options.Data. Numbers
// keep their original formatting (12.50 stays "12.50").
func ParseData(r io.Reader) (map[string]any, error) {
	d := json.NewDecoder(r)
	d.UseNumber()
	var v any
	if err := d.Decode(&v); err != nil {
		if errors.Is(err, io.EOF) {
			return nil, errors.New("no JSON data")
		}
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil, errors.New("data must be a JSON object, like {\"name\": \"value\"}")
	}
	if err := d.Decode(new(any)); !errors.Is(err, io.EOF) {
		return nil, errors.New("invalid JSON: unexpected data after the object")
	}
	return m, nil
}

// MergeData deep-merges overlay into base and returns the result: nested
// objects are merged key by key, while arrays, scalars and null in overlay
// replace the value in base. Neither input is modified.
func MergeData(base, overlay map[string]any) map[string]any {
	out := make(map[string]any, len(base)+len(overlay))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range overlay {
		if bm, ok := out[k].(map[string]any); ok {
			if om, ok := v.(map[string]any); ok {
				out[k] = MergeData(bm, om)
				continue
			}
		}
		out[k] = v
	}
	return out
}
