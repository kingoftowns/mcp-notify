# Roadmap: mcp-notify

## Overview

mcp-notify ships as a series of demonstrable vertical increments that follow the research build order: prove the entire Go application locally before touching the cluster, then walk the strict infra chain (image → chart → secrets → ingress → ArgoCD). Phase 1 already delivers real user value with no cluster — a locally-run MCP server whose `send_notification` tool lands a correctly-formatted email in the inbox. Each subsequent phase adds one demonstrable capability: hardening the binary, packaging it, wiring Vault secrets via ESO, exposing it over trusted streaming HTTPS, and finally placing the whole stack under GitOps with a full real-client acceptance pass.

## Phases

**Phase Numbering:**
- Integer phases (1, 2, 3): Planned milestone work
- Decimal phases (2.1, 2.2): Urgent insertions (marked with INSERTED)

Decimal phases appear between their surrounding integers in numeric order.

- [ ] **Phase 1: MCP Tool + Real Email Delivery** - Local MCP server whose `send_notification` tool lands a formatted email in the inbox
- [ ] **Phase 2: Auth, Secrets Abstraction & Operational Hardening** - Bearer-guarded, env-driven, abuse-resistant, clean-lifecycle binary
- [ ] **Phase 3: Containerize + Helm Chart Baseline** - Immutable distroless image and a chart that runs a healthy pod
- [ ] **Phase 4: ESO Secrets Integration** - Gmail and bearer credentials flow from Vault into the pod via ESO
- [ ] **Phase 5: Public Ingress + TLS Exposure** - MCP endpoint reachable over trusted, streaming HTTPS at the public host
- [ ] **Phase 6: ArgoCD GitOps + Production Verification** - GitOps-managed stack passing the full real-client acceptance checklist

## Phase Details

### Phase 1: MCP Tool + Real Email Delivery
**Goal**: An agent can call `send_notification` against a locally-run server and a correctly-formatted email lands in the user's inbox.
**Mode:** mvp
**Depends on**: Nothing (first phase)
**Requirements**: NOTIF-01, NOTIF-02, NOTIF-03, NOTIF-04, NOTIF-05, DEVEX-01, EMAIL-01, EMAIL-02, EMAIL-03, EMAIL-04, SEC-02, SEC-03
**Success Criteria** (what must be TRUE):
  1. Running `go run ./cmd/server` locally, an MCP client (Inspector/curl) lists a single `send_notification` tool exposing `subject`, markdown `body`, and optional `status` — and NO recipient field; the description tells the agent when to call it and that the recipient is fixed
  2. Calling the tool sends a real multipart HTML+plaintext email that arrives in michael@blacktoaster.com's inbox (verified NOT spam), `From` the authenticated Gmail account, with the subject prefixed by the status tag (e.g. `[⏳ Waiting] …`)
  3. A `<script>`/raw-HTML body is neutralized (bluemonday) in the delivered HTML and CRLF in the subject is stripped — no script or header injection survives
  4. A successful call returns a confirmation result; a forced SMTP failure returns a clear tool error rather than a silent success
  5. The official go-sdk `StreamableHTTPHandler` serves `/mcp` over remote streamable HTTP and the SDK negotiates/echoes the client protocol version on the local handshake
  6. The server runs under the VS Code "Debug mcp-notify" launch config inside the devcontainer (breakpoints hit in `cmd/server`/handlers), and a local MCP client connects to `http://localhost:8080/mcp` and successfully calls the tool
**Plans**: TBD

Plans:
- [ ] 01-01: MCP server skeleton — `*mcp.Server`, `send_notification` registration (no `to` field), `StreamableHTTPHandler` on `/mcp`
- [ ] 01-02: Renderer (goldmark + bluemonday) and status→subject tagging behind the `Renderer` interface
- [ ] 01-03: Email channel (go-mail, Gmail :587 STARTTLS, fixed recipient) behind the `Channel` interface + `NotificationService` wiring
- [ ] 01-04: Real-send verification against the inbox (deliverability / SPF-DKIM-DMARC spike) and protocol-version handshake check
- [ ] 01-05: Local dev — devcontainer + `.vscode/launch.json` (mirrored from SpinWheel) verified: debug the server and connect a local MCP client to `localhost:8080/mcp` (`.devcontainer/`, `.vscode/launch.json`, `.env.example` already scaffolded during init)

**Open questions to resolve in this phase:**
- blacktoaster.com SPF/DKIM/DMARC deliverability — send a real test email and confirm it reaches the inbox, not spam (spam-risk gate before Phase 1 is "done")
- Confirm the MCP protocol version the official SDK negotiates is one Claude Code and neo both support (deeper real-client check deferred to Phase 6)

### Phase 2: Auth, Secrets Abstraction & Operational Hardening
**Goal**: The server is secured and production-shaped — bearer-guarded MCP, env/secret-driven config, inbox-abuse protections, structured logging, and a clean shutdown lifecycle.
**Mode:** mvp
**Depends on**: Phase 1
**Requirements**: SEC-01, SEC-04, SECR-01, SECR-02, OPS-01, OPS-02, OPS-03
**Success Criteria** (what must be TRUE):
  1. A request to `/mcp` with a missing/wrong bearer token returns 401 on BOTH the POST and the streaming GET (constant-time compare); `/healthz` and `/readyz` stay open and require no token
  2. The server runs entirely from environment-sourced config and secrets through a thin `SecretProvider` seam (env + file impls), with no credential values present in code, `values.yaml`, or logs
  3. Duplicate sends (same subject+body+status within TTL) are deduped and excessive sends trip an in-memory token-bucket rate limit, each returning a clear, distinguishable tool result
  4. Tool calls and send outcomes are logged via `log/slog` without ever emitting the Gmail app password or bearer token (verified by a log grep)
  5. On SIGTERM the server flips readiness to not-ready, stops accepting new requests, and lets in-flight SMTP sends complete before exit
**Plans**: TBD

Plans:
- [ ] 02-01: Bearer-auth middleware on `/mcp` only (POST + GET), `/healthz` + `/readyz` open
- [ ] 02-02: `config.Load` + `SecretProvider` interface (env + file impls); redacted structured logging
- [ ] 02-03: In-memory content-hash dedup + token-bucket rate limit with clear tool results
- [ ] 02-04: Graceful shutdown (SIGTERM → readiness flip → drain in-flight sends) + request size limit

**Open questions to resolve in this phase:**
- Confirm the single-replica assumption is acceptable for the POC (keeps dedup + rate-limit as simple in-memory structures; documented, not shared/Redis)
- Decide env-var vs file-mount secret delivery for v1 — env recommended (simpler; pairs with the manual ESO version-bump rotation runbook)

### Phase 3: Containerize + Helm Chart Baseline
**Goal**: The server is packaged as an immutable distroless image and a Helm chart that deploys a running, healthy pod in the cluster.
**Mode:** mvp
**Depends on**: Phase 2
**Requirements**: DEP-01, DEP-02
**Success Criteria** (what must be TRUE):
  1. A multi-stage Dockerfile builds a `CGO_ENABLED=0` static binary on Go 1.25+ to a `distroless/static-debian13:nonroot` image, pushed to `registry.k8s.blacktoaster.com/mcp-notify/server` with an immutable SHA/semver tag — never `latest`
  2. `helm template` renders Deployment, Service, ServiceAccount, Ingress, Certificate, and ExternalSecret from `values.yaml` (modeled on the KubernetesTracker frontend chart) with ZERO secrets in `values.yaml`
  3. Deployed to a namespace with temporary secret values, the pod runs as non-root (UID 65532) with a read-only root filesystem and reports Ready via `/healthz` + `/readyz` probes
  4. From inside the running pod, go-mail reaches `smtp.gmail.com:587` and a real email is delivered — proving the distroless image ships the CA certs Gmail TLS requires
**Plans**: TBD

Plans:
- [ ] 03-01: Multi-stage Dockerfile → distroless/static; build + push immutable-tagged image to the registry
- [ ] 03-02: Helm chart skeleton (Deployment, Service, ServiceAccount, `_helpers.tpl`, `values.yaml`) modeled on the frontend chart
- [ ] 03-03: Deploy with temporary secret values; verify probes, non-root/read-only, and in-cluster real email send

### Phase 4: ESO Secrets Integration
**Goal**: Gmail and bearer credentials flow from Vault into the pod via ESO, with no secrets in git and a verified rotation path.
**Mode:** mvp
**Depends on**: Phase 3
**Requirements**: DEP-03
**Success Criteria** (what must be TRUE):
  1. An `ExternalSecret` (`vault-backend` ClusterSecretStore, version-pinned, sourcing `secret/data/apps/mcp-notify/*`) materializes a Kubernetes Secret with `GMAIL_APP_PASSWORD`, `BEARER_TOKEN`, and `GMAIL_USERNAME`
  2. The Deployment consumes the Secret via `envFrom` and sends a real email from in-cluster using the Vault-sourced credentials — the temporary Phase 3 values are removed
  3. Writing a new value to Vault and bumping the `ExternalSecret` version propagates the rotated value to the pod after a restart (the version-pin rotation runbook is documented and exercised)
  4. Only the `ExternalSecret` is committed to git — the resolved Secret is never in git, and no credential appears in `kubectl describe` output or logs
**Plans**: TBD

Plans:
- [ ] 04-01: `externalsecret.yaml` modeled exactly on the emporia ExternalSecret (KV v2 path, property keys, version pin); Vault entries created
- [ ] 04-02: Switch Deployment to `envFrom` the ESO Secret; verify in-cluster send with Vault creds
- [ ] 04-03: Document + exercise the version-bump rotation runbook (write → bump → resync → restart)

**Open question to resolve in this phase:**
- Confirm the exact Vault KV v2 path and property-key conventions the `vault-backend` ClusterSecretStore expects (model directly on emporia before templating)

### Phase 5: Public Ingress + TLS Exposure
**Goal**: The MCP endpoint is reachable over trusted HTTPS at the public host with streaming intact.
**Mode:** mvp
**Depends on**: Phase 3
**Requirements**: DEP-04
**Success Criteria** (what must be TRUE):
  1. The cert-manager `Certificate` (`vault-issuer` ClusterIssuer, `secretName` matching `ingress.tls[].secretName`) reports `Ready=True` and the TLS secret `mcp-notify-tls` exists
  2. DNS resolves `mcp-notify.k8s.blacktoaster.com` (external-dns target `in.k8s.blacktoaster.com`) and the HTTPS endpoint presents a trusted certificate
  3. `curl -N` through the public host streams responses incrementally (not one terminal burst), proving NGINX `proxy-buffering: "off"` (+ long read/send timeouts and the Go server's `X-Accel-Buffering: no`) is in effect
  4. An authenticated MCP call through the public HTTPS host lists and calls `send_notification` and delivers a real email
**Plans**: TBD

Plans:
- [ ] 05-01: `ingress.yaml` (nginx, `proxy-buffering: "off"`, `proxy-read/send-timeout: "3600"`, route `/mcp`) + Go `X-Accel-Buffering: no`
- [ ] 05-02: Explicit `certificate.yaml` (vault-issuer); gate on `Certificate Ready=True` + DNS resolves + trusted HTTPS
- [ ] 05-03: `curl -N` streaming verification and an authenticated end-to-end call through the public host

### Phase 6: ArgoCD GitOps + Production Verification
**Goal**: The whole stack is GitOps-managed and passes the full real-client, end-to-end acceptance checklist.
**Mode:** mvp
**Depends on**: Phase 4, Phase 5
**Requirements**: DEP-05
**Success Criteria** (what must be TRUE):
  1. An emporia-style ArgoCD `Application` (`path: helm`, `CreateNamespace=true`, `selfHeal: true`) reaches steady `Synced`/`Healthy` with `ignoreDifferences` on the ESO-managed Secret `/data` — no OutOfSync flapping and ESO values are never reverted (ignoreDifferences confirmed BEFORE self-heal is enabled)
  2. Both Claude Code AND neo complete the MCP handshake over the public host and successfully list and call `send_notification`, each delivering a real email to the inbox with `From` = michael@blacktoaster.com
  3. A wrong/missing bearer token returns 401 on both the POST and the streaming GET through the public host
  4. An ArgoCD sync / rolling update does not drop an in-flight send (terminationGracePeriodSeconds exceeds the SMTP timeout), and `values.yaml` shows an immutable image tag
**Plans**: TBD

Plans:
- [ ] 06-01: `argocd/application.yaml` (emporia-style single Application) with `ignoreDifferences` on Secret `/data`, verified before self-heal on
- [ ] 06-02: Full "looks-done-but-isn't" acceptance pass — real Claude Code + neo handshake, auth-on-every-verb, From alignment, inbox-not-spam, rollout drains in-flight send

**Open question to resolve in this phase:**
- Confirm a standalone Application (emporia pattern) is preferred over extending the KubernetesTracker app-of-apps (PROJECT.md recommends standalone)

## Progress

**Execution Order:**
Phases execute in numeric order: 1 → 2 → 3 → 4 → 5 → 6

| Phase | Plans Complete | Status | Completed |
|-------|----------------|--------|-----------|
| 1. MCP Tool + Real Email Delivery | 0/5 | Not started | - |
| 2. Auth, Secrets & Hardening | 0/4 | Not started | - |
| 3. Containerize + Helm Baseline | 0/3 | Not started | - |
| 4. ESO Secrets Integration | 0/3 | Not started | - |
| 5. Public Ingress + TLS Exposure | 0/3 | Not started | - |
| 6. ArgoCD GitOps + Verification | 0/2 | Not started | - |
