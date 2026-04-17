// Command seed bootstraps a fresh deployment with one admin user and a
// simple service region so the UI is immediately usable. When
// SEED_DEMO_USERS=1 it additionally installs the five role-covering
// accounts documented in the top-level README. Safe to run repeatedly;
// it skips rows that already exist. The heavy lifting lives in
// internal/runtime so it is fully test-covered.
package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"

	"github.com/eaglepoint/oops/backend/internal/runtime"
	"github.com/eaglepoint/oops/backend/internal/store"
	_ "github.com/lib/pq"
)

func main() {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		log.Fatal("DATABASE_URL required")
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		log.Fatalf("open db: %v", err)
	}
	if err := db.Ping(); err != nil {
		log.Fatalf("ping: %v", err)
	}
	s := store.NewPostgres(db)
	ctx := context.Background()

	adminPW := os.Getenv("ADMIN_PASSWORD")
	if err := runtime.SeedDeployment(ctx, s, adminPW, nil); err != nil {
		// When SEED_DEMO_USERS=1 and no ADMIN_PASSWORD is set, fall back
		// to the default demo-user seed which already includes an admin
		// with a known password. This is the normal Docker dev path.
		if !errors.Is(err, runtime.ErrMissingAdminPassword) || os.Getenv("SEED_DEMO_USERS") != "1" {
			log.Fatalf("seed: %v", err)
		}
	}
	if os.Getenv("SEED_DEMO_USERS") == "1" {
		if err := runtime.SeedDemoUsers(ctx, s, nil); err != nil {
			log.Fatalf("seed demo users: %v", err)
		}
		log.Println("seeded demo users (admin, frontdesk, labtech, dispatch, analyst) + demo service region")
	} else {
		log.Println("seeded admin user + demo service region")
	}
}
