---
phase: 01-mcp-tool-real-email-delivery
plan: 01-02
subsystem: notify
tags: [go, goldmark, bluemonday, renderer, html-email, sanitization]

requires:
  - phase: 01-mcp-tool-real-email-delivery
    provides: Renderer interface, Rendered struct
provides: [concrete Renderer implementation, inline-styled email template]
affects: [01-04]

tech-stack:
  added: [goldmark v1.8.2, bluemonday v1.0.27]
  patterns: [template.Must compiled inline, goldmark GFM extension]

key-files:
  created: [internal/notify/render.go, internal/notify/render_test.go]
  modified: []

key-decisions:
  - "D-05: Status labels use plain word tags [Completed]/[Waiting]/[Info]/[Error]"
  - "D-06: Unknown/empty status defaults to [Info] with blue banner"
  - "D-07: Subject CRLF stripped before use (SEC-03)"

patterns-established:
  - "Sanitize goldmark FRAGMENT only with bluemonday UGCPolicy — never the assembled wrapper"
  - "Inline-styled HTML template for email client compatibility"
  - "Plaintext fallback = raw markdown body"

requirements-completed: [RND-01, RND-02, RND-03, RND-04, RND-05]

duration: 10min
completed: 2026-06-26
---

# Plan 01-02: Markdown-to-Email Renderer

**Goldmark+GFM → bluemonday UGCPolicy(sanitize fragment) → inline-styled html/template wrapper with status-colored banner, divider, and footer**

## Performance

- **Duration:** 10 min
- **Started:** 2026-06-26T04:01:00Z
- **Completed:** 2026-06-26T04:11:00Z
- **Tasks:** 1
- **Files modified:** 2

## Accomplishments
- goldmark with GFM extension converts markdown body to HTML fragment
- bluemonday UGCPolicy sanitizes only the fragment (never the assembled wrapper)
- Inline-styled email template: status-colored banner → body → thin divider → "via mcp-notify" timestamp footer
- Status color mapping: completed=#16a34a (green), waiting=#d97706 (amber), info=#2563eb (blue), error=#dc2626 (red)
- Subject prefixed with [Completed]/[Waiting]/[Info]/[Error]; defaults to [Info] for empty/unknown
- CRLF stripped from subject before prefixing (SEC-03 / D-07)
- Plaintext fallback preserved as raw markdown
- 5 table tests all pass using real goldmark+bluemonday (no mocks)

## Verification Gate

| Check | Result |
|---|---|
| `go build ./internal/notify/...` | ✅ |
| `go test ./internal/notify -run TestRender` | ✅ 5/5 |
| `gofumpt -l` | ✅ Clean (0 files) |
| `go vet ./internal/notify/...` | ✅ |

## Files Created
- `internal/notify/render.go` — goldmarkRenderer struct, newRenderer(), Render() method, htmlWrapper template
- `internal/notify/render_test.go` — 5 table tests (SanitizesScript, StripsCRLF, StatusTag, DefaultInfo, InlineStyles)

## Decisions Made
- Used `template.Must(template.New(...).Parse(...))` for compile-time template validation
- UGCPolicy with `RequireParseableURLs(false)` to avoid rejecting valid relative/email URLs
- Status label embedded in both subject prefix and HTML banner for dual visibility

## Next Phase Readiness
- Renderer ready for use by NotificationService in composition root (01-04)
- Next: Plan 01-03 — email Channel implementation with go-mail v0.7.3

---
*Phase: 01-mcp-tool-real-email-delivery*
*Plan: 01-02*
*Completed: 2026-06-26*