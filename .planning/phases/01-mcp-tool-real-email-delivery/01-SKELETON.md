# Walking Skeleton — mcp-notify

**Phase:** 1
**Generated:** 2026-06-25

## Capability Proven End-to-End

An AI agent connects an MCP client to a locally-run server over Streamable HTTP, calls the single `send_notification` tool with a Markdown body, and a correctly-formatted (inline-styled, status-tagged) email lands in michael@blacktoaster.com's inbox — not spam — From the authenticated Gmail account.

## Architectural Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Language / runtime | Go 1.26 (min floor 1.25) | Constraint: server must be Go. 1.25 floor — the go-sdk Streamable HTTP security uses `http.CrossOriginProtection` (Go 1.25+) |
| MCP framework | `github.com/modelcontextprotocol/go-sdk` v1.6.1 | Official, spec-authoritative; `NewStreamableHTTPHandler` returns a plain `http.Handler`; owns protocol-version negotiation + tool-error wrapping. (Corrects CLAUDE.md's non-existent v1.7.0 stable) |
| Transport | Streamable HTTP at `/mcp` (POST + streaming GET) | Requirement: remote MCP over streamable HTTP, not stdio/SSE |
| Email | `github.com/wneessen/go-mail` v0.7.3, Gmail SMTP `smtp.gmail.com:587` STARTTLS + app password | Constraint: Gmail SMTP app password (not Gmail API). go-mail gives first-class multipart + message-id. (Corrects CLAUDE.md's v0.6.2 — API-identical) |
| Markdown → HTML | `github.com/yuin/goldmark` v1.8.2 (+ GFM) | De-facto Go CommonMark engine |
| HTML sanitization | `github.com/microcosm-cc/bluemonday` v1.0.27 (`UGCPolicy`) | Allowlist XSS scrubber. CRITICAL: sanitize the agent FRAGMENT only — UGCPolicy strips inline `style`, so the trusted inline-styled wrapper must NOT pass through it |
| Config | `github.com/caarlos0/env/v11` v11.4.1, env-first | Twelve-factor; env contract fixed by `.env.example`; pairs with ESO-synced Secrets later |
| HTTP routing | stdlib `net/http.ServeMux` | One handler (`/mcp`); no router framework justified |
| Module path | `github.com/kingoftowns/mcp-notify` | Confirmed from the repo git remote |
| Directory layout | `cmd/server` entrypoint; `internal/{config,notify,mcpserver}` | `notify` holds the Renderer/Channel/NotificationService seams; `mcpserver` holds the tool + transport |
| Seam interfaces | `Renderer`, `Channel`, `NotificationService` | Locked so Phase 2's `SecretProvider` and a future Slack `Channel` slot in without rework |
| Recipient | Hardcoded server-side from `NOTIFY_RECIPIENT` config; tool exposes no recipient param | Security posture (SEC-02) — removes the spam-relay abuse surface |

## Stack Touched in Phase 1

- [x] Project scaffold (Go module init, pinned deps, build/lint/test runner) — Plan 01-01
- [x] Routing — one real route: `/mcp` Streamable HTTP — Plan 01-04
- [x] "Data" boundary — one real write equivalent: a real email sent via Gmail SMTP — Plan 01-03 (impl) + 01-04 (live send gate)
- [x] "UI" interaction — one real MCP tool call (`send_notification`) over the wire — Plan 01-04 / 01-05
- [x] Run target — `go run ./cmd/server` + the "Debug mcp-notify" devcontainer launch config — Plan 01-04 / 01-05

## Out of Scope (Deferred to Later Slices)

> Explicit so future phases do not re-litigate Phase 1's minimalism. The `Renderer`/`Channel`/`NotificationService`/`SecretProvider` seams are left clean for these to slot into.

- Bearer-token auth on `/mcp` (Phase 2 — SEC-01)
- `SecretProvider` abstraction + ESO/Vault secret sourcing (Phase 2 config seam / Phase 4 ESO)
- Rate limiting + content-hash dedup (Phase 2 — SEC-04)
- Structured `log/slog` logging with secret redaction + graceful shutdown (Phase 2 — OPS-02/OPS-03)
- `/healthz` + `/readyz` probe endpoints (Phase 2 — OPS-01)
- Containerization / distroless image (Phase 3)
- Helm chart, Ingress, TLS Certificate (Phases 3 & 5)
- ArgoCD GitOps (Phase 6)
- Slack delivery channel (v2 — SLACK-01/02)
- Optional `source`/agent field, retry/error-class hints, emoji status tags (declined in CONTEXT.md)
- Workspace DKIM publishing for blacktoaster.com (recommended deliverability hardening, owner action, not a Phase-1 blocker)

## Subsequent Slice Plan

Each later phase adds one vertical slice on top of this skeleton without altering its architectural decisions:

- Phase 2: Bearer-guarded `/mcp`, env/secret-driven config via `SecretProvider`, dedup + rate limit, redacted structured logging, graceful shutdown
- Phase 3: Multi-stage distroless image + Helm chart deploying a healthy pod
- Phase 4: Gmail/bearer creds flow from Vault via ESO `ExternalSecret`
- Phase 5: Public NGINX ingress + cert-manager TLS with streaming intact
- Phase 6: ArgoCD GitOps-managed stack passing the full real-client (Claude Code + neo) acceptance checklist
