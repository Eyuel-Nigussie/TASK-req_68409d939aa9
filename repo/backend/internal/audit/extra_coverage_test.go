package audit

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eaglepoint/oops/backend/internal/models"
)

// badStore triggers the marshal-time error branches by accepting any
// input but returning a sentinel from AppendAudit. For the marshal error
// branch we pass a value that json.Marshal rejects.
type badStore struct{ err error }

func (b *badStore) AppendAudit(_ context.Context, _ models.AuditEntry) error { return b.err }
func (b *badStore) ListAudit(_ context.Context, _, _ string, _ int) ([]models.AuditEntry, error) {
	return nil, b.err
}

// unmarshalable is a type that json.Marshal fails on — channels cannot
// be marshalled.
type unmarshalable struct{ Ch chan int }

func TestLog_MarshalBeforeErrorBubbles(t *testing.T) {
	l := New(&badStore{}, func() time.Time { return time.Unix(1, 0) })
	if err := l.Log(context.Background(), "u", "ws", time.Time{}, "e", "id", "a", "", unmarshalable{Ch: make(chan int)}, nil); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestLog_MarshalAfterErrorBubbles(t *testing.T) {
	l := New(&badStore{}, nil)
	if err := l.Log(context.Background(), "u", "ws", time.Time{}, "e", "id", "a", "", nil, unmarshalable{Ch: make(chan int)}); err == nil {
		t.Fatal("expected marshal error")
	}
}

func TestLog_AppendErrorBubbles(t *testing.T) {
	sentinel := errors.New("append failed")
	l := New(&badStore{err: sentinel}, nil)
	err := l.Log(context.Background(), "u", "ws", time.Time{}, "e", "id", "a", "", nil, nil)
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected %v, got %v", sentinel, err)
	}
}
