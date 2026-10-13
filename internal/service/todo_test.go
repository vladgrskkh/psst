//nolint:testpackage // white-box tests share unexported package fakes (fakeStore, fakeClock).
package service

import (
	"errors"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/pkg/option"
)

type fakeStore struct {
	todos map[string]*domain.Todo
}

func newFakeStore() *fakeStore { return &fakeStore{todos: map[string]*domain.Todo{}} }

func (f *fakeStore) Get(id string) (*domain.Todo, error) {
	t, ok := f.todos[id]
	if !ok {
		return nil, ErrNotFound
	}
	return t, nil
}

func (f *fakeStore) List() ([]*domain.Todo, error) {
	out := make([]*domain.Todo, 0, len(f.todos))
	for _, t := range f.todos {
		out = append(out, t)
	}
	return out, nil
}

func (f *fakeStore) ListByReminderID(rid string) (*domain.Todo, error) {
	for _, t := range f.todos {
		if t.ReminderID.IsPresent() && t.ReminderID.ValueOr("") == rid {
			return t, nil
		}
	}
	return nil, ErrNotFound
}

func (f *fakeStore) Put(todo *domain.Todo) error {
	f.todos[todo.ID] = todo
	return nil
}

func (f *fakeStore) PurgeTombstones(time.Time, time.Time) error { return nil }

type fakeClock struct{ t time.Time }

func (c fakeClock) Now() time.Time { return c.t }

func TestCreate(t *testing.T) {
	t.Parallel()

	svc := NewTodoService(newFakeStore(), fakeClock{t: time.Now()})
	todo, err := svc.Create(CreateTodoParams{
		Title:    "  buy milk  ",
		Deadline: option.Some(time.Now().Add(time.Hour)),
		ListName: " groceries ",
	})
	if err != nil {
		t.Fatal(err)
	}
	if todo.Title != "buy milk" || todo.ListName != "groceries" {
		t.Fatalf("trimming failed: %+v", todo)
	}
	if todo.CreatedAt.IsZero() || todo.UpdatedAt.IsZero() {
		t.Fatal("timestamps must be set")
	}
}

func TestCreateValidation(t *testing.T) {
	t.Parallel()

	svc := NewTodoService(newFakeStore(), fakeClock{t: time.Now()})
	_, err := svc.Create(CreateTodoParams{Title: "  ", ListName: "x"})
	if !errors.Is(err, ErrInvalidTitle) {
		t.Fatalf("err = %v; want ErrInvalidTitle", err)
	}
	_, err = svc.Create(CreateTodoParams{Title: "x", ListName: " "})
	if !errors.Is(err, ErrInvalidList) {
		t.Fatalf("err = %v; want ErrInvalidList", err)
	}
	_, err = svc.Create(CreateTodoParams{Title: "x", ListName: "x", Deadline: option.Some(time.Now().Add(-time.Hour))})
	if !errors.Is(err, ErrDeadlineInPast) {
		t.Fatalf("err = %v; want ErrDeadlineInPast", err)
	}
}

func TestUpdatePartial(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	now := time.Now()
	svc := NewTodoService(store, fakeClock{t: now})
	todo, _ := svc.Create(CreateTodoParams{Title: "original", ListName: "psst"})

	updated, err := svc.Update(todo.ID, UpdateTodoParams{
		Title:       option.Some("renamed"),
		Deadline:    option.Some(option.Some(now.Add(2 * time.Hour))),
		Description: option.None[string](),
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Title != "renamed" || !updated.Deadline.IsPresent() {
		t.Fatalf("update failed: %+v", updated)
	}
	if !updated.UpdatedAt.Equal(now) {
		t.Fatal("UpdatedAt must be stamped")
	}

	cleared, err := svc.Update(todo.ID, UpdateTodoParams{Deadline: option.Some(option.None[time.Time]())})
	if err != nil {
		t.Fatal(err)
	}
	if cleared.Deadline.IsPresent() {
		t.Fatal("Some(None) deadline should clear it")
	}
}

func TestUpdateListNameClearsListID(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	todo, _ := svc.Create(CreateTodoParams{Title: "x", ListName: "psst"})
	todo.ListID = option.Some("l-default")
	if err := store.Put(todo); err != nil {
		t.Fatal(err)
	}

	updated, err := svc.Update(todo.ID, UpdateTodoParams{ListName: option.Some("work")})
	if err != nil {
		t.Fatal(err)
	}
	if updated.ListID.IsPresent() {
		t.Fatalf("ListID = %+v; must be cleared so the push re-resolves by name", updated.ListID)
	}
	if updated.ListName != "work" {
		t.Fatalf("ListName = %q; want work", updated.ListName)
	}
}

func TestToggleDone(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	now := time.Now()
	svc := NewTodoService(store, fakeClock{t: now})
	todo, _ := svc.Create(CreateTodoParams{Title: "x", ListName: "psst"})

	done, err := svc.ToggleDone(todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !done.Done || !done.CompletedAt.IsPresent() {
		t.Fatalf("expected done: %+v", done)
	}
	undone, err := svc.ToggleDone(todo.ID)
	if err != nil {
		t.Fatal(err)
	}
	if undone.Done || undone.CompletedAt.IsPresent() {
		t.Fatalf("expected undone: %+v", undone)
	}
}

func TestDeleteTombstone(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	now := time.Now()
	svc := NewTodoService(store, fakeClock{t: now})
	todo, _ := svc.Create(CreateTodoParams{Title: "x", ListName: "psst"})
	if err := svc.Delete(todo.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := store.Get(todo.ID)
	if !got.Deleted || !got.DeletedAt.IsPresent() {
		t.Fatalf("expected tombstone: %+v", got)
	}
	if err := svc.Delete("nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v; want ErrNotFound", err)
	}
}

func TestListFilters(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	svc.Create(CreateTodoParams{Title: "Buy Milk", ListName: "psst"})
	svc.Create(CreateTodoParams{Title: "gym", Description: "legs day", ListName: "psst"})
	deleted, _ := svc.Create(CreateTodoParams{Title: "gone", ListName: "psst"})
	svc.Delete(deleted.ID)

	all, err := svc.List("")
	if err != nil || len(all) != 2 {
		t.Fatalf("List() = %d todos, err %v; want 2", len(all), err)
	}
	filtered, err := svc.List("milk")
	if err != nil || len(filtered) != 1 || filtered[0].Title != "Buy Milk" {
		t.Fatalf("List(milk) = %+v, err %v", filtered, err)
	}
	filtered, _ = svc.List("legs")
	if len(filtered) != 1 || filtered[0].Title != "gym" {
		t.Fatalf("List(legs) = %+v", filtered)
	}
}
