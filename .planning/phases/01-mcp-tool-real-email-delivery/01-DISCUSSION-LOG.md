# Phase 1: MCP Tool + Real Email Delivery - Discussion Log

> **Audit trail only.** Do not use as input to planning, research, or execution agents.
> Decisions are captured in CONTEXT.md — this log preserves the alternatives considered.

**Date:** 2026-06-25
**Phase:** 1-MCP Tool + Real Email Delivery
**Areas discussed:** Email presentation, Status & subject format, Tool result & errors
**Areas offered but not selected:** Tool description framing

---

## Email presentation

### Q1 — What should the email body look like in the inbox?

| Option | Description | Selected |
|--------|-------------|----------|
| Styled wrapper | Markdown→HTML body wrapped in a template: status badge/header + body + footer | ✓ |
| Bare HTML | Just rendered markdown→HTML, no chrome | |
| Body + minimal footer | Body + one-line footer, no status badge | |

**User's choice:** Styled wrapper

### Q2 — How much styling effort for the POC?

| Option | Description | Selected |
|--------|-------------|----------|
| Clean & minimal | Colored status banner, system-font body, thin divider, muted footer; inline CSS only; no branding | ✓ |
| Just-functional | Single colored status pill line, no banner/divider | |
| Richer branded | Banner + wordmark/icon header + deliberate typography | |

**User's choice:** Clean & minimal

### Q3 — How to handle the footer given locked params?

| Option | Description | Selected |
|--------|-------------|----------|
| Timestamp only | Footer shows timestamp ("via mcp-notify"); no agent identity; param set unchanged | ✓ |
| Add optional 'source' param | Add optional source/agent field shown in footer | |
| No footer | Drop footer entirely | |

**User's choice:** Timestamp only
**Notes:** Keeping subject/body/status as the exact param set was a deliberate choice — no scope expansion.

---

## Status & subject format

### Q1 — How should the status tag appear in the subject?

| Option | Description | Selected |
|--------|-------------|----------|
| Emoji + word | `[✅ Completed]` / `[⏳ Waiting]` / `[ℹ️ Info]` / `[❌ Error]` | |
| Word only | `[Completed]` / `[Waiting]` / `[Info]` / `[Error]` | ✓ |
| Emoji only | ✅ / ⏳ / ℹ️ / ❌ prefix, no word | |

**User's choice:** Word only
**Notes:** Supersedes the `[⏳ Waiting]` emoji sketch in ROADMAP/REQUIREMENTS (that was illustrative). Banner color carries the visual signal instead of emoji.

### Q2 — What happens when status is omitted?

| Option | Description | Selected |
|--------|-------------|----------|
| Default to 'info' | Omitted → treated as info, `[Info]` prefix, blue banner | ✓ |
| No tag at all | Omitted → no prefix, neutral/no banner | |

**User's choice:** Default to 'info'

---

## Tool result & errors

### Q1 — On a successful send, what should the tool return?

| Option | Description | Selected |
|--------|-------------|----------|
| Confirmation + details | Confirmation text + message-id + timestamp | ✓ |
| Minimal 'sent' | Terse 'Notification sent.' | |
| Structured payload | Structured fields (recipient, message-id, status, timestamp) | |

**User's choice:** Confirmation + details

### Q2 — When a send fails, how should the tool report it?

| Option | Description | Selected |
|--------|-------------|----------|
| Clear error, no retry hint | MCP tool error with plain reason; no retry guidance | ✓ |
| Error + explicit 'do not retry' | Same + explicit do-not-retry instruction | |
| Error + retry guidance | Classify transient vs permanent, hint whether retry helps | |

**User's choice:** Clear error, no retry hint
**Notes:** Retry-safety (dedup + rate limit) is a Phase 2 concern; keep failure reporting simple until those protections exist.

---

## Claude's Discretion

- Agent-facing tool description wording (NOTIF-03 "when to call me" guidance) — user declined to constrain frequency posture; planning picks sensible defaults.
- Plaintext-fallback generation approach and input-validation behavior — standard approaches fine.
- Exact banner shades, divider weight, font stack, footer timestamp format.

## Deferred Ideas

- Optional `source`/agent field to distinguish callers (neo vs claude-code) — declined for Phase 1.
- Retry guidance / transient-vs-permanent error classification — deferred to align with Phase 2 protections.
- Emoji status tags — declined in favor of word-only.
