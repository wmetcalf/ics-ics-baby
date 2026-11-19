package clientview

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/chromedp/chromedp"

	"ics-ics-baby/internal/render"
)

// labelState is what Chromium computes for each generated host label.
type labelState struct {
	Display    string  `json:"display"`
	Visibility string  `json:"visibility"`
	Opacity    string  `json:"opacity"`
	FontSize   string  `json:"fontSize"`
	Color      string  `json:"color"`
	Width      float64 `json:"width"`
	Height     float64 `json:"height"`
	OnTop      bool    `json:"onTop"`
	Text       string  `json:"text"`
}

// owaLabels loads the written OWA page in Chromium (inspection only: the test
// evaluates the computed style of every host label) and returns them.
func owaLabels(t *testing.T, body string) []labelState {
	t.Helper()
	bin, err := render.FindChromium("")
	if err != nil {
		t.Skipf("headless Chromium not available: %v", err)
	}
	esc := strings.NewReplacer(`\`, `\\`, ";", `\;`, ",", `\,`, "\n", `\n`).Replace(body)
	cal := parseICS(t, invite("REQUEST", "X-ALT-DESC;FMTTYPE=text/html:"+esc))
	dir := t.TempDir()
	if _, err := WriteOWA(cal, "x", dir); err != nil {
		t.Fatal(err)
	}
	abs, err := filepath.Abs(filepath.Join(dir, OWAHTMLName))
	if err != nil {
		t.Fatal(err)
	}
	alloc, cancel := chromedp.NewExecAllocator(context.Background(),
		chromedp.ExecPath(bin), chromedp.Headless, chromedp.DisableGPU,
		chromedp.UserDataDir(t.TempDir()), chromedp.WindowSize(900, 1200))
	defer cancel()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	ctx, cancel = context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	const probe = `JSON.stringify([...document.querySelectorAll('.owa-body bdi.owa-host')].map(e => {
		const s = getComputedStyle(e), r = e.getBoundingClientRect();
		const top = document.elementFromPoint(r.left + r.width / 2, r.top + r.height / 2);
		return {display: s.display, visibility: s.visibility, opacity: s.opacity, fontSize: s.fontSize,
			color: s.color, width: r.width, height: r.height, onTop: top === e || e.contains(top),
			text: e.textContent};
	}))`
	if err := chromedp.Do(ctx, chromedp.Navigate("file://"+filepath.ToSlash(abs))); err != nil {
		t.Fatal(err)
	}
	raw, err := chromedp.Run(ctx, chromedp.Evaluate[string](probe))
	if err != nil {
		t.Fatal(err)
	}
	var labels []labelState
	if err := json.Unmarshal([]byte(raw), &labels); err != nil {
		t.Fatal(err)
	}
	return labels
}

// Codex review on 70bad14: body CSS could hide the generated host label
// (the real link target) while the misleading link text stays visible.
// Rules that target the label, or that it inherits from, must not change it.
func TestOWAHostLabelSurvivesBodyCSS(t *testing.T) {
	link := `<p><a href="https://evil.example">https://teams.microsoft.com</a></p><p>after</p>`
	for name, css := range map[string]string{
		"display":       `bdi{display:none}`,
		"important":     `p bdi[class]{display:none!important}`,
		"sibling":       `a + *{visibility:hidden!important}`,
		"opacity":       `bdi{opacity:0!important}`,
		"font size":     `bdi{font-size:0!important}`,
		"inherited":     `p{font-size:0;color:transparent} a{font-size:16px;color:#0000ee}`,
		"color":         `bdi{color:#ffffff!important;background:#ffffff!important}`,
		"collapse":      `bdi{width:0!important;height:0!important;overflow:hidden!important;display:inline-block!important}`,
		"offset":        `bdi{position:absolute!important;left:-9999px!important}`,
		"transform":     `bdi{transform:scale(0)!important}`,
		"clip":          `bdi{clip-path:inset(50%)!important}`,
		"sibling cover": `p:first-child + p{position:relative;margin-top:-1.5em;background:#fff;height:3em}`,
	} {
		t.Run(name, func(t *testing.T) {
			labels := owaLabels(t, "<style>"+css+"</style>"+link)
			if len(labels) != 1 {
				t.Fatalf("labels = %+v", labels)
			}
			l := labels[0]
			if l.Text != "(evil.example)" || l.Display == "none" || l.Visibility != "visible" || l.Opacity != "1" ||
				l.FontSize != "12px" || l.Color != "rgb(97, 97, 97)" || l.Width < 20 || l.Height < 8 || !l.OnTop {
				t.Fatalf("label hidden or restyled: %+v", l)
			}
		})
	}
}
