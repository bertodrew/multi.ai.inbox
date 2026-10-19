# Security Penetration Test Report

**Generated:** 2026-10-18 21:55:38 UTC

# Executive Summary

A white-box security review of the multi.ai.inbox landing application (mailcow-dockerized fork, PHP 8 + nginx) identified four confirmed weaknesses: two critical, two medium. The most serious finding is a session-fixation flaw that lets an anonymous attacker lock a visitor into a known session and inherit the Namecheap connection the visitor establishes on it, combined with a single global credential file that every connected session shares — so two users connecting sequentially silently overwrite each other's stored API key and each session proxies whoever connected last. Together these break the confidentiality boundary around the Namecheap API key and domain data. The rate-limiting protecting the connect endpoint is enforced per browser session only and is fully bypassed by scripted clients, and an unauthenticated status endpoint leaks internal state (session CSRF token, server egress IP, last connector's username) that materially aids the other attack chains.

**Overall risk posture:** Elevated — the stored Namecheap API key and domain data are not adequately isolated between users, and no application-level login exists on the landing app itself.

**Key findings**
- Session fixation on the Namecheap connect flow (Critical): client-chosen session identifiers are accepted and never rotated when the connection is established.
- Global credential file shared by all sessions (Critical): last-connector-wins overwrite, cross-user credential mixing, and cross-user deletion via disconnect.
- Brute-force protection bypassed by cookie rotation (Medium): the 5-attempts/10-minute lockout never applies to a client that omits the session cookie.
- Unauthenticated status endpoint discloses the session CSRF token, mailcow origin, server public IP, and last connector's username (Medium).

**Business impact**
- Exposure of Namecheap domain inventories and DNS records across users of a single instance, with the stored API key usable by whoever connects last and re-playable by an attacker who fixed a session.
- The deployment can be abused as an unthrottled relay for Namecheap API-key guessing in production, degrading the service and Namecheap-side reputation.

The controls that did hold were confirmed with equal rigor: the MCP endpoint rejects every request not bearing the exact mailcow API key before any tool executes; the CSRF comparison is timing-safe and the token is session-bound; the connect endpoint cannot be turned into user-controlled SSRF; and the upstream XML parsing is not exposed to XXE.

A separate operational incident during testing — two source files accidentally deleted from the mounted workspace — was fully restored verbatim and verified; it is not a security finding.

# Methodology

Conducted as a white-box review per the OWASP WSTG and PTES methodology, combining full source review with live dynamic validation.

**Engagement type:** White-box (source at `landing/` mounted read-write, live app at http://host.docker.internal:8080, mock Namecheap API at nc-mock:8088 inside the target compose network).

**Scope:** The landing application only — `php/bootstrap.php`, `connect.php`, `namecheap.php`, `status.php`, `domains.php`, `dns.php`, `disconnect.php`, `mcp.php`, `nginx/landing.conf`, static `html/` pages and assets. Upstream mailcow-dockerized platform code and the mailcow UI itself were out of scope.

**Activities**
1. Reconnaissance and mapping: full repository read, endpoint/parameter inventory, trust-boundary derivation, and a shared threat model established before active testing.
2. Static analysis: data-flow tracing of every user-controlled input to every sink (curl handles, session writes, file stores, JSON responses); repo-wide sweeps for missing controls (session regeneration, strict mode, IP limiters, Origin/Referer checks, key allowlists).
3. Dynamic validation: five specialized validation agents reproduced every candidate against the live target with fresh cookie jars — session-fixation mechanics, cross-user credential overwrite, CSRF forgery attempts (header/body fallbacks, cross-origin contexts, token replay), in-window lockout vs cookie-rotation bypass, SSRF/URL-influence traces, XXE/expansion probes against the XML parser, MCP auth-gating tests, and domain-parameter injection payloads (26 variants).
4. Chaining: confirmed findings were combined into end-to-end attack paths (fixation → connection inheritance → cross-user deletion).
5. Coverage reconciliation: 16 surfaces assessed and closed with named controls; zero unresolved items.

# Technical Analysis

**Severity model** reflects demonstrated exploitability and impact.

1. **Session fixation on the Namecheap connect flow** (Critical, CVSS 9.6) — `php/bootstrap.php` starts the session with PHP defaults (`session.use_strict_mode` off, verified live: a tester-chosen 32-hex `MULTIAI_LANDING` cookie was accepted with no reissued cookie and a CSRF token minted for it), and no file in the repository calls `session_regenerate_id()`. The connect flow therefore installs `$_SESSION['nc_connected']` on the session ID the visitor arrived with, so an attacker who fixates the cookie value inherits the connected session the victim establishes on it — and the same fixed sid mints the session-bound CSRF token on `status.php`, defeating the CSRF design and enabling a forged disconnect that deletes the global credential store for every user.
2. **Global credential file shared by all sessions** (Critical, CVSS 9.9) — `/credentials/namecheap.json` is a single path (`php/bootstrap.php` `CRED_FILE`) wholesale-overwritten by every successful connect and read by every session passing `require_namecheap_session()`. Two users connecting sequentially mix accounts: the first session then reports the wrong owner and proxies the second user's Namecheap key; a disconnect deletes the store for everyone. Multi-tenant readiness is absent by construction.
3. **Brute-force protection per session only** (Medium, CVSS 5.3, CWE-307) — failed connect attempts are tracked in `$_SESSION['nc_attempts']` (5/10 min) with no IP-based or global limiter anywhere. Dynamic PoC: with a cookie jar, 5×401 then 429; with a fresh session per attempt, 8×401 and never 429 — the landing server functions as an unthrottled relay for Namecheap API-key guessing.
4. **Unauthenticated status oracle** (Medium, CVSS 5.3) — `GET /api/status.php` requires no credentials and returns the session CSRF token, mailcow origin, the server's public egress IP (via `api.ipify.org`, the value an operator whitelists in Namecheap), and — whenever a connection exists in the shared store — the last connector's Namecheap username and sandbox flag.

**Controls confirmed holding (ruled out with named evidence)**
- MCP endpoint: the Bearer `MAILCOW_API_KEY` check (php/mcp.php:107-114) runs before every method dispatch — `initialize`, `notifications/`, `tools/list`, `tools/call` all unreachable pre-auth (dynamic 401s); comparison is `hash_equals` (timing-safe); the Bearer regex cannot smuggle multiple tokens; an unset key fails closed (verified in a live PHP harness).
- `set_domain_settings` attrs passthrough forwards arbitrary keys to mailcow's edit API, but the only caller must already hold the read-write key — within the MCP trust boundary.
- CSRF on `connect.php`/`disconnect.php` holds without fixation: `hash_equals` + session-bound 128-bit token + SameSite=Lax + POST-only; all direct forgery attempts failed. Residual hardening gaps: token non-rotating, pre-auth fetchable, POST-body fallback accepted, no Origin/Referer check.
- SSRF: no user-controlled egress URL exists — `nc_api_url()` accepts only a boolean; `detect_public_ip()` calls a fixed endpoint; repo-wide data-flow sweep found no user input reaching any URL sink.
- XXE: `@simplexml_load_string` without `LIBXML_NOENT`; libxml 2.15.3 disables external entities by default (canary read fails; succeeds only with the flag).
- `dns.php` domain regex rejected all 26 injection payloads (traversal, encoding, null bytes, punycode edge cases, metadata IPs); nginx static root does not expose `/credentials`; `access_log off` on `/api/` prevents key leakage into logs.

**Systemic themes:** authorization is enforced per browser session rather than per identity — no application-level login exists on the landing app, so every connector boundary rests on a PHP session flag that a fixated or rotated cookie trivially controls; shared mutable state (one credential file) substitutes for per-user scoping; and rate limiting is coupled to the same client-controlled session construct.

# Recommendations

**Immediate**
1. Remediate session fixation: enforce `session.use_strict_mode=1` before `session_start()` in `php/bootstrap.php`, and call `session_regenerate_id(true)` on the authenticated transition in `php/connect.php` (full patch filed in the session-fixation report).
2. Scope the credential store per session: resolve `store_credentials()` / `read_credentials()` / `delete_credentials()` to a per-session path derived from the PHP session ID, keeping the existing lock-plus-atomic-rename write pattern (patch filed in the global-credential-file report).

**Short-term**
3. Enforce brute-force protection outside the session: add a per-remote-address failed-attempt counter (file-based store under the existing 0700 credential directory) checked before the session counter, with the entry cleared on successful connect (patch filed in the rate-limit report); add an nginx `limit_req` rule on `location /api/` for connection-level defense in depth.
4. Restrict `GET /api/status.php`: gate the `connected` / `username` / `sandbox` fields behind the connected-session check, and stop exposing the session CSRF token and server public IP in the unauthenticated response (patch filed in the information-disclosure report).

**Medium-term**
5. Harden the CSRF design: rotate `$_SESSION['csrf']` after successful state changes, drop the `$_POST['csrf']` body fallback so the token must arrive via the `X-Landing-CSRF` header, and add an Origin/Referer check on state-changing POSTs as defense in depth.
6. For multi-tenant readiness, add an application-level identity (authenticated session per user) before exposing Namecheap connections, so credential scoping and audit logging attach to real users rather than anonymous browser sessions.

**Retest & validation guidance:** re-run the session-fixation PoC (fixed sid must now be rejected with strict mode and invalidated by regeneration), the two-session connect/overwrite sequence (session A must keep its own credential after B connects), the 5-fail lockout against a fresh-session attacker (must now return 429), and the unauthenticated status disclosure (connection fields and IP/CSRF must be absent). If the mock Namecheap host is brought up in the test network, replay the connect-success chain end-to-end to confirm the inheritance fix against a live connection.

