package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite golden files")

var (
	builtBinOnce sync.Once
	builtBinPath string
	builtBinErr  error
	builtBinDir  string
)

// testBinary builds the CLI once per test process into a temp dir.
func testBinary(t *testing.T) string {
	t.Helper()
	builtBinOnce.Do(func() {
		dir, err := os.MkdirTemp("", "ics-ics-baby-bin-")
		if err != nil {
			builtBinErr = err
			return
		}
		builtBinDir = dir
		builtBinPath = filepath.Join(dir, "ics-ics-baby")
		out, err := exec.Command("go", "build", "-o", builtBinPath, ".").CombinedOutput()
		if err != nil {
			builtBinErr = fmt.Errorf("go build: %v\n%s", err, out)
		}
	})
	if builtBinErr != nil {
		t.Fatalf("build binary: %v", builtBinErr)
	}
	return builtBinPath
}

func TestMain(m *testing.M) {
	flag.Parse()
	code := m.Run()
	if builtBinDir != "" {
		_ = os.RemoveAll(builtBinDir)
	}
	os.Exit(code)
}

// cliResult is one CLI invocation's observable outputs.
type cliResult struct {
	stdout, stderr string
	exit           int
}

func runCLI(t *testing.T, args ...string) cliResult {
	t.Helper()
	cmd := exec.Command(testBinary(t), args...)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run CLI: %v", err)
		}
		code = ee.ExitCode()
	}
	return cliResult{stdout: so.String(), stderr: se.String(), exit: code}
}

var defaultGoldenFixtures = []string{
	"../../ics-parser-test-suite/ics_files/06-outlook-has-attach.ics",
	"../../ics-parser-test-suite/ics_files/11-html-alt-desc.ics",
	"../../ics-parser-test-suite/ics_files/15-multiple-events.ics",
	"../../ics-parser-test-suite/ics_files/16-cancel-preaccepted.ics",
	"../../internal/icsparse/testdata/description-html.ics",
}

// TestDefaultOutputsUnchanged pins the CLI's outputs with no new flags:
// manifest, preview HTML and preview PNG bytes plus stdout/stderr/exit code.
// The only normalization is replacing the temp output dir with "OUT" in text
// outputs (it is the only varying field; the PNG is hashed raw).
func TestDefaultOutputsUnchanged(t *testing.T) {
	for _, fx := range defaultGoldenFixtures {
		fx := fx
		name := strings.TrimSuffix(filepath.Base(fx), ".ics")
		t.Run(name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), "out")
			res := runCLI(t, "--out", out, fx)
			norm := func(s string) string { return strings.ReplaceAll(s, out, "OUT") }

			lines := map[string]string{}
			hash := func(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
			for _, f := range []string{"ics-ics-baby-manifest.json", "ics-ics-baby-preview.html", "ics-ics-baby-preview.png"} {
				b, err := os.ReadFile(filepath.Join(out, f))
				if err != nil {
					t.Fatalf("read %s: %v", f, err)
				}
				if !strings.HasSuffix(f, ".png") {
					b = []byte(norm(string(b)))
				}
				lines[f] = hash(b)
			}
			lines["stdout"] = hash([]byte(norm(res.stdout)))
			lines["stderr"] = hash([]byte(norm(res.stderr)))
			lines["exit"] = fmt.Sprint(res.exit)

			keys := make([]string, 0, len(lines))
			for k := range lines {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			var got strings.Builder
			for _, k := range keys {
				fmt.Fprintf(&got, "%s  %s\n", lines[k], k)
			}

			golden := filepath.Join("testdata", "default-golden", name+".sha256")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, []byte(got.String()), 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden (run with -update on an unmodified tree): %v", err)
			}
			if got.String() != string(want) {
				t.Errorf("default outputs changed for %s\n got:\n%s\nwant:\n%s", name, got.String(), want)
			}
		})
	}
}
