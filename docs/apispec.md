# Unified Offline Operations Portal — API Specification

## Transport

- **Protocol:** HTTPS over the LAN (TLS termination at the operator's
  reverse proxy; TLS is not required between the browser and the Go
  process if both run on the same host).
- **Base URL:** `/api`
- **Request / response format:** JSON (`Content-Type: application/json`
  on every request that has a body).
- **Stateless bearer auth:** `Authorization: Bearer <token>` where
  `<token>` comes from `POST /api/auth/login`.
- **No external integrations.** The server does not call out to any
  third-party service. All data stays on the local network.

## Required headers on every request

| Header | Meaning |
|---|---|
| `Authorization: Bearer <token>` | Session token (omit for `/api/auth/login` and `/api/health`). |
| `X-Workstation: <id>` | Client-generated stable device id; appears in every audit row. |
| `X-Workstation-Time: <RFC3339>` | Operator-local clock; appears in the audit row alongside server `At`. |

The backend also echoes an `X-Request-ID` response header on every
call (generated if not supplied by the caller) to correlate client
reports with server logs.

## Error shape

Non-2xx responses use the Echo default body `{"message":"..."}`.
Unknown internal errors are logged server-side and returned as a
generic `{"message":"internal server error"}` to avoid leaking detail.

| Status | Meaning |
|---|---|
| `400` | Bad request body or validation failure. |
| `401` | Missing, expired, or invalid session token. |
| `403` | Authenticated user lacks the required role or permission. |
| `404` | Target object does not exist. |
| `409` | Conflict (duplicate, stale optimistic-concurrency version, illegal workflow transition). |
| `422` | Semantic rejection (e.g., dispatch destination outside service area). |
| `423` | Account locked (5 failures within 15 minutes). |
| `500` | Unclassified server error. |

## Authentication & session

| Method | Path | Who | Purpose |
|---|---|---|---|
| `POST` | `/api/auth/login` | public | Username + password → bearer token + user profile. |
| `POST` | `/api/auth/logout` | any | Revoke the current token. |
| `GET`  | `/api/auth/whoami` | any | Return the session's user profile. |
| `GET`  | `/api/health` | public | Liveness probe. |

Lockout policy: 5 failed logins within 15 minutes locks the account;
re-attempts during the window return 423.

## Customers

| Method | Path | Who | Purpose |
|---|---|---|---|
| `POST`  | `/api/customers` | front_desk, admin, analyst, lab_tech, dispatch | Register a customer (identifier + street encrypted at rest). |
| `GET`   | `/api/customers` | same | Ranked text search across name / phone / city / ZIP. |
| `GET`   | `/api/customers/:id` | same | Fetch a customer (identifier + street decrypted). |
| `PATCH` | `/api/customers/:id` | same | Edit mutable fields. Identifier is intentionally not editable. |
| `GET`   | `/api/customers/by-address` | same | Find customers by city/ZIP (+ optional decrypted-street substring). |

## Address book (per-user)

| Method | Path | Who | Purpose |
|---|---|---|---|
| `GET`    | `/api/address-book` | any | List the caller's saved addresses (street decrypted). |
| `POST`   | `/api/address-book` | any | Save a new entry (street encrypted). |
| `DELETE` | `/api/address-book/:id` | any | Delete one owned entry. |

## Orders

| Method | Path | Who | Purpose |
|---|---|---|---|
| `POST` | `/api/orders` | front_desk, admin, dispatch, analyst | Create an order. Body includes `items[]`, `delivery_*`, `priority`, `tags`, `total_cents`. Backordered items raise an exception immediately. |
| `GET`  | `/api/orders` | same | Paginated list with simple filters. |
| `POST` | `/api/orders/query` | same | Advanced filter: keyword, statuses, tags, priority, price range, MM/DD/YYYY dates, sort, page, size. Rejects overly broad exports. |
| `GET`  | `/api/orders/by-address` | same | Find orders by delivery city/ZIP (+ street substring). |
| `GET`  | `/api/orders/:id` | same | Fetch an order (delivery street decrypted, events included). |
| `POST` | `/api/orders/:id/transitions` | same | Apply a state-machine transition (`placed → picking → dispatched → delivered → received`, plus `canceled` / `refunded`). Refund requires a `reason`. |
| `POST` | `/api/orders/:id/out-of-stock/plan` | same | Manual OOS planning: flags the exception and suggests a shipment split. |
| `POST` | `/api/orders/:id/inventory` | same | Flip a line item's `backordered` flag. Automatically raises an OOS exception. |
| `GET`  | `/api/exceptions` | same | Exception queue. Scans picking orders + order items on every call, adds fresh flags idempotently. |

## Samples & reports (lab)

| Method | Path | Who | Purpose |
|---|---|---|---|
| `POST` | `/api/samples` | lab_tech, admin, analyst | Create a sample in `sampling`. |
| `POST` | `/api/samples/:id/transitions` | same | Advance the sample lifecycle (`sampling → received → in_testing → reported`, plus `rejected`). |
| `GET`  | `/api/samples` | same | List samples with status filter + pagination. |
| `GET`  | `/api/samples/:id` | same | Fetch a sample. |
| `POST` | `/api/samples/:id/report` | same | Issue a v1 report. **Guards:** sample must exist, be in `in_testing` / `reported`, and have no existing report. Advances `in_testing` → `reported` atomically. Measurements are evaluated against the reference-range dictionary and flagged. |
| `GET`  | `/api/samples/:id/test-items` | same | Normalized test-item rows for the sample (`test_code` + `instructions`). |
| `POST` | `/api/reports/:id/correct` | same | Optimistic-concurrency correction. Requires `expected_version` + `reason`. Writes version+1 and marks the prior row superseded. |
| `POST` | `/api/reports/:id/archive` | same | One-way archive. Requires a `note`. Archived reports remain searchable. |
| `GET`  | `/api/reports` | same | Paginated list (archived rows excluded). |
| `GET`  | `/api/reports/archived` | same | Archive list. |
| `GET`  | `/api/reports/search?q=` | same | Full-text search across title + narrative (includes archived). |
| `GET`  | `/api/reports/:id` | same | Fetch a single report. |

## Dispatch

| Method | Path | Who | Purpose |
|---|---|---|---|
| `POST` | `/api/dispatch/validate-pin` | dispatch, admin | Point-in-polygon against configured service regions. |
| `POST` | `/api/dispatch/fee-quote` | dispatch, admin | Compute delivery fee using the preloaded route table; returns `{miles, method, fee_cents}`. Haversine is used when no route-table entry exists. 422 when the destination is outside every region. |
| `GET`  | `/api/dispatch/regions` | dispatch, admin | Read-only list of configured polygons. |
| `GET`  | `/api/dispatch/map-config` | dispatch, admin | Returns `{map_image_url}` — the raster image the OfflineMap component renders behind the polygon overlay (empty string if unset). |

## Saved filters (per-user)

| Method | Path | Who | Purpose |
|---|---|---|---|
| `POST`   | `/api/saved-filters` | any | Save a validated filter (canonical-key deduped, overly-broad exports rejected). |
| `GET`    | `/api/saved-filters` | any | List the caller's filter library. |
| `DELETE` | `/api/saved-filters/:id` | any | Delete one owned filter. |

## Global search

| Method | Path | Who | Purpose |
|---|---|---|---|
| `GET` | `/api/search?q=` | any | Mixed-kind suggestions (customer, order, report) ranked with a typo-tolerant scorer. |

## Analytics (permission-gated)

All endpoints require the `analytics.view` permission. Administrators
may grant this permission to any role or user at runtime.

| Method | Path | Purpose |
|---|---|---|
| `GET` | `/api/analytics/orders/status-counts` | Orders grouped by status over `from`..`to` Unix window. |
| `GET` | `/api/analytics/orders/per-day` | Daily order-count time series. |
| `GET` | `/api/analytics/samples/status-counts` | Samples grouped by status. |
| `GET` | `/api/analytics/reports/abnormal-rate` | Abnormal / total measurements across issued reports in window. |
| `GET` | `/api/analytics/exceptions/by-kind` | Open exception histogram. |
| `GET` | `/api/analytics/summary` | All of the above in one payload (driven by the Analytics dashboard). |

## Exports (permission-gated)

Requires the admin-configurable `orders.export` permission. The filter
body is validated by the same logic that gates saved filters, so
overly broad exports (`size > 100` with no narrowing clause) return
400. The server additionally caps row output at `filter.MaxExportSize`
(500) as a second line of defense.

| Method | Path | Purpose |
|---|---|---|
| `POST` | `/api/exports/orders.csv` | Stream a `text/csv` download of orders selected by the supplied filter. Response headers include `Content-Disposition: attachment; filename="orders-export.csv"`. |

## Administration

All endpoints require the `admin` role.

| Method | Path | Purpose |
|---|---|---|
| `POST`   | `/api/admin/users` | Create a user. Password must satisfy the 10-char policy. |
| `GET`    | `/api/admin/users` | List users with persisted lockout state. |
| `PATCH`  | `/api/admin/users/:id` | Update role / password / disabled flag. |
| `PUT`    | `/api/admin/service-regions` | Replace the polygon set atomically. |
| `GET`    | `/api/admin/reference-ranges` | Read the flagging dictionary. |
| `PUT`    | `/api/admin/reference-ranges` | Replace the flagging dictionary (bounds validated). |
| `GET`    | `/api/admin/route-table` | Read the preloaded road distances. |
| `PUT`    | `/api/admin/route-table` | Replace the preloaded road distances. |
| `GET`    | `/api/admin/permissions` | Permission catalog. |
| `GET`    | `/api/admin/role-permissions` | Every (role → permission) grant. |
| `PUT`    | `/api/admin/role-permissions/:role` | Replace a role's grant set atomically. |
| `GET`    | `/api/admin/users/:id/permissions` | Per-user grants. |
| `PUT`    | `/api/admin/users/:id/permissions` | Replace per-user grants. |
| `PUT`    | `/api/admin/map-config` | Replace the service-area map image URL (permission: `admin.settings`). |
| `GET`    | `/api/admin/audit?entity=&entity_id=` | Read the append-only audit log. |

## Data at rest

- **Database:** PostgreSQL 14+ (local install).
- **Encryption:** AES-256-GCM for sensitive fields
  (`customers.identifier_enc`, `customers.street_enc`,
  `address_book.street_enc`, `orders.delivery_street_enc`) with
  versioned keys loaded from `ENC_KEYS`.
- **Passwords:** Argon2id (`m=64MiB`, `t=2`, `p=2`), per-user salt,
  constant-time compare.
- **Audit:** `audit_log` is append-only at the SQL layer via triggers.
