# Audit Fix Check — audit_report-1

Source audit reviewed: `.tmp/audit_report-1.md`  
Checked against current code in: `repo/`  
Verification date: **2026-04-17**

## Summary
- Items checked: 6 (Blocker/High/Medium issues from the audit)
- Addressed: 6
- Partially addressed: 0
- Not addressed: 0

## One-by-one Result

1. **Missing `NSCameraUsageDescription` (Blocker)**  
Status: **Addressed**  
Evidence:
- `repo/Sources/RailCommerceApp/Info.plist:10-11` includes `NSCameraUsageDescription` with non-empty rationale text.
- Regression assertion exists: `repo/Tests/RailCommerceTests/AppConfigAssertionTests.swift:38-45`.

2. **Inbound P2P messages bypass safety controls (High)**  
Status: **Addressed**  
Evidence:
- Transport is wired to inbound validator in init: `repo/Sources/RailCommerce/Services/MessagingService.swift:137-140`.
- Inbound path now enforces block/sensitive/harassment/attachment/masking checks in `acceptInbound`: `repo/Sources/RailCommerce/Services/MessagingService.swift:374-435`.
- Dedicated regression coverage exists: `repo/Tests/RailCommerceTests/InboundMessagingValidationTests.swift:22-118`, `repo/Tests/RailCommerceTests/AuditReport1ClosureTests.swift:18-45`.

3. **Heavy-work gating may misclassify inactivity (High)**  
Status: **Addressed**  
Evidence:
- App-wide touch tracking window reports activity: `repo/Sources/RailCommerceApp/SystemProviders.swift:18-27`.
- `SystemBattery.recordActivity()` wired for touch + lifecycle notifications: `repo/Sources/RailCommerceApp/SystemProviders.swift:49-57,87-89`.
- App installs `ActivityTrackingWindow` and observer in launch path: `repo/Sources/RailCommerceApp/AppDelegate.swift:83-90`.

4. **After-sales closed-loop messaging not exposed in UI (Medium)**  
Status: **Addressed**  
Evidence:
- Request-row tap now opens case thread view: `repo/Sources/RailCommerceApp/Views/AfterSalesViewController.swift:261-267`.
- Thread screen reads/sends via after-sales APIs (`caseMessages`, `postCaseMessage`): `repo/Sources/RailCommerceApp/Views/AfterSalesCaseThreadViewController.swift:53-56,137-142`.

5. **Background task docs drift (`BGAppRefreshTask` vs `BGProcessingTask`) (Medium)**  
Status: **Addressed**  
Evidence:
- Docs now describe both tasks as `BGProcessingTask`: `docs/apispec.md:77-87`.
- App delegate registers publish + cleanup as `BGProcessingTask`: `repo/Sources/RailCommerceApp/AppDelegate.swift:186-203`.

6. **System keychain accessibility class not enforced (Medium)**  
Status: **Addressed**  
Evidence:
- Accessibility class constant is explicitly `kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly`: `repo/Sources/RailCommerceApp/SystemKeychain.swift:17-20`.
- `kSecAttrAccessible` is applied in add/update/seal paths: `repo/Sources/RailCommerceApp/SystemKeychain.swift:33-42,97-103`.

## Verification Runs
Executed targeted tests:
- `swift test --package-path repo --filter 'AppConfigAssertionTests|InboundMessagingValidationTests|AuditReport1ClosureTests'`

Observed result:
- **16 tests passed, 0 failed**.
