package render

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/png"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// WKHTMLSettings describe how to invoke wkhtmltoimage safely.
type WKHTMLSettings struct {
	Bin          string
	WrapperBin   string // Path to wkhtml-wrap (empty to disable sandbox)
	ExtraArgs    []string
	AllowNetwork bool
	AllowedPaths []string
	Timeout      time.Duration
}

const defaultWKHTMLTimeout = 45 * time.Second

// RenderInvitePNGWithWKHTML converts the already-rendered HTML preview into a PNG
// via wkhtmltoimage. The HTML should be trusted output from our renderer.
// Uses wkhtml-wrap secure sandbox if available.
func RenderInvitePNGWithWKHTML(htmlPath, pngPath string, width int, opts WKHTMLSettings) error {
	if htmlPath == "" || pngPath == "" {
		return errors.New("html and png paths must be provided")
	}
	if _, err := os.Stat(htmlPath); err != nil {
		return fmt.Errorf("html input not found: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(pngPath), 0o755); err != nil {
		return fmt.Errorf("create screenshot dir: %w", err)
	}

	bin := opts.Bin
	if strings.TrimSpace(bin) == "" {
		bin = "wkhtmltoimage"
	}
	// Resolve to absolute path if not already absolute
	if !filepath.IsAbs(bin) {
		if fullPath, err := exec.LookPath(bin); err == nil {
			bin = fullPath
		}
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = defaultWKHTMLTimeout
	}
	if width <= 0 {
		width = 1200
	}

	// Build wkhtmltoimage arguments
	wkhtmlArgs := []string{
		"--format", "png",
		"--width", strconv.Itoa(width),
		"--quality", "75",
		"--disable-javascript",
		"--disable-plugins",
		"--enable-local-file-access",
		"--log-level", "warn",
		"--encoding", "utf-8",
	}
	if !opts.AllowNetwork {
		wkhtmlArgs = append(wkhtmlArgs, "--proxy", "http://127.0.0.1:9") // dead proxy blocks network egress
	}
	wkhtmlArgs = append(wkhtmlArgs, opts.ExtraArgs...)
	wkhtmlArgs = append(wkhtmlArgs, htmlPath, pngPath)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Check if wkhtml-wrap sandbox is available
	var cmd *exec.Cmd
	var useWrapper bool
	var wrapperPath string
	userSpecifiedWrapper := opts.WrapperBin != "" && opts.WrapperBin != "wkhtml-wrap"

	// Try to find wrapper: 1) user-specified path, 2) same directory as binary, 3) PATH
	if opts.WrapperBin != "" {
		if p, err := exec.LookPath(opts.WrapperBin); err == nil {
			wrapperPath = p
		} else if userSpecifiedWrapper {
			// User explicitly specified a wrapper path and it doesn't exist - fail
			return fmt.Errorf("wkhtml-wrap not found at specified path %q: %w", opts.WrapperBin, err)
		}
	}
	if wrapperPath == "" && !userSpecifiedWrapper {
		// Try same directory as this binary
		if exePath, err := os.Executable(); err == nil {
			tryPath := filepath.Join(filepath.Dir(exePath), "wkhtml-wrap")
			if _, err := os.Stat(tryPath); err == nil {
				wrapperPath = tryPath
			}
		}
	}
	if wrapperPath == "" && !userSpecifiedWrapper {
		// Try PATH
		if p, err := exec.LookPath("wkhtml-wrap"); err == nil {
			wrapperPath = p
		}
	}

	if wrapperPath != "" {
		// Use secure wrapper
		useWrapper = true
		wrapperArgs := buildWrapperArgs(bin, filepath.Dir(pngPath), opts, wkhtmlArgs)
		cmd = exec.CommandContext(ctx, wrapperPath, wrapperArgs...)
		fmt.Fprintf(os.Stderr, "[wkhtml] Using sandboxed wrapper: %s\n", wrapperPath)
	} else {
		// Fall back to direct execution (less secure)
		cmd = exec.CommandContext(ctx, bin, wkhtmlArgs...)
		cmd.Env = append(os.Environ(),
			"QTWEBKIT_IGNORE_SSL_ERRORS=1",
		)
		fmt.Fprintf(os.Stderr, "[wkhtml] WARNING: Running wkhtmltoimage without sandbox (wkhtml-wrap not found)\n")
		fmt.Fprintf(os.Stderr, "[wkhtml] For better security, ensure wkhtml-wrap is in PATH or the same directory as ics-ics-baby\n")
	}

	cmd.Stdout = io.Discard
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return fmt.Errorf("wkhtmltoimage timed out after %s", timeout)
		}
		mode := "direct"
		if useWrapper {
			mode = "sandboxed"
		}
		return fmt.Errorf("wkhtmltoimage error (%s): %w\n%s", mode, err, stderr.String())
	}

	// Compress the PNG to reduce file size significantly
	if err := compressPNG(pngPath); err != nil {
		return fmt.Errorf("png compression error: %w", err)
	}

	return nil
}

// buildWrapperArgs constructs arguments for wkhtml-wrap secure sandbox.
// Returns: [-outdir DIR -wkhtml BIN -no-net=BOOL -ro PATH ... -- WKHTML_ARGS...]
func buildWrapperArgs(wkhtmlBin, outDir string, opts WKHTMLSettings, wkhtmlArgs []string) []string {
	args := []string{
		"-outdir", outDir,
		"-wkhtml", wkhtmlBin,
		"-no-net=" + strconv.FormatBool(!opts.AllowNetwork),
		"-enforce=true", // Always enforce security in production
	}

	// Add read-only paths for fonts and any user-specified paths
	for _, p := range opts.AllowedPaths {
		args = append(args, "-ro", p)
	}

	// Add common system paths needed for fonts
	args = append(args, "-ro", "/usr/share/fonts")

	// Separator between wrapper args and wkhtmltoimage args
	args = append(args, "--")

	// Add all wkhtmltoimage arguments
	args = append(args, wkhtmlArgs...)

	return args
}

// compressPNG loads a PNG image and re-encodes it with maximum compression
func compressPNG(path string) error {
	// Read the original PNG
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	var img image.Image
	img, err = png.Decode(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("decode png: %w", err)
	}

	// Write back with best compression
	out, err := os.Create(path)
	if err != nil {
		return err
	}
	defer out.Close()

	encoder := &png.Encoder{
		CompressionLevel: png.BestCompression,
	}
	if err := encoder.Encode(out, img); err != nil {
		return fmt.Errorf("encode png: %w", err)
	}

	return nil
}

func allowedPaths(htmlPath string, extra []string) []string {
	base := filepath.Dir(htmlPath)
	all := append([]string{base}, extra...)
	return uniqueNonEmpty(all)
}

func uniqueNonEmpty(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		abs, err := filepath.Abs(v)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		out = append(out, abs)
	}
	return out
}
