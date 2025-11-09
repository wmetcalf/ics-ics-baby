package render

import (
	"path/filepath"
	"testing"
)

func TestRenderInvitePNGWithWKHTMLMissingInput(t *testing.T) {
	tmp := t.TempDir()
	target := filepath.Join(tmp, "out.png")
	err := RenderInvitePNGWithWKHTML(filepath.Join(tmp, "missing.html"), target, 800, WKHTMLSettings{Bin: "wkhtmltoimage"})
	if err == nil {
		t.Fatal("expected error for missing HTML input")
	}
}
