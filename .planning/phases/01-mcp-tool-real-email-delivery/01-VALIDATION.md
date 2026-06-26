---
phase: 1
slug: mcp-tool-real-email-delivery
status: draft
nyquist_compliant: false
wave_0_complete: false
created: 2026-06-25
---

# Phase 1 — Validation Strategy

> Per-phase validation contract for feedback sampling during execution.
> Seeded from `01-RESEARCH.md` §"Validation Architecture". Task IDs are refined post-plan.

---

## Test Infrastructure

| Property | Value |
|----------|-------|
| **Framework** | Go stdlib `testing` (+ table tests). No third-party test dep required. |
| **Config file** | none — `go test` convention |
| **Quick run command** | `go test ./internal/...` |
| **Full suite command** | `go test ./...` |
| **Real-send integration** | `go test -tags=integration ./internal/notify -run TestRealSend` (build-tagged; needs `.env`) |
| **Estimated runtime** | ~5 seconds (unit); real-send is manual/gated |

---

## Sampling Rate

- **After every task commit:** Run `go test ./internal/...` (+ `gofumpt -l`, `golangci-lint run --fast`)
- **After every plan wave:** Run `go test ./...` + `go vet ./...` + `govulncheck ./...`
- **Before `/gsd:verify-work`:** Full suite green + the manual deliverability gate (inbox-not-spam, DMARC PASS)
- **Max feedback latency:** ~5 seconds (unit suite)

---

## Per-Task Verification Map

> Requirement → behavior → test, from RESEARCH §"Phase Requirements → Test Map". Plan/wave/task IDs are assigned by the planner; the nyquist auditor binds rows to task IDs post-plan.

| Requirement | Behavior | Test Type | Automated Command | Seam | Status |
|-------------|----------|-----------|-------------------|------|--------|
| SEC-03 (HTML) | `<script>`/raw HTML in body neutralized | unit | `go test ./internal/notify -run TestRender_SanitizesScript` | Renderer (real goldmark+bluemonday) | ⬜ pending |
| SEC-03 (subject) | CRLF in subject stripped | unit | `go test ./internal/notify -run TestRender_StripsCRLF` | Renderer | ⬜ pending |
| EMAIL-03 / D-05 / D-06 | status→`[Word]` prefix; omitted→`[Info]` blue | unit | `go test ./internal/notify -run TestRender_StatusTag` | Renderer | ⬜ pending |
| EMAIL-02 / D-02 | wrapper keeps inline styles (banner/footer present) | unit | `go test ./internal/notify -run TestRender_InlineStyles` | Renderer | ⬜ pending |
| EMAIL-02 | multipart: text + HTML alternative present | unit | `go test ./internal/notify -run TestSend_Multipart` | Channel (assert on `Msg`, no real dial) | ⬜ pending |
| NOTIF-05 / D-09 | send error → tool error (`IsError`) | unit | `go test ./internal/mcpserver -run TestHandler_ErrorWraps` | mock Channel returns error | ⬜ pending |
| NOTIF-05 / D-08 | success → result contains recipient+msg-id+ts | unit | `go test ./internal/mcpserver -run TestHandler_SuccessResult` | mock Channel returns msg-id | ⬜ pending |
| NOTIF-04 | handshake echoes protocol version | smoke | curl handshake (see Manual-Only) | StreamableHTTPHandler | ⬜ pending |
| EMAIL-01 / EMAIL-04 | real email lands in inbox (not spam) | manual+integration | `go test -tags=integration ...` + Gmail "Show original" | real Channel | ⬜ pending |

*Status: ⬜ pending · ✅ green · ❌ red · ⚠️ flaky*

---

## Wave 0 Requirements

- [ ] `internal/notify/render_test.go` — stubs for SEC-03, D-02, EMAIL-03 (D-05/06), EMAIL-02
- [ ] `internal/mcpserver/tool_test.go` — stubs for NOTIF-05, D-08/D-09 (mock Channel)
- [ ] `internal/notify/email_integration_test.go` (`//go:build integration`) — EMAIL-01/04 real send
- [ ] Test mock: `fakeChannel` implementing `Channel` (canned msg-id / forced error)
- [ ] Framework install: none — stdlib `testing`

---

## Manual-Only Verifications

| Behavior | Requirement | Why Manual | Test Instructions |
|----------|-------------|------------|-------------------|
| Email lands in inbox, not spam | EMAIL-01 / EMAIL-04 | Requires live Gmail app-password creds + human inbox check | Run real-send (`-tags=integration`), open Gmail, "Show original" → confirm SPF/DMARC PASS, message in Inbox not Spam |
| MCP protocol-version handshake | NOTIF-04 | Needs a live server + MCP client/curl | Start `go run ./cmd/server`, curl `initialize` to `/mcp`, assert server echoes a supported `protocolVersion` |
| VS Code "Debug mcp-notify" breakpoints | DEVEX-01 | Interactive debugger session | Launch config in devcontainer, set breakpoint in `cmd/server`/handler, call tool, confirm breakpoint hits |

---

## Validation Sign-Off

- [ ] All tasks have `<automated>` verify or Wave 0 dependencies
- [ ] Sampling continuity: no 3 consecutive tasks without automated verify
- [ ] Wave 0 covers all MISSING references
- [ ] No watch-mode flags
- [ ] Feedback latency < 5s
- [ ] `nyquist_compliant: true` set in frontmatter

**Approval:** pending
