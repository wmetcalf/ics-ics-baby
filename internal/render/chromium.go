package render

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/chromedp/cdproto/cdp"
	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/fetch"
	"github.com/chromedp/cdproto/network"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// ChromiumSettings configure the headless-Chromium renderer. Zero values mean
// the defaults: auto-detect the binary, 45s timeout, sandbox on, 20000px
// maximum capture height.
type ChromiumSettings struct {
	Bin       string
	Timeout   time.Duration
	NoSandbox bool
	MaxHeight int
}

const (
	defaultChromiumTimeout   = 45 * time.Second
	defaultChromiumMaxHeight = 20000
)

var chromiumCandidates = []string{"chromium", "chromium-browser", "google-chrome", "headless_shell"}

// FindChromium resolves the browser binary. A non-empty bin must be an
// executable (absolute path or a name on PATH); otherwise PATH is searched for
// chromium, chromium-browser, google-chrome and headless_shell, in that order.
func FindChromium(bin string) (string, error) {
	if strings.TrimSpace(bin) != "" {
		p, err := exec.LookPath(bin)
		if err != nil {
			return "", fmt.Errorf("chromium binary %q not usable: %w", bin, err)
		}
		return p, nil
	}
	for _, name := range chromiumCandidates {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("no headless Chromium found on PATH (tried %s); pass --chromium-bin", strings.Join(chromiumCandidates, ", "))
}

// RenderHTMLWithChromium rasterizes htmlPath to pngPath with a throwaway,
// offline headless Chromium. JavaScript is disabled, every request other than
// the HTML file itself (and data: URLs) is failed, and DNS/proxy are dead.
// It returns the URLs it blocked. Blocked requests are not an error.
func RenderHTMLWithChromium(htmlPath, pngPath string, width int, opts ChromiumSettings) (blocked []string, err error) {
	if htmlPath == "" || pngPath == "" {
		return nil, errors.New("html and png paths must be provided")
	}
	absHTML, err := filepath.Abs(htmlPath)
	if err != nil {
		return nil, fmt.Errorf("resolve html path: %w", err)
	}
	if _, err := os.Stat(absHTML); err != nil {
		return nil, fmt.Errorf("html input not found: %w", err)
	}
	bin, err := FindChromium(opts.Bin)
	if err != nil {
		return nil, err
	}
	if width <= 0 {
		width = 1200
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultChromiumTimeout
	}
	maxHeight := opts.MaxHeight
	if maxHeight <= 0 {
		maxHeight = defaultChromiumMaxHeight
	}
	if err := os.MkdirAll(filepath.Dir(pngPath), 0o755); err != nil {
		return nil, fmt.Errorf("create screenshot dir: %w", err)
	}

	profileDir, err := os.MkdirTemp("", "ics-ics-baby-chromium-")
	if err != nil {
		return nil, fmt.Errorf("create chromium profile dir: %w", err)
	}
	defer os.RemoveAll(profileDir)

	pageURL := (&url.URL{Scheme: "file", Path: filepath.ToSlash(absHTML)}).String()

	allocOpts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(bin),
		chromedp.UserDataDir(profileDir),
		chromedp.Flag("headless", "new"),
		chromedp.Flag("hide-scrollbars", true),
		chromedp.Flag("mute-audio", true),
		chromedp.Flag("disable-gpu", true),
		chromedp.Flag("no-first-run", true),
		chromedp.Flag("no-default-browser-check", true),
		chromedp.Flag("disable-extensions", true),
		chromedp.Flag("disable-background-networking", true),
		chromedp.Flag("disable-sync", true),
		chromedp.Flag("disable-default-apps", true),
		chromedp.Flag("disable-component-update", true),
		chromedp.Flag("disable-domain-reliability", true),
		chromedp.Flag("disable-breakpad", true),
		chromedp.Flag("disable-client-side-phishing-detection", true),
		chromedp.Flag("disable-dev-shm-usage", true),
		chromedp.Flag("metrics-recording-only", true),
		chromedp.Flag("safebrowsing-disable-auto-update", true),
		chromedp.Flag("password-store", "basic"),
		chromedp.Flag("use-mock-keychain", true),
		chromedp.Flag("force-color-profile", "srgb"),
		// Network belt and braces: dead proxy (loopback included) and no DNS.
		chromedp.Flag("proxy-server", "127.0.0.1:9"),
		chromedp.Flag("proxy-bypass-list", "<-loopback>"),
		chromedp.Flag("host-resolver-rules", "MAP * ~NOTFOUND"),
		// JavaScript off at the engine level too (Emulation below is per page).
		chromedp.Flag("blink-settings", "scriptEnabled=false"),
	}
	if opts.NoSandbox {
		allocOpts = append(allocOpts, chromedp.NoSandbox)
	}

	// The timeout bounds the whole browser lifetime; cancelling the allocator
	// kills the process tree.
	tctx, tcancel := context.WithTimeout(context.Background(), timeout)
	defer tcancel()
	actx, acancel := chromedp.NewExecAllocator(tctx, allocOpts...)
	defer acancel()
	ctx, cancel := chromedp.NewContext(actx)
	defer cancel()

	var mu sync.Mutex
	var blockedURLs []string

	// Start the browser and target, then turn JS off and interception on
	// before any navigation away from about:blank.
	if _, err := chromedp.Call(ctx, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true}); err != nil {
		return nil, chromiumErr(tctx, timeout, "disable javascript", err)
	}
	paused := chromedp.Events(ctx, fetch.RequestPaused)
	handlerDone := make(chan struct{})
	go func() {
		defer close(handlerDone)
		for ev, err := range paused {
			if err != nil {
				return
			}
			if allowedRequest(ev.Request.URL, absHTML) {
				_, _ = chromedp.Call(ctx, fetch.ContinueRequest, fetch.ContinueRequestParams{RequestID: ev.RequestID})
				continue
			}
			mu.Lock()
			blockedURLs = append(blockedURLs, ev.Request.URL)
			mu.Unlock()
			_, _ = chromedp.Call(ctx, fetch.FailRequest, fetch.FailRequestParams{RequestID: ev.RequestID, ErrorReason: network.ErrorReasonBlockedByClient})
		}
	}()
	if _, err := chromedp.Call(ctx, fetch.Enable, fetch.EnableParams{Patterns: []*fetch.RequestPattern{{URLPattern: "*"}}}); err != nil {
		return nil, chromiumErr(tctx, timeout, "enable request interception", err)
	}
	if _, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{
		Width: int64(width), Height: 800, DeviceScaleFactor: 1,
	}); err != nil {
		return nil, chromiumErr(tctx, timeout, "set viewport", err)
	}
	if err := chromedp.Do(ctx, chromedp.Navigate(pageURL)); err != nil {
		return nil, chromiumErr(tctx, timeout, "load html", err)
	}

	metrics, err := chromedp.Call(ctx, page.GetLayoutMetrics, cdp.Empty{})
	if err != nil {
		return nil, chromiumErr(tctx, timeout, "layout metrics", err)
	}
	height := 800.0
	if metrics.CSSContentSize != nil && metrics.CSSContentSize.Height > 0 {
		height = math.Ceil(metrics.CSSContentSize.Height)
	}
	if height > float64(maxHeight) {
		height = float64(maxHeight)
	}
	beyond := true
	shot, err := chromedp.Call(ctx, page.CaptureScreenshot, page.CaptureScreenshotParams{
		Format:                page.CaptureScreenshotFormatPng,
		Clip:                  &page.Viewport{X: 0, Y: 0, Width: float64(width), Height: height, Scale: 1},
		CaptureBeyondViewport: &beyond,
	})
	if err != nil {
		return nil, chromiumErr(tctx, timeout, "capture screenshot", err)
	}

	// Stop the browser before reading the blocked list so no handler is mid-append.
	cancel()
	acancel()
	<-handlerDone

	mu.Lock()
	blocked = dedupSorted(blockedURLs)
	mu.Unlock()

	if err := os.WriteFile(pngPath, shot.Data, 0o644); err != nil {
		return blocked, fmt.Errorf("write png: %w", err)
	}
	if err := compressPNG(pngPath); err != nil {
		return blocked, fmt.Errorf("compress png: %w", err)
	}
	return blocked, nil
}

// allowedRequest permits exactly the HTML file being rendered and data: URLs.
func allowedRequest(raw, absHTML string) bool {
	if strings.HasPrefix(strings.ToLower(raw), "data:") {
		return true
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "file" || (u.Host != "" && u.Host != "localhost") {
		return false
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return filepath.Clean(filepath.FromSlash(u.Path)) == absHTML
}

func chromiumErr(tctx context.Context, timeout time.Duration, step string, err error) error {
	if errors.Is(tctx.Err(), context.DeadlineExceeded) {
		return fmt.Errorf("chromium %s: timed out after %s", step, timeout)
	}
	return fmt.Errorf("chromium %s: %w", step, err)
}

func dedupSorted(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
