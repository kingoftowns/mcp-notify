# mcp-notify

> A remote MCP server that lets AI agents reach you by email — one tool call, well-formatted, delivered.

`mcp-notify` is a small [Model Context Protocol](https://modelcontextprotocol.io) server written in Go. It exposes a single tool, **`send_notification`**, that an agent (Claude Code, neo, …) calls with a subject and a Markdown body — e.g. "task completed" or "waiting on you" — and the server renders it and delivers it to a fixed inbox over Gmail SMTP. Delivery just works.

## How it works

```
agent ──(MCP / Streamable HTTP)──▶  /mcp  ──▶  Renderer ──▶  Channel ──▶  Gmail SMTP ──▶  your inbox
                                              (markdown→HTML)  (multipart)   (STARTTLS :587)
```

- **One tool, fixed recipient.** The recipient is server-side config only — the tool exposes no `to` field, so it can't be turned into a spam relay.
- **Nice emails.** Markdown is rendered to inline-styled HTML with a status-colored banner; a plaintext alternative is always included.
- **Proof of delivery.** A successful call returns the recipient, the SMTP message-id, and a timestamp.

## The tool

**`send_notification`**

| Field | Required | Description |
|-------|----------|-------------|
| `subject` | yes | Email subject. Prefixed automatically with a `[Status]` tag. |
| `body` | yes | Markdown body. Rendered to HTML; raw HTML/script is sanitized. |
| `status` | no | One of `completed`, `waiting`, `info`, `error`. Defaults to `info`. |

Status drives the subject tag and banner color:

| Status | Subject prefix | Banner |
|--------|----------------|--------|
| `completed` | `[Completed]` | 🟢 green |
| `waiting` | `[Waiting]` | 🟠 amber |
| `info` (default) | `[Info]` | 🔵 blue |
| `error` | `[Error]` | 🔴 red |

## Quick start (local)

Requires Go 1.26+ (the module pins toolchain `go1.26.4`).

```bash
# 1. configure
cp .env.example .env
#    fill GMAIL_APP_PASSWORD with a 16-char Gmail app password (no spaces)

# 2. run
set -a; . ./.env; set +a
go run ./cmd/server          # serves the MCP endpoint at http://localhost:8090/mcp
```

Connect any MCP client over Streamable HTTP:

```bash
# MCP Inspector (visual)
npx @modelcontextprotocol/inspector      # Transport: Streamable HTTP → http://localhost:8090/mcp

# or register in Claude Code
claude mcp add --transport http mcp-notify http://localhost:8090/mcp
```

Then call `send_notification` and check your inbox.

A VS Code **"Debug mcp-notify"** launch config and a devcontainer are included for breakpoint debugging.

## Configuration

All config is environment-first (see `.env.example`):

| Variable | Default | Notes |
|----------|---------|-------|
| `PORT` | `8090` | HTTP port; MCP endpoint served at `/mcp`. |
| `GMAIL_USERNAME` | — | Authenticated Gmail account; also the `From` address (keeps SPF/DMARC aligned). |
| `GMAIL_APP_PASSWORD` | — | 16-char Gmail **app password** (not your account password). |
| `NOTIFY_RECIPIENT` | — | Fixed recipient for every notification. |
| `BEARER_TOKEN` | — | Reserved for bearer auth (Phase 2); unused today. |

## Project layout

```
cmd/server/        composition root — config → service → MCP server on /mcp
internal/config/   env-based configuration loader
internal/notify/   Renderer (markdown→HTML), Channel (Gmail SMTP), NotificationService
internal/mcpserver/ send_notification tool + Streamable HTTP server
```

The `Renderer` / `Channel` / `NotificationService` interface seams keep later work (a Slack channel, pluggable secret backends) slotting in without rework.

## Development

```bash
go build ./...
go test ./...                                   # unit suite (no network)
go test -tags=integration ./internal/notify     # real Gmail send (needs creds)
go vet ./... && govulncheck ./...
```

## Security

- **Fixed recipient** — no recipient parameter on the tool (configured server-side only).
- **Input hardening** — Markdown bodies are sanitized with bluemonday (`UGCPolicy`) before emailing; subjects are CRLF-stripped to prevent header injection.
- **Secret hygiene** — the Gmail app password and bearer token are never logged.
- Bearer-token auth, rate-limiting, and content dedup are planned for Phase 2.

## Roadmap

| Phase | Scope | Status |
|-------|-------|--------|
| 1 | MCP tool + real email delivery (local) | ✅ complete |
| 2 | Bearer auth, secret seam, dedup + rate-limit, structured logging, health probes | planned |
| 3 | Distroless container image + Helm chart | planned |
| 4 | Secrets from Vault via External Secrets Operator | planned |
| 5 | Public NGINX ingress + cert-manager TLS | planned |
| 6 | ArgoCD GitOps deployment | planned |
| — | Slack delivery channel | future |

## Tech stack

Go · [modelcontextprotocol/go-sdk](https://github.com/modelcontextprotocol/go-sdk) · [wneessen/go-mail](https://github.com/wneessen/go-mail) · [goldmark](https://github.com/yuin/goldmark) · [bluemonday](https://github.com/microcosm-cc/bluemonday) · [caarlos0/env](https://github.com/caarlos0/env)
