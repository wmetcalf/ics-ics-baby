package clientview

import (
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"ics-ics-baby/internal/render"
)

// Review findings 2 and 3: body CSS must not paint outside the body
// container (over the subject, organizer, time and RSVP chrome), whether by
// restyling the container, negative offsets, or position:var(--fixed).
func TestOWABodyCannotPaintOverChrome(t *testing.T) {
	if _, err := render.FindChromium(""); err != nil {
		t.Skipf("headless Chromium not available: %v", err)
	}
	body := `<style>body{overflow:visible!important;z-index:9;margin-top:-600px!important;background:#ff0000} .o{--p:fixed;position:var(--p);top:0;left:0;width:3000px;height:400px;background:#ff0000}</style>` +
		`<div style="position:absolute;top:-600px;left:0;width:3000px;height:600px;background:#ff0000">X</div><div class="o">Y</div><p>hello</p>`
	assertHeaderClean(t, body)
}

// A CSS string ends at a raw newline (a bad-string token), so quotes the
// sanitizer reads as one string can let Chromium see the rest as top-level,
// unscoped rules. Selector, declaration and @media prelude variants.
func TestOWABodyBadStringCannotEscapeScope(t *testing.T) {
	if _, err := render.FindChromium(""); err != nil {
		t.Skipf("headless Chromium not available: %v", err)
	}
	for name, css := range map[string]string{
		"selector":    "p[title=\"x\n]{}.owa-head, .owa-pane{background:#ff0000!important}\np[title=\"]{color:blue}",
		"declaration": "p{content:\"x\n}.owa-head, .owa-pane{background:#ff0000!important}\np{content:\"}",
		"media":       "@media all and (min-width:\"1\n){}.owa-head, .owa-pane{background:#ff0000!important}\n@media \"){p{color:blue}}",
		// Tokenizer differentials found by review: a quote inside a bad
		// url(), comments re-formed by a single strip pass, a simple block
		// that swallows '}', and <style> in foreign content.
		"bad url":        "a url(x\"y) {}.owa-head, .owa-pane{background:#ff0000!important} p \"){color:blue}",
		"comment rejoin": "a//**/*[*/**//, .owa-head, .owa-pane{background:#ff0000!important}",
		"paren block":    "x{a:(}p)}div{background:#ff0000!important}",
		"bracket block":  "x{a:[}p]}div{background:#ff0000!important}",
	} {
		t.Run(name, func(t *testing.T) {
			assertHeaderClean(t, "<style>"+css+"</style><p>hello</p>")
		})
	}
}

// <style> under MathML text integration points or after an HTML
// placeholder inside <svg> is serialized escaped but parsed as raw text.
func TestOWAForeignStyleCannotEscapeScope(t *testing.T) {
	if _, err := render.FindChromium(""); err != nil {
		t.Skipf("headless Chromium not available: %v", err)
	}
	css := "a \"}{}.owa-head, .owa-pane{background:#ff0000!important}\"{color:blue}"
	for name, body := range map[string]string{
		"mtext":           "<math><mtext><style>" + css + "</style></mtext></math><p>hello</p>",
		"svg placeholder": "<svg><image href=\"http://evil.test/x.png\"/><style>" + css + "</style></svg><p>hello</p>",
	} {
		t.Run(name, func(t *testing.T) { assertHeaderClean(t, body) })
	}
}

// Markup the parser would never build (HTML placeholders inside <svg> or
// <math>, an unwrapped <form>) is re-parsed differently by Chromium, which
// can then merge attributes onto <html> or close the body's wrapper divs.
func TestOWAReparseCannotEscapeBody(t *testing.T) {
	if _, err := render.FindChromium(""); err != nil {
		t.Skipf("headless Chromium not available: %v", err)
	}
	red := `<div style="position:absolute;top:0;left:0;width:3000px;height:300px;background:#ff0000">X</div>`
	for name, body := range map[string]string{
		"html attrs":         `<svg><image href="http://evil.test/x.png"></image><html style="outline:300px solid #ff0000;outline-offset:-300px"></html></svg><p>hello</p>`,
		"mi breakout":        `<li><div><math><image href="cid:x"></image><mi><li></li></mi></math></div></li></div></div></div>` + red,
		"phantom body":       `<object><svg><html><foreignObject><table></table><head></head></foreignObject></html></svg><table></table></object><li><div><div><div><svg width="0" height="0"><html><foreignObject><table></table></foreignObject></html></svg><body><li>A</li></body></div></div></div></li>` + red,
		"phantom body attrs": `<div style="height:200px;background:#ff0000"></div><object><svg><html><foreignObject><table></table><head></head></foreignObject></html></svg><table></table></object><svg width="0" height="0"><html><foreignObject><table></table></foreignObject></html></svg><body style="margin-top:-386px">x</body>`,
		"form unwrap":        `<li><div><form><li></li></form></div></li><dd><div><form><dd></dd></form></div></dd><dt><div><form><dt></dt></form></div></dt></div></div></div>` + red,
	} {
		t.Run(name, func(t *testing.T) { assertHeaderClean(t, body) })
	}
}

func assertHeaderClean(t *testing.T, body string) {
	t.Helper()
	esc := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`).Replace(body)
	cal := parseICS(t, invite("REQUEST", "X-ALT-DESC;FMTTYPE=text/html:"+esc))
	dir := t.TempDir()
	if _, err := WriteOWA(cal, "x", dir); err != nil {
		t.Fatal(err)
	}
	pngPath := filepath.Join(dir, "owa.png")
	if _, err := render.RenderHTMLWithChromium(filepath.Join(dir, OWAHTMLName), pngPath, 900, render.ChromiumSettings{}); err != nil {
		t.Fatal(err)
	}
	f, err := os.Open(pngPath)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatal(err)
	}
	// The header (subject/organizer/time/RSVP) occupies the top ~220px.
	for y := 0; y < 220; y++ {
		for x := 0; x < img.Bounds().Dx(); x += 3 {
			r, g, b, _ := img.At(x, y).RGBA()
			if r>>8 > 240 && g>>8 < 30 && b>>8 < 30 {
				t.Fatalf("attacker red painted over the header at (%d,%d)", x, y)
			}
		}
	}
}
