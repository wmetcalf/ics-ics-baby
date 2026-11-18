package clientview

// Round-4 review checks: each was red before its fix.

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"ics-ics-baby/internal/icsparse"
)

func rcNodes(in string) int {
	ctx := &html.Node{Type: html.ElementNode, Data: "div", DataAtom: atom.Div}
	nodes, err := html.ParseFragment(strings.NewReader(in), ctx)
	if err != nil {
		return -1
	}
	var count func(n *html.Node) int
	count = func(n *html.Node) int {
		c := 1
		for k := n.FirstChild; k != nil; k = k.NextSibling {
			c += count(k)
		}
		return c
	}
	total := 0
	for _, n := range nodes {
		total += count(n)
	}
	return total
}

// 400 distinct <b>s left active (popped by </div>), then cycles that make
// the parser re-create all of them: 400 nodes per 12 input bytes.
func rcBlowup(prefix, mid string, cycles int) string {
	var b strings.Builder
	b.WriteString(prefix + "<div>")
	for i := 0; i < 400; i++ {
		fmt.Fprintf(&b, "<b id=%d>", i)
	}
	b.WriteString("</div>" + mid)
	b.WriteString(strings.Repeat("<div>x</div>", cycles))
	return b.String()
}

func rcCutNodes(in string) (bool, int) {
	cut, ok := depthCut(in, maxSanitizeDepth-depthHeadroom, false)
	if ok {
		in = in[:cut]
	}
	return ok, rcNodes(in)
}

// F1: <td> outside a table (ignored by the parser) clears depthCut's
// active list, so maxRebuiltNodes no longer bounds the re-created nodes.
func TestR4_TdOutsideTableBypassesRebuiltBound(t *testing.T) {
	_, plain := rcCutNodes(rcBlowup("", "", 3000))
	cut, trick := rcCutNodes(rcBlowup("<td>", "<td>", 3000))
	if trick > maxRebuiltNodes {
		t.Errorf("td trick: cut=%v, parser builds %d nodes from %d bytes (plain variant bounded at %d)", cut, trick, len(rcBlowup("<td>", "<td>", 3000)), plain)
	}
}

// F2: depthCut reads <style/> (self-closing) or <style> under a name-only
// "integration point" as raw text; the parser reads markup.
func TestR4_RawTextDesyncBypassesBounds(t *testing.T) {
	for _, p := range []string{"<svg><style/>", "<svg><g><title/>", "<svg><font><style>", "<math><title><style>", "<math><annotation-xml><style>", "<svg><mi><style>"} {
		in := rcBlowup(p, "", 2000)
		cut, n := rcCutNodes(in)
		if n > maxRebuiltNodes/2 {
			t.Errorf("%s: cut=%v, parser builds %d nodes from %d bytes", p, cut, n, len(in))
		}
	}
	res, _ := OWASanitize(`<svg><style/><a href="https://evil.example/pay">Pay</a>`+strings.Repeat("<div>", 600), "")
	if len(res.Links) == 0 {
		t.Errorf("parse-failed body lost its link evidence: dropped=%v links=%v", res.Dropped, res.Links)
	}
}

// F3: an SVG <font> without color/face/size stays foreign, but depthCut
// pops <svg> on it and counts every later self-closing <glyph/>.
func TestR4_SvgFontGlyphsTruncateBody(t *testing.T) {
	in := `<p>Header</p><svg width="0" height="0"><defs><font id="icons" horiz-adv-x="512">` +
		strings.Repeat(`<glyph unicode="&#xe600;" d="M0 0h512v512z"/>`, 600) + `</font></defs></svg><p>TAIL-AFTER-ICONS</p>`
	res, _ := OWASanitize(in, "")
	if !strings.Contains(res.HTML, "TAIL-AFTER-ICONS") {
		t.Errorf("legit SVG icon font truncated the body: dropped=%v", res.Dropped)
	}
}

// F4: text merging across start tags the parser ignores is not budgeted.
func TestR4_MergeBudgetBypassedByIgnoredStartTags(t *testing.T) {
	for _, sep := range []string{"<tr>", "<col>"} {
		in := strings.Repeat("a"+sep, (2<<20)/(1+len(sep)))
		start := time.Now()
		OWASanitize(in, "")
		if d := time.Since(start); d > 3*raceSlowdown*time.Second {
			t.Errorf("%q x %d: sanitize took %v", "a"+sep, len(in)/(1+len(sep)), d)
		}
	}
}

func rcPNG(t *testing.T, w, h int, seed byte) string {
	img := image.NewGray(image.Rect(0, 0, w, h))
	img.Pix[0] = seed
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(buf.Bytes())
}

// F5: each kept data: <img> is charged twice (image() and attrs()).
func TestR4_PixelBudgetChargedTwice(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ics-ics-baby-inline")
	var b strings.Builder
	for i := 0; i < 4; i++ { // 4 x 9 MP = 36 MP, under the 64 MP body cap
		fmt.Fprintf(&b, `<p>photo %d</p><img alt="photo %d" src="data:image/png;base64,%s" width=300 height=300>`, i, i, rcPNG(t, 3000, 3000, byte(i+1)))
	}
	res, _ := OWASanitize(b.String(), dir)
	kept := strings.Count(res.HTML, `src="data:image/png`)
	if kept != 4 {
		tail := res.HTML[strings.LastIndex(res.HTML, "<p>"):]
		t.Errorf("kept %d of 4 images (36 MP < 64 MP cap); placeholders=%d dropped=%v inline refs=%d; html tail=%q",
			kept, strings.Count(res.HTML, "owa-remote-ref"), res.Dropped, len(res.RemoteRefs), tail)
	}
}

// F6: valid 16-bpp BMP (Chromium renders it) is dropped as malformed.
func TestR4_ValidBMPDroppedAsMalformed(t *testing.T) {
	w, h := 40, 20
	row := bytes.Repeat([]byte{0x00, 0x7C}, w) // X1R5G5B5 red
	pix := bytes.Repeat(row, h)
	var bmp bytes.Buffer
	bmp.WriteString("BM")
	binary.Write(&bmp, binary.LittleEndian, []uint32{uint32(54 + len(pix)), 0, 54, 40})
	binary.Write(&bmp, binary.LittleEndian, []int32{int32(w), int32(h)})
	binary.Write(&bmp, binary.LittleEndian, []uint16{1, 16})
	binary.Write(&bmp, binary.LittleEndian, []uint32{0, uint32(len(pix)), 2835, 2835, 0, 0})
	bmp.Write(pix)
	res, _ := OWASanitize(`<img src="data:image/bmp;base64,`+base64.StdEncoding.EncodeToString(bmp.Bytes())+`">`, "")
	if !strings.Contains(res.HTML, "data:image/bmp") {
		t.Errorf("16-bpp BMP dropped: dropped=%v refs=%v", res.Dropped, res.RemoteRefs)
	}
}

func rcWriteOWA(t *testing.T, ics string) (Sidecar, string) {
	p := filepath.Join(t.TempDir(), "in.ics")
	if err := os.WriteFile(p, []byte(ics), 0o644); err != nil {
		t.Fatal(err)
	}
	cal, err := icsparse.ParseICSFile(p, "", 64<<20)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	sc, err := WriteOWA(cal, "x", dir)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, OWAHTMLName))
	return sc, string(b)
}

func rcICS(props ...string) string {
	return "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nMETHOD:REQUEST\r\nBEGIN:VEVENT\r\nUID:p@test\r\nSUMMARY:x\r\n" + strings.Join(props, "\r\n") + "\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
}

// F7: an earlier, unrendered part consumes the shared inline-file cap.
func TestR4_RenderedPartLosesInlineFirstClaim(t *testing.T) {
	var hidden strings.Builder
	for i := 0; i < maxInlineImages; i++ {
		hidden.WriteString(`<img src="data:image/png\;base64\,` + base64.StdEncoding.EncodeToString(tinyPNG(t, i+1)) + `">`)
	}
	sc, h := rcWriteOWA(t, rcICS("X-ALT-DESC;FMTTYPE=text/html:"+hidden.String(),
		`X-ALT-DESC;FMTTYPE=text/html:<p>Shown body</p><img src="data:image/png\;base64\,`+onePxPNG+`">`))
	i := strings.Index(h, "Shown body")
	if i < 0 || !strings.Contains(h[i:], `src="data:image/png`) {
		t.Errorf("rendered part's inline image dropped: dropped=%v", sc.Dropped)
	}
}

// F8: a non-rendered part's parse_failed is reported for the rendered body.
func TestR4_UnrenderedPartDropCodesMerged(t *testing.T) {
	sc, h := rcWriteOWA(t, rcICS("X-ALT-DESC;FMTTYPE=text/html:<svg><style/>"+strings.Repeat("<div>", 600),
		"X-ALT-DESC;FMTTYPE=text/html:<p>Hello shown body</p>"))
	if strings.Contains(h, "Hello shown body") && contains(sc.Dropped, "parse_failed") {
		t.Errorf("rendered body is fine but sidecar dropped=%v", sc.Dropped)
	}
}

// F9: parts beyond the first 8 are never considered; the preview shows the
// last HTML part.
func TestR4_NinthPartIgnored(t *testing.T) {
	var props []string
	for i := 1; i <= 8; i++ {
		props = append(props, fmt.Sprintf("X-ALT-DESC;FMTTYPE=text/html:<p>decoy part %d</p>", i))
	}
	props = append(props, `X-ALT-DESC;FMTTYPE=text/html:<p>REAL PART <a href="https://evil.example/pay">pay</a></p>`)
	sc, h := rcWriteOWA(t, rcICS(props...))
	if !strings.Contains(h, "REAL PART") || len(sc.Links) == 0 {
		t.Errorf("last HTML part not shown/evidenced: shows decoy=%v links=%v dropped=%v", strings.Contains(h, "decoy part 8"), sc.Links, sc.Dropped)
	}
}
