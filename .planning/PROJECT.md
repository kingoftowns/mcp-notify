# mcp-notify

## What This Is

A remote MCP (Model Context Protocol) server, written in Go, that gives AI agents (neo and Claude Code) a tool to notify the user by email. The agent calls `send_notification` with a subject and a markdown body — e.g. "task completed" or "waiting on you" — and the server delivers it to the user's inbox via Gmail SMTP. It runs as a hosted service in the user's Kubernetes cluster, reachable over HTTPS and guarded by a bearer token. Slack delivery is a planned follow-on once the user's Slack app is approved.

## Core Value

An agent can reliably reach the user with a useful, well-formatted email notification by calling a single MCP tool — delivery just works.

## Requirements

### Validated

(None yet — ship to validate)

### Active

- [ ] MCP server exposes a `send_notification` tool callable by neo and Claude Code
- [ ] Tool accepts `subject` + markdown `body` + optional `status` (completed/waiting/info/error); status tags the subject line for inbox scannability
- [ ] Server renders markdown body to HTML and sends a multipart email with a plain-text fallback
- [ ] Email is sent via Gmail SMTP using an app password
- [ ] Recipient is fixed to the user's address (michael@blacktoaster.com); agents cannot send to arbitrary recipients
- [ ] Server speaks MCP over remote streamable HTTP (long-lived k8s service), not stdio
- [ ] Requests to the server are authorized with a bearer token
- [ ] Gmail app password and bearer token are sourced from Vault via External Secrets Operator (ESO) using the existing `vault-backend` ClusterSecretStore
- [ ] Secret backend is abstracted so AWS Secrets Manager can be used in other environments (Vault is the POC default)
- [ ] Packaged as a container image and deployed to the cluster via a Helm chart
- [ ] Deployed through ArgoCD (emporia-style single Application → in-repo Helm chart)
- [ ] Exposed externally via NGINX ingress + cert-manager TLS at `mcp-notify.k8s.blacktoaster.com`
- [ ] Developer can run the server in the VS Code debugger inside a devcontainer and connect a local MCP client to test `send_notification` end-to-end

### Out of Scope

- Slack delivery — deferred until the user's Slack app is approved (planned next milestone)
- Agent-specified arbitrary recipients — fixed recipient only, to limit blast radius
- stdio transport — the value is in the hosted k8s deployment; no local-spawn use case
- Gmail API / OAuth — app password + SMTP is sufficient and already provisioned
- Attachments, scheduling, templating beyond markdown→HTML — not needed for the POC

## Context

- **Caller agents:** neo and Claude Code will invoke the tool to report task status (done, blocked, waiting on the user).
- **Local dev experience:** the developer wants to run the server in the VS Code debugger and connect a local MCP client to test it. Dev tooling mirrors `/Users/michael/_code/SpinWheel` — a docker-compose-based `.devcontainer/` (Go feature + `air`/`dlv`/`golangci-lint`/Claude Code) and a `.vscode/launch.json` with Launch + Debug Go configs. Adapted for mcp-notify: single Go module at repo root, `cmd/server` entrypoint, Go 1.25, `PORT=8080`, and env vars `GMAIL_USERNAME`/`GMAIL_APP_PASSWORD`/`BEARER_TOKEN`/`NOTIFY_RECIPIENT` supplied via a gitignored `.env` (see `.env.example`).
- **Existing k8s infrastructure** (modeled on `emporia` as the closest analog):
  - Secrets: ESO `ClusterSecretStore` named `vault-backend`, Vault KV v2, secrets stored at `secret/data/apps/{app}/*`. `ExternalSecret` resources pin a `version` and require a manual bump to resync (intentional, for auditability).
  - Registry: `registry.k8s.blacktoaster.com/{app-name}/{component}:{tag}`
  - Ingress: NGINX (`ingressClassName: nginx`), cert-manager `ClusterIssuer` named `vault-issuer`, TLS secret per app, external-dns target `in.k8s.blacktoaster.com`, domain `{app}.k8s.blacktoaster.com` (filter `blacktoaster.com`).
  - ArgoCD: simple single-`Application` pattern (emporia) points `path` at an in-repo Helm chart; app-of-apps (KubernetesTracker) exists but is heavier than needed here. ESO-managed Secrets use `ignoreDifferences` on `/data`.
  - **Canonical Helm chart skeleton to model on:** `/Users/michael/_code/k8s/KubernetesTracker/argocd/base/frontend` (cleaner/more parameterized than emporia). Templates: `_helpers.tpl` (name/fullname/labels/selectorLabels), `deployment.yaml`, `service.yaml`, `ingress.yaml`, `certificate.yaml`, plus `externalsecret.yaml` for our secrets. `values.yaml` parameterizes image (repo/tag/pullPolicy), deployment resources, service, ingress (`className: nginx`, annotations `ssl-redirect: "true"` + `backend-protocol: "HTTP"`), `tls`, a `certificate` block, `externalDNS.target: in.k8s.blacktoaster.com`, and `namespace`.
  - **The Certificate must be created explicitly** — the chart includes a `templates/certificate.yaml` rendering a cert-manager `Certificate` (`issuerRef` → name `vault-issuer`, kind `ClusterIssuer`, group `cert-manager.io`). The TLS secret name is shared between the `Certificate.spec.secretName` and `ingress.tls[].secretName`. cert-manager does NOT auto-issue from an ingress annotation in this cluster; the explicit Certificate resource is required.
- **Cluster access:** kubectx is available for reference only — do NOT change anything in the live cluster during this work.
- **User's Gmail app password** is already generated and will live in Vault.

## Constraints

- **Tech stack**: Go — the server must be written in Go.
- **Tech stack**: Email via Gmail SMTP using an app password (not the Gmail API).
- **Transport**: Remote MCP over streamable HTTP (hosted service), not stdio.
- **Security**: Bearer-token auth on the server; fixed recipient only.
- **Secrets**: POC uses Vault + existing ESO; design must allow AWS Secrets Manager as an alternate backend without rework.
- **Deployment**: Container image → Helm chart → ArgoCD on the user's k8s cluster; conform to existing emporia/ESO/ingress conventions.
- **Operational**: Read-only on the live Kubernetes cluster — no kubectl mutations as part of this project.

## Key Decisions

| Decision | Rationale | Outcome |
|----------|-----------|---------|
| Go for the server | User requirement; good fit for a small static-binary k8s service | — Pending |
| Remote streamable-HTTP MCP transport | Server is hosted in k8s; agents connect over the network | — Pending |
| Gmail SMTP + app password | Credential already provisioned; simplest delivery path | — Pending |
| Fixed recipient (michael@blacktoaster.com) | Limits blast radius; agents can't email arbitrary people | — Pending |
| Bearer-token auth | Endpoint is externally exposed via ingress; needs a guard | — Pending |
| Markdown body + optional status tag, rendered to multipart HTML/plaintext | Agents author markdown naturally; status tag makes "waiting" emails scannable | — Pending |
| Pluggable secret backend (Vault now, AWS SM later) | User deploys across environments; avoid lock-in | — Pending |
| ESO + `vault-backend` ClusterSecretStore for POC | Matches existing cluster convention (emporia) | — Pending |
| Email-only for v1, Slack deferred | Slack app pending approval | — Pending |
| Devcontainer + VS Code debug setup mirrored from SpinWheel | Wants to run/debug locally and connect a local MCP client; reuse a proven local-dev pattern | — Pending |

## Evolution

This document evolves at phase transitions and milestone boundaries.

**After each phase transition** (via `/gsd-transition`):
1. Requirements invalidated? → Move to Out of Scope with reason
2. Requirements validated? → Move to Validated with phase reference
3. New requirements emerged? → Add to Active
4. Decisions to log? → Add to Key Decisions
5. "What This Is" still accurate? → Update if drifted

**After each milestone** (via `/gsd:complete-milestone`):
1. Full review of all sections
2. Core Value check — still the right priority?
3. Audit Out of Scope — reasons still valid?
4. Update Context with current state

---
*Last updated: 2026-06-25 after initialization*
