# Chromium Renderer + Outlook-Web Invite View Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in headless-Chromium PNG renderer and an opt-in, offline, unbranded Outlook-on-the-web invite view to ics-ics-baby, with default behaviour byte-for-byte unchanged.

**Architecture:** `internal/render/chromium.go` drives a throwaway headless Chromium over CDP (chromedp) with JS off and every request except the one HTML file failed. A new `internal/clientview` package selects the event, sanitizes the raw `X-ALT-DESC` with OWA-style rules, writes `ics-ics-baby-owa.html` + `ics-ics-baby-owa.json` + `ics-ics-baby-inline/`, and the CLI renders it with the Chromium renderer.

**Tech Stack:** Go 1.25 module `ics-ics-baby`; `github.com/chromedp/chromedp` (+ `cdproto`), `golang.org/x/net/html` (already a dependency); standard `testing`.

**Spec:** `docs/superpowers/specs/2026-11-17-chromium-owa-invite-view-design.md`

## Global Constraints

- With no new flags, manifest, `ics-ics-baby-preview.html` and `ics-ics-baby-preview.png` are byte-for-byte identical to before; `go` stays the default `--screenshot-engine`.
- Standalone CLI: no blastbox code, envelopes or config in this repo. Every new behaviour is behind an opt-in flag.
- No perceptual hashing and no rosetta-squint dependency (Loadout hashes at ingress).
- Release binary stays `CGO_ENABLED=0` and statically linked; Chromium is needed only at runtime when `--screenshot-engine chromium` or `--invite-view` is used.
- No network request (including DNS) from any render. JavaScript never runs from invite content.
- No Microsoft logos/trademarks in the OWA view; it is labeled "Emulated Outlook-on-the-web rendering — ics-ics-baby".
- Manifest JSON schema unchanged (new parser fields are `json:"-"`).
- Tests that need Chromium call `t.Skip` when `render.FindChromium("")` returns an error; `go test ./...` must pass on a machine without Chromium.
- Chromium defaults: timeout 45s, max capture height 20000 px, sandbox on; `data:` images capped at 2 MiB each.

## Review Focus

- A raw `X-ALT-DESC` still in ICS TEXT escaping (`\n`, `\,`, `\;`, `\\`) must be unescaped before HTML parsing, or the view shows literal backslashes → Task 4 test `TestAltDescRawUnescaped`.
- Huge or deeply nested `X-ALT-DESC` (10 MB, 100k nested `<div>`) must not hang or blow the stack; sanitizer caps input at 2 MiB of HTML and depth at 512, recording `dropped: "input_truncated"` / `"depth_limit"` → Task 5 test `TestSanitizeCaps`.
- A Join/meeting URL using `javascript:`, `data:` or `file:` must render as inert text, never an `href` → Task 6 test `TestJoinURLNonHTTPInert`.
- Attacker layout that is enormous (100k px tall) must be clipped at max height and finish within the timeout, not hang → Task 2 test `TestChromiumMaxHeight`.
- `X-ALT-DESC` with `FMTTYPE=text/plain` or no FMTTYPE must be shown as preformatted text, not parsed as HTML → Task 6 test `TestBodyPlainWhenNotHTML`.

---

### Task 1: Default-output regression golden

Pins "behaves as today" before any code changes.

**Files:**
- Create: `cmd/ics-ics-baby/default_output_test.go`
- Create: `cmd/ics-ics-baby/testdata/default-golden/<fixture>.sha256` (generated)

**Interfaces:**
- Produces: `TestDefaultOutputsUnchanged` — later tasks must keep it green.

- [ ] **Step 1: Write the test.** Build the binary once (`go build -o $TMP/ics-ics-baby ./cmd/ics-ics-baby`), run it with no new flags (`--out <tmp>/<name>`) on `ics-parser-test-suite/ics_files/{06-outlook-has-attach,11-html-alt-desc,15-multiple-events,16-cancel-preaccepted}.ics` and `internal/icsparse/testdata/description-html.ics`. For each run, sha256 the manifest, preview HTML and preview PNG, after replacing the temp out-dir prefix with `OUT` in the text files. Compare with the golden file; with `-update` set, write the golden instead.
- [ ] **Step 2: Generate goldens on the unmodified tree:** `go test ./cmd/ics-ics-baby -run TestDefaultOutputsUnchanged -update`, then run again without `-update`. Expected: PASS. Run it twice more to prove the outputs are deterministic. If they aren't, normalize only the fields that vary (record which ones in a comment) and never loosen the PNG check.
- [ ] **Step 3: Commit** `test: pin default CLI outputs before renderer work`.

### Task 2: Chromium renderer

**Files:**
- Create: `internal/render/chromium.go`
- Create: `internal/render/chromium_test.go`
- Modify: `go.mod`, `go.sum` (`go get github.com/chromedp/chromedp@latest`; record the pinned version in the commit message)

**Interfaces:**
- Produces:
  - `type ChromiumSettings struct { Bin string; Timeout time.Duration; NoSandbox bool; MaxHeight int }` (zero values mean the defaults: auto-detect, 45s, sandbox on, 20000)
  - `func FindChromium(bin string) (string, error)`: when `bin` is non-empty, returns it if it's executable. Otherwise it searches PATH for `chromium`, `chromium-browser`, `google-chrome`, `headless_shell` in that order.
  - `func RenderHTMLWithChromium(htmlPath, pngPath string, width int, opts ChromiumSettings) (blocked []string, err error)`

- [ ] **Step 1: Write failing tests** (each one skips when `FindChromium("")` errors):
  - `TestChromiumRendersPNG`: a small HTML page renders, the PNG decodes, and its width equals `width`.
  - `TestChromiumBlocksNetwork`: start an `httptest.Server` that counts hits. The HTML includes `<img src=SERVER/i>`, `<link rel=stylesheet href=SERVER/c>`, a CSS `url(SERVER/b)` and `<iframe src=SERVER/f>`. Assert the server got 0 hits and `blocked` contains the `/i` URL.
  - `TestChromiumBlocksSiblingFiles`: write `secret.png` next to the HTML and reference it by a relative `src`. It must be in `blocked`, and the image must not load (0 bytes read; check with `naturalWidth` via a `data-` attribute screenshot diff, or only assert `blocked` includes it).
  - `TestChromiumNoJS`: `<script>new Image().src="SERVER/js"</script>`. The server gets 0 hits and `/js` is **not** in `blocked`, which proves the script never ran.
  - `TestChromiumMaxHeight`: `<div style="height:100000px">`. With `MaxHeight: 3000`, the PNG height is ≤ 3000 and the call returns before `Timeout`.
  - `TestChromiumMissingBinary`: `Bin: "/nonexistent"` returns an error that mentions the path.
- [ ] **Step 2: Run** `go test ./internal/render -run Chromium -v`. Expected: compile failure (undefined).
- [ ] **Step 3: Implement.**
  - Launch with `chromedp.NewExecAllocator` and a fresh `os.MkdirTemp` user-data dir, removed on return.
  - Flags: `headless=new`, `disable-gpu`, `no-first-run`, `disable-extensions`, `disable-background-networking`, `disable-sync`, `disable-default-apps`, `proxy-server=127.0.0.1:9`, `proxy-bypass-list=<-loopback>`, `host-resolver-rules=MAP * ~NOTFOUND`. Add `no-sandbox` only when `NoSandbox` is set.
  - Turn off JS with `emulation.SetScriptExecutionDisabled(true)` before navigating.
  - Call `fetch.Enable` with pattern `*`. On `EventRequestPaused`, continue only when the URL equals the `file://` URL of `htmlPath` (from `url.URL{Scheme:"file", Path: abs}`) or starts with `data:`. Otherwise call `fetch.FailRequest(BlockedByClient)` and record the URL under a mutex.
  - Set the viewport to `width`×800. Read the content height from `page.GetLayoutMetrics`, clamp it to `MaxHeight`, then call `page.CaptureScreenshot` with that clip and `CaptureBeyondViewport`.
  - Write `pngPath` and run the existing `compressPNG`.
  - Bound the whole call with `context.WithTimeout(opts.Timeout)`; cancelling the allocator kills the process tree.
- [ ] **Step 4: Run** `go test ./internal/render -v`. Expected: PASS (Chromium tests run here; `/usr/bin/google-chrome` exists) and the wkhtml tests are unchanged.
- [ ] **Step 5: Commit** `feat(render): sandboxed offline headless-Chromium renderer`.

### Task 3: `--screenshot-engine chromium` in the CLI

**Files:**
- Modify: `cmd/ics-ics-baby/main.go` (flags ~L103-109, usage ~L145, the `case "invite"` switch ~L262)
- Modify: `cmd/ics-ics-baby/integration_test.go`

**Interfaces:**
- Consumes: `render.RenderHTMLWithChromium`, `render.ChromiumSettings`
- Produces: flags `--chromium-bin` (string, default ""), `--chromium-timeout` (duration, 45s), `--chromium-no-sandbox` (bool), `--chromium-max-height` (int, 20000). These are reused by Task 7.

- [ ] **Step 1: Write failing integration tests.**
  - `TestCLIChromiumEngine` (skips without Chromium): `--screenshot-engine chromium` on `11-html-alt-desc.ics` exits 0 and writes a preview PNG that decodes and differs from the `go` engine's PNG.
  - `TestCLIChromiumMissing`: with `--chromium-bin /nonexistent`, exit is 1, stderr contains `chromium render error`, and the manifest and preview HTML still exist.
- [ ] **Step 2: Run** the tests. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Add a `chromium` branch next to `wkhtml`. Print each blocked URL to stderr as `[chromium] blocked: <url>`. Blocked URLs never fail the run.
  - Keep the existing behaviour that any other engine value falls through to `go`.
  - Update the usage text to `go|wkhtml|chromium`.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS, including `TestDefaultOutputsUnchanged`.
- [ ] **Step 5: Commit** `feat(cli): --screenshot-engine chromium`.

### Task 4: Keep the raw X-ALT-DESC and its FMTTYPE on events

**Files:**
- Modify: `internal/icsparse/icsparse.go` (`EventInfo` ~L49-86, the `X-ALT-DESC` case ~L802)
- Test: `internal/icsparse/icsparse_altdesc_raw_test.go`

**Interfaces:**
- Produces: `EventInfo.AltDescRaw *string \`json:"-"\`` (ICS-unescaped raw value, not sanitized) and `EventInfo.AltDescFmtType *string \`json:"-"\`` (the `FMTTYPE` param, lower-cased). The first X-ALT-DESC per event wins.

- [ ] **Step 1: Write failing tests.**
  - `TestAltDescRawUnescaped`: an event with `X-ALT-DESC;FMTTYPE=text/html:<p>a\, b\; c\\n</p>\n<b>x</b>` gives `*AltDescRaw == "<p>a, b; c\\n</p>\n<b>x</b>"` (the escaped `\\` stays a single backslash and `\n` becomes a newline) and `*AltDescFmtType == "text/html"`.
  - `TestAltDescRawNotInManifest`: marshal the parsed calendar to JSON. It contains neither `alt_desc_raw` nor `AltDescRaw`.
- [ ] **Step 2: Run** `go test ./internal/icsparse -run AltDesc -v`. Expected: FAIL.
- [ ] **Step 3: Implement.** Set the two fields in the `X-ALT-DESC` case, using the parser's existing ICS TEXT unescape helper. Find the one `absorbDescription` uses; don't write a second one. Copy the fields through anywhere `EventInfo` values are rebuilt (see ~L286).
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS, golden unchanged.
- [ ] **Step 5: Commit** `feat(icsparse): retain raw X-ALT-DESC and FMTTYPE (not serialized)`.

### Task 5: OWA-style sanitizer + probe corpus

**Files:**
- Create: `internal/clientview/sanitize.go`
- Create: `internal/clientview/sanitize_test.go`
- Create: `internal/clientview/testdata/owa-probes/*.html` + `*.golden.html` + `*.golden.json`
- Create: `internal/clientview/testdata/owa-probes/README.md`

**Interfaces:**
- Produces:
  - `type RemoteRef struct { Kind string \`json:"kind"\`; Value string \`json:"value"\`; Host string \`json:"host,omitempty"\`; Path string \`json:"path,omitempty"\`; SHA256 string \`json:"sha256,omitempty"\`; Mime string \`json:"mime,omitempty"\` }`. `Kind` is one of `remote`, `local`, `cid`, `inline_data`.
  - `type Link struct { Href string \`json:"href"\`; Host string \`json:"host"\`; Text string \`json:"text"\` }`
  - `type SanitizeResult struct { HTML string; RemoteRefs []RemoteRef; Links []Link; Dropped []string }`
  - `func OWASanitize(raw string, inlineDir string) (SanitizeResult, error)`. When `inlineDir` is non-empty, each `data:image/*` gets written to `inlineDir/<sha256>.<ext>`. The file name always comes from the hash, never from the content.

- [ ] **Step 1: Write the probe corpus**, one small HTML fragment per rule:
  - one probe per dropped element, using the spec list: `script iframe frame frameset object embed applet form input button select textarea meta link base noscript` and `svg` containing `script`
  - `on*` attributes, `formaction`, `srcdoc`, `javascript:`/`vbscript:` hrefs
  - `data:` non-image href, `data:image` img, `data:image` over 2 MiB
  - remote `img`, `background` attribute, CSS `url()` in a style attribute and in a `<style>` block, `srcset`
  - relative `src`, `file:` src, `cid:` src
  - `@import`, `position:fixed`, a `<style>` rule like `body{}` that must end up scoped
  - a plain link
- [ ] **Step 2: Write failing tests.**
  - `TestOWAProbesGolden`: for every probe, `OWASanitize` matches the `.golden.html` and the `.golden.json` (RemoteRefs/Links/Dropped). `-update` regenerates the goldens; review the regenerated goldens by hand before committing.
  - Explicit assertions so the goldens can't drift silently:
    - `TestNoActiveContentSurvives`: across all probe outputs, no `<script`, `on\w+=`, `javascript:` or `<iframe`.
    - `TestRemoteImagePlaceholder`: `<img src="http://evil.test/p.gif" width=1 height=1>` becomes an element with class `owa-remote-ref` and text `remote image: evil.test`, and `RemoteRefs[0] == {Kind:"remote", Host:"evil.test", Value:"http://evil.test/p.gif"}`.
    - `TestLocalAndCidRefs`: `src="logo.png"` gives Kind `local` with text `local reference: logo.png`; `src="cid:abc"` gives Kind `cid` with text `inline cid: abc`.
    - `TestInlineDataWritten`: a 1×1 PNG `data:` image stays as `data:` in the HTML and is written to `inlineDir/<sha>.png`. The RemoteRef has Kind `inline_data` with Path, SHA256 and `Mime == "image/png"`.
    - `TestStyleScoped`: `<style>p{color:red}</style>` comes out with the selector prefixed by `.owa-body `.
    - `TestSanitizeCaps`: an input of 10 MB, and 100k nested `div`s. Both return within 2s, output ≤ 2 MiB, `Dropped` contains `input_truncated` (and `depth_limit` for the second).
- [ ] **Step 3: Run** `go test ./internal/clientview -v`. Expected: FAIL.
- [ ] **Step 4: Implement.**
  - Tokenize with `golang.org/x/net/html` (`html.Parse` on a `<div class="owa-body">` context fragment) and walk iteratively with an explicit stack to enforce the depth cap.
  - Rules exactly as in spec §2 and §2c. Put a `// unverified until probed against real OWA` comment on the rule table.
  - Scope CSS with a minimal selector prefixer for `<style>` blocks: split rules on `}`, prefix each selector in the comma list, and drop `@import`, `@font-face` with remote `url()`, and `position:fixed`. In both `<style>` and `style=`, replace `url(...)` values that aren't `data:image` with `none` and add a RemoteRef.
  - Links keep their `href`, followed by `<span class="owa-host">(host)</span>`.
- [ ] **Step 5: Run** `go test ./internal/clientview -v`. Expected: PASS.
- [ ] **Step 6: Write `testdata/owa-probes/README.md`** describing the manual measurement: send each probe as an ICS invite to an M365 test mailbox, capture the rendered body DOM from Outlook on the web devtools, then update the rules and run `-update`.
- [ ] **Step 7: Commit** `feat(clientview): OWA-style body sanitizer with probe corpus`.

### Task 6: OWA view model, template and sidecar

**Files:**
- Create: `internal/clientview/owa.go`
- Create: `internal/clientview/owa.html.tmpl` (embedded with `//go:embed`)
- Create: `internal/clientview/owa_test.go`

**Interfaces:**
- Consumes: `OWASanitize`, `RemoteRef`, `Link` (Task 5); `EventInfo.AltDescRaw`/`AltDescFmtType` (Task 4); `icsparse.CalendarInfo`
- Produces:
  - `type Sidecar struct { Profile string \`json:"profile"\`; Emulation bool \`json:"emulation"\`; Status string \`json:"status"\`; Reason string \`json:"reason,omitempty"\`; Error string \`json:"error,omitempty"\`; InputSHA256 string \`json:"input_sha256"\`; EventUID string \`json:"event_uid,omitempty"\`; OtherEvents []string \`json:"other_events,omitempty"\`; JoinURL string \`json:"join_url,omitempty"\`; RemoteRefs []RemoteRef \`json:"remote_refs"\`; Links []Link \`json:"links"\`; Dropped []string \`json:"dropped"\`; BlockedRequests []string \`json:"blocked_requests"\` }`. `Status` is one of `ok`, `skipped`, `render_failed`.
  - `func SelectEvent(cal *icsparse.CalendarInfo) (ev *icsparse.EventInfo, canceled bool, reason string)`
  - `func JoinURL(ev *icsparse.EventInfo) (raw string, active bool)`
  - `func WriteOWA(cal *icsparse.CalendarInfo, inputSHA256, outDir string) (Sidecar, error)`. Writes `ics-ics-baby-owa.html` and the `ics-ics-baby-inline/` contents. It doesn't write the sidecar; the CLI does, after rendering.

- [ ] **Step 1: Write failing tests.**
  - `TestSelectEventMethods`: run on fixtures. `01` (REQUEST) and `04` (PUBLISH) select. A calendar with no METHOD selects. `16-cancel-preaccepted` gives `canceled == true`. `20-reply-spoof` (REPLY) returns nil with a reason containing `REPLY`. `internal/icsparse/testdata/journal.ics` returns nil with reason `no VEVENT`.
  - `TestJoinURLPrecedence`: with all three present, `X-MICROSOFT-SKYPETEAMSMEETINGURL` wins over `X-MICROSOFT-ONLINEMEETINGCONFLINK`, which wins over the first `CONFERENCE` URI.
  - `TestJoinURLNonHTTPInert`: a Teams URL of `javascript:alert(1)` gives `active == false`. The rendered HTML has no `href="javascript:` and shows the raw value as text.
  - `TestOWAHTMLContents`: on `09-combo-spoof-urgent.ics` the HTML contains the subject, organizer CN and address, `Accept`, `Tentative`, `Decline` and `Propose new time` (inert `<span>`s, not links), a Join button labeled with the real target host when a URL is present, and the footer `Emulated Outlook-on-the-web rendering — ics-ics-baby` plus the input sha256. It must not contain `Microsoft` or `Outlook` outside that footer, and no `<img` pointing at any logo.
  - `TestOWACanceledBanner`: the `16` output has its subject prefixed `Canceled:` and an element with class `owa-cancel-banner`.
  - `TestBodyPlainWhenNotHTML`: when FMTTYPE is `text/plain` or missing, or there's no X-ALT-DESC, the body is shown escaped inside `<pre class="owa-plain">` from `DESCRIPTION`/`AltDescRaw`, and `RemoteRefs` is empty.
  - `TestOWAMultipleEvents`: `15-multiple-events.ics` renders the first event, and `OtherEvents` lists the other summaries.
- [ ] **Step 2: Run** `go test ./internal/clientview -run OWA -v`. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - Use `html/template` with the layout from spec §2: subject, organizer, when (DTSTART–DTEND in the event's location plus the recurrence summary; copy the format string from `webview.formatTimeValue`), where (a location that looks like a URL is rendered as a link plus its host), RSVP bar, Join, attendees grouped by `ROLE` (`REQ-PARTICIPANT` required, `OPT-PARTICIPANT` optional), body, inert attachment list, footer.
  - `active` means an `http`/`https` scheme with a non-empty host.
  - Unbranded CSS inline in the template; spacing is the builder's choice.
  - Skipped calendars return `Sidecar{Status:"skipped", Reason:…}` and write a minimal HTML page that states the reason.
- [ ] **Step 4: Run** `go test ./internal/clientview -v`. Expected: PASS.
- [ ] **Step 5: Commit** `feat(clientview): unbranded Outlook-web invite view`.

### Task 7: `--invite-view owa` in the CLI

**Files:**
- Modify: `cmd/ics-ics-baby/main.go`
- Modify: `cmd/ics-ics-baby/integration_test.go`

**Interfaces:**
- Consumes: `clientview.WriteOWA`, `clientview.Sidecar`, `render.RenderHTMLWithChromium`, and the Chromium flags from Task 3
- Produces: flag `--invite-view` (string, default "", only `owa` accepted; any other value exits 1 with `unknown invite view`). Outputs: `ics-ics-baby-owa.html`, `ics-ics-baby-owa.json`, `ics-ics-baby-owa.png`, `ics-ics-baby-inline/`.

- [ ] **Step 1: Write failing integration tests.**
  - `TestCLIInviteViewOK` (skips without Chromium): on `11-html-alt-desc.ics` the exit is 0, all four outputs exist, and the sidecar has `status:"ok"`, `profile:"owa"` and `emulation:true`.
  - `TestCLIInviteViewSkipped`: on `20-reply-spoof.ics` the exit is 0, the sidecar has `status:"skipped"`, and there's no OWA PNG.
  - `TestCLIInviteViewChromiumMissing`: with `--chromium-bin /nonexistent`, the exit is 1. The preview PNG (go engine) still exists, the OWA HTML and sidecar exist, and the sidecar has `status:"render_failed"` and a non-empty `error`.
  - `TestCLIInviteViewNoBeacon` (skips without Chromium): build a temp ICS whose X-ALT-DESC references an `httptest.Server`. The server gets 0 hits, and the sidecar's `remote_refs` names its host.
  - `TestCLIInviteViewOrder`: when the preview render fails (bad `--chromium-bin` with `--screenshot-engine chromium`), the OWA HTML and sidecar are still written and the exit is 1.
- [ ] **Step 2: Run** the tests. Expected: FAIL.
- [ ] **Step 3: Implement.**
  - The order is fixed: manifest → preview HTML → preview PNG → OWA HTML → OWA PNG → sidecar.
  - Move the existing `os.Exit(1)` calls on preview-render errors into a `failed` flag set after printing the existing error text, so the later steps still run. The error text and the exit code stay the same as today for runs without the new flags.
  - The OWA PNG always uses Chromium. Its blocked URLs go into `Sidecar.BlockedRequests`, and a render error sets `render_failed` and `Error`.
  - `InputSHA256` is the sha256 of the input file.
  - Exit 1 if any requested render failed, otherwise 0. Print `Wrote OWA view:`, `Wrote OWA sidecar:` and `Wrote OWA PNG:` lines for the files that exist.
- [ ] **Step 4: Run** `go test ./...`. Expected: PASS, `TestDefaultOutputsUnchanged` included.
- [ ] **Step 5: Commit** `feat(cli): --invite-view owa`.

### Task 8: Docs and release check

**Files:**
- Modify: `README.md` (Screenshot Engines section, a new "Outlook-web invite view" section, the flag list)
- Modify: `cmd/ics-ics-baby/main.go` usage text, if not already done

- [ ] **Step 1: Write the docs.**
  - The `chromium` engine and its flags.
  - `--invite-view owa`, its outputs, and the sidecar fields.
  - The statement "emulation, unverified until probed".
  - That Chromium is a runtime-only requirement.
  - That hashing is left to downstream pipelines.
- [ ] **Step 2: Verify the release build stays static.** Run `CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build ./cmd/ics-ics-baby` and `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./cmd/ics-ics-baby`. Expected: both succeed. Then `./build.sh` (the default local build). Expected: success.
- [ ] **Step 3: Run** `go test ./...` and `go vet ./...`. Expected: PASS and clean.
- [ ] **Step 4: Commit** `docs: Chromium renderer and Outlook-web invite view`.
