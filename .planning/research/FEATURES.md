# Feature Research

**Domain:** Agent-to-human notification MCP server (email channel, Go, hosted in k8s)
**Researched:** 2026-06-25
**Confidence:** HIGH (MCP tool design, email mechanics, operational features verified against official MCP spec, Go SDK docs, and transactional-email guidance; channel-abstraction is design judgment, MEDIUM)

## Context Recap

This is a single-purpose tool, not a generic email service. One agent (neo or Claude Code) calls
one tool to reach one fixed human (michael@blacktoaster.com). The product's whole value is
"delivery just works." That framing is what separates table stakes from anti-features here:
anything that widens the surface (arbitrary recipients, attachments, scheduling) is a *liability*,
not a missing feature. The interesting engineering is in (a) making the agent call the tool at the
right moments with the right shape, and (b) making the resulting inbox experience scannable and
spam-resistant.

## Feature Landscape

### Table Stakes (Without These It Isn't Usable)

Features the product cannot ship without. Missing any of these = broken or not credible as a hosted service.

| Feature | Why Expected | Complexity | Notes |
|---------|--------------|------------|-------|
| Single `send_notification` MCP tool | The entire product surface; one verb, one job | LOW | Go SDK `mcp.AddTool[In,Out]` infers JSON schema from a typed struct. Keep to ONE tool — more tools = more agent confusion. |
| Input schema: `subject` (string, req), `body` (markdown string, req), `status` (enum, optional) | Matches PROJECT requirements; minimal yet expressive | LOW | `status` enum: `completed` \| `waiting` \| `info` \| `error`. Constrain via JSON Schema `enum` so the agent can't invent values. Default `info` if omitted. |
| Tool description engineered for correct invocation | Agents only call tools well when the description tells them *when* and *why* | MEDIUM | Lead with one-sentence purpose, then explicit "call this when…" triggers (task done, blocked, needs input), state the recipient is FIXED (so the agent doesn't try to pass one), and give a 1-line example. Tools with full descriptions show 3–4x fewer failed invocations. |
| Tool annotations set correctly | Hosts (Claude Code) use these for safety + parallelism decisions | LOW | `readOnlyHint:false`, `destructiveHint:false` (sending mail is additive, not destructive), `idempotentHint:true` *only if* dedup is implemented (otherwise false), `openWorldHint:true` (hits external SMTP). Defaults are pessimistic, so set them explicitly. |
| Status tag in subject line | Inbox scannability — "waiting" must be visible without opening | LOW | Prefix subject with a status token/emoji, e.g. `[✅ Done] …`, `[⏳ Waiting] …`, `[❌ Error] …`, `[ℹ︎] …`. This is the single highest-value UX touch for an agent inbox. Depends on `status` field. |
| Markdown body → HTML render | Agents author markdown naturally; humans want formatted email | LOW–MEDIUM | Use a maintained Go markdown lib (e.g. goldmark) + sanitize output (bluemonday). Sanitization is mandatory because the body originates from an LLM (see anti-features). |
| Multipart/alternative (HTML + plaintext) | Standards-correct mail; plaintext fallback improves deliverability & trust | LOW | Always send both parts. Plaintext = the raw/markdown source; HTML = rendered. Plaintext-present mail is more trusted in transactional contexts. |
| Sane `From` / `Reply-To` | Gmail rewrites `From` to the authenticated mailbox; mismatches break delivery | LOW | `From` MUST equal the authenticated Gmail account. Set `Reply-To` to the user so replies are natural. Set a clear display name like `Agent Notifier`. |
| `Message-ID` on every send | Required for proper threading/dedup/debugging; servers expect it | LOW | Generate `<uuid@mcp-notify.k8s.blacktoaster.com>`. Foundation for both threading and dedup features below. |
| Bearer-token auth on HTTP transport | Endpoint is publicly exposed via ingress; must be guarded | LOW | Validate `Authorization: Bearer <token>` on every request; constant-time compare; return 401 with a meaningful error. Token from Vault/ESO. |
| Streamable HTTP transport | PROJECT constraint; hosted long-lived k8s service | LOW | Go SDK `NewStreamableHTTPHandler`. Note recent SDK versions add cross-origin protection (Content-Type checks) — keep enabled. |
| Health + readiness probes | k8s needs them to route traffic and restart cleanly | LOW | `/healthz` (liveness, cheap) + `/readyz` (readiness; can verify SMTP reachability/config presence). Separate from the MCP path. |
| Structured logging | Operability: must see who called, what status, success/failure | LOW | Use `log/slog` (stdlib). Log tool calls, send outcome, dedup/rate-limit decisions. Never log full body at info level (PII). |
| Graceful shutdown | k8s sends SIGTERM on rollout; in-flight sends must finish | LOW | Trap SIGTERM, stop accepting new requests, drain, `http.Server.Shutdown(ctx)` with a timeout. |
| Request size limit | Prevent a runaway agent from posting a multi-MB body | LOW | `http.MaxBytesReader` + a body-length cap in the tool handler. Reject oversized with a clear error the agent can act on. |
| Meaningful tool result back to agent | Agent must know if the notice was delivered or refused | LOW | Return structured output: `{delivered, message_id, deduped, rate_limited, reason}`. Errors must be actionable text, not stack traces, so the agent can retry or stop. |

### Differentiators (Make It Feel Solid, Not Just Functional)

Not strictly required for a POC to "work," but each materially improves the agent + inbox experience and is cheap relative to value.

| Feature | Value Proposition | Complexity | Notes |
|---------|-------------------|------------|-------|
| Idempotency / dedup of repeated sends | Agents retry; loops happen. Prevents duplicate inbox spam | MEDIUM | Two viable triggers: (a) optional `idempotency_key` the agent passes, or (b) content hash (subject+body+status) within a TTL window. In-memory TTL map is enough for a single replica; only needs a shared store if you scale replicas. Lets you honestly set `idempotentHint:true`. |
| Basic rate limiting | A confused agent in a loop must not bury the inbox | MEDIUM | Token-bucket per server (e.g. N sends / rolling window). On limit, return a structured "rate_limited" result rather than silently dropping, so the agent learns to back off. In-memory for single replica. |
| Threading related notifications | Group a task's updates ("started"→"waiting"→"done") into one Gmail thread | MEDIUM | Optional `thread_key`; derive a deterministic root `Message-ID` from it and set `In-Reply-To`/`References` on subsequent sends. Big inbox-tidiness win. Depends on Message-ID. Requires light state (key→root-id) or a deterministic hash so no storage is needed. |
| Prometheus metrics | Observability for a hosted service; matches k8s norms | LOW–MEDIUM | `/metrics`: counters for sends by status, dedup hits, rate-limit hits, SMTP errors; latency histogram. Cheap with `prometheus/client_golang`. |
| Channel abstraction (`Notifier` interface) | Makes Slack a drop-in second channel later without rework | LOW (now) | Define `Notifier{ Send(ctx, Notification) (Receipt, error) }` with `EmailNotifier` as the only impl in v1. See "Slack Forward-Compat" section. Building this seam now is near-free; retrofitting later is not. |
| Structured tool output schema | Lets the agent reason about the receipt (id, deduped, etc.) | LOW | Go SDK supports typed `Out` with inferred output schema + `StructuredContent`. Strictly better than returning a plain string. |
| `List-Unsubscribe` / minimal header hygiene | Improves Gmail-to-Gmail deliverability, avoids spam foldering | LOW | Even single-recipient transactional mail benefits from clean headers (Date, Message-ID, MIME-Version, a stable `From` display). Low effort, real deliverability payoff. |
| Config-driven status→subject formatting | Tweak emoji/prefix without code change | LOW | Map status→prefix in config/values.yaml. Lets the inbox convention evolve without redeploys of logic. |

### Anti-Features (Deliberately NOT Building)

Each of these looks reasonable but widens blast radius, increases complexity, or contradicts the "one fixed recipient, delivery just works" thesis.

| Feature | Why Requested | Why Problematic | Alternative |
|---------|---------------|-----------------|-------------|
| Agent-specified arbitrary recipients | "Let the agent email anyone" feels flexible | Turns the tool into an open relay / phishing & exfil vector; an LLM could be prompt-injected into emailing attackers | Recipient is FIXED in config. Not a tool parameter. (Already a PROJECT decision.) |
| Attachments / file uploads | "Send the report as a PDF" | Large payloads, malware-scanning, storage, MIME complexity, request-size blowup — all for a notify tool | Put links in the markdown body (to artifacts the agent already hosts). |
| Raw HTML body from the agent | "Richer formatting" | The body comes from an LLM = untrusted input → stored XSS / spoofed content / mail-injection if header-bearing | Accept markdown only; render server-side; sanitize HTML output (bluemonday). Never pass agent HTML through verbatim. |
| Scheduling / delayed / recurring send | "Remind me in 2 hours" | Requires a scheduler, persistence, timezone logic, retry semantics — a whole subsystem orthogonal to "notify now" | Out of scope (PROJECT). If ever needed, it's the agent's job to call again later, not the server's. |
| Templating engine beyond markdown | "Branded templates, variables" | Template injection surface, more config, no POC value for a personal notifier | Markdown→HTML is the ceiling. (PROJECT decision.) |
| Full OAuth 2.1 / Dynamic Client Registration | It's the MCP spec's recommended remote-auth path | Massive overkill for a single internal caller-set behind ingress; weeks of work, no security gain at this scale | Static bearer token from Vault is appropriate for a private single-tenant internal service. Document the deliberate deviation. (PROJECT decision.) |
| Inbound / reply handling / two-way chat | "Let me reply to the agent by email" | Requires inbound mail parsing, mailbox polling, identity mapping — a different product entirely | One-way notifications only. Replies go to a human (Reply-To), not back into the agent. |
| Inbox-reading or other tools | "While we're here, let the agent read mail too" | Read access = data exfiltration surface; also dilutes the single-purpose tool and confuses agents | Keep exactly ONE tool. Resist surface growth. |
| Multi-tenant / per-agent recipient routing | "Support multiple users later" | YAGNI for a personal POC; adds auth-to-recipient mapping, config sprawl | Single fixed recipient. Revisit only if the product genuinely multi-tenants. |
| Persistent message queue / guaranteed delivery | "Never lose a notification" | Durable queue + retry infra is heavy; SMTP send is already synchronous and reported back to the agent | Synchronous send + clear success/failure result. Agent retries on failure (idempotency makes that safe). |

## Feature Dependencies

```
send_notification tool (core)
    ├──requires──> Tool description + annotations (correct invocation)
    ├──requires──> Input schema (subject/body/status)
    │                   └──enables──> Status subject-tag  (needs status enum)
    ├──requires──> Markdown→HTML render
    │                   └──requires──> HTML sanitization (untrusted LLM input)
    │                   └──enables──> Multipart HTML+plaintext
    ├──requires──> Message-ID generation
    │                   ├──enables──> Threading (In-Reply-To/References)
    │                   └──enables──> Dedup (Message-ID / content hash)
    ├──enhanced-by──> Idempotency/dedup ──unlocks──> idempotentHint:true
    ├──enhanced-by──> Rate limiting
    └──delivered-via──> Notifier interface ──future-impl──> SlackNotifier

Operational layer (independent, all required):
    Bearer auth · Health/readiness · Structured logging · Graceful shutdown · Request-size limit
    Metrics ──enhances──> all of the above (observability)
```

### Dependency Notes

- **Status subject-tag requires the `status` enum field:** the tag is derived from status; no status → no tag.
- **Multipart requires markdown render:** the HTML part is the rendered body; plaintext part is the markdown source.
- **HTML render requires sanitization:** body is LLM-authored = untrusted; rendering without sanitizing is the XSS anti-feature.
- **Threading and dedup both require Message-ID:** generate it centrally so both features build on the same identifier.
- **`idempotentHint:true` depends on dedup existing:** don't advertise idempotency you haven't implemented, or hosts/agents will wrongly assume safe blind retries.
- **Notifier interface enables Slack with no email rework:** the email path must implement the same interface v1 ships, so v2 only adds an impl + routing.

## MVP Definition

### Launch With (v1 — the POC)

Everything in **Table Stakes** plus two cheap-but-high-value differentiators (dedup + rate limit)
because both directly protect the inbox from agent loops, which is the most likely real-world failure.

- [ ] `send_notification` tool (subject + markdown body + status enum) — the product
- [ ] Tool description + annotations tuned for correct agent invocation — without this agents misfire
- [ ] Status → subject-line tagging — the core inbox-UX win
- [ ] Markdown→HTML + sanitize + multipart/plaintext — credible, safe email
- [ ] Sane From/Reply-To + Message-ID — deliverability + correctness
- [ ] Gmail SMTP send with structured success/failure result — delivery + agent feedback
- [ ] Bearer auth, health/readiness, structured logging, graceful shutdown, request-size limit — hosted-service baseline
- [ ] Dedup (content-hash or idempotency key, in-memory TTL) — stops duplicate spam from retries
- [ ] Basic rate limit (in-memory token bucket) — stops loop spam
- [ ] `Notifier` interface seam (email is the only impl) — near-free Slack forward-compat

### Add After Validation (v1.x)

- [ ] Prometheus `/metrics` — add once the service is live and you want trend visibility
- [ ] Threading via `thread_key` → In-Reply-To/References — add once you see multi-update task flows cluttering the inbox
- [ ] `List-Unsubscribe`/header hygiene tuning — add if Gmail spam-foldering appears
- [ ] Config-driven status→subject map — add when you want to tweak conventions without redeploy

### Future Consideration (v2+ / next milestone)

- [ ] Slack channel (`SlackNotifier` impl + channel routing) — explicitly the next milestone, pending Slack app approval
- [ ] Per-channel preference / multi-channel fan-out — only after a second channel exists
- [ ] Shared dedup/rate-limit store (Redis) — only if you scale beyond one replica

## Feature Prioritization Matrix

| Feature | User Value | Implementation Cost | Priority |
|---------|------------|---------------------|----------|
| `send_notification` tool + schema | HIGH | LOW | P1 |
| Tool description + annotations | HIGH | LOW–MED | P1 |
| Status subject-tagging | HIGH | LOW | P1 |
| Markdown→HTML + sanitize + multipart | HIGH | LOW–MED | P1 |
| From/Reply-To + Message-ID | HIGH | LOW | P1 |
| Bearer auth | HIGH | LOW | P1 |
| Health/readiness probes | HIGH | LOW | P1 |
| Structured logging | MED | LOW | P1 |
| Graceful shutdown | MED | LOW | P1 |
| Request-size limit | MED | LOW | P1 |
| Structured tool result | HIGH | LOW | P1 |
| Dedup / idempotency | HIGH | MED | P1 |
| Rate limiting | HIGH | MED | P1 |
| Notifier interface seam | MED (now) / HIGH (later) | LOW | P1 |
| Prometheus metrics | MED | LOW–MED | P2 |
| Threading | MED | MED | P2 |
| Header hygiene / List-Unsubscribe | MED | LOW | P2 |
| Config-driven subject map | LOW | LOW | P3 |
| Slack channel | HIGH (next milestone) | MED | P2 (next milestone) |

**Priority key:** P1 = launch · P2 = add when possible · P3 = nice to have

## Slack Forward-Compatibility (Design Note, Out of Scope for v1)

Slack is the next milestone but it should shape v1's internal seam so it slots in with zero email rework.

**The abstraction:** a single channel-agnostic interface the tool handler depends on.

```go
type Notification struct {
    Subject string
    Body    string // markdown (canonical source)
    Status  Status // completed | waiting | info | error
    // threadKey, idempotencyKey, etc.
}

type Receipt struct {
    Channel   string
    MessageID string
    Deduped   bool
}

type Notifier interface {
    Send(ctx context.Context, n Notification) (Receipt, error)
}
```

**Why this keeps email and Slack interchangeable:**
- The MCP tool handler builds a channel-neutral `Notification` (markdown body, status) and never touches SMTP directly.
- `EmailNotifier` renders markdown→HTML, maps status→subject prefix, sends via Gmail SMTP.
- A future `SlackNotifier` renders the SAME markdown→Slack mrkdwn/Block Kit, maps status→emoji/color, posts to a webhook/API. Markdown-as-canonical-body is what makes both renderers possible from one input.
- Dedup, rate-limit, and validation live ABOVE the interface (in the handler), so they apply to every channel for free.
- Channel selection (later) is config/routing, not a new tool — the agent still calls one `send_notification`. This preserves the single-tool principle even as channels multiply.

Keep this seam thin in v1: one interface, one implementation. Do NOT build channel routing,
multi-channel fan-out, or per-channel config until the second channel actually lands — that's the
anti-feature trap. The interface is the cheap insurance; the routing machinery is the deferred cost.

## Competitor / Prior-Art Feature Analysis

| Feature | General notifier libs (e.g. nikoksr/notify, Symfony Notifier, Flux notification-controller) | Transactional email APIs (Postmark, MailerSend, MailPace) | Our Approach |
|---------|----------------|----------------|--------------|
| Channel abstraction | Core feature (many channels) | N/A (email only) | Thin interface, email-only v1, Slack next |
| Idempotency/dedup | Rarely built-in | Idempotency-Key header is a standard offering | Content-hash or key, in-memory TTL |
| Multipart HTML+text | Varies | Standard | Always both; plaintext = markdown source |
| Threading headers | Usually ignored | Supported via headers | Optional thread_key → In-Reply-To/References |
| Rate limiting | App's responsibility | Provider-side limits | App-side token bucket (protect inbox) |
| Fixed recipient | No (arbitrary by design) | No (arbitrary by design) | YES — fixed by design (key differentiator/constraint) |
| Single MCP tool surface | N/A | N/A | One tool, one verb — deliberate minimalism |

The notable divergence from all prior art: this product *intentionally removes* the recipient-choice
flexibility that general notifiers and email APIs treat as fundamental. That constraint is the
security posture, not a missing feature.

## Sources

MCP tool design & annotations:
- [Writing effective tools for AI agents — Anthropic Engineering](https://www.anthropic.com/engineering/writing-tools-for-agents) (HIGH)
- [Tool Annotations — Model Context Protocol Blog](https://blog.modelcontextprotocol.io/posts/2026-03-16-tool-annotations/) (HIGH)
- [MCP tool descriptions: best practices — Merge.dev](https://www.merge.dev/blog/mcp-tool-description) (MEDIUM)
- [MCP Tool Annotations Explained — ChatForest](https://chatforest.com/guides/mcp-tool-annotations-explained/) (MEDIUM)

Go MCP SDK (capabilities for tools/annotations/structured output/streamable HTTP):
- [modelcontextprotocol/go-sdk — GitHub](https://github.com/modelcontextprotocol/go-sdk) (HIGH)
- [go-sdk design.md — AddTool / typed handlers / structured output](https://github.com/modelcontextprotocol/go-sdk/blob/main/design/design.md) via Context7 (HIGH)
- [mcp package — pkg.go.dev](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp) (HIGH)

Email mechanics:
- [Transactional email best practices — Postmark](https://postmarkapp.com/guides/transactional-email-best-practices) (MEDIUM)
- [Email headers in transactional emails — Zoho ZeptoMail](https://www.zoho.com/zeptomail/articles/email-header.html) (MEDIUM)
- [Email threading: In-Reply-To and References — LobsterMail](https://lobstermail.ai/blog/email-threading-explained-how-in-reply-to-and-references-headers-keep-conversations-together) (MEDIUM)
- [Idempotent email API — MailPace](https://mailpace.com/features/idempotent-email-api) (MEDIUM)
- [Go send email via Gmail — Mailtrap](https://mailtrap.io/blog/golang-send-email-gmail/) (MEDIUM)
- [Gmail SMTP settings & From-rewrite behavior — GMass](https://www.gmass.co/blog/gmail-smtp/) (MEDIUM)

Operational features for hosted MCP:
- [Remote MCP servers: hosting, auth & best practices — kapa.ai](https://www.kapa.ai/blog/remote-mcp-servers-hosting-authentication-best-practices) (MEDIUM)
- [Understanding Authorization in MCP — modelcontextprotocol.io](https://modelcontextprotocol.io/docs/tutorials/security/authorization) (HIGH)
- [MCP gateways for rate limiting & access control — MintMCP](https://www.mintmcp.com/blog/mcp-gateways-rate-limiting-access-control) (MEDIUM)

Channel abstraction prior art:
- [nikoksr/notify (multi-channel Go notifier)](https://github.com/nikoksr/notify) (MEDIUM)
- [Symfony Notifier component](https://symfony.com/doc/current/notifier.html) (MEDIUM)
- [Flux notification-controller notifier package](https://pkg.go.dev/github.com/fluxcd/notification-controller/internal/notifier) (MEDIUM)

---
*Feature research for: agent-to-human notification MCP server (email channel)*
*Researched: 2026-06-25*
