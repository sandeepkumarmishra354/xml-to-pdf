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

// Lookup returns the value of a variable.
func (r *Resolver) Lookup(name string) (string, error) {
	if v, ok := r.overrides[name]; ok {
		return v, nil
	}
	if v, ok, err := lookupData(r.data, name); err != nil || ok {
		return v, err
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
		v, err := r.Expand(raw)
		delete(r.resolving, name)
		if err != nil {
			return "", fmt.Errorf("constant %q: %w", name, err)
		}
		r.resolved[name] = v
		return v, nil
	}
	return "", fmt.Errorf("undefined variable %q", name)
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

func lookupData(data map[string]any, name string) (string, bool, error) {
	if data == nil {
		return "", false, nil
	}
	var cur any = data
	for _, part := range strings.Split(name, ".") {
		switch c := cur.(type) {
		case map[string]any:
			v, ok := c[part]
			if !ok {
				return "", false, nil
			}
			cur = v
		case []any:
			idx, err := strconv.Atoi(part)
			if err != nil || idx < 0 || idx >= len(c) {
				return "", false, nil
			}
			cur = c[idx]
		default:
			return "", false, nil
		}
	}
	switch v := cur.(type) {
	case nil:
		return "", true, nil
	case string:
		return v, true, nil
	case bool:
		return strconv.FormatBool(v), true, nil
	case json.Number:
		return v.String(), true, nil
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64), true, nil
	case int:
		return strconv.Itoa(v), true, nil
	default:
		return "", false, fmt.Errorf("variable %q is an object or array, not a value", name)
	}
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

// ExpandTree expands variables in all attributes and text of the tree,
// skipping <constants> blocks (which are resolved lazily on lookup).
func ExpandTree(n *parser.Node, r *Resolver) error {
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
	for _, c := range n.Children {
		if err := ExpandTree(c, r); err != nil {
			return err
		}
	}
	return nil
}
