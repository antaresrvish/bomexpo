package kicad

import "fmt"

type node struct {
	atom       string
	kids       []*node
	start, end int // byte span in the source, for in-place edits
}

func (n *node) head() string {
	if n == nil || len(n.kids) == 0 {
		return ""
	}
	return n.kids[0].atom
}

func atom(n *node, i int) string {
	if n == nil || i < 0 || i >= len(n.kids) {
		return ""
	}
	return n.kids[i].atom
}

func child(n *node, head string) *node {
	if n == nil {
		return nil
	}
	for _, k := range n.kids {
		if k.head() == head {
			return k
		}
	}
	return nil
}

type sexpParser struct {
	s string
	i int
}

func parseSexp(s string) (*node, error) {
	p := &sexpParser{s: s}
	p.skipSpace()
	if p.i >= len(p.s) || p.s[p.i] != '(' {
		return nil, fmt.Errorf("expected '('")
	}
	return p.list()
}

func (p *sexpParser) skipSpace() {
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r':
			p.i++
		default:
			return
		}
	}
}

func (p *sexpParser) list() (*node, error) {
	start := p.i
	p.i++ // consume '('
	n := &node{start: start}
	for {
		p.skipSpace()
		if p.i >= len(p.s) {
			return nil, fmt.Errorf("unexpected eof")
		}
		switch c := p.s[p.i]; c {
		case ')':
			p.i++
			n.end = p.i
			return n, nil
		case '(':
			kid, err := p.list()
			if err != nil {
				return nil, err
			}
			n.kids = append(n.kids, kid)
		case '"':
			st := p.i
			a := p.quoted()
			n.kids = append(n.kids, &node{atom: a, start: st, end: p.i})
		default:
			st := p.i
			a := p.token()
			n.kids = append(n.kids, &node{atom: a, start: st, end: p.i})
		}
	}
}

func (p *sexpParser) quoted() string {
	p.i++ // consume opening quote
	start := p.i
	var b []byte
	for p.i < len(p.s) {
		c := p.s[p.i]
		if c == '\\' && p.i+1 < len(p.s) {
			b = append(b, p.s[start:p.i]...)
			b = append(b, p.s[p.i+1])
			p.i += 2
			start = p.i
			continue
		}
		if c == '"' {
			b = append(b, p.s[start:p.i]...)
			p.i++
			return string(b)
		}
		p.i++
	}
	return string(b)
}

func (p *sexpParser) token() string {
	start := p.i
	for p.i < len(p.s) {
		switch p.s[p.i] {
		case ' ', '\t', '\n', '\r', '(', ')':
			return p.s[start:p.i]
		default:
			p.i++
		}
	}
	return p.s[start:p.i]
}
