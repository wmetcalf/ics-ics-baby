package render

import (
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func requireChromium(t *testing.T) {
	t.Helper()
	if _, err := FindChromium(""); err != nil {
		t.Skipf("headless Chromium not available: %v", err)
	}
}

// hitServer counts every request it receives.
func hitServer(t *testing.T) (*httptest.Server, *atomic.Int64) {
	t.Helper()
	var hits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(srv.Close)
	return srv, &hits
}

func writeHTML(t *testing.T, body string) (dir, htmlPath string) {
	t.Helper()
	dir = t.TempDir()
	htmlPath = filepath.Join(dir, "page.html")
	doc := "<!doctype html><html><head><meta charset=utf-8></head><body>" + body + "</body></html>"
	if err := os.WriteFile(htmlPath, []byte(doc), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir, htmlPath
}

func decodePNGSize(t *testing.T, path string) (int, int) {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open png: %v", err)
	}
	defer f.Close()
	img, err := png.Decode(f)
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}
	b := img.Bounds()
	return b.Dx(), b.Dy()
}

func containsSuffix(list []string, suffix string) bool {
	for _, s := range list {
		if strings.HasSuffix(s, suffix) {
			return true
		}
	}
	return false
}

func TestChromiumRendersPNG(t *testing.T) {
	requireChromium(t)
	dir, htmlPath := writeHTML(t, "<h1>hello</h1><p>world</p>")
	pngPath := filepath.Join(dir, "out.png")
	if _, err := RenderHTMLWithChromium(htmlPath, pngPath, 640, ChromiumSettings{}); err != nil {
		t.Fatalf("render: %v", err)
	}
	w, h := decodePNGSize(t, pngPath)
	if w != 640 || h <= 0 {
		t.Fatalf("png size = %dx%d, want width 640", w, h)
	}
}

func TestChromiumBlocksNetwork(t *testing.T) {
	requireChromium(t)
	srv, hits := hitServer(t)
	dir, htmlPath := writeHTML(t,
		`<img src="`+srv.URL+`/i">`+
			`<link rel=stylesheet href="`+srv.URL+`/c">`+
			`<div style="width:50px;height:50px;background:url(`+srv.URL+`/b)"></div>`+
			`<iframe src="`+srv.URL+`/f"></iframe>`)
	pngPath := filepath.Join(dir, "out.png")
	blocked, err := RenderHTMLWithChromium(htmlPath, pngPath, 800, ChromiumSettings{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server received %d requests; render must not egress", n)
	}
	if !containsSuffix(blocked, "/i") {
		t.Fatalf("blocked = %v, want the /i image URL", blocked)
	}
}

func TestChromiumBlocksSiblingFiles(t *testing.T) {
	requireChromium(t)
	dir, htmlPath := writeHTML(t, `<img src="secret.png">`)
	if err := os.WriteFile(filepath.Join(dir, "secret.png"), []byte("not really a png"), 0o644); err != nil {
		t.Fatal(err)
	}
	blocked, err := RenderHTMLWithChromium(htmlPath, filepath.Join(dir, "out.png"), 800, ChromiumSettings{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if !containsSuffix(blocked, "/secret.png") {
		t.Fatalf("blocked = %v, want sibling secret.png", blocked)
	}
}

func TestChromiumNoJS(t *testing.T) {
	requireChromium(t)
	srv, hits := hitServer(t)
	dir, htmlPath := writeHTML(t, `<p>x</p><script>new Image().src="`+srv.URL+`/js"</script>`)
	blocked, err := RenderHTMLWithChromium(htmlPath, filepath.Join(dir, "out.png"), 800, ChromiumSettings{})
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	if n := hits.Load(); n != 0 {
		t.Fatalf("server received %d requests", n)
	}
	if containsSuffix(blocked, "/js") {
		t.Fatalf("script executed: /js was requested (blocked=%v)", blocked)
	}
}

func TestChromiumMaxHeight(t *testing.T) {
	requireChromium(t)
	dir, htmlPath := writeHTML(t, `<div style="height:100000px;background:linear-gradient(red,blue)">tall</div>`)
	pngPath := filepath.Join(dir, "out.png")
	timeout := 30 * time.Second
	start := time.Now()
	if _, err := RenderHTMLWithChromium(htmlPath, pngPath, 600, ChromiumSettings{MaxHeight: 3000, Timeout: timeout}); err != nil {
		t.Fatalf("render: %v", err)
	}
	if el := time.Since(start); el >= timeout {
		t.Fatalf("render took %v, not under timeout %v", el, timeout)
	}
	_, h := decodePNGSize(t, pngPath)
	if h > 3000 {
		t.Fatalf("png height %d > MaxHeight 3000", h)
	}
}

func TestChromiumMissingBinary(t *testing.T) {
	dir, htmlPath := writeHTML(t, "<p>x</p>")
	_, err := RenderHTMLWithChromium(htmlPath, filepath.Join(dir, "out.png"), 800, ChromiumSettings{Bin: "/nonexistent"})
	if err == nil || !strings.Contains(err.Error(), "/nonexistent") {
		t.Fatalf("err = %v, want mention of /nonexistent", err)
	}
	if _, err := FindChromium("/nonexistent"); err == nil {
		t.Fatal("FindChromium(/nonexistent) succeeded")
	}
}
