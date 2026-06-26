# Phase 1: MCP Tool + Real Email Delivery - Research

**Researched:** 2026-06-25
**Domain:** Go MCP server (official go-sdk, Streamable HTTP) + Gmail SMTP email delivery + markdown→HTML rendering
**Confidence:** HIGH (all four locked libraries verified against current source; deliverability verified against live DNS)

## Summary

This is a greenfield walking-skeleton phase: a single Go module exposing one MCP tool (`send_notification`) over Streamable HTTP at `/mcp`, which renders an agent-supplied markdown body to sanitized HTML, wraps it in an inline-styled email template, and sends a real multipart (plaintext + HTML) message to a fixed recipient via Gmail SMTP. The stack is already locked in `CLAUDE.md`; this research supplies the **exact current API wiring** and flags two version drifts found against the live Go module proxy.

**Two version corrections to CLAUDE.md** (both verified via `proxy.golang.org`):
1. **go-sdk: use `v1.6.1`, not `v1.7.0`.** The latest *stable* tag is `v1.6.1` (2026-05-22). Only `v1.7.0-pre.1` exists — there is **no stable v1.7.0**. The Streamable HTTP + typed-tool API described below is from v1.6.1 source and is fully sufficient.
2. **go-mail: use `v0.7.3`, not `v0.6.2`.** Latest stable is `v0.7.3` (2026-05-12). The multipart/auth/message-id API CLAUDE.md relies on is byte-for-byte identical in v0.7.3 (verified against source) — this is a safe forward bump, not a breaking change.

`goldmark v1.8.2` and `bluemonday v1.0.27` are confirmed current. `caarlos0/env/v11` → pin `v11.4.1`.

**Deliverability is a solved-but-verify situation, not a risk.** `blacktoaster.com` is a **Google Workspace domain** (MX → `aspmx.l.google.com`), and `GMAIL_USERNAME=michael@blacktoaster.com` is on that same domain — so the `From` domain, the authenticated account, and the SPF/DMARC domain all align. SPF authorizes Google (`v=spf1 include:_spf.google.com ~all`) and DMARC is `p=reject`. Mail sent through authenticated Workspace SMTP passes DMARC via SPF alignment. One gap found: **no DKIM selector is published** (`google._domainkey.blacktoaster.com` is empty) — see the Deliverability Spike section.

**Primary recommendation:** Build three pure seams — `Renderer` (markdown→sanitized-HTML + status→subject tagging), `Channel` (SMTP send), `NotificationService` (wires them) — register one typed tool via `mcp.AddTool[In,Out]`, serve it with `mcp.NewStreamableHTTPHandler` on `GET/POST /mcp`, and let the SDK's handler own protocol-version negotiation and tool-error wrapping. Sanitize **only the agent body fragment**, never the server-authored styled wrapper.

## User Constraints (from CONTEXT.md)

### Locked Decisions

**Email presentation**
- **D-01:** Render markdown→HTML body inside a **styled wrapper**: status banner (top) → body → footer. Not bare HTML.
- **D-02:** **Clean & minimal** styling — color-coded status banner, readable system-font body, thin divider, muted footer. **Inline CSS only** (email clients strip `<style>` blocks). No logo/branding.
- **D-03:** Banner color by status: **completed = green, waiting = amber, info = blue, error = red.**
- **D-04:** Footer shows a **timestamp only** (e.g. "via mcp-notify"). **No agent/sender identity.** Param set stays exactly `subject` / `body` / `status` — no `source` field.

**Status & subject format**
- **D-05:** Subject status tag is **word-only**, bracketed prefix: `[Completed]` / `[Waiting]` / `[Info]` / `[Error]` + the agent's subject. No emoji. (Supersedes the `[⏳ Waiting]` emoji sketch in ROADMAP/REQUIREMENTS EMAIL-03 — emoji was illustrative only.)
- **D-06:** When the agent **omits** `status`, default to **`info`** → `[Info]`, blue banner. Every email is tagged/colored; no untagged special-case.
- **D-07:** Subject has **CRLF stripped** before use (SEC-03) to prevent header injection.

**Tool result & errors**
- **D-08:** On **success**, return a short human-readable confirmation **plus details**: recipient, **message-id**, and **timestamp** (e.g. "Email delivered to michael@blacktoaster.com — message-id <…>, 2026-06-25T…").
- **D-09:** On **failure**, return a **clear MCP tool error** with a plain human-readable reason (e.g. "SMTP send failed: connection refused"). **No retry hint, no transient/permanent classification.**

### Claude's Discretion
- **Agent-facing tool description wording** (NOTIF-03): pick sensible default phrasing — state the recipient is fixed (agent never passes one), and that it's for task done / blocked / waiting-on-user moments. Frequency posture left unconstrained.
- **Plaintext fallback generation** (raw markdown vs. stripped HTML) and **input validation behavior** (empty subject/body handling): standard approaches fine.
- Exact banner shades, divider weight, font stack, footer timestamp format.

### Deferred Ideas (OUT OF SCOPE)
- Optional `source`/agent field — declined; keep param set locked.
- Retry guidance / transient-vs-permanent error classification — deferred to Phase 2 (after dedup + rate-limit exist).
- Emoji status tags — declined in favor of word-only.
- **Out of bounds for this phase entirely:** bearer auth (P2), secrets/ESO (P2/P4), rate-limit/dedup (P2), structured logging & graceful shutdown (P2), containerization (P3), ingress/TLS (P5), ArgoCD (P6). Leave the `Renderer`/`Channel`/`NotificationService`/`SecretProvider` seams clean — do not pull these forward.

## Phase Requirements

| ID | Description | Research Support |
|----|-------------|------------------|
| NOTIF-01 | Single `send_notification` MCP tool | `mcp.AddTool(server, &mcp.Tool{Name:"send_notification",...}, handler)` — Standard Stack §go-sdk |
| NOTIF-02 | `subject` (string), `body` (markdown), optional `status` enum | Typed input struct + `jsonschema` tags + `enum`; default via schema/Go zero-value (D-06) — Code Examples §1 |
| NOTIF-03 | Description tells agent when to call; recipient fixed | Tool `Description` field; recipient never in input struct (SEC-02) — Code Examples §1 |
| NOTIF-04 | Streamable HTTP, negotiate/echo protocol version | `mcp.NewStreamableHTTPHandler`; SDK echoes supported client version else returns `2025-11-25` — Code Examples §2, Pitfall 4 |
| NOTIF-05 | Success → confirmation result; failure → clear tool error | Handler returns `(*CallToolResult, Out, error)`; returning `error` auto-wraps to `IsError:true` (D-09) — Code Examples §4 |
| DEVEX-01 | VS Code debugger in devcontainer + local MCP client | Scaffolding already present; verify dlv + Inspector/curl — Local Dev section |
| EMAIL-01 | Gmail SMTP `:587` STARTTLS + app password | go-mail `NewClient` defaults to `TLSMandatory`/port 587 + `SMTPAuthLogin` — Code Examples §3 |
| EMAIL-02 | Markdown → HTML, multipart with plaintext fallback | goldmark `Convert` → bluemonday → `SetBodyString(TypeTextPlain)` + `AddAlternativeString(TypeTextHTML)` — Code Examples §3 |
| EMAIL-03 | `status` tags subject for inbox scannability | Word-only bracket prefix `[Completed]`… (D-05), default `[Info]` (D-06) |
| EMAIL-04 | `From` = authenticated Gmail account; fixed recipient | `From(cfg.GmailUsername)`, `To(cfg.NotifyRecipient)`; deliverability verified — Deliverability Spike |
| SEC-02 | Recipient hardcoded server-side, no tool param | Input struct has no recipient field; `To` read from config |
| SEC-03 | Sanitize rendered HTML; strip CRLF from subject | bluemonday on body fragment + CRLF strip on subject — Code Examples §1, §3, Security Domain |

## Architectural Responsibility Map

| Capability | Primary Tier | Secondary Tier | Rationale |
|------------|-------------|----------------|-----------|
| MCP protocol handshake / version negotiation | go-sdk `StreamableHTTPHandler` (HTTP server) | — | SDK owns the spec; do not hand-roll JSON-RPC |
| Tool schema + input validation | go-sdk typed tool (`AddTool[In,Out]`) | API/business logic | SDK derives JSON Schema from the Go struct and validates input before the handler runs |
| Markdown → sanitized HTML + subject tagging | `Renderer` (pure business logic) | — | No I/O; fully unit-testable; trust boundary for untrusted LLM body |
| Email composition + SMTP send | `Channel` (email impl) | — | Network boundary to Gmail; mocked in unit tests, real in the one integration check |
| Wiring (validate → render → send → format result) | `NotificationService` | — | Orchestrates seams; where the tool handler delegates |
| Config load from env | `config.Load` (`caarlos0/env`) | — | Twelve-factor; env contract fixed by `.env.example` |
| HTTP routing (`/mcp`) | stdlib `net/http.ServeMux` | — | One handler; no router framework justified |

## Standard Stack

### Core
| Library | Version | Purpose | Why Standard |
|---------|---------|---------|--------------|
| `github.com/modelcontextprotocol/go-sdk/mcp` | **v1.6.1** | MCP server + Streamable HTTP transport + typed tools | Official spec-authoritative SDK. `NewStreamableHTTPHandler` returns a plain `http.Handler`. Supports MCP spec up to `2025-11-25`. `[VERIFIED: proxy.golang.org — latest stable v1.6.1, 2026-05-22]` |
| `github.com/wneessen/go-mail` | **v0.7.3** | Gmail SMTP (STARTTLS + app password) + multipart HTML/plaintext | Actively maintained net/smtp fork. `SetBodyString` + `AddAlternativeString` for alternative bodies; `GetMessageID()` for D-08. `[VERIFIED: proxy.golang.org — latest stable v0.7.3, 2026-05-12]` |
| `github.com/yuin/goldmark` | **v1.8.2** | Markdown → HTML | De-facto Go CommonMark engine (powers Hugo). Zero non-stdlib deps. GFM extension for tables. `[VERIFIED: proxy.golang.org — latest v1.8.2, 2026-03-25]` |
| `github.com/microcosm-cc/bluemonday` | **v1.0.27** | Sanitize the agent-authored HTML fragment (SEC-02) | Allowlist XSS scrubber; `UGCPolicy()` is the recommended pairing with goldmark. `[VERIFIED: proxy.golang.org — latest v1.0.27]` |
| `github.com/caarlos0/env/v11` | **v11.4.1** | Typed struct config from env | Twelve-factor env→struct loader; fits the `.env.example` contract. `[VERIFIED: proxy.golang.org — latest v11.4.1]` |
| Go toolchain | **1.26** (min 1.25) | Language/runtime | 1.25 hard floor: SDK's Streamable HTTP security uses `http.CrossOriginProtection` (Go 1.25+). `[VERIFIED: go version go1.26.0 installed; devcontainer pins Go 1.25]` |
| stdlib `net/http` (`ServeMux`) | Go 1.26 | HTTP server, routing, `/mcp` | The whole surface is one handler; no framework justified. `[CITED: CLAUDE.md]` |

### Supporting (stdlib — no install)
| Package | Purpose | When |
|---------|---------|------|
| `html/template` | Render the inline-styled email wrapper around the sanitized body | Building the HTML part (D-01/D-02) |
| `strings` | CRLF strip on subject (SEC-03), build subject prefix | Renderer |
| `time` | Footer timestamp (D-02) + result timestamp (D-08) | Renderer/result |
| `context` | Tool handler + `DialAndSendWithContext` | Throughout |

### Alternatives Considered
| Instead of | Could Use | Tradeoff |
|------------|-----------|----------|
| go-sdk v1.6.1 | `mark3labs/mcp-go` | Pre-1.0 churn; not spec-authoritative. CLAUDE.md "What NOT to Use." |
| go-mail | stdlib `net/smtp` | Forces manual MIME boundary assembly for multipart; net/smtp is frozen. |
| `SMTPAuthLogin` | `SMTPAuthPlain` | Both work with Gmail on 587; LOGIN is the conventional Gmail choice. Pick one. |
| `caarlos0/env` | `kelseyhightower/envconfig` | Preference only; caarlos0 is more active. |

**Installation:**
```bash
go mod init github.com/<owner>/mcp-notify   # confirm module path with user
go get github.com/modelcontextprotocol/go-sdk@v1.6.1
go get github.com/wneessen/go-mail@v0.7.3
go get github.com/yuin/goldmark@v1.8.2
go get github.com/microcosm-cc/bluemonday@v1.0.27
go get github.com/caarlos0/env/v11@v11.4.1
```

## Package Legitimacy Audit

> Go modules: verified against the authoritative `proxy.golang.org` module proxy (the Go ecosystem registry). `slopcheck` is npm/PyPI-oriented and does not cover Go modules; proxy + known-authoritative-repo verification is the correct equivalent here.

| Package | Registry | Latest stable | Source Repo | Verification | Disposition |
|---------|----------|---------------|-------------|--------------|-------------|
| `modelcontextprotocol/go-sdk` | proxy.golang.org | v1.6.1 (2026-05-22) | github.com/modelcontextprotocol/go-sdk | Official MCP org; source API read directly | Approved — pin v1.6.1 |
| `wneessen/go-mail` | proxy.golang.org | v0.7.3 (2026-05-12) | github.com/wneessen/go-mail | Established maintainer; API read directly | Approved — pin v0.7.3 |
| `yuin/goldmark` | proxy.golang.org | v1.8.2 (2026-03-25) | github.com/yuin/goldmark | Powers Hugo; widely depended | Approved — pin v1.8.2 |
| `microcosm-cc/bluemonday` | proxy.golang.org | v1.0.27 (2024-07-04) | github.com/microcosm-cc/bluemonday | Long-established sanitizer | Approved — pin v1.0.27 |
| `caarlos0/env/v11` | proxy.golang.org | v11.4.1 (2026-05-01) | github.com/caarlos0/env | Established; v11 import path | Approved — pin v11.4.1 |

**Packages removed (SLOP):** none. **Flagged (SUS):** none. All five are mature, widely-depended modules verified directly against current source on their authoritative repos.

> Run `go mod verify` and `govulncheck ./...` after `go get` as a CI gate (CLAUDE.md dev tools).

## Architecture Patterns

### System Architecture Diagram

```
 MCP client (Inspector / curl / Claude Code / neo)
        │  HTTP POST/GET  (Accept: application/json, text/event-stream)
        ▼
  net/http.ServeMux
        ├── GET/POST /mcp  ──►  mcp.StreamableHTTPHandler ──► *mcp.Server
        │                                                        │ (SDK validates input vs JSON Schema,
        │                                                        │  negotiates protocol version)
        │                                                        ▼
        │                                          send_notification handler (typed In/Out)
        │                                                        │ delegates to
        │                                                        ▼
        │                                              NotificationService.Notify(in)
        │                                              │            │
        │                                   Renderer ◄─┘            └─► Channel
        │                          (goldmark→bluemonday→            (go-mail: build multipart
        │                           inline-styled wrapper;           Msg, DialAndSendWithContext
        │                           [Status] subject; CRLF strip)    to smtp.gmail.com:587 STARTTLS)
        │                                                                     │
        └── (Phase 2 adds /healthz, /readyz, bearer mw)                       ▼
                                                                   Gmail → michael@blacktoaster.com inbox
```
Config (`caarlos0/env`) loads PORT / GMAIL_USERNAME / GMAIL_APP_PASSWORD / NOTIFY_RECIPIENT at startup and is injected into `Channel`/`NotificationService`. (`BEARER_TOKEN` is read into config for the contract but is unused until Phase 2.)

### Recommended Project Structure
```
mcp-notify/
├── cmd/server/main.go        # config load, build server, register tool, ListenAndServe on PORT
├── internal/
│   ├── config/config.go      # caarlos0/env struct + Load() (the .env.example contract)
│   ├── notify/
│   │   ├── service.go         # NotificationService + Renderer + Channel interfaces; Notify()
│   │   ├── render.go          # Renderer impl: goldmark+bluemonday, wrapper template, subject tag
│   │   ├── render_test.go     # table tests: markdown, XSS body, CRLF subject, status default
│   │   ├── email.go           # Channel impl: go-mail Gmail send
│   │   └── email_integration_test.go  // build-tagged real send (//go:build integration)
│   └── mcpserver/
│       ├── server.go          # NewServer(): *mcp.Server + AddTool(send_notification)
│       └── tool.go            # Input/Output structs + handler delegating to NotificationService
├── go.mod / go.sum
├── .env.example .vscode/ .devcontainer/   # already scaffolded
```

### Pattern 1: Seam interfaces (locked by roadmap)
**What:** Define `Renderer`, `Channel`, `NotificationService` as interfaces so Phase 2's `SecretProvider` and a future Slack `Channel` slot in without rework.
```go
type Rendered struct{ Subject, HTML, Text string }

type Renderer interface {
    Render(subject, body, status string) (Rendered, error)  // status tagging + sanitize live here
}
type Channel interface {
    Send(ctx context.Context, r Rendered) (messageID string, err error)  // returns msg-id for D-08
}
type NotificationService struct {
    R Renderer
    C Channel
    Recipient string
}
```
**Why:** `Channel.Send` returning `messageID` is what makes D-08's proof-of-delivery result possible without leaking go-mail types upward. Mock `Channel` for unit tests; real go-mail impl for the one integration send.

### Pattern 2: Sanitize the fragment, not the document (CRITICAL)
**What:** bluemonday `UGCPolicy()` **strips inline `style` attributes**. The styled wrapper (D-02 inline CSS) is *server-authored and trusted*; the markdown-rendered body is *untrusted*. Order: `goldmark.Convert(body)` → `bluemonday.UGCPolicy().Sanitize(bodyHTML)` → inject the sanitized fragment into the inline-styled `html/template` wrapper. **Never** run the assembled document through bluemonday — it would erase your banner/footer styles.
**When to use:** Always, in `Renderer.Render`.

### Anti-Patterns to Avoid
- **Sanitizing the whole email HTML:** destroys D-02 inline styling. Sanitize only the agent body fragment.
- **`<style>` blocks / external CSS:** email clients strip them (D-02). Inline `style="..."` attributes only.
- **Hand-rolling MIME multipart:** use go-mail `SetBodyString` + `AddAlternativeString`.
- **Hand-rolling JSON-RPC / protocol-version logic:** the SDK owns it (Pitfall 4).
- **Adding a recipient param "for flexibility":** violates SEC-02. Recipient is config-only.
- **`html/template` auto-escaping the sanitized HTML:** inject via `template.HTML(sanitized)` so the already-sanitized fragment isn't double-escaped — bluemonday is the trust boundary, not template escaping.

## Don't Hand-Roll

| Problem | Don't Build | Use Instead | Why |
|---------|-------------|-------------|-----|
| HTML/XSS sanitization | Regex tag stripper | `bluemonday.UGCPolicy()` | Allowlist parser handles the long tail of XSS vectors |
| Markdown → HTML | Custom parser | `goldmark` + `extension.GFM` | CommonMark + GFM tables; battle-tested |
| MCP JSON-RPC + Streamable HTTP + session mgmt | Custom transport | go-sdk `NewStreamableHTTPHandler` | Spec-correct framing, version negotiation, SSE/JSON modes |
| Protocol-version negotiation | Version string matching | SDK `initialize` handling | SDK echoes supported version or returns latest |
| Multipart MIME assembly | Manual boundaries/headers | go-mail `Add/SetBodyString` | Correct boundaries, encoding, headers |
| Message-ID generation | UUID + hostname concat | go-mail `SetMessageID()` | RFC 5322-format, ~132 bits entropy |
| Env→struct config | `os.Getenv` + manual parse | `caarlos0/env` | Typed, defaults, required validators |

**Key insight:** Every "deceptively simple" piece here (sanitization, MIME, JSON-RPC framing) has a sharp edge that a hand-rolled version gets wrong. The locked stack exists precisely to avoid those.

## Common Pitfalls

### Pitfall 1: bluemonday erases the email's inline styling
**What goes wrong:** Running the full assembled email HTML through `UGCPolicy().Sanitize()` strips the `style` attributes on the banner/divider/footer (D-02), producing an unstyled email.
**Why:** `UGCPolicy` does not allowlist the `style` attribute by design.
**How to avoid:** Sanitize **only** the goldmark-rendered body fragment, then wrap it in the trusted inline-styled template (Pattern 2). Insert via `template.HTML(...)`.
**Warning signs:** Banner color missing in the delivered email; `style=` absent in source.

### Pitfall 2: `template.HTML` double-handling / double-escaping
**What goes wrong:** Passing the sanitized fragment as a plain string into `html/template` re-escapes `<`/`>` so the email shows literal tags.
**Why:** `html/template` auto-escapes `string` interpolations.
**How to avoid:** Type the field as `template.HTML` (sanitized fragment) and `template.HTMLAttr`/literal for the server-authored style values. bluemonday is the trust boundary that makes `template.HTML` safe here.

### Pitfall 3: Wrong/missing `Accept` header on the MCP client
**What goes wrong:** A raw `curl` POST to `/mcp` without `Accept: application/json, text/event-stream` gets rejected by the Streamable HTTP handler.
**Why:** The spec requires the client advertise both content types; the SDK enforces it.
**How to avoid:** Use MCP Inspector (handles this) or send both Accept types in curl (see Local Dev). Optionally set `StreamableHTTPOptions{JSONResponse: true}` for plain JSON responses during local debugging.

### Pitfall 4: Assuming you must negotiate the protocol version yourself (NOTIF-04)
**What goes wrong:** Writing version-matching code that fights the SDK.
**Why:** The SDK already handles `initialize`: if the client's `protocolVersion` is in `supportedProtocolVersions` (`2025-11-25`, `2025-06-18`, `2025-03-26`, `2024-11-05`) it echoes it; otherwise it returns the latest (`2025-11-25`). `[VERIFIED: go-sdk v1.6.1 mcp/shared.go]`
**How to avoid:** Do nothing — just assert the echoed version in the handshake check (success criterion #5). Claude Code/neo negotiate `2025-06-18`/`2025-11-25`, both supported; full real-client confirmation is deferred to Phase 6 (per STATE.md).

### Pitfall 5: DMARC `p=reject` + the From/envelope alignment
**What goes wrong:** Sending `From: michael@blacktoaster.com` through a *different* authenticated account, or via a `@gmail.com` account, would fail DMARC alignment and bounce/spam (because DMARC is `p=reject`).
**Why:** `blacktoaster.com` publishes `p=reject` — unaligned mail is rejected.
**How to avoid:** Keep `From == GMAIL_USERNAME == michael@blacktoaster.com` (EMAIL-04). Because it's a Workspace account on the SPF/DMARC domain, SPF aligns and DMARC passes. See Deliverability Spike.

### Pitfall 6: localhost Host/Origin protection in the SDK handler
**What goes wrong:** The SDK auto-enables DNS-rebinding protection for loopback servers — a request whose `Host` header isn't loopback is `403`'d.
**Why:** Security default in `StreamableHTTPHandler.ServeHTTP`. `[VERIFIED: go-sdk v1.6.1 mcp/streamable.go]`
**How to avoid:** Connect via `http://localhost:8080/mcp` or `http://127.0.0.1:8080/mcp` (both loopback — fine). `CrossOriginProtection` is **off by default** (only enabled when an internal env flag is set), so MCP Inspector's `Origin` header won't be blocked in local dev. If needed, `StreamableHTTPOptions{DisableLocalhostProtection: true}` exists as an escape hatch.

## Code Examples

> All signatures verified against `go-sdk v1.6.1` and `go-mail v0.7.3` source on 2026-06-25.

### §1 — Tool input/output structs + registration (NOTIF-01/02/03, SEC-02, D-05/06)
```go
// Source: go-sdk v1.6.1 README + mcp/tool.go (AddTool, ToolHandlerFor)
type SendInput struct {
    Subject string `json:"subject" jsonschema:"the email subject line"`
    Body    string `json:"body" jsonschema:"the notification body in Markdown"`
    // optional; default handled in code (D-06). NOTE: NO recipient field (SEC-02).
    Status  string `json:"status,omitempty" jsonschema:"one of completed|waiting|info|error; defaults to info,enum=completed,enum=waiting,enum=info,enum=error"`
}
type SendOutput struct {
    Message string `json:"message"` // human-readable proof-of-delivery (D-08)
}

func NewServer(svc *notify.NotificationService) *mcp.Server {
    s := mcp.NewServer(&mcp.Implementation{Name: "mcp-notify", Version: "v0.1.0"}, nil)
    mcp.AddTool(s, &mcp.Tool{
        Name:        "send_notification",
        Description: "Email Michael a notification when a task is done, blocked, or " +
            "waiting on him. The recipient is fixed — never pass a recipient. " +
            "Provide a short `subject`, a Markdown `body`, and optionally `status` " +
            "(completed|waiting|info|error; default info).",
    }, sendHandler(svc))
    return s
}
```
> `mcp.AddTool[In,Out]` derives the JSON Schema from the struct tags and validates incoming arguments before your handler runs. The exact `enum`/default tag syntax should be confirmed at build time against the SDK's `jsonschema` package — if the inline `enum=` tag form doesn't take, set `mcp.Tool.InputSchema` explicitly or validate `status` in the handler (D-06 default-to-info is trivial in Go regardless). `[ASSUMED: precise jsonschema tag syntax for enum/default]`

### §2 — Streamable HTTP wiring on /mcp (NOTIF-04)
```go
// Source: go-sdk v1.6.1 mcp/streamable.go (NewStreamableHTTPHandler)
srv := NewServer(svc)
handler := mcp.NewStreamableHTTPHandler(
    func(r *http.Request) *mcp.Server { return srv }, // one shared server instance
    nil, // &mcp.StreamableHTTPOptions{JSONResponse: true} for easier local curl debugging
)
mux := http.NewServeMux()
mux.Handle("/mcp", handler) // SDK handles both POST and the streaming GET
// Phase 2 wraps /mcp in bearer mw and adds /healthz, /readyz — leave room.
log.Fatal(http.ListenAndServe(":"+cfg.Port, mux))
```

### §3 — Renderer + go-mail Channel (EMAIL-01/02/04, SEC-03, D-01..D-03, D-08)
```go
// --- Renderer.Render (markdown → sanitized fragment → inline-styled wrapper) ---
// Source: goldmark v1.8.2 README; bluemonday v1.0.27 UGCPolicy
var md = goldmark.New(goldmark.WithExtensions(extension.GFM))
var policy = bluemonday.UGCPolicy()

func (r *renderer) Render(subject, body, status string) (Rendered, error) {
    if status == "" { status = "info" }            // D-06
    label, color := tagFor(status)                  // D-03/D-05: [Info], blue, etc.
    subject = stripCRLF(subject)                    // SEC-03 / D-07
    fullSubject := "[" + label + "] " + subject     // D-05 word-only prefix

    var buf bytes.Buffer
    if err := md.Convert([]byte(body), &buf); err != nil { return Rendered{}, err }
    safeFrag := policy.Sanitize(buf.String())       // sanitize FRAGMENT ONLY (Pattern 2)

    html := wrap(template.HTML(safeFrag), color, time.Now()) // inline-styled banner+body+footer (D-01/D-02)
    text := body                                    // plaintext fallback = raw markdown (discretion)
    return Rendered{Subject: fullSubject, HTML: html, Text: text}, nil
}

func stripCRLF(s string) string { // SEC-03
    return strings.NewReplacer("\r", "", "\n", "").Replace(s)
}

// --- Channel.Send (go-mail Gmail SMTP) ---
// Source: go-mail v0.7.3 client.go / msg.go / encoding.go (verified)
func (c *emailChannel) Send(ctx context.Context, r Rendered) (string, error) {
    m := mail.NewMsg()
    if err := m.From(c.from); err != nil { return "", err }      // = GMAIL_USERNAME (EMAIL-04)
    if err := m.To(c.recipient); err != nil { return "", err }   // fixed (SEC-02)
    m.Subject(r.Subject)
    m.SetMessageID()                                             // generate now so we can report it (D-08)
    m.SetBodyString(mail.TypeTextPlain, r.Text)                  // plaintext part (EMAIL-02)
    m.AddAlternativeString(mail.TypeTextHTML, r.HTML)            // HTML alternative (EMAIL-02)

    client, err := mail.NewClient("smtp.gmail.com",
        mail.WithSMTPAuth(mail.SMTPAuthLogin),                   // Gmail app password
        mail.WithUsername(c.username),
        mail.WithPassword(c.appPassword),
        // default: TLSMandatory → STARTTLS on port 587 (EMAIL-01). WithPort(587) to be explicit.
    )
    if err != nil { return "", err }
    if err := client.DialAndSendWithContext(ctx, m); err != nil {
        return "", err                                          // D-09: surfaced verbatim as tool error
    }
    return m.GetMessageID(), nil                                 // D-08 proof-of-delivery
}
```
> Verified: `DefaultTLSPolicy = TLSMandatory` and port 587 is used for `TLSMandatory` — so STARTTLS-on-587 is the default. `SetMessageID()` produces `<random@hostname>`; call it **before** send, read via `GetMessageID()` after. `TypeTextPlain`/`TypeTextHTML` live in `encoding.go`. `[VERIFIED: go-mail v0.7.3 source]`

### §4 — Tool handler: success result + auto error-wrapping (NOTIF-05, D-08/D-09)
```go
// Source: go-sdk v1.6.1 mcp/server.go — a returned (non-jsonrpc) error is auto-wrapped
// into a CallToolResult{IsError: true}; you do NOT build the error result yourself.
func sendHandler(svc *notify.NotificationService) mcp.ToolHandlerFor[SendInput, SendOutput] {
    return func(ctx context.Context, req *mcp.CallToolRequest, in SendInput) (*mcp.CallToolResult, SendOutput, error) {
        msgID, ts, err := svc.Notify(ctx, in.Subject, in.Body, in.Status)
        if err != nil {
            // D-09: plain reason; SDK wraps as tool error (IsError=true). No retry hint.
            return nil, SendOutput{}, fmt.Errorf("email send failed: %w", err)
        }
        msg := fmt.Sprintf("Email delivered to %s — message-id %s, %s",
            svc.Recipient, msgID, ts.Format(time.RFC3339))      // D-08 proof-of-delivery
        return nil, SendOutput{Message: msg}, nil               // SDK fills Content from structured output
    }
}
```
> Verified behavior (`mcp/server.go` lines ~341–353): handler `error` that is **not** a `*jsonrpc.Error` → `CallToolResult.SetError(err)` with `IsError=true`. A `*jsonrpc.Error` would surface as a protocol error instead. For D-09 (a tool-level failure the agent should see), return a **plain** `error` — exactly what the example does.

## Deliverability Spike (EMAIL-04 / open question)

**Status: PASS expected — verified against live DNS on 2026-06-25.** `blacktoaster.com` is a Google Workspace domain and the sender is on that same domain, so the historical "From alignment when GMAIL_USERNAME differs from the domain" worry **does not apply** here.

| Check | Live result (`dig @8.8.8.8`) | Verdict |
|-------|------------------------------|---------|
| MX | `aspmx.l.google.com` + alts | Google Workspace domain `[VERIFIED: dig]` |
| SPF | `v=spf1 include:_spf.google.com ~all` | Google authorized to send `[VERIFIED: dig]` |
| DMARC | `v=DMARC1; p=reject; rua=mailto:dmarc-rejects@blacktoaster.com` | **Strict** — unaligned mail rejected `[VERIFIED: dig]` |
| From alignment | `From` = `GMAIL_USERNAME` = `michael@blacktoaster.com` = SPF/DMARC domain | Aligned `[VERIFIED: env contract + dig]` |
| DKIM (`google._domainkey`) | **empty** (also default/selector1/2/s1/dkim — all empty) | **Gap** `[VERIFIED: dig]` |

**Why it should land in inbox-not-spam:** Authenticated Workspace SMTP send → envelope-from in `blacktoaster.com` → **SPF aligns** → DMARC `p=reject` satisfied via SPF alignment even without aligned DKIM. Google reputation + a real Workspace account further help inbox placement.

**The one gap — DKIM not published.** No custom DKIM selector is published for `blacktoaster.com`. Without it, Google signs outbound with its shared `*.gappssmtp.com` key, which does **not** align with `blacktoaster.com`, so DMARC is passing on SPF alignment alone. That's adequate for direct send, but fragile (e.g., if a recipient auto-forwards, SPF breaks and there's no aligned DKIM to fall back on). **Recommended (cheap, owner action, not code):** enable DKIM in Google Admin console (Apps → Google Workspace → Gmail → Authenticate email → generate key) and publish the `google._domainkey.blacktoaster.com` TXT record. This is a deliverability hardening item, **not a blocker** for Phase 1's success criterion.

**How to verify the real test email (the gate before Phase 1 is "done"):**
1. Send a real notification via the tool to `michael@blacktoaster.com`.
2. In Gmail, open the message → **Show original**. Confirm: `SPF: PASS`, `DMARC: PASS`, and the message is in **Inbox**, not Spam.
3. Confirm `From:` shows `michael@blacktoaster.com` and the subject carries the `[Status]` prefix.
4. (If DKIM was enabled per above) confirm `DKIM: PASS` with `d=blacktoaster.com`.
5. Send one `error`-status and one `[Completed]` to eyeball banner colors render in Gmail web + mobile.

**Gotcha:** Gmail SMTP requires an **app password** (account has 2FA). A normal account password will fail auth. The 16-char app password goes in `GMAIL_APP_PASSWORD` (`.env`, gitignored).

## Local Dev (DEVEX-01 / success criterion #6)

Scaffolding already exists and is **verified, not created** in plan 01-05:
- `.vscode/launch.json` — **"Debug mcp-notify"** config: `mode: debug`, `program: ${workspaceFolder}/cmd/server`, env mapped from the `.env` contract, `dlvFlags: ["--check-go-version=false"]` (so delve doesn't balk on Go 1.26 vs devcontainer's 1.25). Breakpoints in `cmd/server` + `internal/.../*.go` will hit once those files exist. `[VERIFIED: read .vscode/launch.json]`
- `.devcontainer/` — `devcontainer.json` (Go 1.25 feature, forwards 8080, installs air), `docker-compose.yml` (mounts repo, `env_file: ../.env required:false`, exposes 8080), `Dockerfile` (Go 1.25 base + delve + golangci-lint + Node/Claude Code). `[VERIFIED: read all three]`
- `.env.example` → copy to `.env` (gitignored), fill `GMAIL_APP_PASSWORD` + `BEARER_TOKEN`. Compose loads it; launch.json references it. `[VERIFIED: read .env.example]`

**Connecting a local MCP client to `http://localhost:8080/mcp`:**
```bash
# MCP Inspector (Node present in devcontainer) — handles Accept headers + handshake UI
npx @modelcontextprotocol/inspector   # then set transport=Streamable HTTP, URL=http://localhost:8080/mcp

# Raw curl handshake (must advertise BOTH content types — Pitfall 3)
curl -i http://localhost:8080/mcp \
  -H 'Content-Type: application/json' \
  -H 'Accept: application/json, text/event-stream' \
  -d '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{
        "protocolVersion":"2025-06-18","capabilities":{},
        "clientInfo":{"name":"curl","version":"0"}}}'
# Assert the response echoes "protocolVersion":"2025-06-18" (or returns "2025-11-25" if unsupported) — NOTIF-04 / criterion #5
```
> `@modelcontextprotocol/inspector` is the official MCP debugging client `[ASSUMED: exact npm package/invocation — confirm at use]`. The curl handshake is the reliable, dependency-free fallback that also satisfies the protocol-version-echo check.

## Validation Architecture

> `workflow.nyquist_validation: true` (config.json) — section included.

### Test Framework
| Property | Value |
|----------|-------|
| Framework | Go stdlib `testing` (+ table tests). No third-party test dep needed; `testify` optional and not required. |
| Config file | none — `go test` convention |
| Quick run command | `go test ./internal/...` |
| Full suite command | `go test ./...` |
| Real-send integration | `go test -tags=integration ./internal/notify -run TestRealSend` (build-tagged; needs `.env`) |

### Phase Requirements → Test Map
| Req | Behavior | Test Type | Command | Seam |
|-----|----------|-----------|---------|------|
| SEC-03 (HTML) | `<script>`/raw HTML in body is neutralized | unit | `go test ./internal/notify -run TestRender_SanitizesScript` | Renderer (real goldmark+bluemonday) |
| SEC-03 (subject) | CRLF in subject stripped | unit | `go test ./internal/notify -run TestRender_StripsCRLF` | Renderer |
| D-05/D-06 | status→`[Word]` prefix; omitted→`[Info]` blue | unit | `go test ./internal/notify -run TestRender_StatusTag` | Renderer |
| D-02 | wrapper keeps inline styles (banner/footer present) | unit | `go test ./internal/notify -run TestRender_InlineStyles` | Renderer |
| EMAIL-02 | multipart: text + HTML alternative present | unit | `go test ./internal/notify -run TestSend_Multipart` | Channel (assert on `Msg`, no real dial) or mock |
| NOTIF-05/D-09 | send error → tool error (`IsError`) | unit | `go test ./internal/mcpserver -run TestHandler_ErrorWraps` | mock Channel returns error |
| NOTIF-05/D-08 | success → result contains recipient+msg-id+ts | unit | `go test ./internal/mcpserver -run TestHandler_SuccessResult` | mock Channel returns msg-id |
| NOTIF-04 | handshake echoes protocol version | smoke | curl handshake (Local Dev) | StreamableHTTPHandler |
| EMAIL-01/04 | real email lands in inbox (not spam) | manual+integration | `go test -tags=integration ...` + Gmail "Show original" | real Channel |

### Sampling Rate
- **Per task commit:** `go test ./internal/...` (+ `gofumpt -l`, `golangci-lint run --fast`)
- **Per wave merge:** `go test ./...` + `go vet ./...` + `govulncheck ./...`
- **Phase gate:** full suite green + the manual deliverability gate (inbox-not-spam, DMARC PASS) before `/gsd:verify-work`

### What to assert / how to mock
- **Renderer:** pure — assert on returned `Rendered` strings (no I/O). This is where SEC-03, D-02, D-05/06 live. Highest-value, fastest tests.
- **Channel unit:** inject a fake SMTP or assert on the constructed `*mail.Msg` (parts, headers) without dialing — go-mail can write a message to an `io.Writer` for inspection. Avoid real network in unit tests.
- **NotificationService / handler:** use a **mock `Channel`** (returns canned msg-id, or an error) + the **real `Renderer`** to test D-08 success formatting and D-09 error wrapping deterministically.
- **One real send:** build-tagged `integration` test, skipped unless `.env` creds + `-tags=integration` present, asserting `Send` returns a non-empty message-id. The actual inbox-not-spam verdict is a manual step (Deliverability Spike checklist).

### Wave 0 Gaps
- [ ] `internal/notify/render_test.go` — REQ SEC-03, D-02, D-05/06, EMAIL-02
- [ ] `internal/mcpserver/tool_test.go` — REQ NOTIF-05, D-08/D-09 (mock Channel)
- [ ] `internal/notify/email_integration_test.go` (`//go:build integration`) — REQ EMAIL-01/04 real send
- [ ] Test mock: `fakeChannel` implementing `Channel` (canned msg-id / forced error)
- Framework install: none — stdlib `testing`.

## Security Domain

> `security_enforcement` not present in config.json → enabled (absent = enabled). Note: bearer auth (SEC-01) is **Phase 2**, not this phase. The Phase-1 security surface is input handling on an LLM-authored body + fixed recipient.

### Applicable ASVS Categories
| ASVS Category | Applies | Standard Control (this phase) |
|---------------|---------|-------------------------------|
| V2 Authentication | no (Phase 2) | Bearer token deferred to P2 |
| V3 Session Management | no | SDK owns MCP session ids |
| V4 Access Control | yes (partial) | Fixed recipient, no `to` param (SEC-02) — removes abuse surface |
| V5 Input Validation / Output Encoding | **yes** | bluemonday `UGCPolicy` on body fragment; CRLF strip on subject; SDK JSON-Schema validation of tool args |
| V6 Cryptography | no (don't hand-roll) | TLS to Gmail handled by go-mail STARTTLS; no crypto authored here |

### Known Threat Patterns for {Go MCP server + LLM-authored email}
| Pattern | STRIDE | Standard Mitigation |
|---------|--------|---------------------|
| `<script>`/XSS in markdown body | Tampering / Elevation | `bluemonday.UGCPolicy().Sanitize()` on the fragment (SEC-03) |
| Email header injection via CRLF in subject | Tampering | `stripCRLF(subject)` before use (SEC-03/D-07) |
| Arbitrary recipient / spam relay | Spoofing / Abuse | Recipient is config-only; tool exposes no recipient (SEC-02) |
| Sender spoofing / DMARC reject | Spoofing | `From == GMAIL_USERNAME` on the DMARC domain (EMAIL-04); SPF aligned |
| Secret leakage (app password) | Info Disclosure | `.env` gitignored; never logged (structured-logging redaction is Phase 2 — until then, do not log the password) |
| DNS-rebinding / cross-origin on `/mcp` | Spoofing | SDK localhost protection (on) + optional `CrossOriginProtection` |
| App-password auth failure path | DoS (self) | Surface SMTP error verbatim as tool error (D-09); no retry loop (dedup/rate-limit is P2) |

## Environment Availability

| Dependency | Required By | Available | Version | Fallback |
|------------|------------|-----------|---------|----------|
| Go toolchain | build/test (min 1.25) | ✓ | 1.26.0 (host); 1.25 (devcontainer) | — |
| Gmail SMTP `smtp.gmail.com:587` | EMAIL-01 | ✓ (reachable) | — | — |
| Gmail **app password** | EMAIL-01 auth | ✗ (secret, not in repo) | — | none — user must supply in `.env` |
| `dig` (DNS/deliverability checks) | EMAIL-04 spike | ✓ | system | `nslookup`/`host` also present |
| Node/npm (MCP Inspector) | DEVEX-01 client | ✓ in devcontainer (Dockerfile installs Node LTS) | LTS | curl handshake (Pitfall 3) |
| `delve` (dlv) | DEVEX-01 debug | ✓ in devcontainer | latest | — |
| `golangci-lint`, `gofumpt`, `govulncheck` | CI/lint | ✓ devcontainer (lint); install gofumpt/govulncheck as needed | — | `go vet` |

**Missing with no fallback:** Gmail app password — owner action; without it the real-send gate (success criterion #2) cannot pass. **Missing with fallback:** none blocking — MCP Inspector substitutes with curl.

## State of the Art

| Old Approach | Current Approach | When Changed | Impact |
|--------------|------------------|--------------|--------|
| `mark3labs/mcp-go` (community) | Official `modelcontextprotocol/go-sdk` v1.x | 2025–2026, stable v1 | Spec-authoritative, semver-stable; use this (CLAUDE.md) |
| MCP SSE transport | Streamable HTTP | MCP spec 2025-03-26+ | Use `NewStreamableHTTPHandler`; SSE is legacy |
| stdlib `net/smtp` (frozen) | `wneessen/go-mail` | ongoing | First-class multipart + maintained auth |

**Deprecated/outdated:** SSE transport (use Streamable HTTP); CLAUDE.md's `v1.7.0`/`v0.6.2` pins (use v1.6.1 / v0.7.3 — see Assumptions/Summary).

## Assumptions Log

| # | Claim | Section | Risk if Wrong |
|---|-------|---------|---------------|
| A1 | Inline `jsonschema:"enum=...,..."` tag syntax produces a proper enum + default for `status` | Code Examples §1 | LOW — falls back to validating/defaulting `status` in handler code; D-06 trivial in Go either way |
| A2 | `@modelcontextprotocol/inspector` is the current npm package/invocation for the MCP client | Local Dev | LOW — curl handshake is the verified fallback that satisfies criterion #5 |
| A3 | DMARC passes via SPF alignment alone for authenticated Workspace send (no aligned DKIM) | Deliverability Spike | MEDIUM — if mail spams, enable Workspace DKIM (already the recommended hardening); verified via "Show original" in the gate |
| A4 | go-mail default (`TLSMandatory`) selects port 587 without explicit `WithPort` | Code Examples §3 | LOW — source confirms; add `WithPort(587)` to be explicit and remove all doubt |

## Open Questions (RESOLVED)

1. **Go module path** — `go mod init github.com/<owner>/mcp-notify` owner/path not yet fixed.
   - Known: single module at repo root, `cmd/server` entrypoint.
   - **RESOLVED:** `github.com/kingoftowns/mcp-notify` — confirmed from the repo's git remote (`github.com:kingoftowns/mcp-notify`). Baked into Plan 01-01 Task 1 and 01-SKELETON.md.
2. **Enable Workspace DKIM for blacktoaster.com?** (deliverability hardening)
   - Known: SPF aligns, DMARC passes via SPF; DKIM selector not published.
   - **RESOLVED:** Not a Phase-1 blocker. SPF alignment carries DMARC `p=reject`; DKIM is owner-side hardening to enable in Admin console later. Confirmed via "Show original" at the 01-04 deliverability gate.
3. **Protocol version both real clients negotiate** — Claude Code and neo.
   - Known: SDK supports `2025-11-25 / 2025-06-18 / 2025-03-26 / 2024-11-05`; both clients use versions in this set.
   - **RESOLVED:** Local handshake check now (01-04); full real-client confirmation deferred to Phase 6 (per STATE.md).

## Sources

### Primary (HIGH confidence)
- `proxy.golang.org` — version/date verification for all five modules (go-sdk v1.6.1, go-mail v0.7.3, goldmark v1.8.2, bluemonday v1.0.27, caarlos0/env v11.4.1)
- go-sdk **v1.6.1** source: `mcp/tool.go` (AddTool, ToolHandlerFor), `mcp/streamable.go` (NewStreamableHTTPHandler, options, ServeHTTP localhost/origin protection), `mcp/server.go` (error→IsError wrapping), `mcp/shared.go` (protocol version negotiation), `README.md` (typed-tool example)
- go-mail **v0.7.3** source: `client.go` (NewClient, WithSMTPAuth/Username/Password, TLSMandatory default, DialAndSendWithContext), `msg.go` (SetBodyString, AddAlternativeString, SetMessageID/GetMessageID, From/To/Subject), `auth.go` (SMTPAuth constants), `encoding.go` (TypeTextPlain/TypeTextHTML)
- goldmark **v1.8.2** `README.md` (goldmark.New, extension.GFM, Convert); `extension/gfm.go` confirmed present
- bluemonday **v1.0.27** `README.md` (UGCPolicy, Sanitize, style-attr not allowed)
- Live DNS via `dig @8.8.8.8` — blacktoaster.com MX/SPF/DMARC + DKIM selector probes
- Repo files: `CLAUDE.md`, `.env.example`, `.vscode/launch.json`, `.devcontainer/*`, `.planning/{REQUIREMENTS,ROADMAP,STATE}.md`, `01-CONTEXT.md`, `.planning/config.json`

### Secondary (MEDIUM confidence)
- MCP Inspector usage pattern (npx `@modelcontextprotocol/inspector`) — official tooling convention; not re-verified this session (A2)

### Tertiary (LOW confidence)
- Exact `jsonschema` enum/default tag rendering in the SDK's schema generator (A1) — confirm at build time

## Metadata

**Confidence breakdown:**
- Standard stack: HIGH — every module + version + API verified against current source and the Go module proxy
- Architecture / wiring: HIGH — handler, tool registration, error semantics, multipart, message-id all read from source
- Deliverability: HIGH on the DNS facts (live `dig`); MEDIUM on the "passes via SPF alignment" inference (A3) — gated by the manual "Show original" check
- Pitfalls: HIGH — each tied to a verified source behavior (bluemonday style-strip, SDK localhost/version logic, Accept-header requirement)

**Research date:** 2026-06-25
**Valid until:** ~2026-07-25 (go-sdk and go-mail are fast-moving — re-check the proxy if planning slips a month; a stable v1.7.0 may land)
