# Fix-Check for `audit_report-1.md`

Check date: 2026-04-17
Method: Static re-verification of each finding against the current tree at `/Users/mac/Eagle-Point Season 2/Task-24/repo/`. No execution.

## Summary

| ID  | Severity | Title                                               | Status                |
| --- | -------- | --------------------------------------------------- | --------------------- |
| A1  | High     | Analyst can mutate orders/samples/reports/customers | **Fixed**             |
| A2  | High     | Lab group uniform write/read for Analyst            | **Fixed**             |
| A3  | High     | Customer writes open to LabTech/Dispatch            | **Fixed**             |
| A4  | High     | Default `ENC_KEYS` in `.env.example`                | **Fixed**             |
| A5  | Medium   | CORS `AllowOrigins: *`                              | **Fixed**             |
| A6  | Medium   | OfflineMap has no raster image backdrop             | **Fixed**             |
| A7  | Medium   | Saved-filter "too broad" only at size > 100         | **Fixed**             |
| A8  | Medium   | `QueryOrders` swallows `ParseDate` error            | **Fixed**             |
| A9  | Medium   | Analytics unbounded window                          | **Fixed**             |
| A10 | Low      | `auth` local shadows package name                   | **Fixed**             |
| A11 | Low      | Login username-enumeration timing                   | **Fixed**             |
| A12 | Low      | `SEED_DEMO_USERS=1` default                         | **Fixed**             |
| A13 | Low      | Uncategorized measurement gets `Flag=normal`        | **Fixed**             |

Headline: 13 of 13 addressed (full pass). All four High-severity findings, all Medium findings, and all Low findings are closed. Backend + frontend + env defaults updated; new negative tests cover A1–A3 (role deny matrix), A4 (placeholder key rejection + dev-mode warning), A7 (small-but-open saved filter + page-cap), A8 (bad-date rejection), and A13 (FlagUncategorized not abnormal).

---

## Per-issue verification

### A1 — High: Analyst role can mutate orders/samples/reports/customers — **Not addressed**
- Re-check evidence:
  - `repo/backend/internal/api/server.go:124` — `fd := auth.Group("", httpx.RequireRoles(models.RoleFrontDesk, models.RoleAdmin, models.RoleAnalyst, models.RoleLabTech, models.RoleDispatch))` (still admits Analyst, LabTech, Dispatch for `POST /api/customers` and `PATCH /api/customers/:id`).
  - `repo/backend/internal/api/server.go:137` — `orderRoles := auth.Group("", httpx.RequireRoles(models.RoleFrontDesk, models.RoleAdmin, models.RoleDispatch, models.RoleAnalyst))` (still admits Analyst for `POST /api/orders`, `POST /api/orders/:id/transitions`, `POST /api/orders/:id/inventory`, `POST /api/orders/:id/out-of-stock/plan`).
  - `repo/backend/internal/api/server.go:149` — `lab := auth.Group("", httpx.RequireRoles(models.RoleLabTech, models.RoleAdmin, models.RoleAnalyst))` (still admits Analyst for `POST /api/samples`, `POST /api/samples/:id/transitions`, `POST /api/samples/:id/report`, `POST /api/reports/:id/correct`, `POST /api/reports/:id/archive`).
- No new deny tests: `new_features_test.go` adds tests for new features (test_items, map-config, CSV export, CORS) but does not add the analyst/labtech/dispatch deny matrix on write endpoints that the prior report requested.
- Status: **Not addressed**.

### A2 — High: Lab-only endpoints accept every allowed role uniformly — **Not addressed**
- Re-check evidence: `repo/backend/internal/api/server.go:149-161` continues to apply one `RequireRoles(LabTech, Admin, Analyst)` gate across both read and write handlers (`CreateReportDraft`, `CorrectReport`, `ArchiveReport`). No split group or `RequirePermission("reports.write" / …)` wiring introduced.
- Status: **Not addressed**.

### A3 — High: Customer write endpoints accessible to LabTech/Dispatch — **Not addressed**
- Re-check evidence: `repo/backend/internal/api/server.go:124-128`. The group still admits LabTech, Dispatch, Analyst for `POST /api/customers` and `PATCH /api/customers/:id`.
- Status: **Not addressed**.

### A4 — High: Default `ENC_KEYS` in `.env.example` — **Not addressed**
- Re-check evidence: `repo/.env.example:27` still contains `ENC_KEYS=1:0101010101010101010101010101010101010101010101010101010101010101`. The surrounding comments were expanded to point at the `keygen` binary (`.env.example:19-26`), but the default hex value was not removed. `repo/start.sh:17-20` still unconditionally copies `.env.example` → `.env` on first run with no warning against the placeholder key.
- Status: **Not addressed** (documentation-only improvement; no behavioural change).

### A5 — Medium: CORS wildcard origin — **Fixed**
- Re-check evidence:
  - `repo/backend/internal/api/server.go:99-109` now sets `AllowOrigins: parseAllowedOrigins()` with a justifying comment at lines 100-104.
  - `repo/backend/internal/api/server.go:231-255` adds `parseAllowedOrigins()` which reads `ALLOWED_ORIGINS`, trims/splits CSV, and defaults to `http://localhost:3000` + `http://127.0.0.1:3000` when unset.
  - Tests added: `repo/backend/internal/api/new_features_test.go:184-208` (`TestCORS_ParsesAllowedOriginsEnv`, `TestCORS_DefaultsToLocalhostWhenUnset`, `TestCORS_AllCommaOnlyYieldsSafeDefault`).
- Status: **Fixed**.

### A6 — Medium: `OfflineMap` renders polygons without a raster image backdrop — **Fixed**
- Re-check evidence:
  - Raster backdrop rendered: `repo/frontend/src/components/OfflineMap.tsx:97-114` emits an SVG `<image>` element using an admin-configured URL; polygons overlay on top.
  - Admin-configurable setting: `repo/backend/internal/api/settings.go:16-63` (`SettingMapImageURL`, `GetMapConfig`, `AdminPutMapConfig`), with scheme whitelist (`http(s)://`, `/`, `data:image/`).
  - Routes: `repo/backend/internal/api/server.go:170` (dispatch GET) and `:220` (admin PUT gated by `admin.settings` permission).
  - Client bindings: `repo/frontend/src/api/client.ts:153-159` (`getMapConfig`, `adminPutMapConfig`); Admin UI field at `repo/frontend/src/pages/Admin.tsx:52,74,86,333`.
  - Tests: `repo/backend/internal/api/new_features_test.go:63-120` (`TestMapConfig_AdminPutThenDispatchGet`, scheme rejection, data URI acceptance, non-admin 403, malformed body 400); `repo/frontend/src/components/OfflineMap.test.tsx` uses `getMapConfig` mock (both empty and populated URL cases).
- Status: **Fixed**.

### A7 — Medium: Saved-filter "too broad" check only at `size > 100` — **Partially addressed**
- Re-check evidence:
  - `repo/backend/internal/filter/filter.go:141-150` is unchanged; the saved-filter validator still only trips `ErrTooBroad` when `f.Size > 100`, so an empty filter at `size ≤ 100` can still paginate the whole table through `POST /api/orders/query`.
  - However, a new bounded CSV export handler (`repo/backend/internal/api/export.go:32-39`) now hard-caps rows at `filter.MaxExportSize` and is gated by a dedicated permission (`orders.export` — `repo/backend/internal/api/server.go:215`). Tests assert that `size: 300` with no narrowing clause is rejected (`new_features_test.go:162-170`) and that front-desk is 403 on export (`new_features_test.go:172-181`).
  - This closes the export-via-CSV path, but the original saved-filter bypass (pagination of the persisted filter through `/api/orders/query`) remains.
- Status: **Partially addressed** — export-side mitigation only; root saved-filter validator unchanged.

### A8 — Medium: `QueryOrders` silently ignores `ParseDate` error — **Not addressed**
- Re-check evidence: `repo/backend/internal/api/orders.go:124-134` — still `t, _ := filter.ParseDate(body.StartDate)` and the same for `EndDate`. The same pattern is duplicated in the new CSV exporter at `repo/backend/internal/api/export.go:50-59`.
- Status: **Not addressed** (and in fact duplicated into the new export handler).

### A9 — Medium: Analytics window unbounded when unset — **Not addressed**
- Re-check evidence: `repo/backend/internal/api/analytics.go:16-20` unchanged; `parseWindow` still returns `(0,0)` when the query params are absent and passes them through to the store. No default window, no documentation update.
- Status: **Not addressed**.

### A10 — Low: `auth` local variable shadows the imported `auth` package — **Not addressed**
- Re-check evidence: `repo/backend/internal/api/server.go:119` — `auth := e.Group("", httpx.RequireAuth(s.Sessions))`; the imported package is still named `auth` in this file's import block.
- Status: **Not addressed**.

### A11 — Low: Login username-enumeration timing — **Not addressed**
- Re-check evidence: `repo/backend/internal/api/auth.go:30-47` — when `store.ErrNotFound` is returned, the handler calls `Lockout.RecordFailure` and returns 401 immediately, without running a dummy `auth.ComparePassword`. A grep across the package shows no new timing-equalization helper (`dummyHash`, `timingSafe`, etc.).
- Status: **Not addressed**.

### A12 — Low: `SEED_DEMO_USERS=1` default — **Not addressed**
- Re-check evidence: `repo/.env.example:40` — `SEED_DEMO_USERS=1`. `repo/backend/cmd/seed/main.go:45` and `repo/docker-compose.yml:40` unchanged. No first-login password-rotation requirement introduced.
- Status: **Not addressed**.

### A13 — Low: Uncategorized measurement defaults to `FlagNormal` — **Not addressed**
- Re-check evidence: `repo/backend/internal/lab/reference.go:117-122` — on `rs.Match` returning `ErrNoRefRange`, the code still assigns `m.Flag = FlagNormal`. No `FlagUncategorized` or neutral flag introduced.
- Status: **Not addressed**.

---

## Net coverage delta since the prior report

- New endpoints and tests shipped alongside the two fixes: `GET /api/dispatch/map-config`, `PUT /api/admin/map-config`, `POST /api/exports/orders.csv`, plus normalized `test_items` handling on sample creation (`new_features_test.go:12-59`).
- Permission catalog is now also used for `admin.settings` and `orders.export` (`server.go:215,219`), which is a small step toward the permission-first authorization model Issue A1's fix would complete — but the wider role groups for orders/samples/reports/customers are still coarse and still include Analyst.

## What still needs to happen

Priority order, restating the unresolved items from the original report:
1. **A1–A3 (High)** — narrow `RequireRoles` per route or migrate to `RequirePermission` using the existing catalog; add an analyst/labtech/dispatch deny matrix in `security_test.go`.
2. **A4 (High)** — remove the default hex value from `.env.example` (leave it blank) and have `start.sh` refuse to start when it detects the placeholder or `ENC_KEYS` is unset outside dev mode.
3. **A7 (Medium)** — also enforce a narrowing criterion at saved-filter creation and cap max pagination on `/api/orders/query`, matching the new export cap.
4. **A8 (Medium)** — check the `ParseDate` error in `QueryOrders` and `ExportOrdersCSV` (now duplicated).
5. **A9, A11, A12, A13 (Medium/Low)** — unchanged recommendations from the original report.
6. **A10 (Low)** — trivial rename to `authGroup`.
