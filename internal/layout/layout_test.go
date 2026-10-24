package layout

import (
	"fmt"
	"strings"
	"testing"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/model"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
)

// fakeMeasurer gives every character a width of half the font size.
type fakeMeasurer struct{}

func (fakeMeasurer) Encode(s string) string { return s }
func (fakeMeasurer) Width(_, _ string, size float64, s string) float64 {
	return float64(len(s)) * size / 2
}
func (fakeMeasurer) ImageSize(string) (float64, float64) { return 100, 50 }

func layoutSrc(t *testing.T, src string) []Page {
	t.Helper()
	root, err := parser.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := model.Build(root, ".")
	if err != nil {
		t.Fatal(err)
	}
	pages, err := Layout(doc, fakeMeasurer{})
	if err != nil {
		t.Fatal(err)
	}
	return pages
}

func texts(ops []Op) []TextOp {
	var out []TextOp
	for _, o := range ops {
		switch o := o.(type) {
		case TextOp:
			out = append(out, o)
		case GroupOp:
			out = append(out, texts(o.Ops)...)
		}
	}
	return out
}

func strs(ops []Op) string {
	var parts []string
	for _, t := range texts(ops) {
		parts = append(parts, t.S)
	}
	return strings.Join(parts, " ")
}

func TestWrapAndAlign(t *testing.T) {
	// Each word is 4 chars * 5pt = 20pt, a space is 5pt; content width is 50pt.
	pages := layoutSrc(t, `<document page-size="70x200" margin="10" size="10" line-height="1">
  <page>
    <text>aaaa   bbbb
      cccc</text>
    <text align="right">dddd</text>
    <text align="center">eeee</text>
  </page>
</document>`)
	ts := texts(pages[0].Ops)
	if len(ts) != 5 {
		t.Fatalf("got %d text ops: %+v", len(ts), ts)
	}
	type pos struct{ x, y float64 }
	got := map[string]pos{}
	for _, o := range ts {
		got[o.S] = pos{o.X, o.Y}
	}
	baseline := 10 + 8.0 // margin + ascent of a 10pt font with line-height 1
	want := map[string]pos{
		"aaaa": {10, baseline},
		"bbbb": {35, baseline}, // single collapsed space
		"cccc": {10, baseline + 10},
		"dddd": {40, baseline + 20},
		"eeee": {25, baseline + 30},
	}
	for k, w := range want {
		if g := got[k]; g != w {
			t.Errorf("%s at %+v, want %+v", k, g, w)
		}
	}
}

func TestJustifyAndBreaks(t *testing.T) {
	pages := layoutSrc(t, `<document page-size="70x200" margin="10" size="10" line-height="1">
  <page>
    <text align="justify">aa bb cc dd<br/>ee ff</text>
    <text>abcdefghijklmnopqrstuvwxyz</text>
  </page>
</document>`)
	got := map[string]TextOp{}
	for _, o := range texts(pages[0].Ops) {
		got[o.S] = o
	}
	// "aa bb cc" = 40pt on a 50pt line: the extra 10pt splits over 2 gaps.
	if x := got["cc"].X; x != 50 {
		t.Errorf("justified cc at x=%v, want 50", x)
	}
	// The line ending in <br/> is not stretched.
	if x := got["dd"].X; x != 10 {
		t.Errorf("dd at x=%v, want 10 (new line)", x)
	}
	if x := got["ff"].X; x != 25 {
		t.Errorf("ff at x=%v, want 25 (last line not justified)", x)
	}
	// 26 chars at 5pt each break into chunks of 10 chars (50pt).
	for _, chunk := range []string{"abcdefghij", "klmnopqrst", "uvwxyz"} {
		if _, ok := got[chunk]; !ok {
			t.Errorf("missing chunk %q in %v", chunk, strs(pages[0].Ops))
		}
	}
}

func TestTablePaginationRepeatsHeader(t *testing.T) {
	var rows strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&rows, "<row><cell>r%d</cell></row>\n", i)
	}
	pages := layoutSrc(t, `<document page-size="200x200" margin="10" size="10" line-height="1">
  <footer gap="0"><text>{page}/{pages}</text></footer>
  <page>
    <table padding="0" border="0">
      <row header="true"><cell>HEAD</cell></row>
      `+rows.String()+`
    </table>
  </page>
</document>`)
	// Content height is 200 - 2*10 - 10 (footer) = 170pt = 17 rows of 10pt.
	if len(pages) != 2 {
		t.Fatalf("got %d pages, want 2", len(pages))
	}
	for i, p := range pages {
		s := strs(p.Ops)
		if !strings.HasPrefix(s, "HEAD ") {
			t.Errorf("page %d does not start with the header row: %q", i+1, s)
		}
		if want := fmt.Sprintf("%d/2", i+1); !strings.HasSuffix(s, want) {
			t.Errorf("page %d footer: %q, want suffix %q", i+1, s, want)
		}
	}
	if s := strs(pages[1].Ops); !strings.Contains(s, "HEAD r17 ") {
		t.Errorf("page 2 should continue at r17: %q", s)
	}
}

func TestPageBreakAbsoluteAndSelectors(t *testing.T) {
	pages := layoutSrc(t, `<document page-size="200x200" margin="10">
  <header pages="2-"><text>HDR</text></header>
  <watermark text="WM" pages="first"/>
  <page>
    <text>one</text>
    <page-break/>
    <page-break/>
    <text x="50%" y="100">abs</text>
    <text>two</text>
  </page>
  <page header="false" watermark="false"><text>three</text></page>
</document>`)
	if len(pages) != 3 {
		t.Fatalf("got %d pages, want 3", len(pages))
	}
	want := []string{"WM one", "HDR abs two", "three"}
	for i, w := range want {
		got := strs(pages[i].Ops)
		// The watermark is drawn in front, so order its text last.
		if i == 0 {
			got = strings.TrimSuffix("WM "+got, " WM")
		}
		if got != w {
			t.Errorf("page %d: %q, want %q", i+1, got, w)
		}
	}
	for _, o := range texts(pages[1].Ops) {
		if o.S == "abs" && o.X != 100 {
			t.Errorf("abs x = %v, want 100 (50%% of page width)", o.X)
		}
	}
}
