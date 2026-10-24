package model

import (
	"math"
	"strings"
	"testing"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
)

func build(t *testing.T, src string) (*Document, error) {
	t.Helper()
	root, err := parser.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	return Build(root, "testdata")
}

func TestResolveColWidths(t *testing.T) {
	cols, err := ParseColWidths("100, 25%, 2*, *")
	if err != nil {
		t.Fatal(err)
	}
	got := ResolveColWidths(cols, 400)
	want := []float64{100, 100, 400.0 / 3, 200.0 / 3}
	for i := range want {
		if math.Abs(got[i]-want[i]) > 1e-9 {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	for _, bad := range []string{"x", "-1", "0*", "a*"} {
		if _, err := ParseColWidths(bad); err == nil {
			t.Errorf("ParseColWidths(%q) expected error", bad)
		}
	}
}

func TestPageSel(t *testing.T) {
	cases := []struct {
		sel   string
		total int
		want  string // selected pages
	}{
		{"", 4, "1234"},
		{"all", 4, "1234"},
		{"first", 4, "1"},
		{"last", 4, "4"},
		{"odd", 4, "13"},
		{"even", 4, "24"},
		{"2-", 4, "234"},
		{"1,3-4", 5, "134"},
	}
	for _, c := range cases {
		ps, err := ParsePageSel(c.sel)
		if err != nil {
			t.Fatal(err)
		}
		got := ""
		for n := 1; n <= c.total; n++ {
			if ps.Match(n, c.total) {
				got += string(rune('0' + n))
			}
		}
		if got != c.want {
			t.Errorf("%q: got %s want %s", c.sel, got, c.want)
		}
	}
	for _, bad := range []string{"0", "x", "3-2", "-2"} {
		if _, err := ParsePageSel(bad); err == nil {
			t.Errorf("ParsePageSel(%q) expected error", bad)
		}
	}
}

func TestBuild(t *testing.T) {
	doc, err := build(t, `<document page-size="A5" orientation="landscape" margin="10 20">
  <resources><style id="h" size="20"/></resources>
  <header height="30"><text>H</text></header>
  <page>
    Loose <b>inline</b> text
    <text style="h">Title</text>
    <table><row><cell>a</cell><cell colspan="2">b</cell></row></table>
    <spacer height="5mm"/>
    <page-break/>
  </page>
  <page page-size="Letter" header="false"><text x="10" y="10">abs</text></page>
</document>`)
	if err != nil {
		t.Fatal(err)
	}
	if doc.Size.W < doc.Size.H {
		t.Errorf("landscape not applied: %+v", doc.Size)
	}
	if doc.Margin != [4]float64{10, 20, 10, 20} {
		t.Errorf("margin %v", doc.Margin)
	}
	p := doc.Pages[0]
	if len(p.Blocks) != 5 {
		t.Fatalf("got %d blocks", len(p.Blocks))
	}
	if _, ok := p.Blocks[0].(*Text); !ok {
		t.Errorf("loose inline content should become a Text block, got %T", p.Blocks[0])
	}
	if tb := p.Blocks[2].(*Table); len(tb.Cols) != 3 {
		t.Errorf("table cols = %d, want 3 from colspan", len(tb.Cols))
	}
	p2 := doc.Pages[1]
	// Letter, inheriting the document's landscape orientation.
	if p2.Size != (Size{792, 612}) || !p2.NoHeader {
		t.Errorf("page 2 overrides not applied: %+v", p2)
	}
	if !p2.Blocks[0].Base().Absolute() {
		t.Error("expected absolute block")
	}
}

func TestBuildErrors(t *testing.T) {
	cases := map[string]string{
		`<doc/>`:      "root element must be <document>",
		`<document/>`: "no <page>",
		`<document><page><text colr="red">x</text></page></document>`:                                   `unknown attribute "colr"`,
		`<document><page><bogus/></page></document>`:                                                    "unknown element <bogus>",
		`<document><page><text style="nope">x</text></page></document>`:                                 `unknown style "nope"`,
		`<document><page><img ref="logo"/></page></document>`:                                           `unknown image resource "logo"`,
		`<document><page><img src="missing.png"/></page></document>`:                                    "image not found",
		`<document><page><text><table/></text></page></document>`:                                       "not allowed inside text",
		`<document><page><table widths="1*"><row><cell/><cell/></row></table></page></document>`:        "row spans 2 columns",
		`<document><page><table><row><cell><text x="1">a</text></cell></row></table></page></document>`: "not allowed inside table cells",
		`<document><watermark/><page/></document>`:                                                      "exactly one of text, image or src",
		`<document><page>
<text size="big">x</text></page></document>`: "line 2",
	}
	for src, want := range cases {
		_, err := build(t, src)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s\n  got error %v, want containing %q", src, err, want)
		}
	}
}
