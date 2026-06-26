# Requirements: mcp-notify

**Defined:** 2026-06-25
**Core Value:** An agent can reliably reach the user with a useful, well-formatted email notification by calling a single MCP tool — delivery just works.

## v1 Requirements

Requirements for the initial release (email-only POC). Each maps to roadmap phases.

### Notification Tool (MCP)

- [ ] **NOTIF-01**: Server exposes a single `send_notification` MCP tool callable by neo and Claude Code
- [ ] **NOTIF-02**: Tool accepts `subject` (string), `body` (markdown string), and optional `status` enum (`completed` / `waiting` / `info` / `error`)
- [ ] **NOTIF-03**: Tool description tells the agent when to call it (task done / blocked / waiting on user) and that the recipient is fixed, so the agent never passes a recipient
- [ ] **NOTIF-04**: Server speaks MCP over remote streamable HTTP (official go-sdk `NewStreamableHTTPHandler`), negotiating/echoing the client protocol version
- [ ] **NOTIF-05**: A successful call returns a confirmation result; a delivery failure returns a clear tool error

### Local Dev Experience

- [ ] **DEVEX-01**: Developer can run the server in the VS Code debugger inside a devcontainer (mirroring SpinWheel's `.devcontainer/` + `.vscode/launch.json`) and connect a local MCP client to test `send_notification` end-to-end

### Email Delivery

- [ ] **EMAIL-01**: Server sends email via Gmail SMTP (`smtp.gmail.com:587`, STARTTLS) authenticated with an app password
- [ ] **EMAIL-02**: Markdown `body` is rendered to HTML and sent as a multipart message with a plain-text fallback
- [ ] **EMAIL-03**: `status` tags the subject line (e.g. `[⏳ Waiting] …`) so notifications are scannable in the inbox
- [ ] **EMAIL-04**: `From` is the authenticated Gmail account and the recipient is fixed to michael@blacktoaster.com

### Security

- [ ] **SEC-01**: The MCP endpoint requires a valid bearer token; health/readiness endpoints are open (auth scoped to `/mcp` only)
- [ ] **SEC-02**: The recipient is hardcoded server-side — the tool accepts no `to`/recipient parameter
- [ ] **SEC-03**: Rendered HTML is sanitized (no script/raw-HTML injection from the LLM-authored body) and the subject has CRLF stripped (no header injection)
- [ ] **SEC-04**: Inbox is protected from agent retry loops via rate limiting and content-hash dedup (in-memory, single replica)

### Secrets

- [ ] **SECR-01**: Gmail app password and bearer token are read from an ESO-materialized Kubernetes Secret (via env), never from git or logs
- [ ] **SECR-02**: Secret backend is swappable without code change — Vault via the `vault-backend` ClusterSecretStore for the POC, AWS Secrets Manager later, differing only by ESO `SecretStore` config; a thin Go `SecretProvider` interface remains as a test seam

### Operations

- [ ] **OPS-01**: Server exposes liveness/readiness endpoints (`/healthz`, `/readyz`) suitable for k8s probes
- [ ] **OPS-02**: Server emits structured logs of tool calls and send outcomes without leaking the app password or bearer token
- [ ] **OPS-03**: Server shuts down gracefully so in-flight sends complete before exit

### Deployment

- [ ] **DEP-01**: Built as a `CGO_ENABLED=0` static binary on Go 1.25+ in a multi-stage Dockerfile to a `distroless/static` image, published to `registry.k8s.blacktoaster.com/mcp-notify/...` with an immutable tag
- [ ] **DEP-02**: Packaged as a Helm chart (Deployment, Service, ServiceAccount, Ingress, Certificate, ExternalSecret, `_helpers.tpl`, `values.yaml`) modeled on the KubernetesTracker frontend chart
- [ ] **DEP-03**: Secret delivered by an `ExternalSecret` (`vault-backend` ClusterSecretStore, version-pinned) sourcing `secret/data/apps/mcp-notify/*`
- [ ] **DEP-04**: Exposed via NGINX ingress at `mcp-notify.k8s.blacktoaster.com` with `proxy-buffering: "off"` and long read/send timeouts for streaming, plus an explicit cert-manager `Certificate` (`vault-issuer` ClusterIssuer) whose `secretName` matches `ingress.tls`
- [ ] **DEP-05**: Deployed via an emporia-style ArgoCD `Application` pointing at the in-repo chart, with `ignoreDifferences` on the ESO-managed Secret `/data` and `CreateNamespace=true`

## v2 Requirements

Deferred to future releases. Tracked but not in the current roadmap.

### Slack Channel

- **SLACK-01**: Add a Slack delivery channel behind the existing `Channel` interface (pending Slack app approval)
- **SLACK-02**: Allow the notification to route to email and/or Slack

### Multi-Environment Secrets

- **SECR-03**: Deploy in a second environment using AWS Secrets Manager as the ESO backend (no app change)

### Observability

- **OPS-04**: Expose a Prometheus `/metrics` endpoint (and optional ServiceMonitor) for send counts/latency/failures
- **EMAIL-05**: Email threading via stable `Message-ID`/`References` so related notifications group in the inbox

## Out of Scope

Explicitly excluded. Documented to prevent scope creep.

| Feature | Reason |
|---------|--------|
| Agent-specified arbitrary recipients | Fixed recipient is the security posture; removes abuse surface |
| stdio MCP transport | Value is the hosted k8s deployment; no local-spawn use case |
| Gmail API / OAuth | App password + SMTP is sufficient and already provisioned |
| App importing Vault/AWS SDKs directly | ESO materializes the Secret; app stays backend-agnostic |
| Attachments, scheduling, rich templating beyond markdown→HTML | Not needed for the POC |
| Shared/Redis dedup+rate-limit state | Single replica assumed; in-memory is adequate |

## Traceability

Which phases cover which requirements. Populated during roadmap creation.

| Requirement | Phase | Status |
|-------------|-------|--------|
| NOTIF-01 | Phase 1 | Pending |
| NOTIF-02 | Phase 1 | Pending |
| NOTIF-03 | Phase 1 | Pending |
| NOTIF-04 | Phase 1 | Pending |
| NOTIF-05 | Phase 1 | Pending |
| DEVEX-01 | Phase 1 | Pending |
| EMAIL-01 | Phase 1 | Pending |
| EMAIL-02 | Phase 1 | Pending |
| EMAIL-03 | Phase 1 | Pending |
| EMAIL-04 | Phase 1 | Pending |
| SEC-01 | Phase 2 | Pending |
| SEC-02 | Phase 1 | Pending |
| SEC-03 | Phase 1 | Pending |
| SEC-04 | Phase 2 | Pending |
| SECR-01 | Phase 2 | Pending |
| SECR-02 | Phase 2 | Pending |
| OPS-01 | Phase 2 | Pending |
| OPS-02 | Phase 2 | Pending |
| OPS-03 | Phase 2 | Pending |
| DEP-01 | Phase 3 | Pending |
| DEP-02 | Phase 3 | Pending |
| DEP-03 | Phase 4 | Pending |
| DEP-04 | Phase 5 | Pending |
| DEP-05 | Phase 6 | Pending |

**Coverage:**
- v1 requirements: 24 total
- Mapped to phases: 24 (100% — no orphans, no duplicates)
- Unmapped: 0

**Per-phase counts:** Phase 1 = 12 · Phase 2 = 7 · Phase 3 = 2 · Phase 4 = 1 · Phase 5 = 1 · Phase 6 = 1

---
*Requirements defined: 2026-06-25*
*Last updated: 2026-06-25 after roadmap creation (traceability populated)*
