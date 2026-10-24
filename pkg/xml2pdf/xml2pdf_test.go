package xml2pdf

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
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
	out, err := exec.Command("pdftotext", "-layout", path, "-").Output()
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
