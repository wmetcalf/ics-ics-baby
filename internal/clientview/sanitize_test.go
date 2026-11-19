package clientview

import (
	"errors"
	"io"

	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"golang.org/x/net/html"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"
)

var update = flag.Bool("update", false, "rewrite golden files")

const probeDir = "testdata/owa-probes"

type probeGolden struct {
	RemoteRefs []RemoteRef `json:"remote_refs"`
	Links      []Link      `json:"links"`
	Dropped    []string    `json:"dropped"`
}

func probeFiles(t *testing.T) []string {
	t.Helper()
	all, err := filepath.Glob(filepath.Join(probeDir, "*.html"))
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, f := range all {
		if !strings.HasSuffix(f, ".golden.html") {
			out = append(out, f)
		}
	}
	if len(out) < 30 {
		t.Fatalf("probe corpus too small: %d", len(out))
	}
	return out
}

func sanitizeProbe(t *testing.T, path string) SanitizeResult {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	res, err := OWASanitize(string(raw), filepath.Join(t.TempDir(), "ics-ics-baby-inline"))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	return res
}

func TestOWAProbesGolden(t *testing.T) {
	for _, p := range probeFiles(t) {
		p := p
		base := strings.TrimSuffix(p, ".html")
		t.Run(filepath.Base(base), func(t *testing.T) {
			res := sanitizeProbe(t, p)
			gj, err := json.MarshalIndent(probeGolden{res.RemoteRefs, res.Links, res.Dropped}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			gj = append(gj, '\n')
			if *update {
				if err := os.WriteFile(base+".golden.html", []byte(res.HTML+"\n"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(base+".golden.json", gj, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			wantHTML, err := os.ReadFile(base + ".golden.html")
			if err != nil {
				t.Fatalf("missing golden (run -update): %v", err)
			}
			if res.HTML+"\n" != string(wantHTML) {
				t.Errorf("html mismatch\n got: %s\nwant: %s", res.HTML, wantHTML)
			}
			wantJSON, err := os.ReadFile(base + ".golden.json")
			if err != nil {
				t.Fatal(err)
			}
			if string(gj) != string(wantJSON) {
				t.Errorf("json mismatch\n got: %s\nwant: %s", gj, wantJSON)
			}
		})
	}
}

var activeContent = regexp.MustCompile(`(?i)<script|\son\w+\s*=|javascript:|vbscript:|<iframe|<object|<embed|<form|<input|<meta|<link|<base|srcdoc`)

func TestNoActiveContentSurvives(t *testing.T) {
	for _, p := range probeFiles(t) {
		res := sanitizeProbe(t, p)
		if m := activeContent.FindString(res.HTML); m != "" {
			t.Errorf("%s: active content %q survived: %s", p, m, res.HTML)
		}
	}
}

func TestRemoteImagePlaceholder(t *testing.T) {
	res, err := OWASanitize(`<img src="http://evil.test/p.gif" width=1 height=1>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `class="owa-remote-ref"`) || !strings.Contains(res.HTML, "remote image: evil.test") {
		t.Fatalf("no placeholder: %s", res.HTML)
	}
	if strings.Contains(res.HTML, "http://evil.test/p.gif\"") || strings.Contains(res.HTML, "<img") {
		t.Fatalf("remote img survived: %s", res.HTML)
	}
	if len(res.RemoteRefs) != 1 || res.RemoteRefs[0] != (RemoteRef{Kind: "remote", Host: "evil.test", Value: "http://evil.test/p.gif"}) {
		t.Fatalf("RemoteRefs = %+v", res.RemoteRefs)
	}
}

func TestLocalAndCidRefs(t *testing.T) {
	res, err := OWASanitize(`<img src="logo.png"><img src="cid:abc">`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, "local reference: logo.png") || !strings.Contains(res.HTML, "inline cid: abc") {
		t.Fatalf("placeholders missing: %s", res.HTML)
	}
	if len(res.RemoteRefs) != 2 || res.RemoteRefs[0].Kind != "local" || res.RemoteRefs[1].Kind != "cid" {
		t.Fatalf("RemoteRefs = %+v", res.RemoteRefs)
	}
}

const onePxPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="

func TestInlineDataWritten(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ics-ics-baby-inline")
	res, err := OWASanitize(`<img src="data:image/png;base64,`+onePxPNG+`">`, dir)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `src="data:image/png;base64,`) {
		t.Fatalf("data image not kept: %s", res.HTML)
	}
	raw, _ := base64.StdEncoding.DecodeString(onePxPNG)
	sum := sha256.Sum256(raw)
	sha := hex.EncodeToString(sum[:])
	got, err := os.ReadFile(filepath.Join(dir, sha+".png"))
	if err != nil || string(got) != string(raw) {
		t.Fatalf("inline file not written correctly: %v", err)
	}
	want := RemoteRef{Kind: "inline_data", Value: "data:image/png", Path: "ics-ics-baby-inline/" + sha + ".png", SHA256: sha, Mime: "image/png"}
	if len(res.RemoteRefs) != 1 || res.RemoteRefs[0] != want {
		t.Fatalf("RemoteRefs = %+v, want %+v", res.RemoteRefs, want)
	}
}

func TestDataImageCap(t *testing.T) {
	big := make([]byte, maxInlineImageBytes+1)
	copy(big, []byte("\x89PNG\r\n\x1a\n"))
	src := "data:image/png;base64," + base64.StdEncoding.EncodeToString(big)
	s := newSanitizer("")
	if _, ok := s.inlineImage(src); ok {
		t.Fatal("data image over the cap was accepted")
	}
	if !strings.Contains(strings.Join(s.dropped, ","), "data_image_too_large") {
		t.Fatalf("dropped = %v", s.dropped)
	}
}

func TestStyleScoped(t *testing.T) {
	res, err := OWASanitize(`<style>p{color:red}</style><p>x</p>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, ".owa-body p{color:red}") {
		t.Fatalf("style not scoped: %s", res.HTML)
	}
}

func TestSanitizeCaps(t *testing.T) {
	t.Run("10MB", func(t *testing.T) {
		in := "<p>" + strings.Repeat("A", 10<<20) + "</p>"
		start := time.Now()
		res, err := OWASanitize(in, "")
		if err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 2*raceSlowdown*time.Second {
			t.Fatalf("took %v", d)
		}
		if len(res.HTML) > maxSanitizeInput {
			t.Fatalf("output %d bytes > cap", len(res.HTML))
		}
		if !contains(res.Dropped, "input_truncated") {
			t.Fatalf("dropped = %v", res.Dropped)
		}
	})
	t.Run("deep", func(t *testing.T) {
		in := strings.Repeat("<div>", 100000) + "x" + strings.Repeat("</div>", 100000)
		start := time.Now()
		res, err := OWASanitize(in, "")
		if err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 2*raceSlowdown*time.Second {
			t.Fatalf("took %v", d)
		}
		if len(res.HTML) > maxSanitizeInput {
			t.Fatalf("output %d bytes > cap", len(res.HTML))
		}
		if !contains(res.Dropped, "input_truncated") || !contains(res.Dropped, "depth_limit") {
			t.Fatalf("dropped = %v", res.Dropped)
		}
	})
}

// Nested tables add implied tbody/tr/td to the parser stack; the sanitizer
// must still return (truncated) rather than fail.
func TestSanitizeNestedTables(t *testing.T) {
	in := strings.Repeat("<table><tr><td>", 400) + "x"
	res, err := OWASanitize(in, "")
	if err != nil {
		t.Fatalf("nested tables: %v", err)
	}
	if !contains(res.Dropped, "depth_limit") {
		t.Fatalf("dropped = %v", res.Dropped)
	}
}

// tinyPNG is a valid, distinct 1x1 PNG for each i.
func tinyPNG(t *testing.T, i int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	img.Pix[0], img.Pix[1], img.Pix[2], img.Pix[3] = byte(i), byte(i>>8), byte(i>>16), 255
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Selectors must not escape the body container through sibling combinators
// (e.g. hiding the emulation footer with "body ~ div").
func TestSelectorsStayInBody(t *testing.T) {
	res, err := OWASanitize(`<style>body ~ div{display:none} ~ footer{display:none} + .x{color:red} p + p{margin:0}</style>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, ".owa-body ~") || strings.Contains(res.HTML, ".owa-body +") {
		t.Fatalf("sibling escape survived: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, ".owa-body p + p{margin:0}") {
		t.Fatalf("legit sibling selector lost: %s", res.HTML)
	}
}

// Review finding 1: placeholders grow the markup past the cap; the output must
// be cut at node boundaries, never mid-tag or inside an open <style>.
func TestSanitizeOutputCapWellFormed(t *testing.T) {
	in := strings.Repeat("<img src=//a>", 8736) + "<style>p{x:" + strings.Repeat("a", 5000) + "}</style><p>tail</p>"
	res, err := OWASanitize(in, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.HTML) > maxSanitizeInput {
		t.Fatalf("output %d > cap", len(res.HTML))
	}
	if !contains(res.Dropped, "output_truncated") {
		t.Fatalf("dropped = %v", res.Dropped)
	}
	if strings.Count(res.HTML, "<style") != strings.Count(res.HTML, "</style>") {
		t.Fatal("output ends inside a <style> element")
	}
	if strings.Count(res.HTML, "<span") != strings.Count(res.HTML, "</span>") || strings.HasSuffix(res.HTML, "sty") {
		t.Fatalf("output cut mid-element: ...%s", res.HTML[len(res.HTML)-80:])
	}
}

// Review finding 2: root selectors must target an inner element, never the
// clipping container itself.
func TestRootSelectorsTargetInnerRoot(t *testing.T) {
	res, err := OWASanitize(`<style>body{overflow:visible!important} html body p{color:red} :root{margin:0}</style>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, ".owa-body{") || strings.Contains(res.HTML, ".owa-body ") && !strings.Contains(res.HTML, ".owa-body .owa-root") {
		t.Fatalf("root selector styles the container: %s", res.HTML)
	}
	for _, want := range []string{".owa-body .owa-root{overflow:visible!important}", ".owa-body .owa-root p{color:red}"} {
		if !strings.Contains(res.HTML, want) {
			t.Fatalf("missing %q in %s", want, res.HTML)
		}
	}
}

// Review finding 3: position:fixed through a custom property.
func TestPositionVarRejected(t *testing.T) {
	res, err := OWASanitize(`<div style="--p:fixed;position:var(--p);top:0">x</div><style>.o{--p:fixed;position:var(--p)}</style>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "position:var") {
		t.Fatalf("position:var survived: %s", res.HTML)
	}
}

// Review finding 4: raw-text elements that swallow the rest of the page.
func TestRawTextElementsDropped(t *testing.T) {
	for _, tag := range []string{"plaintext", "xmp", "noembed", "noframes", "listing"} {
		res, err := OWASanitize("hi<"+tag+">x</"+tag+">", "")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(res.HTML, "<"+tag) {
			t.Errorf("%s kept: %s", tag, res.HTML)
		}
	}
}

// Deferred minor: body @font-face could remap the view's own fonts with
// local(); body CSS may not declare fonts at all.
func TestFontFaceStripped(t *testing.T) {
	res, err := OWASanitize(`<style>@font-face{font-family:"Segoe UI";src:local(Wingdings)}@font-face{font-family:x;src:url(http://evil.test/f.woff)}p{color:red}</style>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ToLower(res.HTML), "@font-face") {
		t.Fatalf("@font-face survived: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, ".owa-body p{color:red}") {
		t.Fatalf("following rule lost: %s", res.HTML)
	}
	if len(res.RemoteRefs) != 1 || res.RemoteRefs[0].Host != "evil.test" {
		t.Fatalf("remote font not recorded: %+v", res.RemoteRefs)
	}
}

// A single <style> text node bypasses the HTML depth cut, so nested
// conditional group rules must be bounded on their own: hostile invites with
// tens of thousands of nested @media blocks must not stall WriteOWA (which
// runs before Chromium's timeout applies).
func TestCSSNestingBounded(t *testing.T) {
	const levels = 50000
	css := strings.Repeat("@media all{@supports (display:block){", levels/2) +
		"p{color:red}" + strings.Repeat("}", levels)
	start := time.Now()
	res, err := OWASanitize("<style>"+css+"</style><p>x</p>", "")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*raceSlowdown*time.Second {
		t.Fatalf("took %v", d)
	}
	if !contains(res.Dropped, "css:nesting_limit") {
		t.Fatalf("dropped = %v", res.Dropped)
	}
	if strings.Count(res.HTML, "@media") > maxCSSNesting {
		t.Fatalf("kept more than %d nested group rules", maxCSSNesting)
	}

	// Ordinary nesting keeps working.
	res, err = OWASanitize(`<style>@media screen{@supports (display:grid){p{color:red}}}</style><p>x</p>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, "@supports (display:grid){.owa-body p{color:red}}") || contains(res.Dropped, "css:nesting_limit") {
		t.Fatalf("shallow nesting mangled: %s / %v", res.HTML, res.Dropped)
	}
}

// data: candidates in srcset are self-contained, not remote references.
func TestSrcsetDataNotRemote(t *testing.T) {
	res, err := OWASanitize(`<img srcset="data:image/png;base64,`+onePxPNG+` 1x, http://evil.test/a.png 2x">`, "")
	if err != nil {
		t.Fatal(err)
	}
	// The data: candidate is extracted like a src= data image (with its
	// hash); the remote one is a remote ref.
	if len(res.RemoteRefs) != 2 || res.RemoteRefs[0].Kind != "inline_data" || res.RemoteRefs[0].SHA256 == "" ||
		res.RemoteRefs[1].Kind != "remote" || res.RemoteRefs[1].Host != "evil.test" {
		t.Fatalf("RemoteRefs = %+v", res.RemoteRefs)
	}
}

// The number of inline image files written per body is bounded: 2 MiB of
// distinct tiny images must not become tens of thousands of files.
func TestInlineImageFilesCapped(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ics-ics-baby-inline")
	var b strings.Builder
	for i := 0; i < maxInlineImages+50; i++ {
		b.WriteString(`<img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(tinyPNG(t, i)) + `">`)
	}
	res, err := OWASanitize(b.String(), dir)
	if err != nil {
		t.Fatal(err)
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) > maxInlineImages {
		t.Fatalf("wrote %d inline files, cap %d", len(ents), maxInlineImages)
	}
	if !contains(res.Dropped, "inline_images_capped") {
		t.Fatalf("dropped = %v", res.Dropped)
	}
	if n := strings.Count(res.HTML, "<img"); n > maxInlineImages {
		t.Fatalf("kept %d inline images past the cap", n)
	}
}

// Nested style rules inside a declaration block would bypass the
// declaration-level rules (e.g. position:fixed); they are dropped.
func TestNestedRuleInDeclarationsDropped(t *testing.T) {
	res, err := OWASanitize(`<style>p{color:red;& ~ div{position:fixed;top:0}}</style><p>x</p>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "fixed") || !contains(res.Dropped, "css:nested_rule") {
		t.Fatalf("nested rule survived: %s / %v", res.HTML, res.Dropped)
	}
	if !strings.Contains(res.HTML, ".owa-body p{color:red}") {
		t.Fatalf("plain declaration lost: %s", res.HTML)
	}
}

// An SVG <image href="data:image/png..."> is kept like <img src="data:...">.
func TestSVGImageDataHrefKept(t *testing.T) {
	res, err := OWASanitize(`<svg><image href="data:image/png;base64,`+onePxPNG+`" width="10" height="10"></image></svg>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `href="data:image/png;base64,`) {
		t.Fatalf("inline svg image lost its href: %s / %v", res.HTML, res.Dropped)
	}
	if len(res.RemoteRefs) != 1 || res.RemoteRefs[0].Kind != "inline_data" {
		t.Fatalf("RemoteRefs = %+v", res.RemoteRefs)
	}
}

// Unit form of TestOWABodyBadStringCannotEscapeScope: no sanitized style
// sheet may carry a string that a CSS tokenizer ends at a newline.
func TestBadCSSStringDropped(t *testing.T) {
	for _, css := range []string{
		"p[title=\"x\n]{}.owa-head{color:red}\np[title=\"]{color:blue}",
		"p{content:\"x\n}.owa-head{color:red}\np{content:\"}",
		"@media (min-width:\"1\n){}.owa-head{color:red}\n@media \"){p{color:blue}}",
		"p{color:red}q{content:'x\r'}",
	} {
		res, err := OWASanitize("<style>"+css+"</style><p>x</p>", "")
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(res.HTML, "owa-head") || !contains(res.Dropped, "css:bad_string") {
			t.Fatalf("%q -> %s / %v", css, res.HTML, res.Dropped)
		}
	}
}

// SVG presentation attributes (fill, filter, mask, ...) can reference
// resources with url(); remote ones are recorded and the attribute dropped,
// in-document fragments (gradients) are kept.
func TestSVGPresentationURLRecorded(t *testing.T) {
	res, err := OWASanitize(`<svg><rect fill="url(https://evil.test/p.svg#x)"></rect><rect fill="url(#grad)"></rect><rect filter="u\72l(https://evil.test/f)"></rect></svg>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "evil.test") {
		t.Fatalf("remote url() survived: %s", res.HTML)
	}
	if !strings.Contains(res.HTML, `fill="url(#x_grad)"`) {
		t.Fatalf("fragment url() lost: %s", res.HTML)
	}
	if len(res.RemoteRefs) != 1 || res.RemoteRefs[0].Host != "evil.test" {
		t.Fatalf("RemoteRefs = %+v", res.RemoteRefs)
	}
}

// background= on a void element must not make rendering fail.
func TestBackgroundOnVoidElement(t *testing.T) {
	for _, tag := range []string{"br", "hr", "wbr", "img"} {
		res, err := OWASanitize(`<`+tag+` background="https://evil.test/bg.png">x`, "")
		if err != nil {
			t.Fatalf("%s: %v", tag, err)
		}
		if len(res.RemoteRefs) != 1 || res.RemoteRefs[0].Host != "evil.test" {
			t.Fatalf("%s: refs %+v", tag, res.RemoteRefs)
		}
	}
}

// Formatting-element reconstruction can turn a small body into millions of
// nodes; the tree size is bounded before parsing.
func TestFormattingReconstructionBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("<p>")
	for i := 0; i < 450; i++ {
		b.WriteString(`<b x=` + strconv.Itoa(i) + `>`)
	}
	b.WriteString("</p>")
	b.WriteString(strings.Repeat("<p>x</p>", 60000) /* unbounded: ~10 s */)
	start := time.Now()
	res, err := OWASanitize(b.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*raceSlowdown*time.Second {
		t.Fatalf("took %v", d)
	}
	if !contains(res.Dropped, "input_truncated") {
		t.Fatalf("dropped = %v", res.Dropped)
	}
}

// image-set() in an SVG presentation attribute loads images without url().
func TestSVGPresentationImageSet(t *testing.T) {
	res, err := OWASanitize(`<svg><rect mask="image-set('https://mask.evil.test/m.png' 1x)"></rect><rect cursor="-webkit-image-set(&quot;https://c.evil.test/c.png&quot; 1x), auto"></rect></svg>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "evil.test") {
		t.Fatalf("image-set survived: %s", res.HTML)
	}
	if len(res.RemoteRefs) != 2 {
		t.Fatalf("RemoteRefs = %+v", res.RemoteRefs)
	}
}

// The inline image cap holds per output directory, across bodies.
func TestInlineImageCapSharedAcrossBodies(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ics-ics-baby-inline")
	n := 0
	for body := 0; body < 3; body++ {
		var b strings.Builder
		for i := 0; i < maxInlineImages/2+10; i++ {
			b.WriteString(`<img src="data:image/png;base64,` + base64.StdEncoding.EncodeToString(tinyPNG(t, body*100000+i)) + `">`)
		}
		if _, err := OWASanitize(b.String(), dir); err != nil {
			t.Fatal(err)
		}
		n++
	}
	ents, _ := os.ReadDir(dir)
	if len(ents) != maxInlineImages {
		t.Fatalf("%d bodies wrote %d inline files, want the cap %d", n, len(ents), maxInlineImages)
	}
}

// Deeply nested ( in a declaration must not make CSS validation quadratic.
func TestCSSValidateNestedParensLinear(t *testing.T) {
	css := "p{color:" + strings.Repeat("(", 200000) + "x" + strings.Repeat(")", 200000) + "}"
	start := time.Now()
	if _, err := OWASanitize("<style>"+css+"</style><p>x</p>", ""); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*raceSlowdown*time.Second {
		t.Fatalf("took %v", d)
	}
}

// An enclosing end tag pops formatting elements off the open stack, but the
// parser keeps them in its active list and re-creates them later.
func TestFormattingReconstructionAfterEnclosingClose(t *testing.T) {
	var b strings.Builder
	b.WriteString("<div>")
	for i := 0; i < 450; i++ {
		b.WriteString(`<b x=` + strconv.Itoa(i) + `>`)
	}
	b.WriteString("</div>")
	b.WriteString(strings.Repeat("<p>x</p>", 60000))
	start := time.Now()
	res, err := OWASanitize(b.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 3*raceSlowdown*time.Second { // unbounded: ~10 s
		t.Fatalf("took %v", d)
	}
	if !contains(res.Dropped, "input_truncated") {
		t.Fatalf("dropped = %v", res.Dropped)
	}
}

// Round-3 review cases.
func TestSanitizeRound3(t *testing.T) {
	san := func(t *testing.T, in string) SanitizeResult {
		t.Helper()
		res, err := OWASanitize(in, "")
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	t.Run("use dropped", func(t *testing.T) {
		res := san(t, `<svg><g id="a"><rect/></g><use href="#x_a"></use></svg>`)
		if strings.Contains(res.HTML, "<use") {
			t.Fatalf("use kept: %s", res.HTML)
		}
	})
	t.Run("formatting ends inside marquee", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < 400; i++ {
			b.WriteString(`<b x=` + strconv.Itoa(i) + `>`)
		}
		b.WriteString("<marquee>" + strings.Repeat("</b>", 400) + "</marquee>")
		b.WriteString(strings.Repeat("<p>x</p>", 60000))
		start := time.Now()
		res := san(t, b.String())
		if d := time.Since(start); d > 5*raceSlowdown*time.Second {
			t.Fatalf("took %v", d)
		}
		if !contains(res.Dropped, "input_truncated") {
			t.Fatalf("dropped = %v", res.Dropped)
		}
	})
	t.Run("re-created formatting depth", func(t *testing.T) {
		var b strings.Builder
		for i := 0; i < 600; i++ {
			b.WriteString(`<div><b id=` + strconv.Itoa(i) + `></div>`)
		}
		res := san(t, b.String()+"x")
		if strings.Contains(res.HTML, "could not be rendered") && !contains(res.Dropped, "depth_limit") {
			t.Fatalf("dropped = %v", res.Dropped)
		}
	})
	t.Run("svg title raw text never fails", func(t *testing.T) {
		san(t, "<svg><title>"+strings.Repeat("<div>", 600)+"x")
	})
	t.Run("CDO CDC around sheet", func(t *testing.T) {
		res := san(t, "<style><!--\np.MsoNormal{margin:0in;font-family:\"Calibri\",sans-serif}\n--></style><p class=MsoNormal>x</p>")
		if !strings.Contains(res.HTML, ".owa-body p.x_MsoNormal{") {
			t.Fatalf("Outlook sheet dropped: %s / %v", res.HTML, res.Dropped)
		}
	})
	t.Run("srcset-only data image kept", func(t *testing.T) {
		res := san(t, `<img srcset="data:image/png;base64,`+onePxPNG+` 1x" width="200" height="200">`)
		if !strings.Contains(res.HTML, `src="data:image/png;base64,`) || len(res.RemoteRefs) != 1 || res.RemoteRefs[0].SHA256 == "" {
			t.Fatalf("%s / %+v", res.HTML, res.RemoteRefs)
		}
	})
	t.Run("svg leaves do not truncate", func(t *testing.T) {
		res := san(t, `<svg>`+strings.Repeat(`<rect width="1" height="1"/>`, 600)+`</svg><p>PAY https://evil.example/</p>`)
		if !strings.Contains(res.HTML, "PAY") {
			t.Fatalf("tail lost: %v", res.Dropped)
		}
	})
	t.Run("style media kept", func(t *testing.T) {
		res := san(t, `<style media="print">p{display:none}</style><p>visible</p>`)
		if !strings.Contains(res.HTML, `<style media="print">`) {
			t.Fatalf("%s", res.HTML)
		}
	})
	t.Run("html title with url( untouched", func(t *testing.T) {
		res := san(t, `<p title="Pay via url(https://pay.example/inv) today">hi</p>`)
		if !strings.Contains(res.HTML, `title="Pay via url(https://pay.example/inv) today"`) || len(res.RemoteRefs) != 0 {
			t.Fatalf("%s / %+v", res.HTML, res.RemoteRefs)
		}
	})
	t.Run("math backslash attr dropped", func(t *testing.T) {
		res := san(t, `<math><mi fill="\75 rl(http://evil.test/a)">x</mi></math>`)
		if strings.Contains(res.HTML, "evil.test") {
			t.Fatalf("%s", res.HTML)
		}
	})
}

// Round-4 review cases.
func TestSanitizeRound4(t *testing.T) {
	san := func(t *testing.T, in string) SanitizeResult {
		t.Helper()
		res, err := OWASanitize(in, "")
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	t.Run("form in table stays renderable", func(t *testing.T) {
		res := san(t, `<p>Confirm</p><table><form action="https://evil.example/l"><tr><td>Password <input type=password></td></tr></form></table>`)
		if contains(res.Dropped, "unstable_markup") || !strings.Contains(res.HTML, "Password") {
			t.Fatalf("%s / %v", res.HTML, res.Dropped)
		}
	})
	t.Run("parse failure keeps evidence", func(t *testing.T) {
		res := san(t, `<p><a href="https://evil.example/pay">Pay now</a><img src="https://track.example/p.gif"></p><svg><title>`+strings.Repeat("<div>", 600))
		if len(res.Links) == 0 || len(res.RemoteRefs) == 0 {
			t.Fatalf("evidence lost: links %v refs %v dropped %v", res.Links, res.RemoteRefs, res.Dropped)
		}
	})
	t.Run("self-closing svg does not hide later depth", func(t *testing.T) {
		res := san(t, `<p><a href="https://evil.example/pay">Pay</a></p><svg width=0 height=0/>`+strings.Repeat("<span/>", 520))
		if contains(res.Dropped, "parse_failed") || len(res.Links) != 1 {
			t.Fatalf("dropped %v links %v", res.Dropped, res.Links)
		}
	})
	t.Run("unclosed font in cells not double counted", func(t *testing.T) {
		res := san(t, "<table>"+strings.Repeat("<tr><td><font face=Arial size=2>item</td></tr>", 250)+"</table><p>END-OF-AGENDA</p>")
		if !strings.Contains(res.HTML, "END-OF-AGENDA") {
			t.Fatalf("truncated: %v", res.Dropped)
		}
	})
	t.Run("implicitly closed cells do not leak markers", func(t *testing.T) {
		if _, cut := depthCut("<table>"+strings.Repeat("<tr><td>a<td>b<td>c</tr>", 170)+"</table>", 480, true); cut {
			t.Fatal("shallow table cut")
		}
		if _, cut := depthCut(strings.Repeat("<table><caption>Agenda<tr><td>x</td></tr></table>", 600), 480, false); cut {
			t.Fatal("captions cut")
		}
	})
}

// stableInView is the backstop against markup Chromium re-parses
// differently; test it directly so it cannot be removed silently.
func TestStableInView(t *testing.T) {
	for _, ok := range []string{`<p>hi</p>`, `<table><tbody><tr><td>x</td></tr></tbody></table>`, `<style>.owa-body p{color:red}</style><p>x</p>`, `<svg><rect></rect></svg>`} {
		if !stableInView(ok) {
			t.Errorf("legit markup rejected: %s", ok)
		}
	}
	for _, bad := range []string{
		`<svg><span>x</span><html style="color:red"></html></svg>`,
		`<li><div><li></li></div></li>`,
		`</div></div></div><div>out</div>`,
		`<table><span>x</span></table>`,
		`<body onload="x">y`,
	} {
		if stableInView(bad) {
			t.Errorf("unstable markup accepted: %s", bad)
		}
	}
}

func TestTokenEvidence(t *testing.T) {
	s := newSanitizer("")
	s.tokenEvidence(`<a href="https://evil.example/pay">x</a><img src="http://t.example/p.gif"><img srcset="https://s.example/a.png 1x">`)
	if len(s.links) != 1 || s.links[0].Host != "evil.example" || len(s.refs) != 2 {
		t.Fatalf("links %v refs %v", s.links, s.refs)
	}
}

// Round-5 review cases.
func TestSanitizeRound5(t *testing.T) {
	san := func(t *testing.T, in string) SanitizeResult {
		t.Helper()
		res, err := OWASanitize(in, "")
		if err != nil {
			t.Fatal(err)
		}
		return res
	}
	t.Run("phantom body in foreign html dropped", func(t *testing.T) {
		res := san(t, `<object><svg><html><foreignObject><table></table><head></head></foreignObject></html></svg><table></table></object><li><div><div><div><svg width="0" height="0"><html><foreignObject><table></table></foreignObject></html></svg><body style="margin-top:-386px"><li>A</li></body></div></div></div></li>`)
		if strings.Contains(res.HTML, "<body") || strings.Contains(res.HTML, "<html") {
			t.Fatalf("phantom elements kept: %s", res.HTML)
		}
	})
	t.Run("foreign void-named element with children", func(t *testing.T) {
		res := san(t, `<p>keep</p><svg><track>x</track></svg>`)
		if !strings.Contains(res.HTML, "keep") {
			t.Fatalf("%s / %v", res.HTML, res.Dropped)
		}
	})
	t.Run("pixel bomb inline image rejected", func(t *testing.T) {
		var buf bytes.Buffer
		img := image.NewGray(image.Rect(0, 0, 9000, 9000))
		if err := png.Encode(&buf, img); err != nil {
			t.Fatal(err)
		}
		res := san(t, `<img src="data:image/png;base64,`+base64.StdEncoding.EncodeToString(buf.Bytes())+`" width=200 height=200>`)
		if strings.Contains(res.HTML, "data:image/png") || !contains(res.Dropped, "data_image_too_many_pixels") {
			t.Fatalf("dropped %v", res.Dropped)
		}
	})
	t.Run("quadratic text merging bounded", func(t *testing.T) {
		start := time.Now()
		san(t, strings.Repeat("a</x>", 400000)+strings.Repeat("<table><td>", 300))
		if d := time.Since(start); d > 3*raceSlowdown*time.Second {
			t.Fatalf("took %v", d)
		}
	})
}

// Ids get the x_ prefix, so in-document references must follow it.
func TestFragmentRefsFollowIDPrefix(t *testing.T) {
	res, err := OWASanitize(`<svg><linearGradient id="grad"></linearGradient><rect fill="url(#grad)" style="mask:url('#m')"></rect><a href="#top">t</a></svg><p><a href="#sec">jump</a></p>`, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`id="x_grad"`, `fill="url(#x_grad)"`, `mask:url(&#39;#x_m&#39;)`, `href="#x_top"`, `href="#x_sec"`} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("missing %s in %s", want, res.HTML)
		}
	}
}

func TestParseFailureKeepsTokenEvidence(t *testing.T) {
	orig := parseFragment
	parseFragment = func(io.Reader, *html.Node) ([]*html.Node, error) {
		return nil, errors.New("injected")
	}
	defer func() { parseFragment = orig }()
	res, err := OWASanitize(`<p><a href="https://evil.example/pay">Pay</a><img src="https://t.example/p.gif"></p>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Dropped, "parse_failed") || len(res.Links) != 1 || len(res.RemoteRefs) != 1 || !strings.Contains(res.HTML, "could not be rendered") {
		t.Fatalf("%s / %v / %v / %v", res.HTML, res.Dropped, res.Links, res.RemoteRefs)
	}
}

func TestDepthCutForeignRules(t *testing.T) {
	// A self-closing <svg/> is closed: later <abbr/> (not a breakout) open
	// HTML elements and must be counted.
	res, err := OWASanitize(`<svg width="0" height="0"/>`+strings.Repeat("<abbr/>", 520), "")
	if err != nil {
		t.Fatal(err)
	}
	if contains(res.Dropped, "parse_failed") || !contains(res.Dropped, "depth_limit") {
		t.Fatalf("dropped %v", res.Dropped)
	}
	// svg/math elements with table names are dropped (x/net/html resets its
	// insertion mode on them regardless of namespace).
	res, _ = OWASanitize(`<svg><td>x</td><select></select></svg>`, "")
	if !contains(res.Dropped, "element:foreign_td") || !contains(res.Dropped, "element:foreign_select") {
		t.Fatalf("dropped %v", res.Dropped)
	}
}

// Round-5 lens: GIF frames larger than the logical screen, filter blur,
// stack-scan cost.
func TestGIFFrameSizeCharged(t *testing.T) {
	// Logical screen 1x1, one 8000x8000 frame (8000x8000 > 16M pixels).
	var b bytes.Buffer
	b.WriteString("GIF89a")
	b.Write([]byte{1, 0, 1, 0, 0, 0, 0}) // screen 1x1, no global table
	b.Write([]byte{0x2C, 0, 0, 0, 0, 0x40, 0x1F, 0x40, 0x1F, 0x80})
	b.Write([]byte{0, 0, 0, 255, 255, 255}) // 2-entry local table
	b.Write([]byte{2, 2, 0x4C, 0x01, 0})    // LZW min size 2, one tiny sub-block
	b.WriteByte(0x3B)
	w, h, ok := imageSize(b.Bytes())
	if !ok || int64(w)*int64(h) < 8000*8000 {
		t.Fatalf("imageSize = %d x %d (%v), want the 8000x8000 frame", w, h, ok)
	}
}

func TestFilterDropped(t *testing.T) {
	res, err := OWASanitize(`<style>.a{filter:blur(20px);color:red}</style><div class="a" style="backdrop-filter:blur(9px);-webkit-filter:blur(3px)">x</div>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "blur") || !contains(res.Dropped, "css:filter") {
		t.Fatalf("%s / %v", res.HTML, res.Dropped)
	}
}

func TestStackScanCostBounded(t *testing.T) {
	in := strings.Repeat("<div>", 470) + strings.Repeat("<hr>", 500000)
	start := time.Now()
	if _, err := OWASanitize(in, ""); err != nil {
		t.Fatal(err)
	}
	if d := time.Since(start); d > 5*raceSlowdown*time.Second { // unbounded: ~11 s
		t.Fatalf("took %v", d)
	}
}

// Round-5 prosecution cases.
func TestSanitizeRound5Prosecution(t *testing.T) {
	timed := func(t *testing.T, in string) SanitizeResult {
		t.Helper()
		start := time.Now()
		res, err := OWASanitize(in, "")
		if err != nil {
			t.Fatal(err)
		}
		if d := time.Since(start); d > 3*raceSlowdown*time.Second {
			t.Fatalf("took %v", d)
		}
		return res
	}
	t.Run("marker end tags", func(t *testing.T) {
		timed(t, strings.Repeat("<td>", 100000)+strings.Repeat("</th>", 20000))
	})
	t.Run("foster-parented text merges", func(t *testing.T) {
		timed(t, "<table>"+strings.Repeat("xxxx<tr>", 100000))
		timed(t, "<table>"+strings.Repeat("xxxx<!---->", 70000))
	})
	t.Run("attribute-heavy clones", func(t *testing.T) {
		var a strings.Builder
		for k := 0; k < 400; k++ {
			a.WriteString(" a" + strconv.Itoa(k))
		}
		in := "<p><b" + a.String() + "><i" + a.String() + "><u" + a.String() + "><s" + a.String() + "></p>" + strings.Repeat("<p>x</p>", 20000)
		res := timed(t, in)
		if !contains(res.Dropped, "input_truncated") {
			t.Fatalf("dropped %v", res.Dropped)
		}
	})
	t.Run("svg filters dropped", func(t *testing.T) {
		res := timed(t, `<svg><defs><filter id=f><feMorphology operator=dilate radius=400 /></filter></defs><g filter="url(#f)"><rect width=10 height=10 /></g></svg>`)
		if strings.Contains(strings.ToLower(res.HTML), "filter") || strings.Contains(strings.ToLower(res.HTML), "femorphology") {
			t.Fatalf("%s", res.HTML)
		}
	})
	t.Run("unclosed inline in cells and items keeps the tail", func(t *testing.T) {
		var b strings.Builder
		b.WriteString("<table>")
		for r := 0; r < 130; r++ {
			b.WriteString("<tr>")
			for c := 0; c < 4; c++ {
				b.WriteString("<td><font face=Arial>r" + strconv.Itoa(r) + "</td>")
			}
			b.WriteString("</tr>")
		}
		b.WriteString(`</table><p><a href="https://after.example.com/doc">doc</a></p>`)
		res := timed(t, b.String())
		if len(res.Links) != 1 {
			t.Fatalf("tail lost: dropped %v", res.Dropped)
		}
		res = timed(t, "<ul>"+strings.Repeat("<li><span style=color:#333>Item</li>", 600)+`</ul><p><a href="https://after.example.com/doc">doc</a></p>`)
		if len(res.Links) != 1 {
			t.Fatalf("list tail lost: dropped %v", res.Dropped)
		}
	})
}

// Token evidence also reads CSS url() references in style attributes and
// <style> text.
func TestTokenEvidenceCSS(t *testing.T) {
	s := newSanitizer("")
	s.tokenEvidence(`<div style="background:url(https://bg.example/a.png)">x</div><style>p{background:url('https://sheet.example/b.png')}</style>`)
	hosts := map[string]bool{}
	for _, r := range s.refs {
		hosts[r.Host] = true
	}
	if !hosts["bg.example"] || !hosts["sheet.example"] {
		t.Fatalf("refs %+v", s.refs)
	}
}

// Round-6: unclosed url( in style attributes, tail evidence after a cut,
// </br> merging.
func TestSanitizeRound6(t *testing.T) {
	res, err := OWASanitize(`<div style="background-image:url(http://evil.test/painted.png">X</div><div style="cursor:url('http://evil.test/c.png'">Y</div>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "evil.test") || !contains(res.Dropped, "css:url_unclosed") {
		t.Fatalf("%s / %v", res.HTML, res.Dropped)
	}
	res, _ = OWASanitize("<p>Agenda</p>"+strings.Repeat("<div>", 490)+strings.Repeat("</div>", 490)+
		`<p><a href="https://login.evil.example/owa">Review</a><img src="https://track.evil.example/p.gif"></p>`, "")
	if len(res.Links) != 1 || len(res.RemoteRefs) != 1 {
		t.Fatalf("tail evidence lost: links %v refs %v dropped %v", res.Links, res.RemoteRefs, res.Dropped)
	}
	var b strings.Builder
	b.WriteString("<p>Digest</p>")
	for i := 0; i < 2000; i++ {
		b.WriteString("Ticket " + strconv.Itoa(i) + " updated by the service desk team</br>")
	}
	b.WriteString(`<p><a href="https://help.example/desk">desk</a></p>`)
	res, _ = OWASanitize(b.String(), "")
	if contains(res.Dropped, "input_truncated") || !strings.Contains(res.HTML, "help.example") {
		t.Fatalf("legit </br> digest truncated: %v", res.Dropped)
	}
}

// Aliases of one image (extra media parameters) are rewritten to one
// canonical data: URL, so Chromium decodes it once and the budget holds.
func TestInlineDataCanonical(t *testing.T) {
	res, err := OWASanitize(`<img src="data:image/png;a=1;base64,`+onePxPNG+`"><img src="data:image/png;b=2;base64,`+onePxPNG+`">`, "")
	if err != nil {
		t.Fatal(err)
	}
	want := `src="data:image/png;base64,` + onePxPNG + `"`
	if strings.Count(res.HTML, want) != 2 || strings.Contains(res.HTML, "a=1") {
		t.Fatalf("%s", res.HTML)
	}
}

func TestVisibleHTMLIgnoresStyle(t *testing.T) {
	if visibleHTML(`<style>.owa-body [class=owa-remote-ref]{color:red}</style>`) {
		t.Fatal("style-only fragment counted as visible")
	}
}

func TestInlineDataCanonicalMIME(t *testing.T) {
	res, err := OWASanitize(`<img src="data:image/gif;base64,`+onePxPNG+`"><img src="data:image/png;base64,`+onePxPNG+`">`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(res.HTML, `src="data:image/png;base64,`+onePxPNG+`"`) != 2 {
		t.Fatalf("%s", res.HTML)
	}
}

func TestEvidenceClippedAndTokenDataRecorded(t *testing.T) {
	s := newSanitizer("")
	s.tokenEvidence(`<a href="https://e.example/` + strings.Repeat("a", 20000) + `">x</a><img src="data:image/png;base64,AAAA">`)
	if len(s.links) != 1 || len(s.links[0].Href) > maxEvidenceValue+8 {
		t.Fatalf("link not clipped: %d", len(s.links[0].Href))
	}
	found := false
	for _, r := range s.refs {
		found = found || r.Kind == "inline_data"
	}
	if !found {
		t.Fatalf("inline data image not recorded: %+v", s.refs)
	}
}

func TestImageSetDeclarationRecorded(t *testing.T) {
	res, err := OWASanitize(`<div style="background-image:image-set('https://set.example/a.png' 1x, &quot;https://set.example/b.png&quot; 2x)">x</div>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.RemoteRefs) != 2 || strings.Contains(res.HTML, "set.example") {
		t.Fatalf("refs %+v html %s", res.RemoteRefs, res.HTML)
	}
}

func TestDroppedBounded(t *testing.T) {
	var b strings.Builder
	b.WriteString("<p")
	for i := 0; i < 1000; i++ {
		b.WriteString(" on" + strconv.Itoa(i) + "=1")
	}
	b.WriteString(" on" + strings.Repeat("Z", 5000) + "=1>x</p>")
	res, err := OWASanitize(b.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Dropped) > maxDropped+1 || !contains(res.Dropped, "dropped_capped") {
		t.Fatalf("%d dropped codes", len(res.Dropped))
	}
	for _, d := range res.Dropped {
		if len(d) > maxDropLength+4 {
			t.Fatalf("code of %d bytes", len(d))
		}
	}
}

// Attacker-named drop codes cannot crowd out the fixed vocabulary.
func TestDropCapKeepsFixedCodes(t *testing.T) {
	var b strings.Builder
	b.WriteString("<p")
	for i := 0; i < 250; i++ {
		b.WriteString(" on" + strconv.Itoa(i) + "=1")
	}
	b.WriteString(">x</p>")
	for i := 0; i < maxRecorded+200; i++ {
		b.WriteString(`<a href="https://e` + strconv.Itoa(i) + `.example/">l</a>`)
	}
	res, err := OWASanitize(b.String(), "")
	if err != nil {
		t.Fatal(err)
	}
	if !contains(res.Dropped, "links_capped") || !contains(res.Dropped, "dropped_capped") {
		t.Fatalf("dropped tail %v", res.Dropped[len(res.Dropped)-3:])
	}
}

// The generated host label is a bidi isolate: an unterminated override in
// the attacker's link text cannot reverse it.
func TestHostLabelIsolated(t *testing.T) {
	res, err := OWASanitize("<p><a href=\"https://moc.tfosorcim.ai/x\">login ‮</a> more</p>", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, `<bdi class="owa-host">(moc.tfosorcim.ai)</bdi>`) {
		t.Fatalf("%s", res.HTML)
	}
}

// Placeholders and labels show hosts as the browser fetches them, without
// bidi controls; 'https:host' URLs (WHATWG: https://host) get their host.
func TestPlaceholderHostsNeutralized(t *testing.T) {
	res, err := OWASanitize("<img src=\"https://teams.micrоsoft.com/a.png\"> <img src=\"https://‮moc.tfosorcim.nigol‬/b.png\"> <a href=\"https:evil.example/login\">x</a>", "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.HTML, "remote image: teams.xn--micrsoft-") || strings.Contains(res.HTML, "‮") {
		t.Fatalf("%s", res.HTML)
	}
	if len(res.Links) != 1 || res.Links[0].Host != "evil.example" {
		t.Fatalf("links %+v", res.Links)
	}
}

func TestSanitizeFinalRound(t *testing.T) {
	// An SVG anchor cannot carry its host label, so it is inert (recorded).
	res, err := OWASanitize(`<svg><a href="https://evil.example/x"><text>Sign in</text></a></svg>`, "")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(res.HTML, "evil.example") || len(res.Links) != 1 {
		t.Fatalf("%s / %+v", res.HTML, res.Links)
	}
	// Ports do not defeat the ASCII host label.
	if got := displayHost("teams.micrоsoft.com:8443"); !strings.HasPrefix(got, "teams.xn--") || !strings.HasSuffix(got, ":8443") {
		t.Fatalf("displayHost = %q", got)
	}
	// Token evidence reads quoted @import and image-set strings.
	s := newSanitizer("")
	s.tokenEvidence(`<style>@import "https://imp.example/a.css"; p{background:image-set("https://set.example/b.png" 1x)}</style>`)
	hosts := map[string]bool{}
	for _, r := range s.refs {
		hosts[r.Host] = true
	}
	if !hosts["imp.example"] || !hosts["set.example"] {
		t.Fatalf("refs %+v", s.refs)
	}
}

// Attribute selectors on class/id follow the x_ prefix of the attributes.
func TestAttributeSelectorsPrefixed(t *testing.T) {
	res, err := OWASanitize(`<style>[class~="mobile"]{display:block!important} [id="message"]{color:red} [class^=hd]{color:blue} [class="a b"]{color:green} span[class=owa-host]{display:none}</style><div class="mobile" style="display:none">x</div>`, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`[class~="x_mobile"]`, `[id="x_message"]`, `[class^=x_hd]`, `[class="x_a x_b"]`, `span[class=x_owa-host]`} {
		if !strings.Contains(res.HTML, want) {
			t.Errorf("missing %s in %s", want, res.HTML)
		}
	}
}
