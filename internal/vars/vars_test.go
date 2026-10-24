package vars

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
)

func data(t *testing.T, s string) map[string]any {
	t.Helper()
	d := json.NewDecoder(strings.NewReader(s))
	d.UseNumber()
	var m map[string]any
	if err := d.Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestPrecedence(t *testing.T) {
	r := New(
		map[string]string{"a": "set"},
		data(t, `{"a":"json","b":"json","n":12.50,"cust":{"name":"Bob"},"items":[{"x":1}]}`),
		map[string]string{"a": "const", "b": "const", "c": "const", "greet": "Hi ${cust.name}"},
	)
	cases := map[string]string{
		"${a}":          "set",
		"${b}":          "json",
		"${c}":          "const",
		"${n}":          "12.50",
		"${cust.name}!": "Bob!",
		"${items.0.x}":  "1",
		"${greet}":      "Hi Bob",
		"$${a} ${ a }":  "${a} set",
		"no vars":       "no vars",
	}
	for in, want := range cases {
		got, err := r.Expand(in)
		if err != nil || got != want {
			t.Errorf("Expand(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
}

func TestErrors(t *testing.T) {
	r := New(nil, data(t, `{"obj":{"k":1}}`), map[string]string{"x": "${y}", "y": "${x}"})
	for _, in := range []string{"${missing}", "${x}", "${obj}", "${unterminated", "${bad name}"} {
		if _, err := r.Expand(in); err == nil {
			t.Errorf("Expand(%q) expected error", in)
		}
	}
}

func TestTree(t *testing.T) {
	src := `<document title="${t}">
  <constants>
    <const name="t" value="Hello"/>
    <const name="who">World</const>
  </constants>
  <page><text>${t}, ${who}!</text></page>
</document>`
	root, err := parser.Parse(bytes.NewBufferString(src))
	if err != nil {
		t.Fatal(err)
	}
	consts, err := CollectConstants(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := ExpandTree(root, New(nil, nil, consts)); err != nil {
		t.Fatal(err)
	}
	if v, _ := root.Attr("title"); v != "Hello" {
		t.Errorf("title = %q", v)
	}
	if got := root.Elements()[1].InnerText(); got != "Hello, World!" {
		t.Errorf("text = %q", got)
	}

	bad, _ := parser.Parse(bytes.NewBufferString("<document>\n<page>${nope}</page></document>"))
	err = ExpandTree(bad, New(nil, nil, nil))
	if err == nil || !strings.Contains(err.Error(), "line 2") {
		t.Errorf("expected line 2 error, got %v", err)
	}
}

func expandSrc(t *testing.T, src string, d map[string]any) (*parser.Node, error) {
	t.Helper()
	root, err := parser.Parse(bytes.NewBufferString(src))
	if err != nil {
		t.Fatal(err)
	}
	consts, err := CollectConstants(root)
	if err != nil {
		t.Fatal(err)
	}
	return root, ExpandTree(root, New(map[string]string{"cur": "$"}, d, consts))
}

func TestForEach(t *testing.T) {
	d := data(t, `{
	  "items": [
	    {"name": "A", "qty": 2, "tags": ["x", "y"]},
	    {"name": "B", "qty": 1.5, "tags": []}
	  ],
	  "item": "global-shadowed"
	}`)
	root, err := expandSrc(t, `<document>
  <constants><const name="label" value="Item"/></constants>
  <table>
    <for-each items="${items}" index="i" number="n"><row><cell>${n}/${i} ${label} ${item.name} x${item.qty} ${cur}<for-each items="item.tags" var="tag">[${tag}]</for-each></cell></row></for-each>
  </table>
</document>`, d)
	if err != nil {
		t.Fatal(err)
	}
	table := root.Elements()[1]
	rows := table.Elements()
	if len(rows) != 2 {
		t.Fatalf("got %d rows", len(rows))
	}
	want := []string{"1/0 Item A x2 $[x][y]", "2/1 Item B x1.5 $"}
	for i, r := range rows {
		if got := r.InnerText(); got != want[i] {
			t.Errorf("row %d = %q, want %q", i, got, want[i])
		}
	}
}

func TestForEachGoValues(t *testing.T) {
	d := map[string]any{
		"rows":  []map[string]any{{"name": "a", "n": 3}, {"name": "b", "n": int64(4)}},
		"empty": []string{},
	}
	root, err := expandSrc(t, `<document><for-each items="rows" var="r"><text>${r.name}${r.n}</text></for-each><for-each items="empty"><text>never</text></for-each></document>`, d)
	if err != nil {
		t.Fatal(err)
	}
	if got := root.InnerText(); got != "a3b4" {
		t.Errorf("got %q", got)
	}
}

func TestForEachErrors(t *testing.T) {
	d := data(t, `{"items": [{"a": 1}], "scalar": "x"}`)
	cases := map[string]string{
		`<document><for-each><x/></for-each></document>`:                          "missing items",
		`<document><for-each items="nope"/></document>`:                           `undefined variable "nope"`,
		`<document><for-each items="scalar"/></document>`:                         "not an array",
		`<document><for-each items="items" bogus="1"/></document>`:                `unknown attribute "bogus"`,
		`<document><for-each items="items" var="a.b"/></document>`:                "invalid var name",
		`<document><for-each items="items" var="x" index="x"/></document>`:        "used twice",
		`<document><for-each items="items">${item.missing}</for-each></document>`: `undefined variable "item.missing"`,
		`<document><for-each items="items">${item}</for-each></document>`:         "not a value",
		`<for-each items="items"/>`:                                               "root element",
		"<document>\n\n<for-each items=\"items\">${nope}</for-each></document>":   "line 3",
	}
	for src, want := range cases {
		_, err := expandSrc(t, src, d)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s\n  got %v, want error containing %q", src, err, want)
		}
	}
}
