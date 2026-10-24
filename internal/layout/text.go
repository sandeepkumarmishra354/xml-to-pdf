package layout

import (
	"strings"
	"unicode"

	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/model"
	"github.com/sandeepkumarmishra354/xml-to-pdf/internal/style"
)

const eps = 1e-6

// ascent is the approximate baseline offset as a fraction of the font size.
const ascent = 0.8

// piece is a run of text with a single computed style, or a forced break.
type piece struct {
	text string
	c    *style.Computed
	br   bool
}

func (e *engine) flatten(xs []model.Inline, c style.Computed, out []piece) ([]piece, error) {
	cp := &c
	for _, x := range xs {
		switch {
		case x.Br:
			out = append(out, piece{br: true, c: cp})
		case x.Props != nil:
			cc, err := e.doc.Styles.Resolve(c, *x.Props)
			if err != nil {
				return nil, err
			}
			if out, err = e.flatten(x.Children, cc, out); err != nil {
				return nil, err
			}
		default:
			s := x.Text
			if e.subst != nil {
				s = e.subst(s)
			}
			out = append(out, piece{text: s, c: cp})
		}
	}
	return out, nil
}

// seg is an encoded, measured run of a word in one style.
type seg struct {
	s string
	c *style.Computed
	w float64
}

// word is an unbreakable sequence of segs (e.g. "<b>bold</b>," is one word).
type word struct {
	segs     []seg
	w        float64
	hasSpace bool    // preceded by whitespace
	space    float64 // width of that whitespace
}

type token struct {
	br   bool
	word word
}

func isSpace(r rune) bool { return r != ' ' && unicode.IsSpace(r) }

func (e *engine) width(c *style.Computed, s string) float64 {
	return e.m.Width(c.Font, c.FontStyle(), c.Size, s)
}

// tokenize collapses whitespace and splits pieces into words.
func (e *engine) tokenize(pieces []piece) []token {
	var toks []token
	var cur word
	var buf strings.Builder
	var bufC *style.Computed
	pending := false
	var spaceC *style.Computed

	flushSeg := func() {
		if buf.Len() == 0 {
			return
		}
		s := e.m.Encode(buf.String())
		w := e.width(bufC, s)
		cur.segs = append(cur.segs, seg{s: s, c: bufC, w: w})
		cur.w += w
		buf.Reset()
	}
	flushWord := func() {
		flushSeg()
		if len(cur.segs) > 0 {
			toks = append(toks, token{word: cur})
			cur = word{}
		}
	}
	for _, p := range pieces {
		if p.br {
			flushWord()
			toks = append(toks, token{br: true})
			pending = false
			continue
		}
		for _, r := range p.text {
			if isSpace(r) {
				flushWord()
				pending = true
				spaceC = p.c
				continue
			}
			if len(cur.segs) == 0 && buf.Len() == 0 && pending {
				cur.hasSpace = true
				cur.space = e.width(spaceC, " ")
				pending = false
			}
			if bufC != p.c {
				flushSeg()
				bufC = p.c
			}
			buf.WriteRune(r)
		}
	}
	flushWord()
	return toks
}

type placed struct {
	word
	gap float64
}

type line struct {
	words []placed
	w     float64
	brk   bool // ended by an explicit <br/>
}

// wrap breaks tokens into lines no wider than maxW.
func (e *engine) wrap(toks []token, maxW float64) []line {
	var lines []line
	var cur line
	for _, t := range toks {
		if t.br {
			cur.brk = true
			lines = append(lines, cur)
			cur = line{}
			continue
		}
		wd := t.word
		gap := 0.0
		if len(cur.words) > 0 && wd.hasSpace {
			gap = wd.space
		}
		if len(cur.words) > 0 && cur.w+gap+wd.w > maxW+eps {
			lines = append(lines, cur)
			cur = line{}
			gap = 0
		}
		if wd.w > maxW+eps {
			chunks := e.splitWord(wd, maxW)
			for _, ch := range chunks[:len(chunks)-1] {
				lines = append(lines, line{words: []placed{{word: ch}}, w: ch.w})
			}
			wd = chunks[len(chunks)-1]
		}
		cur.words = append(cur.words, placed{word: wd, gap: gap})
		cur.w += gap + wd.w
	}
	if len(cur.words) > 0 {
		lines = append(lines, cur)
	}
	return lines
}

// splitWord breaks a word wider than maxW into chunks at character
// boundaries. Encoded text is single-byte, so byte slicing is safe.
func (e *engine) splitWord(wd word, maxW float64) []word {
	var out []word
	var cur word
	for _, s := range wd.segs {
		start := 0
		segW := 0.0
		for i := 0; i < len(s.s); i++ {
			cw := e.width(s.c, s.s[i:i+1])
			if cur.w+segW+cw > maxW+eps && (cur.w+segW > 0) {
				if i > start {
					cur.segs = append(cur.segs, seg{s: s.s[start:i], c: s.c, w: segW})
					cur.w += segW
				}
				out = append(out, cur)
				cur = word{}
				start, segW = i, 0
			}
			segW += cw
		}
		if start < len(s.s) {
			cur.segs = append(cur.segs, seg{s: s.s[start:], c: s.c, w: segW})
			cur.w += segW
		}
	}
	if len(cur.segs) > 0 {
		out = append(out, cur)
	}
	return out
}

// text lays out a paragraph; each line becomes its own fragment so the
// paragraph can break across pages.
func (e *engine) text(t *model.Text, c style.Computed, maxW float64) ([]*frag, error) {
	pieces, err := e.flatten(t.Inlines, c, nil)
	if err != nil {
		return nil, err
	}
	lines := e.wrap(e.tokenize(pieces), maxW)
	var frags []*frag
	for i, ln := range lines {
		lineH, maxSize := 0.0, 0.0
		for _, w := range ln.words {
			for _, s := range w.segs {
				lineH = max(lineH, s.c.LineH())
				maxSize = max(maxSize, s.c.Size)
			}
		}
		if len(ln.words) == 0 {
			lineH, maxSize = c.LineH(), c.Size
		}
		baseline := (lineH-maxSize)/2 + maxSize*ascent

		x, extra := 0.0, 0.0
		switch c.Align {
		case "center":
			x = (maxW - ln.w) / 2
		case "right":
			x = maxW - ln.w
		case "justify":
			gaps := 0
			for _, w := range ln.words[min(1, len(ln.words)):] {
				if w.gap > 0 {
					gaps++
				}
			}
			if i < len(lines)-1 && !ln.brk && gaps > 0 {
				extra = (maxW - ln.w) / float64(gaps)
			}
		}
		var ops []Op
		var prev *seg // last seg drawn, to continue underlines across spaces
		for _, w := range ln.words {
			gap := w.gap
			if gap > 0 {
				gap += extra
			}
			if first := w.segs[0]; gap > 0 && prev != nil && prev.c.Underline && first.c.Underline {
				col := first.c.Color
				ops = append(ops, RectOp{X: x, Y: baseline + 0.1*first.c.Size, W: gap, H: 0.05 * first.c.Size, Fill: &col})
			}
			x += gap
			for i := range w.segs {
				s := &w.segs[i]
				prev = s
				ops = append(ops, TextOp{X: x, Y: baseline, S: s.s, Font: s.c.Font, Style: s.c.FontStyle(), Size: s.c.Size, Color: s.c.Color})
				if s.c.Underline {
					col := s.c.Color
					ops = append(ops, RectOp{X: x, Y: baseline + 0.1*s.c.Size, W: s.w, H: 0.05 * s.c.Size, Fill: &col})
				}
				x += s.w
			}
		}
		frags = append(frags, &frag{h: lineH, ops: ops})
	}
	return frags, nil
}
