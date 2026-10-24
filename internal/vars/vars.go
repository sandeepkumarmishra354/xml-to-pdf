// Package vars resolves ${name} references against command-line overrides,
// external JSON data and constants declared in the document.
//
// Lookup precedence is: overrides (--set) > data (--data JSON) > constants.
// Dotted names (${customer.name}, ${items.0.price}) walk into JSON objects
// and arrays. "$${" produces a literal "${".
package vars

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/parser"
)

var nameRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_.\-]*$`)

// Resolver looks up variable values.
type Resolver struct {
	overrides map[string]string
	data      map[string]any
	consts    map[string]string

	// locals are <for-each> loop scopes, innermost last. They shadow all
	// other sources.
	locals []map[string]any

	resolved  map[string]string
	resolving map[string]bool
}

// New creates a Resolver. Any of the maps may be nil.
func New(overrides map[string]string, data map[string]any, consts map[string]string) *Resolver {
	return &Resolver{
		overrides: overrides,
		data:      data,
		consts:    consts,
		resolved:  map[string]string{},
		resolving: map[string]bool{},
	}
}

// with returns a resolver with an additional local scope.
func (r *Resolver) with(scope map[string]any) *Resolver {
	c := *r
	c.locals = append(append([]map[string]any(nil), r.locals...), scope)
	return &c
}

// global returns the resolver without loop scopes. Constants are always
// resolved globally so their cached values stay valid inside loops.
func (r *Resolver) global() *Resolver {
	if len(r.locals) == 0 {
		return r
	}
	c := *r
	c.locals = nil
	return &c
}

// Lookup returns the value of a variable.
func (r *Resolver) Lookup(name string) (string, error) {
	v, ok, err := r.lookupLocal(name)
	if err != nil || ok {
		return v, err
	}
	if v, ok := r.overrides[name]; ok {
		return v, nil
	}
	if v, ok := walk(r.data, name); ok {
		return format(name, v)
	}
	if raw, ok := r.consts[name]; ok {
		// Constants may reference other variables; resolve lazily.
		if v, ok := r.resolved[name]; ok {
			return v, nil
		}
		if r.resolving[name] {
			return "", fmt.Errorf("circular reference in constant %q", name)
		}
		r.resolving[name] = true
		v, err := r.global().Expand(raw)
		delete(r.resolving, name)
		if err != nil {
			return "", fmt.Errorf("constant %q: %w", name, err)
		}
		r.resolved[name] = v
		return v, nil
	}
	return "", fmt.Errorf("undefined variable %q", name)
}

func (r *Resolver) lookupLocal(name string) (string, bool, error) {
	head, rest, _ := strings.Cut(name, ".")
	for i := len(r.locals) - 1; i >= 0; i-- {
		if v, ok := r.locals[i][head]; ok {
			if rest != "" {
				if v, ok = walk(v, rest); !ok {
					return "", false, fmt.Errorf("undefined variable %q", name)
				}
			}
			s, err := format(name, v)
			return s, true, err
		}
	}
	return "", false, nil
}

// value returns the raw (possibly non-scalar) value of a variable, used for
// the items of a <for-each>. Constants and overrides are always strings.
func (r *Resolver) value(name string) (any, bool) {
	head, rest, _ := strings.Cut(name, ".")
	for i := len(r.locals) - 1; i >= 0; i-- {
		if v, ok := r.locals[i][head]; ok {
			if rest == "" {
				return v, true
			}
			return walk(v, rest)
		}
	}
	if v, ok := r.overrides[name]; ok {
		return v, true
	}
	if v, ok := walk(r.data, name); ok {
		return v, true
	}
	if _, ok := r.consts[name]; ok {
		v, err := r.Lookup(name)
		return v, err == nil
	}
	return nil, false
}

// Expand replaces every ${name} in s.
func (r *Resolver) Expand(s string) (string, error) {
	if !strings.Contains(s, "${") {
		return s, nil
	}
	var sb strings.Builder
	for {
		i := strings.Index(s, "${")
		if i < 0 {
			sb.WriteString(s)
			return sb.String(), nil
		}
		if i > 0 && s[i-1] == '$' {
			// "$${" escape: emit a literal "${".
			sb.WriteString(s[:i-1])
			sb.WriteString("${")
			s = s[i+2:]
			continue
		}
		sb.WriteString(s[:i])
		end := strings.IndexByte(s[i:], '}')
		if end < 0 {
			return "", fmt.Errorf("unterminated ${ in %q", s[i:])
		}
		name := strings.TrimSpace(s[i+2 : i+end])
		if !nameRe.MatchString(name) {
			return "", fmt.Errorf("invalid variable name %q", name)
		}
		v, err := r.Lookup(name)
		if err != nil {
			return "", err
		}
		sb.WriteString(v)
		s = s[i+end+1:]
	}
}

// walk follows a dotted path through maps (string keys) and slices
// (numeric indexes). It accepts decoded JSON as well as typed Go values.
func walk(cur any, path string) (any, bool) {
	if cur == nil {
		return nil, false
	}
	for _, part := range strings.Split(path, ".") {
		switch c := cur.(type) {
		case map[string]any:
			v, ok := c[part]
			if !ok {
				return nil, false
			}
			cur = v
			continue
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(c) {
				return nil, false
			}
			cur = c[idx]
			continue
		}
		rv := reflect.ValueOf(cur)
		for rv.Kind() == reflect.Pointer || rv.Kind() == reflect.Interface {
			if rv.IsNil() {
				return nil, false
			}
			rv = rv.Elem()
		}
		switch rv.Kind() {
		case reflect.Map:
			if rv.Type().Key().Kind() != reflect.String {
				return nil, false
			}
			v := rv.MapIndex(reflect.ValueOf(part).Convert(rv.Type().Key()))
			if !v.IsValid() {
				return nil, false
			}
			cur = v.Interface()
		case reflect.Slice, reflect.Array:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= rv.Len() {
				return nil, false
			}
			cur = rv.Index(idx).Interface()
		default:
			return nil, false
		}
	}
	return cur, true
}

// format converts a scalar value to text.
func format(name string, v any) (string, error) {
	switch v := v.(type) {
	case nil:
		return "", nil
	case string:
		return v, nil
	case json.Number:
		return v.String(), nil
	case bool:
		return strconv.FormatBool(v), nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), nil
	case float32:
		return strconv.FormatFloat(float64(v), 'f', -1, 32), nil
	case fmt.Stringer:
		return v.String(), nil
	}
	switch reflect.ValueOf(v).Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return fmt.Sprint(v), nil
	}
	return "", fmt.Errorf("variable %q is an object or array, not a value", name)
}

// CollectConstants reads the <constants> block(s) directly under root.
// Each constant is <const name="x" value="..."/> or <const name="x">...</const>.
func CollectConstants(root *parser.Node) (map[string]string, error) {
	out := map[string]string{}
	for _, c := range root.Elements() {
		if c.Name != "constants" {
			continue
		}
		for _, k := range c.Children {
			if k.IsText() {
				if strings.TrimSpace(k.Text) != "" {
					return nil, parser.Errorf(k.Line, "unexpected text in <constants>")
				}
				continue
			}
			if k.Name != "const" {
				return nil, parser.Errorf(k.Line, "unexpected <%s> in <constants>, expected <const>", k.Name)
			}
			name, ok := k.Attr("name")
			if !ok || !nameRe.MatchString(name) {
				return nil, parser.Errorf(k.Line, "<const> needs a valid name attribute")
			}
			for _, a := range k.Attrs {
				if a.Name != "name" && a.Name != "value" {
					return nil, parser.Errorf(k.Line, "<const>: unknown attribute %q", a.Name)
				}
			}
			value, hasValue := k.Attr("value")
			if !hasValue {
				value = k.InnerText()
			}
			if _, dup := out[name]; dup {
				return nil, parser.Errorf(k.Line, "constant %q defined twice", name)
			}
			out[name] = value
		}
	}
	return out, nil
}

// ExpandTree expands variables in all attributes and text of the tree and
// unrolls <for-each> loops, skipping <constants> blocks (which are resolved
// lazily on lookup).
func ExpandTree(n *parser.Node, r *Resolver) error {
	if n.Name == forEachTag {
		return parser.Errorf(n.Line, "<%s> cannot be the root element", forEachTag)
	}
	return r.expandNode(n)
}

func (r *Resolver) expandNode(n *parser.Node) error {
	if n.IsText() {
		v, err := r.Expand(n.Text)
		if err != nil {
			return parser.Errorf(n.Line, "%v", err)
		}
		n.Text = v
		return nil
	}
	if n.Name == "constants" {
		return nil
	}
	for i := range n.Attrs {
		v, err := r.Expand(n.Attrs[i].Value)
		if err != nil {
			return parser.Errorf(n.Line, "<%s %s>: %v", n.Name, n.Attrs[i].Name, err)
		}
		n.Attrs[i].Value = v
	}
	kids, err := r.expandChildren(n.Children)
	n.Children = kids
	return err
}

// expandChildren expands a child list, replacing each <for-each> with its
// unrolled content.
func (r *Resolver) expandChildren(children []*parser.Node) ([]*parser.Node, error) {
	out := make([]*parser.Node, 0, len(children))
	for _, c := range children {
		if c.Name == forEachTag {
			nodes, err := r.forEach(c)
			if err != nil {
				return nil, err
			}
			out = append(out, nodes...)
			continue
		}
		if err := r.expandNode(c); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}
