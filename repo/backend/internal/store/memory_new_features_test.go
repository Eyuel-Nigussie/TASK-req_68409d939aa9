package store

import (
	"context"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/models"
)

// Tests for the two new memory-store surfaces added to match the
// prompt: test_items (normalized per-sample test list) and
// system_settings (global key/value store, used by the map image).

func TestMemory_TestItems_ReplaceAndRead(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// Empty sample → empty list, not an error.
	got, err := m.ListTestItems(ctx, "s1")
	if err != nil || len(got) != 0 {
		t.Fatalf("expected empty slice, got %v %v", got, err)
	}

	items := []models.TestItem{
		{ID: "t1", SampleID: "s1", TestCode: "GLU", Instructions: "fasting", CreatedAt: time.Unix(1, 0)},
		{ID: "t2", SampleID: "s1", TestCode: "LIP", Instructions: "", CreatedAt: time.Unix(2, 0)},
	}
	if err := m.ReplaceTestItems(ctx, "s1", items); err != nil {
		t.Fatal(err)
	}
	got, _ = m.ListTestItems(ctx, "s1")
	if len(got) != 2 || got[0].TestCode != "GLU" || got[1].Instructions != "" {
		t.Fatalf("unexpected items: %+v", got)
	}

	// Replace with an empty list clears the sample's rows entirely.
	if err := m.ReplaceTestItems(ctx, "s1", nil); err != nil {
		t.Fatal(err)
	}
	got, _ = m.ListTestItems(ctx, "s1")
	if len(got) != 0 {
		t.Fatalf("expected zero after clear, got %v", got)
	}
}

func TestMemory_TestItems_IsolatedPerSample(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()
	_ = m.ReplaceTestItems(ctx, "s1", []models.TestItem{{ID: "a", SampleID: "s1", TestCode: "A"}})
	_ = m.ReplaceTestItems(ctx, "s2", []models.TestItem{{ID: "b", SampleID: "s2", TestCode: "B"}})
	if got, _ := m.ListTestItems(ctx, "s1"); len(got) != 1 || got[0].TestCode != "A" {
		t.Fatalf("s1 cross-leaked: %v", got)
	}
	if got, _ := m.ListTestItems(ctx, "s2"); len(got) != 1 || got[0].TestCode != "B" {
		t.Fatalf("s2 cross-leaked: %v", got)
	}
}

func TestMemory_SystemSettings_PutGetList(t *testing.T) {
	m := NewMemory()
	ctx := context.Background()

	// Missing key returns ErrNotFound.
	if _, err := m.GetSetting(ctx, "nope"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	if err := m.PutSetting(ctx, "map.image.url", "/static/map.png"); err != nil {
		t.Fatal(err)
	}
	got, err := m.GetSetting(ctx, "map.image.url")
	if err != nil || got != "/static/map.png" {
		t.Fatalf("round trip wrong: %q %v", got, err)
	}
	// Overwrite behavior — PutSetting replaces, does not append.
	_ = m.PutSetting(ctx, "map.image.url", "")
	got, _ = m.GetSetting(ctx, "map.image.url")
	if got != "" {
		t.Fatalf("expected empty value after overwrite, got %q", got)
	}
	// List returns every key.
	_ = m.PutSetting(ctx, "k2", "v2")
	list, _ := m.ListSettings(ctx)
	if len(list) < 2 {
		t.Fatalf("expected >=2 settings, got %v", list)
	}
}
