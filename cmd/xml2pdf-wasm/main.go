//go:build js && wasm

// Command xml2pdf-wasm exposes the renderer to JavaScript for the browser
// playground. It registers a global object:
//
//	xml2pdf.render(xml, json, files) -> {ok, pdf, error, line, ms}
//
// where json is a JSON object string ("" for none), files maps file names
// to Uint8Array image data, and pdf is a Uint8Array.
package main

import (
	"bytes"
	"errors"
	"strings"
	"syscall/js"
	"testing/fstest"
	"time"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
	"github.com/sandeepkumarmishra354/xml-to-pdf/pkg/xml2pdf"
)

var version = "dev"

func main() {
	api := js.Global().Get("Object").New()
	api.Set("version", version)
	api.Set("render", js.FuncOf(render))
	js.Global().Set("xml2pdf", api)
	// Signal readiness to the page, then keep the Go runtime alive.
	if ready := js.Global().Get("onXml2pdfReady"); ready.Type() == js.TypeFunction {
		ready.Invoke()
	}
	select {}
}

func render(_ js.Value, args []js.Value) any {
	start := time.Now()
	result := js.Global().Get("Object").New()
	fail := func(err error) any {
		result.Set("ok", false)
		result.Set("error", err.Error())
		var pe *parser.Error
		if errors.As(err, &pe) && pe.Line > 0 {
			result.Set("line", pe.Line)
		}
		return result
	}
	if len(args) < 1 || args[0].Type() != js.TypeString {
		return fail(errors.New("render(xml, json, files): xml must be a string"))
	}

	opts := xml2pdf.Options{
		BaseDir:      ".",
		FS:           fstest.MapFS{},
		CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	if len(args) > 1 && args[1].Type() == js.TypeString && strings.TrimSpace(args[1].String()) != "" {
		data, err := xml2pdf.ParseData(strings.NewReader(args[1].String()))
		if err != nil {
			return fail(errors.New("data: " + err.Error()))
		}
		opts.Data = data
	}
	if len(args) > 2 && args[2].Type() == js.TypeObject {
		files := opts.FS.(fstest.MapFS)
		keys := js.Global().Get("Object").Call("keys", args[2])
		for i := 0; i < keys.Length(); i++ {
			name := keys.Index(i).String()
			src := args[2].Get(name)
			buf := make([]byte, src.Get("length").Int())
			js.CopyBytesToGo(buf, src)
			files[name] = &fstest.MapFile{Data: buf}
		}
	}

	var out bytes.Buffer
	if err := xml2pdf.Render(strings.NewReader(args[0].String()), &out, opts); err != nil {
		return fail(err)
	}
	pdf := js.Global().Get("Uint8Array").New(out.Len())
	js.CopyBytesToJS(pdf, out.Bytes())
	result.Set("ok", true)
	result.Set("pdf", pdf)
	result.Set("ms", float64(time.Since(start).Microseconds())/1000)
	return result
}
