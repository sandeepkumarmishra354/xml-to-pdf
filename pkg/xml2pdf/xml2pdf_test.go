package xml2pdf

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
	"testing/fstest"
	"time"
)

var countRe = regexp.MustCompile(`/Type /Pages\s*/Kids \[[^\]]*\]\s*/Count (\d+)`)

func pageCount(t *testing.T, pdf []byte) string {
	t.Helper()
	m := countRe.FindSubmatch(pdf)
	if m == nil {
		t.Fatal("page count not found in PDF")
	}
	return string(m[1])
}

// pdfText extracts text with pdftotext when available.
func pdfText(t *testing.T, path string) string {
	t.Helper()
	if _, err := exec.LookPath("pdftotext"); err != nil {
		t.Skip("pdftotext not installed")
	}
	out, err := exec.Command("pdftotext", "-enc", "UTF-8", "-layout", path, "-").Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestExamples(t *testing.T) {
	data, err := LoadData("../../examples/invoice.json")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		name  string
		opts  Options
		pages string
		want  []string
	}{
		{"invoice", Options{Data: data, Vars: map[string]string{"status": "PAID"}}, "1",
			[]string{"INV-2026-0042", "Globex Ltd.", "$1,800.00", "Domain renewal", "$2,500.00", "Page 1 of 1"}},
		{"report", Options{}, "5",
			[]string{"Quarterly Operations Report", "INC-124", "Page 5 of 5", "Zürich", "€100"}},
	}
	dir := t.TempDir()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := filepath.Join(dir, c.name+".pdf")
			if err := RenderFile("../../examples/"+c.name+".xml", out, c.opts); err != nil {
				t.Fatal(err)
			}
			b, err := os.ReadFile(out)
			if err != nil {
				t.Fatal(err)
			}
			if got := pageCount(t, b); got != c.pages {
				t.Errorf("pages = %s, want %s", got, c.pages)
			}
			text := pdfText(t, out)
			for _, w := range c.want {
				if !strings.Contains(text, w) {
					t.Errorf("output missing %q", w)
				}
			}
		})
	}
}

func TestRenderErrors(t *testing.T) {
	cases := map[string]string{
		`<document><page>${nope}</page></document>`: `undefined variable "nope"`,
		`<document><page><text>x</page></document>`: "line 1",
		`not xml`: "",
	}
	for src, want := range cases {
		err := Render(strings.NewReader(src), &bytes.Buffer{}, Options{})
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want error containing %q", src, err, want)
		}
	}
}

func TestVarsOverrideConstants(t *testing.T) {
	src := `<document><constants><const name="x" value="const"/></constants><page>${x}</page></document>`
	var a, b bytes.Buffer
	if err := Render(strings.NewReader(src), &a, Options{}); err != nil {
		t.Fatal(err)
	}
	if err := Render(strings.NewReader(src), &b, Options{Vars: map[string]string{"x": "override"}}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(a.Bytes(), []byte("%PDF")) || bytes.Equal(a.Bytes(), b.Bytes()) {
		t.Error("expected two different PDFs")
	}
}

func TestConcurrentDeterministic(t *testing.T) {
	data, err := LoadData("../../examples/invoice.json")
	if err != nil {
		t.Fatal(err)
	}
	src, err := os.ReadFile("../../examples/invoice.xml")
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{Data: data, BaseDir: "../../examples", CreationDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	outs := make([][]byte, 8)
	var wg sync.WaitGroup
	for i := range outs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			var buf bytes.Buffer
			if err := Render(bytes.NewReader(src), &buf, opts); err != nil {
				t.Error(err)
			}
			outs[i] = buf.Bytes()
		}(i)
	}
	wg.Wait()
	for i := 1; i < len(outs); i++ {
		if !bytes.Equal(outs[0], outs[i]) {
			t.Fatalf("render %d produced different bytes", i)
		}
	}
}

func TestParseData(t *testing.T) {
	m, err := ParseData(strings.NewReader(`{"price": 12.50, "n": {"x": [1, 2]}}`))
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprint(m["price"]); got != "12.50" {
		t.Errorf("price = %s, want 12.50 (formatting kept)", got)
	}
	for in, want := range map[string]string{
		``:                  "no JSON data",
		`{bad`:              "invalid JSON",
		`[1, 2]`:            "must be a JSON object",
		`"text"`:            "must be a JSON object",
		`null`:              "must be a JSON object",
		`{"a": 1} {"b": 2}`: "unexpected data after the object",
	} {
		if _, err := ParseData(strings.NewReader(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ParseData(%q) = %v, want error containing %q", in, err, want)
		}
	}
}

func TestMergeData(t *testing.T) {
	base := map[string]any{
		"a":    "base",
		"keep": "yes",
		"obj":  map[string]any{"x": 1, "y": 2},
		"list": []any{1, 2, 3},
		"gone": "value",
	}
	overlay := map[string]any{
		"a":    "overlay",
		"obj":  map[string]any{"y": 20, "z": 30},
		"list": []any{9},
		"gone": nil,
		"new":  true,
	}
	got := MergeData(base, overlay)
	want := map[string]any{
		"a":    "overlay",
		"keep": "yes",
		"obj":  map[string]any{"x": 1, "y": 20, "z": 30},
		"list": []any{9},
		"gone": nil,
		"new":  true,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("MergeData =\n  %v\nwant\n  %v", got, want)
	}
	if base["a"] != "base" || len(base["obj"].(map[string]any)) != 2 {
		t.Error("MergeData modified its base argument")
	}
	if got := MergeData(nil, map[string]any{"k": 1}); got["k"] != 1 {
		t.Errorf("MergeData(nil, ...) = %v", got)
	}
}

func TestImagesFromFS(t *testing.T) {
	logo, err := os.ReadFile("../../examples/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	fsys := fstest.MapFS{
		"logo.png":        {Data: logo},
		"assets/logo.png": {Data: logo},
	}
	render := func(src string, opts Options) error {
		opts.FS = fsys
		var buf bytes.Buffer
		err := Render(strings.NewReader(src), &buf, opts)
		if err == nil && !bytes.Contains(buf.Bytes(), []byte("/Subtype /Image")) {
			t.Errorf("%s: no image in output", src)
		}
		return err
	}
	ok := []struct {
		src  string
		opts Options
	}{
		{`<document><page><img src="logo.png" width="50"/></page></document>`, Options{}},
		{`<document><resources><image id="l" src="./assets/logo.png"/></resources><page><img ref="l"/></page></document>`, Options{}},
		{`<document><page><img src="logo.png"/></page></document>`, Options{BaseDir: "assets"}},
		{`<document><watermark image="l"/><resources><image id="l" src="logo.png"/></resources><page>x</page></document>`, Options{}},
	}
	for _, c := range ok {
		if err := render(c.src, c.opts); err != nil {
			t.Errorf("%s: %v", c.src, err)
		}
	}
	bad := map[string]string{
		`<document><page><img src="missing.png"/></page></document>`:   "image not found: missing.png",
		`<document><page><img src="/etc/logo.png"/></page></document>`: "image path must be relative",
		`<document><page><img src="../logo.png"/></page></document>`:   "invalid image path",
	}
	for src, want := range bad {
		if err := render(src, Options{}); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: err = %v, want containing %q", src, err, want)
		}
	}
}
