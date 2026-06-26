---
status: complete
phase: 01-mcp-tool-real-email-delivery
source: [01-01-SUMMARY.md, 01-02-SUMMARY.md, 01-03-SUMMARY.md, 01-04-SUMMARY.md]
started: 2026-06-26T05:42:19Z
updated: 2026-06-26T05:45:17Z
---

## Current Test

[testing complete]

## Tests

### 1. Cold Start Smoke Test
expected: Fresh start (`go run ./cmd/server` or "Debug mcp-notify") boots with no errors, listens on :8090, and an `initialize` POST to /mcp echoes a supported protocolVersion.
result: pass

### 2. Connect & List the Tool
expected: An MCP client (Inspector / Claude Code / curl) connects to http://localhost:8090/mcp; the handshake succeeds and exactly one tool `send_notification` is listed, exposing subject/body/status and NO recipient field.
result: pass

### 3. Send a Notification → Email Arrives
expected: Calling `send_notification` delivers a real email to michael@blacktoaster.com (Inbox, not Spam) and the tool returns a confirmation with recipient + message-id + RFC3339 timestamp.
result: pass

### 4. Status Tag & Banner Color
expected: status=completed → subject `[Completed]` + green banner; omitted status → `[Info]` + blue; error → red; waiting → amber. Subject prefix and banner color match the status.
result: pass

### 5. Markdown Rendering & Safety
expected: A markdown body renders as formatted HTML (headings, bold, lists) inside the styled wrapper; any `<script>`/raw HTML in the body is neutralized (no scripts run); plaintext fallback present.
result: pass

### 6. Failure Surfaces as a Tool Error
expected: A forced send failure (e.g., wrong app password) returns a clear MCP tool error with a readable reason — not a silent success.
result: pass

### 7. Fixed Recipient (Security)
expected: The tool schema has no recipient/to parameter; every notification is delivered only to the server-configured NOTIFY_RECIPIENT.
result: pass

### 8. Debug Experience
expected: The server runs under the "Debug mcp-notify" launch config inside the devcontainer; breakpoints in cmd/server/main.go and the send handler bind and hit during a tool call.
result: pass

## Summary

total: 8
passed: 8
issues: 0
pending: 0
skipped: 0
blocked: 0

## Gaps

[none]
