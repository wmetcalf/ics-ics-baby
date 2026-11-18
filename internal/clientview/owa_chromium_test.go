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
	} {
		t.Run(name, func(t *testing.T) {
			assertHeaderClean(t, "<style>"+css+"</style><p>hello</p>")
		})
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
