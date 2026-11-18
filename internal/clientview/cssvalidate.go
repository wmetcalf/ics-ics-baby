package clientview

import "strings"

// validScopedCSS re-reads a sanitized style sheet the way a CSS Syntax
// Level 3 parser (Chromium's) does and reports whether every rule it would
// apply is still scoped to the body container. The sanitizer's own scanner
// is simpler than a real tokenizer (quotes in bad url(), comments, simple
// blocks that swallow '}' ...), so this is the backstop: any sheet whose
// structure differs from what the sanitizer meant to emit is rejected whole.
//
// Accepted: a list of style rules whose every comma-separated selector
// starts with ".owa-body" and whose block holds declarations only, and
// @media/@supports rules holding such lists. Anything else (another
// at-rule, a nested block in a declaration list, a bad string or bad url,
// a comment, CDO/CDC, a backslash, an unclosed block) fails.
func validScopedCSS(css string) bool {
	if strings.ContainsAny(css, "\\\x00") || strings.Contains(css, "/*") ||
		strings.Contains(css, "<!--") || strings.Contains(css, "-->") {
		return false
	}
	toks, ok := cssTokens(css)
	if !ok {
		return false
	}
	p := &cssStructure{toks: toks}
	return p.ruleList(false) && p.i == len(p.toks)
}

type cssTok struct {
	kind byte // '{' '}' '(' ')' '[' ']' ';' ',' '@' (at-keyword) 'w' (space) 'x' (other)
	text string
}

// cssTokens is a CSS Syntax tokenizer reduced to what structure depends on.
// Input has no backslashes and no comments (checked by the caller).
func cssTokens(s string) ([]cssTok, bool) {
	var out []cssTok
	isName := func(c byte) bool {
		return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-' || c == '_' || c >= 0x80
	}
	i := 0
	for i < len(s) {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f':
			j := i
			for j < len(s) && strings.IndexByte(" \t\n\r\f", s[j]) >= 0 {
				j++
			}
			out = append(out, cssTok{'w', " "})
			i = j
		case c == '"' || c == '\'':
			j := i + 1
			for j < len(s) && s[j] != c {
				if s[j] == '\n' || s[j] == '\r' || s[j] == '\f' {
					return nil, false // bad-string
				}
				j++
			}
			if j >= len(s) {
				return nil, false // string open at EOF
			}
			out = append(out, cssTok{'x', s[i : j+1]})
			i = j + 1
		case strings.IndexByte("{}()[];,", c) >= 0:
			out = append(out, cssTok{c, string(c)})
			i++
		case c == '@':
			j := i + 1
			for j < len(s) && isName(s[j]) {
				j++
			}
			out = append(out, cssTok{'@', strings.ToLower(s[i+1 : j])})
			i = j
		case isName(c):
			j := i
			for j < len(s) && isName(s[j]) {
				j++
			}
			name := s[i:j]
			if j < len(s) && s[j] == '(' {
				if strings.EqualFold(name, "url") {
					// url( followed by a non-quote is a url token: it must
					// close with ')' and hold no quote, paren or space
					// before it, or it is a bad url that runs to ')'.
					k := j + 1
					for k < len(s) && strings.IndexByte(" \t\n\r\f", s[k]) >= 0 {
						k++
					}
					if k < len(s) && s[k] != '"' && s[k] != '\'' {
						e := k
						for e < len(s) && s[e] != ')' {
							if strings.IndexByte("\"'( \t\n\r\f", s[e]) >= 0 || s[e] < 0x20 || s[e] == 0x7f {
								return nil, false
							}
							e++
						}
						if e >= len(s) {
							return nil, false
						}
						out = append(out, cssTok{'x', s[i : e+1]})
						i = e + 1
						continue
					}
				}
				out = append(out, cssTok{'x', name}, cssTok{'(', "("})
				i = j + 1
				continue
			}
			out = append(out, cssTok{'x', name})
			i = j
		default:
			out = append(out, cssTok{'x', string(c)})
			i++
		}
	}
	return out, true
}

type cssStructure struct {
	toks []cssTok
	i    int
}

func (p *cssStructure) peek() (cssTok, bool) {
	if p.i >= len(p.toks) {
		return cssTok{}, false
	}
	return p.toks[p.i], true
}

// ruleList consumes rules until EOF (top level) or the '}' closing a group
// rule (nested), which it consumes.
func (p *cssStructure) ruleList(nested bool) bool {
	for {
		t, ok := p.peek()
		if !ok {
			return !nested // a group rule left open
		}
		switch t.kind {
		case 'w', ';':
			p.i++
		case '}':
			if !nested {
				return false
			}
			p.i++
			return true
		case '@':
			if t.text != "media" && t.text != "supports" {
				return false
			}
			p.i++
			if !p.prelude(false) {
				return false
			}
			if !p.ruleList(true) {
				return false
			}
		default:
			if !p.prelude(true) || !p.declarationBlock() {
				return false
			}
		}
	}
}

// prelude consumes component values up to and including the '{' that opens
// the rule's block. For a style rule it checks every selector is scoped.
func (p *cssStructure) prelude(styleRule bool) bool {
	var parts []string
	var cur strings.Builder
	for {
		t, ok := p.peek()
		if !ok {
			return false
		}
		switch t.kind {
		case '{':
			p.i++
			if styleRule {
				parts = append(parts, cur.String())
				for _, sel := range parts {
					if !scopedSelector(sel) {
						return false
					}
				}
			}
			return true
		case ';', '}', ')', ']', '@':
			return false
		case '(', '[':
			s, ok := p.simpleBlock()
			if !ok {
				return false
			}
			cur.WriteString(s)
		case ',':
			p.i++
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			p.i++
			cur.WriteString(t.text)
		}
	}
}

// simpleBlock consumes a ( or [ block, which ends only at its own closer:
// a '}' or ';' inside it does not end anything else. Blocks of '{' are not
// expected inside a prelude or a declaration value.
func (p *cssStructure) simpleBlock() (string, bool) {
	open := p.toks[p.i]
	want := byte(')')
	if open.kind == '[' {
		want = ']'
	}
	p.i++
	var b strings.Builder
	b.WriteString(open.text)
	for {
		t, ok := p.peek()
		if !ok {
			return "", false
		}
		switch {
		case t.kind == want:
			p.i++
			b.WriteString(t.text)
			return b.String(), true
		case t.kind == '(' || t.kind == '[':
			s, ok := p.simpleBlock()
			if !ok {
				return "", false
			}
			b.WriteString(s)
		case t.kind == '{' || t.kind == '}' || t.kind == ')' || t.kind == ']' || t.kind == ';' || t.kind == '@':
			return "", false
		default:
			p.i++
			b.WriteString(t.text)
		}
	}
}

// declarationBlock consumes declarations up to the closing '}'. A nested
// '{' (a nested rule) or a stray closer fails.
func (p *cssStructure) declarationBlock() bool {
	for {
		t, ok := p.peek()
		if !ok {
			return false
		}
		switch t.kind {
		case '}':
			p.i++
			return true
		case '(', '[':
			if _, ok := p.simpleBlock(); !ok {
				return false
			}
		case '{', ')', ']', '@':
			return false
		default:
			p.i++
		}
	}
}

// scopedSelector: the selector starts with the body container and does not
// step to its siblings.
func scopedSelector(sel string) bool {
	sel = strings.TrimSpace(sel)
	const body, root = ".owa-body", ".owa-root"
	if !strings.HasPrefix(sel, body) {
		return false
	}
	rest := sel[len(body):]
	if rest != "" && rest[0] != ' ' {
		return false // e.g. ".owa-body-wrap", ".owa-body~x"
	}
	rest = strings.TrimSpace(rest)
	if strings.HasPrefix(rest, root) {
		after := rest[len(root):]
		if after != "" && !strings.ContainsRune(" .#[:>", rune(after[0])) {
			return false
		}
		rest = strings.TrimSpace(after)
	}
	return !strings.HasPrefix(rest, "~") && !strings.HasPrefix(rest, "+") && !strings.HasPrefix(rest, ",")
}
