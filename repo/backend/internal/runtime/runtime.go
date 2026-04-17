// Package runtime contains the testable bootstrap helpers for the cmd
// binaries. The `main` functions under `cmd/*` are intentionally thin:
// they parse the environment, call helpers from this package, and block
// on os.Signal. Putting the logic here lets us exercise it with unit
// tests that cover the happy paths and every error branch.
package runtime

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/eaglepoint/oops/backend/internal/auth"
	"github.com/eaglepoint/oops/backend/internal/crypto"
	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/models"
)

// SeedStore is the minimal persistence surface required by SeedDeployment.
type SeedStore interface {
	GetUserByUsername(ctx context.Context, username string) (models.User, error)
	CreateUser(ctx context.Context, u models.User) error
	ReplaceRegions(ctx context.Context, regions []geo.Region) error
}

// SeedDeployment installs a default admin user (if absent) and replaces
// the service-region table with a demo downtown polygon. Errors from the
// store are returned so callers can choose to log or abort.
//
// `adminPassword` must satisfy the password policy; an empty value
// yields ErrMissingAdminPassword so operators don't accidentally ship a
// default credential.
func SeedDeployment(ctx context.Context, s SeedStore, adminPassword string, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	if _, err := s.GetUserByUsername(ctx, "admin"); err != nil {
		if adminPassword == "" {
			return ErrMissingAdminPassword
		}
		hash, herr := auth.HashPassword(adminPassword)
		if herr != nil {
			return fmt.Errorf("admin password rejected: %w", herr)
		}
		u := models.User{
			ID:           "u_admin",
			Username:     "admin",
			Role:         models.RoleAdmin,
			PasswordHash: hash,
			CreatedAt:    now(),
			UpdatedAt:    now(),
		}
		if err := s.CreateUser(ctx, u); err != nil {
			return fmt.Errorf("create admin: %w", err)
		}
	}
	region := geo.Region{
		Polygon: geo.Polygon{
			ID: "downtown",
			Vertices: []geo.Point{
				{Lat: 40.70, Lng: -74.02},
				{Lat: 40.70, Lng: -73.98},
				{Lat: 40.74, Lng: -73.98},
				{Lat: 40.74, Lng: -74.02},
			},
		},
		BaseFeeCents:    500,
		PerMileFeeCents: 25,
	}
	return s.ReplaceRegions(ctx, []geo.Region{region})
}

// ErrMissingAdminPassword is returned when SeedDeployment is called
// without an ADMIN_PASSWORD env var set.
var ErrMissingAdminPassword = errors.New("ADMIN_PASSWORD not set — refusing to seed an admin with a default password")

// DemoUser describes a user that can be seeded as part of a Docker /
// evaluator walk-through. Passwords must satisfy the ≥10-character
// policy enforced by auth.ValidatePolicy.
type DemoUser struct {
	Username string
	Role     models.Role
	Password string
}

// DefaultDemoUsers are the five role-covering accounts installed by
// SeedDemoUsers. They appear in the README's Seeded Credentials table.
func DefaultDemoUsers() []DemoUser {
	return []DemoUser{
		{Username: "admin", Role: models.RoleAdmin, Password: "AdminTest123!"},
		{Username: "frontdesk", Role: models.RoleFrontDesk, Password: "FrontDeskTest1!"},
		{Username: "labtech", Role: models.RoleLabTech, Password: "LabTechTest123!"},
		{Username: "dispatch", Role: models.RoleDispatch, Password: "DispatchTest1!"},
		{Username: "analyst", Role: models.RoleAnalyst, Password: "AnalystTest123!"},
	}
}

// SeedDemoUsers idempotently creates each entry in DefaultDemoUsers that
// does not already exist in the store. Existing accounts are left
// untouched; passwords are never rewritten. Safe to run on every
// container start.
func SeedDemoUsers(ctx context.Context, s SeedStore, now func() time.Time) error {
	return seedUsers(ctx, s, DefaultDemoUsers(), now)
}

// seedUsers is the reusable implementation behind SeedDemoUsers; it
// takes the user list as a parameter so tests can exercise the
// password-policy error branch without editing DefaultDemoUsers.
func seedUsers(ctx context.Context, s SeedStore, users []DemoUser, now func() time.Time) error {
	if now == nil {
		now = time.Now
	}
	for _, du := range users {
		if _, err := s.GetUserByUsername(ctx, du.Username); err == nil {
			continue
		}
		hash, err := auth.HashPassword(du.Password)
		if err != nil {
			return fmt.Errorf("hash %s: %w", du.Username, err)
		}
		u := models.User{
			ID:           "u_" + du.Username,
			Username:     du.Username,
			Role:         du.Role,
			PasswordHash: hash,
			CreatedAt:    now(),
			UpdatedAt:    now(),
		}
		if err := s.CreateUser(ctx, u); err != nil {
			return fmt.Errorf("create %s: %w", du.Username, err)
		}
	}
	return nil
}

// GenerateKeyLine returns a line in the format accepted by the ENC_KEYS
// environment variable for a fresh random 32-byte AES key. The `randRead`
// parameter is injectable so tests can exercise both the happy path and
// the rare "rand.Read failed" branch without waiting for entropy.
func GenerateKeyLine(version int, randRead func([]byte) (int, error)) (string, error) {
	if randRead == nil {
		randRead = rand.Read
	}
	b := make([]byte, 32)
	if _, err := randRead(b); err != nil {
		return "", fmt.Errorf("read random bytes: %w", err)
	}
	return fmt.Sprintf("%d:%s", version, hex.EncodeToString(b)), nil
}

// EnvOr returns the value of the named env var, falling back to def when
// unset or empty.
func EnvOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// SplitCSV tokenizes a comma-separated string, trimming surrounding
// whitespace and dropping empty tokens. Preserved behavior from the
// original inline implementation.
func SplitCSV(s string) []string {
	out := []string{}
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ErrMissingKeys is returned when ENC_KEYS is absent and the dev-mode
// escape hatch is not set.
var ErrMissingKeys = errors.New("ENC_KEYS is required; set OOPS_DEV_MODE=1 to permit an ephemeral key for local development only")

// ErrPlaceholderKey is returned when ENC_KEYS matches the historical
// demo value from .env.example. That hex string is published in the
// repo, so accepting it would make at-rest encryption trivially
// reversible by anyone who cloned the source.
var ErrPlaceholderKey = errors.New("ENC_KEYS uses the public placeholder value; run `go run ./backend/cmd/keygen` to generate a fresh key")

// placeholderKey is the single well-known demo key that previously
// shipped in .env.example. Compared as a literal so we catch stale
// copies that were generated before the default was removed.
const placeholderKey = "1:0101010101010101010101010101010101010101010101010101010101010101"

// BuildVault parses ENC_KEYS from the provided env-getter and returns a
// configured Vault. When `devMode` is true and keys are absent, an
// ephemeral key is issued with a console warning.
//
// `logf` and `getenv` are dependencies so tests can substitute a stub.
// Real callers pass os.Getenv / log.Printf.
func BuildVault(getenv func(string) string, logf func(string, ...any)) (*crypto.Vault, error) {
	if logf == nil {
		logf = log.Printf
	}
	if getenv == nil {
		getenv = os.Getenv
	}
	raw := getenv("ENC_KEYS")
	if raw == "" {
		if getenv("OOPS_DEV_MODE") != "1" {
			return nil, ErrMissingKeys
		}
		logf("WARNING: OOPS_DEV_MODE=1 and ENC_KEYS unset — generating an ephemeral development key. Data encrypted now will NOT be readable after restart.")
		return crypto.NewVault(map[uint16][]byte{1: crypto.DeriveKey([]byte("dev-only-not-for-production-use"))})
	}
	if strings.TrimSpace(raw) == placeholderKey {
		if getenv("OOPS_DEV_MODE") != "1" {
			return nil, ErrPlaceholderKey
		}
		logf("WARNING: OOPS_DEV_MODE=1 and ENC_KEYS is the published placeholder value. This is only safe for evaluators; rotate before real data is stored.")
	}
	keys := make(map[uint16][]byte)
	for _, pair := range SplitCSV(raw) {
		var ver uint16
		var hexStr string
		if _, err := fmt.Sscanf(pair, "%d:%s", &ver, &hexStr); err != nil {
			return nil, fmt.Errorf("malformed ENC_KEYS entry %q: %w", pair, err)
		}
		key, err := hex.DecodeString(hexStr)
		if err != nil {
			return nil, fmt.Errorf("decode key v%d: %w", ver, err)
		}
		keys[ver] = key
	}
	return crypto.NewVault(keys)
}
