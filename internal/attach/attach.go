package attach

import (
	"crypto/md5"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gabriel-vasile/mimetype"
	"ics-ics-baby/internal/icsparse"
	"ics-ics-baby/internal/util"
)

const (
	attachmentFilePerm        = 0o644
	attachmentDownloadTimeout = 30 * time.Second
	maxRedirects              = 5
)

var (
	errAttachmentTooLarge = errors.New("attachment exceeds maximum size limit")
	errPathTraversal      = errors.New("path traversal detected")
)

// validatePath ensures the final path is within the allowed directory
func validatePath(path, allowedDir string) error {
	// Clean and resolve both paths to absolute form
	absPath, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("%w: cannot resolve path", errPathTraversal)
	}

	absDir, err := filepath.Abs(filepath.Clean(allowedDir))
	if err != nil {
		return fmt.Errorf("%w: cannot resolve directory", errPathTraversal)
	}

	// Ensure absDir ends with separator for accurate comparison
	if !strings.HasSuffix(absDir, string(filepath.Separator)) {
		absDir += string(filepath.Separator)
	}

	// Use filepath.Rel for proper path relationship checking
	rel, err := filepath.Rel(absDir, absPath)
	if err != nil {
		return fmt.Errorf("%w: path relationship error", errPathTraversal)
	}

	// Check if the relative path tries to escape using ".."
	if strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
		return fmt.Errorf("%w: path %s is outside %s", errPathTraversal, absPath, absDir)
	}

	return nil
}

// ExtractAll saves inline attachments and optionally downloads remote ones.  Each
// saved attachment is bounded by maxBytes; oversized or failing downloads are
// skipped and reported via stderr while processing continues.
func ExtractAll(cal *icsparse.CalendarInfo, outDir string, download bool, maxBytes int64) error {
	attDir := filepath.Join(outDir, "ics-ics-baby-attachments")
	if err := os.MkdirAll(attDir, 0o755); err != nil {
		return err
	}
	if maxBytes <= 0 {
		maxBytes = 1 << 30
	}

	var firstErr error

	for i := range cal.Events {
		ev := &cal.Events[i]
		evSlug := util.Slugify(fmt.Sprintf("%d-%s", i+1, ev.Summary))
		for j := range ev.Attachments {
			a := &ev.Attachments[j]
			base := fmt.Sprintf("%s-%d", evSlug, j+1)

			switch a.Source {
			case "inline":
				path, size, md5sum, sha, mt, err := saveInline(a.Value, attDir, base, a.Fmt, maxBytes)
				if err != nil {
					noteAttachmentError(base, err)
					firstErr = pickFirstError(firstErr, err)
					continue
				}
				rel, _ := filepath.Rel(outDir, path)
				a.SavedAs = &rel
				a.Size = intPtrFromInt64(size)
				a.MD5 = &md5sum
				a.SHA256 = &sha
				a.Mime = &mt
			case "url":
				if !download {
					a.Href = &a.Value
					continue
				}
				if strings.HasPrefix(a.Value, "http://") || strings.HasPrefix(a.Value, "https://") {
					path, size, md5sum, sha, mt, err := downloadURL(a.Value, attDir, base, a.Fmt, maxBytes)
					if err != nil {
						noteAttachmentError(a.Value, err)
						firstErr = pickFirstError(firstErr, err)
						continue
					}
					rel, _ := filepath.Rel(outDir, path)
					a.SavedAs = &rel
					a.Size = intPtrFromInt64(size)
					a.MD5 = &md5sum
					a.SHA256 = &sha
					a.Mime = &mt
				} else {
					a.Href = &a.Value
				}
			default:
				// Nothing to do.
			}
		}
	}

	return firstErr
}

func saveInline(raw string, attDir, base string, fmtType *string, maxBytes int64) (string, int64, string, string, string, error) {
	// Use secure temp file creation to avoid race conditions
	tmpFile, err := os.CreateTemp(attDir, base+"-*.tmp")
	if err != nil {
		return "", 0, "", "", "", err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	// Try base64 with padding first, then without padding (RawStdEncoding), then raw string
	n, md5sum, shaSum, err := writeWithLimit(base64.NewDecoder(base64.StdEncoding, strings.NewReader(raw)), tmpPath, maxBytes)
	if err != nil {
		os.Remove(tmpPath)
		if isBase64DecodeError(err) {
			// Try RawStdEncoding (base64 without padding) - common in real-world ICS files
			n, md5sum, shaSum, err = writeWithLimit(base64.NewDecoder(base64.RawStdEncoding, strings.NewReader(raw)), tmpPath, maxBytes)
			if err != nil {
				os.Remove(tmpPath)
				if isBase64DecodeError(err) {
					// Fall back to treating as raw string
					n, md5sum, shaSum, err = writeWithLimit(strings.NewReader(raw), tmpPath, maxBytes)
					if err != nil {
						os.Remove(tmpPath)
						return "", 0, "", "", "", err
					}
				} else {
					return "", 0, "", "", "", err
				}
			}
		} else {
			return "", 0, "", "", "", err
		}
	}

	mt, err := detectMIME(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return "", 0, "", "", "", err
	}

	finalName := base + chooseExtension(fmtType, mt)
	finalPath := filepath.Join(attDir, finalName)

	// Validate path to prevent traversal attacks
	if err := validatePath(finalPath, attDir); err != nil {
		os.Remove(tmpPath)
		return "", 0, "", "", "", err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return "", 0, "", "", "", err
	}

	// Set final permissions
	if err := os.Chmod(finalPath, attachmentFilePerm); err != nil {
		os.Remove(finalPath)
		return "", 0, "", "", "", err
	}

	return finalPath, n, fmtHex(md5sum), fmtHex(shaSum), mt, nil
}

func downloadURL(u, attDir, base string, fmtType *string, maxBytes int64) (string, int64, string, string, string, error) {
	// Create HTTP client with redirect limiting
	redirectCount := 0
	client := &http.Client{
		Timeout: attachmentDownloadTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			redirectCount++
			if redirectCount > maxRedirects {
				return fmt.Errorf("too many redirects (max %d)", maxRedirects)
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", u, nil)
	if err != nil {
		return "", 0, "", "", "", err
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, "", "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", 0, "", "", "", fmt.Errorf("unexpected HTTP status %s", resp.Status)
	}

	if resp.ContentLength > 0 && resp.ContentLength > maxBytes {
		return "", 0, "", "", "", fmt.Errorf("%w: maximum %d bytes", errAttachmentTooLarge, maxBytes)
	}

	filename := filenameFromURL(u, fmtType, base)
	if filename == "" {
		filename = base
	}

	// Use secure temp file creation
	tmpFile, err := os.CreateTemp(attDir, filename+"-*.tmp")
	if err != nil {
		return "", 0, "", "", "", err
	}
	tmpPath := tmpFile.Name()
	tmpFile.Close()

	n, md5sum, shaSum, err := writeWithLimit(resp.Body, tmpPath, maxBytes)
	if err != nil {
		os.Remove(tmpPath)
		return "", 0, "", "", "", err
	}

	mt, err := detectMIME(tmpPath)
	if err != nil {
		os.Remove(tmpPath)
		return "", 0, "", "", "", err
	}

	ext := chooseExtension(fmtType, mt)
	finalName := filename
	if ext != "" && !strings.HasSuffix(strings.ToLower(finalName), strings.ToLower(ext)) {
		finalName = base + ext
	}
	finalPath := filepath.Join(attDir, finalName)

	// Validate path to prevent traversal attacks
	if err := validatePath(finalPath, attDir); err != nil {
		os.Remove(tmpPath)
		return "", 0, "", "", "", err
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		os.Remove(tmpPath)
		return "", 0, "", "", "", err
	}

	// Set final permissions
	if err := os.Chmod(finalPath, attachmentFilePerm); err != nil {
		os.Remove(finalPath)
		return "", 0, "", "", "", err
	}

	return finalPath, n, fmtHex(md5sum), fmtHex(shaSum), mt, nil
}

func filenameFromURL(u string, fmtType *string, fallbackBase string) string {
	if parsed, err := url.Parse(u); err == nil {
		if base := filepath.Base(parsed.Path); base != "" && base != "/" {
			return base
		}
	}
	if fmtType != nil {
		if exts, _ := mime.ExtensionsByType(*fmtType); len(exts) > 0 {
			return fallbackBase + exts[0]
		}
	}
	return fallbackBase
}

func writeWithLimit(r io.Reader, path string, maxBytes int64) (int64, []byte, []byte, error) {
	if maxBytes <= 0 {
		return 0, nil, nil, errors.New("maxBytes must be positive")
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, attachmentFilePerm)
	if err != nil {
		return 0, nil, nil, err
	}
	defer f.Close()

	md5h := md5.New()
	sha := sha256.New()

	limited := &io.LimitedReader{R: r, N: maxBytes + 1}
	n, err := io.Copy(io.MultiWriter(f, md5h, sha), limited)
	if err != nil {
		return n, nil, nil, err
	}
	if limited.N <= 0 {
		return n, nil, nil, fmt.Errorf("%w: maximum %d bytes", errAttachmentTooLarge, maxBytes)
	}

	return n, md5h.Sum(nil), sha.Sum(nil), nil
}

func detectMIME(path string) (string, error) {
	mt, err := mimetype.DetectFile(path)
	if err != nil {
		return "", err
	}
	if mt == nil {
		return "", nil
	}
	return mt.String(), nil
}

func chooseExtension(fmtType *string, detected string) string {
	if fmtType != nil {
		if exts, err := mime.ExtensionsByType(*fmtType); err == nil && len(exts) > 0 {
			return exts[0]
		}
	}
	if detected != "" {
		if exts, err := mime.ExtensionsByType(detected); err == nil && len(exts) > 0 {
			return exts[0]
		}
	}
	return ""
}

func fmtHex(sum []byte) string {
	const digits = "0123456789abcdef"
	out := make([]byte, len(sum)*2)
	for i, v := range sum {
		out[i*2] = digits[v>>4]
		out[i*2+1] = digits[v&0x0f]
	}
	return string(out)
}

func isBase64DecodeError(err error) bool {
	if err == nil {
		return false
	}
	errStr := err.Error()
	return strings.Contains(errStr, "illegal base64 data") || strings.Contains(errStr, "unexpected EOF")
}

func intPtrFromInt64(v int64) *int {
	if v < 0 {
		return nil
	}
	if v > int64(^uint(0)>>1) {
		v = int64(^uint(0) >> 1)
	}
	val := int(v)
	return &val
}

func noteAttachmentError(identifier string, err error) {
	if err == nil {
		return
	}
	fmt.Fprintf(os.Stderr, "warning: skipping attachment %q: %v\n", identifier, err)
}

func pickFirstError(existing, candidate error) error {
	if existing != nil {
		return existing
	}
	return candidate
}
