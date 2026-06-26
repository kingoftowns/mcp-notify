# Project Research Summary

**Project:** mcp-notify
**Domain:** Go remote MCP server (streamable HTTP) -- Gmail email notification service on Kubernetes
**Researched:** 2026-06-25
**Confidence:** HIGH (core stack, architecture, and pitfalls verified against official sources; minor version pins and MCP protocol-version negotiation are MEDIUM)

## Executive Summary

mcp-notify is a single-purpose, single-tool remote MCP server: an AI agent calls send_notification(subject, body, status) and a formatted email lands in the user inbox. The entire product surface is one tool, one fixed recipient, one delivery channel. Research across all four dimensions converges strongly on approach: use the **official github.com/modelcontextprotocol/go-sdk v1.7.0** (not the community mark3labs/mcp-go, which is still pre-1.0), wire its NewStreamableHTTPHandler as a plain http.Handler into a stdlib ServeMux, and build the domain behind two thin Go interfaces (Channel and Renderer) so the email implementation is swappable without touching the MCP transport layer. Go 1.25 is the hard minimum floor -- the official SDK Streamable HTTP security uses http.CrossOriginProtection added in 1.25; build on 1.26. The container must be distroless/static (not scratch) because outbound TLS to smtp.gmail.com requires CA certificates that only distroless includes.

The recommended stack for email delivery is wneessen/go-mail over smtp.gmail.com:587 STARTTLS with an app password, composing multipart text/html + text/plain messages. Markdown bodies from LLM agents must be rendered with yuin/goldmark and then sanitized with microcosm-cc/bluemonday before being placed in the HTML part -- treating agent-authored content as untrusted is non-negotiable for a publicly exposed endpoint. Secret management is clean: the Go binary never imports a Vault or AWS SDK. ESO materializes Vault secrets into a Kubernetes Secret; the app reads env vars or mounted files. Swapping to AWS Secrets Manager later is a pure ESO SecretStore config change with zero Go code change. A thin SecretProvider interface stays in Go only as a testing seam.

The dominant risk cluster is infrastructure, not application code. NGINX proxy buffering will silently break MCP streaming unless the ingress annotation proxy-buffering: "off" and the response header X-Accel-Buffering: no are both set. The ArgoCD Application must carry ignoreDifferences on the managed Secret /data before self-heal is enabled, or ArgoCD will perpetually revert ESO populated values. The build order is the key mitigation: complete all pure-Go work and validate it locally (steps 1-4) before touching the cluster -- this isolates infra problems from application logic.

---

## Key Findings

### Recommended Stack

The single most important call in the stack research: **use github.com/modelcontextprotocol/go-sdk v1.7.0, not mark3labs/mcp-go**. The official SDK reached stable v1.7.0 (co-maintained with Google), ships NewStreamableHTTPHandler returning a plain http.Handler, includes built-in session management and http.CrossOriginProtection, and tracks the MCP spec authoritatively. mark3labs/mcp-go is at v0.55.1 with no semver guarantees -- unacceptable for a greenfield 2026 project. There is no router framework needed: the MCP handler + /healthz + /metrics + bearer auth wrapper are the entire HTTP surface; stdlib http.ServeMux covers it cleanly.

For email, wneessen/go-mail v0.6.2 is the correct choice: it handles STARTTLS correctly, supports SetBodyString + AddAlternativeString for multipart HTML+plaintext, and is the most actively maintained modern Go mail library. Avoid net/smtp directly -- multipart assembly is error-prone and net/smtp is frozen. For config, caarlos0/env/v11 is the recommended loader: typed struct, env-first, minimal, twelve-factor compliant.

**Core technologies:**
- github.com/modelcontextprotocol/go-sdk v1.7.0 -- MCP server + Streamable HTTP transport -- official SDK, http.Handler interface, CrossOriginProtection, stable v1
- github.com/wneessen/go-mail v0.6.2 -- Gmail SMTP delivery, multipart HTML+plaintext -- STARTTLS-correct, maintained, no net/smtp footguns
- github.com/yuin/goldmark v1.8.2 -- markdown to HTML rendering -- CommonMark-compliant, zero non-stdlib deps, GFM extensions
- github.com/microcosm-cc/bluemonday latest v1.0.x -- HTML sanitization -- mandatory; agent body is untrusted LLM output
- github.com/caarlos0/env/v11 -- typed config from environment -- env-first k8s config, required/default validators
- Go 1.26 (minimum 1.25) -- runtime -- hard floor: 1.25 adds http.CrossOriginProtection used by the SDK
- gcr.io/distroless/static-debian13:nonroot -- container runtime -- includes CA certs for outbound Gmail TLS; scratch breaks SMTP

### Expected Features

The entire product is one tool. Everything that widens the blast radius or dilutes the single-purpose thesis is an anti-feature.

**Must have (table stakes -- v1 ships with all of these):**
- send_notification tool: subject (required), body (markdown, required), status (enum: completed/waiting/info/error, optional, default info)
- Status tag in subject line ([DONE], [WAITING], [ERROR], [INFO]) -- the core inbox-scannability win
- Markdown to HTML render + bluemonday sanitization + multipart HTML+plaintext -- safe, standards-correct email
- From = authenticated Gmail account; Reply-To = user; Message-ID on every send -- deliverability and correctness
- Recipient hardcoded server-side (michael@blacktoaster.com); no to param in schema -- this is a security guarantee, not a convention
- Bearer-token auth on /mcp only; /healthz and /metrics unauthenticated
- /healthz (liveness) and /readyz (readiness) probes -- k8s baseline
- Structured tool result back to agent: {delivered, message_id, deduped, rate_limited, reason}
- Graceful shutdown on SIGTERM -- k8s rollouts must not drop in-flight SMTP sends
- Request size limit via http.MaxBytesReader
- Structured logging via log/slog; never log Authorization header or SMTP auth credentials

**Should have (differentiators -- include in v1):**
- Dedup by content hash (subject+body+status) with in-memory TTL -- stops duplicate spam from agent retries; enables idempotentHint:true
- Basic rate limit (in-memory token bucket) -- stops agent loop spam; single-replica assumption keeps this simple
- Notifier/Channel interface seam -- near-free Slack forward-compat; the email impl is the only impl in v1

**Defer to v1.x / v2+:**
- Prometheus /metrics (ServiceMonitor) -- add once service is live and trend visibility is wanted
- Email threading via thread_key to In-Reply-To/References -- add when multi-update task flows clutter the inbox
- List-Unsubscribe header hygiene -- add if Gmail spam-foldering appears
- SlackNotifier impl + channel routing -- next milestone, pending Slack app approval
- Shared dedup/rate-limit store (Redis) -- only if scaling beyond one replica

**Deliberate anti-features (never build):**
- to param in tool schema -- open relay/exfil vector
- Raw HTML body from agent -- XSS injection into email
- Attachments, scheduling, inbox-reading, multi-tenant routing

### Architecture Approach

The Go application follows a ports-and-adapters (hexagonal-lite) pattern with two key swappable seams: Channel (email now, Slack later) and Renderer (goldmark now), both consumed by a domain NotificationService. The MCP tool handler is transport-only -- it decodes args and delegates immediately to NotificationService, never touching go-mail or goldmark directly. The composition root (cmd/server/main.go) wires all concrete implementations. Bearer auth is a middleware wrapping only the /mcp route; health and metrics are open on the same single listener. The secret-backend swap lives in ESO SecretStore configuration, not in Go code; the SecretProvider interface in Go exists only for unit testing and as a non-ESO escape hatch.

The Kubernetes resource set models the frontend chart from the KubernetesTracker repo: Deployment (envFrom the ESO-materialized Secret, non-root, read-only rootfs), Service (ClusterIP), Ingress (nginx, routes /mcp externally, proxy-buffering annotation required), explicit Certificate (vault-issuer ClusterIssuer -- cert-manager does NOT auto-issue from ingress annotations in this cluster), ExternalSecret (vault-backend, version-pinned), ServiceAccount (automountServiceAccountToken: false). The ArgoCD Application uses the emporia-style single Application pattern pointing at path: helm, with ignoreDifferences on the ESO-managed Secret /data.

**Major components:**
1. cmd/server/main.go -- composition root: load config, build deps, wire mux, start server + graceful shutdown
2. internal/transport/mcphttp -- mcp.Server registration and StreamableHTTPHandler wiring (SDK boundary)
3. internal/notify -- NotificationService: subject tagging, orchestrate render+send; depends only on interfaces
4. internal/render -- Renderer interface + goldmark+bluemonday impl
5. internal/channel/email -- Channel interface impl via go-mail, Gmail SMTP
6. internal/auth -- bearer token middleware (constant-time compare)
7. internal/config + internal/secrets -- typed config from env; SecretProvider (env and file impls)
8. internal/httpserver -- mux assembly (/mcp, /healthz, /metrics), timeouts, graceful shutdown
9. helm/ -- Deployment, Service, Ingress, Certificate, ExternalSecret, ServiceAccount, ArgoCD Application

### Critical Pitfalls

1. **NGINX proxy buffering kills MCP streaming** -- Set ingress annotation nginx.ingress.kubernetes.io/proxy-buffering: "off" and have the Go server emit X-Accel-Buffering: no on stream responses. Also set proxy-read-timeout and proxy-send-timeout to 3600. Without this, tool calls hang ~60s then fail; the connection looks alive at the TLS layer, making this hard to diagnose. Verify with curl -N through the public hostname before declaring the endpoint working.

2. **ArgoCD vs ESO fight over Secret /data** -- Only commit the ExternalSecret to git, never the resolved Secret. Add ignoreDifferences on the Secret /data in the ArgoCD Application before enabling auto-sync/self-heal. Without this, ArgoCD perpetually reverts ESO populated values and can blank the Secret, breaking auth and SMTP.

3. **ESO ExternalSecret version pin requires manual bump to rotate** -- This cluster convention pins a version in the ExternalSecret (intentional, for auditability). Rotating the Gmail app password or bearer token in Vault does NOT automatically resync ESO. Runbook: write new secret to Vault, bump version in ExternalSecret, ESO resyncs k8s Secret, restart Deployment (or use checksum annotation). Document this explicitly.

4. **Gmail SMTP port/TLS mismatch and From-address rejection** -- Use port 587 STARTTLS (not 465). From must equal the authenticated Gmail account (michael@blacktoaster.com); Gmail silently rewrites or rejects a mismatched From. Use go-mail, not raw net/smtp, to avoid manual STARTTLS/TLS mode errors.

5. **MCP protocol-version negotiation against real clients** -- The spec has had multiple revisions (2025-03-26, 2025-06-18, 2025-11-25, 2026-07 pending). Verify that Claude Code and neo both successfully list and call the tool -- not just curl. The official SDK handles version negotiation; let it own session lifecycle rather than hand-rolling anything.

---

## Implications for Roadmap

The build order from ARCHITECTURE.md is the roadmap backbone. Steps 1-4 (all pure Go) are locally testable with no cluster access. Steps 5-9 form a strict dependency chain: image to chart to secrets to ingress to ArgoCD. Grouping by this boundary gives clean, independently demonstrable milestones.

### Phase 1: Go Core -- MCP + Email + Auth

**Rationale:** Steps 1-4 from the architecture build order are entirely local and cluster-free. This phase proves the entire application domain before any infrastructure work. Email delivery (step 2) is the core-value proof and should be validated against the real inbox early. Bearer auth (step 3) and config abstraction (step 4) are independent of each other and can be built in parallel. This phase produces a fully functional binary verifiable with go run.

**Delivers:** A working mcp-notify binary: send_notification tool registered and callable, real Gmail email delivered, bearer auth enforced on /mcp, /healthz and /readyz open, config loaded from env, SecretProvider interface with env and file impls, graceful shutdown on SIGTERM, structured logging (slog), request size limit, dedup (in-memory content-hash TTL), rate limit (in-memory token bucket), Channel/Renderer/NotificationService domain model with full test coverage.

**Addresses:** All P1 features -- the entire table-stakes list plus dedup, rate limit, and the Notifier interface seam.

**Avoids:** Coupling rendering/SMTP into the tool handler; recipient accepted from tool input; auth on health endpoints; unsanitized markdown HTML.

**Research flags:** Standard patterns -- well-documented Go SDK, go-mail, goldmark. No deep research phase needed. Confirm: MCP tool registration API in go-sdk v1.7.0 (verify AddTool typed handler signature against current docs at build time).

### Phase 2: Containerize + Helm Chart Baseline

**Rationale:** Step 5 (Dockerfile) depends on the working binary. Step 6 (Helm chart) depends on the image. Together these form the smallest deployable unit: image in the cluster registry + chart that can render and deploy, using temporary secret values to verify the Pod runs. No ESO or ingress yet.

**Delivers:** Multi-stage Dockerfile (Go 1.26 build to distroless/static-debian13:nonroot, CGO_ENABLED=0, -trimpath -ldflags="-s -w"), image pushed to registry.k8s.blacktoaster.com/mcp-notify/server:<sha> (immutable tag, never latest), Helm chart with Deployment + Service + ServiceAccount templates following the frontend chart conventions, liveness/readiness probes wired to /healthz//readyz, non-root user (UID 65532), read-only root filesystem.

**Uses:** distroless/static-debian13:nonroot (CA certs for Gmail TLS); values.yaml with no secrets (git-tracked); immutable image tag in values.

**Avoids:** scratch base image (breaks Gmail TLS); latest image tag (breaks GitOps); secrets in values.yaml.

**Research flags:** Standard patterns -- Go multi-stage builds and distroless are well-documented.

### Phase 3: ESO Secrets + NGINX Ingress + TLS

**Rationale:** Steps 7 and 8 from the build order are independent of each other (both depend on the chart from Phase 2) but are grouped here because they are both cluster-infrastructure work. This is where the endpoint becomes publicly reachable over HTTPS. ESO wiring and ingress/cert work are the two highest-risk infra steps; keeping them in one phase with explicit acceptance criteria prevents them from being rushed.

**Delivers:** ExternalSecret resource (vault-backend ClusterSecretStore, secret/data/apps/mcp-notify/*, version-pinned), Vault secret entries for GMAIL_APP_PASSWORD/BEARER_TOKEN/GMAIL_USERNAME, app picks up secrets via envFrom secretRef, Ingress (nginx, proxy-buffering: "off", proxy-read-timeout: "3600", routes /mcp externally), explicit Certificate resource (vault-issuer ClusterIssuer, secretName: mcp-notify-tls), external-dns target in.k8s.blacktoaster.com, domain mcp-notify.k8s.blacktoaster.com.

**Avoids:** NGINX buffering killing MCP streaming (explicit annotation + X-Accel-Buffering: no in Go); ArgoCD vs ESO fight (ignoreDifferences set before ArgoCD phase); ESO version-pin surprise (rotation runbook documented); cert not issued (gate on Certificate Ready=True + DNS resolves + HTTPS trusted before proceeding).

**Acceptance criteria:** curl -N through public host streams incrementally; Certificate Ready=True; DNS resolves; HTTPS trusted; go-mail reaches smtp.gmail.com:587 from inside the Pod; real email arrives in inbox with correct From.

**Research flags:** Standard patterns for ESO + NGINX -- pitfalls are all well-documented. The NGINX buffering annotation is load-bearing; verify it explicitly.

### Phase 4: ArgoCD GitOps + Production Hardening

**Rationale:** Step 9 (ArgoCD Application) is last because it orchestrates all prior artifacts. This phase also adds Prometheus metrics and performs the full end-to-end verification checklist. ArgoCD self-heal must not be enabled before ignoreDifferences is confirmed working.

**Delivers:** ArgoCD Application manifest (argocd/application.yaml), ignoreDifferences on managed Secret /data, syncPolicy: automated with prune: true / selfHeal: true / CreateNamespace=true, steady Synced/Healthy state verified. Optional Prometheus ServiceMonitor. ESO secret rotation runbook tested end-to-end.

**Avoids:** ArgoCD self-heal reverting ESO Secret (ignoreDifferences verified before self-heal on); graceful shutdown dropping in-flight emails on rollout (terminationGracePeriodSeconds exceeds SMTP timeout).

**Acceptance criteria (the full looks-done-but-isnt checklist):** Streaming verified through public hostname; Claude Code AND neo both list and call the tool; wrong/missing token returns 401 on POST and streaming GET; received email From is michael@blacktoaster.com; email arrives in inbox (not spam); ESO version bump propagates correctly; ArgoCD Synced/Healthy with self-heal on; rollout does not drop an in-flight send; values.yaml shows immutable tag; recipient is immutable in code; HTML body with <script> is neutralized by bluemonday; rate limit trips on excess sends.

**Research flags:** ArgoCD + ESO ignoreDifferences is a well-documented cluster convention (emporia pattern); follow it exactly. ServiceMonitor is only needed if Prometheus Operator is present -- confirm before adding.

### Phase Ordering Rationale

- Phases 1-2 are pure local work with no cluster dependency, isolating application bugs from infrastructure bugs and providing strong early validation that the core product works.
- Phase 3 groups ESO and ingress/TLS because both depend on the Phase 2 chart, both are async infrastructure operations with wait periods, and their acceptance criteria are co-located (real email from a real HTTPS endpoint).
- Phase 4 is last because ArgoCD requires all prior artifacts (image, chart, secrets, ingress, cert) to be in place and validates the full GitOps loop.
- Dedup and rate limiting are in Phase 1 (not Phase 3/4) because they are pure Go domain features that protect the inbox from day one and are testable locally without any cluster.

### Research Flags

Phases needing a research spike during planning:
- **Phase 3 (ESO):** Confirm exact Vault KV v2 path and property key conventions match what the vault-backend ClusterSecretStore expects. Model directly on the emporia ExternalSecret before templating.
- **Phase 4 (ArgoCD):** Confirm whether the KubernetesTracker app-of-apps should be extended or whether a standalone Application is preferred. PROJECT.md recommends standalone (emporia pattern).

Phases with standard patterns (no research phase needed):
- **Phase 1 (Go core):** All libraries have stable APIs and good documentation.
- **Phase 2 (Dockerfile + Helm):** Go multi-stage builds and distroless are well-documented; the Helm chart structure is directly modeled on the existing frontend chart.

---

## Confidence Assessment

| Area | Confidence | Notes |
|------|------------|-------|
| Stack | HIGH | Official SDK v1.7.0 confirmed; go-mail v0.6.2 confirmed; goldmark v1.8.2 confirmed; Go 1.25 floor from CrossOriginProtection. bluemonday exact latest tag is MEDIUM -- pin at install time. |
| Features | HIGH | Tool schema and anti-feature list are well-grounded in MCP spec and email mechanics. Channel abstraction design judgment is MEDIUM -- thin and reversible, so risk is low. |
| Architecture | HIGH | Ports-and-adapters pattern, ESO-as-backend-swap boundary, and Kubernetes resource set all verified against official sources and existing cluster conventions. |
| Pitfalls | HIGH for infra/SMTP/ESO | NGINX buffering, ArgoCD/ESO fight, ESO version pin, Gmail port/From, graceful shutdown -- all verified. MCP protocol version negotiation is MEDIUM (spec is a moving target; 2026-07 revision pending). |

**Overall confidence:** HIGH

### Gaps to Address

These four open questions each need a decision or spike before or during Phase 1:

- **Single-replica assumption:** Research and architecture both assume a single replica, which keeps dedup and rate-limiting as simple in-memory structures. Decision needed: confirm single replica is acceptable for the POC.

- **Gmail / blacktoaster.com SPF/DKIM/DMARC deliverability:** If blacktoaster.com is a Google Workspace domain, SPF/DKIM/DMARC must be configured or emails may land in spam. Spike: send a test email via go-mail before Phase 1 is considered complete and verify it reaches the inbox (not spam folder).

- **Secret delivery mode (env vars vs file mount):** Env vars require a pod restart to pick up ESO-rotated secrets; file mounts are updated in place by kubelet. Decision: pick one for v1. Env vars are simpler and adequate given the manual-bump rotation runbook already in place.

- **MCP protocol version Claude Code and neo negotiate:** Confirm the initialize handshake succeeds with both actual clients before building the full tool handler on top of the SDK. Spike: run the SDK example server against Claude Code and neo locally.

---

## Sources

### Primary (HIGH confidence)
- github.com/modelcontextprotocol/go-sdk releases + source -- SDK v1.7.0, NewStreamableHTTPHandler, CrossOriginProtection, typed tool handlers
- github.com/mark3labs/mcp-go releases -- v0.55.1 (still pre-1.0; basis for the SDK choice)
- github.com/wneessen/go-mail -- v0.6.2, multipart, STARTTLS, net/smtp fork rationale
- github.com/yuin/goldmark -- v1.8.2, CommonMark, GFM, bluemonday pairing recommendation
- pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp -- NewStreamableHTTPHandler, StreamableHTTPOptions API
- modelcontextprotocol.io/docs/tutorials/security/authorization -- bearer-on-streamable-HTTP
- modelcontextprotocol.io/specification/2025-11-25 -- MCP transport spec, session header, version negotiation
- NGINX SSE buffering (proxy_buffering off, X-Accel-Buffering: no) -- verified against NGINX proxy docs and known SSE patterns
- /Users/michael/_code/k8s/KubernetesTracker/argocd/base/frontend -- canonical Helm chart skeleton (cluster-authoritative)
- .planning/PROJECT.md -- project constraints, cluster conventions, emporia analog

### Secondary (MEDIUM confidence)
- MCP 2026-07 spec changes (removal of Mcp-Session-Id) -- pending revision, not yet final
- gcr.io/distroless/static-debian13 + Go multi-stage build best practices -- community consensus 2026
- github.com/caarlos0/env/v11 -- best-practice consensus for env-first k8s config
- Transactional email best practices (Postmark, Zoho) -- deliverability and header hygiene guidance
- Anthropic Engineering: Writing effective tools for AI agents -- tool description best practices
- MCP Tool Annotations blog post -- readOnlyHint, destructiveHint, idempotentHint, openWorldHint
- nikoksr/notify, Flux notification-controller -- channel abstraction prior art

### Tertiary (LOW confidence)
- Minor version pins (bluemonday latest tag, caarlos0/env v11 exact patch) -- verify at install time
- Gmail daily sending limits (500/day consumer, 2000/day Workspace) -- confirm against current docs if rate limit thresholds matter

---
*Research completed: 2026-06-25*
*Ready for roadmap: yes*
