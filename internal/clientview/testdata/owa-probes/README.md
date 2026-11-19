# OWA sanitizer probe corpus

Each `<name>.html` is a small HTML body fragment that exercises one rule of
`OWASanitize` (`internal/clientview/sanitize.go`). Its goldens are:

- `<name>.golden.html`: the sanitized fragment
- `<name>.golden.json`: the `remote_refs`, `links` and `dropped` lists

`TestOWAProbesGolden` checks every probe against its goldens. After a rule
change, regenerate them with
`go test ./internal/clientview -run TestOWAProbesGolden -update` and review
the diff by hand before you commit.

## The rules are unverified

The rule table models known Outlook-on-the-web behaviour. **No rule has been
measured against real OWA yet.** Until each probe has been measured, treat the
OWA view as an approximation.

## Measuring real OWA (manual)

1. Wrap each probe in a minimal invite with its fragment as
   `X-ALT-DESC;FMTTYPE=text/html:` (ICS-escape `,` `;` `\` and newlines), and
   give it a unique `SUMMARY` such as `owa-probe <name>`.
2. Send each invite to a Microsoft 365 test mailbox. Use a mailbox with no
   production data and no forwarding rules, because remote probes contact
   `evil.test`-style hosts that you must replace with a host you control.
3. Open the invite in Outlook on the web. In the browser devtools, copy the
   rendered body's DOM: the element that holds the description in the reading
   pane.
4. Compare that DOM with `<name>.golden.html`. Look at which elements and
   attributes survive, how `class`/`id` are renamed, how CSS is scoped, and
   what happens to remote images.
5. Update the rule table in `sanitize.go` to match, rerun with `-update`, and
   note the measurement date and OWA build next to the rule.
