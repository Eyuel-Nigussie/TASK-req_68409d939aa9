# Unified Test Coverage + README Audit

Audit date: 2026-04-18
Auditor mode: strict, static-only, evidence-based
Project path: `/Users/mac/Eagle-Point Season 2/Task-24/repo/`
Project type (declared at [README.md:3](repo/README.md)): **fullstack** (Go/Echo backend + React/TypeScript frontend + PostgreSQL + Docker)

---

# PART 1: Test Coverage Audit

## Backend Endpoint Inventory

Extracted from [backend/internal/api/server.go](repo/backend/internal/api/server.go). Path prefixes already include `/api`; parameterised segments are normalised (`:id`, `:role`). **Total: 64 endpoints.**

### Public (no auth)
1. `POST /api/auth/login` — [server.go:127](repo/backend/internal/api/server.go)
2. `GET /api/health` — [server.go:128](repo/backend/internal/api/server.go)

### Auth (behind `RequireAuth` + rotation gate)
3. `POST /api/auth/logout` — [server.go:145](repo/backend/internal/api/server.go)
4. `GET /api/auth/whoami` — [server.go:146](repo/backend/internal/api/server.go)
5. `POST /api/auth/rotate-password` — [server.go:147](repo/backend/internal/api/server.go)

### Customers
6. `POST /api/customers` — [server.go:164](repo/backend/internal/api/server.go)
7. `GET /api/customers/:id` — [server.go:165](repo/backend/internal/api/server.go)
8. `GET /api/customers` — [server.go:166](repo/backend/internal/api/server.go)
9. `PATCH /api/customers/:id` — [server.go:167](repo/backend/internal/api/server.go)
10. `GET /api/customers/by-address` — [server.go:168](repo/backend/internal/api/server.go)

### Address Book
11. `GET /api/address-book` — [server.go:172](repo/backend/internal/api/server.go)
12. `POST /api/address-book` — [server.go:173](repo/backend/internal/api/server.go)
13. `DELETE /api/address-book/:id` — [server.go:174](repo/backend/internal/api/server.go)

### Orders
14. `POST /api/orders` — [server.go:177](repo/backend/internal/api/server.go)
15. `GET /api/orders` — [server.go:178](repo/backend/internal/api/server.go)
16. `POST /api/orders/query` — [server.go:179](repo/backend/internal/api/server.go)
17. `GET /api/orders/by-address` — [server.go:180](repo/backend/internal/api/server.go)
18. `GET /api/orders/:id` — [server.go:181](repo/backend/internal/api/server.go)
19. `POST /api/orders/:id/transitions` — [server.go:182](repo/backend/internal/api/server.go)
20. `GET /api/exceptions` — [server.go:183](repo/backend/internal/api/server.go)
21. `POST /api/orders/:id/out-of-stock/plan` — [server.go:184](repo/backend/internal/api/server.go)
22. `POST /api/orders/:id/inventory` — [server.go:185](repo/backend/internal/api/server.go)

### Samples
23. `POST /api/samples` — [server.go:188](repo/backend/internal/api/server.go)
24. `POST /api/samples/:id/transitions` — [server.go:189](repo/backend/internal/api/server.go)
25. `GET /api/samples/:id` — [server.go:190](repo/backend/internal/api/server.go)
26. `GET /api/samples/:id/test-items` — [server.go:191](repo/backend/internal/api/server.go)
27. `GET /api/samples` — [server.go:192](repo/backend/internal/api/server.go)

### Reports
28. `POST /api/samples/:id/report` — [server.go:195](repo/backend/internal/api/server.go)
29. `POST /api/reports/:id/correct` — [server.go:196](repo/backend/internal/api/server.go)
30. `POST /api/reports/:id/archive` — [server.go:197](repo/backend/internal/api/server.go)
31. `GET /api/reports` — [server.go:198](repo/backend/internal/api/server.go)
32. `GET /api/reports/archived` — [server.go:199](repo/backend/internal/api/server.go)
33. `GET /api/reports/search` — [server.go:200](repo/backend/internal/api/server.go)
34. `GET /api/reports/:id` — [server.go:201](repo/backend/internal/api/server.go)

### Dispatch
35. `POST /api/dispatch/validate-pin` — [server.go:206](repo/backend/internal/api/server.go)
36. `POST /api/dispatch/fee-quote` — [server.go:207](repo/backend/internal/api/server.go)
37. `GET /api/dispatch/regions` — [server.go:208](repo/backend/internal/api/server.go)
38. `GET /api/dispatch/map-config` — [server.go:209](repo/backend/internal/api/server.go)

### Saved Filters
39. `POST /api/saved-filters` — [server.go:212](repo/backend/internal/api/server.go)
40. `GET /api/saved-filters` — [server.go:213](repo/backend/internal/api/server.go)
41. `DELETE /api/saved-filters/:id` — [server.go:214](repo/backend/internal/api/server.go)

### Global Search
42. `GET /api/search` — [server.go:217](repo/backend/internal/api/server.go)

### Admin
43. `POST /api/admin/users` — [server.go:222](repo/backend/internal/api/server.go)
44. `GET /api/admin/users` — [server.go:223](repo/backend/internal/api/server.go)
45. `PATCH /api/admin/users/:id` — [server.go:224](repo/backend/internal/api/server.go)
46. `GET /api/admin/audit` — [server.go:225](repo/backend/internal/api/server.go)
47. `PUT /api/admin/service-regions` — [server.go:226](repo/backend/internal/api/server.go)
48. `GET /api/admin/reference-ranges` — [server.go:227](repo/backend/internal/api/server.go)
49. `PUT /api/admin/reference-ranges` — [server.go:228](repo/backend/internal/api/server.go)
50. `GET /api/admin/route-table` — [server.go:229](repo/backend/internal/api/server.go)
51. `PUT /api/admin/route-table` — [server.go:230](repo/backend/internal/api/server.go)
52. `GET /api/admin/permissions` — [server.go:231](repo/backend/internal/api/server.go)
53. `GET /api/admin/role-permissions` — [server.go:232](repo/backend/internal/api/server.go)
54. `PUT /api/admin/role-permissions/:role` — [server.go:233](repo/backend/internal/api/server.go)
55. `GET /api/admin/users/:id/permissions` — [server.go:234](repo/backend/internal/api/server.go)
56. `PUT /api/admin/users/:id/permissions` — [server.go:235](repo/backend/internal/api/server.go)
57. `PUT /api/admin/map-config` — [server.go:236](repo/backend/internal/api/server.go)

### Analytics
58. `GET /api/analytics/orders/status-counts` — [server.go:239](repo/backend/internal/api/server.go)
59. `GET /api/analytics/orders/per-day` — [server.go:240](repo/backend/internal/api/server.go)
60. `GET /api/analytics/samples/status-counts` — [server.go:241](repo/backend/internal/api/server.go)
61. `GET /api/analytics/reports/abnormal-rate` — [server.go:242](repo/backend/internal/api/server.go)
62. `GET /api/analytics/exceptions/by-kind` — [server.go:243](repo/backend/internal/api/server.go)
63. `GET /api/analytics/summary` — [server.go:244](repo/backend/internal/api/server.go)

### Exports
64. `POST /api/exports/orders.csv` — [server.go:248](repo/backend/internal/api/server.go)

---

## API Test Mapping Table

All backend API tests route through Echo's real mux via the shared rig at [api_test.go:25-75](repo/backend/internal/api/api_test.go): `srv := New(mem, vault, clk); srv.Register(e); e.ServeHTTP(rec, req)`. The request traverses the real middleware chain (`RequireAuth` → `RequirePasswordRotation` → `RequirePermission`) → real handler → real `store.Memory` (a production-supported persistence implementation, not a mock — see §5). No transport or handler layer is substituted.

| # | Endpoint | Covered | Test type | Test file(s) | Evidence |
|---|---|---|---|---|---|
| 1 | `POST /api/auth/login` | yes | True No-Mock HTTP | api_test.go, full_coverage_test.go | [api_test.go:129-167](repo/backend/internal/api/api_test.go), [full_coverage_test.go:60-90](repo/backend/internal/api/full_coverage_test.go) |
| 2 | `GET /api/health` | **no** | — | — | Not found in any `*_test.go` under `backend/internal/api/` |
| 3 | `POST /api/auth/logout` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go:22-46](repo/backend/internal/api/full_coverage_test.go) |
| 4 | `GET /api/auth/whoami` | yes | True No-Mock HTTP | full_coverage_test.go, security_test.go | [full_coverage_test.go:48-58](repo/backend/internal/api/full_coverage_test.go) |
| 5 | `POST /api/auth/rotate-password` | yes | True No-Mock HTTP | full_coverage_test.go | `TestRotatePassword_GateBlocksThenClears` [full_coverage_test.go:92-187](repo/backend/internal/api/full_coverage_test.go) |
| 6 | `POST /api/customers` | yes | True No-Mock HTTP | api_test.go, security_test.go, matrix_test.go | [api_test.go:172-200](repo/backend/internal/api/api_test.go) |
| 7 | `GET /api/customers/:id` | yes | True No-Mock HTTP | full_coverage_test.go, matrix_test.go | [full_coverage_test.go:82-89](repo/backend/internal/api/full_coverage_test.go) |
| 8 | `GET /api/customers` (search) | yes | True No-Mock HTTP | api_test.go | [api_test.go:196-199](repo/backend/internal/api/api_test.go) |
| 9 | `PATCH /api/customers/:id` | yes | True No-Mock HTTP | full_coverage_test.go, security_test.go | [full_coverage_test.go:91-120](repo/backend/internal/api/full_coverage_test.go) |
| 10 | `GET /api/customers/by-address` | yes | True No-Mock HTTP | api_test.go, full_coverage_test.go | [api_test.go:202-209](repo/backend/internal/api/api_test.go) |
| 11 | `GET /api/address-book` | yes | True No-Mock HTTP | security_test.go | [security_test.go:235-257](repo/backend/internal/api/security_test.go) |
| 12 | `POST /api/address-book` | yes | True No-Mock HTTP | security_test.go | [security_test.go:240-246](repo/backend/internal/api/security_test.go) |
| 13 | `DELETE /api/address-book/:id` | yes | True No-Mock HTTP | security_test.go | [security_test.go:253](repo/backend/internal/api/security_test.go) |
| 14 | `POST /api/orders` | yes | True No-Mock HTTP | api_test.go, matrix_test.go, new_features_test.go | [api_test.go:213-231](repo/backend/internal/api/api_test.go) |
| 15 | `GET /api/orders` | yes | True No-Mock HTTP | security_test.go, matrix_test.go, full_coverage_test.go | [security_test.go:33](repo/backend/internal/api/security_test.go) |
| 16 | `POST /api/orders/query` | yes | True No-Mock HTTP | api_test.go | `TestQueryOrders_PageCapRejected`, `TestQueryOrders_BadDateRejected` [api_test.go:375-401](repo/backend/internal/api/api_test.go) |
| 17 | `GET /api/orders/by-address` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 18 | `GET /api/orders/:id` | yes | True No-Mock HTTP | security_test.go, matrix_test.go | [security_test.go:396-399](repo/backend/internal/api/security_test.go) |
| 19 | `POST /api/orders/:id/transitions` | yes | True No-Mock HTTP | api_test.go, security_test.go | [api_test.go:222-230](repo/backend/internal/api/api_test.go) |
| 20 | `GET /api/exceptions` | yes | True No-Mock HTTP | extra_coverage_test.go | `TestListExceptions_SweepThrottled` [extra_coverage_test.go:203-258](repo/backend/internal/api/extra_coverage_test.go) |
| 21 | `POST /api/orders/:id/out-of-stock/plan` | yes | True No-Mock HTTP | security_test.go | [security_test.go:125](repo/backend/internal/api/security_test.go) |
| 22 | `POST /api/orders/:id/inventory` | yes | True No-Mock HTTP | security_test.go | [security_test.go:126](repo/backend/internal/api/security_test.go) |
| 23 | `POST /api/samples` | yes | True No-Mock HTTP | api_test.go, new_features_test.go | [new_features_test.go:12-59](repo/backend/internal/api/new_features_test.go) |
| 24 | `POST /api/samples/:id/transitions` | yes | True No-Mock HTTP | api_test.go, security_test.go | [api_test.go:245-249](repo/backend/internal/api/api_test.go) |
| 25 | `GET /api/samples/:id` | yes | True No-Mock HTTP | matrix_test.go | [matrix_test.go:158-160](repo/backend/internal/api/matrix_test.go) |
| 26 | `GET /api/samples/:id/test-items` | yes | True No-Mock HTTP | new_features_test.go | [new_features_test.go:30](repo/backend/internal/api/new_features_test.go) |
| 27 | `GET /api/samples` | yes | True No-Mock HTTP | security_test.go, matrix_test.go | [security_test.go:34](repo/backend/internal/api/security_test.go) |
| 28 | `POST /api/samples/:id/report` | yes | True No-Mock HTTP | api_test.go, coverage_test.go | `TestSampleGate_RejectsReportOnWrongStatus` [coverage_test.go:17-60](repo/backend/internal/api/coverage_test.go) |
| 29 | `POST /api/reports/:id/correct` | yes | True No-Mock HTTP | api_test.go, security_test.go | [api_test.go:262-297](repo/backend/internal/api/api_test.go) |
| 30 | `POST /api/reports/:id/archive` | yes | True No-Mock HTTP | full_coverage_test.go, security_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 31 | `GET /api/reports` | yes | True No-Mock HTTP | security_test.go, matrix_test.go | [security_test.go](repo/backend/internal/api/security_test.go) |
| 32 | `GET /api/reports/archived` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 33 | `GET /api/reports/search` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 34 | `GET /api/reports/:id` | yes | True No-Mock HTTP | matrix_test.go, security_test.go | [matrix_test.go:159](repo/backend/internal/api/matrix_test.go) |
| 35 | `POST /api/dispatch/validate-pin` | yes | True No-Mock HTTP | api_test.go | [api_test.go:301-326](repo/backend/internal/api/api_test.go) |
| 36 | `POST /api/dispatch/fee-quote` | yes | True No-Mock HTTP | security_test.go, full_coverage_test.go | [security_test.go:40](repo/backend/internal/api/security_test.go) |
| 37 | `GET /api/dispatch/regions` | yes | True No-Mock HTTP | matrix_test.go | [matrix_test.go:75-77](repo/backend/internal/api/matrix_test.go) |
| 38 | `GET /api/dispatch/map-config` | yes | True No-Mock HTTP | new_features_test.go | [new_features_test.go:76](repo/backend/internal/api/new_features_test.go) |
| 39 | `POST /api/saved-filters` | yes | True No-Mock HTTP | api_test.go, security_test.go | `TestSavedFilters_Validation` [api_test.go:342-370](repo/backend/internal/api/api_test.go) |
| 40 | `GET /api/saved-filters` | yes | True No-Mock HTTP | security_test.go | [security_test.go:215-218](repo/backend/internal/api/security_test.go) |
| 41 | `DELETE /api/saved-filters/:id` | yes | True No-Mock HTTP | security_test.go | [security_test.go:221-223](repo/backend/internal/api/security_test.go) |
| 42 | `GET /api/search` | yes | True No-Mock HTTP | security_test.go, extra_coverage_test.go | [security_test.go:44](repo/backend/internal/api/security_test.go) |
| 43 | `POST /api/admin/users` | yes | True No-Mock HTTP | full_coverage_test.go, security_test.go | [full_coverage_test.go:594](repo/backend/internal/api/full_coverage_test.go) |
| 44 | `GET /api/admin/users` | yes | True No-Mock HTTP | api_test.go, security_test.go, matrix_test.go | [api_test.go:334](repo/backend/internal/api/api_test.go) |
| 45 | `PATCH /api/admin/users/:id` | yes | True No-Mock HTTP | security_test.go | [security_test.go:356](repo/backend/internal/api/security_test.go) |
| 46 | `GET /api/admin/audit` | yes | True No-Mock HTTP | security_test.go | [security_test.go:71](repo/backend/internal/api/security_test.go) |
| 47 | `PUT /api/admin/service-regions` | yes | True No-Mock HTTP | api_test.go, security_test.go | [api_test.go:305-312](repo/backend/internal/api/api_test.go) |
| 48 | `GET /api/admin/reference-ranges` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 49 | `PUT /api/admin/reference-ranges` | yes | True No-Mock HTTP | security_test.go | [security_test.go:326](repo/backend/internal/api/security_test.go) |
| 50 | `GET /api/admin/route-table` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 51 | `PUT /api/admin/route-table` | yes | True No-Mock HTTP | security_test.go | [security_test.go:337](repo/backend/internal/api/security_test.go) |
| 52 | `GET /api/admin/permissions` | yes | True No-Mock HTTP | permissions_test.go | [permissions_test.go](repo/backend/internal/api/permissions_test.go) |
| 53 | `GET /api/admin/role-permissions` | yes | True No-Mock HTTP | permissions_test.go | [permissions_test.go](repo/backend/internal/api/permissions_test.go) |
| 54 | `PUT /api/admin/role-permissions/:role` | yes | True No-Mock HTTP | permissions_test.go | [permissions_test.go](repo/backend/internal/api/permissions_test.go) |
| 55 | `GET /api/admin/users/:id/permissions` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go:769](repo/backend/internal/api/full_coverage_test.go) |
| 56 | `PUT /api/admin/users/:id/permissions` | yes | True No-Mock HTTP | permissions_test.go, bind_errors_test.go | [permissions_test.go:113](repo/backend/internal/api/permissions_test.go) |
| 57 | `PUT /api/admin/map-config` | yes | True No-Mock HTTP | new_features_test.go | [new_features_test.go:69-118](repo/backend/internal/api/new_features_test.go) |
| 58 | `GET /api/analytics/orders/status-counts` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 59 | `GET /api/analytics/orders/per-day` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 60 | `GET /api/analytics/samples/status-counts` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 61 | `GET /api/analytics/reports/abnormal-rate` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 62 | `GET /api/analytics/exceptions/by-kind` | yes | True No-Mock HTTP | full_coverage_test.go | [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go) |
| 63 | `GET /api/analytics/summary` | yes | True No-Mock HTTP | security_test.go, full_coverage_test.go | [security_test.go:189](repo/backend/internal/api/security_test.go) |
| 64 | `POST /api/exports/orders.csv` | yes | True No-Mock HTTP | new_features_test.go | [new_features_test.go:122-181](repo/backend/internal/api/new_features_test.go) |

---

## API Test Classification

- **True No-Mock HTTP tests**: every test file under [backend/internal/api/](repo/backend/internal/api/) that shares the `setup(t)` rig (10 files: `api_test.go`, `bind_errors_test.go`, `coverage_test.go`, `extra_coverage_test.go`, `full_coverage_test.go`, `matrix_test.go`, `new_features_test.go`, `permissions_test.go`, `security_test.go`). All requests pass through `echo.New()` + `srv.Register(e)` + `e.ServeHTTP(rec, req)` at [api_test.go:49-69](repo/backend/internal/api/api_test.go).
- **HTTP with targeted fault injection** (one file): [error_paths_test.go](repo/backend/internal/api/error_paths_test.go) wraps `store.Memory` with `faultyStore` to return `errSentinel` from specific store methods. This is fault-injection of the persistence layer — controllers, middleware, auth, and business logic all remain real. Every endpoint touched here also has a True-No-Mock test in another file, so no endpoint relies solely on fault-injection tests.
- **Non-HTTP unit/integration tests**: pure-Go tests for domain packages — [order/workflow_test.go](repo/backend/internal/order/workflow_test.go), [lab/lab_test.go](repo/backend/internal/lab/lab_test.go), [filter/filter_test.go](repo/backend/internal/filter/filter_test.go), [geo/polygon_test.go](repo/backend/internal/geo/polygon_test.go), [geo/distance_test.go](repo/backend/internal/geo/distance_test.go), [search/fuzzy_test.go](repo/backend/internal/search/fuzzy_test.go), [auth/password_test.go](repo/backend/internal/auth/password_test.go), [auth/lockout_test.go](repo/backend/internal/auth/lockout_test.go), [auth/store_lockout_test.go](repo/backend/internal/auth/store_lockout_test.go), [auth/session_test.go](repo/backend/internal/auth/session_test.go), [crypto/crypto_test.go](repo/backend/internal/crypto/crypto_test.go), [audit/audit_test.go](repo/backend/internal/audit/audit_test.go), [runtime/runtime_test.go](repo/backend/internal/runtime/runtime_test.go), [httpx/middleware_test.go](repo/backend/internal/httpx/middleware_test.go), [httpx/logging_test.go](repo/backend/internal/httpx/logging_test.go), [store/memory_*_test.go](repo/backend/internal/store/), [store/postgres_integration_test.go](repo/backend/internal/store/postgres_integration_test.go) (gated on `INTEGRATION_DB` — real Postgres behavior-parity).
- **Frontend tests**: all use `vi.mock("../api/client")` to stub the HTTP client (§7). These are **component unit tests**, not API tests.

---

## Mock Detection

### Backend
- Repo-wide grep for `jest.mock`, `vi.mock`, `sinon`, or `mockery`: **zero hits under `backend/`**.
- Dependency-injection substitutions actually present:
  1. `store.Memory` — a production-supported alternative to Postgres wired in [cmd/server/main.go:34-49](repo/backend/cmd/server/main.go) when `DATABASE_URL` is unset. The Prompt's Quickstart path relies on it. Using it in tests is **integration with an in-process store**, not mocking.
  2. `faultyStore` at [error_paths_test.go:29-40](repo/backend/internal/api/error_paths_test.go) — deliberate fault injection on a named allowlist of methods, used only to exercise the generic-500 sanitizer path. Does not mock the handler or middleware.
  3. `faultyAudit` at [audit/audit_test.go:73-84](repo/backend/internal/audit/audit_test.go) — purpose-specific stub to verify the audit-drop error sink.
- **No mock of the transport layer, Echo middleware, handlers, session store, vault, or domain services.** Auth/role/permission gates always run.

### Frontend
- `vi.mock(...)` appears in **20 test files, 105 total occurrences** (mostly targeting `../api/client` so component logic can be tested in isolation). Representative hits:
  - [frontend/src/components/OfflineMap.test.tsx](repo/frontend/src/components/OfflineMap.test.tsx): mocks `listRegions`, `getMapConfig`, `validatePin`.
  - [frontend/src/pages/Lab.test.tsx](repo/frontend/src/pages/Lab.test.tsx): mocks `listSamples`, `createSample`, `transitionSample`, `createReport`, `listTestItems`.
  - [frontend/src/hooks/useAuth.test.tsx](repo/frontend/src/hooks/useAuth.test.tsx): mocks `api.login`, `api.whoami`.
- Purpose: classic **frontend unit testing** — verify UI logic without a live backend. These are NOT API tests and do NOT contribute to backend API coverage numbers.
- No true FE↔BE E2E test (e.g., Playwright/Cypress) was found.

---

## Coverage Summary

| Metric | Value |
|---|---|
| Total endpoints | **64** |
| Endpoints with HTTP tests | **63** (only `GET /api/health` is untested) |
| Endpoints with True No-Mock HTTP tests | **63** |
| **HTTP coverage** | **98.4%** (63/64) |
| **True API coverage (no-mock)** | **98.4%** (63/64) |
| Uncovered endpoints | `GET /api/health` — trivial liveness probe; covered *operationally* by `start.sh:213-230` curl poll |

---

## Unit Test Analysis

### Backend Unit Tests

Files and areas covered (every file lives under `backend/internal/<pkg>/*_test.go` and is exercised by `go test ./...`):

| Module | Test file | Coverage scope |
|---|---|---|
| Controllers / handlers | [api_test.go](repo/backend/internal/api/api_test.go), [coverage_test.go](repo/backend/internal/api/coverage_test.go), [extra_coverage_test.go](repo/backend/internal/api/extra_coverage_test.go), [full_coverage_test.go](repo/backend/internal/api/full_coverage_test.go), [new_features_test.go](repo/backend/internal/api/new_features_test.go), [bind_errors_test.go](repo/backend/internal/api/bind_errors_test.go) | Happy paths, 4xx validation, 409 conflicts, 423 lockout, controlled workflow gates |
| Authorization matrix | [matrix_test.go](repo/backend/internal/api/matrix_test.go), [security_test.go](repo/backend/internal/api/security_test.go), [permissions_test.go](repo/backend/internal/api/permissions_test.go) | 401/403 matrix, role deny matrix, IDOR, cross-user isolation, password rotation gate, audit-write-on-mutation |
| Error / sanitizer paths | [error_paths_test.go](repo/backend/internal/api/error_paths_test.go) | Unknown-error → 500 generic message, no leak |
| Password policy + hash | [auth/password.go](repo/backend/internal/auth/password.go) via [auth/password_test.go](repo/backend/internal/auth/password_test.go) | Blank / too short / repeat / single-class / common-list / happy path + Argon2 round-trip |
| Session store | [auth/session_test.go](repo/backend/internal/auth/session_test.go) | Issue, lookup, expire, revoke |
| Lockout (persistent) | [auth/store_lockout_test.go](repo/backend/internal/auth/store_lockout_test.go) | 5-fail → locked, 15-min reset |
| Lockout (in-mem) | [auth/lockout_test.go](repo/backend/internal/auth/lockout_test.go) | Same contract, stand-alone |
| Crypto | [crypto/crypto_test.go](repo/backend/internal/crypto/crypto_test.go), [crypto/extra_coverage_test.go](repo/backend/internal/crypto/extra_coverage_test.go) | Encrypt/decrypt, key rotation, wrong version, malformed envelope |
| Runtime bootstrap | [runtime/runtime_test.go](repo/backend/internal/runtime/runtime_test.go) | BuildVault env / dev-mode (non-determinism asserted), SeedDeployment, SeedDemoUsers forces MustRotatePassword |
| Order workflow | [order/workflow_test.go](repo/backend/internal/order/workflow_test.go) | State machine, refund reason required, OOS, 30-min picking timeout, split plan |
| Lab domain | [lab/lab_test.go](repo/backend/internal/lab/lab_test.go), [lab/extra_coverage_test.go](repo/backend/internal/lab/extra_coverage_test.go) | Correct / ReportArchive / Evaluate / RangeSet / FlagUncategorized |
| Filter | [filter/filter_test.go](repo/backend/internal/filter/filter_test.go) | Validate, narrowing, page cap, date parse |
| Geo | [geo/polygon_test.go](repo/backend/internal/geo/polygon_test.go), [geo/distance_test.go](repo/backend/internal/geo/distance_test.go) | Contains, edge, RouteTable lookup, Haversine, FeeCents |
| Search | [search/fuzzy_test.go](repo/backend/internal/search/fuzzy_test.go) | Ranker threshold, top-N, typo tolerance |
| httpx middleware | [httpx/middleware_test.go](repo/backend/internal/httpx/middleware_test.go) | RequireAuth, RequireRoles, RequirePermission |
| Logging | [httpx/logging_test.go](repo/backend/internal/httpx/logging_test.go), [httpx/logging_extra_test.go](repo/backend/internal/httpx/logging_extra_test.go) | JSON-line structure, status, error routing |
| Audit | [audit/audit_test.go](repo/backend/internal/audit/audit_test.go) | Structured entry, nil snapshot, zero workstation-time, error sink |
| Store (memory) | [store/memory_test.go](repo/backend/internal/store/memory_test.go), [store/memory_full_test.go](repo/backend/internal/store/memory_full_test.go), [store/memory_remaining_test.go](repo/backend/internal/store/memory_remaining_test.go), [store/memory_new_features_test.go](repo/backend/internal/store/memory_new_features_test.go) | CRUD, filter query, pagination, FTS emulation |
| Store (postgres, integration) | [store/postgres_integration_test.go](repo/backend/internal/store/postgres_integration_test.go), [store/postgres_full_integration_test.go](repo/backend/internal/store/postgres_full_integration_test.go) | Real Postgres parity: user round-trip, audit-log immutability, report correction transactional |

**Important backend modules NOT tested directly:**
- `cmd/server/main.go` — thin bootstrap shell; its logic is extracted into `runtime/runtime.go` which *is* tested. Acceptable.
- `cmd/keygen/main.go` — trivial hex emitter; `runtime.GenerateKeyLine` (the testable core) is covered by runtime_test.go.

No material backend modules are untested.

---

### Frontend Unit Tests — STRICT CHECK

#### Detection Rules Check
- **Identifiable test files:** YES — 20 `*.test.ts(x)` files across `src/api/`, `src/components/`, `src/hooks/`, `src/lib/`, and `src/pages/`.
- **Targets frontend logic/components:** YES — files under `components/` and `pages/` import and render the actual components.
- **Framework evident:** YES — `vitest` + `@testing-library/react` + `jsdom` at [frontend/package.json:27-31](repo/frontend/package.json); test runner at `"test": "vitest run"` ([package.json:11](repo/frontend/package.json)).
- **Imports / renders real frontend modules:** YES — e.g., [OfflineMap.test.tsx](repo/frontend/src/components/OfflineMap.test.tsx) imports `OfflineMap` and renders it; [Login.test.tsx](repo/frontend/src/pages/Login.test.tsx) imports `LoginPage`; [useAuth.test.tsx](repo/frontend/src/hooks/useAuth.test.tsx) imports the hook under test.

#### Frontend Test Files
- API: [src/api/client.test.ts](repo/frontend/src/api/client.test.ts)
- Hooks: [src/hooks/useAuth.test.tsx](repo/frontend/src/hooks/useAuth.test.tsx), [src/hooks/useRecentSearches.test.tsx](repo/frontend/src/hooks/useRecentSearches.test.tsx)
- Library: [src/lib/fuzzy.test.ts](repo/frontend/src/lib/fuzzy.test.ts)
- Components: [AdvancedFilters](repo/frontend/src/components/AdvancedFilters.test.tsx), [BarChart](repo/frontend/src/components/BarChart.test.tsx), [GlobalSearch](repo/frontend/src/components/GlobalSearch.test.tsx), [Modal](repo/frontend/src/components/Modal.test.tsx), [OfflineMap](repo/frontend/src/components/OfflineMap.test.tsx), [OrderTimeline](repo/frontend/src/components/OrderTimeline.test.tsx), [ReportWorkspace](repo/frontend/src/components/ReportWorkspace.test.tsx)
- Pages: [AddressBook](repo/frontend/src/pages/AddressBook.test.tsx), [Admin](repo/frontend/src/pages/Admin.test.tsx), [Analytics](repo/frontend/src/pages/Analytics.test.tsx), [App](repo/frontend/src/pages/App.test.tsx), [CustomerDetail](repo/frontend/src/pages/CustomerDetail.test.tsx), [Customers](repo/frontend/src/pages/Customers.test.tsx), [Dashboard](repo/frontend/src/pages/Dashboard.test.tsx), [Dispatch](repo/frontend/src/pages/Dispatch.test.tsx), [Lab](repo/frontend/src/pages/Lab.test.tsx), [Login](repo/frontend/src/pages/Login.test.tsx), [OrderDetail](repo/frontend/src/pages/OrderDetail.test.tsx), [Orders](repo/frontend/src/pages/Orders.test.tsx), [ReportDetail](repo/frontend/src/pages/ReportDetail.test.tsx)

**Frameworks/tools detected:** `vitest@^2.1.1`, `@testing-library/react@^16.0.1`, `@testing-library/user-event@^14.5.2`, `@testing-library/jest-dom@^6.5.0`, `jsdom@^25.0.0`, `@vitest/coverage-v8@^2.1.1`.

**Components/modules covered:** all 7 components, both hooks, the API client module, the fuzzy helper, and every one of the 13 pages.

**Important frontend components NOT tested:** None identified. Every file under `src/components/*.tsx`, `src/pages/*.tsx`, and `src/hooks/` has a sibling `.test.tsx(x)` file. The only untested code paths are trivial (e.g., `main.tsx` bootstrap).

#### Mandatory Verdict: **Frontend unit tests: PRESENT**

No CRITICAL GAP triggered.

---

### Cross-Layer Observation

Testing balance: **balanced, not backend-heavy**.
- Backend: 245+ Go test functions across ~39 test files, with both True-No-Mock HTTP and domain unit tests.
- Frontend: 20 test files across every major UI surface, using vitest + RTL.
- Integration: Postgres parity suite gated on `INTEGRATION_DB`, wired into `docker-compose.test.yml`.
- Not compensating for a missing layer — every layer has its own assertions.

---

## API Observability Check

Tests meet the observability bar:
- Every `r.do(t, METHOD, PATH, token, body)` call at [api_test.go:54-75](repo/backend/internal/api/api_test.go) explicitly names the method + path, attaches the Authorization bearer + `X-Workstation` header, and encodes the request body to JSON. Response `Code` and parsed JSON `body` are both returned and asserted.
- Assertions inspect specific response fields (e.g., `body["id"]`, `body["TestItems"]`, `body["must_rotate_password"]`, `body["valid"]`, `body["region_id"]`, `body["token"]`).
- The `setup(t)` rig seeds four roles (admin1, tech1, desk1, dispatch1) and `mkUser` can add `analyst1` on demand, so role-specific asserts are explicit per test.

Not flagged as weak.

---

## Test Quality & Sufficiency

Evaluated against the strict rubric:
- **Success paths:** every entity has a create/transition happy-path test (e.g., [api_test.go:213-231](repo/backend/internal/api/api_test.go) for orders, [api_test.go:235-297](repo/backend/internal/api/api_test.go) for reports).
- **Failure cases:** bad body 400 ([bind_errors_test.go](repo/backend/internal/api/bind_errors_test.go)), missing required field 400, bad date 400 ([api_test.go:391-401](repo/backend/internal/api/api_test.go)), wrong expected_version 409 ([api_test.go:262-272](repo/backend/internal/api/api_test.go)), stale/superseded report 409 ([coverage_test.go:17-60](repo/backend/internal/api/coverage_test.go)), unauth 401 matrix ([security_test.go:23-58](repo/backend/internal/api/security_test.go)), role 403 matrix ([matrix_test.go:19-109](repo/backend/internal/api/matrix_test.go)), write-deny matrix ([security_test.go:102-173](repo/backend/internal/api/security_test.go)), lockout 423 ([api_test.go:137-167](repo/backend/internal/api/api_test.go)), CSV broad filter 400, CSV permission 403 ([new_features_test.go:162-181](repo/backend/internal/api/new_features_test.go)).
- **Edge cases:** unicode password rejection, repeated-char password rejection, common-password rejection ([auth/password_test.go:14-33](repo/backend/internal/auth/password_test.go)); zero workstation-time omitted ([audit/audit_test.go:55-67](repo/backend/internal/audit/audit_test.go)); map-config data URI acceptance + non-http scheme rejection ([new_features_test.go:82-102](repo/backend/internal/api/new_features_test.go)); pagination page cap ([api_test.go:375-386](repo/backend/internal/api/api_test.go)); disabled-account timing pad + lockout participation ([full_coverage_test.go:71-90](repo/backend/internal/api/full_coverage_test.go)).
- **Validation:** date format, page/size caps, narrowing clause required at saved-filter create ([api_test.go:353-369](repo/backend/internal/api/api_test.go)), polygon shape ([security_test.go:388-395](repo/backend/internal/api/security_test.go)).
- **Auth / permissions:** 401 matrix, 403 matrix, cross-user isolation on address-book + saved-filters ([security_test.go:200-257](repo/backend/internal/api/security_test.go)), password-rotation gate [full_coverage_test.go:92-187](repo/backend/internal/api/full_coverage_test.go).
- **Integration boundaries:** audit immutability verified at the DB trigger level ([postgres_integration_test.go:62-74](repo/backend/internal/store/postgres_integration_test.go)); transactional report correction ([postgres_integration_test.go:76-120](repo/backend/internal/store/postgres_integration_test.go)).
- **Real assertions**: span status code, body shape, specific field values, header presence (CSV content-type), side-effects (audit rows listed and counted). No superficial `expect(true).toBe(true)`-style tests.
- **Depth**: many tests are table-driven with per-case identifiers, aiding diagnosis.
- **Meaningful not autogenerated**: rationale comments throughout tie each test to a specific audit finding or product invariant.

### `run_tests.sh` verification
[run_tests.sh](repo/run_tests.sh) uses `docker compose -f docker-compose.test.yml` to run the `backend-test` and `frontend-test` containers. **No local `npm install`, `go install`, `pip`, `apt-get`, or manual DB setup.** Cleanup via trap at [run_tests.sh:24-27](repo/run_tests.sh). **OK — Docker-based.**

---

## End-to-End Expectations

- No dedicated FE↔BE E2E harness (no Playwright, Cypress, Selenium files found in `frontend/` or root).
- Compensating coverage: backend True-No-Mock HTTP at 98.4%, full-stack path validated in a single process (Echo + real middleware + real handler + real Memory store). Frontend is unit-tested at component granularity with mocked API client; the client module itself has a dedicated test ([api/client.test.ts](repo/frontend/src/api/client.test.ts)).
- **Verdict:** strong per-layer coverage compensates for the absence of a true E2E suite. This is acceptable for a fullstack but is a noted gap.

---

## Evidence Rule

Every conclusion above cites `file:line` or `file:function` — see inline links.

---

## Tests Check — summary

- Backend API tests go through the real HTTP layer (httptest + echo mux). ✓
- No mocking of controllers, middleware, auth, sessions, vault, or business services. ✓
- `store.Memory` is a production-supported implementation, not a mock. ✓
- Frontend unit tests present and comprehensive (vitest + RTL). ✓
- `run_tests.sh` is fully Docker-based. ✓
- Postgres integration parity tests exist (gated on `INTEGRATION_DB`). ✓
- True E2E (FE↔BE) NOT present. ✗ (compensated, not a critical gap)
- `GET /api/health` not tested. ✗ (trivial liveness probe, low risk)

---

## Test Coverage Score

**Score: 90 / 100**

### Score Rationale
- +30 HTTP endpoint coverage (98.4% — 63/64)
- +20 True no-mock HTTP execution (real middleware, real handlers, real Memory store; no transport/controller mocks)
- +15 Depth of failure, edge, auth, and validation coverage (role matrix, 401/403/404/409/422/423 all asserted; cross-user isolation, timing pad, rotation gate covered)
- +15 Complete backend unit-test coverage across every domain package + bootstrap
- +10 Frontend unit tests present across all components/pages/hooks with vitest + RTL
- −5 No true FE↔BE E2E test
- −3 `GET /api/health` not directly tested (start.sh curl is operational, not a unit/integration test)
- −2 Frontend tests universally mock the backend — legitimate for component tests but leaves the React side's integration with a real server unverified

---

## Key Gaps

1. **No true end-to-end (FE↔BE) test.** Recommend adding one Playwright scenario: log in with a seeded role, perform a cross-page flow (e.g., create customer → create order → validate dispatch pin), and assert the final DB state.
2. **`GET /api/health` untested.** Add a one-liner: `rec, _ := r.do(t, "GET", "/api/health", "", nil); if rec.Code != 200 { t.Fatal(...) }`.
3. **Frontend tests all mock the API client.** Acceptable for component logic, but consider one contract test per page (using MSW against a stable mock server or the real backend in a docker-compose override) to catch breaking schema changes.

---

## Confidence & Assumptions

- **Confidence: high**. All conclusions draw on `file:line` evidence; no assumption was made about runtime.
- **Assumption:** `store.Memory` is not a mock because (a) it implements the full `store.Store` interface, (b) [cmd/server/main.go:34-49](repo/backend/cmd/server/main.go) wires it as the production persistence when `DATABASE_URL` is unset, and (c) its behavior is validated against the Postgres implementation via shared tests (`memory_*_test.go` + `postgres_integration_test.go`).
- **Assumption:** `faultyStore` / `faultyAudit` are fault-injection, not transport/controller mocks. They wrap named persistence methods only; the HTTP layer and handlers are unchanged.
- Tests are assumed to be runnable inside the dockerised test suite; this was not executed (static-only).

---

# PART 2: README Audit

## Project Type
**fullstack** — explicitly declared at [README.md:3](repo/README.md) ("A full-stack operations portal…") and reiterated in the architecture section at [README.md:14-23](repo/README.md). **Type gate: PASS.**

## README Location
[README.md](repo/README.md) exists at `repo/README.md`. **Location gate: PASS.**

## Hard Gates

### Formatting
- Clean Markdown headings (`#`, `##`), fenced code blocks (bash + text), ASCII tree, aligned pipe table.
- Renders cleanly on GitHub/Gitea.
- **PASS.**

### Startup Instructions (fullstack → Docker Compose required)
- Primary path: `./start.sh` ([README.md:69-72](repo/README.md)).
- Raw alternative: `cp .env.example .env && docker compose up --build -d` ([README.md:74-79](repo/README.md)).
- Both paths are Docker-Compose-based; no host toolchain required ([README.md:59-60](repo/README.md): "No Go, Node.js, or PostgreSQL installation is required on the host").
- **PASS.**

### Access Method
- Frontend URL: `http://localhost:3000` ([README.md:83](repo/README.md)).
- Backend API URL: `http://localhost:3000/api` or `http://localhost:8080/api` ([README.md:84-85](repo/README.md)).
- Health probe: `http://localhost:8080/api/health` ([README.md:86](repo/README.md)).
- API reference pointer: [`docs/apispec.md`](docs/apispec.md) ([README.md:87](repo/README.md)).
- **PASS.**

### Verification Method
- Implicit (log in with a seeded credential at `http://localhost:3000`, verify role-based access) — covered by the Seeded Credentials table ([README.md:134-140](repo/README.md)).
- Health probe documented ([README.md:86](repo/README.md)) for quick curl check.
- Testing section ([README.md:99-123](repo/README.md)) provides a single-command verification.
- **No explicit step-by-step curl / UI flow narrative** (e.g., "sign in as admin, visit /admin, confirm user table loads"). The information is sufficient for a technical reviewer but a first-time evaluator benefits from a short "first-run checklist".
- **PARTIAL PASS** — information is present but not narrativised.

### Environment Rules
- No `npm install`, `pip install`, `apt-get`, or manual DB setup in the README.
- Explicit negation of local toolchain: "No Go, Node.js, or PostgreSQL installation is required on the host — every language toolchain and the database run inside containers" ([README.md:59-60](repo/README.md)).
- `docker-compose up` and the test script are both Docker-only.
- **PASS.**

### Demo Credentials (auth-gated app → required)
- All five roles enumerated with **username, password, and notes** at [README.md:134-140](repo/README.md):
  - admin / `AdminTest123!`
  - frontdesk / `FrontDeskTest1!`
  - labtech / `LabTechTest123!`
  - dispatch / `DispatchTest1!`
  - analyst / `AnalystTest123!`
- Activation flag documented (`SEED_DEMO_USERS=1`) with safety commentary ([README.md:125-133](repo/README.md)).
- Post-rotation gate documented ([README.md:146-161](repo/README.md)) so an evaluator knows they'll be prompted to rotate before the API unlocks.
- **PASS.**

---

## Engineering Quality (non-hard-gate)

| Dimension | Status | Notes |
|---|---|---|
| Tech-stack clarity | Strong | Architecture block at [README.md:14-23](repo/README.md) names versions (React 18, TS 5, Vite 5, Go 1.22, Echo v4, Postgres 14, Argon2id, AES-256-GCM). |
| Architecture explanation | Strong | Intro paragraph plus project-structure ASCII tree at [README.md:27-47](repo/README.md). |
| Testing instructions | Strong | Dedicated Testing section [README.md:99-123](repo/README.md) with `./run_tests.sh` and what it actually executes. |
| Security / roles | Strong | Role table + SEED_DEMO_USERS discipline + password-rotation gate all covered. |
| Workflows | Medium | Business flows described in intro but there's no "day-one user walkthrough". |
| Presentation | Strong | Clean formatting, consistent section naming, pinned versions. |

---

## README Output

### High Priority Issues
None.

### Medium Priority Issues
1. **Verification method is implicit rather than stepwise.** Add a short "First-run checklist" section with 3–5 concrete steps (e.g., `curl /api/health` → open `http://localhost:3000` → sign in as `admin/AdminTest123!` → rotate password → verify Admin page loads). Reduces onboarding friction for a reviewer.

### Low Priority Issues
1. The seeded-credentials table shows the *pre-rotation* passwords but does not show what policy constraints apply to the replacement. Adding a one-line pointer — "new password must satisfy the policy at `backend/internal/auth/password.go`" — closes the loop.
2. `docs/apispec.md` is referenced ([README.md:87](repo/README.md)) but its existence / scope is not teased in the README narrative. A one-line "the full endpoint catalog lives in `docs/apispec.md`" earlier in the README would help.
3. The tree figure at [README.md:27-47](repo/README.md) omits the newly-added `.env.bak` rotation artefact (now referenced in `start.sh`). Minor documentation drift.

### Hard Gate Failures
None.

### README Verdict: **PASS**
All hard gates cleared. One Medium-priority suggestion (stepwise verification), three Low-priority suggestions. No blockers.

---

# Combined Final Verdicts

| Audit | Verdict |
|---|---|
| **Test Coverage** | **Score 90/100** — strong HTTP + unit + integration coverage, no transport/controller mocking, frontend unit tests present across every surface; deducted for no FE↔BE E2E and one untested liveness probe. |
| **README** | **PASS** — every hard gate met; three minor polish suggestions only. |
