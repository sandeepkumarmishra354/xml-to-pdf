// Package parser reads an XML document into a generic node tree that keeps
// mixed content (text interleaved with elements) and source line numbers.
package parser

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Attr is a single element attribute.
type Attr struct {
	Name  string
	Value string
}

// Node is either an element (Name != "") or a text node (Name == "").
type Node struct {
	Name     string
	Attrs    []Attr
	Children []*Node
	Text     string // text content for text nodes
	Line     int
}

// IsText reports whether n is a character-data node.
func (n *Node) IsText() bool { return n.Name == "" }

// Attr returns the value of the named attribute.
func (n *Node) Attr(name string) (string, bool) {
	for _, a := range n.Attrs {
		if a.Name == name {
			return a.Value, true
		}
	}
	return "", false
}

// Elements returns the element children of n, skipping text nodes.
func (n *Node) Elements() []*Node {
	var out []*Node
	for _, c := range n.Children {
		if !c.IsText() {
			out = append(out, c)
		}
	}
	return out
}

// InnerText concatenates all descendant text.
func (n *Node) InnerText() string {
	if n.IsText() {
		return n.Text
	}
	var sb strings.Builder
	for _, c := range n.Children {
		sb.WriteString(c.InnerText())
	}
	return sb.String()
}

// Error is a parse or validation error tied to a source line.
type Error struct {
	Line int
	Msg  string
}

func (e *Error) Error() string {
	if e.Line > 0 {
		return fmt.Sprintf("line %d: %s", e.Line, e.Msg)
	}
	return e.Msg
}

// Errorf creates an *Error for the given line.
func Errorf(line int, format string, args ...any) error {
	return &Error{Line: line, Msg: fmt.Sprintf(format, args...)}
}

// Parse reads a complete XML document and returns its root element.
func Parse(r io.Reader) (*Node, error) {
	d := xml.NewDecoder(r)
	d.Strict = true
	var stack []*Node
	var root *Node
	line := 1
	for {
		tok, err := d.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			var se *xml.SyntaxError
			if errors.As(err, &se) {
				return nil, &Error{Line: se.Line, Msg: se.Msg}
			}
			return nil, err
		}
		tokLine := line
		line, _ = d.InputPos()
		switch t := tok.(type) {
		case xml.StartElement:
			n := &Node{Name: t.Name.Local, Line: tokLine}
			for _, a := range t.Attr {
				if a.Name.Space == "xmlns" || a.Name.Local == "xmlns" {
					continue
				}
				n.Attrs = append(n.Attrs, Attr{Name: a.Name.Local, Value: a.Value})
			}
			if len(stack) == 0 {
				if root != nil {
					return nil, Errorf(tokLine, "multiple root elements")
				}
				root = n
			} else {
				p := stack[len(stack)-1]
				p.Children = append(p.Children, n)
			}
			stack = append(stack, n)
		case xml.EndElement:
			stack = stack[:len(stack)-1]
		case xml.CharData:
			if len(stack) == 0 {
				if strings.TrimSpace(string(t)) != "" {
					return nil, Errorf(tokLine, "text outside of root element")
				}
				continue
			}
			p := stack[len(stack)-1]
			// Merge adjacent text (e.g. text split around a comment or CDATA).
			if k := len(p.Children); k > 0 && p.Children[k-1].IsText() {
				p.Children[k-1].Text += string(t)
				continue
			}
			p.Children = append(p.Children, &Node{Text: string(t), Line: tokLine})
		}
	}
	if root == nil {
		return nil, errors.New("empty document")
	}
	return root, nil
}
