# Pitfalls Research

**Domain:** Go remote MCP server (streamable HTTP) → Gmail SMTP email, on Kubernetes (Helm + ArgoCD + ESO/Vault + NGINX/cert-manager)
**Researched:** 2026-06-25
**Confidence:** HIGH on infra/SMTP/ESO (official docs + known cluster conventions), MEDIUM on MCP transport specifics (spec moving fast — 2025-03-26 → 2025-06-18 → 2025-11-25, with a 2026-07 revision pending)

> Scope note: This is a greenfield email-only POC. "Scale" here means a handful of agent clients (neo, Claude Code) sending occasional notifications — not high throughput. Pitfalls are prioritized for *correctness, deployability, and not-getting-pwned*, not for load.

---

## Critical Pitfalls

### Pitfall 1: NGINX ingress buffers the streaming/SSE response and breaks MCP

**What goes wrong:**
The MCP streamable-HTTP transport returns responses as `text/event-stream` (SSE) for many interactions. NGINX, by default, buffers proxied responses — so the agent client hangs waiting for bytes that NGINX is holding, the connection looks alive but no events arrive, and tool calls time out or never resolve. Worse, the default `proxy_read_timeout` (60s) kills any GET stream the server holds open.

**Why it happens:**
`proxy_buffering on` is the NGINX default and the right choice for normal request/response APIs, so nobody thinks to change it. Streaming is the one case where it silently corrupts behavior. The connection succeeds at the TLS/HTTP layer, so it doesn't look like an ingress problem.

**How to avoid:**
- Set ingress annotations: `nginx.ingress.kubernetes.io/proxy-buffering: "off"`, `nginx.ingress.kubernetes.io/proxy-read-timeout: "3600"`, `nginx.ingress.kubernetes.io/proxy-send-timeout: "3600"`.
- Belt-and-suspenders: have the Go server set the `X-Accel-Buffering: no` response header on stream responses — NGINX honors this per-response even if an annotation is missed.
- Ensure HTTP/1.1 to upstream (ingress default) so chunked streaming works.

**Warning signs:**
Tool call hangs ~60s then errors; `curl -N` directly against the Pod/Service streams fine but through the ingress it stalls; events arrive in one burst at the end instead of incrementally.

**Phase to address:** Ingress/TLS exposure phase. Verify with a direct `curl -N` through the public hostname before declaring the endpoint "working."

---

### Pitfall 2: MCP protocol-version / handshake mismatch between the Go SDK and the agent clients

**What goes wrong:**
Claude Code (and neo) initialize with a specific `protocolVersion` and, on every subsequent HTTP request, send an `MCP-Protocol-Version` header. If the server pins/declares a version the client doesn't support — or ignores the header and answers in a different revision's shape — the handshake fails or tools never register. The spec is a moving target (2025-03-26, 2025-06-18, 2025-11-25, plus a 2026-07 revision that *removes* protocol-level sessions/`Mcp-Session-Id`), so a server built against one revision can mis-negotiate with a newer client.

**Why it happens:**
Developers test against one client at one point in time and assume the handshake is static. The spec note that "if no `MCP-Protocol-Version` header is received the server should assume `2025-03-26`" leads people to hardcode that default and never echo the negotiated version.

**How to avoid:**
- Use the **official `github.com/modelcontextprotocol/go-sdk`** (maintained with Google) rather than hand-rolling the transport — it implements version negotiation and the streamable-HTTP handler correctly. Pin a known SDK version and read its release notes.
- During `initialize`, negotiate down to a version both sides support; echo the negotiated `protocolVersion`. Respect the inbound `MCP-Protocol-Version` header on later requests rather than assuming.
- Test the handshake against **both** real clients (Claude Code and neo) early, not just a curl script.

**Warning signs:**
`initialize` returns but `tools/list` is empty; client logs "unsupported protocol version" or "protocol mismatch"; works in one agent but not the other; works locally with the SDK's own example client but not Claude Code.

**Phase to address:** MCP server core phase. Add a smoke test that runs the real client handshake end-to-end.

---

### Pitfall 3: Session handling assumptions — `Mcp-Session-Id`, multiple replicas, and statefulness

**What goes wrong:**
Streamable HTTP may assign an `Mcp-Session-Id` at `initialize` that the client must echo on every later request. If the server stores session state in memory and you run >1 replica behind the Service, a follow-up request can land on a replica that has never seen that session → "session not found" / re-initialize loops. (Note: the pending 2026-07 spec removes protocol-level sessions entirely, so building heavy session state now is also future-debt.)

**Why it happens:**
Default Deployment thinking ("just scale replicas") collides with in-memory session affinity. Nobody notices with a single replica in dev.

**How to avoid:**
- For this POC, run a **single replica** (`replicas: 1`) and keep the tool **stateless** — `send_notification` needs no cross-request session state. Document this explicitly.
- If you ever scale out, either make sessions stateless (stuff everything needed into the token/request) or add session affinity — but prefer stateless.
- Don't over-invest in session machinery the SDK already handles; let the SDK own session lifecycle.

**Warning signs:**
Intermittent "session expired / not initialized" errors that correlate with replica count > 1 or a rollout; works right after deploy then breaks after a Pod restart.

**Phase to address:** MCP server core + Helm deployment phase (set `replicas: 1`).

---

### Pitfall 4: Bearer-token auth that breaks (or bypasses) the MCP handshake

**What goes wrong:**
Two failure modes: (a) auth middleware rejects the very first `initialize` POST because it expects a session that doesn't exist yet, or rejects the SSE GET because the client can't always set custom headers on an EventSource — so the handshake never completes; or (b) the opposite — a health/metrics path or the MCP path is left unauthenticated, exposing the email tool to the internet.

**Why it happens:**
People bolt auth on after the transport works and don't account for the handshake ordering (auth must precede MCP but apply uniformly to POST and GET on the MCP path). Health endpoints get excluded from auth and then accidentally leak more than intended.

**How to avoid:**
- Apply bearer-token middleware uniformly to the MCP endpoint (both POST and the streaming GET). Validate `Authorization: Bearer <token>` with a **constant-time compare** before dispatching to the MCP handler.
- Return `401` with `WWW-Authenticate: Bearer` on failure (clean, debuggable).
- Keep `/healthz`/`/readyz` unauthenticated but make them reveal *nothing* (no version, no config) — and never put the MCP tool behind them.
- Confirm Claude Code / neo can attach the bearer token to the remote MCP connection (they support a configured auth header for remote servers) before assuming the scheme works.

**Warning signs:**
`initialize` returns 401; streaming GET 401s while POST works; or — the scary one — an unauthenticated `curl https://host/mcp` succeeds.

**Phase to address:** Auth phase, immediately after transport works. Add a test asserting that a missing/wrong token gets 401 on every MCP verb.

---

### Pitfall 5: Gmail SMTP port/TLS mismatch and `From`-address rejection

**What goes wrong:**
Auth or send fails because of a port/TLS mismatch: port **587 = STARTTLS** (start plaintext, upgrade), port **465 = implicit TLS** (TLS from byte one). Using a 587 client flow against 465 (or vice-versa) hangs or errors. Separately, Gmail **rejects or rewrites the `From`** if it doesn't match the authenticated account (or a configured "Send mail as" alias) — so `send_notification` "succeeds" but the From is silently the raw Gmail account, or the send is rejected as `550`.

**Why it happens:**
Go's stdlib `net/smtp` is low-level and easy to wire to the wrong TLS mode; tutorials mix 465 and 587 freely. The From-alignment rule is invisible until Gmail enforces it.

**How to avoid:**
- Pick **one** transport explicitly. Recommended: `smtp.gmail.com:587` + STARTTLS, with an explicit TLS config (verify `ServerName: smtp.gmail.com`). Prefer a maintained library (e.g. `wneessen/go-mail` or `gomail`) over hand-rolled `net/smtp` to get STARTTLS/465 handling right.
- Set `From` to the **authenticated Gmail account** (or an alias verified under "Send mail as"). For this POC, sender and recipient are both `michael@blacktoaster.com` — confirm that address *is* the authenticated account or a verified alias.
- Set a sane timeout on the SMTP dial/send so a stuck connection doesn't wedge a tool call.

**Warning signs:**
`535 5.7.8 Username and Password not accepted` (auth/app-password issue), `550` From rejection, connection hangs on send (wrong TLS mode), or emails arrive `From` the wrong address.

**Phase to address:** Email-delivery core phase. Test an actual send to the real inbox early.

---

### Pitfall 6: Gmail app password / bearer token leaking into logs or git

**What goes wrong:**
The app password or bearer token ends up in: structured logs (logging the full request including the `Authorization` header, or logging SMTP debug traces that include AUTH), an error message, a `values.yaml` committed to git, or a `kubectl describe` of an Env var. Once in git history or a log aggregator, it's compromised.

**Why it happens:**
Default request loggers dump headers. SMTP libraries have debug modes that print credentials. Helm `values.yaml` is git-tracked and tempting for "just for now" secrets. ENV-injected secrets show up in `describe pod`/crash dumps.

**How to avoid:**
- Source both secrets from **Vault via ESO** into a Kubernetes Secret; mount as env or file, never hardcode. Keep `values.yaml` secret-free (use the git-ignored `values.local.yaml` for any local override).
- Redact `Authorization` and credential fields in any request/SMTP logging. Never enable SMTP wire-debug in production.
- Add a pre-commit / CI secret scan (gitleaks) so an accidental commit fails the build.
- Treat the app password as rotatable: regenerating it in Google takes seconds; design so rotation is just a Vault write + version bump.

**Warning signs:**
Grepping logs for the token prefix returns hits; `git log -p` shows a credential; `kubectl describe` shows the value in plaintext env.

**Phase to address:** Secrets/ESO phase + logging setup. Verify with a log grep and a gitleaks run before exposing the endpoint.

---

### Pitfall 7: ESO `ExternalSecret` version pin won't resync until manually bumped

**What goes wrong:**
This cluster's convention pins a `version` in the `ExternalSecret` (intentional, for auditability). You rotate the app password or bearer token in Vault, expect ESO to pull it, and **nothing changes** — because ESO is honoring the pinned version. The app keeps using the stale credential and auth/email starts failing after a Google-side rotation.

**Why it happens:**
The pin is deliberate but non-obvious; people assume `refreshInterval` alone makes ESO track the latest. The Pod also won't pick up a changed Secret without a restart unless you wire reloader/checksum annotations.

**How to avoid:**
- Document the rotation runbook explicitly: write new secret to Vault → **bump the `version` in the `ExternalSecret`** → ESO resyncs the K8s Secret → restart the Deployment (or use a `reloader`/pod-template checksum annotation) so the Pod re-reads it.
- Match the existing emporia `ExternalSecret` shape exactly (KV v2 path `secret/data/apps/mcp-notify/*`, the `vault-backend` ClusterSecretStore, property keys).
- Set a reasonable `refreshInterval` but understand it's gated by the version pin.

**Warning signs:**
Vault has the new value but `kubectl get secret -o yaml` shows the old (base64) value; ESO `status` shows `SecretSynced` at an old `version`; app auth/SMTP fails right after a rotation.

**Phase to address:** Secrets/ESO phase. Add the rotation runbook to docs; verify a deliberate version bump propagates.

---

### Pitfall 8: ArgoCD and ESO fight over the managed Secret (perpetual OutOfSync)

**What goes wrong:**
ESO writes the `data` of the Kubernetes Secret; ArgoCD sees that `data` as drift from the (empty/templated) manifest and tries to revert it — flapping `OutOfSync`/`Synced`, and with self-heal on, ArgoCD can blow away the ESO-populated values, breaking auth/SMTP.

**Why it happens:**
ArgoCD doesn't know the Secret's `data` is owned by another controller. The `ExternalSecret` CRD is in git; the resulting `Secret` is not (and shouldn't be).

**How to avoid:**
- Only commit the `ExternalSecret` resource to git, never the resolved `Secret`.
- Add `ignoreDifferences` on the Secret's `/data` (and `/metadata` as needed) in the ArgoCD `Application` — this matches the existing emporia/ESO convention. Confirm it's present before enabling auto-sync/self-heal.

**Warning signs:**
ArgoCD shows the `Secret` perpetually `OutOfSync`; sync history shows repeated reverts; secret values vanish after an auto-sync.

**Phase to address:** ArgoCD wiring phase. Verify the app reaches steady `Synced` with self-heal on.

---

### Pitfall 9: cert-manager certificate never issues / external-dns record timing → endpoint unreachable or untrusted

**What goes wrong:**
The ingress comes up but HTTPS fails: cert-manager hasn't issued the cert yet (CertificateRequest stuck against the `vault-issuer`), or external-dns hasn't created/propagated the A/CNAME for `mcp-notify.k8s.blacktoaster.com`, so the client can't resolve the host or gets a self-signed/`ERR_CERT` warning. Agents then can't connect at all, which looks like an MCP bug.

**Why it happens:**
Cert issuance and DNS propagation are async and racy; people test immediately after `kubectl apply` and conclude "MCP is broken." Issuer misconfig (wrong `ClusterIssuer` name/ref) silently leaves the Certificate `False/Pending`.

**How to avoid:**
- Reference the existing `ClusterIssuer` named `vault-issuer` exactly; mirror emporia's `Certificate`/ingress TLS annotations and the external-dns target (`in.k8s.blacktoaster.com`) and domain filter (`blacktoaster.com`).
- After deploy, **wait and verify**: `Certificate` is `Ready=True`, the TLS Secret exists, DNS resolves, then `curl` the HTTPS host. Don't test in the first 60s.

**Warning signs:**
`Certificate` shows `Ready=False`/`Issuing`; `CertificateRequest` pending; `dig mcp-notify.k8s.blacktoaster.com` returns nothing; browser/`curl` cert errors.

**Phase to address:** Ingress/TLS phase. Make "cert Ready + DNS resolves + HTTPS 200" an explicit acceptance check.

---

### Pitfall 10: Graceful shutdown drops in-flight emails on rollout

**What goes wrong:**
During an ArgoCD sync / rolling update, the Pod gets SIGTERM and exits immediately, killing an in-progress `send_notification` mid-SMTP-send. The agent sees a tool error or, worse, a silent failure where it's ambiguous whether the email went out.

**Why it happens:**
Default Go `http.ListenAndServe` doesn't handle SIGTERM; the container dies on the spot. SMTP sends take a beat (connect + STARTTLS + send), widening the window.

**How to avoid:**
- Implement graceful shutdown: trap SIGTERM, stop accepting new connections, `http.Server.Shutdown(ctx)` with a timeout that exceeds a worst-case SMTP send, and finish in-flight sends.
- Set `terminationGracePeriodSeconds` comfortably above the SMTP timeout; ensure readiness probe flips to not-ready on shutdown so the Service stops routing.

**Warning signs:**
Tool calls error specifically during deploys/rollouts; duplicate or missing emails after a sync.

**Phase to address:** Helm/deployment phase (shutdown + probes) alongside the SMTP core.

---

### Pitfall 11: Email tool abuse — recipient override, content injection, and unbounded sending

**What goes wrong:**
The endpoint is internet-exposed. Three abuse vectors: (a) if the recipient is ever taken from tool input, a leaked token turns this into an open relay / spam cannon to arbitrary addresses; (b) markdown→HTML rendering without sanitization lets a crafted body inject `<script>`/`<img onerror>`/dangerous links into the HTML email (and arbitrary CRLF in subject → header injection); (c) no rate limit means a compromised token (or a runaway agent loop) floods the inbox and burns the Gmail daily quota.

**Why it happens:**
POC mindset trusts the caller. Markdown libraries emit raw HTML by default. Subject lines are naively concatenated. "It's just me calling it" ignores that it's publicly reachable.

**How to avoid:**
- **Hardcode the recipient** server-side (`michael@blacktoaster.com`); never accept a `to` field. This is already a stated requirement — enforce it in code, not just docs.
- Sanitize rendered HTML (e.g. `bluemonday` UGC policy after markdown→HTML) so no script/onerror/javascript: survives. Strip CRLF/control chars from the subject and from the status tag before building headers.
- Add a simple per-process rate limit (e.g. N emails/minute + N/day) well under Gmail's limits, returning a clear tool error when exceeded.
- Keep the bearer token long/random (32+ bytes), store only in Vault, and have a rotation path (Pitfall 6/7).

**Warning signs:**
Tool schema exposes a recipient/`to` param; rendered emails contain unescaped HTML from the body; subject contains injected newlines; a test can trigger unbounded sends.

**Phase to address:** Email-rendering + auth/hardening phases. Add tests: recipient is immutable; a `<script>` body is neutralized; rate limit trips.

---

### Pitfall 12: Image tag pinning (`latest`) defeats GitOps and hides what's deployed

**What goes wrong:**
Helm `values.yaml` references `…/mcp-notify:latest`. ArgoCD syncs the manifest, but since the tag string didn't change, ArgoCD sees no diff and doesn't roll Pods when you push a new image — or it rolls unpredictably depending on `imagePullPolicy`. You can't tell what's actually running, and rollbacks are meaningless.

**Why it happens:**
`latest` is the path of least resistance in early dev; GitOps wants the desired state (image digest/tag) to *be in git*.

**How to avoid:**
- Pin an immutable tag (semver or git SHA) in `values.yaml`: `registry.k8s.blacktoaster.com/mcp-notify/server:<sha>`. Deploy = commit a tag bump = ArgoCD syncs the new ReplicaSet.
- Use `imagePullPolicy: IfNotPresent` with immutable tags. Avoid `latest` entirely.

**Warning signs:**
New image pushed but Pods unchanged; `kubectl describe` shows `:latest`; can't answer "which commit is running?"

**Phase to address:** CI/image + Helm/ArgoCD phase.

---

### Pitfall 13: Readiness/liveness probes that mislead the Service or kill a healthy Pod

**What goes wrong:**
Two opposite failures: (a) a liveness probe that hits an endpoint requiring auth or dependent on SMTP/Vault → probe 401s/fails → kubelet restarts a perfectly healthy Pod in a crash loop; or (b) no readiness probe → the Service routes traffic before the server (and its secrets) are ready, so the first agent calls fail.

**Why it happens:**
People point probes at `/mcp` (authed) or conflate "can reach Gmail" with "is the process alive." Liveness should test the process; readiness should test "ready to serve."

**How to avoid:**
- Dedicated unauthenticated `/healthz` (liveness: process up) and `/readyz` (readiness: secrets loaded, listener bound). Do **not** make liveness depend on Gmail/Vault reachability — a transient SMTP outage shouldn't restart the Pod.
- Keep probe responses trivial and side-effect-free.

**Warning signs:**
Pod `CrashLoopBackOff` with healthy logs; restarts spike when Gmail/Vault blips; first request after deploy fails.

**Phase to address:** Helm/deployment phase.

---

## Technical Debt Patterns

| Shortcut | Immediate Benefit | Long-term Cost | When Acceptable |
|----------|-------------------|----------------|-----------------|
| Hand-rolled MCP transport instead of official Go SDK | No dependency to learn | Re-implement version negotiation/session/SSE; breaks on spec bumps (2025-11-25, 2026-07) | Never for this — use `modelcontextprotocol/go-sdk` |
| `net/smtp` stdlib instead of a mail lib | No dependency | Easy STARTTLS/465 mistakes, manual MIME multipart | Only if you genuinely need zero deps; prefer `go-mail` |
| Secret in `values.yaml` "just to test" | Fast local run | Leaks to git history; compromised credential | Never — use `values.local.yaml` (git-ignored) or ESO |
| `image: latest` | No tag bumping | GitOps can't track/rollback | Never in ArgoCD-managed deploy |
| Skipping HTML sanitization (markdown→HTML) | Less code | Injection into emails | Never — endpoint is public |
| Multiple replicas "for HA" | Feels robust | In-memory MCP sessions break across replicas | Defer; single replica is correct for POC |
| ArgoCD self-heal on before `ignoreDifferences` set | Hands-off | ESO Secret gets reverted | Never — set ignoreDifferences first |

## Integration Gotchas

| Integration | Common Mistake | Correct Approach |
|-------------|----------------|------------------|
| Gmail SMTP | Mixing 587 STARTTLS vs 465 implicit TLS; spoofed `From` | One explicit mode (587+STARTTLS); `From` = authenticated account/verified alias |
| Gmail deliverability | Assuming custom-domain `From` is auto-aligned | If `blacktoaster.com` is Workspace, ensure SPF/DKIM/DMARC configured or mail lands in spam; app-password SMTP relays through Google so alignment matters |
| ESO + Vault (KV v2) | Path/property mismatch (`secret/apps/...` vs `secret/data/apps/...`), forgetting version bump | Use KV v2 `secret/data/apps/mcp-notify/*`, exact property keys, bump `version` to rotate |
| ArgoCD + ESO | App fights ESO over Secret `/data` | `ignoreDifferences` on the Secret `/data`; only commit the `ExternalSecret` |
| NGINX ingress + SSE | Default buffering/60s timeout kills streams | `proxy-buffering: "off"`, long read/send timeouts, `X-Accel-Buffering: no` |
| cert-manager `vault-issuer` | Testing before issuance/DNS settle; wrong issuerRef | Mirror emporia refs; gate on `Certificate Ready=True` + DNS resolves |
| Claude Code / neo as MCP client | Assuming static protocol version; auth header not attachable | Negotiate version, respect `MCP-Protocol-Version` header; verify both clients can send the bearer token |

## Performance Traps

| Trap | Symptoms | Prevention | When It Breaks |
|------|----------|------------|----------------|
| Synchronous SMTP send blocks the tool call | Tool call latency = full SMTP round-trip; a slow Gmail send stalls the handler | Set SMTP dial/send timeouts; consider returning after enqueue (but keep it simple for POC) | Noticeable under any Gmail slowness; not load-related |
| Gmail daily sending limit | Sends start `550`-rejecting after a threshold (~500/day consumer, ~2000/day Workspace) | App-side rate limit; this POC's volume is tiny — limit guards against runaway loops, not traffic | A compromised token or agent retry-loop |
| No connection reuse / per-send TLS handshake | Slight latency per send | Fine at POC volume; only pool if volume grows | Never at expected scale |

## Security Mistakes

| Mistake | Risk | Prevention |
|---------|------|------------|
| Recipient taken from tool input | Open relay / spam from your Gmail; reputation/quota damage | Hardcode recipient server-side; no `to` param in schema |
| Unsanitized markdown→HTML | Script/`onerror`/malicious-link injection into emails | `bluemonday` sanitize after render; strip CRLF from subject (header injection) |
| Weak/static bearer token | Brute-force or leak → anyone emails you / abuses Gmail | 32+ byte random token in Vault; constant-time compare; rotation runbook |
| Auth excludes a path, or applies only to POST | Unauthenticated access to the tool via GET/stream | Apply auth uniformly to MCP POST + streaming GET; only trivial `/healthz` open |
| Credentials in logs / git / `describe` | Permanent credential compromise | Redact `Authorization`/SMTP debug; gitleaks in CI; secrets via ESO only |
| Verbose health/error endpoints | Info leak (version, config) aids attackers | Minimal `/healthz`/`/readyz`; generic error responses externally |

## UX Pitfalls

| Pitfall | User Impact | Better Approach |
|---------|-------------|-----------------|
| HTML-only email, no plaintext part | Renders poorly / flagged as spam in some clients | Multipart `text/html` + `text/plain` fallback (already a requirement — enforce it) |
| Status not reflected in subject | "Waiting on you" emails not scannable in inbox | Prefix subject with status tag (e.g. `[WAITING]`), as specified |
| Opaque tool errors to the agent | Agent can't tell if email sent; retries blindly → dupes | Return clear success/failure from `send_notification`; make idempotency/retry behavior explicit |
| Markdown that renders wrong (tables, code) | Ugly notifications | Pick a known markdown lib; test representative agent output (code blocks, lists) |

## "Looks Done But Isn't" Checklist

- [ ] **Streaming through ingress:** Often missing buffering-off — verify `curl -N` through `https://mcp-notify.k8s.blacktoaster.com` streams incrementally, not in one burst.
- [ ] **Handshake with real clients:** Often only curl-tested — verify Claude Code *and* neo both list and call the tool.
- [ ] **Auth on every verb:** Often only POST is checked — verify wrong/missing token → 401 on POST and streaming GET.
- [ ] **From-address alignment:** Often "sends" but From is wrong — verify the received email's From is `michael@blacktoaster.com`.
- [ ] **Deliverability:** Often lands in spam — verify a real email reaches the *inbox* (SPF/DKIM for the domain if Workspace).
- [ ] **Secret rotation:** Often never tested — verify bumping the `ExternalSecret` version propagates the new value to the Pod.
- [ ] **ArgoCD steady state:** Often flapping — verify `Synced/Healthy` with self-heal on and `ignoreDifferences` set.
- [ ] **Cert + DNS:** Often tested too early — verify `Certificate Ready=True`, DNS resolves, HTTPS trusted.
- [ ] **Graceful shutdown:** Often skipped — verify a rollout doesn't drop an in-flight send.
- [ ] **Recipient immutability:** Often only documented — verify code rejects/ignores any caller-supplied recipient.
- [ ] **Image pinning:** Often `latest` — verify `values.yaml` uses an immutable tag/SHA.

## Recovery Strategies

| Pitfall | Recovery Cost | Recovery Steps |
|---------|---------------|----------------|
| NGINX buffers stream | LOW | Add buffering-off annotations + `X-Accel-Buffering: no`; redeploy |
| Protocol version mismatch | MEDIUM | Upgrade/pin Go SDK to a revision matching clients; fix negotiation; re-test both clients |
| ESO version pin stale credential | LOW | Bump `ExternalSecret` version; restart Deployment |
| ArgoCD reverting ESO Secret | LOW | Add `ignoreDifferences` on `/data`; re-sync |
| Cert not issued | LOW–MEDIUM | Fix `issuerRef`; inspect CertificateRequest/Order events; wait for issuance |
| Leaked credential (logs/git) | HIGH | Rotate Gmail app password + bearer token in Vault, bump version, restart; scrub logs; purge git history |
| Email abuse via recipient override | HIGH | Hotfix to hardcode recipient; rotate token; audit sent mail |
| Dropped in-flight send on rollout | MEDIUM | Add graceful shutdown + grace period; verify with a rollout test |

## Pitfall-to-Phase Mapping

| Pitfall | Prevention Phase | Verification |
|---------|------------------|--------------|
| NGINX buffering breaks streaming | Ingress/TLS exposure | `curl -N` through public host streams incrementally |
| MCP protocol/handshake mismatch | MCP server core | Real Claude Code + neo handshake lists/calls tool |
| Session/replica statefulness | MCP core + Helm | `replicas: 1`; survives Pod restart |
| Bearer auth vs handshake | Auth/hardening | 401 on missing token across POST + GET |
| Gmail port/TLS + From rejection | Email-delivery core | Real send arrives, correct From |
| Credential leak (logs/git) | Secrets/ESO + logging | gitleaks clean; log grep clean |
| ESO version-pin resync | Secrets/ESO | Version bump propagates to Pod |
| ArgoCD vs ESO Secret fight | ArgoCD wiring | Steady `Synced` with self-heal + `ignoreDifferences` |
| cert/DNS timing | Ingress/TLS | `Certificate Ready=True` + DNS + trusted HTTPS |
| Graceful shutdown | Helm/deployment | Rollout drops no in-flight send |
| Email tool abuse | Email-render + auth/hardening | Recipient immutable; HTML sanitized; rate limit trips |
| `latest` image tag | CI/image + ArgoCD | `values.yaml` uses immutable tag; push rolls Pods |
| Misleading probes | Helm/deployment | No crashloop on Gmail/Vault blip; readiness gates traffic |

## Sources

- MCP transport spec (sessions, `Mcp-Session-Id`, `MCP-Protocol-Version` header, version-default behavior): https://modelcontextprotocol.io/specification/2025-11-25/basic/transports — HIGH
- MCP 2026-07 spec changes (removal of protocol-level sessions / `Mcp-Session-Id`): https://stacktr.ee/blog/mcp-2026-spec-changes and https://dev.to/akaranjkar08/mcp-spec-ships-july-28-every-breaking-change-and-how-to-migrate-4co8 — MEDIUM (pending revision)
- Official Go SDK (streamable HTTP, auth primitives): https://github.com/modelcontextprotocol/go-sdk and https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp — HIGH
- NGINX + SSE buffering/timeouts (`proxy_buffering off`, `X-Accel-Buffering: no`, long read timeout): https://oneuptime.com/blog/post/2025-12-16-server-sent-events-nginx/view and gin-gonic/gin#1589 — HIGH
- Gmail SMTP host/port/TLS, app passwords, sending limits, From alignment — Google Workspace SMTP docs + training data — HIGH
- ESO version pinning / KV v2 path conventions, ArgoCD `ignoreDifferences` for ESO Secrets — existing emporia cluster convention (PROJECT.md) + ESO docs — HIGH
- cert-manager `ClusterIssuer`/Certificate readiness, external-dns timing — cert-manager/external-dns docs + cluster convention — HIGH

---
*Pitfalls research for: Go remote MCP (streamable HTTP) → Gmail SMTP on K8s (Helm/ArgoCD/ESO/NGINX/cert-manager)*
*Researched: 2026-06-25*
