package clientview

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"regexp"
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
		if d := time.Since(start); d > 2*time.Second {
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
		if d := time.Since(start); d > 2*time.Second {
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
