package store_test

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/infra/store"
	"github.com/vladgrskkh/psst/internal/service"
	"github.com/vladgrskkh/psst/pkg/option"
)

func newTestStore(t *testing.T) *store.TodoStore {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return store.NewTodoStore(db)
}

func sampleTodo() *domain.Todo {
	t := domain.NewTodo("buy milk")
	t.CreatedAt = time.Now()
	t.UpdatedAt = t.CreatedAt
	t.Deadline = option.Some(time.Now().Add(time.Hour))
	t.ReminderID = option.Some("rem-1")
	return t
}

func TestPutGetRoundTrip(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	in := sampleTodo()
	if err := s.Put(in); err != nil {
		t.Fatal(err)
	}
	out, err := s.Get(in.ID)
	if err != nil {
		t.Fatal(err)
	}
	if out.Title != in.Title || out.ReminderID.ValueOr("") != "rem-1" ||
		!out.Deadline.IsPresent() || out.Deadline.ValueOr(time.Time{}).IsZero() {
		t.Fatalf("round trip mismatch: %+v", out)
	}
}

func TestGetMissing(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	if _, err := s.Get("nope"); !errors.Is(err, service.ErrNotFound) {
		t.Fatalf("err = %v; want service.ErrNotFound", err)
	}
}

func TestListSortedByCreatedAt(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	first := sampleTodo()
	first.CreatedAt = time.Now().Add(-time.Hour)
	second := sampleTodo()
	second.CreatedAt = time.Now()
	if err := s.Put(second); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(first); err != nil {
		t.Fatal(err)
	}
	all, err := s.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 || all[0].ID != first.ID {
		t.Fatalf("unexpected order: %+v", all)
	}
}

func TestListByReminderID(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	todo := sampleTodo()
	if err := s.Put(todo); err != nil {
		t.Fatal(err)
	}
	got, err := s.ListByReminderID("rem-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != todo.ID {
		t.Fatalf("got %s; want %s", got.ID, todo.ID)
	}
	if _, listErr := s.ListByReminderID("nope"); !errors.Is(listErr, service.ErrNotFound) {
		t.Fatalf("err = %v; want service.ErrNotFound", listErr)
	}
}

func TestPurgeTombstones(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	old := time.Now().Add(-48 * time.Hour)
	cutoff := time.Now().Add(-24 * time.Hour)

	purgeable := sampleTodo()
	purgeable.Deleted = true
	purgeable.DeletedAt = option.Some(old)
	purgeable.UpdatedAt = old
	if err := s.Put(purgeable); err != nil {
		t.Fatal(err)
	}

	notPushed := sampleTodo()
	notPushed.Deleted = true
	notPushed.DeletedAt = option.Some(old)
	notPushed.UpdatedAt = time.Now() // newer than lastPushAt
	if err := s.Put(notPushed); err != nil {
		t.Fatal(err)
	}

	recent := sampleTodo()
	recent.Deleted = true
	recent.DeletedAt = option.Some(time.Now())
	recent.UpdatedAt = recent.DeletedAt.ValueOr(time.Time{})
	if err := s.Put(recent); err != nil {
		t.Fatal(err)
	}

	lastPushAt := time.Now().Add(-time.Minute)
	if err := s.PurgeTombstones(cutoff, lastPushAt); err != nil {
		t.Fatal(err)
	}
	all, _ := s.List()
	if len(all) != 2 {
		t.Fatalf("after purge: %d todos; want 2", len(all))
	}
	for _, todo := range all {
		if todo.ID == purgeable.ID {
			t.Fatal("purgeable tombstone was not purged")
		}
	}
}

func TestSyncStateRoundTrip(t *testing.T) {
	t.Parallel()

	s := newTestStore(t)
	state, err := s.GetSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if !state.LastPullAt.IsZero() || !state.LastPushAt.IsZero() {
		t.Fatalf("fresh state should be zero: %+v", state)
	}
	want := domain.SyncState{LastPullAt: time.Now(), LastPushAt: time.Now().Add(-time.Minute)}
	if saveErr := s.SaveSyncState(want); saveErr != nil {
		t.Fatal(saveErr)
	}
	got, err := s.GetSyncState()
	if err != nil {
		t.Fatal(err)
	}
	if !got.LastPullAt.Equal(want.LastPullAt) || !got.LastPushAt.Equal(want.LastPushAt) {
		t.Fatalf("got %+v; want %+v", got, want)
	}
}
