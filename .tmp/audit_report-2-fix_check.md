# Audit Fix Check — audit_report-2

Source audit reviewed: `.tmp/audit_report-2.md`  
Checked against current code in: `repo/`

## Summary
- Items checked: 7 (Blocker/High/Medium findings from the audit)
- Addressed: 7
- Partially addressed: 0
- Not addressed: 0

## One-by-one Result

1. **Release authentication path unusable on fresh install (Blocker)**  
Status: **Addressed**  
Evidence:
- First-run administrator enrollment path exists in login UI (`Create Administrator Account`) and is shown only when no credentials exist: `repo/Sources/RailCommerceApp/LoginViewController.swift:107`, `:124`, `:186`.
- Credential presence check API used for bootstrap gating: `repo/Sources/RailCommerce/Core/CredentialStore.swift` (`hasAnyCredentials`), exercised by closure tests.
- Regression coverage: `repo/Tests/RailCommerceTests/AuditV7ClosureTests.swift:16` and `repo/Tests/RailCommerceTests/AuditV7ExtendedCoverageTests.swift:486`.

2. **Cross-user data isolation gap: addresses are global and not user-scoped (High)**  
Status: **Addressed**  
Evidence:
- `USAddress` now carries ownership (`ownerUserId`) and `AddressBook` exposes scoped APIs: `repo/Sources/RailCommerce/Models/Address.swift:18`, `:122`, `:150`.
- Checkout UI now reads/writes/removes addresses scoped to signed-in user: `repo/Sources/RailCommerceApp/Views/CheckoutViewController.swift:64`, `:164`, `:194`, `:225`.
- Regression coverage: `repo/Tests/RailCommerceTests/AuditV7ClosureTests.swift:34`, `repo/Tests/RailCommerceTests/AuditV7ExtendedCoverageTests.swift:43`.

3. **Cross-user data isolation gap: cart persistence is global/shared (High)**  
Status: **Addressed**  
Evidence:
- Cart supports per-user persistence key (`cart.lines.<userId>`): `repo/Sources/RailCommerce/Services/Cart.swift:37`, `:60`.
- Container exposes user-scoped cart accessor: `repo/Sources/RailCommerce/RailCommerce.swift:97`.
- UI uses `cart(forUser:)` in browse/cart flows: `repo/Sources/RailCommerceApp/Views/BrowseViewController.swift:177`, `repo/Sources/RailCommerceApp/Views/CartViewController.swift:21`.
- Regression coverage: `repo/Tests/RailCommerceTests/AuditV7ClosureTests.swift:81`, `repo/Tests/RailCommerceTests/AuditV7ExtendedCoverageTests.swift:162`.

4. **Function-level authorization bypass in talent search API (High)**  
Status: **Addressed**  
Evidence:
- Public entry point enforces `.matchTalent`: `repo/Sources/RailCommerce/Services/TalentMatchingService.swift:175`.
- Unguarded search path is now internal-only (`searchUnchecked`): `repo/Sources/RailCommerce/Services/TalentMatchingService.swift:182`.
- Regression coverage: `repo/Tests/RailCommerceTests/AuditV7ClosureTests.swift:130`, `repo/Tests/RailCommerceTests/AuditV7ExtendedCoverageTests.swift:433`, `repo/Tests/RailCommerceTests/AuthorizationTests.swift:253`.

5. **Content rollback semantics break published visibility (High)**  
Status: **Addressed**  
Evidence:
- Rollback now preserves `.published` status when rolling back published items: `repo/Sources/RailCommerce/Services/ContentPublishingService.swift:327`, `:343`.
- Published-only listing still includes rolled-back published item by design (`items(...publishedOnly: true)` checks status): `repo/Sources/RailCommerce/Services/ContentPublishingService.swift:132`.
- Regression coverage: `repo/Tests/RailCommerceTests/AuditV7ClosureTests.swift:161`, `repo/Tests/RailCommerceTests/AuditV7ExtendedCoverageTests.swift:310`.

6. **Membership points API allows negative values (High)**  
Status: **Addressed**  
Evidence:
- Both accrual and redemption now enforce `points > 0` and throw `MembershipError.invalidPoints`: `repo/Sources/RailCommerce/Services/MembershipService.swift:108`, `:123`.
- Regression coverage for negative/zero values: `repo/Tests/RailCommerceTests/AuditV7ClosureTests.swift:201`, `:212`, `:223`.

7. **Persistence failures silently swallowed in critical mutators (Medium)**  
Status: **Addressed**  
Evidence:
- Address writes now rollback in-memory state and throw `AddressBookError.persistenceFailed`: `repo/Sources/RailCommerce/Models/Address.swift:110`, `:114`.
- Seat reserve/release/confirm rollback on persistence failure and throw: `repo/Sources/RailCommerce/Services/SeatInventoryService.swift:147`, `:189`, `:216`.
- After-sales automation now rolls back in-memory transition on persist failure and logs explicit failure: `repo/Sources/RailCommerce/Services/AfterSalesService.swift:410`, `:416`, `:437`.
- Regression coverage: `repo/Tests/RailCommerceTests/AuditV7ClosureTests.swift:251`, `:272`, `:293`, `repo/Tests/RailCommerceTests/AuditV7ExtendedCoverageTests.swift:400`.

## Verification Runs
Executed tests:
- `./run_tests.sh --filter AuditV7ClosureTests --filter AuditV7ExtendedCoverageTests`

Observed result:
- Test runner executed the full suite in this environment.
- **559 tests passed, 0 failed**.
