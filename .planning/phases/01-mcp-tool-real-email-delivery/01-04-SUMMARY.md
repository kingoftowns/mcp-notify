---
phase: 01-mcp-tool-real-email-delivery
plan: 04
subsystem: api
tags: [mcp, go-sdk, tool-registration, composition-root, main]

requires:
  - phase: 01-mcp-tool-real-email-delivery
    plan: 01
    provides: config struct, notify seam interfaces
  - phase: 01-mcp-tool-real-email-delivery
    plan: 02
    provides: renderer (goldmarkRenderer)
  - phase: 01-mcp-tool-real-email-delivery
    plan: 03
    provides: email channel (Gmail SMTP)
provides:
  - MCP tool registration (send_notification via go-sdk v1.6.1)
  - Composition root (cmd/server/main.go)
  - Handler unit tests with fakeChannel mock
  - Exported constructor wrappers (NewRenderer, NewEmailChannel)
affects: []

tech-stack:
  added: [github.com/modelcontextprotocol/go-sdk@v1.6.1]
  patterns: [TDD w/ fakeChannel mock + real Renderer, go-sdk AddTool generic for schema inference]

key-files:
  created:
    - internal/mcpserver/tool.go
    - internal/mcpserver/server.go
    - internal/mcpserver/tool_test.go
    - cmd/server/main.go
  modified:
    - internal/notify/render.go (added NewRenderer export wrapper)
    - internal/notify/email.go (added NewEmailChannel export wrapper)
    - go.mod (added go-sdk v1.6.1 dep)

key-decisions:
  - "NewStreamableHTTPHandler takes getServer func(*http.Request) *Server — wrapped in closure returning the single srv pointer"
  - "mcp.AddTool(server, tool, handlerFunc) infers JSON schema from SendInput/SendOutput struct types automatically"
  - "mcp.NewServer(impl, opts) with empty ServerCapabilities{} — Tools capability auto-inferred by AddTool"
  - "Handler returns (nil, *CallToolResult, error) pattern: nil for CallToolResult on success, *output for SendOutput"

patterns-established:
  - "Tool handler closure captures service reference, delegates to sendHandler for testability"
  - "Tests use fakeChannel (sendFunc closure) + real Renderer for integration-style unit tests"
  - "RFC3339 timestamp verified via time.Parse, not char-level checks"

requirements-completed:
  - "04-MCP-TOOL-REGISTRATION"
  - "04-MCP-HANDLER-TESTS"
  - "04-COMPOSITION-ROOT"

duration: 18min
completed: 2026-06-26
---

# Plan 01-04 Tasks 1&2: MCP Tool Registration & Composition Root Summary

**MCP send_notification tool registered via go-sdk v1.6.1 AddTool, handler tests with fakeChannel mock, and cmd/server/main.go composition root wiring all layers**

## Performance

- **Duration:** 18 min
- **Completed:** 2026-06-26
- **Tasks:** 2 (code only — did NOT execute Task 3 deliverability gate)
- **Files modified:** 11 (4 created, 7 modified)

## Accomplishments
- `internal/mcpserver/tool.go` — `SendInput` (no recipient field, SEC-02), `SendOutput`, `sendHandler` delegates to `svc.Notify` with D-08 success/error formatting
- `internal/mcpserver/server.go` — `NewServer(svc)` creates `mcp.Server` with `AddTool[SendInput, SendOutput]` registering `send_notification` tool (description cites fixed admin recipient per NOTIF-03)
- `internal/mcpserver/tool_test.go` — `fakeChannel` mock, `TestHandler_SuccessResult` (verifies recipient, message-id, RFC3339 in output), `TestHandler_ErrorWraps` (verifies "email send failed" wrapping)
- `cmd/server/main.go` — Composition root: config.Load → renderer/emailChannel → NotificationService → mcpserver.NewServer → mcp.NewStreamableHTTPHandler → http.ListenAndServe on cfg.Port
- Exported `NewRenderer()` and `NewEmailChannel(cfg)` for cross-package use from main/go and mcpserver tests

## Files Created/Modified
- `internal/mcpserver/tool.go` — new, 35 lines (SendInput/SendOutput/sendHandler)
- `internal/mcpserver/server.go` — new, 45 lines (NewServer with go-sdk v1.6.1 API)
- `internal/mcpserver/tool_test.go` — new, 96 lines (2 handler tests)
- `cmd/server/main.go` — new, 47 lines (composition root)
- `internal/notify/render.go` — modified: +`NewRenderer() Renderer` wrapper
- `internal/notify/email.go` — modified: +`NewEmailChannel(cfg) Channel` wrapper
- `go.mod` + `go.sum` — added go-sdk v1.6.1 dependency

## Decisions Made
- `NewStreamableHTTPHandler` takes a `func(*http.Request) *Server` factory — used a closure returning the pre-built `srv`
- `mcp.AddTool[In, Out]` auto-infers JSON schemas from Go struct types — no manual schema creation
- Tests use real `NewRenderer()` + fakeChannel mock for realistic handler testing without network

## Deviations from Plan
None — executed exactly as specified. Task 3 (live deliverability) intentionally deferred.

## Issues Encountered
- go-sdk v1.6.1 `NewStreamableHTTPHandler` requires a closure factory, not a direct server — this is the standard API pattern, just needed discovery during implementation
- `mcp.AddTool` uses Go generics with input/output type params — discovered the correct signature: `AddTool[In, Out any](s *Server, tool *Tool, h ToolHandlerFor[In, Out])`

## Human Gates Remaining
- **Plan 01-04 Task 3** (live deliverability): User needs to provide `.env` with Gmail app password, run the server, trigger the tool, and verify the email arrives in inbox (not spam). This requires a human to do the "Show original" Gmail check.
- **Plan 01-05** (devcontainer debug): User needs VS Code launch.json + .devcontainer for breakpoint debugging. Entirely interactive setup.

## Full Gate Results
```
$ go build ./...       → exit 0
$ go test ./...        → 9/9 PASS (3 config + 2 mcpserver + 4 notify + 1 email multipart)
$ go vet ./...         → clean
$ go mod verify        → all modules verified
$ govulncheck ./...    → 18 Go 1.26.0 stdlib vulns (fix: upgrade Go to ≥1.26.4)
```
- No `.env` staged
- No secrets committed
- Integration test guarded by `//go:build integration`

---
*Plan: 01-04*
*Completed: 2026-06-26*

## Post-Hardening Security Note (2026-06-26)

After initial Phase 1 completion, `govulncheck ./...` reported 18 vulnerabilities (17 Go stdlib CVEs fixable by toolchain upgrade, 1 `golang.org/x/net v0.26.0` CVE GO-2025-3595/GO-2026-4918). Applied:

- **`golang.org/x/net`** bumped from `v0.26.0` → `v0.53.0` via `go get`
- **Toolchain** pinned to `go1.26.4` via `toolchain go1.26.4` directive in `go.mod` (tests/build now run under auto-downloaded 1.26.4)
- All 5 pinned direct deps unchanged: go-sdk v1.6.1, go-mail v0.7.3, goldmark v1.8.2, bluemonday v1.0.27, caarlos0/env v11.4.1

Post-hardening gates:
```
$ go build ./...       → exit 0
$ go test ./...        → 9/9 PASS (3 config + 2 mcpserver + 4 notify)
$ go vet ./...         → clean
$ go mod verify        → all modules verified
$ govulncheck ./...    → "No vulnerabilities found" (0 code-affecting vulns; 5+2 findings in uncalled imported packages)
$ gofmt -l .           → (empty — all clean)
```

Commit: `dedc0c9` `chore(deps): bump x/net to v0.53.0 and pin go1.26.4 toolchain to clear govulncheck`
