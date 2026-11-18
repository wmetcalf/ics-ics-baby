// Package clientview builds client-emulation views of calendar invites, such
// as an unbranded approximation of how Outlook on the web shows a meeting
// request. Views are emulations, not authoritative renderings.
package clientview

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"image"
	_ "image/gif" // DecodeConfig for inline images
	_ "image/jpeg"
	_ "image/png"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	_ "golang.org/x/image/bmp"
	_ "golang.org/x/image/webp"
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
	maxSanitizeInput     = 2 << 20 // bytes of HTML considered
	maxSanitizeDepth     = 512     // element nesting kept
	maxInlineImageBytes  = 2 << 20 // decoded bytes per data: image
	maxRecorded          = 1000    // remote refs / links recorded
	maxLabel             = 120     // characters of a value shown in a placeholder
	maxLinkText          = 200
	maxCSSNesting        = 16       // nested @media/@supports kept
	maxRebuiltNodes      = 1000000  // formatting elements the parser may re-create
	maxInlinePixels      = 16 << 20 // decoded pixels per inline image (e.g. 4096x4096)
	maxInlinePixelsTotal = 64 << 20 // decoded pixels of all inline images of a body
	maxMergedText        = 32 << 20 // bytes copied merging adjacent text in the parser
	maxStackScan         = 16 << 20 // open-stack entries the parser's scope checks walk
	maxRebuiltAttrs      = 4 << 20  // attributes copied into re-created formatting elements
	maxRebuiltBytes      = 64 << 20 // bytes of those attributes
	maxInlineImages      = 1000     // data: image files written per body
	depthHeadroom        = 32       // parser stack used by html/context/implied elements
	outputSlack          = 4096
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
	// <use> chains fan out exponentially in the renderer.
	"use": true,
	// Document-level elements only occur as parser artifacts (e.g. inside
	// svg/math, where x/net/html and Chromium disagree on insertion modes);
	// Chromium merges a stray <body>/<html>'s attributes onto the page.
	"html": true, "head": true, "body": true,
}

// renderVoid are the names html.Render refuses to write with children.
var renderVoid = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true, "img": true,
	"input": true, "keygen": true, "link": true, "meta": true, "param": true, "source": true,
	"track": true, "wbr": true,
}

// foreignDropped are names that must not appear on svg/math elements:
// x/net/html resets its insertion mode on them without checking the
// namespace, so its tree (the stability oracle) would diverge from
// Chromium's.
var foreignDropped = map[string]bool{
	"td": true, "th": true, "tr": true, "tbody": true, "thead": true, "tfoot": true,
	"caption": true, "colgroup": true, "table": true, "select": true,
}

// unwrappedElements lose their tag but keep their content (OWA strips the
// form but the recipient still sees the text inside it).
var unwrappedElements = map[string]bool{}

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

// formattingElements are the HTML "active formatting elements".
var formattingElements = map[string]bool{
	"a": true, "b": true, "big": true, "code": true, "em": true, "font": true, "i": true,
	"nobr": true, "s": true, "small": true, "strike": true, "strong": true, "tt": true, "u": true,
}

// tableParts cannot hold a <span> child.
var tableParts = map[string]bool{
	"table": true, "tbody": true, "thead": true, "tfoot": true, "tr": true, "colgroup": true, "col": true,
}

// formattingMarkers push a scope marker onto the active formatting list.
var formattingMarkers = map[string]bool{
	"applet": true, "object": true, "marquee": true, "template": true,
	"td": true, "th": true, "caption": true,
}

// tableStructure elements hold no text: text there is foster-parented.
var tableStructure = map[string]bool{
	"table": true, "tbody": true, "thead": true, "tfoot": true, "tr": true, "col": true, "colgroup": true,
}

// tableEnd end tags close through cells and rows up to their table.
var tableEnd = map[string]bool{
	"table": true, "tbody": true, "thead": true, "tfoot": true, "tr": true, "td": true, "th": true, "caption": true,
}

// tableScope and buttonScope stop the search for an element to close.
var tableScope = map[string]bool{"table": true, "html": true, "template": true}
var buttonScope = map[string]bool{
	"applet": true, "caption": true, "html": true, "table": true, "td": true, "th": true,
	"marquee": true, "object": true, "template": true, "button": true,
	"svg": true, "math": true, "foreignobject": true, "desc": true, "title": true,
	"mi": true, "mo": true, "mn": true, "ms": true, "mtext": true, "annotation-xml": true,
}

// closesP start tags close an open <p> (in button scope).
var closesP = map[string]bool{
	"address": true, "article": true, "aside": true, "blockquote": true, "center": true,
	"details": true, "dialog": true, "dir": true, "div": true, "dl": true, "fieldset": true,
	"figcaption": true, "figure": true, "footer": true, "header": true, "hgroup": true,
	"main": true, "menu": true, "nav": true, "ol": true, "p": true, "search": true,
	"section": true, "summary": true, "ul": true, "h1": true, "h2": true, "h3": true,
	"h4": true, "h5": true, "h6": true, "pre": true, "listing": true, "form": true,
	"hr": true, "xmp": true, "plaintext": true, "li": true, "dd": true, "dt": true,
}

// specialElements are the parser's "special" category.
var specialElements = map[string]bool{
	"address": true, "applet": true, "area": true, "article": true, "aside": true, "base": true,
	"basefont": true, "bgsound": true, "blockquote": true, "body": true, "br": true, "button": true,
	"caption": true, "center": true, "col": true, "colgroup": true, "dd": true, "details": true,
	"dir": true, "div": true, "dl": true, "dt": true, "embed": true, "fieldset": true,
	"figcaption": true, "figure": true, "footer": true, "form": true, "frame": true,
	"frameset": true, "h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"head": true, "header": true, "hgroup": true, "hr": true, "html": true, "iframe": true,
	"img": true, "input": true, "keygen": true, "li": true, "link": true, "listing": true,
	"main": true, "marquee": true, "menu": true, "meta": true, "nav": true, "noembed": true,
	"noframes": true, "noscript": true, "object": true, "ol": true, "p": true, "param": true,
	"plaintext": true, "pre": true, "script": true, "search": true, "section": true,
	"select": true, "source": true, "style": true, "summary": true, "table": true,
	"tbody": true, "td": true, "template": true, "textarea": true, "tfoot": true, "th": true,
	"thead": true, "title": true, "tr": true, "track": true, "ul": true, "wbr": true, "xmp": true,
	"mi": true, "mo": true, "mn": true, "ms": true, "mtext": true, "annotation-xml": true,
	"foreignobject": true, "desc": true,
}

// ignoredInBody start tags are dropped by the parser outside a table (or
// only merge attributes), so text on both sides joins one text node.
var ignoredInBody = map[string]bool{
	"tr": true, "td": true, "th": true, "tbody": true, "thead": true, "tfoot": true,
	"col": true, "colgroup": true, "caption": true, "html": true, "head": true,
	"body": true, "frameset": true, "frame": true,
}

// htmlBreakout start tags make the parser leave <svg>/<math> content.
var htmlBreakout = map[string]bool{
	"b": true, "big": true, "blockquote": true, "body": true, "br": true, "center": true,
	"code": true, "dd": true, "div": true, "dl": true, "dt": true, "em": true, "embed": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true, "head": true,
	"hr": true, "i": true, "img": true, "li": true, "listing": true, "menu": true, "meta": true,
	"nobr": true, "ol": true, "p": true, "pre": true, "ruby": true, "s": true, "small": true,
	"span": true, "strong": true, "strike": true, "sub": true, "sup": true, "table": true,
	"tt": true, "u": true, "ul": true, "var": true, "font": true,
}

var voidElements = map[string]bool{
	"area": true, "base": true, "br": true, "col": true, "embed": true, "hr": true,
	"img": true, "input": true, "link": true, "meta": true, "source": true,
	"track": true, "wbr": true, "param": true, "keygen": true, "image": true,
}

type sanitizer struct {
	inlineDir     string
	refs          []RemoteRef
	generated     map[*html.Node]bool // placeholders/labels we inserted
	refSeen       map[string]bool
	links         []Link
	linkSeen      map[string]bool
	dropped       []string
	droppedSeen   map[string]bool
	capped        bool
	inlineSeen    map[string]bool // inline image files written
	inlineFiles   int             // files in inlineDir, this body's included
	inlinePixels  int64           // decoded pixels of the kept inline images
	pixelSeen     map[string]bool // sha256 of images already charged
	lastCanonical string          // canonical data: URL of the last kept inline image
	styles        []*html.Node    // sanitized <style> elements, in order
}

func newSanitizer(inlineDir string) *sanitizer {
	s := &sanitizer{
		inlineDir:   inlineDir,
		generated:   map[*html.Node]bool{},
		refSeen:     map[string]bool{},
		linkSeen:    map[string]bool{},
		droppedSeen: map[string]bool{},
		inlineSeen:  map[string]bool{},
		pixelSeen:   map[string]bool{},
	}
	if inlineDir != "" {
		// The cap is per directory: every body of one view shares it.
		if ents, err := os.ReadDir(inlineDir); err == nil {
			s.inlineFiles = len(ents)
		}
	}
	return s
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
	tail := "" // input past a depthCut cut: not rendered, but evidence
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
			tail = raw[cut:]
		} else {
			tail = ""
		}
		ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
		nodes, err = parseFragment(strings.NewReader(in), ctx)
		if err == nil {
			break
		}
	}
	if err != nil {
		// Never fail the whole view over one body: record it and show a
		// placeholder instead.
		s.drop("parse_failed")
		s.tokenEvidence(raw)
		return SanitizeResult{HTML: unrenderedBody, RemoteRefs: s.refs, Links: s.links, Dropped: s.dropped}, nil
	}
	root := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	for _, n := range nodes {
		root.AppendChild(n)
	}

	s.walk(root)
	s.hoistStyles(root)
	if tail != "" {
		// Links and references after the cut still reach the sidecar.
		s.tokenEvidence(tail)
	}

	// Placeholders can grow the markup past the cap. Prune whole nodes in
	// document order so the output is a well-formed prefix; never slice text.
	if s.prune(root, maxSanitizeInput-outputSlack) {
		s.drop("output_truncated")
	}
	var buf bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			// Never fail the whole view over one body.
			s.drop("render_failed")
			s.tokenEvidence(raw)
			return SanitizeResult{HTML: unrenderedBody, RemoteRefs: s.refs, Links: s.links, Dropped: s.dropped}, nil
		}
	}
	out := buf.String()
	if len(out) > maxSanitizeInput {
		// The size estimate was off; emit nothing rather than a cut tag.
		out = ""
		s.drop("output_truncated")
	}
	if out != "" && !stableInView(out) {
		// The browser would build a different tree from this markup than
		// the one sanitized (a breakout from <svg>/<math>, an end tag that
		// reaches the view's own wrappers, ...). Show none of it.
		s.drop("unstable_markup")
		out = unrenderedBody
	}
	return SanitizeResult{HTML: out, RemoteRefs: s.refs, Links: s.links, Dropped: s.dropped}, nil
}

// parseFragment is html.ParseFragment; a variable so tests can make it fail.
var parseFragment = html.ParseFragment

// tokenEvidence records links and resource references from the raw tokens
// of a body the parser could not build, so its evidence still reaches the
// sidecar.
func (s *sanitizer) tokenEvidence(raw string) {
	z := html.NewTokenizer(strings.NewReader(raw))
	cssRefs := func(css string) {
		for _, m := range cssURL.FindAllStringSubmatch(css, -1) {
			if u := strings.TrimSpace(firstNonEmpty(m[1], m[2], m[3])); u != "" && !strings.HasPrefix(u, "#") &&
				!strings.HasPrefix(strings.ToLower(u), "data:") && !scriptScheme(u) {
				s.addRef(s.classify(u))
			}
		}
	}
	inStyle := false
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return
		}
		if tt == html.TextToken && inStyle {
			cssRefs(string(z.Text()))
		}
		if tt != html.StartTagToken && tt != html.SelfClosingTagToken {
			inStyle = false
			continue
		}
		name, hasAttr := z.TagName()
		tag := string(name)
		inStyle = tag == "style" && tt == html.StartTagToken
		for hasAttr {
			var k, v []byte
			k, v, hasAttr = z.TagAttr()
			key, val := strings.ToLower(string(k)), strings.TrimSpace(string(v))
			if val == "" || scriptScheme(val) {
				continue
			}
			switch {
			case key == "style":
				cssRefs(val)
			case key == "href" && (tag == "a" || tag == "area"):
				host := ""
				if u, err := url.Parse(val); err == nil {
					host = u.Host
				}
				s.addLink(Link{Href: val, Host: host})
			case key == "href" || key == "xlink:href" || resourceAttrs[key]:
				if strings.HasPrefix(strings.ToLower(val), "data:") {
					continue
				}
				s.addRef(s.classify(val))
			case key == "srcset":
				for _, c := range splitSrcset(val) {
					if r := s.classify(c); r.Kind != "inline_data" {
						s.addRef(r)
					}
				}
			}
		}
	}
}

// unrenderedBody replaces a body that cannot be shown safely.
const unrenderedBody = `<p class="owa-remote-ref">The message body could not be rendered safely; see the sidecar.</p>`

// stableInView re-parses the sanitized body inside a copy of the view's
// wrappers, as Chromium will, and reports whether it comes back as the same
// markup in the same place: nothing merged onto <html>/<body>, no wrapper
// closed early, nothing moved out of the body container.
func stableInView(body string) bool {
	const pre = `<!doctype html><html><head></head><body><div class="owa-pane"><div class="owa-body-wrap"><div class="owa-body"><div class="owa-root">`
	const post = `</div></div></div><div class="owa-after"></div></div></body></html>`
	doc, err := html.Parse(strings.NewReader(pre + body + post))
	if err != nil {
		return false
	}
	// only returns the single element child of n when n has exactly that
	// element (plus whitespace) and the expected tag and class.
	only := func(n *html.Node, tag, class string) *html.Node {
		var found *html.Node
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			switch c.Type {
			case html.ElementNode:
				if found != nil {
					return nil
				}
				found = c
			case html.TextNode:
				if strings.TrimSpace(c.Data) != "" {
					return nil
				}
			default:
				return nil
			}
		}
		if found == nil || found.Data != tag || found.Namespace != "" || !hasOnlyClass(found, class) {
			return nil
		}
		return found
	}
	htmlEl := only(doc, "html", "")
	if htmlEl == nil {
		// doctype is a child of doc too
		for c := doc.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && c.Data == "html" && hasOnlyClass(c, "") {
				htmlEl = c
			}
		}
	}
	if htmlEl == nil {
		return false
	}
	head, bodyEl := htmlEl.FirstChild, htmlEl.LastChild
	if head == nil || head.Data != "head" || head.FirstChild != nil || len(head.Attr) != 0 ||
		bodyEl == nil || bodyEl.Data != "body" || head.NextSibling != bodyEl || len(bodyEl.Attr) != 0 {
		return false
	}
	pane := only(bodyEl, "div", "owa-pane")
	if pane == nil {
		return false
	}
	wrap := pane.FirstChild
	after := pane.LastChild
	if wrap == nil || after == nil || wrap.NextSibling != after ||
		wrap.Data != "div" || !hasOnlyClass(wrap, "owa-body-wrap") ||
		after.Data != "div" || !hasOnlyClass(after, "owa-after") || after.FirstChild != nil {
		return false
	}
	bdy := only(wrap, "div", "owa-body")
	if bdy == nil {
		return false
	}
	root := only(bdy, "div", "owa-root")
	if root == nil {
		return false
	}
	var buf bytes.Buffer
	for c := root.FirstChild; c != nil; c = c.NextSibling {
		if err := html.Render(&buf, c); err != nil {
			return false
		}
	}
	return buf.String() == body
}

func hasOnlyClass(n *html.Node, class string) bool {
	if class == "" {
		return len(n.Attr) == 0
	}
	return len(n.Attr) == 1 && n.Attr[0].Key == "class" && n.Attr[0].Val == class
}

// depthCut tokenizes raw and returns the byte offset of the first tag that
// would nest deeper than max, so the tree builder never sees deep input.
// With strict, elements with optional end tags count too.
func depthCut(raw string, max int, strict bool) (int, bool) {
	z := html.NewTokenizer(strings.NewReader(raw))
	// A model of the tree builder that bounds what x/net/html will build.
	// Where it cannot follow the parser exactly it errs toward the larger
	// tree, so the bounds hold and only truncation comes sooner:
	//  - stack: open elements. An end tag closes only an open element (a
	//    stray </span> does not lower it); "<div/>" opens a div in HTML
	//    content but is closed inside <svg>/<math>.
	//  - active: the active formatting elements (tag plus attributes). One
	//    stays active until its own end tag, even after an enclosing end tag
	//    popped it off the stack, and the parser re-creates it for later
	//    text: that adds depth and nodes. The Noah's-Ark clause keeps at most
	//    three identical entries, as the parser does. "|"-prefixed entries
	//    are scope markers (cells, captions, marquee, object, applet,
	//    template) that end tags do not reach past. A marker is removed by
	//    its own end tag only and never clears the entries after it: a <td>
	//    outside a table is ignored by the parser, which then keeps them.
	var stack, active []string
	onStack, nFmt, tables, foreignOpen := 0, 0, 0, 0
	// Each re-created element copies its attributes (and walk re-sanitizes
	// them): charge attribute counts and bytes of the active entries too.
	activeAttrs, activeBytes := 0, 0
	depth := func() int {
		extra := nFmt - onStack
		if extra < 0 {
			extra = 0
		}
		return len(stack) + extra
	}
	foreign := func() bool { // the insertion point is in svg/math content
		for i := len(stack) - 1; i >= 0; i-- {
			switch stack[i] {
			case "svg", "math":
				return true
			case "foreignobject", "desc", "title", "mi", "mo", "mn", "ms", "mtext", "annotation-xml":
				return false
			}
		}
		return false
	}
	push := func(tag string) {
		stack = append(stack, tag)
		switch {
		case formattingElements[tag]:
			onStack++
		case tag == "table":
			tables++
		case tag == "svg" || tag == "math":
			foreignOpen++
		}
	}
	clearToMarker := func() { // the parser does this when a cell/marquee/... closes
		for j := len(active) - 1; j >= 0; j-- {
			if active[j][0] == '|' {
				for _, a := range active[j+1:] {
					nFmt--
					activeAttrs -= strings.Count(a, "\x00")
					activeBytes -= len(a)
				}
				active = active[:j]
				return
			}
		}
	}
	pop := func(i int) { // close stack[i:]
		for k := len(stack) - 1; k >= i; k-- {
			if formattingMarkers[stack[k]] {
				clearToMarker()
				for j := len(active) - 1; j >= 0; j-- { // and the marker
					if active[j] == "|"+stack[k] {
						active = append(active[:j], active[j+1:]...)
						break
					}
				}
			}
		}
		for _, t := range stack[i:] {
			switch {
			case formattingElements[t]:
				onStack--
			case t == "table":
				tables--
			case t == "svg" || t == "math":
				foreignOpen--
			}
		}
		stack = stack[:i]
	}
	off := 0
	rebuilt, scanned := 0, 0
	rebuiltAttrs, rebuiltBytes := 0, 0
	keyAttrs := func(key string) int { return strings.Count(key, "\x00") }
	tableCtx := func() bool { // text here is foster-parented before the table
		return tables > 0 && len(stack) > 0 && tableStructure[stack[len(stack)-1]]
	}
	// closeTo pops to (and including) the nearest open element in names,
	// unless an element in stops (or a special one, when special) comes
	// first: the parser's implied end tags for li, dd/dt, p, cells, rows.
	closeTo := func(names map[string]bool, stops map[string]bool, special bool) {
		for i := len(stack) - 1; i >= 0; i-- {
			t := stack[i]
			if names[t] {
				pop(i)
				return
			}
			if stops[t] || (special && specialElements[t] && t != "address" && t != "div" && t != "p") {
				return
			}
		}
	}
	// The parser appends each text token to an adjacent text node
	// (n.Data += text) when only ignored tags separate them: "a</x>" or
	// "a<tr>" (outside a table) x 400k copies quadratically. Track the
	// growing node and the bytes copied.
	merging, mergeLen, merged := false, 0, 0
	for {
		tt := z.Next()
		if tt == html.ErrorToken {
			return 0, false
		}
		n := len(z.Raw())
		if tt == html.TextToken {
			if merging {
				merged += mergeLen
				mergeLen += n
			} else {
				merging, mergeLen = true, n
			}
			if merged > maxMergedText {
				return off, true
			}
		}
		if tt == html.StartTagToken || tt == html.SelfClosingTagToken || tt == html.TextToken || tt == html.EndTagToken {
			rebuilt += nFmt
			rebuiltAttrs += activeAttrs
			rebuiltBytes += activeBytes
			if tt != html.TextToken {
				// The parser scans the open stack for scope on most tags
				// (<hr>, </p>, </li>, ...) and the active list on others:
				// charge both.
				scanned += len(stack) + len(active)
			}
			if rebuilt > maxRebuiltNodes || rebuiltAttrs > maxRebuiltAttrs || rebuiltBytes > maxRebuiltBytes ||
				scanned > maxStackScan || depth() > max {
				return off, true
			}
		}
		switch tt {
		case html.CommentToken:
			if !tableCtx() {
				merging = false
			}
		case html.StartTagToken, html.SelfClosingTagToken:
			name, hasAttr := z.TagName()
			tag := string(name)
			var attrs strings.Builder
			fontBreakout := false
			for hasAttr {
				var k, v []byte
				k, v, hasAttr = z.TagAttr()
				attrs.Write(k)
				attrs.WriteByte('=')
				attrs.Write(v)
				attrs.WriteByte(0)
				switch string(k) {
				case "color", "face", "size":
					fontBreakout = true
				}
			}
			if foreignOpen > 0 {
				// Inside svg/math, <style>, <title>, ... hold markup for the
				// parser (in an HTML integration point they would not; read
				// as markup there too, which only over-counts).
				z.NextIsNotRawText()
			}
			if tableCtx() {
				// Text before and after a row/section start tag is foster-
				// parented into one text node before the table.
				if !tableStructure[tag] {
					merging = false
				}
			} else if !ignoredInBody[tag] || tables > 0 {
				merging = false
			}
			inForeignContent := foreign()
			if inForeignContent && htmlBreakout[tag] && (tag != "font" || fontBreakout) {
				// The parser leaves foreign content here: close it.
				for i := len(stack) - 1; i >= 0; i-- {
					if stack[i] == "svg" || stack[i] == "math" {
						pop(i)
						break
					}
				}
				inForeignContent = foreign()
			}
			if tt == html.SelfClosingTagToken && (inForeignContent || tag == "svg" || tag == "math") {
				break // closed at once
			}
			if ignoredInBody[tag] && tables == 0 {
				break // outside a table the parser ignores the tag
			}
			if formattingMarkers[tag] {
				active = append(active, "|"+tag)
			}
			if voidElements[tag] {
				break
			}
			// Implied end tags, so elements left open inside a closed item,
			// paragraph or cell do not stay on the modeled stack.
			switch {
			case tag == "li":
				closeTo(map[string]bool{"li": true}, nil, true)
			case tag == "dd" || tag == "dt":
				closeTo(map[string]bool{"dd": true, "dt": true}, nil, true)
			case tag == "td" || tag == "th":
				closeTo(map[string]bool{"caption": true}, tableScope, false)
				closeTo(map[string]bool{"td": true, "th": true}, tableScope, false)
			case tag == "tr":
				closeTo(map[string]bool{"caption": true, "td": true, "th": true}, tableScope, false)
				closeTo(map[string]bool{"tr": true}, tableScope, false)
			case tag == "tbody" || tag == "thead" || tag == "tfoot" || tag == "colgroup" || tag == "caption":
				closeTo(map[string]bool{"caption": true, "td": true, "th": true}, tableScope, false)
				closeTo(map[string]bool{"tr": true}, tableScope, false)
				closeTo(map[string]bool{"tbody": true, "thead": true, "tfoot": true}, tableScope, false)
			case tag == "option":
				closeTo(map[string]bool{"option": true}, tableScope, false)
			}
			if closesP[tag] {
				closeTo(map[string]bool{"p": true}, buttonScope, false)
			}
			push(tag)
			if formattingElements[tag] {
				key := tag + "\x00" + attrs.String()
				same, first := 0, -1
				for i := len(active) - 1; i >= 0 && active[i][0] != '|'; i-- {
					if active[i] == key {
						same++
						first = i
					}
				}
				if same >= 3 {
					activeAttrs -= keyAttrs(active[first])
					activeBytes -= len(active[first])
					active = append(active[:first], active[first+1:]...)
					nFmt--
				}
				active = append(active, key)
				nFmt++
				activeAttrs += keyAttrs(key)
				activeBytes += len(key)
			}
			if depth() > max {
				return off, true
			}
		case html.EndTagToken:
			name, _ := z.TagName()
			tag := string(name)
			if formattingElements[tag] {
				// Only back to the last scope marker, as the parser does.
				for i := len(active) - 1; i >= 0 && active[i][0] != '|'; i-- {
					if strings.HasPrefix(active[i], tag+"\x00") {
						activeAttrs -= keyAttrs(active[i])
						activeBytes -= len(active[i])
						active = append(active[:i], active[i+1:]...)
						nFmt--
						break
					}
				}
			}
			if tag == "br" || tag == "p" {
				// </br> becomes a <br> element; a </p> without an open p
				// inserts an empty one: either way the text node ends.
				merging = false
			}
			foster := tableCtx() && tableStructure[tag]
			if tables > 0 && ignoredInBody[tag] && !foster {
				merging = false
			}
			if voidElements[tag] {
				break
			}
			// An end tag closes its element only when it is reachable: an
			// inline one ("any other end tag") stops at any special element,
			// a special one at a scope boundary.
			for i := len(stack) - 1; i >= 0; i-- {
				if stack[i] == tag {
					pop(i)
					if !foster {
						merging = false
					}
					break
				}
				switch {
				case tableEnd[tag]:
					if stack[i] == "html" || stack[i] == "template" || (tag != "table" && stack[i] == "table") {
						i = -1
					}
				case !specialElements[tag] && specialElements[stack[i]], buttonScope[stack[i]]:
					i = -1
				}
				if i < 0 {
					break
				}
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
		if n.Namespace == "svg" && (tag == "filter" || strings.HasPrefix(tag, "fe")) {
			// SVG filter primitives paint in software and can take tens of
			// seconds (feMorphology radius=400 on nested groups).
			n.Parent.RemoveChild(n)
			s.drop("element:filter")
			continue
		}
		if n.Namespace != "" && (foreignDropped[tag] || renderVoid[tag]) {
			// (a void HTML name with children cannot be serialized)
			n.Parent.RemoveChild(n)
			s.drop("element:foreign_" + tag)
			continue
		}
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
		if tag == "form" {
			s.drop("element:form")
			if n.FirstChild == nil {
				// In table structure the parser inserts the form empty;
				// nothing is lost removing it.
				n.Parent.RemoveChild(n)
				continue
			}
			// Unwrapping a form can leave a tree the parser never builds
			// (li>div>li), which Chromium re-parses differently. Replace it
			// with an attribute-less <section>: also a block the parser
			// treats as "special", but inert.
			n.Data, n.DataAtom, n.Attr = "section", atom.Section, nil
			tag = "section"
		}
		if tag == "img" || tag == "image" {
			if ph := s.image(n); ph != nil {
				if !inForeign(n) {
					// An HTML node inside <svg>/<math> makes the browser
					// leave foreign content there: no placeholder.
					n.Parent.InsertBefore(ph, n)
				}
				n.Parent.RemoveChild(n)
				continue
			}
		}
		s.attrs(n, tag)
		if tag == "style" {
			s.styleElement(n)
			s.styles = append(s.styles, n)
			continue
		}
		pushChildren(n, f.depth+1)
	}
}

// hoistStyles moves every <style> to the top of the fragment as a plain
// HTML <style>. Style sheets apply page-wide wherever they sit, but inside
// <svg>/<math> (or after a placeholder that makes Chromium leave foreign
// content) the serializer escapes the text while Chromium reads it raw, so
// the sanitizer's quotes would not be the browser's.
func (s *sanitizer) hoistStyles(root *html.Node) {
	type sheet struct{ css, media string }
	var sheets []sheet
	for _, n := range s.styles {
		media, _ := getAttr(n, "media")
		media = strings.TrimSpace(media)
		typ, hasType := getAttr(n, "type")
		if hasType && strings.TrimSpace(typ) != "" && !strings.EqualFold(strings.TrimSpace(typ), "text/css") {
			// The browser ignores a sheet of another type.
			if n.Parent != nil {
				n.Parent.RemoveChild(n)
			}
			continue
		}
		if !safeMedia.MatchString(media) {
			s.drop("css:media_attr")
			if n.Parent != nil {
				n.Parent.RemoveChild(n)
			}
			continue
		}
		var css strings.Builder
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.TextNode {
				css.WriteString(c.Data)
			}
		}
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
		if strings.TrimSpace(css.String()) != "" {
			sheets = append(sheets, sheet{css.String(), media})
		}
	}
	anchor := root.FirstChild
	for _, sh := range sheets {
		st := &html.Node{Type: html.ElementNode, Data: "style", DataAtom: atom.Style}
		if sh.media != "" {
			// media= keeps print-only or narrow-screen sheets inactive.
			st.Attr = []html.Attribute{{Key: "media", Val: sh.media}}
		}
		st.AppendChild(&html.Node{Type: html.TextNode, Data: sh.css})
		root.InsertBefore(st, anchor)
	}
}

// imageSize reads an inline image's pixel size from its header. Chromium
// decodes more BMP variants than x/image/bmp, so a BMP header is read
// directly when DecodeConfig gives up.
func imageSize(data []byte) (w, h int, ok bool) {
	if len(data) >= 6 && (string(data[:6]) == "GIF87a" || string(data[:6]) == "GIF89a") {
		return gifSize(data)
	}
	if cfg, _, err := image.DecodeConfig(bytes.NewReader(data)); err == nil {
		return cfg.Width, cfg.Height, true
	}
	if len(data) < 26 || data[0] != 'B' || data[1] != 'M' {
		return 0, 0, false
	}
	le32 := func(b []byte) int64 {
		return int64(int32(uint32(b[0]) | uint32(b[1])<<8 | uint32(b[2])<<16 | uint32(b[3])<<24))
	}
	var ww, hh int64
	if le32(data[14:18]) == 12 { // OS/2 BITMAPCOREHEADER: 16-bit sizes
		ww, hh = int64(uint16(data[18])|uint16(data[19])<<8), int64(uint16(data[20])|uint16(data[21])<<8)
	} else {
		ww, hh = le32(data[18:22]), le32(data[22:26])
	}
	if hh < 0 {
		hh = -hh
	}
	if ww <= 0 || hh <= 0 || ww > 1<<20 || hh > 1<<20 {
		return 0, 0, false
	}
	return int(ww), int(hh), true
}

// gifSize is the size a browser decodes a GIF at: the logical screen grown
// to hold every frame (a 1x1 screen can carry a 16384x16384 frame).
func gifSize(d []byte) (w, h int, ok bool) {
	le16 := func(i int) int { return int(d[i]) | int(d[i+1])<<8 }
	if len(d) < 13 {
		return 0, 0, false
	}
	w, h = le16(6), le16(8)
	i := 13
	if d[10]&0x80 != 0 {
		i += 3 << (d[10]&7 + 1)
	}
	skipBlocks := func() bool {
		for i < len(d) {
			n := int(d[i])
			i++
			if n == 0 {
				return true
			}
			i += n
		}
		return false
	}
	for frames := 0; i < len(d); {
		switch d[i] {
		case 0x3B: // trailer
			return w, h, w > 0 && h > 0
		case 0x21: // extension
			i += 2
			if !skipBlocks() {
				return w, h, w > 0 && h > 0
			}
		case 0x2C: // image descriptor
			if i+10 > len(d) { // truncated: the frame never decodes
				return w, h, w > 0 && h > 0
			}
			if r := le16(i+1) + le16(i+5); r > w {
				w = r
			}
			if b := le16(i+3) + le16(i+7); b > h {
				h = b
			}
			packed := d[i+9]
			i += 10
			if packed&0x80 != 0 {
				i += 3 << (packed&7 + 1)
			}
			i++ // LZW minimum code size
			if !skipBlocks() {
				return w, h, w > 0 && h > 0
			}
			if frames++; frames > 10000 {
				return 0, 0, false
			}
		default:
			return w, h, w > 0 && h > 0
		}
	}
	return w, h, w > 0 && h > 0
}

// inForeign reports whether n is, or sits inside, an <svg>/<math> element.
func inForeign(n *html.Node) bool {
	for p := n; p != nil; p = p.Parent {
		if p.Type == html.ElementNode && p.Namespace != "" {
			return true
		}
	}
	return false
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
				// Record and extract it like a src= data image; with no
				// src, it becomes the image's src.
				if _, ok := s.inlineImage(cand); ok && (!hasSrc || strings.TrimSpace(src) == "") {
					src, hasSrc = s.lastCanonical, true
					removeAttr(n, "src")
					n.Attr = append(n.Attr, html.Attribute{Key: "src", Val: src})
				}
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
			a.Val = s.lastCanonical
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
					a.Val = s.lastCanonical
					break
				}
				continue
			}
			r := s.classify(v)
			s.addRef(r)
			if key == "background" && bgPlaceholder == nil && !inForeign(n) {
				bgPlaceholder = placeholder(n, refLabel(r))
			}
			continue
		default:
			if scriptScheme(a.Val) {
				s.drop("attr:" + key + ":" + schemeOf(a.Val))
				continue
			}
			if n.Namespace != "" && key == "filter" {
				s.drop("attr:filter") // see element:filter
				continue
			}
			if n.Namespace != "" && strings.Contains(a.Val, `\`) {
				// CSS escapes in a presentation attribute could spell url(.
				s.drop("attr:" + key + ":escaped")
				continue
			}
			if lv := strings.ToLower(a.Val); n.Namespace != "" && (strings.Contains(lv, "image-set(") || strings.Contains(lv, "src(")) {
				// image-set() takes plain string URLs, no url() needed.
				for _, m := range cssQuoted.FindAllStringSubmatch(a.Val, -1) {
					s.cssRef(firstNonEmpty(m[1], m[2]))
				}
				s.attrURLs(a.Val)
				s.drop("attr:" + key + ":image_set")
				continue
			}
			if n.Namespace != "" && s.attrURLs(a.Val) {
				s.drop("attr:" + key + ":url")
				continue
			}
			if n.Namespace != "" {
				a.Val = fragmentURL.ReplaceAllString(a.Val, "${1}#x_")
			}
		}
		kept = append(kept, a)
	}
	n.Attr = kept
	if bgPlaceholder != nil {
		s.generated[bgPlaceholder] = true
		switch {
		case tableParts[tag]:
			// A <span> inside table structure is moved out by the parser
			// (foster parenting): label the whole table from before it.
			t := n
			for t.Parent != nil && tableParts[strings.ToLower(t.Data)] && strings.ToLower(t.Data) != "table" {
				t = t.Parent
			}
			if t.Parent != nil {
				t.Parent.InsertBefore(bgPlaceholder, t)
			}
		case !voidElements[tag]:
			n.InsertBefore(bgPlaceholder, n.FirstChild)
		case n.Parent != nil:
			// A void element cannot hold children: label it from outside.
			n.Parent.InsertBefore(bgPlaceholder, n.NextSibling)
		}
	}
}

// attrURLs records the url() references in an attribute value (SVG
// presentation attributes such as fill, filter and mask take them) and
// reports whether any points outside the document; "url(#id)" is kept.
func (s *sanitizer) attrURLs(v string) bool {
	if !strings.Contains(strings.ToLower(v), "url(") {
		return false
	}
	external := false
	for _, m := range cssURL.FindAllStringSubmatch(v, -1) {
		u := strings.TrimSpace(firstNonEmpty(m[1], m[2], m[3]))
		if strings.HasPrefix(u, "#") {
			continue
		}
		external = true
		s.cssRef(u)
	}
	if strings.Count(strings.ToLower(v), "url(") != len(cssURL.FindAllStringIndex(v, -1)) {
		external = true // an url( the pattern did not parse: drop it
	}
	return external
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
		a.Val = prefixFragment(v)
		return true
	}
	if v == "" || strings.HasPrefix(v, "#") {
		a.Val = prefixFragment(v)
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
		if n.Parent != nil && !inForeign(n) {
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
	// The renderer decodes the whole bitmap whatever size it is shown at:
	// a 51 KB 16000x16000 PNG costs ~1 GB. Bound the pixels per image and
	// per body.
	sum := sha256.Sum256(data)
	sha := hex.EncodeToString(sum[:])
	r := RemoteRef{Kind: "inline_data", Value: "data:" + mime, SHA256: sha, Mime: mime}
	if !s.pixelSeen[sha] {
		w, h, ok := imageSize(data)
		if !ok {
			s.drop("data_image_malformed")
			s.addRef(r) // dropped, but still evidence
			return RemoteRef{}, false
		}
		px := int64(w) * int64(h)
		if px > maxInlinePixels || s.inlinePixels+px > maxInlinePixelsTotal {
			s.drop("data_image_too_many_pixels")
			s.addRef(r)
			return RemoteRef{}, false
		}
		s.inlinePixels += px // each distinct image once
		s.pixelSeen[sha] = true
	}
	if s.inlineDir != "" {
		name := sha + "." + ext
		if !s.inlineSeen[name] {
			if _, err := os.Stat(filepath.Join(s.inlineDir, name)); err != nil {
				if s.inlineFiles >= maxInlineImages {
					s.drop("inline_images_capped")
					return RemoteRef{}, false
				}
				s.inlineFiles++
			}
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
	// The kept URL is rewritten to one canonical form per image, so
	// aliases of the same bytes (extra media parameters, percent- vs
	// base64-encoding) are one resource to Chromium too: the pixel budget
	// charges each distinct image once.
	s.lastCanonical = "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data)
	return r, true
}

// ---------------------------------------------------------------------------
// CSS
// ---------------------------------------------------------------------------

// fragmentURL matches the start of url(#...) / url("#...") / url('#...').
var fragmentURL = regexp.MustCompile(`(?i)(url\(\s*["']?)#`)

// prefixFragment rewrites "#name" to "#x_name", matching the x_ id prefix.
func prefixFragment(v string) string {
	if len(v) > 1 && v[0] == '#' {
		return "#x_" + v[1:]
	}
	return v
}

// safeMedia is a media query list made of plain tokens only.
var safeMedia = regexp.MustCompile(`^[a-zA-Z0-9 ,:().\-]*$`)

var (
	cssComment = regexp.MustCompile(`(?s)/\*.*?\*/`)
	cssQuoted  = regexp.MustCompile(`"([^"]*)"|'([^']*)'`)
	cssURL     = regexp.MustCompile(`(?i)url\(\s*(?:"([^"]*)"|'([^']*)'|([^)]*?))\s*\)`)
)

// stylesheet sanitizes a style sheet and scopes every selector to .owa-body.
func (s *sanitizer) stylesheet(css string) string {
	css = stripCSSComments(css)
	// CSS ignores <!-- and --> around a sheet (Word/Outlook always write
	// them); remove them rather than let them join the first selector.
	css = strings.NewReplacer("<!--", " ", "-->", " ").Replace(css)
	var out strings.Builder
	s.ruleList(css, &out, 0)
	if badCSSString(out.String()) {
		// Every piece is checked below; this guards the assembled sheet.
		s.drop("css:bad_string")
		return ""
	}
	if out.Len() > 0 && !validScopedCSS(out.String()) {
		// A real CSS parser would not read this sheet the way the scanner
		// above did; drop it rather than risk unscoped rules.
		s.drop("css:unsafe_structure")
		return ""
	}
	return out.String()
}

// stripCSSComments removes comments until none are left: one pass can join
// the halves around a removed comment into a new one ("//**/*" -> "/*").
func stripCSSComments(css string) string {
	for i := 0; i < 64 && strings.Contains(css, "/*"); i++ {
		next := cssComment.ReplaceAllString(css, "")
		if next == css {
			break
		}
		css = next
	}
	return css
}

// badCSSString reports whether s has a quoted string that a CSS tokenizer
// ends early: at a raw newline (a bad-string token) or at the end of s. The
// sanitizer reads quotes without that rule, so such a string could hide
// braces from it that Chromium then sees as top-level, unscoped rules.
func badCSSString(s string) bool {
	var quote byte
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			} else if c == '\n' || c == '\r' || c == '\f' {
				return true
			}
		case c == '"' || c == '\'':
			quote = c
		}
	}
	return quote != 0
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
		if badCSSString(prelude) {
			s.drop("css:bad_string")
			continue
		}
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
	for _, d := range splitTop(stripCSSComments(body), ';') {
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
		case badCSSString(d):
			s.drop("css:bad_string")
			continue
		case strings.ContainsAny(d, "{}"):
			// A nested style rule would escape the checks below.
			s.drop("css:nested_rule")
			continue
		case prop == "filter" || prop == "backdrop-filter" || prop == "-webkit-filter" || prop == "-webkit-backdrop-filter":
			// Nested blurs cost seconds each to paint: a few hundred bytes
			// pushed the render past its timeout, so no view was captured.
			s.drop("css:filter")
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
		case strings.Count(lval, "url(") != len(cssURL.FindAllStringIndex(val, -1)):
			// An url( left open runs to the end of the declaration in CSS
			// and still loads; the pattern only neutralizes closed ones.
			s.drop("css:url_unclosed")
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
					return "url(" + s.lastCanonical + ")"
				}
				return "none"
			}
			if strings.HasPrefix(u, "#") && len(u) > 1 && !strings.ContainsAny(u, `"'()`) {
				// An in-document reference (gradient, mask, filter): follow
				// the x_ id prefix.
				return "url('" + prefixFragment(u) + "')"
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
