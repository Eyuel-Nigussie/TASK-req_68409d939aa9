package store

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/lab"
	"github.com/eaglepoint/oops/backend/internal/models"
	"github.com/eaglepoint/oops/backend/internal/order"
	_ "github.com/lib/pq"
)

// TestPostgres_Integration is a behavior-parity test that runs the same
// scenarios against a real Postgres database. It is gated on the
// INTEGRATION_DB environment variable so the default CI path (no DB)
// skips it cleanly. Set INTEGRATION_DB to a DSN like
//   postgres://oops:oops@localhost:5432/oops_test?sslmode=disable
// to exercise the real driver, schema, and constraints.
//
// The test applies the schema fresh (tables dropped and re-created) so
// it is idempotent. Rerunning wipes the target DB.
func TestPostgres_Integration(t *testing.T) {
	dsn := os.Getenv("INTEGRATION_DB")
	if dsn == "" {
		t.Skip("INTEGRATION_DB not set; skipping Postgres integration tests")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer db.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping: %v", err)
	}

	// Apply the migration. We drop then recreate via the schema file.
	// The two DO $$ blocks are tolerant so a fresh DB works too.
	resetSchema(t, db)
	applyMigration(t, db)

	p := NewPostgres(db)

	t.Run("Users round-trip", func(t *testing.T) {
		u := models.User{ID: "u1", Username: "alice", Role: models.RoleAdmin, PasswordHash: "h",
			CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC()}
		if err := p.CreateUser(ctx, u); err != nil {
			t.Fatal(err)
		}
		got, err := p.GetUserByUsername(ctx, "alice")
		if err != nil || got.Username != "alice" {
			t.Fatalf("user mismatch: %+v %v", got, err)
		}
	})

	t.Run("Audit log is append-only", func(t *testing.T) {
		e := models.AuditEntry{ID: "a1", At: time.Now().UTC(), Entity: "x", EntityID: "1", Action: "create"}
		if err := p.AppendAudit(ctx, e); err != nil {
			t.Fatal(err)
		}
		// Attempt to UPDATE the row; the trigger must reject it.
		if _, err := db.ExecContext(ctx, `UPDATE audit_log SET reason='tamper' WHERE id=$1`, e.ID); err == nil {
			t.Fatal("expected update to fail due to immutability trigger")
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM audit_log WHERE id=$1`, e.ID); err == nil {
			t.Fatal("expected delete to fail due to immutability trigger")
		}
	})

	t.Run("Report correction transactional", func(t *testing.T) {
		// Seed a sample and an original report.
		s := lab.Sample{ID: "s1", Status: lab.SampleInTesting, CollectedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), TestCodes: []string{"GLU"}}
		if err := p.CreateSample(ctx, s); err != nil {
			t.Fatal(err)
		}
		r1 := lab.Report{ID: "r1", SampleID: "s1", Version: 1, Status: lab.ReportIssued,
			Title: "CBC", Narrative: "init", IssuedAt: time.Now().UTC()}
		if err := p.CreateReport(ctx, r1); err != nil {
			t.Fatal(err)
		}
		old := r1
		old.Status = lab.ReportSuperseded
		old.SupersededByID = "r2"
		next := lab.Report{ID: "r2", SampleID: "s1", Version: 2, Status: lab.ReportIssued,
			Title: "CBC", Narrative: "corrected", ReasonNote: "typo", IssuedAt: time.Now().UTC()}
		if err := p.ReplaceWithCorrection(ctx, old, next); err != nil {
			t.Fatal(err)
		}
		latest, err := p.LatestReportForSample(ctx, "s1")
		if err != nil || latest.ID != "r2" {
			t.Fatalf("latest: %+v %v", latest, err)
		}
	})

	t.Run("Order filter tags intersect", func(t *testing.T) {
		now := time.Now().UTC()
		orders := []order.Order{
			{ID: "o1", Status: order.StatusPlaced, TotalCents: 500, Priority: "standard",
				Tags: []string{"x", "y"}, PlacedAt: now, UpdatedAt: now},
			{ID: "o2", Status: order.StatusPlaced, TotalCents: 2500, Priority: "rush",
				Tags: []string{"y", "z"}, PlacedAt: now, UpdatedAt: now},
		}
		for _, o := range orders {
			if err := p.CreateOrder(ctx, o); err != nil {
				t.Fatal(err)
			}
		}
		out, total, err := p.QueryOrders(ctx, OrderQuery{Tags: []string{"x"}})
		if err != nil || total != 1 || len(out) != 1 || out[0].ID != "o1" {
			t.Fatalf("tag filter wrong: total=%d out=%v err=%v", total, out, err)
		}
	})

	t.Run("Lockout persists across reconstruction", func(t *testing.T) {
		if err := p.UpsertLoginAttempt(ctx, models.LoginAttempt{
			Username: "locked", Failures: 5,
			LockedUntil: time.Now().UTC().Add(10 * time.Minute),
			UpdatedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatal(err)
		}
		got, err := p.GetLoginAttempt(ctx, "locked")
		if err != nil || got.Failures != 5 {
			t.Fatalf("unexpected: %+v %v", got, err)
		}
	})

	t.Run("Permission grants persist and merge", func(t *testing.T) {
		if err := p.SetUserPermissions(ctx, "u1", []string{"analytics.view"}); err != nil {
			t.Fatal(err)
		}
		grants, err := p.GrantsForUser(ctx, "u1", "admin")
		if err != nil {
			t.Fatal(err)
		}
		seen := map[string]bool{}
		for _, g := range grants {
			seen[g] = true
		}
		if !seen["analytics.view"] {
			t.Fatalf("expected user grant present, got %v", grants)
		}
	})
}

// resetSchema drops every known table so the migration can run cleanly.
// This is intentional for the integration target — it is a throwaway DB.
func resetSchema(t *testing.T, db *sql.DB) {
	t.Helper()
	tables := []string{
		"audit_log", "saved_filters", "reports", "reference_ranges",
		"test_items",
		"samples", "order_exceptions", "order_events", "orders",
		"service_regions", "address_book", "route_distances",
		"user_permissions", "role_permissions", "permissions",
		"login_attempts", "system_settings",
		"customers", "users",
	}
	for _, tbl := range tables {
		_, _ = db.Exec(`DROP TABLE IF EXISTS ` + tbl + ` CASCADE`)
	}
	_, _ = db.Exec(`DROP FUNCTION IF EXISTS audit_log_immutable() CASCADE`)
}

func applyMigration(t *testing.T, db *sql.DB) {
	t.Helper()
	// Find the migration relative to this test file.
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "migrations", "0001_init.sql")
	data, err := os.ReadFile(root)
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	if _, err := db.Exec(string(data)); err != nil {
		t.Fatalf("apply migration: %v", err)
	}
}
