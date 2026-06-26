# Stack Research

**Domain:** Remote Go MCP server (streamable HTTP) for email notifications, deployed to Kubernetes
**Researched:** 2026-06-25
**Confidence:** HIGH (core decisions verified against official repos/releases; minor version pins MEDIUM)

## Headline Decision

**Use the official `github.com/modelcontextprotocol/go-sdk`, not `mark3labs/mcp-go`.**

This is the single most important call and it is a *change* from the 2025 community
default. Through most of 2025, `mark3labs/mcp-go` was the pragmatic choice because the
official SDK was unreleased/pre-1.0. As of mid-2026 the official SDK is stable at **v1.7.0**,
is co-maintained with Google, tracks the MCP spec authoritatively, and ships a
production-grade Streamable HTTP server transport with session management and built-in
security. `mark3labs/mcp-go` is still **v0.x** (v0.55.1) with no stable v1 — unacceptable
API-churn risk for a new, long-lived service. For a greenfield 2026 project the prescriptive
choice is the official SDK.

## Recommended Stack

### Core Technologies

| Technology | Version | Purpose | Why Recommended |
|------------|---------|---------|-----------------|
| `github.com/modelcontextprotocol/go-sdk/mcp` | v1.7.0 | MCP server + Streamable HTTP transport | Official, spec-authoritative SDK (co-maintained w/ Google). Stable v1 with semver guarantees. Ships `NewStreamableHTTPHandler` returning a plain `http.Handler` — composes cleanly with stdlib middleware (bearer auth). Built-in session management, configurable event store, and security (Content-Type checks, `http.CrossOriginProtection`). |
| `github.com/wneessen/go-mail` | v0.6.2 | Gmail SMTP (app password) + multipart HTML/plaintext | Most actively maintained modern Go mail library. Forked & hardened `net/smtp` (more auth methods, concurrency-safe, logging). First-class multipart alternative bodies (`SetBodyString` text + `AddAlternativeString` HTML) and `html/template` support. Handles STARTTLS to `smtp.gmail.com:587` and app-password auth cleanly. |
| `github.com/yuin/goldmark` | v1.8.2 | Markdown → HTML for email bodies | The de-facto standard Go Markdown engine (powers Hugo). CommonMark-compliant, zero non-stdlib deps, extensible, GFM extension for tables/strikethrough/tasklists. `gomarkdown/markdown` is the only real alternative and is less maintained/less compliant. |
| Go (toolchain) | 1.26.x (min 1.25) | Language/runtime | Current stable. **Minimum 1.25** is a hard floor: the official MCP SDK's Streamable HTTP security uses `net/http.CrossOriginProtection`, added in Go 1.25. Use 1.26 for current security/perf fixes. |
| stdlib `net/http` (`http.ServeMux`) | (Go 1.26) | HTTP server, routing, middleware, health endpoints | Go 1.22+ `ServeMux` supports method+path patterns (`GET /healthz`). The whole surface is the MCP handler + `/healthz` + `/readyz` + a bearer-auth wrapper — no router framework justified. Keep deps minimal. |

### Supporting Libraries

| Library | Version | Purpose | When to Use |
|---------|---------|---------|-------------|
| `github.com/caarlos0/env/v11` | v11.x | Typed struct config from environment | Recommended config loader. Twelve-factor, env-first, tiny, actively maintained. Maps env vars → struct with defaults/required validators. Fits k8s where ESO-synced Secrets are exposed as env vars. |
| `github.com/microcosm-cc/bluemonday` | v1.0.x (latest) | Sanitize goldmark HTML output before emailing | Use if Markdown source is ever untrusted/agent-supplied. Allowlist-based XSS scrubber; ships a UGC policy. goldmark explicitly recommends pairing with it. Optional but cheap insurance for email HTML. |
| `github.com/kelseyhightower/envconfig` | v1.4.0 | Alternative env→struct loader | Only if you prefer its prefix conventions. `caarlos0/env` is the more active project; pick one, not both. |

### Development Tools

| Tool | Purpose | Notes |
|------|---------|-------|
| `golangci-lint` | Aggregated linting | Standard Go meta-linter; wire into CI. |
| `gofumpt` | Stricter formatting | Superset of `gofmt`. |
| Docker BuildKit + multi-stage | Container build | See Dockerfile pattern below; target `distroless/static-debian13:nonroot`. |
| `govulncheck` | Vulnerability scan | Run in CI; especially relevant since email + network-exposed. |

## Installation

```bash
# Core
go get github.com/modelcontextprotocol/go-sdk/mcp@v1.7.0
go get github.com/wneessen/go-mail@v0.6.2
go get github.com/yuin/goldmark@v1.8.2

# Config + sanitizing
go get github.com/caarlos0/env/v11
go get github.com/microcosm-cc/bluemonday

# net/http, html/template, text/template, log/slog are stdlib — no install
```

## Streamable HTTP + Bearer Auth Wiring (the load-bearing pattern)

The official SDK's transport is a standard `http.Handler`, so auth and health endpoints
are plain stdlib middleware — no framework needed:

```go
// server is *mcp.Server with tools registered.
mcpHandler := mcp.NewStreamableHTTPHandler(
    func(r *http.Request) *mcp.Server { return server },
    nil, // or &mcp.StreamableHTTPOptions{...} to customize CrossOriginProtection
)

mux := http.NewServeMux()
mux.HandleFunc("GET /healthz", liveness)   // always 200 if process is up
mux.HandleFunc("GET /readyz", readiness)   // 200 only once SMTP config validated
mux.Handle("/mcp", bearerAuth(token)(mcpHandler)) // wrap MCP endpoint only

// bearerAuth: compare subtle.ConstantTimeCompare against the token from the
// Authorization: Bearer <token> header; 401 on mismatch.
```

Health endpoints are intentionally **outside** the auth wrapper so kubelet probes don't
need the token.

## Pluggable Secrets: how to structure it in Go

Key architectural fact: with **External Secrets Operator (Vault backend)**, the app does
**not** talk to Vault. ESO syncs Vault → a native k8s `Secret`, which is surfaced to the pod
as env vars (or a mounted file). So "Vault now, AWS Secrets Manager later" is *entirely an
ESO `SecretStore` concern* — it does not change app code.

Still, define a thin Go seam so the app is decoupled from *how* a value arrives:

```go
type SecretsProvider interface {
    Get(ctx context.Context, key string) (string, error)
}
```

- **Now (ESO/Vault):** `EnvProvider` reads `os.Getenv` (Secret mounted as env), or a
  `FileProvider` reads files under a mounted secret volume (e.g. `/etc/secrets/<key>`).
  File-based is preferable for rotation (kubelet updates mounted files in place; env vars
  require a pod restart).
- **Later (AWS Secrets Manager):** swap the ESO `SecretStore` backend — *zero app change*.
  Only if you ever drop ESO and call the cloud API directly do you add an `AWSSMProvider`
  implementation behind the same interface.

Load typed config with `caarlos0/env` at startup; resolve secret-shaped fields
(SMTP app password, bearer token) through the `SecretsProvider` so the source is swappable
and unit-testable with a fake.

## Container / Build

**Recommended:** Go 1.26 build stage → distroless static runtime. Static binary, non-root,
no shell.

```dockerfile
# syntax=docker/dockerfile:1
FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/mcp-notify ./cmd/server

FROM gcr.io/distroless/static-debian13:nonroot
COPY --from=build /out/mcp-notify /mcp-notify
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/mcp-notify"]
```

MCP/service-specific build notes:
- `CGO_ENABLED=0` → fully static binary; `distroless/static` (no libc) works. No CGO is
  needed by any library in this stack.
- **`distroless/static` includes CA certificates** — required because the server makes an
  outbound TLS connection to `smtp.gmail.com`. Do **not** use `scratch` unless you also copy
  CA certs and tzdata; distroless avoids that footgun.
- `-trimpath -ldflags="-s -w"` strips paths/symbols for smaller, reproducible images (~10–15MB).
- Runs as UID 65532 (`nonroot`) — aligns with restricted PodSecurity standards.
- Container listens on a single HTTP port (e.g. 8080); the streamable-HTTP MCP endpoint plus
  health probes are all served from it.

## Alternatives Considered

| Recommended | Alternative | When to Use Alternative |
|-------------|-------------|-------------------------|
| official `go-sdk` v1.7.0 | `mark3labs/mcp-go` v0.55.1 | Only if you need a higher-level/opinionated API *today* and accept v0.x churn, or are matching an existing mcp-go codebase. Not for greenfield. |
| `wneessen/go-mail` | stdlib `net/smtp` | Trivial single-part text mail with zero deps. For multipart HTML+plaintext it forces manual MIME boundary/header assembly — error-prone; `net/smtp` is also frozen. |
| `wneessen/go-mail` | `jordan-wright/email`, `go-mail/mail` (gomail fork) | Existing codebase already on one. wneessen is the most actively maintained modern option. |
| stdlib `net/http` | `go-chi/chi` v5.2.3 | If routing grows beyond a handful of endpoints or you want chi's middleware stack (RequestID, Recoverer, timeout). chi is 100% net/http-compatible, so adopting later is cheap. Not needed for this surface. |
| `caarlos0/env` | `spf13/viper` | Only if you need multi-format files (YAML/TOML), live reload, or remote config. Overkill + heavy deps for an env-first k8s service. |
| goldmark | `gomarkdown/markdown` | No strong reason; goldmark is more compliant and maintained. |

## What NOT to Use

| Avoid | Why | Use Instead |
|-------|-----|-------------|
| `mark3labs/mcp-go` for greenfield | Pre-1.0 (v0.x), no semver stability guarantee, not the spec-authoritative implementation | official `modelcontextprotocol/go-sdk` v1.7.0 |
| stdio transport | Requirement is a *remote* server an agent connects to over the network | Streamable HTTP via `NewStreamableHTTPHandler` |
| SSE transport (legacy) | Superseded by Streamable HTTP in the MCP spec; SSE is legacy/compat-only | Streamable HTTP |
| Gmail API (OAuth) | Constraint is SMTP + app password; Gmail API adds OAuth/scopes/quota complexity | `smtp.gmail.com:587` STARTTLS + app password via go-mail |
| `spf13/viper` | Pulls a large dep tree for features unused by an env-first service | `caarlos0/env/v11` |
| `scratch` base image | No CA certs/tzdata → outbound TLS to Gmail fails unless manually added | `distroless/static-debian13:nonroot` |
| App talking to Vault directly | Cluster uses ESO; direct Vault coupling defeats the backend-swap design | Read ESO-synced k8s Secret (env/file) behind a `SecretsProvider` interface |
| `go build` with CGO enabled (default) | Dynamic libc link breaks `distroless/static`/`scratch` | `CGO_ENABLED=0` |

## Stack Patterns by Variant

**If Markdown content is agent-supplied / untrusted:**
- Pipe goldmark output through `bluemonday.UGCPolicy()` before composing the HTML part.
- Because rendered HTML is emailed and could carry XSS/markup injection.

**If secret rotation without pod restart is required:**
- Use file-mounted secrets (`FileProvider`) instead of env vars.
- Because kubelet updates mounted secret files in place; env vars are fixed at pod start.

**If endpoint surface grows (multiple tools/admin routes):**
- Introduce `go-chi/chi` v5 + its middleware stack.
- Because it's net/http-compatible, so the bearer-auth wrapper and MCP handler port over unchanged.

## Version Compatibility

| Package A | Compatible With | Notes |
|-----------|-----------------|-------|
| `modelcontextprotocol/go-sdk` v1.7.0 | Go >= 1.25 | Streamable HTTP security relies on `http.CrossOriginProtection` (Go 1.25+). Build on 1.26. |
| `wneessen/go-mail` v0.6.2 | Go 1.24+ | v0.6.2 explicitly readied for Go 1.24; works on 1.25/1.26. Avoid v0.6.0 (multipart boundary regression — fixed in v0.6.1). |
| `goldmark` v1.8.2 | stdlib only | No non-stdlib deps; no conflicts. |
| `caarlos0/env/v11` | Go 1.18+ | v11 import path includes `/v11`. |
| `distroless/static-debian13` | `CGO_ENABLED=0` static binary | Includes CA certs + tzdata; required for outbound Gmail TLS. |

## Confidence by Recommendation

| Decision | Confidence | Basis |
|----------|------------|-------|
| Official go-sdk over mark3labs | HIGH | Verified official SDK at stable v1.7.0 with Streamable HTTP handler (`streamable.go`, `NewStreamableHTTPHandler`) on the repo; mark3labs confirmed still v0.55.1. |
| Streamable HTTP support in official SDK | HIGH | Confirmed via repo source + release notes (session mgmt, event store, CrossOriginProtection). |
| wneessen/go-mail | HIGH | Confirmed v0.6.2, multipart support, net/smtp fork rationale from official repo/docs. |
| goldmark v1.8.2 | HIGH | Confirmed latest on official releases. |
| stdlib net/http for routing | HIGH | Architectural fit; MCP handler is `http.Handler`. |
| caarlos0/env for config | MEDIUM | Best-practice consensus for env-first k8s services; choice over envconfig is preference, not correctness. |
| Go 1.26 / min 1.25 floor | MEDIUM | 1.25 floor inferred from SDK's use of `http.CrossOriginProtection` (Go 1.25 feature); confirm against the SDK's `go.mod` during setup. |
| bluemonday latest version | MEDIUM | Confirmed maintained; exact latest tag (>= v1.0.20) should be pinned at install time. |

## Sources

- https://github.com/modelcontextprotocol/go-sdk/releases — official SDK at v1.7.0 stable (verified, HIGH)
- https://github.com/modelcontextprotocol/go-sdk/blob/main/mcp/streamable.go — Streamable HTTP server transport source (verified, HIGH)
- https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp — `NewStreamableHTTPHandler`, `StreamableHTTPOptions` API (verified, HIGH)
- https://github.com/mark3labs/mcp-go/releases — mcp-go at v0.55.1, still v0.x (verified, HIGH)
- https://github.com/wneessen/go-mail / releases — v0.6.2, multipart + net/smtp fork rationale (verified, HIGH)
- https://github.com/yuin/goldmark — v1.8.2, CommonMark, GFM, recommends bluemonday (verified, HIGH)
- https://github.com/microcosm-cc/bluemonday/releases — maintained sanitizer, email policy tool (verified, MEDIUM on exact latest tag)
- https://github.com/go-chi/chi — v5.2.3, net/http-compatible router (verified, HIGH; alternative)
- https://pkg.go.dev/github.com/caarlos0/env / kelseyhightower/envconfig — env-struct loaders (verified, MEDIUM)
- https://gcr.io/distroless/static-debian13 + Go multi-stage build best practices (community consensus 2026, MEDIUM)

---
*Stack research for: remote Go MCP email-notification server on Kubernetes*
*Researched: 2026-06-25*
