package xml2pdf

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
	"testing/fstest"
)

// editorSchema mirrors web/editor/schema.json, which drives autocomplete in
// the browser playground.
type editorSchema struct {
	StyleAttributes map[string][]string `json:"styleAttributes"`
	Elements        map[string]struct {
		Style      bool                `json:"style"`
		Children   []string            `json:"children"`
		Attributes map[string][]string `json:"attributes"`
		Required   string              `json:"required"`
		Content    string              `json:"content"`
		Context    string              `json:"context"`
	} `json:"elements"`
}

// sampleValue returns a valid value for an attribute in the drift test.
func sampleValue(attr string, values []string) string {
	if len(values) > 0 {
		return values[0]
	}
	switch attr {
	case "color", "bg", "border-color", "header-bg", "stripe":
		return "#336699"
	case "line-height", "opacity":
		return "0.9"
	case "rotate":
		return "30"
	case "widths":
		return "1*"
	case "colspan":
		return "1"
	case "pages":
		return "all"
	case "style", "extends":
		return "s"
	case "ref", "image":
		return "img"
	case "src":
		return "logo.png"
	case "text":
		return "T"
	case "items":
		return "items"
	case "title", "author", "subject", "keywords", "id", "name", "value", "var", "index", "number":
		return "v"
	}
	return "10" // lengths: size, margin, width, x, gap, padding, ...
}

// Attributes the renderer recognizes but rejects in combination with the
// element's required sample attributes (e.g. both text= and image= on a
// watermark). The expected error proves the attribute itself is known.
var expectedConflicts = map[string]string{
	"watermark.image": "needs exactly one of text, image or src",
	"watermark.src":   "needs exactly one of text, image or src",
	"img.ref":         "use either ref or src",
}

// TestEditorSchemaMatchesRenderer guarantees that the playground never
// suggests an element or attribute the renderer rejects.
func TestEditorSchemaMatchesRenderer(t *testing.T) {
	raw, err := os.ReadFile("../../web/editor/schema.json")
	if err != nil {
		t.Fatal(err)
	}
	var s editorSchema
	if err := json.Unmarshal(raw, &s); err != nil {
		t.Fatal(err)
	}
	logo, err := os.ReadFile("../../examples/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	opts := Options{
		FS:   fstest.MapFS{"logo.png": {Data: logo}},
		Data: map[string]any{"items": []any{map[string]any{"a": "1"}}},
	}
	// Every document gets a style "s" for style= and extends= samples.
	withStyle := func(doc string) string {
		if strings.Contains(doc, "<resources>") {
			return strings.Replace(doc, "<resources>", `<resources><style id="s"/>`, 1)
		}
		// Insert right after the <document ...> start tag.
		i := strings.Index(doc, ">") + 1
		return doc[:i] + `<resources><style id="s"/></resources>` + doc[i:]
	}
	render := func(doc string) error {
		return Render(strings.NewReader(doc), &bytes.Buffer{}, opts)
	}

	names := make([]string, 0, len(s.Elements))
	for n := range s.Elements {
		names = append(names, n)
	}
	sort.Strings(names)
	checked := 0
	for _, name := range names {
		el := s.Elements[name]
		if !strings.Contains(el.Context, "{}") {
			t.Errorf("%s: context has no {} placeholder", name)
			continue
		}
		for _, c := range el.Children {
			if _, ok := s.Elements[c]; !ok {
				t.Errorf("%s: child %q is not a defined element", name, c)
			}
		}
		attrs := map[string][]string{}
		for a, v := range el.Attributes {
			attrs[a] = v
		}
		if el.Style {
			for a, v := range s.StyleAttributes {
				attrs[a] = v
			}
		}
		build := func(extra string) string {
			open := strings.TrimSpace(fmt.Sprintf("%s %s %s", name, el.Required, extra))
			elem := fmt.Sprintf("<%s>%s</%s>", open, el.Content, name)
			if name == "document" {
				return withStyle(elem)
			}
			return withStyle(strings.Replace(el.Context, "{}", elem, 1))
		}
		// The element alone, with its required attributes, must render.
		if err := render(build("")); err != nil {
			t.Errorf("%s: base document fails: %v\n%s", name, err, build(""))
			continue
		}
		for attr, values := range attrs {
			if strings.Contains(el.Required, attr+"=") {
				continue
			}
			doc := build(fmt.Sprintf(`%s="%s"`, attr, sampleValue(attr, values)))
			err := render(doc)
			checked++
			key := name + "." + attr
			want, conflict := expectedConflicts[key]
			switch {
			case conflict && (err == nil || !strings.Contains(err.Error(), want)):
				t.Errorf("%s: expected conflict error %q, got %v", key, want, err)
			case !conflict && err != nil:
				t.Errorf("%s: renderer rejects a schema attribute: %v\n%s", key, err, doc)
			}
			// Every listed value must be accepted too.
			for _, v := range values[min(1, len(values)):] {
				if err := render(build(fmt.Sprintf(`%s="%s"`, attr, v))); err != nil && !conflict {
					t.Errorf("%s=%q rejected: %v", key, v, err)
				}
			}
		}
	}
	if checked < 150 {
		t.Errorf("only %d attribute checks ran; schema looks incomplete", checked)
	}
	t.Logf("checked %d element/attribute pairs", checked)
}
