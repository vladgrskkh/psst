//nolint:testpackage // white-box tests share unexported package fakes (fakeStore, fakeGateway).
package service

import (
	"errors"
	"testing"
	"time"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/pkg/option"
)

type fakeMeta struct {
	state domain.SyncState
}

func (f *fakeMeta) GetSyncState() (domain.SyncState, error) { return f.state, nil }
func (f *fakeMeta) SaveSyncState(s domain.SyncState) error  { f.state = s; return nil }

type fakeGateway struct {
	lists     []ReminderList
	reminders []RemoteReminder
	created   []CreateReminderParams
	updated   []UpdateReminderParams
	deleted   []string
	ensured   []string
	createErr error
	updateErr error
	deleteErr error
	pullErr   error
	listErr   error
	nextID    int
}

func newFakeGateway() *fakeGateway {
	return &fakeGateway{lists: []ReminderList{{ID: "l-default", Name: "psst"}}}
}

func (g *fakeGateway) Permissions() (string, error) { return "authorized", nil }
func (g *fakeGateway) ListLists() ([]ReminderList, error) {
	return g.lists, g.listErr
}
func (g *fakeGateway) EnsureList(name string) (ReminderList, error) {
	g.ensured = append(g.ensured, name)
	for _, l := range g.lists {
		if l.Name == name {
			return l, nil
		}
	}
	l := ReminderList{ID: "new-list", Name: name}
	g.lists = append(g.lists, l)
	return l, nil
}
func (g *fakeGateway) Pull([]string) ([]RemoteReminder, error) { return g.reminders, g.pullErr }
func (g *fakeGateway) Create(p CreateReminderParams) (string, error) {
	if g.createErr != nil {
		return "", g.createErr
	}
	g.created = append(g.created, p)
	g.nextID++
	id := "rem-" + string(rune('0'+g.nextID))
	g.reminders = append(g.reminders, RemoteReminder{
		ID: id, ListID: p.ListID, Title: p.Title, Notes: p.Notes, DueDate: p.DueDate,
	})
	return id, nil
}
func (g *fakeGateway) Update(p UpdateReminderParams) error {
	for _, r := range g.reminders {
		if r.ID == p.ID {
			g.updated = append(g.updated, p)
			return g.updateErr
		}
	}
	return ErrReminderNotFound
}
func (g *fakeGateway) Delete(id string) error {
	g.deleted = append(g.deleted, id)
	return g.deleteErr
}

func newTestEngine(g *fakeGateway, store *fakeStore) *SyncEngine {
	return NewSyncEngine(store, &fakeMeta{}, g, fakeClock{t: time.Now()}, "psst", 30*24*time.Hour)
}

func TestPushNewTodo(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	gw := newFakeGateway()
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	todo, _ := svc.Create(CreateTodoParams{Title: "buy milk", ListName: "psst"})

	engine := newTestEngine(gw, store)
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	if len(gw.created) != 1 {
		t.Fatalf("created %d; want 1", len(gw.created))
	}
	got, _ := store.Get(todo.ID)
	if !got.ReminderID.IsPresent() {
		t.Fatal("ReminderID must be set after push")
	}
	if got.RemoteSnapshot == "" || got.LocalSnapshot == "" {
		t.Fatal("snapshots must be set after push")
	}
	meta, _ := engine.meta.GetSyncState()
	if meta.LastPushAt.IsZero() {
		t.Fatal("LastPushAt must be set")
	}
}

func TestPushDoesNotRepeatUnchanged(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	gw := newFakeGateway()
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	svc.Create(CreateTodoParams{Title: "buy milk", ListName: "psst"})
	engine := newTestEngine(gw, store)
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	if len(gw.created) != 1 {
		t.Fatalf("created %d; want 1", len(gw.created))
	}
}

func TestPushUpdateAfterLocalChange(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	gw := newFakeGateway()
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	todo, _ := svc.Create(CreateTodoParams{Title: "buy milk", ListName: "psst"})
	engine := newTestEngine(gw, store)
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}

	if _, err := svc.Update(todo.ID, UpdateTodoParams{Title: option.Some("buy oat milk")}); err != nil {
		t.Fatal(err)
	}
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	if len(gw.updated) != 1 {
		t.Fatalf("updated %d; want 1", len(gw.updated))
	}
	if gw.updated[0].Title.ValueOr("") != "buy oat milk" {
		t.Fatalf("update title = %q", gw.updated[0].Title.ValueOr(""))
	}
}

func TestPushDeleteTombstone(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	gw := newFakeGateway()
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	todo, _ := svc.Create(CreateTodoParams{Title: "buy milk", ListName: "psst"})
	engine := newTestEngine(gw, store)
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(todo.ID); err != nil {
		t.Fatal(err)
	}
	if err := engine.PushChanges(); err != nil {
		t.Fatal(err)
	}
	if len(gw.deleted) != 1 || gw.deleted[0] != todo.ReminderID.ValueOr("") {
		t.Fatalf("deleted = %v; want [%s]", gw.deleted, todo.ReminderID.ValueOr(""))
	}
}

func TestPushAccessDeniedPropagates(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	gw := newFakeGateway()
	denied := errors.New("access denied")
	gw.createErr = denied
	svc := NewTodoService(store, fakeClock{t: time.Now()})
	svc.Create(CreateTodoParams{Title: "x", ListName: "psst"})
	engine := newTestEngine(gw, store)
	if err := engine.PushChanges(); !errors.Is(err, denied) {
		t.Fatalf("err = %v; want gateway error propagated", err)
	}
}

func TestSaveSnapshotsReturnsMarshalError(t *testing.T) {
	t.Parallel()

	store := newFakeStore()
	engine := newTestEngine(newFakeGateway(), store)
	todo := domain.NewTodo("out of range date")
	todo.Deadline = option.Some(time.Date(10000, time.January, 1, 0, 0, 0, 0, time.UTC))

	if err := engine.saveSnapshots(todo); err == nil {
		t.Fatal("saveSnapshots() succeeded with an unmarshalable time")
	}
	if _, err := store.Get(todo.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("todo was written after snapshot marshal failed: err = %v", err)
	}
}
