// Package clientview builds client-emulation views of calendar invites, such
// as an unbranded approximation of how Outlook on the web shows a meeting
// request. Views are emulations, not authoritative renderings.
package clientview

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// RemoteRef is a reference from the invite body to a resource that the view
// did not load. Kind is one of remote, local, cid, inline_data.
type RemoteRef struct {
	Kind   string `json:"kind"`
	Value  string `json:"value"`
	Host   string `json:"host,omitempty"`
	Path   string `json:"path,omitempty"`
	SHA256 string `json:"sha256,omitempty"`
	Mime   string `json:"mime,omitempty"`
}

// Link is an anchor found in the invite body.
type Link struct {
	Href string `json:"href"`
	Host string `json:"host"`
	Text string `json:"text"`
}

// SanitizeResult is the sanitized body fragment plus everything that was
// neutralized on the way.
type SanitizeResult struct {
	HTML       string
	RemoteRefs []RemoteRef
	Links      []Link
	Dropped    []string
}

const (
	maxSanitizeInput    = 2 << 20 // bytes of HTML considered
	maxSanitizeDepth    = 512     // element nesting kept
	maxInlineImageBytes = 2 << 20 // decoded bytes per data: image
	maxRecorded         = 1000    // remote refs / links recorded
	maxLabel            = 120     // characters of a value shown in a placeholder
	maxLinkText         = 200
	maxCSSNesting       = 16   // nested @media/@supports kept
	maxInlineImages     = 1000 // data: image files written per body
	depthHeadroom       = 32   // parser stack used by html/context/implied elements
	outputSlack         = 4096
)

// ---------------------------------------------------------------------------
// Rule table. Every rule here models known Outlook-on-the-web behaviour and is
// UNVERIFIED UNTIL PROBED against real OWA (see testdata/owa-probes/README.md).
// ---------------------------------------------------------------------------

// droppedElements are removed together with their content.
var droppedElements = map[string]bool{
	"script": true, "iframe": true, "frame": true, "frameset": true, "object": true,
	"embed": true, "applet": true, "input": true, "button": true,
	"select": true, "textarea": true, "meta": true, "link": true, "base": true,
	"noscript": true, "title": true, "template": true, "param": true,
	// Raw-text elements: kept verbatim they can swallow the rest of the page.
	"plaintext": true, "xmp": true, "noembed": true, "noframes": true, "listing": true,
	// SVG animation can rewrite attributes (e.g. href) without script.
	"set": true, "animate": true, "animatemotion": true, "animatetransform": true,
	"handler": true, "listener": true,
}

// unwrappedElements lose their tag but keep their content (OWA strips the
// form but the recipient still sees the text inside it).
var unwrappedElements = map[string]bool{"form": true}

// droppedAttrs are removed outright (on* handlers are handled separately).
var droppedAttrs = map[string]bool{
	"formaction": true, "srcdoc": true, "action": true, "ping": true,
	"cite": true, "longdesc": true, "codebase": true, "archive": true, "data": true,
	"dynsrc": true, "lowsrc": true,
}

// resourceAttrs load a resource when rendered.
var resourceAttrs = map[string]bool{"src": true, "background": true, "poster": true}

// linkSchemesKept keep a live href on anchors.
var linkSchemesKept = map[string]bool{"http": true, "https": true, "mailto": true}

// inlineImageTypes are the data: image types kept and extracted.
var inlineImageTypes = map[string]string{
	"image/png": "png", "image/jpeg": "jpg", "image/jpg": "jpg", "image/gif": "gif",
	"image/webp": "webp", "image/bmp": "bmp",
}

// optionalEndTags are auto-closed by the parser, so they do not deepen the tree.
var optionalEndTags = map[string]bool{
	"p": true, "li": true, "dt": true, "dd": true, "option": true, "optgroup": true,
	"tr": true, "td": true, "th": true, "thead": true, "tbody": true, "tfoot": true,
	"rb": true, "rt": true, "rp": true, "rtc": true, "colgroup": true,
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true,
	"track": true, "wbr": true, "param": true, "keygen": true, "image": true,
}

type sanitizer struct {
	inlineDir   string
	refs        []RemoteRef
	generated   map[*html.Node]bool // placeholders/labels we inserted
	refSeen     map[string]bool
	links       []Link
	linkSeen    map[string]bool
	dropped     []string
	droppedSeen map[string]bool
	capped      bool
	inlineSeen  map[string]bool // inline image files written
}

func newSanitizer(inlineDir string) *sanitizer {
	return &sanitizer{
		inlineDir:   inlineDir,
		generated:   map[*html.Node]bool{},
		refSeen:     map[string]bool{},
		linkSeen:    map[string]bool{},
		droppedSeen: map[string]bool{},
		inlineSeen:  map[string]bool{},
	}
}

func (s *sanitizer) drop(what string) {
	if !s.droppedSeen[what] {
		s.droppedSeen[what] = true
		s.dropped = append(s.dropped, what)
	}
}

func (s *sanitizer) addRef(r RemoteRef) {
	key := r.Kind + "\x00" + r.Value + "\x00" + r.SHA256
	if s.refSeen[key] {
		return
	}
	if len(s.refs) >= maxRecorded {
		s.drop("refs_capped")
		return
	}
	s.refSeen[key] = true
	s.refs = append(s.refs, r)
}

func (s *sanitizer) addLink(l Link) {
	key := l.Href + "\x00" + l.Text
	if s.linkSeen[key] {
		return
	}
	if len(s.links) >= maxRecorded {
		s.drop("links_capped")
		return
	}
	s.linkSeen[key] = true
	s.links = append(s.links, l)
}

// OWASanitize sanitizes a raw X-ALT-DESC HTML body the way Outlook on the web
// approximately does, making every remote/local/cid reference a visible
// placeholder. When inlineDir is non-empty, each kept data: image is written
// to inlineDir/<sha256>.<ext>; the name never comes from the content.
func OWASanitize(raw string, inlineDir string) (SanitizeResult, error) {
	s := newSanitizer(inlineDir)

	if len(raw) > maxSanitizeInput {
		raw = truncateUTF8(raw, maxSanitizeInput)
		s.drop("input_truncated")
	}
	// The tree builder refuses more than 512 open elements, and deep input is
	// slow to build, so cut the input before the nesting limit. The first
	// pass ignores optional-end-tag elements (the parser auto-closes them);
	// if the parser still overflows (implied elements such as tbody), retry
	// counting every element, then with smaller budgets.
	var nodes []*html.Node
	var err error
	budget := maxSanitizeDepth - depthHeadroom
	attempts := []struct {
		limit  int
		strict bool
	}{{budget, false}, {budget, true}, {budget / 2, true}, {budget / 4, true}, {budget / 8, true}}
	for _, at := range attempts {
		in := raw
		if cut, ok := depthCut(raw, at.limit, at.strict); ok {
			in = raw[:cut]
			s.drop("input_truncated")
			s.drop("depth_limit")
		}
		ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
		nodes, err = html.ParseFragment(strings.NewReader(in), ctx)
		if err == nil {
			break
		}
	}
	if err != nil {
		return SanitizeResult{}, fmt.Errorf("parse body: %w", err)
	}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		root.AppendChild(n)
	}

	s.walk(root)

	// Placeholders can grow the markup past the cap. Prune whole nodes in
	// document order so the output is a well-formed prefix; never slice text.
	if s.prune(root, maxSanitizeInput-outputSlack) {
		s.drop("output_truncated")
	}
	var buf bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return SanitizeResult{}, fmt.Errorf("render body: %w", err)
		}
	}
	out := buf.String()
	if len(out) > maxSanitizeInput {
		// The size estimate was off; emit nothing rather than a cut tag.
		out = ""
		s.drop("output_truncated")
	}
	return SanitizeResult{HTML: out, RemoteRefs: s.refs, Links: s.links, Dropped: s.dropped}, nil
}

// depthCut tokenizes raw and returns the byte offset of the first tag that
// would nest deeper than max, so the tree builder never sees deep input.
// With strict, elements with optional end tags count too.
func depthCut(raw string, max int, strict bool) (int, bool) {
	z := html.NewTokenizer(strings.NewReader(raw))
	depth, off := 0, 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return 0, false
		}
		n := len(z.Raw())
		switch tt {
		case html.StartTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if !voidElements[tag] && (strict || !optionalEndTags[tag]) {
				depth++
				if depth > max {
					return off, true
				}
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if !voidElements[tag] && (strict || !optionalEndTags[tag]) && depth > 0 {
				depth--
			}
		}
		off += n
	}
}

// prune removes, in document order, every node from the first one that would
// take the rendered size past budget. It reports whether anything was removed.
func (s *sanitizer) prune(root *html.Node, budget int) bool {
	size := 0
	over := false
	stack := []*html.Node{}
	pushKids := func(n *html.Node) {
		var kids []*html.Node
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			kids = append(kids, c)
		}
		for i := len(kids) - 1; i >= 0; i-- {
			stack = append(stack, kids[i])
		}
	}
	pushKids(root)
	for len(stack) > 0 {
		n := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if over {
			n.Parent.RemoveChild(n)
			continue
		}
		size += renderedSize(n)
		if size > budget {
			over = true
			n.Parent.RemoveChild(n)
			continue
		}
		pushKids(n)
	}
	return over
}

// renderedSize is an upper bound on the bytes html.Render emits for n itself
// (its tags and attributes, or its text), excluding its children.
func renderedSize(n *html.Node) int {
	switch n.Type {
	case html.TextNode:
		if n.Parent != nil && n.Parent.Type == html.ElementNode && n.Parent.DataAtom == atom.Style {
			return len(n.Data)
		}
		return escapedLen(n.Data)
	case html.ElementNode:
		size := 2*len(n.Data) + 5
		for _, a := range n.Attr {
			size += len(a.Namespace) + len(a.Key) + escapedLen(a.Val) + 5
		}
		return size
	default:
		return 0
	}
}

func escapedLen(s string) int {
	n := len(s)
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&', '<', '>', '"', '\'', '\r':
			n += 5
		}
	}
	return n
}

type frame struct {
	n     *html.Node
	depth int
}

// walk sanitizes the tree under root iteratively (no recursion on input depth).
func (s *sanitizer) walk(root *html.Node) {
	var stack []frame
	pushChildren := func(n *html.Node, depth int) {
		var kids []*html.Node
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			kids = append(kids, c)
		}
		for i := len(kids) - 1; i >= 0; i-- {
			stack = append(stack, frame{kids[i], depth})
		}
	}
	pushChildren(root, 1)
	for len(stack) > 0 {
		f := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		n := f.n
		if s.generated[n] {
			continue
		}
		switch n.Type {
		case html.CommentNode, html.DoctypeNode:
			n.Parent.RemoveChild(n)
			continue
		case html.TextNode:
			continue
		case html.ElementNode:
		default:
			n.Parent.RemoveChild(n)
			continue
		}
		if f.depth > maxSanitizeDepth {
			n.Parent.RemoveChild(n)
			s.drop("depth_limit")
			continue
		}
		tag := strings.ToLower(n.Data)
		if droppedElements[tag] {
			n.Parent.RemoveChild(n)
			s.drop("element:" + tag)
			continue
		}
		if unwrappedElements[tag] {
			s.drop("element:" + tag)
			var kids []*html.Node
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				kids = append(kids, c)
			}
			for _, c := range kids {
				n.RemoveChild(c)
				n.Parent.InsertBefore(c, n)
			}
			n.Parent.RemoveChild(n)
			for i := len(kids) - 1; i >= 0; i-- {
				stack = append(stack, frame{kids[i], f.depth})
			}
			continue
		}
		if tag == "img" || tag == "image" {
			if ph := s.image(n); ph != nil {
				n.Parent.InsertBefore(ph, n)
				n.Parent.RemoveChild(n)
				continue
			}
		}
		s.attrs(n, tag)
		if tag == "style" {
			s.styleElement(n)
			continue
		}
		pushChildren(n, f.depth+1)
	}
}

// image handles <img>: returns a placeholder node when the image must not load.
func (s *sanitizer) image(n *html.Node) *html.Node {
	src, hasSrc := getAttr(n, "src")
	if !hasSrc {
		src, hasSrc = getAttrNS(n, "href")
	}
	srcset, hasSet := getAttr(n, "srcset")
	var first *RemoteRef
	if hasSet {
		for _, cand := range splitSrcset(srcset) {
			r := s.classify(cand)
			if r.Kind == "inline_data" {
				continue
			}
			s.addRef(r)
			if first == nil {
				rc := r
				first = &rc
			}
		}
		removeAttr(n, "srcset")
		s.drop("attr:srcset")
	}
	if hasSrc && strings.TrimSpace(src) != "" {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(src)), "data:") {
			if _, ok := s.inlineImage(src); ok {
				return nil // keep the self-contained image
			}
			return placeholder(n, "inline image dropped")
		}
		r := s.classify(src)
		if scriptScheme(src) {
			s.drop("attr:src:" + schemeOf(src))
			return placeholder(n, "blocked image")
		}
		s.addRef(r)
		return placeholder(n, refLabel(r))
	}
	if first != nil {
		return placeholder(n, refLabel(*first))
	}
	return nil
}

// attrs sanitizes the attributes of a kept element in place.
func (s *sanitizer) attrs(n *html.Node, tag string) {
	kept := n.Attr[:0]
	var bgPlaceholder *html.Node
	for _, a := range n.Attr {
		key := strings.ToLower(a.Key)
		switch {
		case strings.HasPrefix(key, "on"):
			s.drop("attr:" + key)
			continue
		case droppedAttrs[key]:
			s.drop("attr:" + key)
			continue
		case key == "srcset":
			for _, cand := range splitSrcset(a.Val) {
				if r := s.classify(cand); r.Kind != "inline_data" {
					s.addRef(r)
				}
			}
			s.drop("attr:srcset")
			continue
		case key == "style":
			a.Val = s.declarations(a.Val)
			if strings.TrimSpace(a.Val) == "" {
				continue
			}
		case key == "class":
			a.Val = prefixTokens(a.Val)
		case key == "id":
			a.Val = "x_" + strings.TrimSpace(a.Val)
		case key == "href" && (tag == "image" || tag == "img") && strings.HasPrefix(strings.ToLower(strings.TrimSpace(a.Val)), "data:"):
			// image() already validated it: keep a self-contained SVG image.
			if _, ok := s.inlineImage(a.Val); !ok {
				continue
			}
		case key == "href":
			if !s.href(n, tag, &a) {
				continue
			}
		case resourceAttrs[key]:
			v := strings.TrimSpace(a.Val)
			if scriptScheme(v) {
				s.drop("attr:" + key + ":" + schemeOf(v))
				continue
			}
			if strings.HasPrefix(strings.ToLower(v), "data:") {
				if _, ok := s.inlineImage(v); ok {
					break
				}
				continue
			}
			r := s.classify(v)
			s.addRef(r)
			if key == "background" && bgPlaceholder == nil {
				bgPlaceholder = placeholder(n, refLabel(r))
			}
			continue
		default:
			if scriptScheme(a.Val) {
				s.drop("attr:" + key + ":" + schemeOf(a.Val))
				continue
			}
		}
		kept = append(kept, a)
	}
	n.Attr = kept
	if bgPlaceholder != nil {
		s.generated[bgPlaceholder] = true
		n.InsertBefore(bgPlaceholder, n.FirstChild)
	}
}

// href handles a link target. It returns false when the attribute is removed.
func (s *sanitizer) href(n *html.Node, tag string, a *html.Attribute) bool {
	v := strings.TrimSpace(a.Val)
	scheme := schemeOf(v)
	if scriptScheme(v) {
		s.drop("attr:href:" + scheme)
		return false
	}
	if scheme == "data" {
		s.drop("attr:href:data")
		return false
	}
	if tag != "a" && tag != "area" {
		// href on svg/other elements: treat as a resource reference.
		if v != "" && !strings.HasPrefix(v, "#") {
			s.addRef(s.classify(v))
			return false
		}
		return true
	}
	if v == "" || strings.HasPrefix(v, "#") {
		return true
	}
	host := ""
	if u, err := url.Parse(v); err == nil {
		host = u.Host
	}
	if host == "" && strings.HasPrefix(v, `\\`) {
		host = s.classify(v).Host // UNC path
	}
	s.addLink(Link{Href: v, Host: host, Text: linkText(n)})
	label := host
	if !linkSchemesKept[scheme] || (scheme != "mailto" && host == "") {
		label = clip(v, maxLabel)
	}
	if label != "" {
		span := &html.Node{Type: html.ElementNode, Data: "span", DataAtom: atom.Span,
			Attr: []html.Attribute{{Key: "class", Val: "owa-host"}}}
		span.AppendChild(&html.Node{Type: html.TextNode, Data: "(" + label + ")"})
		if n.Parent != nil {
			s.generated[span] = true
			n.Parent.InsertBefore(span, n.NextSibling)
		}
	}
	if !linkSchemesKept[scheme] {
		s.drop("attr:href:inert")
		return false
	}
	return true
}

// styleElement scopes the CSS of a <style> element to the body container.
func (s *sanitizer) styleElement(n *html.Node) {
	var css strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			css.WriteString(c.Data)
		}
	}
	for n.FirstChild != nil {
		n.RemoveChild(n.FirstChild)
	}
	out := s.stylesheet(css.String())
	out = strings.ReplaceAll(out, "</", `<\/`)
	n.AppendChild(&html.Node{Type: html.TextNode, Data: out})
}

// classify turns a resource reference into a RemoteRef (not recorded).
func (s *sanitizer) classify(raw string) RemoteRef {
	v := strings.TrimSpace(raw)
	switch schemeOf(v) {
	case "data":
		// Self-contained; record only the media type, never the payload.
		mime := strings.ToLower(strings.TrimSpace(v[strings.IndexByte(v, ':')+1:]))
		if i := strings.IndexAny(mime, ";,"); i >= 0 {
			mime = mime[:i]
		}
		return RemoteRef{Kind: "inline_data", Value: "data:" + strings.TrimSpace(mime)}
	case "cid":
		return RemoteRef{Kind: "cid", Value: v}
	case "file":
		r := RemoteRef{Kind: "local", Value: v}
		if u, err := url.Parse(v); err == nil && u.Host != "" && !strings.EqualFold(u.Host, "localhost") {
			r.Host = u.Host // file://host/share is an SMB fetch target
		}
		return r
	case "":
		if strings.HasPrefix(v, "//") {
			if u, err := url.Parse(v); err == nil && u.Host != "" {
				return RemoteRef{Kind: "remote", Value: v, Host: u.Host}
			}
		}
		if strings.HasPrefix(v, `\\`) {
			// UNC path: a remote SMB host.
			host := strings.SplitN(strings.TrimPrefix(v, `\\`), `\`, 2)[0]
			return RemoteRef{Kind: "remote", Value: v, Host: host}
		}
		return RemoteRef{Kind: "local", Value: v}
	default:
		host := ""
		if u, err := url.Parse(v); err == nil {
			host = u.Host
		}
		return RemoteRef{Kind: "remote", Value: v, Host: host}
	}
}

// inlineImage validates a data: image URL, records it and (if configured)
// writes it out. It returns false, recording why, when the image is dropped.
func (s *sanitizer) inlineImage(raw string) (RemoteRef, bool) {
	v := strings.TrimSpace(raw)
	comma := strings.IndexByte(v, ',')
	if comma < 0 || !strings.HasPrefix(strings.ToLower(v), "data:") {
		s.drop("data_url_malformed")
		return RemoteRef{}, false
	}
	meta := strings.ToLower(v[len("data:"):comma])
	payload := v[comma+1:]
	parts := strings.Split(meta, ";")
	mime := strings.TrimSpace(parts[0])
	ext, ok := inlineImageTypes[mime]
	if !ok {
		if strings.HasPrefix(mime, "image/") {
			s.drop("data_image_unsupported")
		} else {
			s.drop("data_url_non_image")
		}
		return RemoteRef{}, false
	}
	isB64 := false
	for _, p := range parts[1:] {
		if strings.TrimSpace(p) == "base64" {
			isB64 = true
		}
	}
	var data []byte
	if isB64 {
		if base64.StdEncoding.DecodedLen(len(payload)) > maxInlineImageBytes+3 {
			s.drop("data_image_too_large")
			return RemoteRef{}, false
		}
		clean := strings.Map(func(r rune) rune {
			if r == ' ' || r == '\n' || r == '\r' || r == '\t' {
				return -1
			}
			return r
		}, payload)
		d, err := base64.StdEncoding.DecodeString(clean)
		if err != nil {
			d, err = base64.RawStdEncoding.DecodeString(strings.TrimRight(clean, "="))
		}
		if err != nil {
			s.drop("data_image_malformed")
			return RemoteRef{}, false
		}
		data = d
	} else {
		d, err := url.PathUnescape(payload)
		if err != nil {
			s.drop("data_image_malformed")
			return RemoteRef{}, false
		}
		data = []byte(d)
	}
	if len(data) > maxInlineImageBytes {
		s.drop("data_image_too_large")
		return RemoteRef{}, false
	}
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	r := RemoteRef{Kind: "inline_data", Value: "data:" + mime, SHA256: sha, Mime: mime}
	if s.inlineDir != "" {
		name := sha + "." + ext
		if !s.inlineSeen[name] && len(s.inlineSeen) >= maxInlineImages {
			s.drop("inline_images_capped")
			return RemoteRef{}, false
		}
		s.inlineSeen[name] = true
		if err := os.MkdirAll(s.inlineDir, 0o755); err == nil {
			if err := os.WriteFile(filepath.Join(s.inlineDir, name), data, 0o644); err == nil {
				r.Path = filepath.ToSlash(filepath.Join(filepath.Base(s.inlineDir), name))
			} else {
				s.drop("inline_write_failed")
			}
		} else {
			s.drop("inline_write_failed")
		}
	}
	s.addRef(r)
	return r, true
}

// ---------------------------------------------------------------------------
// CSS
// ---------------------------------------------------------------------------

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssURL     = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)]*?))\s*\)`)
)

// stylesheet sanitizes a style sheet and scopes every selector to .owa-body.
func (s *sanitizer) stylesheet(css string) string {
	css = cssComment.ReplaceAllString(css, "")
	var out strings.Builder
	s.ruleList(css, &out, 0)
	return out.String()
}

// ruleList sanitizes a rule list nested inside depth group rules. The depth
// bound keeps hostile nesting linear: a whole stylesheet is one text node, so
// the HTML depth cut never sees it, and each level rescans its body.
func (s *sanitizer) ruleList(css string, out *strings.Builder, depth int) {
	i := 0
	for i < len(css) {
		for i < len(css) && isCSSSpace(css[i]) {
			i++
		}
		if i >= len(css) {
			return
		}
		if css[i] == '}' || css[i] == ';' {
			i++
			continue
		}
		open, stmtEnd := scanPrelude(css, i)
		if css[i] == '@' && stmtEnd >= 0 && (open < 0 || stmtEnd < open) {
			name := atName(css[i:stmtEnd])
			s.drop("css:@" + name)
			for _, m := range cssURL.FindAllStringSubmatch(css[i:stmtEnd], -1) {
				s.cssRef(firstNonEmpty(m[1], m[2], m[3]))
			}
			if name == "import" && !strings.Contains(strings.ToLower(css[i:stmtEnd]), "url(") {
				if q := quoted(css[i:stmtEnd]); q != "" {
					s.cssRef(q)
				}
			}
			i = stmtEnd + 1
			continue
		}
		if open < 0 {
			return // trailing garbage without a block
		}
		close := matchBrace(css, open)
		prelude := strings.TrimSpace(css[i:open])
		body := css[open+1 : close]
		if close < len(css) {
			close++
		}
		i = close
		if strings.HasPrefix(prelude, "@") {
			s.atRule(prelude, body, out, depth)
			continue
		}
		sel := s.selectors(prelude)
		if sel == "" {
			continue
		}
		out.WriteString(sel)
		out.WriteByte('{')
		out.WriteString(s.declarations(body))
		out.WriteByte('}')
	}
}

func (s *sanitizer) atRule(prelude, body string, out *strings.Builder, depth int) {
	name := atName(prelude)
	switch name {
	case "media", "supports":
		if strings.ContainsAny(prelude, `\`) || cssURL.MatchString(prelude) {
			s.drop("css:@" + name)
			return
		}
		if depth >= maxCSSNesting {
			s.drop("css:nesting_limit")
			return
		}
		var inner strings.Builder
		s.ruleList(body, &inner, depth+1)
		out.WriteString(prelude)
		out.WriteByte('{')
		out.WriteString(inner.String())
		out.WriteByte('}')
	case "font-face":
		// Fonts are global to the page: a body @font-face could remap the
		// view's own font families. Record remote sources, drop the rule.
		for _, m := range cssURL.FindAllStringSubmatch(body, -1) {
			if u := firstNonEmpty(m[1], m[2], m[3]); !strings.HasPrefix(strings.ToLower(strings.TrimSpace(u)), "data:") {
				s.cssRef(u)
			}
		}
		s.drop("css:@font-face")
	default:
		s.drop("css:@" + name)
	}
}

// selectors scopes a selector list; it returns "" when nothing is usable.
func (s *sanitizer) selectors(prelude string) string {
	if strings.Contains(prelude, `\`) {
		s.drop("css:escaped_selector")
		return ""
	}
	var out []string
	for _, sel := range splitTop(prelude, ',') {
		sel = strings.TrimSpace(sel)
		if sel == "" {
			continue
		}
		scoped := scopeSelector(prefixSelectorNames(sel))
		rest := strings.TrimPrefix(scoped, ".owa-body")
		rest = strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(rest), ".owa-root"))
		if strings.HasPrefix(rest, "~") || strings.HasPrefix(rest, "+") {
			s.drop("css:sibling_of_body")
			continue
		}
		out = append(out, scoped)
	}
	return strings.Join(out, ", ")
}

var rootSelector = regexp.MustCompile(`(?i)^(?:html\s+body|html|body|:root)(?:$|[\s.#\[:>+~])`)

func scopeSelector(sel string) string {
	if loc := rootSelector.FindStringIndex(sel); loc != nil {
		end := loc[1]
		// keep the delimiter that ended the root token
		if end > 0 && end <= len(sel) && !isRootTokenChar(sel[end-1]) {
			end--
		}
		return ".owa-body .owa-root" + sel[end:]
	}
	return ".owa-body " + sel
}

func isRootTokenChar(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == ':'
}

// prefixSelectorNames rewrites .class and #id to .x_class / #x_id, the way
// OWA namespaces body markup, skipping strings and attribute selectors.
func prefixSelectorNames(sel string) string {
	var b strings.Builder
	depth := 0
	var quote byte
	for i := 0; i < len(sel); i++ {
		c := sel[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '[':
			depth++
		case c == ']':
			if depth > 0 {
				depth--
			}
		case depth == 0 && (c == '.' || c == '#') && i+1 < len(sel) && isIdentStart(sel[i+1]):
			b.WriteByte(c)
			b.WriteString("x_")
			continue
		}
		b.WriteByte(c)
	}
	return b.String()
}

func isIdentStart(c byte) bool {
	return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c == '_' || c == '-' || c >= 0x80
}

// declarations sanitizes a declaration block (style attribute or rule body).
func (s *sanitizer) declarations(body string) string {
	var kept []string
	for _, d := range splitTop(cssComment.ReplaceAllString(body, ""), ';') {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		colon := strings.IndexByte(d, ':')
		if colon <= 0 {
			continue
		}
		prop := strings.ToLower(strings.TrimSpace(d[:colon]))
		val := strings.TrimSpace(d[colon+1:])
		lval := strings.ToLower(val)
		switch {
		case strings.Contains(d, `\`):
			s.drop("css:escaped_declaration")
			continue
		case strings.ContainsAny(d, "{}"):
			// A nested style rule would escape the checks below.
			s.drop("css:nested_rule")
			continue
		case prop == "behavior" || prop == "-moz-binding" || strings.Contains(lval, "expression("):
			s.drop("css:script_like")
			continue
		case prop == "position" && strings.Contains(lval, "fixed"):
			s.drop("css:position_fixed")
			continue
		case prop == "position" && (strings.Contains(lval, "var(") || strings.Contains(lval, "env(") || strings.Contains(lval, "attr(")):
			s.drop("css:position_indirect")
			continue
		case strings.Contains(lval, "image-set(") || strings.Contains(lval, "src("):
			s.drop("css:image_set")
			continue
		}
		val = cssURL.ReplaceAllStringFunc(val, func(m string) string {
			sub := cssURL.FindStringSubmatch(m)
			u := strings.TrimSpace(firstNonEmpty(sub[1], sub[2], sub[3]))
			if strings.HasPrefix(strings.ToLower(u), "data:") {
				if _, ok := s.inlineImage(u); ok {
					return m
				}
				return "none"
			}
			s.cssRef(u)
			return "none"
		})
		kept = append(kept, prop+":"+val)
	}
	return strings.Join(kept, ";")
}

func (s *sanitizer) cssRef(u string) {
	u = strings.TrimSpace(u)
	if u == "" {
		return
	}
	if scriptScheme(u) {
		s.drop("css:url:" + schemeOf(u))
		return
	}
	s.addRef(s.classify(u))
}

// scanPrelude finds the next '{' and the next ';' at nesting level 0 from i,
// skipping strings; -1 when absent.
func scanPrelude(css string, i int) (open, semi int) {
	open, semi = -1, -1
	var quote byte
	paren := 0
	for j := i; j < len(css); j++ {
		c := css[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(':
			paren++
		case c == ')':
			if paren > 0 {
				paren--
			}
		case paren == 0 && c == '{':
			return j, semi
		case paren == 0 && c == ';' && semi < 0:
			semi = j
			if css[i] == '@' {
				return -1, semi
			}
		}
	}
	return open, semi
}

// matchBrace returns the index of the '}' closing the '{' at open (or len).
func matchBrace(css string, open int) int {
	depth := 0
	var quote byte
	for j := open; j < len(css); j++ {
		c := css[j]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '{':
			depth++
		case c == '}':
			depth--
			if depth == 0 {
				return j
			}
		}
	}
	return len(css)
}

// splitTop splits on sep outside strings, parentheses and brackets.
func splitTop(s string, sep byte) []string {
	var out []string
	depth := 0
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '"' || c == '\'':
			quote = c
		case c == '(' || c == '[':
			depth++
		case c == ')' || c == ']':
			if depth > 0 {
				depth--
			}
		case c == sep && depth == 0:
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	return append(out, s[start:])
}

func atName(s string) string {
	s = strings.TrimPrefix(strings.TrimSpace(s), "@")
	end := 0
	for end < len(s) && (isIdentStart(s[end]) || s[end] >= '0' && s[end] <= '9') {
		end++
	}
	return strings.ToLower(s[:end])
}

func quoted(s string) string {
	for _, q := range []byte{'"', '\''} {
		if a := strings.IndexByte(s, q); a >= 0 {
			if b := strings.IndexByte(s[a+1:], q); b >= 0 {
				return s[a+1 : a+1+b]
			}
		}
	}
	return ""
}

func isCSSSpace(c byte) bool { return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\f' }

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

// placeholder builds the visible stand-in for a resource that was not loaded,
// sized from the original width/height attributes when they are sane.
func placeholder(orig *html.Node, label string) *html.Node {
	style := "display:inline-block;box-sizing:border-box;border:1px dashed #a19f9d;background:#f3f2f1;color:#605e5c;font:11px/1.3 sans-serif;padding:2px 4px;overflow:hidden;vertical-align:middle"
	if w := dim(orig, "width"); w > 0 {
		style += ";min-width:" + strconv.Itoa(w) + "px"
	}
	if h := dim(orig, "height"); h > 0 {
		style += ";min-height:" + strconv.Itoa(h) + "px"
	}
	span := &html.Node{Type: html.ElementNode, Data: "span", DataAtom: atom.Span,
		Attr: []html.Attribute{{Key: "class", Val: "owa-remote-ref"}, {Key: "style", Val: style}}}
	span.AppendChild(&html.Node{Type: html.TextNode, Data: label})
	return span
}

func dim(n *html.Node, key string) int {
	v, ok := getAttr(n, key)
	if !ok {
		return 0
	}
	i, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(v), "px"))
	if err != nil || i <= 0 || i > 2000 {
		return 0
	}
	return i
}

func refLabel(r RemoteRef) string {
	switch r.Kind {
	case "cid":
		return "inline cid: " + clip(strings.TrimSpace(r.Value[len("cid:"):]), maxLabel)
	case "local":
		return "local reference: " + clip(r.Value, maxLabel)
	default:
		if r.Host != "" {
			return "remote image: " + clip(r.Host, maxLabel)
		}
		return "remote image: " + clip(r.Value, maxLabel)
	}
}

func getAttr(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if a.Namespace == "" && strings.EqualFold(a.Key, key) {
			return a.Val, true
		}
	}
	return "", false
}

func getAttrNS(n *html.Node, key string) (string, bool) {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val, true
		}
	}
	return "", false
}

func removeAttr(n *html.Node, key string) {
	kept := n.Attr[:0]
	for _, a := range n.Attr {
		if !strings.EqualFold(a.Key, key) {
			kept = append(kept, a)
		}
	}
	n.Attr = kept
}

func prefixTokens(v string) string {
	f := strings.Fields(v)
	for i := range f {
		f[i] = "x_" + f[i]
	}
	return strings.Join(f, " ")
}

// splitSrcset returns the candidate URLs of a srcset the way the HTML
// srcset parser reads them: a URL is a run of non-space characters (so a
// data: URL keeps its commas), and its descriptors run to the next comma.
func splitSrcset(v string) []string {
	var out []string
	i := 0
	for i < len(v) {
		for i < len(v) && (isCSSSpace(v[i]) || v[i] == ',') {
			i++
		}
		start := i
		for i < len(v) && !isCSSSpace(v[i]) {
			i++
		}
		u := v[start:i]
		if u == "" {
			break
		}
		if trimmed := strings.TrimRight(u, ","); trimmed != u {
			out = append(out, trimmed)
			continue
		}
		out = append(out, u)
		paren := false
		for i < len(v) {
			c := v[i]
			if c == '(' {
				paren = true
			} else if c == ')' {
				paren = false
			} else if c == ',' && !paren {
				break
			}
			i++
		}
	}
	return out
}

// schemeOf returns the lower-cased URL scheme, ignoring the whitespace and
// control characters browsers strip ("java\tscript:"), or "" when relative.
func schemeOf(v string) string {
	var b strings.Builder
	for _, r := range v {
		if r <= ' ' || r == 0x7f {
			continue
		}
		if r == ':' {
			sch := strings.ToLower(b.String())
			if sch == "" || !isSchemeText(sch) {
				return ""
			}
			if len(sch) == 1 {
				return "file" // drive letter, e.g. C:\path
			}
			return sch
		}
		if r == '/' || r == '?' || r == '#' || r == '\\' {
			return ""
		}
		b.WriteRune(r)
		if b.Len() > 32 {
			return ""
		}
	}
	return ""
}

func isSchemeText(s string) bool {
	for i, c := range s {
		if c >= 'a' && c <= 'z' || i > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.') {
			continue
		}
		return false
	}
	return true
}

func scriptScheme(v string) bool {
	switch schemeOf(v) {
	case "javascript", "vbscript", "livescript", "mocha":
		return true
	}
	return false
}

func linkText(n *html.Node) string {
	var b strings.Builder
	stack := []*html.Node{n}
	for len(stack) > 0 && b.Len() < maxLinkText*4 {
		c := stack[len(stack)-1]
		stack = stack[:len(stack)-1]
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
			continue
		}
		if c.Type == html.ElementNode && (c.DataAtom == atom.Script || c.DataAtom == atom.Style) {
			continue
		}
		var kids []*html.Node
		for k := c.FirstChild; k != nil; k = k.NextSibling {
			kids = append(kids, k)
		}
		for i := len(kids) - 1; i >= 0; i-- {
			stack = append(stack, kids[i])
		}
	}
	return clip(strings.Join(strings.Fields(b.String()), " "), maxLinkText)
}

func clip(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func firstNonEmpty(v ...string) string {
	for _, s := range v {
		if s != "" {
			return s
		}
	}
	return ""
}
