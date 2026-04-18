# Unified Offline Operations Portal — Delivery & Architecture Audit (Report 2)

Audit date: 2026-04-18
Auditor: static review only
Scope: `/Users/mac/Eagle-Point Season 2/Task-24/repo/`
Prior report: `.tmp/audit_report-1.md` (and `.tmp/audit_report-1-fix_check.md`, which itself is stale — it was produced *before* commit `b7af2a2` "Address audit_report-1: close A1–A13")

---

## 1. Verdict

**Partial Pass** — with note of tightened scope.

The delivery is a complete, end-to-end, offline-capable stack (Go/Echo backend, React/TypeScript frontend, PostgreSQL schema, Docker compose, test runner, docs) whose implementation is squarely on the Prompt's business goal. The 13 findings from the prior audit (A1–A13) have been addressed in code (not merely documented), and new regression tests back the fixes. Remaining issues are Medium/Low scope-of-polish items: a dev-mode cryptographic default that is not actually "ephemeral", a sidebar/authorization drift in the SPA, a few silently-ignored audit write errors, and one lockout-path that skips the timing pad. No Blocker-class defect was found statically. Runtime behavior not verified (static-only).

---

## 2. Scope and Static Verification Boundary

### Reviewed
- Root: [README.md](repo/README.md), [start.sh](repo/start.sh), [run_tests.sh](repo/run_tests.sh), [docker-compose.yml](repo/docker-compose.yml), [docker-compose.test.yml](repo/docker-compose.test.yml), [.env.example](repo/.env.example), [.gitignore](repo/.gitignore)
- Docs: [docs/apispec.md](docs/apispec.md), [docs/design.md](docs/design.md), [docs/questions.md](docs/questions.md)
- Backend Go packages: [cmd/server](repo/backend/cmd/server/main.go), [cmd/seed](repo/backend/cmd/seed/main.go), [cmd/keygen](repo/backend/cmd/keygen/main.go), [internal/api](repo/backend/internal/api/), [internal/auth](repo/backend/internal/auth/), [internal/audit](repo/backend/internal/audit/audit.go), [internal/crypto](repo/backend/internal/crypto/crypto.go), [internal/filter](repo/backend/internal/filter/filter.go), [internal/geo](repo/backend/internal/geo/), [internal/httpx](repo/backend/internal/httpx/), [internal/lab](repo/backend/internal/lab/), [internal/order](repo/backend/internal/order/workflow.go), [internal/runtime](repo/backend/internal/runtime/runtime.go), [internal/store](repo/backend/internal/store/), [internal/search](repo/backend/internal/search/fuzzy.go), [internal/models](repo/backend/internal/models/models.go)
- Migrations: [migrations/0001_init.sql](repo/backend/migrations/0001_init.sql)
- Frontend: [frontend/src/App.tsx](repo/frontend/src/App.tsx), [src/api/client.ts](repo/frontend/src/api/client.ts), [src/components/OfflineMap.tsx](repo/frontend/src/components/OfflineMap.tsx), [src/hooks/useRecentSearches.ts](repo/frontend/src/hooks/useRecentSearches.ts), all pages and components, [nginx.conf](repo/frontend/nginx.conf), [Dockerfile](repo/frontend/Dockerfile)
- Tests: Go `*_test.go` under `internal/**`, Vitest `*.test.ts(x)` under `frontend/src/**`

### Not executed
- No `docker compose up`, no `go test`, no `npm test`, no `curl`. All conclusions are from reading source.
- No DB was started; Postgres integration assertions rely on static schema + query-text inspection only.

### Requires manual verification
- Runtime startup (`./start.sh`) — documented but not executed.
- Container-based test runner (`./run_tests.sh`) — CI shape looks correct, but actual test pass/fail unverified.
- Frontend visual rendering quality — visual inspection not performed in a browser.
- Postgres integration tests are gated on `INTEGRATION_DB` and skip cleanly without a DB; we cannot confirm they pass on a live Postgres.

---

## 3. Repository / Requirement Mapping Summary

**Prompt goal:** offline-first portal for a lab + fulfillment business. Core flows:
1. Auth (local user/pw, ≥10 chars, 5-fail/15-min lockout, Argon2id hash)
2. Customer register / search / lookup by address, with AES-GCM at-rest PII
3. Orders: placed → picking → dispatched → delivered → received → refunded, with OOS + 30-min picking-timeout exceptions and split-shipment suggestion
4. Lab: samples sampling→received→in_testing→reported, versioned reports with optimistic concurrency, abnormal-result flagging by configurable reference ranges, FTS search across titles+narratives, archive (readable, not deletable)
5. Dispatch: offline map pin with geofence validation + per-region fee quote using preloaded route table with Haversine fallback
6. Global search with typo-tolerance + last-20 per-user per-device recent list
7. Advanced filters: MM/DD/YYYY dates, status, tags, priority, price range in USD, sort + paginate; saved filters per user with anti-broad-export guard
8. Audit: immutable, actor + before/after + server time + workstation time
9. Admin: dictionaries, permissions, reference ranges, route table
10. Analyst: read-only operational analytics; bounded CSV export
11. No external network; PostgreSQL only; Go/Echo backend; React/TS frontend

**Mapping to implementation** (high-level):
| Prompt area | Primary backend | Frontend | Evidence |
|---|---|---|---|
| Auth + lockout + Argon2 | [internal/auth](repo/backend/internal/auth/password.go), [store_lockout.go](repo/backend/internal/auth/store_lockout.go), [api/auth.go](repo/backend/internal/api/auth.go) | [Login.tsx](repo/frontend/src/pages/Login.tsx), [useAuth.tsx](repo/frontend/src/hooks/useAuth.tsx) | password.go:18, store_lockout.go:48, auth.go:39 |
| Customers + AES-GCM PII | [api/customers.go](repo/backend/internal/api/customers.go), [crypto/crypto.go](repo/backend/internal/crypto/crypto.go) | [Customers.tsx](repo/frontend/src/pages/Customers.tsx), [CustomerDetail.tsx](repo/frontend/src/pages/CustomerDetail.tsx) | customers.go:33,37; crypto.go:64 |
| Order state machine + exceptions | [order/workflow.go](repo/backend/internal/order/workflow.go), [api/orders.go](repo/backend/internal/api/orders.go) | [Orders.tsx](repo/frontend/src/pages/Orders.tsx), [OrderDetail.tsx](repo/frontend/src/pages/OrderDetail.tsx), [OrderTimeline.tsx](repo/frontend/src/components/OrderTimeline.tsx) | workflow.go:38, orders.go:257 |
| Lab samples + reports + reference ranges + archive | [lab/*.go](repo/backend/internal/lab/), [api/lab.go](repo/backend/internal/api/lab.go) | [Lab.tsx](repo/frontend/src/pages/Lab.tsx), [ReportWorkspace.tsx](repo/frontend/src/components/ReportWorkspace.tsx) | lab.go:192, report.go:102, reference.go:117 |
| Dispatch pin + fee | [geo/*.go](repo/backend/internal/geo/), [api/dispatch.go](repo/backend/internal/api/dispatch.go) | [Dispatch.tsx](repo/frontend/src/pages/Dispatch.tsx), [OfflineMap.tsx](repo/frontend/src/components/OfflineMap.tsx) | polygon.go:48, distance.go:75, dispatch.go:14 |
| Global search + recent-20 | [search/fuzzy.go](repo/backend/internal/search/fuzzy.go), [filters_search.go:86](repo/backend/internal/api/filters_search.go) | [GlobalSearch.tsx](repo/frontend/src/components/GlobalSearch.tsx), [useRecentSearches.ts](repo/frontend/src/hooks/useRecentSearches.ts) | fuzzy.go, useRecentSearches.ts:7 |
| Advanced filter + saved filters + bounded export | [filter/filter.go](repo/backend/internal/filter/filter.go), [api/orders.go#QueryOrders](repo/backend/internal/api/orders.go), [api/export.go](repo/backend/internal/api/export.go) | [AdvancedFilters.tsx](repo/frontend/src/components/AdvancedFilters.tsx) | filter.go:84,170, orders.go:105, export.go:22 |
| Audit log (append-only) | [audit/audit.go](repo/backend/internal/audit/audit.go), [migrations/0001_init.sql](repo/backend/migrations/0001_init.sql) | — | audit.go:37, migrations/0001_init.sql:275 |
| Admin dictionaries | [api/admin.go](repo/backend/internal/api/admin.go), [api/permissions.go](repo/backend/internal/api/permissions.go), [api/settings.go](repo/backend/internal/api/settings.go) | [Admin.tsx](repo/frontend/src/pages/Admin.tsx) | admin.go:17, permissions.go, settings.go |
| Analytics | [api/analytics.go](repo/backend/internal/api/analytics.go) | [Analytics.tsx](repo/frontend/src/pages/Analytics.tsx), [BarChart.tsx](repo/frontend/src/components/BarChart.tsx) | analytics.go:22 |

---

## 4. Section-by-section Review

### 1. Hard Gates

#### 1.1 Documentation and static verifiability — **Pass**
- Startup path documented and wired: [README.md:62-97](repo/README.md), [start.sh](repo/start.sh), [docker-compose.yml](repo/docker-compose.yml). `start.sh` refuses a known placeholder key (start.sh:25-32) and polls `/api/health` for readiness.
- Test path documented: [README.md:99-123](repo/README.md), [run_tests.sh](repo/run_tests.sh), [docker-compose.test.yml](repo/docker-compose.test.yml). The test compose builds a dedicated `backend-test` stage (CMD: `go test -cover ./...`) and `frontend-test` stage (CMD: `npm test`).
- Config documented: [.env.example](repo/.env.example) names every variable; keygen utility provides a secure default generator ([cmd/keygen/main.go](repo/backend/cmd/keygen/main.go)).
- Project structure matches the tree laid out in the README. Entrypoints, handlers, storage, and migrations are statically consistent.

#### 1.2 Prompt deviation — **Pass**
Every implementation area maps to a Prompt requirement; no unrelated functionality was added. Persistence is PostgreSQL, transport is REST JSON, auth is local, encryption is AES-256-GCM per-env key, search uses tsvector + fuzzy client-side ranker — all aligned.

### 2. Delivery Completeness

#### 2.1 Coverage of explicit core requirements — **Pass**
Every prompt feature is represented in code:
- Global search: [filters_search.go:86-116](repo/backend/internal/api/filters_search.go), with typo-tolerant fuzzy rank ([search/fuzzy.go](repo/backend/internal/search/fuzzy.go)); frontend recent-20 per user ([useRecentSearches.ts:7](repo/frontend/src/hooks/useRecentSearches.ts)).
- Advanced filter with MM/DD/YYYY, status, tags, priority, USD price, sort, paginate: [filter/filter.go:49-177](repo/backend/internal/filter/filter.go); frontend [AdvancedFilters.tsx](repo/frontend/src/components/AdvancedFilters.tsx).
- Address book + customer-by-address + orders-by-address: [addressbook.go](repo/backend/internal/api/addressbook.go), [customers.go:154-183](repo/backend/internal/api/customers.go), [orders.go:192-214](repo/backend/internal/api/orders.go).
- Dispatch pin + geofence + fee (route table + haversine fallback): [dispatch.go:14-71](repo/backend/internal/api/dispatch.go), [geo/polygon.go:48-89](repo/backend/internal/geo/polygon.go), [geo/distance.go:75-96](repo/backend/internal/geo/distance.go). Offline map backdrop ([OfflineMap.tsx:97-114](repo/frontend/src/components/OfflineMap.tsx), [settings.go](repo/backend/internal/api/settings.go)).
- Lab sampling→received→in_testing→reported: [lab/sample.go](repo/backend/internal/lab/sample.go), [api/lab.go:132-254](repo/backend/internal/api/lab.go).
- Abnormal flagging: [lab/reference.go:87-137](repo/backend/internal/lab/reference.go); uncategorized is a distinct flag, not "normal" (reference.go:19,129).
- Versioned reports + optimistic concurrency + reason-required + archive retrievable: [lab/report.go:102-127](repo/backend/internal/lab/report.go); schema enforces `UNIQUE(sample_id, version)` ([migrations/0001_init.sql:220](repo/backend/migrations/0001_init.sql)); FTS index across title+narrative (migrations/0001_init.sql:217-223).
- Order state machine + refund reason + OOS + 30-min picking-timeout + split suggestion: [order/workflow.go:38-232](repo/backend/internal/order/workflow.go), [api/orders.go:257-383](repo/backend/internal/api/orders.go).
- Encryption: AES-256-GCM with versioned envelope ([crypto/crypto.go:64-131](repo/backend/internal/crypto/crypto.go)). Per-env keys from `ENC_KEYS` ([runtime/runtime.go:201-236](repo/backend/internal/runtime/runtime.go)).
- Password policy + Argon2id: [auth/password.go:18-67](repo/backend/internal/auth/password.go). Lockout persisted so a restart cannot bypass: [auth/store_lockout.go:48-98](repo/backend/internal/auth/store_lockout.go), schema login_attempts table ([migrations/0001_init.sql:42-47](repo/backend/migrations/0001_init.sql)).
- Audit: immutable trigger in schema ([migrations/0001_init.sql:268-278](repo/backend/migrations/0001_init.sql)); log includes actor, workstation, client workstation-time ([audit/audit.go:37-68](repo/backend/internal/audit/audit.go)).
- Admin: users, roles, permission catalog, reference ranges, service regions, route table, map image, audit viewer ([api/admin.go](repo/backend/internal/api/admin.go), [api/permissions.go](repo/backend/internal/api/permissions.go), [api/settings.go](repo/backend/internal/api/settings.go), wired at [server.go:196-210](repo/backend/internal/api/server.go)).

#### 2.2 End-to-end deliverable (not a fragment) — **Pass**
- Real project structure, not scattered files: see section 3 mapping.
- No placeholder "mock" business logic; the Memory store is a genuine implementation used only when `DATABASE_URL` is unset and documented as such ([cmd/server/main.go:34-49](repo/backend/cmd/server/main.go)).
- README covers setup, run, test, stop, credentials.

### 3. Engineering and Architecture Quality

#### 3.1 Structure and module decomposition — **Pass**
Clear package layout: transport (`api`, `httpx`), domain (`order`, `lab`, `geo`, `filter`, `search`, `audit`, `crypto`), persistence (`store` with `Memory` + `Postgres` implementations behind one interface at [store.go:27-44](repo/backend/internal/store/store.go)), bootstrap (`runtime`, `cmd/*`). No obvious dead folders. No giant single-file implementations.

#### 3.2 Maintainability / extensibility — **Pass**
- Admin-configurable permission catalog allows role policy changes without deploy.
- Reference ranges hot-reload into in-memory set ([admin.go:203-234](repo/backend/internal/api/admin.go), [server.go:80-92](repo/backend/internal/api/server.go)).
- Route table hot-reloads too ([server.go:67-78](repo/backend/internal/api/server.go)).
- Clock / sessions / vaults are injected, enabling deterministic tests.
- Minor debt: one private helper `itoa` is rolled in [admin.go:276-289](repo/backend/internal/api/admin.go) to avoid a single strconv import — harmless but illustrative of isolated micro-choices.

### 4. Engineering Details and Professionalism

#### 4.1 Error handling, logging, validation, API design — **Pass**
- Unknown errors are translated to a generic 500 via [httpx/context.go#WriteError](repo/backend/internal/httpx/context.go) so internals (SQL, stack text, crypto errors) don't leak. A test asserts this ([security_test.go:411-431](repo/backend/internal/api/security_test.go)).
- Structured JSON-line logging with request-id, actor, role, workstation, latency, status ([httpx/logging.go:36-146](repo/backend/internal/httpx/logging.go)).
- Input validation: filter validator enforces date format, page/size caps, narrowing criterion, sort allowlist ([filter/filter.go:118-179](repo/backend/internal/filter/filter.go)); admin PUTs validate refrange ordering, route non-negative miles, polygon vertex shape; `map_image_url` is scheme-whitelisted.
- API: REST-style verbs, consistent error shape `{"message":"..."}`, `X-Request-ID` echoed on every response.

#### 4.2 Organized like a real product — **Pass**
- Dockerized, multi-stage, reproducible builds for both services.
- Standardized test runner aggregates suites and emits CI-friendly exit codes.
- Auditable authorization matrix tests, IDOR tests, 401 matrix tests.
- Frontend has components with accessible labels, role-based navigation, data-testid attributes used by component tests, per-workspace color cues.

### 5. Prompt Understanding and Requirement Fit

#### 5.1 Requirement fidelity — **Pass**
- Offline-only constraint respected: no external HTTP dependencies in backend (`grep -R "http.Get\|http.Post\|http.Client" backend/internal` finds only Echo's middleware). Map backdrop accepts `http(s)://`, relative `/`, or `data:image/` — an operator can therefore serve entirely from the local LAN.
- "≥10 chars" policy enforced in [auth/password.go:18,44](repo/backend/internal/auth/password.go).
- "5 failed attempts, 15 minutes" — [auth/lockout.go:11-13](repo/backend/internal/auth/lockout.go); persistent in [store_lockout.go:49](repo/backend/internal/auth/store_lockout.go) so restarts do not reset.
- "30-minute picking timeout" — [order/workflow.go:34](repo/backend/internal/order/workflow.go).
- "Optimistic concurrency with version numbers" and "correction requires reason" — [lab/report.go:102-127](repo/backend/internal/lab/report.go).
- "Superseded report remains readable but cannot be deleted" — schema `reports.status CHECK IN ('draft','issued','superseded')` + [report.go:129-135](repo/backend/internal/lab/report.go); `CanDelete` returns `ErrCannotDelete` for superseded.
- "Per-environment keys stored on the host" — `ENC_KEYS` is plumbed through env only; [runtime/runtime.go:201-236](repo/backend/internal/runtime/runtime.go) is the only reader.

### 6. Aesthetics (frontend)

#### 6.1 Visual/interaction design — **Partial Pass** (Cannot Confirm Statistically for visual quality)
- Defined color system with per-workspace accent stripe ([styles.css:14-76](repo/frontend/src/styles.css)), card+grid layout, semantic OK/error banners. Component tests cover hover/state for interactive cards (Modal, OrderTimeline, etc.).
- Actual rendered appearance not verified (no browser run). **Manual verification note:** open the running app and confirm spacing/alignment/color contrast on each page.

---

## 5. Issues / Suggestions (Severity-Rated)

Priority order: highest-impact first. Items from the prior report that are now closed are recorded in § 5.3.

### 5.1 Blocker / High

_No Blocker or High findings were confirmed in this review._

### 5.2 Medium

#### M1 — Dev-mode "ephemeral" key is a deterministic well-known constant
- **Severity:** Medium
- **Conclusion:** The dev-mode fallback misrepresents its security model. With `OOPS_DEV_MODE=1` (the default shipped in [.env.example:36](repo/.env.example)) and `ENC_KEYS` unset (also the default, [.env.example:30](repo/.env.example)), the server initializes the vault with `crypto.DeriveKey([]byte("dev-only-not-for-production-use"))` — a deterministic 32-byte key derived from a publicly visible constant ([runtime/runtime.go:213-214](repo/backend/internal/runtime/runtime.go), [crypto/crypto.go:136-140](repo/backend/internal/crypto/crypto.go)). The log warns that "Data encrypted now will NOT be readable after restart", but because the key is constant the data *is* readable across restarts — and by anyone holding this repository.
- **Evidence:** [runtime.go:213-214](repo/backend/internal/runtime/runtime.go), [crypto.go:136-140](repo/backend/internal/crypto/crypto.go), [.env.example:30,36](repo/.env.example).
- **Impact:** An evaluator who follows the documented Quickstart ships the portal with a publicly known AES key. Customer identifiers and street addresses stored under this key are decryptable by anyone with repo access. The Prompt explicitly requires "per-environment keys stored on the host", which the default configuration does not satisfy.
- **Minimum actionable fix:** Either (a) make the dev-mode key genuinely ephemeral (generate random bytes at startup and never persist) and document that restart loses data; or (b) ship `.env.example` with `OOPS_DEV_MODE=0` and let the server refuse to boot until the operator runs `keygen` and pastes a real line.

#### M2 — Sidebar / backend permission drift after A1–A3 fix
- **Severity:** Medium
- **Conclusion:** The A1–A3 fix removed `customers.read` from the `lab_tech` role grant ([migrations/0001_init.sql:304-328](repo/backend/migrations/0001_init.sql), [memory.go:97-111](repo/backend/internal/store/memory.go)), and tests assert `tech->customer` is 403 ([matrix_test.go:163-164](repo/backend/internal/api/matrix_test.go)). However, the SPA sidebar still shows the "Customers" link to `lab_tech` ([App.tsx:67-69](repo/frontend/src/App.tsx)). Clicking it renders a page that then 403s on every fetch — a confusing UX and a small vector for future authorization bugs if someone "fixes" it by broadening the backend again.
- **Evidence:** [App.tsx:67](repo/frontend/src/App.tsx), [migrations/0001_init.sql:304-311](repo/backend/migrations/0001_init.sql).
- **Impact:** Cosmetic + UX; no security leak. Confuses lab-tech users.
- **Minimum actionable fix:** Drop `"lab_tech"` from the Customers visibility array in App.tsx, and while there re-check that Orders/Analytics visibility matches the permission catalog (e.g., Orders includes `analyst` which has only `orders.read` — the link is fine because the Orders page is read-capable).

#### M3 — Login disabled-account branch skips the timing pad
- **Severity:** Medium (degrades A11's fix rather than undoing it)
- **Conclusion:** The username-enumeration timing fix now runs a dummy Argon2id compare on the `ErrNotFound` branch ([api/auth.go:56-64](repo/backend/internal/api/auth.go)). But the `u.Disabled` branch short-circuits *before* `ComparePassword` runs (`auth.go:68-70`), returning 403 without the CPU cost of a real hash check. A probe can therefore tell apart "disabled account" from "valid credentials rejected" in two ways: the 403 vs 401 status code *and* a shorter response time.
- **Evidence:** [api/auth.go:56-75](repo/backend/internal/api/auth.go).
- **Impact:** Any attacker who enumerates valid usernames can additionally learn which are disabled. Modest privacy leak; not a credential leak.
- **Minimum actionable fix:** In the disabled-account branch, call `auth.ComparePassword(u.PasswordHash, body.Password)` (ignore the result) before returning 403; and consider returning 401 instead of 403 so the status-code channel also closes.

#### M4 — Audit-log write failures are silently ignored
- **Severity:** Medium
- **Conclusion:** Every `s.Audit.Log(...)` call discards the error with `_ =` (for example [orders.go:61,245,340,349,378,380](repo/backend/internal/api/orders.go), [customers.go:59,145](repo/backend/internal/api/customers.go), [lab.go:102,249,252,286,349](repo/backend/internal/api/lab.go), [admin.go:126,186,232,272](repo/backend/internal/api/admin.go), [addressbook.go:72,100](repo/backend/internal/api/addressbook.go)). If the underlying `AppendAudit` transiently fails (connection blip, disk full, trigger misconfig), the business mutation still succeeds and the audit row is lost. The Prompt's "all state changes write immutable audit entries including who acted, what changed, and the workstation time" is therefore a best-effort invariant rather than a hard one.
- **Evidence:** paths above; contrast with `s.Store.*` calls whose errors *are* surfaced.
- **Impact:** Audit completeness not guaranteed under DB pressure. For a business that names auditability as a core requirement this is a real gap.
- **Minimum actionable fix:** Either return the audit error (causing a 500 that the client retries) or record a metric/log line so operators notice the drop. A middle ground: log at level=error with entity + entity_id so at least the tokenized event is preserved in stdout.

#### M5 — `ExportOrdersCSV` offset is computed from unbounded `body.Size`
- **Severity:** Medium
- **Conclusion:** The handler caps `limit` at `MaxExportSize` ([export.go:35-38](repo/backend/internal/api/export.go)) but computes the offset as `(body.Page - 1) * body.Size` using the *uncapped* `body.Size` ([export.go:48](repo/backend/internal/api/export.go)). A caller who passes `Page=1, Size=500` is fine, but a caller who passes `Page=10, Size=400` (passes Validate because `Size<=500`) effectively paginates at offset 3,600 with a 500-row window. This is not a broken security guard — `filter.Validate` still requires a narrowing criterion for `Size>100` — but the export handler's own documented "hard cap" semantics are slightly inconsistent with how offset advances.
- **Evidence:** [export.go:35-48](repo/backend/internal/api/export.go).
- **Impact:** Low direct risk; mostly an inconsistency that will confuse operators reading offsets back from an audited payload.
- **Minimum actionable fix:** Compute `offset = (Page-1) * limit` (the capped value), not `Size`.

### 5.3 Low

#### L1 — Password-policy is length-only
- **Severity:** Low
- **Conclusion:** `auth.ValidatePolicy` rejects blank/≤9-char passwords but has no complexity / breached-password / character-class rule ([auth/password.go:40-48](repo/backend/internal/auth/password.go)). The Prompt only specifies "at least 10 characters", so this is within spec, but a future review should consider `"aaaaaaaaaa"` is accepted.
- **Minimum actionable fix:** Optional; add a dictionary check or require a mix of classes. Not required by the Prompt.

#### L2 — `SEED_DEMO_USERS=1` seeds well-known passwords via a single env flip
- **Severity:** Low
- **Conclusion:** A12 was addressed — the shipped default is now `SEED_DEMO_USERS=0` ([.env.example:46](repo/.env.example), [docker-compose.yml:40](repo/docker-compose.yml)). However, flipping to `1` installs five accounts whose passwords are hardcoded in source ([runtime.go:96-101](repo/backend/internal/runtime/runtime.go)) and repeated in README.md:136-140. If an operator leaves the flag on for evaluation then promotes the instance, the published passwords remain. The README warns about this, but there is no first-login-must-rotate enforcement.
- **Minimum actionable fix:** Set a `password_rotate_required` flag on demo users and refuse further API calls from those sessions until they rotate.

#### L3 — `OrdersByAddress` / `CustomersByAddress` return unbounded result sets
- **Severity:** Low
- **Conclusion:** Both handlers stream the full matching slice back without a `limit` ([api/orders.go:199-213](repo/backend/internal/api/orders.go), [api/customers.go:161-182](repo/backend/internal/api/customers.go)). Store implementations also have no cap on this path (`Memory.OrdersByAddress` / `Postgres.OrdersByAddress` do no LIMIT). Not exploitable as a broad-export channel because city/zip is required, but a single shared zip can still dump thousands of rows.
- **Minimum actionable fix:** Add a `limit` query parameter (default 100, max 500) and threaded to the store.

#### L4 — `CORS` with wildcard still possible
- **Severity:** Low
- **Conclusion:** `parseAllowedOrigins` intentionally lets an operator set `ALLOWED_ORIGINS=*` to re-enable the wildcard ([server.go:233-257](repo/backend/internal/api/server.go)). The comment says this is "for operators who explicitly want it", but there is no log line when the wildcard resolves, and no guard rails when `AllowCredentials` is eventually set. Low severity because credentials today are bearer tokens in `Authorization:`, not cookies.
- **Minimum actionable fix:** Emit a single `log.Printf` on startup whenever `ALLOWED_ORIGINS == "*"` so an operator sees it in the journal.

#### L5 — `ListExceptions` runs two unbounded detectors on each call
- **Severity:** Low
- **Conclusion:** `ListExceptions` fetches `limit=500` of orders in "picking" plus `limit=500` of *all* recent orders on every read ([api/orders.go:258-306](repo/backend/internal/api/orders.go)). Deterministic but does O(orders) work per queue read. Fine at small scale; worth tagging for later.
- **Minimum actionable fix:** Record detector run times; convert to a scheduled sweep + read-from-queue.

#### L6 — Audit log entity name is a free-form string
- **Severity:** Low
- **Conclusion:** Callers pass the `entity` string by hand (`"order"`, `"customer"`, `"order_exception"`, `"saved_filter"`, `"service_regions"`, `"reference_ranges"`, `"system_settings"` …). A typo in one handler would silently split an entity's history. There is no const or type for these.
- **Minimum actionable fix:** Introduce `audit.Entity` typed string with named constants; the compile step catches typos.

### 5.4 Status of prior findings (A1–A13)

Re-verifying each against the current tree. (The existing `audit_report-1-fix_check.md` is stale: it was written before commit `b7af2a2` and marked most items "Not addressed"; the code now tells a different story.)

| ID | Prior severity | Status now | Evidence in current tree |
|---|---|---|---|
| A1 — Analyst can mutate | High | **Fixed** | All mutations gated by `RequirePermission` ([server.go:137-170](repo/backend/internal/api/server.go)); analyst grants omit `*.write` ([migrations:316-318](repo/backend/migrations/0001_init.sql), [memory.go:101](repo/backend/internal/store/memory.go)); deny-matrix test covers it ([security_test.go:102-173](repo/backend/internal/api/security_test.go)). |
| A2 — Lab group uniform | High | **Fixed** | Each lab handler has its own `needs("…")` ([server.go:162-174](repo/backend/internal/api/server.go)); non-lab roles denied ([security_test.go:135-156](repo/backend/internal/api/security_test.go)). |
| A3 — Customer writes open | High | **Fixed** | Customer POST/PATCH require `customers.write`; only `front_desk` and `admin` hold it ([migrations:306-320](repo/backend/migrations/0001_init.sql)). Deny test at [security_test.go:136-146](repo/backend/internal/api/security_test.go). |
| A4 — Default `ENC_KEYS` placeholder | High | **Fixed** | `.env.example:30` is empty; `runtime.BuildVault` returns `ErrPlaceholderKey` when the historical constant is supplied outside dev mode ([runtime.go:184-221](repo/backend/internal/runtime/runtime.go)); `start.sh:25-32` refuses to launch with the placeholder. |
| A5 — CORS wildcard default | Medium | **Fixed** | `parseAllowedOrigins` defaults to localhost SPA only ([server.go:239-257](repo/backend/internal/api/server.go)); 3 tests in [new_features_test.go:186-208](repo/backend/internal/api/new_features_test.go). |
| A6 — Offline map backdrop | Medium | **Fixed** | Admin-set map URL ([settings.go](repo/backend/internal/api/settings.go)); SVG image overlay ([OfflineMap.tsx:97-114](repo/frontend/src/components/OfflineMap.tsx)); scheme whitelist + tests ([new_features_test.go:63-120](repo/backend/internal/api/new_features_test.go)). |
| A7 — Saved-filter too-broad | Medium | **Fixed** | `HasNarrowingCriterion` required *regardless* of size for saved filters ([filters_search.go:35-37](repo/backend/internal/api/filters_search.go)); pagination also capped via `MaxQueryPage=200` ([filter.go:76,171-173](repo/backend/internal/filter/filter.go)); tests [api_test.go:342-401](repo/backend/internal/api/api_test.go). |
| A8 — `QueryOrders` swallows date error | Medium | **Fixed** | Both `QueryOrders` and `ExportOrdersCSV` now check `ParseDate` ([orders.go:124-140](repo/backend/internal/api/orders.go), [export.go:50-65](repo/backend/internal/api/export.go)); test [api_test.go:391-401](repo/backend/internal/api/api_test.go). |
| A9 — Analytics unbounded window | Medium | **Fixed** | `parseWindow` defaults to 90-day trailing ([analytics.go:18-39](repo/backend/internal/api/analytics.go)). |
| A10 — `auth` local shadows package | Low | **Fixed** | Renamed to `authGroup` at [server.go:119](repo/backend/internal/api/server.go). |
| A11 — Login timing channel | Low | **Fixed** (with M3 caveat) | Dummy Argon2id compare on the not-found branch ([auth.go:14-35,56-64](repo/backend/internal/api/auth.go)). Disabled-account branch still skips the pad — see M3 above. |
| A12 — `SEED_DEMO_USERS=1` default | Low | **Fixed** (with L2 residual) | `.env.example:46` is `0`; docker-compose uses `${SEED_DEMO_USERS:-0}` ([docker-compose.yml:40](repo/docker-compose.yml)). |
| A13 — Uncategorized flagged normal | Low | **Fixed** | `FlagUncategorized` exists and is excluded from `IsAbnormal` ([lab/reference.go:19,25-27,128-131](repo/backend/internal/lab/reference.go)). |

---

## 6. Security Review Summary

| Concern | Conclusion | Evidence / reasoning |
|---|---|---|
| **Authentication entry point** | Pass | Single `/api/auth/login` with body validation ([auth.go:39-49](repo/backend/internal/api/auth.go)); Argon2id hash ([password.go:52-67](repo/backend/internal/auth/password.go)); persistent 5-fail/15-min lockout ([store_lockout.go](repo/backend/internal/auth/store_lockout.go)); timing pad for not-found path ([auth.go:56-64](repo/backend/internal/api/auth.go)). Disabled-account residual covered in M3. |
| **Route-level authorization** | Pass | Every non-public route is in `authGroup` behind `RequireAuth`; every mutation carries an explicit `needs("permID")`; tests at [security_test.go:23-58](repo/backend/internal/api/security_test.go) (401 matrix), [matrix_test.go:19-109](repo/backend/internal/api/matrix_test.go) (403 matrix). |
| **Object-level authorization** | Partial Pass | Owner-scoped resources (address book, saved filters) do enforce owner match at the store layer and surface 404, not 403 ([memory.go:269-278,700-…](repo/backend/internal/store/memory.go); [security_test.go:200-257](repo/backend/internal/api/security_test.go)). Shared-entity 403 vs 404 covered in [matrix_test.go:116-186](repo/backend/internal/api/matrix_test.go). Statically: no explicit object-level check ties a customer / order / sample / report to a particular user, but the Prompt does not require that — those records are shared operational data. |
| **Function-level authorization** | Pass | Separate permissions for archive (`reports.archive`), export (`orders.export`), settings (`admin.settings`), users (`admin.users`), audit read (`admin.audit`), reference ranges (`admin.reference`), dispatch config (`dispatch.configure`). Admin can hand out the "view audit" permission without "manage users" — the Prompt's "configure dictionaries, permissions" requirement is cleanly fulfilled. |
| **Tenant / user isolation** | Pass (single-tenant) | Product is explicitly single-organization / offline. Address book and saved filters are per-user, which is enforced ([memory.go:269-278,700-…](repo/backend/internal/store/memory.go)). |
| **Admin / internal / debug protection** | Pass | All `/api/admin/*` routes require an admin-mapped permission; no debug or profiling endpoint is registered; no "kill switch" or pprof route exposed; `/api/health` is public and returns only `{status:"ok"}`. |
| **Secrets / crypto defaults** | Partial Pass | AES-256-GCM envelope with versioned keys ([crypto.go](repo/backend/internal/crypto/crypto.go)); keygen tool provided; placeholder key rejected. Default `.env.example` still enables dev-mode + empty ENC_KEYS, which silently uses a deterministic key (see M1). |
| **SQL injection** | Pass | All dynamic SQL uses `$N` parameters; sort column is whitelisted before string interpolation ([postgres.go:432-490](repo/backend/internal/store/postgres.go)). |
| **Response leakage** | Pass | `httpx.WriteError` maps unknown errors to a generic 500 message; test asserts SQL/stack text is not leaked ([security_test.go:384-431](repo/backend/internal/api/security_test.go)). |
| **Audit tamper resistance** | Pass | DB trigger blocks UPDATE/DELETE on `audit_log` ([migrations:268-278](repo/backend/migrations/0001_init.sql)); integration test asserts it ([postgres_integration_test.go:62-74](repo/backend/internal/store/postgres_integration_test.go)). Audit completeness under failure is M4. |

---

## 7. Tests and Logging Review

### Unit tests — Pass
- ~245 Go test functions, ~122 frontend `it/test/describe` lines. Rough distribution:
  - `internal/api/*_test.go` covers handlers, auth gates, 401/403 matrices, IDOR, role deny matrix, CSV export, map config, saved-filter validator, timing-pad, audit recording, generic-500 sanitization.
  - `internal/auth` covers policy, Argon2id hash round-trip, both in-memory and store-backed lockout, sessions.
  - `internal/crypto` covers encrypt/decrypt, key rotation, malformed ciphertext, wrong version.
  - `internal/lab`, `internal/order`, `internal/geo`, `internal/filter`, `internal/search`, `internal/audit`, `internal/httpx`, `internal/runtime`, `internal/store` all have dedicated test files.
  - Frontend component tests: `OfflineMap.test.tsx`, `AdvancedFilters.test.tsx`, `GlobalSearch.test.tsx`, `Modal.test.tsx`, `OrderTimeline.test.tsx`, `ReportWorkspace.test.tsx`, `BarChart.test.tsx`, plus one test per page.

### Integration tests — Pass (static)
- Real Postgres behavior parity exercised in [postgres_integration_test.go](repo/backend/internal/store/postgres_integration_test.go) and [postgres_full_integration_test.go](repo/backend/internal/store/postgres_full_integration_test.go). Gated on `INTEGRATION_DB`; `docker-compose.test.yml` supplies the DSN for `backend-test`.
- Cannot Confirm Statistically that they pass — runtime not exercised.

### Logging and observability — Pass
- JSON-line structured logging with request-id, latency, status, actor, role, workstation ([httpx/logging.go:36-146](repo/backend/internal/httpx/logging.go)).
- Error branch log-then-sanitize in [httpx/context.go:85-89](repo/backend/internal/httpx/context.go).
- Level transition rules (`info` < `warn` for 4xx < `error` for 5xx) are explicit.

### Sensitive-data leakage in logs / responses — Pass
- No password, token, or ENC_KEYS is written anywhere in `httpx/logging.go` — the LogEntry struct does not include the Body.
- Audit logs redact customer identifiers and delivery street before marshalling ([orders.go:78-86](repo/backend/internal/api/orders.go), [customers.go:207-222](repo/backend/internal/api/customers.go), [addressbook.go:72-77](repo/backend/internal/api/addressbook.go)).
- Decryption happens only in `*view` helpers that produce the response; encrypted envelopes never appear in JSON output.

---

## 8. Test Coverage Assessment (Static Audit)

### 8.1 Test Overview
- Unit tests: Go `testing` with `testify`-free assertions. Frontend uses Vitest + Testing Library.
- Integration tests: optional Postgres parity suite under [store/postgres_*_test.go](repo/backend/internal/store/).
- Entry points: `run_tests.sh` → `docker compose -f docker-compose.test.yml run --rm backend-test` (`go test -cover ./...`) + `frontend-test` (`npm test`).
- Docs: [README.md:99-123](repo/README.md) documents the commands.

### 8.2 Coverage Mapping Table

| # | Requirement / Risk | Mapped test(s) | Key assertion | Coverage | Gap | Minimum test addition |
|---|---|---|---|---|---|---|
| 1 | Login policy + lockout | [api_test.go:129-167](repo/backend/internal/api/api_test.go), [auth/lockout_test.go](repo/backend/internal/auth/lockout_test.go), [auth/store_lockout_test.go](repo/backend/internal/auth/store_lockout_test.go) | 5th fail returns 423, persists via store | Sufficient | — | — |
| 2 | Username timing pad | [api_test.go:156-168](repo/backend/internal/api/api_test.go) | Ghost-user path still rate-limits | Basically covered | Disabled-account path not asserted (M3) | Add a test that seeds a disabled user and asserts the password-compare branch is entered before 403 |
| 3 | Customers write/read split | [security_test.go:102-173](repo/backend/internal/api/security_test.go), [matrix_test.go:60-68](repo/backend/internal/api/matrix_test.go) | Analyst/tech/dispatch 403 on `POST /customers` | Sufficient | — | — |
| 4 | Encryption at rest | [api_test.go:172-200](repo/backend/internal/api/api_test.go), [crypto/crypto_test.go](repo/backend/internal/crypto/crypto_test.go) | Stored identifier is not plaintext | Sufficient | — | — |
| 5 | Order state machine | [order/workflow_test.go](repo/backend/internal/order/workflow_test.go), [api_test.go:213-231](repo/backend/internal/api/api_test.go) | Invalid transition 400, valid 200, refund without reason fails | Sufficient | — | — |
| 6 | OOS + split suggestion | [order/workflow_test.go](repo/backend/internal/order/workflow_test.go), [api/extra_coverage_test.go](repo/backend/internal/api/extra_coverage_test.go) | Backordered lines raise exception at create time | Sufficient | — | — |
| 7 | Picking timeout | [order/workflow_test.go](repo/backend/internal/order/workflow_test.go) | Detector fires after 30 minutes | Sufficient | — | — |
| 8 | Sample → report state gate | [api/coverage_test.go:17-60](repo/backend/internal/api/coverage_test.go) | Report rejected until `in_testing`; second v1 409 | Sufficient | — | — |
| 9 | Report optimistic concurrency + reason | [api_test.go:235-297](repo/backend/internal/api/api_test.go), [lab/lab_test.go](repo/backend/internal/lab/lab_test.go) | Wrong expected_version 409; missing reason fails | Sufficient | — | — |
| 10 | Abnormal flagging + uncategorized | [lab/lab_test.go](repo/backend/internal/lab/lab_test.go), [lab/extra_coverage_test.go](repo/backend/internal/lab/extra_coverage_test.go) | Uncategorized is not abnormal | Sufficient | — | — |
| 11 | Archive readable-not-deletable | [lab/lab_test.go](repo/backend/internal/lab/lab_test.go), archive tests in [api/full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) | Archive 200; superseded rows remain via ListArchived | Basically covered | — | — |
| 12 | Geofence pin validation | [api_test.go:301-326](repo/backend/internal/api/api_test.go), [geo/polygon_test.go](repo/backend/internal/geo/polygon_test.go) | Inside 200 valid; outside 200 invalid | Sufficient | — | — |
| 13 | Fee quote (route table + fallback) | [geo/distance_test.go](repo/backend/internal/geo/distance_test.go), [api/full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) | Method string flips route/haversine | Sufficient | — | — |
| 14 | Advanced filter validator | [filter/filter_test.go](repo/backend/internal/filter/filter_test.go), [api_test.go:342-401](repo/backend/internal/api/api_test.go) | Broad filter rejected; date parse error 400; page > MaxQueryPage 400 | Sufficient | — | — |
| 15 | Saved-filter narrowing mandatory | [api_test.go:353-370](repo/backend/internal/api/api_test.go) | Small-but-open filter still rejected | Sufficient | — | — |
| 16 | Bounded CSV export | [new_features_test.go:122-181](repo/backend/internal/api/new_features_test.go) | Shape + broad filter 400 + permission 403 | Sufficient | Offset computation w.r.t. L5/M5 not asserted | Assert returned row count is ≤ `filter.MaxExportSize` irrespective of `size` |
| 17 | Audit completeness per mutation | [security_test.go:262-380](repo/backend/internal/api/security_test.go), [audit/audit_test.go](repo/backend/internal/audit/audit_test.go) | Every mutation writes ≥1 audit row | Sufficient | No test for `AppendAudit` error path (M4) | Add a faultyStore test that breaks `AppendAudit` and asserts client still succeeds and an error log is emitted |
| 18 | Append-only enforcement | [postgres_integration_test.go:62-74](repo/backend/internal/store/postgres_integration_test.go) | UPDATE/DELETE on audit_log raises | Sufficient (requires INTEGRATION_DB) | — | — |
| 19 | 401 / 403 matrix | [security_test.go:23-93](repo/backend/internal/api/security_test.go), [matrix_test.go:19-109](repo/backend/internal/api/matrix_test.go) | Unauth 401; non-admin admin-endpoint 403 | Sufficient | — | — |
| 20 | Owner-scoped isolation | [security_test.go:200-257](repo/backend/internal/api/security_test.go) | Bob cannot list or delete Alice's filter / address | Sufficient | — | — |
| 21 | Dev-mode crypto default (M1) | [runtime/runtime_test.go](repo/backend/internal/runtime/runtime_test.go) | ErrMissingKeys when unset + no dev mode; dev-mode warning path | Basically covered | Does not assert that the dev key is cryptographically unsafe or warn about determinism | Add a test that asserts two successive `BuildVault(devMode)` calls return vaults that can decrypt each other's output (proves non-ephemeral) — failing test forces M1 to be fixed |
| 22 | Global search typo tolerance | [search/fuzzy_test.go](repo/backend/internal/search/fuzzy_test.go), [filters_search.go#GlobalSearch](repo/backend/internal/api/filters_search.go) | Ranker threshold + mixed-kind suggestions | Basically covered | — | — |
| 23 | Recent searches per-user on device | [useRecentSearches.test.tsx](repo/frontend/src/hooks/useRecentSearches.test.tsx) | LIMIT=20, scoped per userID | Sufficient | — | — |
| 24 | Offline map rendering with admin URL | [OfflineMap.test.tsx](repo/frontend/src/components/OfflineMap.test.tsx), [new_features_test.go:63-120](repo/backend/internal/api/new_features_test.go) | Image rendered when URL set; scheme allowlist | Sufficient | — | — |

### 8.3 Security Coverage Audit
- **Authentication:** Sufficient — success + 4 wrong + lockout + ghost-username + disabled all covered (M3 is an unpatched residual but not an uncovered line; it is a behavior gap).
- **Route authorization:** Sufficient — unauth 401 matrix + admin-only 403 matrix + per-role endpoint matrix + write-deny matrix.
- **Object-level authorization:** Basically covered for owner-scoped resources (address book, saved filters). Shared entities (customers, orders, samples, reports) have no per-object ACL by design; the Prompt does not require one.
- **Tenant / data isolation:** Not applicable (single-tenant).
- **Admin / internal protection:** Sufficient — every `/api/admin/*` endpoint has at least one positive (admin allowed) and negative (non-admin forbidden) test.

### 8.4 Final Coverage Judgment — **Partial Pass**
**Covered major risks:** auth, lockout, route + mutation authorization, object isolation for owner-scoped resources, encryption at rest, optimistic concurrency, state-machine gates, filter over-broad guard, picking timeout, OOS, archive readability, geofence, fee calc, audit write (happy path), append-only enforcement, generic-500 sanitization.

**Uncovered risks that could still allow severe defects to land unnoticed:**
1. M1 / T21 — the "dev-mode ephemeral key" behavior is not asserted to actually be ephemeral, so a change that silently enables the deterministic constant in production would not fail any test.
2. M4 / T17 — audit write failures are not simulated, so a regression that drops writes silently has no failing test.
3. M3 / T2 — the disabled-account timing channel is not asserted.
4. M5 / T16 — export pagination offset semantics aren't locked down.

These are bounded, well-described gaps; none mask a Blocker risk on a static read.

---

## 9. Final Notes

- The delivery is materially consistent with the Prompt: offline-first, Go+Echo+Postgres, local auth, per-env AES keys, versioned reports with optimistic concurrency + reason-required, append-only audit, offline geofenced dispatch with route-table fee fallback, typo-tolerant global search with last-20 per-user on the device, advanced filter + saved filter anti-broad-export, admin-configurable dictionaries and permissions.
- Every prior High finding (A1–A4) was addressed with real code changes and now has regression tests. Every prior Medium finding was addressed with code changes. Every prior Low finding was addressed.
- Residual issues (M1–M5, L1–L6) are polish-grade. M1 is the most worthwhile follow-up because it undermines the "per-environment keys" Prompt constraint under the shipped `.env.example` defaults.
- Static audit does not run tests; overall pass/fail of CI was not confirmed. Manual verification recommended for: `./start.sh` cold boot on a clean host, `./run_tests.sh` green exit code, browser rendering of the SPA pages under the five roles.
