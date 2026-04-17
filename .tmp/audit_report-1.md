# Static Delivery Acceptance & Project Architecture Audit

Audit date: 2026-04-17
Scope root: `/Users/mac/Eagle-Point Season 2/Task-24/` (repo under `repo/`)
Static-only. No execution, no Docker, no live tests, no code modifications.

---

## 1. Verdict

Overall conclusion: **Partial Pass**

The delivery is a coherent, product-shaped implementation that materially matches the Prompt's business scenario (offline lab + fulfillment portal, Go/Echo + React + PostgreSQL, audit log, per-env encryption, MM/DD/YYYY filters, geofence pin, versioned reports, etc.). Hard-gate documentation and basic end-to-end structure pass. However, there are **High-severity role-authorization leaks** where non-write roles (notably Analyst) are allowed by route groups to perform mutating operations, and several Medium/Low issues (map-image deviation, default dev encryption key, missing full-coverage of deny tests for role leakage, etc.). These prevent a clean Pass without remediation.

---

## 2. Scope and Static Verification Boundary

- What was reviewed:
  - Top-level docs/config/scripts: `repo/README.md:1`, `repo/.env.example:1`, `repo/docker-compose.yml:1`, `repo/docker-compose.test.yml:1`, `repo/start.sh:1`, `repo/run_tests.sh:1`, `metadata.json:1`, `docs/design.md` (listed), `docs/apispec.md` (listed).
  - Backend entrypoint & bootstrap: `repo/backend/cmd/server/main.go:1`, `repo/backend/cmd/seed/main.go:1`, `repo/backend/cmd/keygen/main.go` (listed), `repo/backend/internal/runtime/runtime.go:1`, `repo/backend/docker-entrypoint.sh:1`, `repo/backend/Dockerfile:1`.
  - Routing, middleware, auth, sessions: `repo/backend/internal/api/server.go:1`, `repo/backend/internal/httpx/middleware.go:1`, `repo/backend/internal/httpx/context.go:1`, `repo/backend/internal/httpx/logging.go:1`, `repo/backend/internal/auth/password.go:1`, `repo/backend/internal/auth/lockout.go:1`, `repo/backend/internal/auth/store_lockout.go:1`, `repo/backend/internal/auth/session.go:1`.
  - Domain handlers: `repo/backend/internal/api/auth.go:1`, `customers.go:1`, `orders.go:1`, `lab.go:1`, `addressbook.go:1`, `dispatch.go:1`, `filters_search.go:1`, `admin.go:1`, `permissions.go:1`, `analytics.go:1`.
  - Domain logic: `repo/backend/internal/order/workflow.go:1`, `repo/backend/internal/lab/report.go:1`, `sample.go:1`, `reference.go:1`, `repo/backend/internal/geo/polygon.go:1`, `distance.go:1`, `repo/backend/internal/filter/filter.go:1`, `repo/backend/internal/search/fuzzy.go:1`, `repo/backend/internal/crypto/crypto.go:1`, `repo/backend/internal/audit/audit.go:1`.
  - Persistence: `repo/backend/internal/store/store.go:1`, `memory.go:1` (partial), `postgres.go:1` (partial), migration `repo/backend/migrations/0001_init.sql:1`.
  - Frontend: `repo/frontend/Dockerfile:1`, `nginx.conf:1`, `package.json:1`, `src/App.tsx:1`, `src/api/client.ts:1`, `src/hooks/useRecentSearches.ts:1`, `src/components/GlobalSearch.tsx:1`, `OfflineMap.tsx:1`, `AdvancedFilters.tsx:1`, `ReportWorkspace.tsx:1`, `src/pages/Login.tsx:1`, `Admin.tsx` (partial), `Orders.tsx` (partial).
  - Tests listed and several read: `repo/backend/internal/api/security_test.go:1`, `api_test.go:1`, `coverage_test.go:1`, and multiple per-package `_test.go` files.
- What was not reviewed in depth: the complete body of `internal/store/postgres.go` (very long), every frontend page in full, every `*_test.go` body (sampled).
- What was intentionally not executed: project launch, Docker build, `run_tests.sh`, Postgres bootstrap, browser interaction.
- Claims requiring manual verification: runtime readiness of `start.sh` + migrations + seed, actual 401/403 behavior in a live server, actual CSP/browser behavior, actual tsvector search behavior, real abnormal-value highlighting end-to-end.

---

## 3. Repository / Requirement Mapping Summary

Prompt core goal: Offline operations portal for a lab + fulfillment business, with front desk / lab / dispatch / analyst / admin roles; global typo-tolerant search + last-20 recent per user + advanced filters (MM/DD/YYYY, status, tags, priority, USD, sort/pagination); address book; offline-map pin validation vs. geofence; lab lifecycle + reference-range abnormal highlighting + archived reports full-text search; order lifecycle + exception queues (30-min picking timeout, OOS split); Go/Echo + React + PostgreSQL; immutable audit; local-only auth with Argon2+lockout; per-env AES at-rest encryption of sensitive fields; optimistic concurrency for report edits; offline route table + haversine fallback.

Implementation mapped to these requirements:
- Stack: Go/Echo + React/Vite + PostgreSQL 14 as prompted — `repo/README.md:16`, `backend/go.mod:1`, `frontend/package.json:1`, `migrations/0001_init.sql:6`.
- Auth, lockout, Argon2id: `backend/internal/auth/password.go:30`, `lockout.go:9`, `store_lockout.go:44`.
- AES-256-GCM vault with versioned keys: `backend/internal/crypto/crypto.go:29`, wired via `cmd/server/main.go:28` and `runtime.BuildVault`.
- Encrypted fields: customers identifier/street, orders delivery_street, address_book street — `migrations/0001_init.sql:51`, `105`, `78`.
- Audit log append-only with triggers: `migrations/0001_init.sql:225`; application logger `backend/internal/audit/audit.go:37`.
- State machines + exceptions: `backend/internal/order/workflow.go:38`, detectors `DetectPickingTimeout`/`DetectOutOfStock` `workflow.go:148`, `185`; exception wiring `internal/api/orders.go:251`.
- Reports with optimistic concurrency + reason + archive: `backend/internal/lab/report.go:65`, `102`; handlers `internal/api/lab.go:187`, `255`.
- Offline geofence + route table + haversine fallback + fee computation: `backend/internal/geo/polygon.go:48`, `distance.go:75`.
- Advanced filter + saved filters + too-broad check: `backend/internal/filter/filter.go:88`, `internal/api/filters_search.go:16`.
- Global search with fuzzy ranking + last-20 recent per user: `backend/internal/search/fuzzy.go:96`, `frontend/src/hooks/useRecentSearches.ts:7`, `src/components/GlobalSearch.tsx:15`.
- MM/DD/YYYY parsing: `backend/internal/filter/filter.go:73`, `frontend/src/components/AdvancedFilters.tsx:14`.
- Role-based routing: `backend/internal/api/server.go:117` (customers), `130` (orders), `142` (lab), `156` (dispatch), `170` (admin), with dynamic permission gate applied to analytics only `192`.

Bottom line: the delivery is on-topic for the Prompt, not a generic CRUD or unrelated product.

---

## 4. Section-by-section Review

### 1. Hard Gates

#### 1.1 Documentation and static verifiability
- Conclusion: **Pass**
- Rationale: Run/test/config instructions are present and consistent with the referenced files (Dockerfiles, compose files, entrypoint, seed, server cmd, env). The documented structure matches actual directories.
- Evidence: `repo/README.md:49-111`, `repo/README.md:113-123`, `repo/start.sh:10`, `repo/run_tests.sh:9`, `repo/docker-compose.yml:13`, `repo/docker-compose.test.yml:6`, `repo/backend/Dockerfile:10`, `repo/frontend/Dockerfile:10`, `repo/backend/docker-entrypoint.sh:11`, `repo/.env.example:8`.
- Manual verification: Actual container startup / migration application / health probe cannot be verified statically (intentionally not executed).

#### 1.2 Material deviation from Prompt
- Conclusion: **Partial Pass**
- Rationale: The business goal is faithfully implemented (lab + fulfillment offline portal). One Prompt phrase specifies an "offline map **image** of the service territory"; the implementation renders polygons on a dark SVG canvas rather than loading a bundled raster image — functionally equivalent for geofence feedback but not strictly a map "image". No other major unrelated scope found.
- Evidence: `frontend/src/components/OfflineMap.tsx:79-97` (SVG viewbox + polygons; no bundled map image asset), `frontend/src/styles.css:171-188` (pin + `.map` background `#1f2937`).

### 2. Delivery Completeness

#### 2.1 Coverage of core prompt requirements
- Conclusion: **Partial Pass**
- Rationale: Almost all core requirements have a concrete implementation. However, the role-to-capability mapping in the Prompt (Front Desk registers customers, Lab Tech manages samples/results, Dispatch plans deliveries, Analyst builds operational reports, Admin configures) is weakly enforced: analysts and other non-write roles can reach mutating endpoints (see Issues A1–A3). Additionally, inventory-based OOS handling is present but the "prompt staff to split" flow is implemented via a separate `/plan` endpoint rather than an automatic prompt in the UI on exception trigger.
- Evidence: `repo/backend/internal/api/server.go:117,130,142`; orders detectors `internal/api/orders.go:65,251,305`; plan endpoint `orders.go:350`.

#### 2.2 End-to-end deliverable vs partial demo
- Conclusion: **Pass**
- Rationale: Full stack present — backend cmd + internal packages + migrations + seeds; frontend Vite app with routes, pages, components, hooks; Docker stacks for dev and test; README with startup and tests. Not a code fragment or single-file example.
- Evidence: `repo/README.md:25-47`, `repo/backend/internal/*`, `repo/frontend/src/**`, `repo/docker-compose.yml`, `repo/docker-compose.test.yml`.

### 3. Engineering and Architecture Quality

#### 3.1 Structure and module decomposition
- Conclusion: **Pass**
- Rationale: Clear separation: cmd/ (entrypoints) · internal/api (handlers) · internal/{auth,crypto,audit,geo,lab,order,search,filter,httpx,store,models,runtime}. Frontend split into api/components/hooks/lib/pages. Store interface is cleanly defined and has memory + Postgres implementations.
- Evidence: `repo/backend/internal/store/store.go:25-42`, `repo/backend/internal/api/server.go:38`, `repo/frontend/src/App.tsx:17`.

#### 3.2 Maintainability and extensibility
- Conclusion: **Partial Pass**
- Rationale: Codebase is modular and tested; uses interfaces (Store, AttemptStore, PermissionResolver). However, authorization is inconsistent: a flexible permission middleware (`RequirePermission`) exists but is only applied to analytics; all other endpoints rely on coarse `RequireRoles` groups. The admin UI exposes a permission matrix but no route honors those grants outside analytics.
- Evidence: `backend/internal/httpx/middleware.go:66-85`, `backend/internal/api/server.go:192-198`, `server.go:117-185`.

### 4. Engineering Details and Professionalism

#### 4.1 Error handling, logging, validation, API design
- Conclusion: **Pass**
- Rationale: `httpx.WriteError` centralizes mapping (404/409/401/423/500 generic), hiding internal error details. Structured JSON logging middleware emits a fixed schema with request ID correlation. Input validation is present on key mutating endpoints (title required for report; note required for archive; test_codes required for sample; name required for customer; role/permissions validated). Filter validator enforces entity/sort/status/date/price/page constraints and "too broad" guard.
- Evidence: `backend/internal/httpx/context.go:65-89`, `backend/internal/httpx/logging.go:87-146`, `backend/internal/api/lab.go:131,143,149,262`, `backend/internal/api/customers.go:31`, `backend/internal/filter/filter.go:88-152`.

#### 4.2 Product-like organization vs demo
- Conclusion: **Pass**
- Rationale: Schema is real (tsvector, generated search columns, triggers for immutable audit, unique partial index for exception dedupe, array+JSONB columns), security headers set, CORS scoped, admin management endpoints, analytics aggregates, multi-stage Dockerfiles, ephemeral test DB compose. Not a toy example.
- Evidence: `migrations/0001_init.sql:64,145,209,225-250`, `backend/internal/httpx/middleware.go:88-100`, `backend/Dockerfile:1`, `frontend/Dockerfile:1`.

### 5. Prompt Understanding and Requirement Fit

#### 5.1 Business objective and constraints fit
- Conclusion: **Partial Pass**
- Rationale: Most domain semantics are faithfully implemented: Argon2id + 10-char policy + 5-attempt/15-min lockout persisted in DB (survives restart), versioned reports with reason notes and forbidden delete of superseded, archive requires note, MM/DD/YYYY parser, cent-based pricing, per-env ENC_KEYS with rotation, per-user saved filters, per-user recent-20 in localStorage, address-book per-user with encrypted street, audit records workstation + workstation-time headers. Deviations and gaps:
  1. Role constraints weakly enforced at the route layer (see Issues A1–A3).
  2. "Offline map image" rendered as bare SVG polygons without a raster image (`OfflineMap.tsx:79-97`).
  3. CORS is wide-open (`AllowOrigins: {"*"}`) even though the Prompt implies LAN-only — noted as a deliberate choice in code comment but still loose for a "portal".
  4. Default `ENC_KEYS` in `.env.example` is a well-known value (`1:0101...01`); flagged in the file itself but if copied as-is to production renders at-rest encryption useless.
- Evidence: `backend/internal/auth/password.go:31,40`, `auth/store_lockout.go:88-96`, `migrations/0001_init.sql:42-47`, `lab/report.go:102-128`, `filter/filter.go:73-84`, `frontend/src/hooks/useRecentSearches.ts:7-39`, `backend/internal/api/server.go:97-101`, `.env.example:22`.

### 6. Aesthetics (frontend)

#### 6.1 Visual and interaction design
- Conclusion: **Pass**
- Rationale: Single cohesive stylesheet with CSS variables, per-workspace accent stripe, card layout with consistent spacing, explicit abnormal-row styling, OK/error banners, hover states for dropdown, focus-visible not explicit but inputs consistent. Not a raw unstyled demo.
- Evidence: `frontend/src/styles.css:1-224`, `frontend/src/App.tsx:50-103`, `frontend/src/components/OfflineMap.tsx` (map theme + pin colors), `frontend/src/components/ReportWorkspace.tsx:66-80` (abnormal red).
- Manual verification: Actual browser rendering not executed.

---

## 5. Issues / Suggestions (Severity-Rated)

### A1 — High: Analyst role can perform mutating operations on orders, samples, reports, customers
- Severity: **High**
- Conclusion: Role gates on the order, lab, and customer route groups include `RoleAnalyst`, contradicting the Prompt ("Analysts who build operational reports") and the README ("Analyst — Read-only operational analytics").
- Evidence:
  - `repo/backend/internal/api/server.go:117` — customers group admits `RoleAnalyst` and exposes `POST /api/customers`, `PATCH /api/customers/:id`.
  - `repo/backend/internal/api/server.go:130-139` — `orderRoles` admits `RoleAnalyst` and exposes `POST /api/orders`, `POST /api/orders/:id/transitions`, `POST /api/orders/:id/out-of-stock/plan`, `POST /api/orders/:id/inventory`.
  - `repo/backend/internal/api/server.go:142-153` — `lab` group admits `RoleAnalyst` and exposes `POST /api/samples`, `POST /api/samples/:id/transitions`, `POST /api/samples/:id/report`, `POST /api/reports/:id/correct`, `POST /api/reports/:id/archive`.
  - `repo/README.md:138` — documents Analyst as read-only.
- Impact: Separation-of-duties violation. An operator with the analyst role can originate, mutate, and archive primary records; the audit log would show the analyst as actor, but the access should not be permitted.
- Minimum actionable fix: Tighten `RequireRoles` on each mutating route to the correct subset (e.g., orders-write to `FrontDesk, Admin`; sample-write to `LabTech, Admin`; customer-write to `FrontDesk, Admin`), or migrate all handlers onto `RequirePermission` with dedicated permission IDs (orders.write, samples.write, reports.write, customers.write already exist in the catalog — see `migrations/0001_init.sql:254-296`).
- Verification path: After tightening, a static role-matrix test in `backend/internal/api/security_test.go` should assert 403 for analyst on each of those endpoints. The current matrix (`security_test.go:61-82`) covers only admin endpoints for non-admins and a single `POST /api/samples` deny for front desk (`security_test.go:86-93`), which is insufficient.

### A2 — High: Lab-only endpoints accept every allowed role for read and write without distinction
- Severity: **High**
- Conclusion: The `lab` group admits `LabTech, Admin, Analyst` uniformly across read and write endpoints, so `GetReport`, `SearchReports`, `ListSamples` are read-available to the right roles, but `CreateReportDraft`, `CorrectReport`, and `ArchiveReport` are equally exposed to `RoleAnalyst`. The Prompt reserves those for lab staff.
- Evidence: `repo/backend/internal/api/server.go:142-153`; handlers `repo/backend/internal/api/lab.go:121,187,255`.
- Impact: Same separation-of-duties concern as A1; specifically, an analyst could supersede an issued report or archive it.
- Minimum actionable fix: Split `lab` into read-group (LabTech, Admin, Analyst) and write-group (LabTech, Admin), or gate writes via `RequirePermission("reports.write" / "samples.write" / "reports.archive")`.

### A3 — High: Customer write endpoints accessible to LabTech and Dispatch
- Severity: **High**
- Conclusion: The customer group admits FrontDesk, Admin, Analyst, LabTech, Dispatch and exposes `POST/PATCH` under the same gate, allowing LabTech or Dispatch to create or edit customers although the Prompt scopes customer registration to Front Desk.
- Evidence: `repo/backend/internal/api/server.go:117-122`.
- Impact: PII ownership boundary is weakened; changes may bypass intended front-desk workflow and any business-rule enforcement tied to it.
- Minimum actionable fix: Gate `POST /api/customers` and `PATCH /api/customers/:id` to `FrontDesk, Admin` while keeping GETs open to the wider set (or permission-driven).

### A4 — High: `.env.example` ships a well-known default `ENC_KEYS` and deploys as-is via `start.sh`
- Severity: **High**
- Conclusion: `ENC_KEYS=1:0101010101...01` is the default value that `start.sh` propagates on first run (it copies `.env.example` → `.env` when absent). The code accepts this value as a legitimate 32-byte AES key. A deployment that forgets to rotate the key has at-rest encryption that is trivially reversible by anyone who knows the default.
- Evidence: `.env.example:22`, `start.sh:17-20`, `backend/internal/runtime/runtime.go:198-212` (accepts any hex-64 key of version ≥1), `docker-compose.yml:37`.
- Impact: Reduces the at-rest-encryption requirement to a no-op in any environment where the default is copied.
- Minimum actionable fix: Either make `.env.example` not contain a hex key (leave `ENC_KEYS=`), require `OOPS_DEV_MODE=1` for an ephemeral key, and print a warning in `start.sh` when the default is detected; or generate a random key on first copy.

### A5 — Medium: CORS allows `*` origin on a portal that expects LAN-only operation
- Severity: **Medium**
- Conclusion: Echo is configured with `AllowOrigins: {"*"}` and explicit allowed headers including `Authorization`. Token is stored in `localStorage` in the SPA, so classic cookie-CSRF is not triggered; however, the wide-open origin lets any third-party page issue authenticated JSON requests once a user's token is exfiltrated or when the portal is bridged.
- Evidence: `backend/internal/api/server.go:97-101`, `frontend/src/api/client.ts:7-15`.
- Impact: Defense-in-depth weakened. Combined with `unsafe-inline` style CSP (`middleware.go:97`), browser-level isolation is looser than the "LAN-only" framing implies.
- Minimum actionable fix: Default `AllowOrigins` to the frontend host (`http://localhost:3000` in compose, configurable via env), keeping `*` behind an explicit opt-in env flag.

### A6 — Medium: `OfflineMap` renders polygons without a bundled map raster image
- Severity: **Medium**
- Conclusion: Prompt says "offline map **image** of the service territory"; implementation draws polygons on a solid-color SVG background. Functionally the pin/geofence UX is supported, but the "image" requirement is not strictly met.
- Evidence: `frontend/src/components/OfflineMap.tsx:79-98`, `frontend/src/styles.css:171-178`.
- Impact: Operators lose the spatial context that a real map image provides when identifying which part of the service area they are clicking; audit/UX fit for dispatch is reduced.
- Minimum actionable fix: Ship a preloaded raster (PNG/SVG) map asset under `frontend/public/` or `src/assets/`, overlay the polygons on top of it in the `svg` or a `background-image`, with the pixel-to-latlng transform still anchored to the bounding box.

### A7 — Medium: Saved-filter "too broad" check only fires when `size > 100`
- Severity: **Medium**
- Conclusion: Prompt says saved filters must be "validated to prevent overly broad exports". Current rule only rejects the filter when `size > 100` AND there are no narrowing criteria. At `size ≤ 100` an entirely empty filter is accepted and can still paginate the full dataset by incrementing `page`.
- Evidence: `backend/internal/filter/filter.go:141-150`, `backend/internal/api/filters_search.go:24-46`.
- Impact: Export guardrail is bypassable via paging. An analyst with access to saved filters can still dump the table.
- Minimum actionable fix: Require at least one narrowing criterion on **save**, regardless of page size; separately cap max page count on query (or total rows returned per saved-filter query).

### A8 — Medium: `QueryOrders` silently ignores date parse errors after `Validate`
- Severity: **Medium**
- Conclusion: After the filter passes `Validate`, the handler calls `filter.ParseDate` again but discards the error. A validated-then-mutated body (race window) or a subtle format drift would produce a zero time and silently skew the query without signaling the caller.
- Evidence: `backend/internal/api/orders.go:124-134`.
- Impact: Low-probability correctness hazard; logs/audit still record the dates, but response data could be unexpectedly broad.
- Minimum actionable fix: Re-check the error from `ParseDate` or parse once inside `Validate` and stash the parsed times on the filter.

### A9 — Medium: `OrderStatusCounts` and related analytics APIs don't enforce bounded window
- Severity: **Medium**
- Conclusion: `parseWindow` treats unset `from`/`to` as 0/0 and passes them through; whether the store-layer interprets `(0,0)` as "all time" is an implementation-detail gap visible at this layer.
- Evidence: `backend/internal/api/analytics.go:16-20,75-89`; store interface `backend/internal/store/store.go:191-197`.
- Impact: Cannot confirm without reading full `memory.go`/`postgres.go` analytics methods; marked as a consistency concern.
- Minimum actionable fix: Document the "0 means unbounded" contract and default `to = now` / `from = now - 90d` at the handler when both are zero.

### A10 — Low: `api/server.go` sets `auth` as a local variable name that shadows the imported `auth` package
- Severity: **Low**
- Conclusion: `auth := e.Group(...)` on `server.go:112` shadows the `auth` package name inside `Register`. No handlers use the package afterwards in this function, so it compiles, but is a readability/maintainability hazard.
- Evidence: `backend/internal/api/server.go:112`.
- Minimum actionable fix: Rename the local to `authGroup`.

### A11 — Low: Login response leaks whether username exists via timing
- Severity: **Low**
- Conclusion: For a missing username the handler calls `Lockout.RecordFailure` and returns 401; for a wrong password it hashes and compares then records failure. The latter costs tens-to-hundreds of ms for Argon2id; the former returns almost immediately. A timing probe can enumerate usernames.
- Evidence: `backend/internal/api/auth.go:30-47`, `backend/internal/auth/password.go:71-94`.
- Minimum actionable fix: Run a dummy `auth.ComparePassword` against a static hash on the username-not-found branch to equalize timing.

### A12 — Low: Seeded demo users with documented passwords active by default
- Severity: **Low**
- Conclusion: `SEED_DEMO_USERS=1` in `.env.example`; on first start the portal installs five accounts whose passwords are published in the README.
- Evidence: `.env.example:35`, `README.md:127-139`, `runtime.go:94-102`.
- Impact: Convenience for evaluators, but a real deployment that forgets to flip this flag ships with five default credentials.
- Minimum actionable fix: Default to `SEED_DEMO_USERS=0` and document the opt-in, or force password rotation on first admin login.

### A13 — Low: `Unmeasurable` measurements are returned with `Flag = "normal"` when no ref-range matches
- Severity: **Low**
- Conclusion: In `EvaluateAll`, when `rs.Match` returns `ErrNoRefRange`, the code stores `FlagNormal` for a measurement that is not actually known to be normal. The frontend then will not red-highlight it, which may mislead clinicians for an uncategorized test code.
- Evidence: `backend/internal/lab/reference.go:115-122`.
- Minimum actionable fix: Use a distinct `FlagUncategorized` (or empty flag) and render it neutrally (e.g., grey) rather than "normal".

---

## 6. Security Review Summary

- Authentication entry points: **Pass** — only `POST /api/auth/login` is public; lockout + Argon2id + policy enforced; persistent store-backed lockout survives restart. Evidence: `api/auth.go:15`, `auth/password.go:40`, `auth/store_lockout.go:53`, `migrations/0001_init.sql:42`.
- Route-level authorization: **Partial Pass** — `RequireAuth` and `RequireRoles` are applied uniformly, admin group is correctly gated. However several role groups are too permissive (Issues A1–A3). Evidence: `api/server.go:112-198`.
- Object-level authorization: **Pass (partial static proof)** — owner-scoped resources (address book, saved filters) are filtered by `sess.UserID` at the store layer and delete operations require the owner match. Evidence: `api/addressbook.go:14,81-98`, `api/filters_search.go:54-73`, `store/postgres.go:214,233`. Tests assert cross-user isolation for both resources (`security_test.go:96-153`).
- Function-level authorization: **Partial Pass** — analytics uses `RequirePermission("analytics.view")`; other endpoints do not reach the finer permission catalog, so admin-granted permission changes do not flow to orders/samples/customers. Evidence: `api/server.go:192`, middleware `httpx/middleware.go:66`.
- Tenant / user isolation: **Pass** — per-user scoping for address book and saved filters verified statically + by tests.
- Admin / internal / debug protection: **Pass** — `/api/admin/*` routed through `RequireRoles(RoleAdmin)` and admin mutations audited. No unprotected `/debug` endpoint found. Evidence: `api/server.go:170-185`, audit calls throughout `api/admin.go`, `api/permissions.go`.
- Suspected risk (cannot confirm statically): `OOPS_DEV_MODE=1` enabling ephemeral in-process key path; real risk only if an operator sets it in production (Issue A4 related).

---

## 7. Tests and Logging Review

- Unit tests: **Pass** — every core package has `_test.go` files (`auth`, `crypto`, `audit`, `lab`, `order`, `geo`, `search`, `filter`, `httpx`, `runtime`, `store`, `api`). Evidence: `backend/internal/*/(_test.go)`.
- API / integration tests: **Pass (basic)** — `backend/internal/api/*_test.go` exercises handlers via `httptest`, including role matrix for admin, cross-user isolation, unauthenticated matrix, sample-gate conflict, audit coverage assertions. A Postgres integration suite is wired through `INTEGRATION_DB` and `docker-compose.test.yml`. Evidence: `api/security_test.go:23,61,96,131,158`, `api/coverage_test.go:17`, `store/postgres_integration_test.go` (listed), `docker-compose.test.yml:27`.
- Logging categories / observability: **Pass** — structured JSON middleware with request-ID correlation, level assignment based on status, session/workstation fields. Error handler funnels unknown errors with `log.Printf("[err] %s %s: %v", ...)`. Evidence: `httpx/logging.go:39,66,115-142`, `httpx/context.go:85-88`.
- Sensitive-data leakage risk in logs/responses: **Pass (partial)** — audit uses `orderRedact`/`redactCustomer` so plaintext PII doesn't enter the audit log; `WriteError` hides internal error detail. The structured log does include `Path` and `Error` but not request bodies; passwords are never logged. Risk residual: tokens in `Authorization` header are not logged. Evidence: `api/orders.go:76`, `api/customers.go:207`, `httpx/context.go:85`.

---

## 8. Test Coverage Assessment (Static Audit)

### 8.1 Test Overview
- Unit tests and API/integration tests both exist.
- Frameworks: Go stdlib `testing` + `net/http/httptest` on the backend; Vitest (`@testing-library/react`) on the frontend (`frontend/package.json`, `frontend/src/test-setup.ts`).
- Entry points: `go test -race -cover ./...` (`docker-compose.test.yml:34`); `npm test` → `vitest run` (`package.json` — referenced by `docker-compose.test.yml:40`).
- Documentation provides test commands: `repo/README.md:99-123`, `repo/run_tests.sh:29-69`.
- Evidence: `backend/internal/api/security_test.go:1`, `coverage_test.go:1`, `extra_coverage_test.go`, `full_coverage_test.go`, `permissions_test.go`, `bind_errors_test.go`, `error_paths_test.go`, `matrix_test.go`; `frontend/src/**/*.test.ts*`.

### 8.2 Coverage Mapping Table

| Requirement / Risk Point | Mapped Test (`file:line`) | Key assertion / fixture | Coverage | Gap | Minimum addition |
|---|---|---|---|---|---|
| Password policy ≥10 chars | `backend/internal/auth/password_test.go:1` | `ValidatePolicy` / `HashPassword` happy + error | sufficient | — | — |
| Lockout after 5 failures / 15 min | `backend/internal/auth/lockout_test.go:1`, `store_lockout_test.go:1` | counter/expiry transitions | sufficient | — | — |
| AES-256-GCM vault + rotation | `backend/internal/crypto/crypto_test.go:1`, `extra_coverage_test.go:1` | encrypt/decrypt + unknown version | sufficient | — | — |
| Order state machine + refund reason | `backend/internal/order/workflow_test.go:1`, `extra_coverage_test.go:1` | transitions + refund reason required | sufficient | — | — |
| Picking timeout (30 min) | `backend/internal/order/extra_coverage_test.go` (detector) | DetectPickingTimeout | basically covered | Boundary (exactly 30 min) not explicit | Add exact-threshold test |
| Out-of-stock + split suggestion | workflow_test / extra_coverage_test | DetectOutOfStock + PlanOutOfStock | basically covered | UI-level "prompt to split" flow not integration-tested | Add handler test for `/plan` including audit emission |
| Report versioning + optimistic concurrency + reason | `backend/internal/lab/lab_test.go:1`, `final_test.go` | Correct returns ErrVersionConflict / ErrReasonRequired | sufficient | — | — |
| Archive requires note + one-way | `backend/internal/lab/extra_coverage_test.go` + handler in api | Archive returns ErrAlreadyArchived; handler rejects empty note | sufficient | — | — |
| Geofence: point-in-polygon | `backend/internal/geo/polygon_test.go:1`, `final_test.go` | Contains edge cases + RegionForPoint | sufficient | — | — |
| Route-table + haversine fallback | `backend/internal/geo/distance_test.go:1` | route_table vs haversine method | sufficient | — | — |
| Fuzzy search ranking | `backend/internal/search/fuzzy_test.go:1`, `final_test.go` | Levenshtein + tolerance + Rank ordering | sufficient | — | — |
| Filter validation (MM/DD/YYYY, too-broad) | `backend/internal/filter/filter_test.go:1`, `final_test.go` | ParseDate + Validate coverage | basically covered | Page-iteration bypass (Issue A7) not covered | Add test asserting that an empty filter at size ≤ 100 cannot access unbounded page |
| Recent-20 searches per user | `frontend/src/hooks/useRecentSearches.test.tsx` | LIMIT enforcement + keyed by user | basically covered | — | — |
| 401 for unauthenticated routes | `backend/internal/api/security_test.go:23-58` | matrix of 20 endpoints returns 401 | sufficient | — | — |
| 403 role enforcement (admin endpoints) | `security_test.go:61-82` | non-admin → 403 on admin endpoints | sufficient | — | — |
| 403 role enforcement (lab endpoints vs non-lab) | `security_test.go:86-93` | front-desk → 403 on POST /api/samples | **insufficient** | Analyst NOT tested on any write endpoint (Issues A1–A3) | Add analyst-as-actor deny matrix for `/api/orders`, `/api/samples`, `/api/reports/:id/correct`, `/api/reports/:id/archive`, `POST/PATCH /api/customers` |
| Cross-user isolation (saved filters / address book) | `security_test.go:96-153` | bob cannot see/delete alice's rows | sufficient | — | — |
| 404 not-found mapping | `error_paths_test.go` (listed) | `WriteError` mapping | basically covered | Cannot confirm every handler branch; sampled |
| Conflict on report re-issue for same sample | `coverage_test.go:17-50` | 409 on wrong sample status | sufficient | — | — |
| Sensitive log exposure | None dedicated | — | missing | — | Add a test that runs a mutating endpoint and asserts the structured log line does not contain `delivery_street`, `identifier`, or `Authorization` header content |
| Audit mutation coverage | `security_test.go:158-276` | audit entry created for each mutation | sufficient | — | — |
| CSRF / CORS behavior | None | — | cannot confirm statically | Route-level CORS test is absent | Add a handler test asserting CORS headers match expectation |

### 8.3 Security Coverage Audit

- Authentication: **basically covered** — policy + lockout + session issue/lookup all exercised.
- Route authorization: **insufficient** — admin-endpoint deny is covered; the analyst/labtech/dispatch cross-role deny matrix for write endpoints is NOT covered, directly corresponding to the defects in Issues A1–A3. A green test run therefore does not catch the current role leak.
- Object-level authorization: **basically covered** — saved filters and address book cross-user isolation tested; no test confirms that customer or order reads are scoped (though they are not owner-scoped by design).
- Tenant/data isolation: **basically covered** for per-user resources.
- Admin/internal protection: **basically covered** — matrix test asserts non-admin receives 403 on admin routes.

### 8.4 Final Coverage Judgment

**Partial Pass.**

- Covered risks: password policy, lockout, encryption envelope, state-machine transitions, report versioning, filter validation, recent-20 hook, admin-route deny, cross-user isolation.
- Uncovered risks (material): analyst role allowed to mutate orders/samples/reports/customers (Issues A1–A3) — test suite lacks the deny assertions that would surface this; saved-filter "too broad" bypass by paging (A7); CORS/CSP behavior; sensitive-field leakage in logs; CSRF-by-open-origin; default ENC_KEYS usage. Consequence: the test suite could pass while each of these defects remains in the code.

---

## 9. Final Notes

- The delivery is substantial and on-topic for the Prompt; the headline risks are concentrated in role-authorization consistency and the default secret-handling in `.env.example`. Fixing those would materially raise confidence in an operationally deployed portal.
- Where this report says Pass, it is based on static evidence only; any claim that depends on running the stack (container startup, real Postgres tsvector search quality, real browser rendering of abnormal highlighting) is **Manual Verification Required**.
- Recommended next steps, in priority order: (1) tighten route groups per A1–A3 and add deny tests; (2) strip the default hex key from `.env.example` (A4); (3) restrict CORS origin (A5); (4) bundle a map raster or adjust the Prompt expectation (A6); (5) harden saved-filter validation (A7); (6) minor correctness/hygiene items (A8–A13).
