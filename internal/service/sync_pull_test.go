//nolint:testpackage // white-box tests share unexported package fakes (fakeStore, fakeGateway); each test builds its own state.
package service

import (
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/pkg/option"
)

func seededEngine(t *testing.T, gw *fakeGateway) (*fakeStore, *SyncEngine, *domain.Todo) {
	t.Helper()
	store := newFakeStore()
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	todo, _ := svc.Create(CreateTodoParams{Title: "buy milk", ListName: "psst"})
	engine := newTestEngine(gw, store)
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	return store, engine, todo
}

func TestPullUnknownReminderCreatesTodo(t *testing.T) {
	t.Parallel()

	gw := newFakeGateway()
	store, engine, _ := seededEngine(t, gw)
	gw.reminders = append(gw.reminders, RemoteReminder{
		ID: "r-new", ListID: "l-default", Title: "from phone", Notes: "n",
		DueDate: option.Some(time.Now().Add(time.Hour)),
	})

	if err := engine.PullChanges(); err != nil {
		t.Fatal(err)
	}
	all, _ := store.List()
	if len(all) != 2 {
		t.Fatalf("todos = %d; want 2", len(all))
	}
	var created *domain.Todo
	for _, todo := range all {
		if todo.Title == "from phone" {
			created = todo
		}
	}
	if created == nil {
		t.Fatal("remote reminder was not imported")
	}
	if !created.ReminderID.IsPresent() || created.ReminderID.ValueOr("") != "r-new" {
		t.Fatalf("ReminderID = %+v", created.ReminderID)
	}
	if created.ListName != "psst" {
		t.Fatalf("ListName = %q; want psst", created.ListName)
	}
}

func TestPullAppliesRemoteChange(t *testing.T) {
	t.Parallel()

	gw := newFakeGateway()
	store, engine, todo := seededEngine(t, gw)
	// The gateway's Pull returns its current reminder list; the seeded push
	// stored a snapshot. Change the remote title.
	gw.reminders[0].Title = "buy oat milk"

	if err := engine.PullChanges(); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(todo.ID)
	if got.Title != "buy oat milk" {
		t.Fatalf("Title = %q; want remote change applied", got.Title)
	}
}

func TestPullConflictKeepsLocal(t *testing.T) {
	t.Parallel()

	gw := newFakeGateway()
	store, engine, todo := seededEngine(t, gw)
	gw.reminders[0].Title = "remote title"

	// local change to the same field
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	if _, err := svc.Update(todo.ID, UpdateTodoParams{Title: option.Some("local title")}); err != nil {
		t.Fatal(err)
	}

	if err := engine.PullChanges(); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(todo.ID)
	if got.Title != "local title" {
		t.Fatalf("Title = %q; local must win on conflict", got.Title)
	}
}

func TestPullMergesUntouchedFieldOnConflict(t *testing.T) {
	t.Parallel()

	gw := newFakeGateway()
	store, engine, todo := seededEngine(t, gw)
	gw.reminders[0].Title = "remote title"
	gw.reminders[0].Notes = "remote notes"

	svc := NewTodoService(store, fakeClock{t: time.Now()})
	if _, err := svc.Update(todo.ID, UpdateTodoParams{Title: option.Some("local title")}); err != nil {
		t.Fatal(err)
	}

	if err := engine.PullChanges(); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(todo.ID)
	if got.Title != "local title" {
		t.Fatalf("Title = %q; local must win on conflict", got.Title)
	}
	if got.Description != "remote notes" {
		t.Fatalf("Description = %q; untouched field must merge from remote", got.Description)
	}
}

func TestPullRemoteDeletionCreatesTombstone(t *testing.T) {
	t.Parallel()

	gw := newFakeGateway()
	store, engine, todo := seededEngine(t, gw)
	gw.reminders = nil // reminder deleted in Reminders.app

	if err := engine.PullChanges(); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(todo.ID)
	if !got.Deleted {
		t.Fatal("expected tombstone for remotely-deleted todo")
	}
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	if len(gw.deleted) != 0 {
		t.Fatal("tombstone must not re-delete remotely")
	}
}

func TestPullRevivesTombstoneWhenReminderReappears(t *testing.T) {
	t.Parallel()

	gw := newFakeGateway()
	store, engine, todo := seededEngine(t, gw)
	rid := todo.ReminderID.ValueOr("")

	gw.reminders = nil // reminder deleted in Reminders.app
	if err := engine.PullChanges(); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(todo.ID)
	if !got.Deleted {
		t.Fatal("expected tombstone for remotely-deleted todo")
	}

	gw.reminders = []RemoteReminder{{ID: rid, ListID: "l-default", Title: "buy milk"}} // re-added remotely
	if err := engine.PullChanges(); err != nil {
		t.Fatal(err)
	}
	got, _ = store.Get(todo.ID)
	if got.Deleted {
		t.Fatal("tombstone must be revived when the reminder reappears")
	}
	if got.ReminderID.ValueOr("") != rid {
		t.Fatalf("ReminderID = %q; want %q", got.ReminderID.ValueOr(""), rid)
	}
}

func TestPullRemoteDeletionWithLocalChangesDoesNotTombstone(t *testing.T) {
	t.Parallel()

	gw := newFakeGateway()
	store, engine, todo := seededEngine(t, gw)
	gw.reminders = nil

	svc := NewTodoService(store, fakeClock{t: time.Now()})
	if _, err := svc.Update(todo.ID, UpdateTodoParams{Title: option.Some("local edit")}); err != nil {
		t.Fatal(err)
	}

	if err := engine.PullChanges(); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(todo.ID)
	if got.Deleted {
		t.Fatal("locally-modified todo must not be tombstoned")
	}
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	if len(gw.created) != 2 {
		t.Fatalf("created = %d; want re-push as create (2 total)", len(gw.created))
	}
}

func TestFullSyncPullsBeforePush(t *testing.T) {
	t.Parallel()

	gw := newFakeGateway()
	store := newFakeStore()
	engine := newTestEngine(gw, store)
	// A remote todo exists; a local todo exists (offline-created).
	gw.reminders = []RemoteReminder{{ID: "r-remote", ListID: "l-default", Title: "remote"}}
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	svc.Create(CreateTodoParams{Title: "local", ListName: "psst"})

	if err := engine.FullSync(); err != nil {
		t.Fatal(err)
	}
	all, _ := store.List()
	if len(all) != 2 {
		t.Fatalf("todos = %d; want 2 after FullSync", len(all))
	}
	if len(gw.created) != 1 {
		t.Fatalf("created = %d; want 1", len(gw.created))
	}
}

func TestPushSkipsDeletedUnpushedTodo(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	gw := newFakeGateway()
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	todo, _ := svc.Create(CreateTodoParams{Title: "never pushed", ListName: "psst"})
	if err := svc.Delete(todo.ID); err != nil {
		t.Fatal(err)
	}

	engine := newTestEngine(gw, store)
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	if len(gw.created) != 0 {
		t.Fatalf("created = %d; want 0 (nothing exists remotely)", len(gw.created))
	}
	if len(gw.deleted) != 0 {
		t.Fatalf("deleted = %d; want 0", len(gw.deleted))
	}
	got, _ := store.Get(todo.ID)
	if !got.Deleted {
		t.Fatal("todo must remain in the store as a tombstone")
	}
}

func TestPullRejectsMalformedSnapshots(t *testing.T) {
	t.Parallel()

	for _, tc := range []struct {
		name       string
		local      bool
		remoteGone bool
	}{
		{name: "remote snapshot during merge"},
		{name: "local snapshot during merge", local: true},
		{name: "local snapshot during tombstone", local: true, remoteGone: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			gw := newFakeGateway()
			store, engine, todo := seededEngine(t, gw)
			if tc.remoteGone {
				gw.reminders = nil
			}
			if tc.local {
				todo.LocalSnapshot = "{"
			} else {
				todo.RemoteSnapshot = "{"
			}
			if err := store.Put(todo); err != nil {
				t.Fatal(err)
			}

			if err := engine.PullChanges(); err == nil {
				t.Fatal("PullChanges() succeeded with malformed stored snapshot")
			}
		})
	}
}

func TestDiffSnapshotsComparesOptionalTimesByInstant(t *testing.T) {
	t.Parallel()

	utc := time.Date(2026, 8, 20, 14, 0, 0, 0, time.UTC)
	plusTwo := time.Date(2026, 8, 20, 16, 0, 0, 0, time.FixedZone("plus-two", 2*60*60))
	before := domain.TodoSnapshot{
		Deadline:    option.Some(utc),
		CompletedAt: option.Some(utc),
	}
	after := domain.TodoSnapshot{
		Deadline:    option.Some(plusTwo),
		CompletedAt: option.Some(plusTwo),
	}
	if diff := diffSnapshots(before, after); len(diff) != 0 {
		t.Fatalf("equal instants differ by snapshot fields: %v", diff)
	}
}
