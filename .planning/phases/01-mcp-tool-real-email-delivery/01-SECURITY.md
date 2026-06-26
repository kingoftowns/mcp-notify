---
phase: 1
slug: mcp-tool-real-email-delivery
status: verified
threats_open: 0
asvs_level: 1
created: 2026-06-26
---

# Phase 1 — Security

> Per-phase security contract: threat register, accepted risks, and audit trail.
> **Result: SECURED — 10/10 threats closed, 0 open.** Register authored at plan time
> (PLAN.md threat models 01-01..01-04); this audit verifies each declared mitigation
> exists in implemented code (verify-only, no new-threat scan). Verified by gsd-security-auditor.

---

## Trust Boundaries

| Boundary | Description | Data Crossing |
|----------|-------------|---------------|
| MCP client → `/mcp` | Untrusted HTTP clients call the tool over Streamable HTTP | Agent-supplied subject/body/status |
| LLM-authored body/subject → email | Untrusted Markdown/subject become HTML body + SMTP headers | Potential script/HTML/CRLF injection |
| env → process | Secrets (Gmail app password, bearer token) enter via env vars | Credentials |
| process → Gmail SMTP | Outbound authenticated TLS carrying the app password | Credentials + message |
| go module proxy → build | Third-party dependency code enters the build | Code (supply chain) |

---

## Threat Register

| Threat ID | Category | Component | Disposition | Mitigation (verified evidence) | Status |
|-----------|----------|-----------|-------------|--------------------------------|--------|
| T-01-01 | Tampering/Elevation | render.go markdown→HTML | mitigate | `render.go:27,75` `bluemonday.UGCPolicy().Sanitize(rawHTML)` on the goldmark FRAGMENT only; wrapper assembled after (`:94`), never re-sanitized | closed |
| T-01-02 | Tampering | render.go subject | mitigate | `render.go:62-64` `strings.NewReplacer("\r","","\n","")` strips CRLF before the `[Status]` prefix (SEC-03/D-07) | closed |
| T-01-03 | Spoofing/Abuse | recipient sourcing | mitigate | `tool.go:13-17` `SendInput` has no recipient field; `service.go:23` `Send` takes no recipient; `email.go:28,39` recipient = `cfg.NotifyRecipient` only (SEC-02) | closed |
| T-01-04 | Spoofing | From alignment | mitigate | `email.go:36` `msg.From(ch.username)` == `cfg.GmailUsername` (`:27`), same as SMTP auth user → SPF/DMARC aligned (EMAIL-04) | closed |
| T-01-05 | Information Disclosure | config secrets | mitigate | No `%+v`/`String()` dump of `Config` (grep clean); password only to `email.go:56` `mail.WithPassword`; `main.go` logs port + error wrapper only | closed |
| T-01-06 | Spoofing | StreamableHTTPHandler | mitigate | `main.go:31-34` `NewStreamableHTTPHandler` used as-is (nil opts) → SDK localhost/DNS-rebinding/Origin protection intact; bearer auth correctly absent (Phase 2) | closed |
| T-01-07 | Tampering | template.HTML injection | mitigate | `render.go:89` `template.HTML(sanitizedFragment)` — sole conversion, input is the sanitized fragment from `:75` | closed |
| T-01-08 | DoS (self) | SMTP failure path | accept | `email.go:63-65` single `DialAndSendWithContext`, no retry loop; documented in Accepted Risks Log | closed |
| T-01-09 | Repudiation/Info | tool error path | mitigate | `tool.go:29` wrapped error (SDK `IsError`); `:32-34` success returns message-id + RFC3339, no secrets | closed |
| T-01-SC | Tampering (supply chain) | go module installs | mitigate | `go.mod` exact pins; `go mod verify` → "all modules verified"; `govulncheck ./...` → "No vulnerabilities found" (0 code-affecting); `x/net v0.53.0`, `toolchain go1.26.4` | closed |

*Status: open · closed*
*Disposition: mitigate (implementation required) · accept (documented risk) · transfer (third-party)*

---

## Accepted Risks Log

| Risk ID | Threat Ref | Rationale | Accepted By | Date |
|---------|------------|-----------|-------------|------|
| AR-01 | T-01-08 | SMTP send failures surface verbatim with no retry/backoff. Phase 1 is single-shot best-effort delivery; the agent sees the error and decides. Dedup + rate-limit + retry are explicitly Phase-2 work, and blast radius is one fixed recipient. | Michael (owner) | 2026-06-26 |

*Revisit AR-01 in Phase 2 (rate-limit/dedup).*

---

## Security Audit Trail

| Audit Date | Threats Total | Closed | Open | Run By |
|------------|---------------|--------|------|--------|
| 2026-06-26 | 10 | 10 | 0 | gsd-security-auditor (opus) |

---

## Phase-2 Deferred Controls (informational — NOT Phase-1 gaps)

Bearer-token auth, rate-limiting, dedup, structured-log redaction, and health/readiness probes are explicitly deferred to Phase 2 per phase scope. Their absence is by design and was not assessed as a Phase-1 threat.

## Audit Notes

- `go.mod:10` pins `modelcontextprotocol/go-sdk v1.6.1` (CLAUDE.md text references the non-existent v1.7.0). Not a threat — pinned + `go mod verify`-reproducible — flagged for maintainer awareness only.
- Supply-chain claim independently re-confirmed live: `go mod verify` passes and `govulncheck ./...` reports 0 code-affecting vulnerabilities.

---

## Sign-Off

- [x] All threats have a disposition (mitigate / accept / transfer)
- [x] Accepted risks documented in Accepted Risks Log
- [x] `threats_open: 0` confirmed
- [x] `status: verified` set in frontmatter

**Approval:** verified 2026-06-26
