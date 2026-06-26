# Architecture Research

**Domain:** Remote MCP server (Go, streamable HTTP) sending Gmail email, deployed via Helm + ArgoCD + ESO into an existing Kubernetes cluster
**Researched:** 2026-06-25
**Confidence:** HIGH (Go SDK, email stack, ESO/Helm conventions all verified against current sources)

## Standard Architecture

### System Overview — Go application

```
                         Incoming HTTPS (via NGINX Ingress)
                                      │
┌─────────────────────────────────────────────────────────────────────┐
│                    net/http.Server  (single listener :8080)          │
│                                                                       │
│   ServeMux                                                            │
│   ├── /mcp     → [ AuthMiddleware ] → [ StreamableHTTPHandler ]       │
│   │                 (bearer token)        (go-sdk *mcp.Server)        │
│   ├── /healthz → (unauthenticated, k8s liveness/readiness)           │
│   └── /metrics → (unauthenticated, Prometheus)                       │
└───────────────────────────────┬───────────────────────────────────────┘
                                 │  tool call: send_notification
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│  Tool handler (transport-agnostic)                                    │
│    decode args (subject, body, status) → call NotificationService     │
└───────────────────────────────┬───────────────────────────────────────┘
                                 ▼
┌─────────────────────────────────────────────────────────────────────┐
│  NotificationService  (orchestration / domain core)                   │
│    1. build subject line from status tag                              │
│    2. render markdown body → HTML (+ plaintext fallback)              │
│    3. hand a Notification to the configured Channel                   │
└──────────────┬───────────────────────────────┬────────────────────────┘
               │ Renderer iface                 │ Channel iface
               ▼                                ▼
   ┌──────────────────────┐        ┌──────────────────────────────────┐
   │ goldmark renderer    │        │  EmailChannel (go-mail / SMTP)   │
   │ markdown → HTML/text │        │  Gmail :587 STARTTLS, app pwd    │
   └──────────────────────┘        │  ── future: SlackChannel ──      │
                                    └──────────────────────────────────┘
               Config + secrets injected at composition root (cmd/server/main.go)
                          ▲
                          │ reads materialized k8s Secret (env or file)
                          │
                   ESO ExternalSecret ──► Vault KV v2 (vault-backend)
                                          (future: AWS Secrets Manager — ESO swap only)
```

### Component Responsibilities (Go)

| Component | Responsibility | Typical Implementation |
|-----------|----------------|------------------------|
| `cmd/server/main.go` | Composition root: load config, build dependencies, wire mux, start server + graceful shutdown | `main()` + small `run() error` |
| `internal/config` | Parse non-secret config (port, recipient, SMTP host/port, from-address) and locate secrets | env vars via `os.Getenv` / `caarlos0/env` |
| `internal/secrets` | Resolve secret values from the materialized k8s Secret | `SecretProvider` iface: env impl + file impl |
| `internal/transport/mcphttp` | Build `*mcp.Server`, register `send_notification`, expose `StreamableHTTPHandler` | official `modelcontextprotocol/go-sdk` |
| `internal/auth` | Bearer-token HTTP middleware (constant-time compare) | `func(http.Handler) http.Handler` |
| `internal/notify` | Domain core: subject tagging, render+send orchestration | plain struct, depends on ifaces only |
| `internal/render` | Markdown → HTML + plaintext fallback | `yuin/goldmark` (+ optional bluemonday) |
| `internal/channel` | `Channel` interface | one file per impl |
| `internal/channel/email` | SMTP delivery to fixed recipient | `wneessen/go-mail` |
| `internal/httpserver` | Assemble mux (mcp/health/metrics), timeouts, shutdown | stdlib `net/http` |

**Key boundary rule:** `internal/notify` and the tool handler depend ONLY on the `Channel` and `Renderer` interfaces — never on `go-mail`, `goldmark`, or any SMTP type directly. This is what makes "email now, Slack later" a one-file addition.

## Recommended Project Structure

```
mcp-notify/
├── cmd/
│   └── server/
│       └── main.go              # composition root: config → deps → mux → serve
├── internal/
│   ├── config/
│   │   └── config.go            # Config struct, Load() from env (+ _FILE convention)
│   ├── secrets/
│   │   ├── provider.go          # SecretProvider interface
│   │   ├── env.go               # reads from env vars (ESO envFrom)
│   │   └── file.go              # reads from mounted files (ESO volume) — rotation-friendly
│   ├── auth/
│   │   └── bearer.go            # BearerAuth(token) middleware, ConstantTimeCompare
│   ├── transport/
│   │   └── mcphttp/
│   │       ├── server.go        # NewServer(): *mcp.Server + AddTool registration
│   │       └── handler.go       # NewStreamableHTTPHandler wiring
│   ├── notify/
│   │   ├── service.go           # NotificationService.Send(ctx, Notification)
│   │   └── notification.go      # Notification, Status types
│   ├── render/
│   │   └── markdown.go          # Renderer iface + goldmark impl
│   ├── channel/
│   │   ├── channel.go           # Channel interface
│   │   └── email/
│   │       └── email.go         # EmailChannel implements Channel via go-mail
│   └── httpserver/
│       └── server.go            # mux assembly, health, metrics, timeouts, shutdown
├── helm/
│   ├── Chart.yaml
│   ├── values.yaml              # git-tracked, NO secrets
│   ├── values.local.yaml        # git-ignored (local overrides)
│   └── templates/
│       ├── _helpers.tpl
│       ├── deployment.yaml
│       ├── service.yaml
│       ├── ingress.yaml
│       ├── certificate.yaml
│       ├── externalsecret.yaml
│       ├── serviceaccount.yaml
│       └── servicemonitor.yaml  # optional (only if Prometheus Operator present)
├── argocd/
│   └── application.yaml         # emporia-style single Application → path: helm
├── Dockerfile                   # multi-stage, distroless/static final
├── go.mod
└── go.sum
```

### Structure Rationale

- **`cmd/` vs `internal/`:** Standard Go layout. `internal/` prevents the packages from being imported by anything outside this module — appropriate for a single-purpose service.
- **`channel/` is a package with sub-packages per impl:** The `Channel` interface lives in `channel/channel.go`; `channel/email` imports nothing the domain depends on. Adding Slack = `channel/slack/slack.go` implementing the same interface. No change to `notify` or the tool handler.
- **`secrets/` separate from `config/`:** Config is non-secret and stable; secrets have a distinct lifecycle (rotation, ESO version bumps). Keeping them apart lets the file-based provider support hot-reload later without touching config parsing.
- **`transport/mcphttp` isolates the SDK:** If the go-sdk API shifts (it is past v1.0 but still evolving the streamable transport), the blast radius is one package. The tool handler signature is the only SDK type that leaks, and it immediately delegates to `notify`.

## Architectural Patterns

### Pattern 1: Ports & Adapters (Hexagonal-lite) for swappable channel/secret backends

**What:** Domain core (`notify`) defines interfaces (ports); concrete tech (`email`, `goldmark`, `env`/`file` secret readers) are adapters wired at the composition root.
**When to use:** Exactly this situation — a requirement to swap delivery channel and secret backend without rework.
**Trade-offs:** One extra interface indirection. For a service this small the cost is trivial and the payoff (Slack drop-in, no Vault SDK in the binary) is high.

**The two interfaces that matter:**

```go
// internal/channel/channel.go
type Channel interface {
    Send(ctx context.Context, n notify.Notification) error
    Name() string // "email", later "slack"
}

// internal/secrets/provider.go
type SecretProvider interface {
    Get(key string) (string, error) // e.g. "GMAIL_APP_PASSWORD", "BEARER_TOKEN"
}
```

**Composition root wires the chosen impls:**

```go
// cmd/server/main.go (sketch)
sp := secrets.NewEnv()                 // or secrets.NewFile(dir) — config-selected
cfg := config.Load(sp)
renderer := render.NewGoldmark()
ch := email.New(email.Config{          // Channel impl chosen here
    Host: cfg.SMTPHost, Port: cfg.SMTPPort,
    Username: cfg.GmailUser, Password: cfg.GmailAppPassword,
    To: cfg.Recipient,                 // fixed recipient, never from tool args
})
svc := notify.New(renderer, ch)
mcpSrv := mcphttp.NewServer(svc)       // registers send_notification
```

### Pattern 2: Secret backend swap lives in ESO, not Go (CRITICAL design call)

**What:** The Go app never imports a Vault or AWS SDK. ESO materializes a normal k8s `Secret`; the app reads it as env vars or files. Swapping Vault → AWS Secrets Manager is purely an ESO `SecretStore`/`ExternalSecret` change — **zero Go code change, zero rebuild.**
**When to use:** Any ESO-based cluster. This is the cleanest interpretation of "abstract the secret backend."
**Trade-offs:** The Go-level `SecretProvider` interface becomes an optional seam (only needed if you ever run WITHOUT ESO and want the binary to call AWS SM directly). Keep the interface for testability and that escape hatch, but the *primary* abstraction is the ESO boundary.

```
POC:    Go reads Secret  ◄── ESO ExternalSecret ◄── ClusterSecretStore "vault-backend" ◄── Vault
Future: Go reads Secret  ◄── ESO ExternalSecret ◄── (Cluster)SecretStore "aws-backend"  ◄── AWS SM
        └── identical app, identical Deployment; only the ESO store/refs differ
```

### Pattern 3: One listener, middleware only on `/mcp`

**What:** `NewStreamableHTTPHandler` returns a plain `http.Handler`, so it slots into a stdlib `ServeMux`. Wrap only the `/mcp` route in the bearer middleware; leave `/healthz` and `/metrics` open so k8s probes and Prometheus don't need the token.
**When to use:** Always for this app — keeps probes working even if the token is misconfigured, and keeps scrape config simple.
**Trade-offs:** `/metrics` is unauthenticated; rely on NGINX ingress not exposing it (only route `/mcp` externally) and/or NetworkPolicy. Note this explicitly in the chart.

```go
mux := http.NewServeMux()
mux.Handle("/mcp", auth.BearerAuth(cfg.BearerToken)(streamableHandler))
mux.HandleFunc("/healthz", healthHandler)   // 200 OK, no auth
mux.Handle("/metrics", promhttp.Handler())  // no auth; not routed via ingress
srv := &http.Server{Addr: cfg.Addr, Handler: mux,
    ReadHeaderTimeout: 10 * time.Second}
```

## Data Flow

### Request Flow

```
neo / Claude Code (MCP client over streamable HTTP)
    │  POST /mcp  Authorization: Bearer <token>  {jsonrpc: tools/call, send_notification}
    ▼
NGINX Ingress (TLS terminate) → Service → Pod :8080
    ▼
ServeMux "/mcp" → BearerAuth middleware ──(401 if bad token)──► reject
    ▼ (ok)
StreamableHTTPHandler → *mcp.Server → send_notification handler
    ▼  decode {subject, body, status}
NotificationService.Send:
    subject = tag(status) + subject           # e.g. "[WAITING] ..."
    html, text = Renderer.Render(body)        # goldmark
    Channel.Send(Notification{To: FIXED, subject, html, text})
    ▼
EmailChannel → go-mail dial Gmail :587 STARTTLS, AUTH app-password → send multipart/alternative
    ▼
CallToolResult{ ok / error } ──► streamed back to client
```

### Secret / Config Flow

```
Vault: secret/data/apps/mcp-notify/{gmail-app-password, bearer-token, gmail-username}
    ▼ (ESO, version-pinned)
ExternalSecret → k8s Secret "mcp-notify"  (keys: GMAIL_APP_PASSWORD, BEARER_TOKEN, GMAIL_USERNAME)
    ▼ (Deployment: envFrom secretRef  OR  volume mount)
Pod env / files
    ▼
config.Load(secretProvider) → Config{ BearerToken, GmailAppPassword, ... }
    ▼
composition root wires into auth middleware + EmailChannel
```

### Key Data Flows

1. **Fixed-recipient enforcement:** The recipient is set once at the composition root from config and is never derived from tool arguments. The tool input schema for `send_notification` deliberately has no `to` field. This is an architectural guarantee, not a runtime check.
2. **Status → subject tagging:** `status` (completed/waiting/info/error) maps to a subject prefix in `NotificationService`, keeping presentation logic out of the transport and channel layers.

## Kubernetes Resource Set (maps to existing emporia/ESO conventions)

| Resource | Template | Conforms to convention | Key fields |
|----------|----------|------------------------|------------|
| Deployment | `deployment.yaml` | Registry `registry.k8s.blacktoaster.com/mcp-notify/server:{tag}` | `envFrom: secretRef: mcp-notify`; liveness/readiness `/healthz`; non-root, read-only rootfs |
| Service | `service.yaml` | ClusterIP :80 → :8080 | selector on app labels |
| Ingress | `ingress.yaml` | `ingressClassName: nginx`; host `mcp-notify.k8s.blacktoaster.com`; external-dns target `in.k8s.blacktoaster.com`; route ONLY `/mcp` (+ `/` if desired) | `tls:` secret `mcp-notify-tls` |
| Certificate | `certificate.yaml` | cert-manager `ClusterIssuer: vault-issuer`; `secretName: mcp-notify-tls` | dnsNames `mcp-notify.k8s.blacktoaster.com` |
| ExternalSecret | `externalsecret.yaml` | `ClusterSecretStore: vault-backend`; `remoteRef` `secret/data/apps/mcp-notify/*`; **version pin** | `target.name: mcp-notify`; manual version bump to resync |
| ServiceAccount | `serviceaccount.yaml` | dedicated SA, no extra RBAC needed | `automountServiceAccountToken: false` (app calls no k8s API) |
| ServiceMonitor | `servicemonitor.yaml` *(optional)* | only if Prometheus Operator present | scrape `/metrics` on the service port |

**values.yaml parameterization** (git-tracked, no secrets): `image.repository`, `image.tag`, `replicaCount`, `host`, `recipient` (michael@blacktoaster.com), `smtp.host`, `smtp.port`, `smtp.username` (non-secret), `externalSecret.version`, `resources`, `ingress.className`, `tls.secretName`, `certificate.issuerRef`. Secrets (`GMAIL_APP_PASSWORD`, `BEARER_TOKEN`) come ONLY through the ExternalSecret → never in values.

### ArgoCD Application (emporia-style single Application)

```yaml
# argocd/application.yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: mcp-notify
  namespace: argocd
spec:
  project: default
  source:
    repoURL: <this repo>
    targetRevision: main
    path: helm                      # in-repo Helm chart
    helm:
      valueFiles: [values.yaml]
  destination:
    server: https://kubernetes.default.svc
    namespace: mcp-notify
  syncPolicy:
    automated: { prune: true, selfHeal: true }
    syncOptions: [CreateNamespace=true]
  ignoreDifferences:                # ESO-managed Secret churns on /data
    - group: ""
      kind: Secret
      name: mcp-notify
      jsonPointers: [/data]
```

The `ignoreDifferences` on the `Secret` `/data` is mandatory: ESO writes the Secret's data, and without this ArgoCD perpetually reports OutOfSync / tries to revert it — exactly the emporia pattern noted in PROJECT.md.

## Suggested Build Order (each step independently demonstrable)

| Step | Component | Demonstrable result | Depends on |
|------|-----------|---------------------|------------|
| 1 | MCP server skeleton: `*mcp.Server` + `send_notification` (stub) + `StreamableHTTPHandler` on `/mcp` | MCP Inspector / curl gets a tool list and a stubbed call locally | — |
| 2 | Email channel: `goldmark` render + `go-mail` SMTP, wired behind `Channel`/`Renderer` ifaces | Real formatted email lands in inbox (run locally with app password in env) | 1 |
| 3 | Bearer auth middleware + `/healthz` + `/metrics` on one mux | 401 without token, 200 with; probes/metrics open | 1 |
| 4 | Config/secrets abstraction: `config.Load` + `SecretProvider` (env + file) | Runs purely from env vars; file mode works too | 2,3 |
| 5 | Containerize: multi-stage Dockerfile → distroless/static, push to `registry.k8s.blacktoaster.com/mcp-notify/server` | Image runs the server identically to local | 4 |
| 6 | Helm chart: Deployment + Service (+ ServiceAccount) | `helm template` renders; deploys and runs in a namespace (secrets via temp values) | 5 |
| 7 | ExternalSecret / ESO | Secret materialized from Vault `secret/data/apps/mcp-notify/*`; app picks it up via `envFrom` | 6 |
| 8 | Ingress + Certificate (NGINX + cert-manager `vault-issuer`) | Reachable at `https://mcp-notify.k8s.blacktoaster.com/mcp` with valid TLS | 6 |
| 9 | ArgoCD Application (+ `ignoreDifferences`) | GitOps-managed; sync is Healthy/Synced; push-to-deploy | 7,8 |

**Dependency notes for roadmapping:**
- Steps 1–4 are pure Go and locally testable with **no cluster** — strong early-validation block. The whole app can be proven before any k8s work.
- Step 2 (email) is the core-value proof and depends only on the skeleton; prioritize it right after 1.
- Auth (3) is independent of email (2) and can be built in parallel.
- k8s steps (6–9) are a strict chain: image → chart → secrets → ingress → ArgoCD. ESO (7) and Ingress (8) both depend on the chart (6) but are independent of each other.
- ArgoCD (9) is last because it orchestrates the artifacts the prior steps produced.

## Scaling Considerations

| Scale | Architecture Adjustments |
|-------|--------------------------|
| Current (agent notifications, low volume) | Single replica is fine. Gmail SMTP app-password limits (~500 msgs/day) are far above need. |
| If volume grows / multiple senders | 2+ replicas behind the Service. Use `StreamableHTTPOptions{Stateless: true}` so any replica handles any request without sticky sessions. Consider a small send queue if Gmail rate-limits. |
| Multi-channel / multi-tenant | The `Channel` interface already supports fan-out; introduce a `MultiChannel` composite. Move off Gmail SMTP to a transactional provider (SES/SendGrid) for higher limits — a new `Channel` impl, no domain change. |

### Scaling Priorities
1. **First bottleneck:** Gmail SMTP rate/connection limits, not Go throughput. Mitigation: provider swap behind `Channel`, not more replicas.
2. **Second bottleneck:** Streamable-HTTP session state if running multiple replicas. Mitigation: stateless mode (supported by the SDK) — no shared session store needed.

## Anti-Patterns

### Anti-Pattern 1: Importing a Vault/AWS SDK into the Go binary
**What people do:** Add `hashicorp/vault/api` (or AWS SM SDK) and fetch secrets at startup to "abstract the backend."
**Why it's wrong:** Duplicates what ESO already does, couples the binary to a backend, and breaks the "swap backend = config-only" goal. The cluster already has ESO.
**Do this instead:** Read the ESO-materialized k8s Secret (env/file). Swap backends in ESO's `SecretStore`. Keep the Go `SecretProvider` interface only as a testing seam / non-ESO escape hatch.

### Anti-Pattern 2: Letting the tool argument choose the recipient
**What people do:** Add a `to` field to `send_notification` for flexibility.
**Why it's wrong:** Expands blast radius — an agent could email anyone. Violates a stated requirement.
**Do this instead:** Fix the recipient at the composition root; omit `to` from the input schema entirely.

### Anti-Pattern 3: Coupling rendering/SMTP into the tool handler
**What people do:** Call `goldmark` and `go-mail` directly inside the MCP tool handler.
**Why it's wrong:** Locks delivery to email and rendering to one lib; Slack later means rewriting the handler; unit testing requires a live SMTP server.
**Do this instead:** Tool handler → `NotificationService` → `Channel`/`Renderer` interfaces. Test the domain with fakes.

### Anti-Pattern 4: Authenticating the health/metrics endpoints
**What people do:** Wrap the whole mux in bearer auth.
**Why it's wrong:** k8s probes and Prometheus then need the token; a token misconfig takes down liveness and the pod crash-loops.
**Do this instead:** Auth middleware only on `/mcp`; keep `/healthz` open; keep `/metrics` open but unrouted by the ingress.

## Integration Points

### External Services

| Service | Integration Pattern | Notes |
|---------|---------------------|-------|
| Gmail SMTP | `wneessen/go-mail`, smtp.gmail.com:587 STARTTLS, AUTH PLAIN with app password | go-mail defaults to mandatory TLS; app password (not OAuth) per requirement |
| MCP clients (neo, Claude Code) | Streamable HTTP transport, `Authorization: Bearer` on every request | go-sdk `StreamableHTTPHandler`; stateless mode for multi-replica |
| Vault (via ESO) | No direct integration — ESO bridges Vault KV v2 to a k8s Secret | `vault-backend` ClusterSecretStore; version-pinned ExternalSecret |
| cert-manager | Declarative `Certificate` → `vault-issuer` ClusterIssuer → TLS secret | per-app TLS secret `mcp-notify-tls` |

### Internal Boundaries

| Boundary | Communication | Notes |
|----------|---------------|-------|
| transport/mcphttp ↔ notify | direct call, domain types only | SDK types stop at the handler |
| notify ↔ channel | `Channel` interface | swap point for Slack |
| notify ↔ render | `Renderer` interface | swap point for templating later |
| config/secrets ↔ everything | injected at composition root | no global state, no init-time fetch |
| Go app ↔ secret backend | k8s Secret (env/file), NOT an SDK | the real backend-swap seam is ESO |

## Library Decisions (verified)

| Concern | Choice | Confidence | Why |
|---------|--------|------------|-----|
| MCP server + streamable HTTP | `github.com/modelcontextprotocol/go-sdk` (official, past v1.0; streamable transport mature, supports `Stateless` mode) | HIGH (Context7 + GitHub releases) | Official, Google-maintained; `NewStreamableHTTPHandler` returns `http.Handler` → composes with stdlib mux/middleware |
| Email/SMTP | `github.com/wneessen/go-mail` | HIGH (pkg.go.dev + GitHub) | Mandatory-TLS by default, full multipart/alternative, minimal deps, actively maintained |
| Markdown → HTML | `github.com/yuin/goldmark` (+ optional `bluemonday` sanitizer) | HIGH | CommonMark-compliant, GFM extensions, does not emit raw HTML by default (safe) |
| Config | stdlib `os` or `caarlos0/env` | MEDIUM | Either is fine; keep secrets out of values.yaml |

> Note on `mark3labs/mcp-go`: a popular community alternative that also supports streamable HTTP. The **official go-sdk** is the recommendation here given it is now past v1.0, Google-co-maintained, and exposes a clean `http.Handler` for the middleware/mux composition this design relies on.

## Sources

- Official Go SDK — streamable HTTP handler & tool registration: https://github.com/modelcontextprotocol/go-sdk (Context7 `/modelcontextprotocol/go-sdk`, design.md / protocol.md examples) — HIGH
- Go SDK releases (past v1.0, streamable transport, stateless mode): https://github.com/modelcontextprotocol/go-sdk/releases — HIGH
- go-mail (SMTP/TLS/multipart): https://pkg.go.dev/github.com/wneessen/go-mail , https://github.com/wneessen/go-mail — HIGH
- goldmark (markdown→HTML, safety defaults): https://github.com/yuin/goldmark — HIGH
- MCP authorization / bearer-on-streamable-HTTP: https://modelcontextprotocol.io/docs/tutorials/security/authorization , https://auth0.com/blog/mcp-streamable-http/ — MEDIUM
- ESO/Helm/ArgoCD conventions: project `.planning/PROJECT.md` (emporia analog) — authoritative for this cluster

---
*Architecture research for: Go remote MCP email-notification server on Kubernetes (Helm + ArgoCD + ESO)*
*Researched: 2026-06-25*
