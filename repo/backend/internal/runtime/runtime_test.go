package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/geo"
	"github.com/eaglepoint/oops/backend/internal/models"
)

func TestEnvOr(t *testing.T) {
	t.Setenv("OOPS_TEST_KEY", "hello")
	if EnvOr("OOPS_TEST_KEY", "def") != "hello" {
		t.Fatal("env override not used")
	}
	t.Setenv("OOPS_TEST_KEY", "")
	if EnvOr("OOPS_TEST_KEY", "def") != "def" {
		t.Fatal("default not used when unset")
	}
}

func TestSplitCSV(t *testing.T) {
	cases := map[string][]string{
		"":            {},
		"a":           {"a"},
		"a,b":         {"a", "b"},
		"a, b ,  ,c": {"a", "b", "c"},
	}
	for in, want := range cases {
		got := SplitCSV(in)
		if len(got) != len(want) {
			t.Errorf("%q -> %v, want %v", in, got, want)
			continue
		}
		for i := range got {
			if got[i] != want[i] {
				t.Errorf("%q[%d] = %q want %q", in, i, got[i], want[i])
			}
		}
	}
}

func TestBuildVault_RejectsMissingKeysInProd(t *testing.T) {
	get := func(k string) string { return "" }
	_, err := BuildVault(get, nil)
	if !errors.Is(err, ErrMissingKeys) {
		t.Fatalf("expected ErrMissingKeys, got %v", err)
	}
}

func TestBuildVault_EphemeralKeyInDevMode(t *testing.T) {
	env := map[string]string{"OOPS_DEV_MODE": "1"}
	get := func(k string) string { return env[k] }
	var logs []string
	logf := func(f string, a ...any) {
		_ = a
		logs = append(logs, f)
	}
	v, err := BuildVault(get, logf)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v == nil || v.ActiveVersion() != 1 {
		t.Fatalf("bad vault: %+v", v)
	}
	if len(logs) == 0 || !strings.Contains(logs[0], "WARNING") {
		t.Fatalf("expected warning log, got %v", logs)
	}
}

// TestBuildVault_EphemeralKeyIsNonDeterministic locks the M1 fix in
// place: if the dev-mode path regresses to a deterministic constant,
// two successive BuildVault calls would share a key and each could
// decrypt the other's ciphertext. That would make the "data NOT
// readable after restart" warning a lie and leak customer PII to
// anyone holding this repo.
func TestBuildVault_EphemeralKeyIsNonDeterministic(t *testing.T) {
	env := map[string]string{"OOPS_DEV_MODE": "1"}
	get := func(k string) string { return env[k] }
	discard := func(string, ...any) {}

	a, err := BuildVault(get, discard)
	if err != nil {
		t.Fatalf("first build: %v", err)
	}
	b, err := BuildVault(get, discard)
	if err != nil {
		t.Fatalf("second build: %v", err)
	}
	ct, err := a.Encrypt("patient-123")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	if _, err := b.Decrypt(ct); err == nil {
		t.Fatal("second dev vault decrypted the first vault's ciphertext — dev key is not ephemeral")
	}
}

func TestBuildVault_ParsesHexKeys(t *testing.T) {
	// Two 32-byte keys in hex.
	env := map[string]string{"ENC_KEYS": "1:" + strings.Repeat("ab", 32) + ",2:" + strings.Repeat("cd", 32)}
	get := func(k string) string { return env[k] }
	v, err := BuildVault(get, nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v.ActiveVersion() != 2 {
		t.Fatalf("expected active=2, got %d", v.ActiveVersion())
	}
}

func TestBuildVault_RejectsPlaceholderKeyInProd(t *testing.T) {
	env := map[string]string{"ENC_KEYS": placeholderKey}
	get := func(k string) string { return env[k] }
	_, err := BuildVault(get, nil)
	if !errors.Is(err, ErrPlaceholderKey) {
		t.Fatalf("expected ErrPlaceholderKey, got %v", err)
	}
}

func TestBuildVault_AcceptsPlaceholderKeyInDevModeWithWarning(t *testing.T) {
	env := map[string]string{"ENC_KEYS": placeholderKey, "OOPS_DEV_MODE": "1"}
	get := func(k string) string { return env[k] }
	var logs []string
	logf := func(f string, a ...any) {
		_ = a
		logs = append(logs, f)
	}
	v, err := BuildVault(get, logf)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v == nil {
		t.Fatal("nil vault")
	}
	var warned bool
	for _, l := range logs {
		if strings.Contains(l, "placeholder") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("expected placeholder warning, got %v", logs)
	}
}

func TestBuildVault_RejectsMalformedEntry(t *testing.T) {
	env := map[string]string{"ENC_KEYS": "no-colon-here"}
	get := func(k string) string { return env[k] }
	_, err := BuildVault(get, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildVault_RejectsBadHex(t *testing.T) {
	env := map[string]string{"ENC_KEYS": "1:ZZZZ"}
	get := func(k string) string { return env[k] }
	_, err := BuildVault(get, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGenerateKeyLine_HappyPath(t *testing.T) {
	line, err := GenerateKeyLine(7, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(line, "7:") {
		t.Fatalf("unexpected prefix: %s", line)
	}
	if len(line) != 2+64 {
		t.Fatalf("unexpected length: %d (%s)", len(line), line)
	}
}

func TestGenerateKeyLine_RandError(t *testing.T) {
	fake := func(_ []byte) (int, error) { return 0, errors.New("oops") }
	if _, err := GenerateKeyLine(1, fake); err == nil {
		t.Fatal("expected error")
	}
}

func TestBuildVault_DefaultGetterAndLoggerSafe(t *testing.T) {
	t.Setenv("ENC_KEYS", "")
	t.Setenv("OOPS_DEV_MODE", "1")
	// nil getenv/logf should fall back to real os.Getenv and log.Printf.
	v, err := BuildVault(nil, nil)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if v.ActiveVersion() != 1 {
		t.Fatalf("expected version 1, got %d", v.ActiveVersion())
	}
}

// fakeSeedStore mirrors the SeedStore surface. Behavior is swappable per
// test so we can exercise success, missing-admin-fresh-install, and
// propagated errors.
type fakeSeedStore struct {
	hasAdmin      bool
	createErr     error
	replaceErr    error
	createdUser   models.User
	replacedAreas []geo.Region
}

func (f *fakeSeedStore) GetUserByUsername(_ context.Context, name string) (models.User, error) {
	if f.hasAdmin && name == "admin" {
		return models.User{ID: "u_admin", Username: "admin"}, nil
	}
	return models.User{}, errors.New("not found")
}
func (f *fakeSeedStore) CreateUser(_ context.Context, u models.User) error {
	if f.createErr != nil {
		return f.createErr
	}
	f.createdUser = u
	return nil
}
func (f *fakeSeedStore) ReplaceRegions(_ context.Context, rs []geo.Region) error {
	if f.replaceErr != nil {
		return f.replaceErr
	}
	f.replacedAreas = rs
	return nil
}

func TestSeedDeployment_AdminAlreadyExists_SkipsCreate(t *testing.T) {
	s := &fakeSeedStore{hasAdmin: true}
	if err := SeedDeployment(context.Background(), s, "", time.Now); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if s.createdUser.ID != "" {
		t.Fatal("admin should not be recreated")
	}
	if len(s.replacedAreas) != 1 {
		t.Fatalf("expected 1 region seeded, got %d", len(s.replacedAreas))
	}
}

func TestSeedDeployment_MissingPassword(t *testing.T) {
	s := &fakeSeedStore{hasAdmin: false}
	err := SeedDeployment(context.Background(), s, "", time.Now)
	if !errors.Is(err, ErrMissingAdminPassword) {
		t.Fatalf("expected ErrMissingAdminPassword, got %v", err)
	}
}

func TestSeedDeployment_BadPasswordPolicy(t *testing.T) {
	s := &fakeSeedStore{hasAdmin: false}
	err := SeedDeployment(context.Background(), s, "short", time.Now)
	if err == nil {
		t.Fatal("expected policy error")
	}
	if strings.Contains(err.Error(), "ADMIN_PASSWORD not set") {
		t.Fatalf("wrong error kind: %v", err)
	}
}

func TestSeedDeployment_HappyPath(t *testing.T) {
	s := &fakeSeedStore{hasAdmin: false}
	err := SeedDeployment(context.Background(), s, "correct-horse-battery-staple", time.Now)
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if s.createdUser.Username != "admin" {
		t.Fatalf("admin not created: %+v", s.createdUser)
	}
	if len(s.replacedAreas) != 1 || s.replacedAreas[0].Polygon.ID != "downtown" {
		t.Fatalf("region not seeded correctly: %+v", s.replacedAreas)
	}
}

func TestSeedDeployment_CreateUserError(t *testing.T) {
	s := &fakeSeedStore{createErr: errors.New("db down")}
	err := SeedDeployment(context.Background(), s, "correct-horse-battery-staple", time.Now)
	if err == nil || !strings.Contains(err.Error(), "create admin") {
		t.Fatalf("expected create-admin error, got %v", err)
	}
}

func TestSeedDeployment_ReplaceRegionsError(t *testing.T) {
	s := &fakeSeedStore{hasAdmin: true, replaceErr: errors.New("boom")}
	err := SeedDeployment(context.Background(), s, "", time.Now)
	if err == nil {
		t.Fatal("expected replace error")
	}
}

func TestSeedDemoUsers_InstallsAllFiveRoles(t *testing.T) {
	s := &fakeSeedStore{hasAdmin: false}
	// Track every CreateUser invocation so we can assert on the set.
	created := map[string]models.Role{}
	wrap := &recordingSeedStore{inner: s, created: created}
	if err := SeedDemoUsers(context.Background(), wrap, func() time.Time { return time.Unix(0, 0) }); err != nil {
		t.Fatal(err)
	}
	// All five DefaultDemoUsers should have been created.
	if len(created) != len(DefaultDemoUsers()) {
		t.Fatalf("expected %d creations, got %d (%v)", len(DefaultDemoUsers()), len(created), created)
	}
	for _, du := range DefaultDemoUsers() {
		if created[du.Username] != du.Role {
			t.Errorf("user %s: role %q != %q", du.Username, created[du.Username], du.Role)
		}
	}
}

// TestSeedDemoUsers_ForcesMustRotate pins the L2 invariant: every
// account installed through SeedDemoUsers (the SEED_DEMO_USERS=1
// path) ships with MustRotatePassword=true, so the README-published
// shared credentials cannot be used against a live deployment without
// being rotated first.
func TestSeedDemoUsers_ForcesMustRotate(t *testing.T) {
	s := &fakeSeedStore{hasAdmin: false}
	seen := []models.User{}
	wrap := &capturingSeedStore{inner: s, sink: &seen}
	if err := SeedDemoUsers(context.Background(), wrap, func() time.Time { return time.Unix(0, 0) }); err != nil {
		t.Fatal(err)
	}
	if len(seen) == 0 {
		t.Fatal("no demo users were created")
	}
	for _, u := range seen {
		if !u.MustRotatePassword {
			t.Errorf("demo user %q seeded without MustRotatePassword=true", u.Username)
		}
	}
}

// capturingSeedStore records every User the seeder creates so tests can
// assert on the full payload (not just username/role).
type capturingSeedStore struct {
	inner *fakeSeedStore
	sink  *[]models.User
}

func (c *capturingSeedStore) GetUserByUsername(ctx context.Context, name string) (models.User, error) {
	return c.inner.GetUserByUsername(ctx, name)
}
func (c *capturingSeedStore) CreateUser(ctx context.Context, u models.User) error {
	*c.sink = append(*c.sink, u)
	return c.inner.CreateUser(ctx, u)
}
func (c *capturingSeedStore) ReplaceRegions(ctx context.Context, rs []geo.Region) error {
	return c.inner.ReplaceRegions(ctx, rs)
}

// badHashStore simulates an auth.HashPassword failure by returning an
// error at CreateUser time, letting us exercise the wrap-in-fmt branch.
type badHashStore struct {
	fakeSeedStore
	err error
}

func (b *badHashStore) CreateUser(_ context.Context, _ models.User) error { return b.err }

func TestSeedUsers_RejectsBadPasswordPolicy(t *testing.T) {
	s := &fakeSeedStore{}
	// A demo user whose password is too short triggers auth.HashPassword
	// → auth.ErrPasswordTooShort, which seedUsers must wrap with "hash".
	err := seedUsers(context.Background(), s, []DemoUser{
		{Username: "bad", Role: models.RoleAdmin, Password: "short"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "hash bad") {
		t.Fatalf("expected hash-wrap error, got %v", err)
	}
}

func TestSeedDemoUsers_PropagatesCreateError(t *testing.T) {
	s := &badHashStore{err: errors.New("no write")}
	err := SeedDemoUsers(context.Background(), s, nil)
	if err == nil || !strings.Contains(err.Error(), "create admin") {
		t.Fatalf("expected create-admin error, got %v", err)
	}
}

func TestSeedDemoUsers_DefaultNowIsReal(t *testing.T) {
	s := &fakeSeedStore{}
	// nil now => time.Now is used internally. Test is a smoke check that
	// the branch executes without panicking and at least one user is
	// created (the fake's hasAdmin=false default).
	if err := SeedDemoUsers(context.Background(), s, nil); err != nil {
		t.Fatal(err)
	}
}

func TestSeedDemoUsers_SkipsExisting(t *testing.T) {
	// hasAdmin=true makes GetUserByUsername("admin") succeed; the rest
	// still fail and will be created.
	s := &fakeSeedStore{hasAdmin: true}
	count := 0
	wrap := &recordingSeedStore{inner: s, created: map[string]models.Role{}, onCreate: func() { count++ }}
	if err := SeedDemoUsers(context.Background(), wrap, nil); err != nil {
		t.Fatal(err)
	}
	// admin already exists -> 4 creations for the other roles.
	if count != len(DefaultDemoUsers())-1 {
		t.Fatalf("expected %d creations, got %d", len(DefaultDemoUsers())-1, count)
	}
}

// recordingSeedStore wraps a fakeSeedStore so tests can assert which
// users were actually created.
type recordingSeedStore struct {
	inner    *fakeSeedStore
	created  map[string]models.Role
	onCreate func()
}

func (r *recordingSeedStore) GetUserByUsername(ctx context.Context, name string) (models.User, error) {
	return r.inner.GetUserByUsername(ctx, name)
}
func (r *recordingSeedStore) CreateUser(ctx context.Context, u models.User) error {
	if err := r.inner.CreateUser(ctx, u); err != nil {
		return err
	}
	r.created[u.Username] = u.Role
	if r.onCreate != nil {
		r.onCreate()
	}
	return nil
}
func (r *recordingSeedStore) ReplaceRegions(ctx context.Context, rr []geo.Region) error {
	return r.inner.ReplaceRegions(ctx, rr)
}

func TestSeedDeployment_DefaultNowIsReal(t *testing.T) {
	s := &fakeSeedStore{hasAdmin: false}
	if err := SeedDeployment(context.Background(), s, "correct-horse-battery-staple", nil); err != nil {
		t.Fatal(err)
	}
	if s.createdUser.CreatedAt.IsZero() {
		t.Fatal("expected real time.Now default")
	}
}
