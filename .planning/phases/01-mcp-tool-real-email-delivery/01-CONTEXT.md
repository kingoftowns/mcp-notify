# Phase 1: MCP Tool + Real Email Delivery - Context

**Gathered:** 2026-06-25
**Status:** Ready for planning

<domain>
## Phase Boundary

A locally-run MCP server whose `send_notification` tool renders a markdown body and lands a correctly-formatted email in michael@blacktoaster.com's inbox. Delivered without any cluster: `go run ./cmd/server`, connect a local MCP client, call the tool, verify the email arrives (not spam).

**In scope:** the `send_notification` tool (subject + markdown body + optional status, no recipient param), markdown→HTML rendering with sanitization, status→subject tagging, multipart HTML+plaintext email via Gmail SMTP, the official go-sdk `StreamableHTTPHandler` on `/mcp`, and the local devcontainer + VS Code debug experience.

**Out of bounds (later phases):** bearer auth (P2), secrets/ESO (P2/P4), rate-limit/dedup (P2), structured logging & graceful shutdown (P2), containerization (P3), ingress/TLS (P5), ArgoCD (P6). Do not pull these forward — but leave the `Renderer` / `Channel` / `NotificationService` seams clean so they slot in later.
</domain>

<decisions>
## Implementation Decisions

### Email presentation
- **D-01:** Render the markdown→HTML body inside a **styled wrapper**, not bare HTML. Structure: status banner (top) → body → footer.
- **D-02:** **Clean & minimal** styling. A color-coded status banner, readable system-font body, a thin divider, and a muted footer. **Inline CSS only** (email clients strip `<style>` blocks). No logo/wordmark/branding — keep it intentional but low-effort for the POC.
- **D-03:** Banner color maps to status: **completed = green, waiting = amber, info = blue, error = red.**
- **D-04:** Footer shows a **timestamp only** (e.g. "via mcp-notify"). **No agent/sender identity** — the locked param set (`subject` / `body` / `status`) stays exactly as-is; no `source`/agent field added in this phase.

### Status & subject format
- **D-05:** Subject status tag is **word-only**, bracketed prefix: `[Completed]` / `[Waiting]` / `[Info]` / `[Error]` followed by the agent's subject. No emoji (avoids cross-client rendering quirks). NOTE: this supersedes the `[⏳ Waiting]` emoji sketch in ROADMAP/REQUIREMENTS (EMAIL-03) — emoji was illustrative; word-only is the decision.
- **D-06:** When the agent **omits** `status`, default to **`info`** → subject prefixed `[Info]`, blue banner. Every email is consistently tagged and color-coded; no untagged/neutral special-case.
- **D-07:** Subject still has CRLF stripped before use (SEC-03, already locked) to prevent header injection.

### Tool result & errors
- **D-08:** On **success**, the tool returns a short human-readable confirmation **plus details**: recipient, **message-id**, and **timestamp** (e.g. "Email delivered to michael@blacktoaster.com — message-id <…>, 2026-06-25T…"). Gives the agent and transcript proof of actual delivery.
- **D-09:** On **failure**, return a **clear MCP tool error** with a plain human-readable reason (e.g. "SMTP send failed: connection refused"). **No retry hint and no transient/permanent classification** — keep failure reporting simple. Retry-safety (dedup + rate limit) is explicitly a Phase 2 concern; do not encourage agents to loop on failures before those protections exist.

### Claude's Discretion
- **Agent-facing tool description wording** (the "when to call me" guidance for neo/Claude Code, NOTIF-03): not deep-dived. Research/planning may pick sensible default phrasing — state that the recipient is fixed (so the agent never passes one) and that it's for task done / blocked / waiting-on-user moments. The user opted not to constrain the exact frequency posture.
- **Plaintext fallback generation** (from raw markdown vs. stripped HTML) and **input validation behavior** (empty subject/body handling): standard approaches fine.
- Exact banner shades, divider weight, font stack, and footer timestamp format.
</decisions>

<canonical_refs>
## Canonical References

**Downstream agents MUST read these before planning or implementing.**

### Tech stack & wiring patterns
- `CLAUDE.md` — recommended stack table (official `modelcontextprotocol/go-sdk` v1.7.0, `wneessen/go-mail` v0.6.2, `goldmark` v1.8.2, `bluemonday`), the "Streamable HTTP + Bearer Auth Wiring" pattern, and the "What NOT to Use" list. The authoritative source for library choices in this phase.

### Local dev experience (already scaffolded — model on these)
- `.env.example` — the fixed env-var contract: `PORT=8080`, `GMAIL_USERNAME`, `GMAIL_APP_PASSWORD`, `BEARER_TOKEN`, `NOTIFY_RECIPIENT`. Config loader must read these.
- `.devcontainer/devcontainer.json`, `.devcontainer/docker-compose.yml`, `.devcontainer/Dockerfile` — devcontainer the server runs/debugs in.
- `.vscode/launch.json` — the "Debug mcp-notify" launch config (DEVEX-01 success criterion #6).
- `/Users/michael/_code/SpinWheel/.devcontainer` and `/Users/michael/_code/SpinWheel/.vscode/launch.json` — the proven pattern this scaffolding was mirrored from; consult if adjusting dev tooling.

### Requirements & roadmap
- `.planning/REQUIREMENTS.md` — Phase 1 requirements: NOTIF-01..05, DEVEX-01, EMAIL-01..04, SEC-02, SEC-03.
- `.planning/ROADMAP.md` §"Phase 1" — goal, 6 success criteria, the 5 drafted plans (01-01..01-05), and the deliverability open question.

</canonical_refs>

<code_context>
## Existing Code Insights

### Reusable Assets
- **Scaffolding only — no Go code yet** (no `go.mod`). Phase 1 writes the application from scratch.
- `.env.example` + `.devcontainer/` + `.vscode/launch.json` already exist (committed during init) — DEVEX plan (01-05) verifies rather than creates these.

### Established Patterns
- **Interface seams locked in the roadmap:** `Renderer` (markdown→HTML + status tagging), `Channel` (email send), `NotificationService` (wiring). Build against these so Phase 2's `SecretProvider` and a future Slack `Channel` slot in without rework.
- **Env-var config contract** is fixed by `.env.example` — `caarlos0/env`-style struct load expected (per CLAUDE.md).
- Single Go module at repo root, `cmd/server` entrypoint, server listens on `PORT` (8080), MCP at `/mcp`.

### Integration Points
- MCP client (Inspector/curl) → `http://localhost:8080/mcp` (streamable HTTP handshake, protocol-version echo).
- `Channel` → `smtp.gmail.com:587` STARTTLS with app password; `From` = `GMAIL_USERNAME`, `To` = `NOTIFY_RECIPIENT` (hardcoded server-side, no tool param).

</code_context>

<specifics>
## Specific Ideas

- "Clean & minimal" email = a color-coded banner + system-font body + thin divider + muted timestamp footer. Think functional status notification, not a marketing email.
- Status labels are the four enum words in brackets; banner color is the only color signal. Inbox scannability comes from the `[Word]` subject prefix, not emoji.
- Success result should read like proof-of-delivery (includes message-id) so the user can trust it in agent transcripts.

</specifics>

<deferred>
## Deferred Ideas

- **Optional `source`/agent field** (so the footer/result can say which agent — neo vs claude-code — sent it): considered and declined for Phase 1 to keep the param set locked. Revisit if telling callers apart becomes useful (would be a small tool-schema addition).
- **Retry guidance / transient-vs-permanent error classification:** deferred to align with Phase 2's dedup + rate-limit protections, after which encouraging retries is safe.
- **Emoji status tags:** declined in favor of word-only; could revisit if word tags feel insufficiently glanceable.

</deferred>

---

*Phase: 1-MCP Tool + Real Email Delivery*
*Context gathered: 2026-06-25*
