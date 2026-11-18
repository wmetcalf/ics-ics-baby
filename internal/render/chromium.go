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
	"runtime"
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
	// Logf receives chromedp's own log and error lines, already prefixed with
	// "[chromium] chromedp: ". Nil writes them to stderr.
	Logf func(format string, args ...any)
}

const (
	maxTrackedRequests       = 10000
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
	res, err := RenderHTMLWithChromiumResult(htmlPath, pngPath, width, opts)
	return res.Blocked, err
}

// ChromiumResult is what a Chromium render reports besides errors.
type ChromiumResult struct {
	// Blocked are the URLs the render blocked (sorted, de-duplicated).
	Blocked []string
	// Truncated is true when the page was taller than MaxHeight and the
	// capture was cut there.
	Truncated bool
}

// RenderHTMLWithChromiumResult is RenderHTMLWithChromium returning a
// ChromiumResult. Blocked URLs are returned on error too.
func RenderHTMLWithChromiumResult(htmlPath, pngPath string, width int, opts ChromiumSettings) (ChromiumResult, error) {
	blocked, truncated, err := renderChromium(htmlPath, pngPath, width, opts)
	return ChromiumResult{Blocked: blocked, Truncated: truncated}, err
}

func renderChromium(htmlPath, pngPath string, width int, opts ChromiumSettings) (blocked []string, truncated bool, err error) {
	if htmlPath == "" || pngPath == "" {
		return nil, false, errors.New("html and png paths must be provided")
	}
	absHTML, err := filepath.Abs(htmlPath)
	if err != nil {
		return nil, false, fmt.Errorf("resolve html path: %w", err)
	}
	if _, err := os.Stat(absHTML); err != nil {
		return nil, false, fmt.Errorf("html input not found: %w", err)
	}
	bin, err := FindChromium(opts.Bin)
	if err != nil {
		return nil, false, err
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
		return nil, false, fmt.Errorf("create screenshot dir: %w", err)
	}

	profileDir, err := os.MkdirTemp("", "ics-ics-baby-chromium-")
	if err != nil {
		return nil, false, fmt.Errorf("create chromium profile dir: %w", err)
	}
	defer os.RemoveAll(profileDir)

	pageURL := fileURL(absHTML)

	allocOpts := []chromedp.ExecAllocatorOption{
		chromedp.ExecPath(bin),
		chromedp.UserDataDir(profileDir),
	}
	for _, f := range chromiumFlags(opts.NoSandbox) {
		allocOpts = append(allocOpts, chromedp.Flag(f.name, f.value))
	}

	// The timeout bounds the whole browser lifetime; cancelling the allocator
	// kills the process tree.
	tctx, tcancel := context.WithTimeout(context.Background(), timeout)
	defer tcancel()
	actx, acancel := chromedp.NewExecAllocator(tctx, allocOpts...)
	defer acancel()
	ctx, cancel := chromedp.NewContext(actx, chromedp.WithBrowserOption(chromedpLogOptions(opts.Logf)...))
	defer cancel()

	var mu sync.Mutex
	var blockedURLs []string
	var handlerDone chan struct{}
	// Requests the page's own CSP (or another renderer policy) refuses never
	// reach Fetch interception; the Network domain still reports them.
	requestURLs := map[network.RequestID]string{}
	var policyBlocked []network.RequestID
	var netDone chan struct{}
	// collect stops the browser (so no handler is mid-append) and returns the
	// blocked URLs gathered so far. Used on success and on every error path.
	collect := func() []string {
		cancel()
		acancel()
		if handlerDone != nil {
			<-handlerDone
		}
		if netDone != nil {
			<-netDone
		}
		mu.Lock()
		defer mu.Unlock()
		for _, id := range policyBlocked {
			if u, ok := requestURLs[id]; ok && !allowedRequest(u, absHTML) {
				blockedURLs = append(blockedURLs, u)
			}
		}
		return dedupSorted(blockedURLs)
	}
	fail := func(step string, err error) ([]string, bool, error) {
		return collect(), false, chromiumErr(tctx, timeout, step, err)
	}

	// Start the browser and target, then turn JS off and interception on
	// before any navigation away from about:blank.
	if _, err := chromedp.Call(ctx, emulation.SetScriptExecutionDisabled, emulation.SetScriptExecutionDisabledParams{Value: true}); err != nil {
		return fail("disable javascript", err)
	}
	paused := chromedp.Events(ctx, fetch.RequestPaused)
	handlerDone = make(chan struct{})
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
		return fail("enable request interception", err)
	}
	sent := chromedp.Events(ctx, network.RequestWillBeSent)
	failed := chromedp.Events(ctx, network.LoadingFailed)
	sentDone := make(chan struct{})
	go func() {
		defer close(sentDone)
		for ev, err := range sent {
			if err != nil {
				return
			}
			// data: URLs are allowed and can be megabytes; never kept.
			if ev.Request != nil && !strings.HasPrefix(strings.ToLower(ev.Request.URL), "data:") {
				mu.Lock()
				if len(requestURLs) < maxTrackedRequests {
					requestURLs[ev.RequestID] = ev.Request.URL
				}
				mu.Unlock()
			}
		}
	}()
	netDone = make(chan struct{})
	go func() {
		defer close(netDone)
		defer func() { <-sentDone }()
		for ev, err := range failed {
			if err != nil {
				return
			}
			if ev.BlockedReason != "" {
				mu.Lock()
				if len(policyBlocked) < maxTrackedRequests {
					policyBlocked = append(policyBlocked, ev.RequestID)
				}
				mu.Unlock()
			}
		}
	}()
	if _, err := chromedp.Call(ctx, network.Enable, network.EnableParams{}); err != nil {
		return fail("enable network events", err)
	}
	if _, err := chromedp.Call(ctx, emulation.SetDeviceMetricsOverride, emulation.SetDeviceMetricsOverrideParams{
		Width: int64(width), Height: 800, DeviceScaleFactor: 1,
	}); err != nil {
		return fail("set viewport", err)
	}
	if err := chromedp.Do(ctx, chromedp.Navigate(pageURL)); err != nil {
		return fail("load html", err)
	}

	if chromiumAfterLoadHook != nil {
		if err := chromiumAfterLoadHook(); err != nil {
			return fail("after load", err)
		}
	}

	metrics, err := chromedp.Call(ctx, page.GetLayoutMetrics, cdp.Empty{})
	if err != nil {
		return fail("layout metrics", err)
	}
	height := 800.0
	if metrics.CSSContentSize != nil && metrics.CSSContentSize.Height > 0 {
		height = math.Ceil(metrics.CSSContentSize.Height)
	}
	if height > float64(maxHeight) {
		height = float64(maxHeight)
		truncated = true
	}
	beyond := true
	shot, err := chromedp.Call(ctx, page.CaptureScreenshot, page.CaptureScreenshotParams{
		Format:                page.CaptureScreenshotFormatPng,
		Clip:                  &page.Viewport{X: 0, Y: 0, Width: float64(width), Height: height, Scale: 1},
		CaptureBeyondViewport: &beyond,
	})
	if err != nil {
		return fail("capture screenshot", err)
	}

	blocked = collect()

	if err := os.WriteFile(pngPath, shot.Data, 0o644); err != nil {
		return blocked, truncated, fmt.Errorf("write png: %w", err)
	}
	if err := compressPNG(pngPath); err != nil {
		return blocked, truncated, fmt.Errorf("compress png: %w", err)
	}
	return blocked, truncated, nil
}

type chromiumFlag struct {
	name  string
	value any
}

// chromiumFlags are the browser command-line flags. no-sandbox is always set
// explicitly: left unset, chromedp adds --no-sandbox whenever it runs as
// root, silently dropping the sandbox. As root without --chromium-no-sandbox
// Chromium then refuses to start, which fails closed.
func chromiumFlags(noSandbox bool) []chromiumFlag {
	return []chromiumFlag{
		{"headless", "new"},
		{"hide-scrollbars", true},
		{"mute-audio", true},
		{"disable-gpu", true},
		{"no-first-run", true},
		{"no-default-browser-check", true},
		{"disable-extensions", true},
		{"disable-background-networking", true},
		{"disable-sync", true},
		{"disable-default-apps", true},
		{"disable-component-update", true},
		{"disable-domain-reliability", true},
		{"disable-breakpad", true},
		{"disable-client-side-phishing-detection", true},
		{"disable-dev-shm-usage", true},
		{"metrics-recording-only", true},
		{"safebrowsing-disable-auto-update", true},
		{"password-store", "basic"},
		{"use-mock-keychain", true},
		{"force-color-profile", "srgb"},
		// Network belt and braces: dead proxy (loopback included) and no DNS.
		{"proxy-server", "127.0.0.1:9"},
		{"proxy-bypass-list", "<-loopback>"},
		{"host-resolver-rules", "MAP * ~NOTFOUND"},
		// JavaScript off at the engine level too (Emulation below is per page).
		{"blink-settings", "scriptEnabled=false"},
		{"no-sandbox", noSandbox},
	}
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
	got := filepath.Clean(filepath.FromSlash(fileURLSlashPath(u)))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(got, absHTML)
	}
	return got == absHTML
}

// fileURL is the file: URL of an absolute path. A Windows drive path
// (C:\dir\page.html) needs a leading slash: file:///C:/dir/page.html.
func fileURL(absPath string) string {
	p := filepath.ToSlash(absPath)
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

// fileURLSlashPath is the slash path of a file: URL, without the leading
// slash before a Windows drive letter (/C:/dir -> C:/dir).
func fileURLSlashPath(u *url.URL) string {
	p := u.Path
	if len(p) >= 3 && p[0] == '/' && p[2] == ':' && (p[1] >= 'a' && p[1] <= 'z' || p[1] >= 'A' && p[1] <= 'Z') {
		return p[1:]
	}
	return p
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

// chromiumAfterLoadHook, when set by tests, fails a render after the page loaded.
var chromiumAfterLoadHook func() error

// chromedpLogOptions routes chromedp's log and error output (which otherwise
// goes unprefixed through the standard logger) to logf with a "[chromium]"
// prefix, and drops page console output (JavaScript is off anyway).
func chromedpLogOptions(logf func(format string, args ...any)) []chromedp.BrowserOption {
	if logf == nil {
		logf = func(format string, args ...any) { fmt.Fprintf(os.Stderr, format+"\n", args...) }
	}
	prefixed := func(format string, args ...any) { logf("[chromium] chromedp: "+format, args...) }
	return []chromedp.BrowserOption{
		chromedp.WithBrowserLogf(prefixed),
		chromedp.WithBrowserErrorf(func(format string, args ...any) { prefixed("error: "+format, args...) }),
		chromedp.WithConsolef(func(string, ...any) {}),
	}
}
