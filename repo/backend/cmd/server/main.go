// Command server is the HTTP entry point for the Unified Offline
// Operations Portal. Wires configuration, the store backend (Postgres),
// and the Echo router, then serves on the configured address. The main
// function is a thin shell — testable bootstrap logic lives in
// internal/runtime so it can be covered without running a real server.
package main

import (
	"context"
	"database/sql"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/eaglepoint/oops/backend/internal/api"
	"github.com/eaglepoint/oops/backend/internal/runtime"
	"github.com/eaglepoint/oops/backend/internal/store"
	"github.com/labstack/echo/v4"
	_ "github.com/lib/pq"
)

func main() {
	addr := runtime.EnvOr("LISTEN_ADDR", ":8080")
	dsn := runtime.EnvOr("DATABASE_URL", "")

	vault, err := runtime.BuildVault(nil, nil)
	if err != nil {
		log.Fatalf("vault: %v", err)
	}

	var s store.Store
	if dsn == "" {
		log.Println("DATABASE_URL not set; using in-memory store (data lost on restart)")
		s = store.NewMemory()
	} else {
		db, err := sql.Open("postgres", dsn)
		if err != nil {
			log.Fatalf("open db: %v", err)
		}
		db.SetMaxOpenConns(25)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(time.Hour)
		if err := db.PingContext(context.Background()); err != nil {
			log.Fatalf("ping db: %v", err)
		}
		s = store.NewPostgres(db)
	}

	srv := api.New(s, vault, nil)

	e := echo.New()
	e.HideBanner = true
	srv.Register(e)

	go func() {
		if err := e.Start(addr); err != nil {
			log.Printf("server exited: %v", err)
		}
	}()
	log.Printf("Unified Offline Operations Portal listening on %s", addr)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)
	<-sig
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}
