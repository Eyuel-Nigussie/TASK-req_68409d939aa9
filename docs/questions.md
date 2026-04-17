# Unified Offline Operations Portal — Open Questions, Risks, and Assumptions

**Document version:** 1.0
**Date:** 2026-04-17
**Relates to:** `docs/design.md` v1.0

This document enumerates ambiguities, risks, and implicit decisions in
the design that need explicit confirmation before implementation is
considered complete. Each item includes a question, the assumption we
are proceeding under if no answer is given, and the concrete solution
we will implement under that assumption.

---

## Q1. Sessions are process-memory only

**Question.** `auth.SessionStore` holds tokens in a `map` guarded by a
`sync.RWMutex`. A server restart invalidates every session, forcing
every operator to sign in again. For a single-box LAN deploy a
restart is infrequent, but during a deploy window the entire shop
gets kicked out at once. Is that acceptable?

**My understanding.** Yes for this release. The product is a single-box
LAN service; deploys happen overnight; re-authentication is cheap.
Distributed-session storage would add a moving part that we don't
need today.

**solution.** Keep sessions in process memory. If a future release
clusters the server for HA, move the session cache to a
`store.Session` interface backed by PostgreSQL, keyed by the same
32-byte hex token, and TTL-enforced at the query layer
(`expires_at > now()`). The front end already holds the token in
`localStorage`, so the only layer that needs to change is the
backend cache.

---

## Q2. Multi-tenancy is out of scope — reserve `tenant_id` now?

**Question.** The schema has no tenant dimension. Every migration
query, index, and handler assumes a single organization. Should we
preemptively add `tenant_id` columns to avoid a disruptive migration
if multi-tenancy is later adopted?

**My understanding.** No. Adding `tenant_id` everywhere speculatively
introduces cost (wider rows, composite indexes, joins, handler
threading) for a feature that may never be needed. Single-tenant
operation cannot produce cross-tenant contamination.

**solution.** Leave the schema single-tenant. If multi-tenancy is
introduced, every table gains `tenant_id NOT NULL`, every unique
index becomes `(tenant_id, ...)`, and the handler reads the tenant
from the session. The rollout would be mechanical but contained, and
doing it speculatively now is waste.

---

## Q3. `OOPS_DEV_MODE=1` silently generates an ephemeral key

**Question.** Without `ENC_KEYS` the server refuses to start — unless
`OOPS_DEV_MODE=1`, in which case it generates an ephemeral AES key
from the string `"dev-only-not-for-production-use"`. The env var is
trivially set in a misconfigured container. Is the guardrail strong
enough?

**My understanding.** Strong enough for today's LAN deploy model, but
fragile. Anyone who sets the dev-mode flag in production will silently
encrypt real PII with a throwaway key and lose it on restart.

**solution.** Keep the current behavior but emit a `WARNING` log on
every startup where `OOPS_DEV_MODE=1` is observed, and also on every
write path that encrypts with version 1 of the fallback key (by
comparing the derived bytes). A future release adds a second required
variable (`OOPS_DEV_MODE_CONFIRM=yes-i-understand-this-is-not-production`)
so an operator has to re-affirm the choice on each deploy.

---

## Q4. `X-Workstation-Time` is trusted verbatim for the audit log

**Question.** The audit log records an operator-supplied
`X-Workstation-Time` value alongside the server's `At` timestamp. The
client can send any RFC3339 value, including one far in the past or
future. Do we validate skew?

**My understanding.** No skew validation today. The field is a forensic
signal, not an authorization input — server `At` is the canonical
ordering — so a lying workstation can lie to itself in the log but
cannot alter server semantics.

**solution.** Keep the current model. If an investigation ever needs
tighter correlation, add an admin tool that flags audit rows whose
`workstation_time` differs from `at` by more than a configured
tolerance (e.g., 5 minutes) so outliers surface. Rejecting skewed
values at write time would drop legitimate audit entries from
workstations whose clocks were briefly wrong.

---

## Q5. Lockout increments failures for unknown usernames

**Question.** When a login attempt is made against an unknown
username, `Lockout.RecordFailure` still increments a counter keyed by
that username. This blocks enumeration via timing, but it also lets
an attacker pre-emptively "lock out" any username by failing five
logins against it. Is that the right trade?

**My understanding.** Yes for a closed LAN portal. The attack requires
network access, and the mitigation (a 15-minute window) is
proportionate to the exposure. The alternative — not incrementing on
unknown usernames — leaks the valid-username set via differential
latency.

**solution.** Keep the current behavior. Document it explicitly in
the security summary. A future release could tie lockout escalation
to source IP (shorter window for trusted LAN ranges, longer for
unknown), but that requires a reverse-proxy contract we don't have
today.

---

## Q6. Advanced-filter broad-export threshold is hard-coded at 100

**Question.** `filter.Validate` rejects filter payloads with
`size > 100` unless they also carry a narrowing clause
(keyword/status/tag/date/price). The threshold is a literal. What is
the right number for an analyst pulling a report?

**My understanding.** 100 is a sensible UI default — it fits one
screen of the Advanced Filters table — but a policy value, not a
hard law. An analyst pulling a month's worth of orders could
reasonably ask for 500.

**solution.** Keep the default at 100. If analysts hit the cap
regularly, expose it as a per-role admin setting (permissions are
already admin-configurable; a second admin dictionary for "filter
limits per role" follows the same pattern). Until that need is
observed, the 500-row hard cap on `size` and the `ErrTooBroad`
rejection above 100 with no narrowing are the right defaults.

---

## Q7. `ReplaceWithCorrection` inserts-then-updates instead of using a deferred FK

**Question.** The Postgres `ReplaceWithCorrection` transaction inserts
the new report row first (so the old row's `superseded_by_id` FK
target exists), then updates the old row's status and FK. An
alternative would be a `DEFERRABLE INITIALLY DEFERRED` FK that lets
us do the old → new update in either order. Which is cleaner?

**My understanding.** The current insert-then-update is fine. It runs
inside a single transaction, so a partial-write window is not visible
to readers, and it avoids a non-default FK mode that other admins
might miss during schema review.

**solution.** Keep insert-then-update. Document the ordering in the
method comment so a future refactor doesn't accidentally swap the
statements. If we ever need to correct a correction in a nested
transaction, revisit the deferred-FK approach at that point.

---

## Q8. Route table and reference ranges are reloaded in-process

**Question.** Admin edits to the route table and reference-range
dictionary call `ReloadRouteTable` / `ReloadRefRanges` on the
*current* server. A second server (if one existed) would keep serving
stale data until it was restarted. Does that matter?

**My understanding.** Not today. The deployment is a single box; a
second process would share the same `store.Store` but would still
need to be told to reload. We don't run clusters.

**solution.** Keep the in-process reload. If multi-process is ever
introduced, add a `LISTEN`/`NOTIFY` channel on the `route_distances`
and `reference_ranges` tables, and every server process subscribes
and reloads on notification. For now, a single-server reload is
sufficient.

---

## Q9. Seed admin password is taken from `ADMIN_PASSWORD`, not rotated on first login

**Question.** `cmd/seed` refuses to run without `ADMIN_PASSWORD`, so
there is no hard-coded default. But the admin can continue using that
bootstrap password indefinitely; we never force a rotation on first
login. Is that right for a production-grade portal?

**My understanding.** Forced rotation is a common control but adds a
first-login UX branch (change-password screen, validation, audit
entry) that isn't in scope today. The seed password is operator-chosen
and lives only as a hash; the risk window is small if operators
rotate manually shortly after setup.

**solution.** Document "rotate the admin password after first login"
in the README seed section (already present). In a later release,
add a `must_rotate_password` boolean on the `users` row with a
first-login redirect to a change-password endpoint, and set it on
seed.

---

## Q10. The offline map is a single flat SVG

**Question.** `components/OfflineMap.tsx` computes a bounding box over
every configured region and projects into a flat 1000×600 SVG. That
works for one contiguous service territory; it degrades for a region
set that's geographically disjoint (e.g., a hub city plus a distant
satellite). Is contiguous the assumed shape?

**My understanding.** Yes for this release. Regional lab + fulfillment
operators typically draw a single contiguous service area with
carve-outs. Disjoint service areas would need a per-region zoom
control that we haven't scoped.

**solution.** Keep the bounding-box projection. If operators ever
ask for disjoint regions, split the map into tabs (one SVG per
bounding cluster), or add a zoom-and-pan control. Both are additive
UI work; they don't change the backend contract.

---

## Q11. Recent-search history is keyed by user id, not session

**Question.** `useRecentSearches` stores the last 20 searches under a
localStorage key like `oops.recent-searches.u1`. If user `u1` signs
out and user `u2` signs in on the same browser, `u2`'s history is
isolated. But two sibling tabs acting as the same user share the
history, including races where both mutate the array. Is that okay?

**My understanding.** Yes. The product requirement is "last 20 searches
per user on the device". Two-tab races on the same user produce a
best-effort merge (whoever writes last wins) and that is the spirit
of a personal history.

**solution.** Keep the current implementation. If two-tab writes
ever produce visible lost updates, move to a `BroadcastChannel` that
re-reads from localStorage on every write from a sibling tab.

---

## Q12. Report archive is intentionally one-way

**Question.** `lab.Archive` sets `ArchivedAt` but there is no
`Unarchive`. Is that deliberate, or is the symmetric action still to
come?

**My understanding.** Deliberate. Archive is a retention-management
signal ("this report is past its active window"); toggling it back
would break the invariant that `archived_at` reflects a one-way
decision. A mis-archive can be corrected by issuing a correction
(which creates a new `version+1` row that is not archived).

**solution.** Keep archive one-way. Document in the endpoint
description. If operators accumulate mis-archive errors, add a
dedicated admin-only endpoint `/api/admin/reports/:id/unarchive`
that re-sets `archived_at = NULL`, audit-logged with the reason, and
available only to `admin` role.

---

## Q13. Inventory signals are explicit; we have no true inventory system

**Question.** The automatic out-of-stock detector reads
`Order.Items[*].Backordered`. That field is flipped by a manual
`POST /api/orders/:id/inventory` call. There is no SKU-level
inventory, no receipt-triggered release, and no replenishment
cadence. Is that the intended model?

**My understanding.** Yes. This build models orders with line items
for workflow purposes, not a warehouse. Real inventory is upstream of
this portal. Operators flag backordered items explicitly when they
discover a shortage during picking.

**solution.** Keep the explicit-signal model. If a future release
integrates with a warehouse system, add an `InventoryProvider`
interface with a `StockLevel(sku)` method and automate the
`Backordered` flag when stock crosses the order quantity. The
current explicit endpoint remains as a manual override for edge
cases.

---

## Q14. Advanced-filter sort is allowlisted per entity

**Question.** `filter.Validate` restricts `sort_by` to a
hard-coded allowlist per entity (e.g., orders can sort by
`placed_at`, `status`, `total_cents`, `priority`). Analysts cannot
sort by an arbitrary column. Is that too restrictive?

**My understanding.** No. The allowlist forestalls injection into
dynamic ORDER BY clauses, and it also steers callers away from
unindexed columns that would produce slow queries at scale. Adding a
sort key means adding an index and an entry to the list.

**solution.** Keep the allowlist. Sort keys that operators ask for
(e.g., `customer_id`) get added along with the index that makes them
fast; neither happens without the other.

---

## Q15. Dispatch pin validation and fee quote surface outcomes differently

**Question.** `POST /api/dispatch/validate-pin` returns
`200 {valid: false, reason: "..."}` for a pin outside the service
area, while `POST /api/dispatch/fee-quote` returns
`422 Unprocessable Entity` for the same condition. The inconsistency
is visible to the UI.

**My understanding.** The two endpoints have different audiences.
`validate-pin` is informational (the UI wants to draw the pin red);
`fee-quote` is a refusal (the UI cannot display a fee for an invalid
destination). 200+flag and 422 both have their merits; the difference
is intentional even if surprising at first read.

**solution.** Keep the current behavior. Document in `apispec.md`
that `validate-pin` always returns 200 with an outcome field, while
`fee-quote` returns 422 for out-of-area destinations. The SPA already
handles both correctly.

---

## Q16. Correction reason is a single free-form field

**Question.** `POST /api/reports/:id/correct` requires a `reason`
string, stored verbatim on the new version's `reason_note`. There is
no category (typo / clinical-change / range-update), no free-text
length limit, no structured before/after diff beyond the snapshot
already held in the audit log. Is the single field sufficient?

**My understanding.** Yes for this release. A laboratory regulator
typically asks for a human-readable reason, and free text is what
technicians produce naturally. Over-structuring ("pick one of:
typo / re-run / …") produces data that is only as accurate as the
picker's attention.

**solution.** Keep the free-text reason. Add a server-side length
cap (e.g., 512 characters) to prevent pathological submissions and
document it in the endpoint spec. If a regulator later requires
categorized reasons, a CategoryEnum column alongside the free text
is an additive migration.

---

## Q17. No CSV export for analysts

**Question.** Analysts can filter and paginate orders, samples, and
reports, but they cannot export a result set to CSV. Is that a gap
or an intentional offline-safety choice?

**My understanding.** Intentional. CSV export is the most common path
to an accidental data exfiltration on an air-gapped system; the
product explicitly gates broad queries via `ErrTooBroad` so nobody
can dump the dataset. Any export feature needs matching
limits, audit coverage, and role gating.

**solution.** Defer CSV export. When added, it goes behind a
dedicated `analytics.export` permission, is audit-logged with the
full filter payload, inherits the `ErrTooBroad` guard, and is
rate-limited at the handler (one export per minute per user).

---

## Q18. Full-text search uses the `simple` tsvector configuration

**Question.** The `customers.search_tsv` and `reports.search_tsv`
GENERATED columns use `to_tsvector('simple', …)`. That skips
language-specific stemming. A search for "urinalysis" will not match
"urinalyses". Is `simple` the right choice?

**My understanding.** Yes as a default. A lab or fulfillment shop
typing exact test codes, names, and ZIPs benefits from literal
tokenization. English stemming can over-match (the "billing"/"bill"
collision is classically frustrating in lab note search).

**solution.** Keep `simple`. Per-installation tuning (e.g., switch to
`english` for the narrative column but keep `simple` for titles) is
a migration away. Add a short note in the migration file explaining
the decision so future readers don't assume it was arbitrary.

---

## Q19. The faulty-store tests rely on hand-typed method overrides

**Question.** `internal/api/error_paths_test.go` wraps the memory
store and overrides selected methods to return a sentinel error, so
we can exercise every handler's 500 branch. Adding a new store
method requires remembering to add an override. Is that fragile?

**My understanding.** Yes, mildly. Go has no native mocking, and
every `gomock`-style solution introduces its own build-time cost.
The current approach favors test clarity: a reader of the test file
can see exactly which method is being subverted.

**solution.** Keep the hand-typed overrides. When the number of
store methods grows (e.g., if we add a second store aggregate for
e-commerce), migrate to `mockery` or hand-written `StoreMock` with a
map of method → error and a single wrapper per method. For today's
surface, clarity beats automation.

---

## Q20. First clean `go mod tidy` requires network access

**Question.** The backend pins Echo, lib/pq, and golang.org/x/crypto
in `go.mod`. A first clean build on an air-gapped machine cannot
fetch them from `proxy.golang.org`. Air-gapped CI environments are
not a current constraint, but should we vendor?

**My understanding.** Not today. Vendoring adds ~10 MB to the repo,
and every Go toolchain the operator might run already has module
cache support. An offline first-build is still possible by copying a
populated `$GOMODCACHE` from a connected machine.

**solution.** Keep modules unvendored. Commit `go.sum` to pin exact
hashes so the fetch is deterministic. If air-gapped CI appears as a
requirement, run `go mod vendor` and check in `vendor/` behind a
dedicated PR; the switch is a build-flag flip (`-mod=vendor`) away.

---

*End of questions document. Items without explicit answers will be
implemented under the stated assumption.*
