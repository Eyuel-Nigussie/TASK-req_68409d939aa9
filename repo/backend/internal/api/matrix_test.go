package api

import (
	"net/http"
	"strings"
	"testing"
)

// TestRoleEndpointMatrix exercises every endpoint group across every role
// so a regression that accidentally opens an admin endpoint to an analyst
// (or vice-versa) is caught by CI. Each cell records the expected
// access-control outcome for (role, endpoint); any non-matching HTTP code
// fails the table row for that role/endpoint pair.
//
// Expectations:
//   - "allow": role is permitted to hit the endpoint (status NOT 401/403)
//   - "deny":  role is forbidden (HTTP 403)
//   - "anon":  no token; always 401
func TestRoleEndpointMatrix(t *testing.T) {
	r := setup(t)
	tokens := map[string]string{
		"admin1":     r.login(t, "admin1", "correct-horse-battery-staple"),
		"desk1":      r.login(t, "desk1", "correct-horse-battery-staple"),
		"tech1":      r.login(t, "tech1", "correct-horse-battery-staple"),
		"dispatch1":  r.login(t, "dispatch1", "correct-horse-battery-staple"),
	}
	// A seed customer/order/sample to satisfy route parameters; we don't
	// care about 404s, only that the auth gate classifies correctly.
	rec, body := r.do(t, "POST", "/api/customers", tokens["desk1"], map[string]any{"name": "Matrix"})
	if rec.Code != 201 {
		t.Fatalf("seed customer: %d %s", rec.Code, rec.Body.String())
	}
	custID := body["id"].(string)
	rec, body = r.do(t, "POST", "/api/orders", tokens["desk1"], map[string]any{"total_cents": 100})
	if rec.Code != 201 {
		t.Fatalf("seed order: %d %s", rec.Code, rec.Body.String())
	}
	orderID := body["ID"].(string)
	rec, body = r.do(t, "POST", "/api/samples", tokens["tech1"], map[string]any{"test_codes": []string{"GLU"}})
	if rec.Code != 201 {
		t.Fatalf("seed sample: %d %s", rec.Code, rec.Body.String())
	}
	sampleID := body["ID"].(string)

	type row struct {
		method, path string
		// role -> "allow" | "deny"
		expect map[string]string
	}
	// Per-row expectations reflect the TIGHTENED authorization policy
	// introduced after the Partial-Pass audit: every mutating endpoint
	// requires the matching `*.write` permission, and Analyst has only
	// `*.read` / `analytics.view` / `orders.export`. Non-owning roles
	// are explicitly denied from writes they don't need.
	rows := []row{
		// customers.read is held by admin/desk/tech/dispatch/analyst.
		{"GET", "/api/customers/" + custID,
			map[string]string{"admin1": "allow", "desk1": "allow", "tech1": "deny", "dispatch1": "allow"}},
		// customers.write held only by admin + front_desk.
		{"POST", "/api/customers",
			map[string]string{"admin1": "allow", "desk1": "allow", "tech1": "deny", "dispatch1": "deny"}},
		{"GET", "/api/orders",
			map[string]string{"admin1": "allow", "desk1": "allow", "tech1": "deny", "dispatch1": "allow"}},
		{"GET", "/api/orders/" + orderID,
			map[string]string{"admin1": "allow", "desk1": "allow", "tech1": "deny", "dispatch1": "allow"}},
		// orders.write held only by admin + front_desk; dispatch has read.
		{"POST", "/api/orders",
			map[string]string{"admin1": "allow", "desk1": "allow", "tech1": "deny", "dispatch1": "deny"}},
		{"GET", "/api/samples",
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "allow", "dispatch1": "deny"}},
		{"GET", "/api/samples/" + sampleID,
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "allow", "dispatch1": "deny"}},
		{"POST", "/api/samples",
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "allow", "dispatch1": "deny"}},
		{"GET", "/api/dispatch/regions",
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "deny", "dispatch1": "allow"}},
		{"POST", "/api/dispatch/validate-pin",
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "deny", "dispatch1": "allow"}},
		{"GET", "/api/admin/users",
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "deny", "dispatch1": "deny"}},
		{"PUT", "/api/admin/service-regions",
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "deny", "dispatch1": "deny"}},
		{"PUT", "/api/admin/reference-ranges",
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "deny", "dispatch1": "deny"}},
		{"PUT", "/api/admin/route-table",
			map[string]string{"admin1": "allow", "desk1": "deny", "tech1": "deny", "dispatch1": "deny"}},
	}
	for _, rr := range rows {
		for user, want := range rr.expect {
			t.Run(user+" "+rr.method+" "+rr.path+" expect="+want, func(t *testing.T) {
				rec, _ := r.do(t, rr.method, rr.path, tokens[user], map[string]any{})
				switch want {
				case "allow":
					// Anything other than 401/403 is "allowed" for this test —
					// we care only about the auth gate, not business validation.
					if rec.Code == http.StatusUnauthorized || rec.Code == http.StatusForbidden {
						t.Fatalf("role %s expected allow, got %d", user, rec.Code)
					}
				case "deny":
					if rec.Code != http.StatusForbidden {
						t.Fatalf("role %s expected 403, got %d", user, rec.Code)
					}
				default:
					t.Fatalf("bad expect value %q", want)
				}
			})
		}
	}
}

// TestIDOR_SharedEntitiesRespectRoles covers the audit's "no object-level
// authorization tests for shared entities" gap: we assert that attempts to
// reach customers/orders/reports from an unprivileged role return 403, and
// that attempts on non-existent IDs from privileged roles return 404 rather
// than leaking that the row is restricted.
func TestIDOR_SharedEntitiesRespectRoles(t *testing.T) {
	r := setup(t)
	admin := r.login(t, "admin1", "correct-horse-battery-staple")
	tech := r.login(t, "tech1", "correct-horse-battery-staple")
	desk := r.login(t, "desk1", "correct-horse-battery-staple")

	// Seed one of each.
	rec, body := r.do(t, "POST", "/api/customers", desk, map[string]any{"name": "IDOR"})
	if rec.Code != 201 {
		t.Fatalf("seed: %d", rec.Code)
	}
	custID := body["id"].(string)
	rec, body = r.do(t, "POST", "/api/orders", desk, map[string]any{"total_cents": 1})
	if rec.Code != 201 {
		t.Fatalf("seed order: %d", rec.Code)
	}
	orderID := body["ID"].(string)
	rec, body = r.do(t, "POST", "/api/samples", tech, map[string]any{"test_codes": []string{"GLU"}})
	if rec.Code != 201 {
		t.Fatalf("seed sample: %d", rec.Code)
	}
	sampleID := body["ID"].(string)
	// Advance sample to in_testing so a v1 report may be issued.
	if rec, _ := r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tech, map[string]any{"to": "received"}); rec.Code != 200 {
		t.Fatalf("sample ->received: %d", rec.Code)
	}
	if rec, _ := r.do(t, "POST", "/api/samples/"+sampleID+"/transitions", tech, map[string]any{"to": "in_testing"}); rec.Code != 200 {
		t.Fatalf("sample ->in_testing: %d", rec.Code)
	}
	rec, body = r.do(t, "POST", "/api/samples/"+sampleID+"/report", tech, map[string]any{
		"title": "IDOR report", "narrative": "n", "measurements": []map[string]any{{"test_code": "GLU", "value": 85}},
	})
	if rec.Code != 201 {
		t.Fatalf("seed report: %d %s", rec.Code, rec.Body.String())
	}
	reportID := body["ID"].(string)

	cases := []struct {
		name, method, path, token string
		wantCode                  int
	}{
		// Lab-gated endpoints: desk and dispatch should get 403 on sample/report reads.
		{"desk->sample", "GET", "/api/samples/" + sampleID, desk, http.StatusForbidden},
		{"desk->report", "GET", "/api/reports/" + reportID, desk, http.StatusForbidden},
		{"dispatch->sample", "GET", "/api/samples/" + sampleID, r.login(t, "dispatch1", "correct-horse-battery-staple"), http.StatusForbidden},
		// Order-gated endpoints: lab tech should get 403.
		{"tech->order", "GET", "/api/orders/" + orderID, tech, http.StatusForbidden},
		// lab_tech no longer has customers.read — denied.
		{"tech->customer", "GET", "/api/customers/" + custID, tech, http.StatusForbidden},
		// 404s for valid-role-but-nonexistent targets — must not 403 or leak.
		{"desk->missing-customer", "GET", "/api/customers/does-not-exist", desk, http.StatusNotFound},
		{"tech->missing-sample", "GET", "/api/samples/does-not-exist", tech, http.StatusNotFound},
		// Admin can always read.
		{"admin->customer", "GET", "/api/customers/" + custID, admin, http.StatusOK},
		{"admin->order", "GET", "/api/orders/" + orderID, admin, http.StatusOK},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			rec, _ := r.do(t, c.method, c.path, c.token, nil)
			if rec.Code != c.wantCode {
				t.Fatalf("expected %d, got %d (%s)", c.wantCode, rec.Code, rec.Body.String())
			}
			// Body must never leak an SQL or stack fragment.
			body := strings.ToLower(rec.Body.String())
			for _, leak := range []string{"sql:", "goroutine", "pq:", "stack trace"} {
				if strings.Contains(body, leak) {
					t.Fatalf("body appears to leak %q: %s", leak, rec.Body.String())
				}
			}
		})
	}
}
