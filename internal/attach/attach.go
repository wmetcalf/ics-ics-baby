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

	"github.com/gabriel-vasile/mimetype"
	"ics-ics-baby/internal/icsparse"
	"ics-ics-baby/internal/util"
)

func ExtractAll(cal *icsparse.CalendarInfo, outDir string, download bool) error {
	attDir := filepath.Join(outDir, "ics-ics-baby-attachments")
	if err := os.MkdirAll(attDir, 0o755); err != nil {
		return err
	}
	for i := range cal.Events {
		ev := &cal.Events[i]
		evSlug := util.Slugify(fmt.Sprintf("%d-%s", i+1, ev.Summary))
		for j := range ev.Attachments {
			a := &ev.Attachments[j]
			base := fmt.Sprintf("%s-%d", evSlug, j+1)
			switch a.Source {
			case "inline":
				p, n, md5s, sha, mt, err := saveInline(a.Value, attDir, base, a.Fmt)
				if err != nil { continue }
				rel, _ := filepath.Rel(outDir, p)
				a.SavedAs = &rel
				a.Size = &n
				a.MD5 = &md5s
				a.SHA256 = &sha
				a.Mime = &mt
			case "url":
				if !download { a.Href = &a.Value; break }
				if strings.HasPrefix(a.Value, "http://") || strings.HasPrefix(a.Value, "https://") {
					p, n, md5s, sha, mt, err := downloadURL(a.Value, attDir, base, a.Fmt)
					if err != nil { continue }
					rel, _ := filepath.Rel(outDir, p)
					a.SavedAs = &rel
					a.Size = &n
					a.MD5 = &md5s
					a.SHA256 = &sha
					a.Mime = &mt
				}
			}
		}
	}
	return nil
}

func saveInline(b64 string, attDir, base string, fmt *string) (string, int, string, string, string, error) {
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil { data = []byte(b64) }

	// Detect MIME type
	mt := mimetype.Detect(data).String()

	// Try to get extension from provided format first, then from detected MIME type
	ext := ""
	if fmt != nil {
		if e, err := mime.ExtensionsByType(*fmt); err == nil && len(e) > 0 { ext = e[0] }
	}
	if ext == "" {
		// Use detected MIME type to get extension
		if e, err := mime.ExtensionsByType(mt); err == nil && len(e) > 0 { ext = e[0] }
	}

	fn := base + ext
	path := filepath.Join(attDir, fn)
	if err := os.WriteFile(path, data, 0o644); err != nil { return "", 0, "", "", "", err }
	h1 := md5.Sum(data)
	h2 := sha256.Sum256(data)
	md5s := fmtHex(h1[:])
	sha := fmtHex(h2[:])
	return path, len(data), md5s, sha, mt, nil
}

func filenameFromURL(u string, fmt *string, fallbackBase string) string {
	if uu, err := url.Parse(u); err == nil {
		if base := filepath.Base(uu.Path); base != "" && base != "/" { return base }
	}
	if fmt != nil {
		if exts, _ := mime.ExtensionsByType(*fmt); len(exts) > 0 { return fallbackBase + exts[0] }
	}
	return fallbackBase
}

func downloadURL(u, attDir, base string, fmt *string) (string, int, string, string, string, error) {
	req, _ := http.NewRequest("GET", u, nil)
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil { return "", 0, "", "", "", err }
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 { return "", 0, "", "", "", errors.New(resp.Status) }
	fn := filenameFromURL(u, fmt, base)
	path := filepath.Join(attDir, fn)
	f, err := os.Create(path)
	if err != nil { return "", 0, "", "", "", err }
	defer f.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil { return "", 0, "", "", "", err }
	if _, err := f.Write(body); err != nil { return "", 0, "", "", "", err }
	h1 := md5.Sum(body)
	h2 := sha256.Sum256(body)
	md5s := fmtHex(h1[:])
	sha := fmtHex(h2[:])
	mt := mimetype.Detect(body).String()
	return path, len(body), md5s, sha, mt, nil
}

func fmtHex(b []byte) string {
    const hexdig = "0123456789abcdef"
    out := make([]byte, len(b)*2)
    for i, v := range b {
        out[i*2] = hexdig[v>>4]
        out[i*2+1] = hexdig[v&0x0f]
    }
    return string(out)
}
func min(a,b int) int { if a<b { return a }; return b }
