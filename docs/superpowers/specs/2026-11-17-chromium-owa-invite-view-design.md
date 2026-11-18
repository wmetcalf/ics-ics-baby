# Chromium renderer + Outlook-web invite view — design

Date: 2026-11-17 · Repo: `wmetcalf/ics-ics-baby` · Branch: `feat/chromium-owa-invite-view`

## Intent

ics-ics-baby's PNG is an *analyst* card: ics-ics-baby's own template over sanitized content,
rasterized by a pure-Go drawer (`go`, description as plain text) or by `wkhtmltoimage`
(Qt5 WebKit, upstream archived 2023, unpatched). Two gaps:

1. **Render quality/safety** — a maintained engine should rasterize the existing HTML preview.
2. **Victim fidelity** — nobody can see what the recipient actually saw. The targets are mostly
   Microsoft 365 users, so the view that matters is **Outlook on the web / new Outlook**, a web
   app that Chromium can reproduce closely.

This is the first half of a larger flow: a later blastbox engine (offline, warm Firecracker) will
wrap this binary for Loadout. That adapter is **out of scope here**.

## Goals

- `--screenshot-engine chromium`: render the existing `ics-ics-baby-preview.html` to PNG with
  headless Chromium.
- `--invite-view owa`: emit a second, separate HTML + PNG pair that approximates how Outlook on
  the web displays the meeting request.
- Both work fully **offline**: no request ever leaves the process; remote references are made
  visible rather than fetched.

## Non-goals

- Gmail, Apple Calendar/Mail, classic (Word-engine) Outlook views. Apple and classic Outlook are
  better served later by real-client engines (Mac host / CAPE Windows VM); the design leaves a
  profile seam (`--invite-view <profile>`) so more profiles can be added.
- Network-enabled rendering, Tor/VPN, Camoufox. Remote content and link-following belong to
  Loadout's `email-render` and browser engines.
- Converting MSG/TNEF/vCalendar 1.0 to ICS (separate follow-on in Loadout `email-convert`).
- The blastbox engine and Loadout adapter.

## Anti-goals (failure even if it "works")

- Any change to existing default output: with no new flags, manifest, HTML and PNG are
  byte-for-byte the same as before (`go` stays the default engine).
- Any network fetch from the render, including DNS. A beacon must never fire.
- JavaScript executing from invite content.
- Microsoft logos/trademarks in the view — layout is modeled on OWA, unbranded.
- Presenting the OWA view as authoritative: it is labeled as an emulation.

## Constraints

- **Standalone first (Will, 2026-11-17).** ics-ics-baby stays a self-contained CLI that works
  exactly as today. No blastbox imports, envelopes, or required config in this repo. Every new
  behaviour is behind an opt-in flag. The blastbox engine is a separate wrapper that invokes the
  binary; ics-ics-baby never knows whether it runs inside one.

- The release binary stays statically linked with no runtime deps: Chromium is a runtime
  requirement **only** when `chromium` engine or `--invite-view` is selected; missing browser →
  clear error, non-zero exit for that step (same pattern as wkhtml).
- Go module (go 1.25). Existing tests must not require Chromium (CI/Apple-silicon lesson from
  af9e68a); Chromium tests skip when no browser binary is found.
- Attacker-controlled input: cap sizes, time-box the browser, never let content pick paths.

## Approaches considered

- **A. One unbranded OWA-style view with a profile seam (chosen).** M365 is the target
  population; OWA is a web app so Chromium is faithful; the abused chrome (RSVP bar, Teams Join
  button) is reproduced.
- B. Per-client profiles (OWA, Gmail, Apple) now — three layouts + three measured sanitizers to
  maintain; Gmail builds its own card and Apple is native, so emulation there is weak. Deferred.
- C. Raw description only, no client frame — misses the Join/RSVP chrome phishing relies on.

Driver: **chromedp** (pure Go CDP client) — keeps the binary static, gives full-page screenshots,
JS disable, and request interception. Exec'ing `chrome --headless --screenshot` was rejected: no
full-page sizing and no request interception.

## Design

### 1. Chromium renderer (`internal/render/chromium.go`)

`RenderHTMLWithChromium(htmlPath, pngPath string, width int, opts ChromiumSettings) error`

- Launch a fresh headless Chromium per call with a throwaway profile dir inside the output dir's
  temp area; kill the process tree on return/timeout.
- Hardening flags: `--headless=new`, `--disable-gpu`, `--no-first-run`, `--disable-extensions`,
  `--disable-background-networking`, `--disable-sync`, `--disable-default-apps`,
  `--proxy-server=127.0.0.1:9` + `--proxy-bypass-list=<-loopback>` (dead proxy, belt),
  `--host-resolver-rules=MAP * ~NOTFOUND` (no DNS, braces).
- **Suspenders**: CDP `Fetch.enable` on all URLs; continue only `file://` requests under the
  HTML file's directory and `data:` URLs; fail everything else and record it
  (`blocked_requests`). JS disabled via `Emulation.setScriptExecutionDisabled`.
- Viewport width = `--width`; full-page capture (`Page.captureScreenshot` with
  `captureBeyondViewport`), height capped (default 20000 px) to bound attacker layouts.
- Chrome sandbox stays **on** by default; `--chromium-no-sandbox` exists only for containers
  that cannot provide it (blastbox decides).
- Settings: `Bin` (default: search `chromium`, `chromium-browser`, `google-chrome`, `headless_shell`),
  `Timeout` (default 45s), `NoSandbox`, `MaxHeight`.
- Output PNG run through existing `compressPNG`.

CLI: `--screenshot-engine go|wkhtml|chromium`, `--chromium-bin`, `--chromium-timeout`,
`--chromium-no-sandbox`. Agenda view unchanged (Go only).

### 2. OWA invite view (`internal/clientview/`)

`--invite-view owa` writes `ics-ics-baby-owa.html` and, rendered **always with Chromium**,
`ics-ics-baby-owa.png`. Off by default. One view per VEVENT that has `METHOD:REQUEST`/`PUBLISH`
semantics; with several events, the first is rendered and the rest listed (builder may refine).

**Layout** (template `owa.html.tmpl`, unbranded, OWA-like reading pane):

```text
subject (SUMMARY)
organizer display name + address (ORGANIZER CN / mailto)
when (DTSTART–DTEND in invite TZ, recurrence summary)
where (LOCATION; URL-looking locations rendered as links, as OWA does)
RSVP bar: Accept · Tentative · Decline · Propose new time   (inert)
Join button when a meeting URL is present
  (X-MICROSOFT-SKYPETEAMSMEETINGURL, then X-MICROSOFT-ONLINEMEETINGCONFLINK, then CONFERENCE)
  — label shows the real target host beneath it
attendees (required / optional)
body: X-ALT-DESC (FMTTYPE=text/html) if present, else DESCRIPTION as preformatted text
attachments list (names + sizes, inert)
footer: "Emulated Outlook-on-the-web rendering — ics-ics-baby" + input sha256
```

**Body sanitizer** (`owaSanitize`), taking the **raw** `X-ALT-DESC` from `RawProps`, not
ics-ics-baby's analyst-sanitized `description_html`. Built on `golang.org/x/net/html` (already a
dependency). Initial rule set models known OWA behaviour; every rule is marked
*unverified until probed* in code and doc:

```text
drop elements:   script, iframe, frame, frameset, object, embed, applet, form, input,
                 button, select, textarea, meta, link, base, svg-script, noscript
drop attributes: on*, formaction, srcdoc, any javascript:/vbscript: URL, data: URLs except images
keep:            style elements and style attributes (OWA keeps them, scoping them to the body)
scope CSS:       prefix selectors to the body container; drop @import, position:fixed
remote refs:     img/src, background, CSS url(), srcset → replaced by a visible placeholder
                 box "remote image: <host>" (size from width/height attrs if given)
links:           kept, href shown on hover-equivalent: rendered with a small host label
```

Every remote reference, link and dropped construct is also listed in a sidecar
`ics-ics-baby-owa.json` (`remote_refs`, `links`, `dropped`, `blocked_requests`, `profile:"owa"`,
`emulation:true`) so Loadout can turn them into evidence without re-parsing HTML.

### 2a. Which inputs get an OWA view *(call made during review)*

`METHOD` is calendar-level (`cal.Method`). Many phishing `.ics` files have none.

```text
REQUEST / PUBLISH / no METHOD   → render as a meeting request
CANCEL                          → render with a "Canceled:" subject prefix and cancellation banner
REPLY / COUNTER / DECLINECOUNTER / REFRESH / ADD → no view; sidecar status "skipped", reason
no VEVENT (VTODO/VJOURNAL/free-busy only)       → no view; sidecar status "skipped", reason
```

### 2b. Failure and ordering semantics *(call made during review)*

- Order: manifest → preview HTML → preview PNG → OWA HTML → OWA sidecar → OWA PNG.
  An OWA failure never removes or blocks the preview outputs.
- The OWA HTML and sidecar are always written when `--invite-view` is set. The sidecar
  carries `status: ok | skipped | render_failed` and an `error` string.
- Exit code: `0` when everything requested succeeded or the OWA view was `skipped`;
  `1` when a requested render failed (Chromium missing or timed out, either PNG), after
  writing everything else and printing the "Wrote …" lines for what exists.

### 2c. Local, relative and `cid:` references *(call made during review)*

- Request interception allows **exactly one** `file://` URL: the generated HTML being rendered.
  Everything else is failed, including other files in the output dir (which holds the
  extracted, attacker-controlled `ics-ics-baby-attachments/`).
- In the OWA sanitizer, relative, `file:` and `cid:` references are treated like remote ones:
  replaced by a placeholder ("local reference: …" / "inline cid: …") and listed in
  `remote_refs` with a `kind` field (`remote | local | cid`).
- `data:` images stay allowed (they are self-contained), capped at 2 MiB each.

### 2d. Blocked requests from the preview render *(call made during review)*

`RenderHTMLWithChromium` returns the list of blocked URLs. For the preview (no sidecar) they are
printed to stderr as `[chromium] blocked: <url>` and do not fail the run; no new output file.
For the OWA render they go in the sidecar's `blocked_requests`.

### 2e. Image hashing is the pipeline's job *(Will, 2026-11-17)*

ics-ics-baby computes **no** perceptual hashes and takes no rosetta-squint dependency (it would
need cgo + libturbojpeg/libwebp and break the static cross-compiled release). Instead it hands
back every image as a file and Loadout hashes them at ingress (`image_hashes.py`, rosetta-squint
`phash`/`colorhash`, which it recomputes for worker screenshots anyway):

```text
preview      ics-ics-baby-preview.png
owa          ics-ics-baby-owa.png (when --invite-view)
attachment   extracted ATTACH files (existing behaviour)
inline       data: images from the raw X-ALT-DESC, written to ics-ics-baby-inline/
             (when --invite-view; listed in the sidecar as remote_refs kind "inline_data"
             with path, sha256, mime; same 2 MiB cap)
```

### 3. Probe corpus (measurement, not runtime)

`testdata/owa-probes/` — one small ICS per sanitizer rule (tag, attribute, CSS feature). Golden
tests assert the sanitizer's current output. A README describes how to measure real OWA (deliver
each probe to an M365 test mailbox, capture the rendered DOM) and update rules + goldens. Running
that measurement is a later manual step; this change ships the corpus and goldens only.

### 4. Preserved behaviour

- No new flags → identical outputs (golden test on an existing testdata file before/after).
- `wkhtml` path untouched. `wkhtml-wrap` untouched.
- Manifest schema unchanged; OWA facts go in the new sidecar.

## Testing

- Unit: sanitizer goldens over the probe corpus; Join-URL precedence; remote-ref extraction.
- Chromium (skipped without a browser): renders preview and OWA view; a probe with
  `<img src=http://…>` and CSS `url(http://…)` produces zero non-file requests (asserted from the
  CDP request log) and a placeholder; a `<script>` probe does not execute (marker element absent).
- Regression: default-flags output identical to pre-change for existing testdata.

## Left to the builder

- Exact OWA spacing/typography (approximate is fine; unbranded).
- Multi-event handling beyond "first event + list".
- chromedp version pin and the temp-profile location details.
- README updates for the new flags.
