package vars

import (
	"reflect"
	"regexp"
	"strings"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
)

const forEachTag = "for-each"

var localRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_\-]*$`)

// forEach unrolls a loop:
//
//	<for-each items="${items}" var="item" index="i" number="n">...</for-each>
//
// The body is repeated once per element of the items array. Inside it,
// ${item} (or ${item.field}) is the current element, ${i} its 0-based index
// and ${n} its 1-based number. var defaults to "item"; index and number are
// optional. Loops can be nested and may appear anywhere in the document.
func (r *Resolver) forEach(n *parser.Node) ([]*parser.Node, error) {
	var path, varName, indexName, numberName string
	varName = "item"
	for _, a := range n.Attrs {
		v := strings.TrimSpace(a.Value)
		switch a.Name {
		case "items":
			path = v
		case "var":
			varName = v
		case "index":
			indexName = v
		case "number":
			numberName = v
		default:
			return nil, parser.Errorf(n.Line, "<%s>: unknown attribute %q", forEachTag, a.Name)
		}
	}
	// Accept both items="list" and items="${list}".
	if strings.HasPrefix(path, "${") && strings.HasSuffix(path, "}") {
		path = strings.TrimSpace(path[2 : len(path)-1])
	}
	if path == "" {
		return nil, parser.Errorf(n.Line, "<%s>: missing items attribute", forEachTag)
	}
	if !nameRe.MatchString(path) {
		return nil, parser.Errorf(n.Line, "<%s>: invalid items %q", forEachTag, path)
	}
	names := map[string]bool{}
	for _, nm := range []struct{ attr, val string }{{"var", varName}, {"index", indexName}, {"number", numberName}} {
		if nm.val == "" && nm.attr != "var" {
			continue
		}
		if !localRe.MatchString(nm.val) {
			return nil, parser.Errorf(n.Line, "<%s>: invalid %s name %q", forEachTag, nm.attr, nm.val)
		}
		if names[nm.val] {
			return nil, parser.Errorf(n.Line, "<%s>: name %q used twice", forEachTag, nm.val)
		}
		names[nm.val] = true
	}

	val, ok := r.value(path)
	if !ok {
		return nil, parser.Errorf(n.Line, "<%s>: undefined variable %q", forEachTag, path)
	}
	items, ok := toSlice(val)
	if !ok {
		return nil, parser.Errorf(n.Line, "<%s>: %q is not an array", forEachTag, path)
	}

	var out []*parser.Node
	for i, item := range items {
		scope := map[string]any{varName: item}
		if indexName != "" {
			scope[indexName] = i
		}
		if numberName != "" {
			scope[numberName] = i + 1
		}
		body := make([]*parser.Node, len(n.Children))
		for k, c := range n.Children {
			body[k] = clone(c)
		}
		body, err := r.with(scope).expandChildren(body)
		if err != nil {
			return nil, err
		}
		out = append(out, body...)
	}
	return out, nil
}

// toSlice converts JSON arrays and Go slices/arrays to []any.
func toSlice(v any) ([]any, bool) {
	if s, ok := v.([]any); ok {
		return s, true
	}
	if v == nil {
		return nil, false
	}
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Slice && rv.Kind() != reflect.Array {
		return nil, false
	}
	out := make([]any, rv.Len())
	for i := range out {
		out[i] = rv.Index(i).Interface()
	}
	return out, true
}

func clone(n *parser.Node) *parser.Node {
	c := *n
	c.Attrs = append([]parser.Attr(nil), n.Attrs...)
	c.Children = make([]*parser.Node, len(n.Children))
	for i, k := range n.Children {
		c.Children[i] = clone(k)
	}
	return &c
}
