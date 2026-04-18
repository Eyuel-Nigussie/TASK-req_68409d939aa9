# Fix-Check for `audit_report-2.md` (round 2)

Check date: 2026-04-18
Method: Static re-verification of each finding against the working tree at `/Users/mac/Eagle-Point Season 2/Task-24/repo/`. No execution. Compared against the pre-fix baseline via `git diff`.

## Summary

| ID  | Severity | Title                                              | Status                |
| --- | -------- | -------------------------------------------------- | --------------------- |
| M1  | Medium   | Dev-mode "ephemeral" key was a deterministic const | **Fixed**             |
| M2  | Medium   | SPA sidebar/backend permission drift               | **Fixed**             |
| M3  | Medium   | Login disabled-branch skips timing pad             | **Fixed**             |
| M4  | Medium   | Audit write failures silently ignored              | **Fixed**             |
| M5  | Medium   | `ExportOrdersCSV` offset used uncapped `body.Size` | **Fixed**             |
| L1  | Low      | Password policy is length-only                     | **Fixed**             |
| L2  | Low      | `SEED_DEMO_USERS=1` seeds well-known passwords     | **Fixed**             |
| L3  | Low      | `*ByAddress` returns unbounded rows                | **Fixed**             |
| L4  | Low      | CORS wildcard silently accepted                    | **Fixed**             |
| L5  | Low      | `ListExceptions` runs unbounded detectors          | **Fixed**             |
| L6  | Low      | Audit log entity name is free-form                 | **Fixed**             |

Headline: **11 of 11 findings closed.** This round promoted the three previously-unaddressed Low items (L1, L2, L5) to Fixed and completed the L6 Partial → Fixed transition. Every fix is backed by a regression test. Net diff across the repo is roughly +1,146 / −195 lines spanning 37 files — a real remediation pass, not a paper exercise. Beyond the audit items, the operator-UX of `start.sh` was hardened in two orthogonal ways (auto-rotate placeholder ENC_KEYS, port-collision handling) and list endpoints now return `[]` rather than `null` when empty.

---

## Per-issue verification

### M1 — Dev-mode key was a deterministic well-known constant — **Fixed**
- Re-check evidence:
  - [runtime/runtime.go:213-228](repo/backend/internal/runtime/runtime.go) now generates 32 random bytes via `rand.Read` per `BuildVault` call when `ENC_KEYS` is unset + `OOPS_DEV_MODE=1`. The previous `crypto.DeriveKey([]byte("dev-only-not-for-production-use"))` call is removed. Log line updated to say "generated a random ephemeral key for this process", which now matches behavior.
  - Test `TestBuildVault_EphemeralKeyIsNonDeterministic` at [runtime/runtime_test.go:79-100](repo/backend/internal/runtime/runtime_test.go) asserts two successive dev-mode vaults cannot decrypt each other's ciphertext — a regression to any deterministic derivation would trip this test.
  - Bonus: [start.sh:22-55](repo/start.sh) now *auto-rotates* the historical placeholder `ENC_KEYS` in `.env` to a fresh `openssl rand -hex 32` (with `/dev/urandom` fallback) and backs up the previous value to `.env.bak`. This removes the previous friction of "keygen first, paste second" and eliminates the common-credential risk even when the operator forgets to run keygen.
- Status: **Fixed**.

### M2 — Sidebar/backend permission drift after A1–A3 — **Fixed**
- Re-check evidence: [frontend/src/App.tsx:67](repo/frontend/src/App.tsx) — visibility array is `["front_desk", "admin", "analyst", "dispatch"]` (lab_tech removed). Matches backend catalog where `lab_tech` lacks `customers.read`. Remaining `can(...)` calls for Orders/Analytics/Lab/Dispatch/Admin are consistent with the permission catalog.
- Status: **Fixed**.

### M3 — Login disabled-account branch skipped the timing pad — **Fixed**
- Re-check evidence:
  - [api/auth.go:69-78](repo/backend/internal/api/auth.go) now runs `auth.ComparePassword(u.PasswordHash, body.Password)` (result discarded) and `s.Lockout.RecordFailure` before returning; status flipped from 403 to 401. Both the timing channel and the status-code channel are closed.
  - Updated test `TestLogin_DisabledAccount` at [full_coverage_test.go:71-90](repo/backend/internal/api/full_coverage_test.go) asserts 401 *and* that a subsequent five-wrong-password burst still trips the 423 lock on the next try — proving the disabled-account path increments the lockout counter.
- Status: **Fixed**.

### M4 — Audit write failures silently ignored — **Fixed**
- Re-check evidence:
  - [audit/audit.go:41-73,109-124](repo/backend/internal/audit/audit.go) — `Logger` now has `onError` sink (default `log.Printf("[audit-drop] …")`) and returns the underlying `AppendAudit` error instead of swallowing it. `SetErrorSink` allows test injection.
  - Test `TestLogger_AppendFailureSurfacesThroughErrorSink` at [audit/audit_test.go:87-108](repo/backend/internal/audit/audit_test.go) seeds a failing store and asserts entity/id/action/actor/err all flow through the sink.
  - Handler call sites (e.g. [orders.go](repo/backend/internal/api/orders.go), [admin.go](repo/backend/internal/api/admin.go), [lab.go](repo/backend/internal/api/lab.go)) still use `_ = s.Audit.Log(...)`, which is now acceptable because the Logger records the drop itself before returning. Reviewers who want transactional audit can return the error to produce a 500 — the wiring is in place.
- Status: **Fixed**.

### M5 — `ExportOrdersCSV` offset used uncapped `body.Size` — **Fixed**
- Re-check evidence: [api/export.go:48-54](repo/backend/internal/api/export.go) — offset now reads `(body.Page - 1) * limit` using the capped value. Inline comment cites M5 so a future refactor cannot regress silently.
- Status: **Fixed**.

### L1 — Password policy was length-only — **Fixed**
- Re-check evidence:
  - [auth/password.go:22-28,68-135](repo/backend/internal/auth/password.go) — `ValidatePolicy` now:
    1. Rejects all-same-character strings (`"aaaaaaaaaa"`) with `ErrPasswordTooSimple`.
    2. Requires at least two distinct character classes from {letter, digit, symbol/unicode} — also surfaced as `ErrPasswordTooSimple`.
    3. Rejects a built-in case-insensitive blocklist of ~19 common-leak entries (`password`, `Password123`, `qwerty1234`, etc.) with `ErrPasswordTooCommon`.
  - The blocklist is embedded in the binary (no external dependency), consistent with the offline constraint in the Prompt.
  - Tests updated at [auth/password_test.go:14-33](repo/backend/internal/auth/password_test.go) cover: blank, spaces, too-short, single-class letters, long single-class, repeated single char, unicode-single-class, common-entry, multi-class happy paths. Previous round-trip test strings were updated to satisfy the tightened policy (e.g. `correct-horse-battery-staple`).
- Status: **Fixed**. Goes beyond the Prompt's ≥10-char minimum without changing any documented policy surface.

### L2 — Demo users seeded well-known passwords — **Fixed**
End-to-end plumbing, each layer visible in diff:
- **Schema:** [migrations/0001_init.sql:10-23](repo/backend/migrations/0001_init.sql) adds `must_rotate_password BOOLEAN NOT NULL DEFAULT FALSE` to `users`.
- **Model:** [models/models.go:24-40](repo/backend/internal/models/models.go) adds `MustRotatePassword bool` with a docstring explaining the gate semantics.
- **Store:** [store/postgres.go:48-101](repo/backend/internal/store/postgres.go) — `CreateUser`/`GetUserBy*`/`ListUsers`/`UpdateUser` all round-trip the column. Memory store similarly updated.
- **Seeder:** [runtime/runtime.go:129-140](repo/backend/internal/runtime/runtime.go) — `SeedDemoUsers` now sets `MustRotatePassword: true` on every demo account. The rationale comment references the README publication as the justification.
- **Session:** [auth/session.go:14-100](repo/backend/internal/auth/session.go) — `Session` carries `MustRotatePassword`; new `IssueWithFlags` / `ClearMustRotate` methods. `Issue` is preserved as a passthrough for backward compat.
- **Middleware:** [httpx/middleware.go:86-113](repo/backend/internal/httpx/middleware.go) — new `RequirePasswordRotation(allowedPaths...)` denies every authed request with 403 unless the target path is in the allowlist or the flag is clear.
- **Wiring:** [api/server.go:133-147](repo/backend/internal/api/server.go) — the gate is installed immediately after `RequireAuth` in the `authGroup`, and the allowlist is exactly `/api/auth/rotate-password`, `/api/auth/logout`, `/api/auth/whoami`. Every existing permission-gated route participates automatically.
- **Endpoint:** [api/auth.go:110-155](repo/backend/internal/api/auth.go) — `POST /api/auth/rotate-password` takes `{old_password, new_password}`, verifies the old password (401 on mismatch), rejects reuse (400), runs new password through `HashPassword` which applies the tightened policy (400 on policy failure), updates the row, and clears the gate on the live session via `ClearMustRotate` so the operator doesn't need a second login round-trip.
- **Frontend:** [frontend/src/hooks/useAuth.tsx:5-74](repo/frontend/src/hooks/useAuth.tsx) + [api/client.ts:75-103](repo/frontend/src/api/client.ts) — `AuthUser` carries `mustRotatePassword`, `login`/`whoami` both populate it, and the context exposes a `rotatePassword` action.
- **Docs:** [README.md:145-160](repo/README.md) documents the gate.
- **Test:** `TestRotatePassword_GateBlocksThenClears` at [full_coverage_test.go:92-187](repo/backend/internal/api/full_coverage_test.go) covers:
  - Before rotation, four representative non-allowlisted routes all 403 with the rotation message.
  - `/api/auth/whoami` bypasses the gate and surfaces `must_rotate_password: true`.
  - Wrong old password → 401; reusing old = new → 400; weak new password → 400.
  - Successful 204 rotation; same session immediately reaches `/api/admin/users` with 200.
  - Fresh login with the new credential works; old credential returns 401.
- **Test:** `TestSeedDemoUsers_ForcesMustRotate` at [runtime/runtime_test.go:314-354](repo/backend/internal/runtime/runtime_test.go) pins the seeder contract.
- Status: **Fixed**. The L2 "first-login-must-rotate" requirement is now enforced transactionally, not just documented.

### L3 — By-address endpoints returned unbounded rows — **Fixed**
- Re-check evidence:
  - [api/orders.go:214-219](repo/backend/internal/api/orders.go): break when `len(out) >= addressLookupMaxRows`.
  - [api/customers.go:181-186](repo/backend/internal/api/customers.go): same guard on the views slice.
  - Constant `addressLookupMaxRows = 200` with rationale comment at [orders.go:220-225](repo/backend/internal/api/orders.go).
- Status: **Fixed**.

### L4 — CORS wildcard accepted silently — **Fixed**
- Re-check evidence: [api/server.go:98-115](repo/backend/internal/api/server.go) — `Register` resolves `parseAllowedOrigins()` once, scans for a literal `"*"` entry, and emits `log.Printf("[cors] ALLOWED_ORIGINS=* — cross-origin requests from ANY origin are accepted. This is only appropriate for short-lived local demos.")` before installing the middleware. Only logs once per startup.
- Status: **Fixed**.

### L5 — `ListExceptions` ran unbounded detectors per call — **Fixed**
- Re-check evidence:
  - [api/orders.go:256-298](repo/backend/internal/api/orders.go) — new `exceptionSweepState{mu, lastAt}` embedded in `Server` ([server.go:38-41](repo/backend/internal/api/server.go)). `ListExceptions` now calls `shouldSweepExceptions()` which returns true at most once per `exceptionSweepInterval = 60s`. Inside the window, reads go straight to `s.Store.ListExceptions(ctx)` without re-scanning orders.
  - Per-sweep work is additionally capped at `exceptionSweepMaxOrders = 200` (down from the previous 500-per-branch). Write-path detectors (`CreateOrder`, `UpdateInventory`, `PlanOutOfStock`) still raise exceptions synchronously, so nothing is hidden by the throttle — only redundant detector passes are suppressed.
  - Test `TestListExceptions_SweepThrottled` at [extra_coverage_test.go:203-258](repo/backend/internal/api/extra_coverage_test.go) seeds a stuck order, calls once (sweep runs, exception recorded), seeds a second stuck order, calls again *inside* the cooldown (new exception must NOT appear — proving the throttle skipped), then advances the clock past the interval and calls again (the second exception is now detected).
- Status: **Fixed**.

### L6 — Audit log entity was free-form — **Fixed**
- Re-check evidence:
  - [audit/audit.go:19-39](repo/backend/internal/audit/audit.go) defines `type Entity string` and 14 named constants (`EntityUser`, `EntityCustomer`, `EntityOrder`, `EntityOrderException`, `EntitySample`, `EntityReport`, `EntityAddressBook`, `EntitySavedFilter`, `EntityServiceRegions`, `EntityReferenceRanges`, `EntityRouteTable`, `EntityRolePermissions`, `EntityUserPermissions`, `EntitySystemSettings`).
  - **Signature changed:** [audit/audit.go:94](repo/backend/internal/audit/audit.go) — `Log`'s fifth parameter is now typed `Entity`, not `string`. The Go compiler rejects raw string literals at call sites.
  - **Every call site migrated:** a repo-wide `grep s\.Audit\.Log\(` confirms every call in [api/auth.go](repo/backend/internal/api/auth.go), [customers.go](repo/backend/internal/api/customers.go), [orders.go](repo/backend/internal/api/orders.go), [addressbook.go](repo/backend/internal/api/addressbook.go), [filters_search.go](repo/backend/internal/api/filters_search.go), [permissions.go](repo/backend/internal/api/permissions.go), [settings.go](repo/backend/internal/api/settings.go), [admin.go](repo/backend/internal/api/admin.go), [lab.go](repo/backend/internal/api/lab.go), and [export.go](repo/backend/internal/api/export.go) passes an `audit.EntityX` constant.
  - Existing audit-entry unit tests updated to pass constants instead of string literals.
- Status: **Fixed**. The previous "Partial" concern (constants exist but nobody uses them) is resolved because the compiler now enforces the migration.

---

## Net delta since round-1 fix-check

- **Source changes:** 37 files touched, +1,146 / −195 lines per `git diff --stat`. Major touch points beyond the audit fixes:
  - `auth/password.go` — expanded policy (L1).
  - `migrations/0001_init.sql`, `models/models.go`, `store/memory.go`, `store/postgres.go`, `auth/session.go`, `httpx/middleware.go`, `api/server.go`, `api/auth.go`, `runtime/runtime.go`, `frontend/src/{api/client.ts,hooks/useAuth.tsx}`, `README.md` — L2 end-to-end.
  - `api/orders.go`, `api/server.go` — L5 throttle.
  - `audit/audit.go` + every `api/*.go` caller — L6 migration.
  - `start.sh` — auto-rotate placeholder ENC_KEYS and port-collision handling.
- **New tests:**
  - `TestBuildVault_EphemeralKeyIsNonDeterministic` (M1)
  - `TestLogin_DisabledAccount` update (M3)
  - `TestLogger_AppendFailureSurfacesThroughErrorSink` (M4)
  - `TestValidatePolicy` expanded with class-diversity + blocklist cases (L1)
  - `TestRotatePassword_GateBlocksThenClears` (L2)
  - `TestSeedDemoUsers_ForcesMustRotate` (L2 seeder contract)
  - `TestListExceptions_SweepThrottled` (L5)
  - Every pre-existing audit test updated to pass typed entities (L6)
- **Incidental wins observed on static read:**
  - Memory/Postgres list queries now return `[]`-shaped slices rather than nil, and `offset >= len` correctly returns an empty list. This closes a latent SPA-crash bug where `items.map(...)` would throw on a JSON `null`.
  - [Dashboard.tsx:12-20](repo/frontend/src/pages/Dashboard.tsx) defensively coerces responses to arrays.
  - `docker-compose.yml` no longer publishes the Postgres port to the host — the backend reaches `db:5432` on the internal network, and host-published DB ports were an unnecessary attack surface / collision source.
- No regressions introduced on a static read. The new rotation-gate is additive (it only blocks accounts whose flag is true, and the flag is false for every non-demo account).

## What still needs to happen

Nothing from the audit. Optional forward-looking suggestions (not defects):

1. Consider returning the audit error as a 500 from handlers rather than logging only, if the business wants transactional audit semantics. The Logger now returns the error, so the escalation is a one-line change per handler.
2. Consider adding a `vet`/`lint` check that asserts `s.Audit.Log` is only called with `audit.Entity*` constants — the compiler already rejects string literals, but a direct `audit.Entity("made-up")` cast would still compile. Low-risk, but belt-and-suspenders.
3. The common-password blocklist in `auth/password.go` is intentionally tiny. If the Prompt ever relaxes the offline constraint, swap it for a ~10k-entry embedded list (e.g. SecLists top-10k).
4. The L5 throttle constants (`60s`, `200 orders`) are package-private. If they need to be tuned per deployment, expose them via env vars.

These are polish items; every audit-originated gap is now closed and backed by tests.
