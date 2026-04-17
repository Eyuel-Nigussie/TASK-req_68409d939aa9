# Unified Offline Operations Portal — System Design

**Document version:** 1.0
**Date:** 2026-04-17
**Target platform:** Linux / macOS server, any modern browser for the UI
**Backend language:** Go 1.22
**Frontend language:** TypeScript 5 / React 18
**Web framework:** Echo v4 (backend), Vite + React Router v6 (frontend)
**Database:** PostgreSQL 14+
**Deployment model:** Offline, single-tenant, local network or single machine

---

## 1. Overview

The Unified Offline Operations Portal serves a regional laboratory and
fulfillment business that must run without any external network
dependency. A single Go/Echo backend, a React/TypeScript SPA, and a local
PostgreSQL instance together support five operator roles — Front Desk,
Lab Technician, Dispatch Coordinator, Analyst, Administrator — through a
single web UI accessible only on the internal LAN.

Every feature is offline by design: geofence validation, full-text
search, report versioning, distance-based fee computation, abnormal
result flagging, and dispatch exception queues all operate without any
outbound network call. The server is a thin Echo router; the heavy
lifting lives in platform-free domain packages that are independently
testable.

### 1.1 Roles

Defined in `internal/models/models.go` as the `Role` constant set. The
permission catalog + role grants live in PostgreSQL (`permissions`,
`role_permissions`, `user_permissions` tables) and are seeded to a
sensible default by the initial migration. Administrators edit grants
at runtime via `/api/admin/role-permissions/:role`.

| Role | Default permissions |
|---|---|
| `front_desk` | `customers.read`, `customers.write`, `orders.read`, `orders.write` |
| `lab_tech` | `samples.read`, `samples.write`, `reports.read`, `reports.write`, `reports.archive` |
| `dispatch` | `orders.read`, `dispatch.validate`, `customers.read` |
| `analyst` | `customers.read`, `orders.read`, `samples.read`, `reports.read`, `analytics.view` |
| `admin` | all 16 catalog permissions |

### 1.2 Primary use cases

1. A **front-desk clerk** registers a customer, creates an order with line
   items + delivery address + tags, and transitions it through placement
   → picking → dispatched → delivered → received.
2. A **lab technician** submits a sample, advances it sampling → received
   → in-testing, issues a v1 report with measurements; the server
   evaluates each measurement against the reference-range dictionary and
   flags `normal` / `low` / `high` / `critical_low` / `critical_high`.
3. A **lab technician** corrects an issued report. The server requires
   an `expected_version` for optimistic concurrency, writes a new
   `version+1` row, marks the prior row `superseded`, and keeps it
   readable forever.
4. A **dispatch coordinator** clicks the offline SVG map to place a pin,
   the server validates the location against configured service-area
   polygons using ray-casting, and returns immediate in/out-of-zone
   feedback. A second flow quotes a delivery fee using the preloaded
   route table, falling back to haversine distance.
5. The **exception queue** automatically flags: orders stuck in `picking`
   for more than 30 minutes, and orders with one or more backordered
   line items.
6. An **analyst** views the Analytics dashboard (order status counts,
   daily series, abnormal result rate, open exception breakdown) bounded
   by a date range.
7. An **administrator** manages users, role↔permission grants,
   reference-range dictionary, service-area polygons, route table, and
   reviews the append-only audit log.

### 1.3 Out of scope

- Internet connectivity, external identity providers (SSO, OAuth),
  push notifications from a backend, server-driven analytics.
- Mobile apps; the UI targets modern desktop browsers on the LAN.
- Multi-tenancy. Each installation serves one operating organization.
- Server-to-server federation (e.g., shipping tracking feeds).

---

## 2. Architecture

### 2.1 Layering

```
┌─────────────────────────────────────────────────────────────┐
│              Browser (React + TypeScript SPA)               │
│  Login · Dashboard · Customers · Orders · Lab · Dispatch    │
│  AddressBook · Analytics · Admin · Global search · Modal    │
│  GlobalSearch · AdvancedFilters · OfflineMap · BarChart     │
│  useAuth · useRecentSearches · typed api client             │
└─────────────────────────────────────────────────────────────┘
                               │ HTTPS on the LAN
┌─────────────────────────────────────────────────────────────┐
│                      api (Echo v4 router)                    │
│  handlers: auth · customers · orders · lab · dispatch ·     │
│  admin · permissions · analytics · filters_search ·         │
│  addressbook                                                │
│  middleware: RequireAuth · RequireRoles · RequirePermission │
│  · SecurityHeaders · StructuredLogging · Recover · CORS     │
└─────────────────────────────────────────────────────────────┘
                               │
┌─────────────────────────────────────────────────────────────┐
│                  Domain Packages (platform-free)             │
│  order (state machine + exceptions · LineItem · OOS)        │
│  lab (sample lifecycle · reference ranges · report          │
│     versioning · archive)                                   │
│  geo (point-in-polygon · route table · haversine · fee)     │
│  filter (validated advanced-filter payloads · canonical key)│
│  search (typo-tolerant ranking · tokens · Levenshtein)      │
│  auth (Argon2id · session store · lockout · StoreLockout)   │
│  crypto (AES-GCM vault with versioned keys)                 │
│  audit (structured immutable log)                           │
│  runtime (env parsing · key generation · seed)              │
│  httpx (middleware · structured logging · error mapping)    │
└─────────────────────────────────────────────────────────────┘
                               │
┌─────────────────────────────────────────────────────────────┐
│                    store (repository interface)              │
│  Store = Users · Customers · AddressBook · ServiceAreas ·   │
│     Orders · Samples · Reports · SavedFilters · Audit ·     │
│     RefRanges · Routes · Permissions · LoginAttempts ·      │
│     Analytics                                               │
│  Implementations: Memory (tests, quickstart) · Postgres     │
└─────────────────────────────────────────────────────────────┘
                               │
┌─────────────────────────────────────────────────────────────┐
│                       PostgreSQL 14+                         │
│  tsvector indexes (customers, reports)                      │
│  Append-only audit_log enforced by trigger                  │
│  Versioned reports with unique (sample_id, version)         │
│  JSONB polygons, JSONB measurements, TEXT[] tags            │
└─────────────────────────────────────────────────────────────┘
```

### 2.2 Repository layout

```
repo/
├── backend/
│   ├── cmd/
│   │   ├── server/main.go       # Echo entry point
│   │   ├── seed/main.go         # admin + demo region bootstrap
│   │   └── keygen/main.go       # ENC_KEYS generator
│   ├── internal/
│   │   ├── api/                 # Echo handlers + wiring
│   │   ├── audit/               # Logger (append-only)
│   │   ├── auth/                # password, lockout, session
│   │   ├── crypto/              # AES-GCM vault
│   │   ├── filter/              # advanced-filter validation
│   │   ├── geo/                 # polygon + distance + fee
│   │   ├── httpx/                # middleware + logging + error mapping
│   │   ├── lab/                 # sample + report domain
│   │   ├── models/              # shared domain types
│   │   ├── order/               # order state machine + exceptions
│   │   ├── runtime/             # cmd helpers (env, keygen, seed)
│   │   ├── search/              # typo-tolerant ranking
│   │   └── store/               # interface + Memory + Postgres
│   └── migrations/
│       └── 0001_init.sql        # schema + seed permissions
├── frontend/
│   ├── index.html
│   ├── src/
│   │   ├── api/client.ts        # typed fetch wrapper + endpoint map
│   │   ├── components/          # GlobalSearch, OfflineMap, …
│   │   ├── hooks/               # useAuth, useRecentSearches
│   │   ├── pages/               # Login, Dashboard, Orders, …
│   │   ├── lib/fuzzy.ts         # client-side fuzzy scorer
│   │   ├── styles.css
│   │   └── App.tsx
│   ├── vite.config.ts
│   └── package.json
├── docs/
│   ├── design.md                # (this document)
│   └── apispec.md
└── README.md
```

### 2.3 Dependencies

- **Echo v4** — HTTP router and middleware chain.
- **lib/pq** — database/sql driver for PostgreSQL.
- **golang.org/x/crypto/argon2** — password hashing.
- **React 18 + React Router v6** — SPA shell and routing.
- **Vite 5** — dev server + production build.
- **Vitest 2 + Testing Library** — frontend unit and component tests.

No server-side frontend rendering; the SPA is served as static files.

### 2.4 Composition root

`internal/api.New(store.Store, *crypto.Vault, auth.Clock) *Server`
constructs every domain service with its required dependencies wired.
Tests pass a `store.NewMemory()` + an ephemeral vault + a fake clock;
production passes `store.NewPostgres(db)` + the real vault loaded from
`ENC_KEYS` + `auth.RealClock`. `Server.Register(echo.Echo)` mounts every
route group in one pass.

### 2.5 Threading model

Echo handles requests concurrently; every handler reads from and writes
to `store.Store`, which is goroutine-safe in both implementations
(Memory uses `sync.RWMutex`, Postgres relies on the driver). Domain
helpers (`order.Transition`, `lab.Correct`, `geo.RegionForPoint`,
`search.Rank`) are pure functions operating on pass-by-value inputs,
trivially safe to share. The in-memory `auth.SessionStore` is the only
process-local mutable cache; losing it on restart simply requires users
to log in again.

### 2.6 Navigation

- On launch, the SPA checks `localStorage` for a bearer token and calls
  `/api/auth/whoami` to hydrate `useAuth`. Absence of a session redirects
  to `/login`.
- Authenticated routes live inside `App.tsx`'s `Shell` component with a
  role-gated sidenav. Entries are shown only when the user's role is in
  the allowlist for the page (e.g., `/admin` only for `admin`).
- Global search results navigate to `/customers/:id`, `/orders/:id`, or
  `/reports/:id` depending on the suggestion `kind`. Detail pages for
  each kind are wired.
- A `data-workspace="..."` attribute on `.app-shell` drives per-workspace
  accent stripe colors (customers → sky, orders → blue, lab → violet,
  dispatch → emerald, analytics → amber, admin → slate).

---

## 3. Domain Model

### 3.1 Identity & Access

**`User`** (`internal/models/models.go`)
```go
type User struct {
    ID           string
    Username     string
    Role         Role   // front_desk | lab_tech | dispatch | analyst | admin
    PasswordHash string // Argon2id encoded
    Disabled     bool
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

**`Permission`** — atom of authorization policy (e.g., `orders.write`).
Persisted in the `permissions` table; admin CRUD available via
`/api/admin/permissions` and the role-permission matrix endpoints.

**`RolePolicy` (dynamic)** — resolved at runtime by
`store.Permissions.GrantsForUser(userID, role)`. The `RequirePermission`
middleware refreshes grants on every request so admin changes take
effect without a session refresh.

**`RequireRoles`** — fixed role-set gate for route groups whose policy
doesn't vary (e.g., admin routes). Layered in front of handlers that
still use role gating for backwards compatibility.

### 3.2 Customers

**`Customer`** — `ID`, `Name`, encrypted `Identifier`, encrypted
`Street`, plain `City`/`State`/`ZIP`, `Phone`, `Email`, `Tags []string`.
The `identifier_enc` and `street_enc` columns hold `v1:<base64(…)>`
envelopes; city/ZIP remain plain to support indexed searches.

**`AddressBookEntry`** — per-user favorites (`OwnerID` = user id) with
an encrypted street; listable via `/api/address-book`.

### 3.3 Orders

**`Order`** — `ID`, `Status`, `CustomerID`, `PlacedAt`/`UpdatedAt`,
`TotalCents`, `Priority`, `Tags []string`, `Items []LineItem`, and
delivery fields (`DeliveryStreet` encrypted, `DeliveryCity/State/ZIP`
plain).

**`LineItem`** — `SKU`, `Description`, `Qty`, `Backordered bool`.
Backordered=true is the inventory signal that drives automatic
out-of-stock detection.

**State machine** (`internal/order/workflow.go`) — `placed` → `picking`
→ `dispatched` → `delivered` → `received`; terminal `refunded` and
`canceled`. Illegal transitions return 400. Refunds require a
non-empty `reason`.

**`Exception`** — `OrderID`, `Kind` (`picking_timeout` |
`out_of_stock`), `DetectedAt`, `Description`. Detectors are pure:

- `DetectPickingTimeout(order, now)` flags an order in `picking` whose
  `UpdatedAt` is older than 30 minutes.
- `DetectOutOfStock(order, now)` flags an order with any
  `LineItem.Backordered == true`.

The exception queue endpoint runs both detectors over the current
working set on every read, idempotent by (order, kind).

### 3.4 Lab workflow

**`Sample`** — `ID`, `OrderID`/`CustomerID`, `Status`, `CollectedAt`,
`UpdatedAt`, `TestCodes []string`, `Notes`. Lifecycle:

`sampling → received → in_testing → reported` (plus `rejected` as a
terminal failure from any pre-reporting state).

**`Report`** — `ID`, `SampleID`, `Version`, `Status`
(`draft|issued|superseded`), `Title`, `Narrative`, `Measurements`,
`AuthorID`, `ReasonNote` (required for v≥2), `IssuedAt`,
`SupersededByID`, `ArchivedAt/By/Note`, `SearchText` (feeds the
tsvector index).

**Controlled workflow guards** (in `CreateReportDraft`):
1. Sample must exist (404).
2. Sample must be in `in_testing` or `reported` (409 otherwise).
3. Sample must not already have a report (409 — use the correction
   endpoint).
On success, an `in_testing` sample is atomically transitioned to
`reported`.

**`RefRange`** — per-test-code thresholds with optional demographic
refinement. `EvaluateAll` classifies measurements into `normal`, `low`,
`high`, `critical_low`, `critical_high`, or `unmeasurable`.

**Correction** (`lab.Correct`) — takes the prior report and an expected
version; returns the new `version+1` row and marks the prior row
superseded. `ErrVersionConflict` is thrown on optimistic-concurrency
failure. A correction REQUIRES a non-empty reason note.

**Archive** (`lab.Archive`) — one-way action with a mandatory note.
Archived reports are hidden from the default list, included in the
archived-list endpoint, and remain searchable via full-text search.

### 3.5 Dispatch & geo

**`Point`** — `Lat`, `Lng` (decimal degrees, WGS-84).
**`Polygon`** — ordered `Vertices []Point` (implicitly closed).
**`Region`** — a `Polygon` plus `BaseFeeCents` and `PerMileFeeCents`.

`Polygon.Contains(Point)` uses the ray-casting algorithm with an inline
on-segment short-circuit so boundary points count as inside.

**`RouteTable`** — preloaded origin/destination mileage matrix keyed by
normalized `(from, to)` pair. `Distance(fromID, toID, fromPt, toPt)`
returns `DistanceResult{Miles, Method}` where `Method` is
`"route_table"` (hit) or `"haversine"` (fallback).

**Fee math** — `FeeCents(region, miles)` is integer cents; fractional
cents are half-up rounded with a tiny epsilon to stabilize float drift
(e.g., `4.02 * 25 = 100.5` rounds up to 101).

### 3.6 Analytics

`store.Analytics` is a read-only interface producing deterministic,
bounded aggregates for the analyst workspace:

- `OrderStatusCounts(from, to)` — grouped counts.
- `OrdersPerDay(from, to)` — daily bucket time series.
- `SampleStatusCounts(from, to)` — grouped counts.
- `AbnormalReportRate(from, to)` — (abnormal / total) over issued
  reports in window.
- `ExceptionCountsByKind()` — open-exception histogram.

Every method returns a map or slice capped by the underlying SQL scope;
there is no unbounded export path.

### 3.7 Persistence

Every domain service persists through the `store.Store` interface.
On a Postgres-backed install, `migrations/0001_init.sql` creates:

- `users`, `customers`, `address_book`
- `orders`, `order_events`, `order_exceptions`
- `samples`, `reports`, `reference_ranges`, `saved_filters`
- `service_regions`, `route_distances`
- `permissions`, `role_permissions`, `user_permissions`
- `login_attempts`
- `audit_log` with an immutable trigger rejecting UPDATE/DELETE

tsvector GENERATED columns on `customers.search_tsv` and
`reports.search_tsv` feed GIN indexes for full-text search.

### 3.8 Encryption at rest

`crypto.Vault` holds one or more 32-byte AES keys keyed by version
number. Every ciphertext is an envelope:

```
v1:<base64(keyVersion||nonce||ciphertext)>
```

Writes use the highest version; reads decrypt whatever version is
embedded so rotation is additive. Keys are loaded from the `ENC_KEYS`
env var (`1:hex32,2:hex32`). Production refuses to start with
`ENC_KEYS` unset; the `OOPS_DEV_MODE=1` escape hatch exists only for
local development and generates an ephemeral key with a warning.

---

## 4. Authentication, Session & Access Control

### 4.1 Local credentials

`POST /api/auth/login` validates username + password:

1. `auth.Lockout.Check` short-circuits with 423 when the account is
   within the lockout window.
2. `store.GetUserByUsername` resolves the record; unknown users still
   increment the failure counter to blunt enumeration.
3. `auth.ComparePassword` verifies the Argon2id hash (constant-time).
4. On mismatch, `auth.StoreLockout.RecordFailure` persists the updated
   counter; a 5th failure sets `locked_until = now + 15m`.
5. On success, `auth.SessionStore.Issue` returns a 32-byte-random hex
   token; the session lives 8 hours.

### 4.2 Lockout persistence

`auth.StoreLockout` delegates to the `store.LoginAttempts` interface
so the 5-fail / 15-min window survives a process restart. The adapter
in `api/lockout_adapter.go` bridges the store layer to the minimal
surface `auth` needs without creating a cycle.

### 4.3 Session model

Sessions are held in process memory only (`auth.SessionStore`) — a
restart requires users to sign in again. Session tokens go in the
`Authorization: Bearer <token>` header. On every authenticated route:

- `RequireAuth` resolves the token into an `*auth.Session`.
- `RequireRoles(...)` (optional) enforces one or more roles.
- `RequirePermission(...)` (on analytics and selected endpoints) reads
  the admin-configured grants from the store and checks for the named
  permission.

### 4.4 Role- & permission-based access control

Every mutating endpoint:

1. Passes through `RequireAuth`.
2. Enforces roles (fixed) and/or permissions (dynamic) via middleware.
3. For object-level access, the handler re-checks ownership for
   owner-scoped resources (address book, saved filters).

Administrators may reconfigure grants at runtime:

| Endpoint | Purpose |
|---|---|
| `GET /api/admin/permissions` | list the catalog |
| `GET /api/admin/role-permissions` | list every role → permission grant |
| `PUT /api/admin/role-permissions/:role` | replace a role's grant set atomically |
| `GET /api/admin/users/:id/permissions` | list per-user overrides |
| `PUT /api/admin/users/:id/permissions` | replace per-user grants |

The middleware re-reads grants on every request; changes do not
require a session refresh.

### 4.5 At-rest encryption

Sensitive PII columns (`customers.identifier_enc`,
`customers.street_enc`, `address_book.street_enc`,
`orders.delivery_street_enc`) are written as vault envelopes and
decrypted in handlers that have the vault in scope. The audit log
stores a redacted view (booleans indicating presence) rather than
plaintext so the append-only log never contains PII.

---

## 5. Commerce Pipeline

### 5.1 Order creation

`POST /api/orders` accepts customer + delivery address + items +
tags + priority + total. Items may include `Backordered: true`; if
any such item is present at create time the server raises an
`out_of_stock` exception synchronously and writes an audit entry for
it. The delivery street is encrypted with the active vault key
before persistence; the response decrypts it back for the caller.

### 5.2 State transitions

`POST /api/orders/:id/transitions` runs the pure state machine:

1. `GetOrder` fetches the current state.
2. `Order.Transition(to, actor, reason, note, now)` validates the move
   and appends an in-memory `Event{From,To,At,Actor,Reason,Note}`.
3. `UpdateOrder` + `AppendOrderEvent` persist atomically.
4. `Audit.Log` records a structured before/after snapshot.

Refunds require a non-empty reason; the frontend enforces this with a
modal dialog, the backend re-enforces it with `ErrNoReason` → 400.

### 5.3 Exception queue

`GET /api/exceptions` scans every picking order + every order with
items on every call, applies the two detectors, persists fresh
exceptions (deduped by `(order_id, kind)`), audits each new flag, then
returns the current queue. The endpoint is idempotent — repeated calls
do not duplicate exceptions.

### 5.4 Out-of-stock inventory signal

`POST /api/orders/:id/inventory` flips a line item's `Backordered`
flag. The handler runs `DetectOutOfStock` inline and raises the
exception at mutation time, so operators see the flag without having
to refresh the exception list.

### 5.5 Jobs by address

`GET /api/orders/by-address` narrows on the indexed
`delivery_city`/`delivery_zip` columns, decrypts the street envelope,
and applies a case-insensitive substring match when a `street` query
parameter is supplied. The symmetric `customers/by-address` endpoint
uses the same decrypt-then-match pattern for the customer side.

---

## 6. Lab Workflow

### 6.1 Samples

`POST /api/samples` creates a sample in `sampling`. Transitions use
`POST /api/samples/:id/transitions` and the pure state machine in
`internal/lab/sample.go`. Rejection is a one-way terminal state from any
pre-reporting status.

### 6.2 Reports

`POST /api/samples/:id/report` issues a v1 report. The handler runs
the three controlled-workflow gates above, evaluates measurements
against the live reference-range set, and writes the report + (if
needed) advances the sample to `reported` in the same request. Audit
entries are produced for both the report creation and the sample
transition.

### 6.3 Corrections

`POST /api/reports/:id/correct` requires `expected_version`; a stale
value yields a 409 via `lab.ErrVersionConflict`. The store method
`ReplaceWithCorrection` is a single DB transaction: it inserts the new
row first (so the old row's FK target exists), then updates the prior
row's `status = superseded` and `superseded_by_id`.

### 6.4 Archive

`POST /api/reports/:id/archive` requires a non-empty `note` (audit
trail). Archive is one-way; a second attempt returns 409 via
`lab.ErrAlreadyArchived`. `GET /api/reports` omits archived rows;
`GET /api/reports/archived` returns only archived rows;
`GET /api/reports/search` intentionally returns both.

---

## 7. Dispatch

### 7.1 Service-area pin

`POST /api/dispatch/validate-pin` loads every configured region,
evaluates `geo.RegionForPoint`, and returns `{valid, region_id}` or
`{valid: false, reason}`. The SPA renders the polygons in an SVG canvas
sized to their bounding box so the check is visual + server-confirmed
(no tile server).

### 7.2 Fee quote

`POST /api/dispatch/fee-quote` resolves the destination's region (422
if outside all regions), computes `RouteTable.Distance` (returns
`miles` + `method`), and returns `{region_id, miles, method, fee_cents,
fee_usd}`. `method` is either `"route_table"` (preloaded entry hit) or
`"haversine"` (straight-line fallback).

### 7.3 Admin config

Regions and route-table rows are admin-editable via
`PUT /api/admin/service-regions` and `PUT /api/admin/route-table`. Both
calls trigger an in-process reload so subsequent fee quotes and
validations see the new data immediately.

---

## 8. Search & Filters

### 8.1 Global search

`GET /api/search?q=` returns a mixed-kind suggestion list: customers
via the tsvector index + ILIKE fallback, reports via tsvector over
title+narrative, and orders via a bounded recent-order window scored
with `internal/search`. The client applies its own fuzzy scorer on the
merged list to handle typos introduced between the server response and
the user's continuing keystrokes.

The SPA's `GlobalSearch` component also stores the last 20 distinct
searches per user in `localStorage` (keyed by user id), surfaced on
focus as a "Recent searches" drop-down.

### 8.2 Advanced filters

`POST /api/orders/query` accepts a full `filter.Filter` payload:

- `keyword`, `statuses`, `tags`, `priority`
- `start_date`, `end_date` (MM/DD/YYYY, inclusive, UTC)
- `min_price_usd`, `max_price_usd`
- `sort_by` (allowlisted per entity), `sort_desc`, `page`, `size`

`filter.Validate` rejects overly broad exports (a `size > 100` without
at least one narrowing clause returns `ErrTooBroad` → 400). The store
implements the query in both backends: memory iterates with the same
predicates; Postgres builds parameterized SQL using `tags && $N`, ILIKE
over the keyword, numeric range, and the allowlisted sort column.

### 8.3 Saved filters

`POST /api/saved-filters` stores a per-user filter with a canonical
key to dedupe identical filters. The key normalizes tag and status
order so `{tags: ["a","b"]}` and `{tags: ["b","a"]}` collapse.

---

## 9. Analytics

`/api/analytics/*` serve KPI aggregations used by the Analytics page:

- `orders/status-counts`, `orders/per-day`
- `samples/status-counts`
- `reports/abnormal-rate` (server iterates issued reports in window,
  counts measurements flagged non-normal)
- `exceptions/by-kind`
- `summary` (rolls the above into one payload)

All endpoints are gated by the `analytics.view` permission (not a
fixed role) so administrators can grant analyst-equivalent access to
other users at runtime.

The SPA renders the summary with two dependency-free SVG charts in
`components/BarChart.tsx` (bar + line). No external chart library is
pulled in — the bundle ships nothing the operator can't audit.

---

## 10. Audit Log

Every mutating endpoint calls `audit.Logger.Log(ctx, actor,
workstation, workstationTime, entity, entityID, action, reason,
before, after)`. The store writes to `audit_log`; an `AFTER UPDATE`
and `AFTER DELETE` trigger rejects any modification so the log is
append-only at the database layer too.

Entries include:

- `at` — server-assigned UTC timestamp.
- `workstation_time` — operator's local clock parsed from the
  `X-Workstation-Time` header (never substituted by the server).
- `workstation` — client identifier from the `X-Workstation` header
  (falls back to remote address).
- `actor_id`, `entity`, `entity_id`, `action`, `reason`.
- `before_json` / `after_json` — structured snapshots (redacted for
  sensitive fields).

Admin query: `GET /api/admin/audit?entity=&entity_id=`.

---

## 11. Structured Request Logging

`httpx.StructuredLogging` middleware emits one JSON line per request
with `request_id`, `method`, `path`, `status`, `latency_ms`,
`actor_id`, `role`, `workstation`, `remote_addr`, optional `error`,
and level (`info` / `warn` / `error`). A client-supplied
`X-Request-ID` is preserved; otherwise the middleware generates an
8-byte hex id and echoes it on the response for client-side
correlation.

---

## 12. Security Summary

| Area | Control |
|---|---|
| **Authentication** | Local username/password, Argon2id (m=64MiB, t=2, p=2), salt, constant-time compare, 10-char minimum |
| **Lockout** | 5 failures → 15-minute window; persisted in `login_attempts` so a restart does not clear it |
| **Authorization** | Static role gates + admin-configurable permission gates re-evaluated per request |
| **Object-level** | Address book / saved filters scoped by `owner_id`; tests cover cross-user attempts |
| **At-rest encryption** | AES-256-GCM, per-env versioned keys, rotation-additive envelopes, sensitive columns only |
| **Audit** | Append-only log with DB trigger; workstation-time captured separately from server time |
| **Transport headers** | `X-Content-Type-Options: nosniff`, `X-Frame-Options: DENY`, strict CSP, `Referrer-Policy: no-referrer` |
| **Sensitive errors** | `WriteError` classifies store errors; unclassified errors log server-side and return a generic 500 |
| **Filter abuse** | Overly broad exports (size > 100 with no narrowing clause) rejected with 400 |
| **Secrets bootstrap** | `ENC_KEYS` required; `OOPS_DEV_MODE=1` is the only path to an ephemeral key, with a loud warning |
| **Seed password** | `ADMIN_PASSWORD` env var required; no hardcoded default |

---

## 13. Performance & Scale

### 13.1 Target workload

The product is designed for a regional shop: O(10k) customers, O(100k)
orders, O(100k) samples over the life of the system, with a handful
of concurrent operators on the LAN. A single Postgres instance and a
single Go process comfortably cover this footprint.

### 13.2 Full-text search

Customer and report full-text searches use `tsvector` GIN indexes on
generated columns, so search is O(log n) on the result set rather
than O(n) scans. Search results are bounded at 50 by default, capped
at 500 for address lookups.

### 13.3 Advanced filter pagination

`size` is clamped to 500 by `filter.Validate`; the broad-export guard
requires narrowing clauses for anything above 100 rows. Every
`QueryOrders` call returns a `total` so the UI can render a progress
counter.

### 13.4 Exception detection at scale

`TestPickingTimeout_ScaleDoesNotDuplicate` exercises 50 stuck orders
across 3 exception-list calls and confirms the detector is idempotent
(exactly 50 exceptions, not 150). The deduplication key is
`(order_id, kind)` in both the memory store and Postgres
(`order_exceptions` unique index).

---

## 14. Accessibility & HIG

- Semantic HTML: `<nav>`, `<main>`, `<label htmlFor>` on every input,
  `<table>` with `<th>`.
- ARIA: `role="dialog" aria-modal="true"` on Modal, `role="alert"` on
  error banners, `role="status"` on success banners, `aria-label` on
  key interactive regions (e.g., the offline map SVG).
- Dynamic Type: the CSS uses `rem`/`em` units and lets the browser
  scale.
- Dark Mode: the palette is tuned on semantic CSS variables; swapping
  the root colors makes a dark theme trivial (not wired in this
  release).
- Keyboard: every form submits on Enter; Escape closes modals.

---

## 15. Testing Strategy

### 15.1 Backend

- **Unit tests** per domain package cover every state machine, error
  branch, and boundary (e.g., 30-min picking timeout boundary,
  4.02-mile fee rounding, Levenshtein limit, polygon on-segment).
- **HTTP handler tests** (`internal/api/*_test.go`) use
  `echo.ServeHTTP` against an in-memory store to exercise every
  endpoint's success + validation failure + 401 + 403 paths.
- **Faulty-store tests** wrap the memory store with a map of methods
  that return a sentinel error and verify each handler maps the
  error to a generic 500 without leaking.
- **Integration tests** (`internal/store/postgres_integration_test.go`
  and `postgres_full_integration_test.go`) exercise every Postgres
  method against a fresh schema behind the `INTEGRATION_DB` env var.
  CI uses a Postgres 14 service container.
- **Coverage** — audit/geo/lab/order/runtime at 100%; search 99%,
  httpx 96%, filter 95%, auth 94%, api 92%, store 90%, crypto 89%.

### 15.2 Frontend

- **Vitest + Testing Library** with 94 tests across the api client,
  every hook, every component, and every page including App routing,
  OrdersPage, OrderDetail, LabPage, Dispatch, Analytics, Customers,
  AddressBook, Admin, Login, Dashboard, CustomerDetail, and
  ReportDetail.
- `test:cov` emits a v8 coverage summary (~84% statements, ~81%
  branches).

### 15.3 CI

GitHub Actions runs three jobs on every push / PR:

- `backend` — `go vet`, `go build`, `go test -race -cover`.
- `backend-integration` — with a Postgres 14 service container and
  `INTEGRATION_DB` set.
- `frontend` — `npm ci`, `npm run typecheck`, `npm test`, `npm run build`.

---

## 16. Design decisions that remain open

- **Session persistence.** Sessions are process-local; a restart
  forces every user to sign in again. Acceptable for a single-box LAN
  deploy; a larger footprint would persist sessions in the DB.
- **No per-user analytics quota.** Analysts can run any bounded query
  at any frequency. A future rate-limit could move into the
  permission layer.
- **Attachment storage.** Reports currently carry only numeric
  measurements + narrative text. Image/PDF attachments (e.g.,
  pathology slides) are explicitly out of scope in this release.
- **Multi-tenancy.** The schema and handlers have no tenant dimension.
  A second organization would require schema migration and a
  `tenant_id` gate on every query.

---

*End of design document.*
