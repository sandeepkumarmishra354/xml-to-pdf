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
