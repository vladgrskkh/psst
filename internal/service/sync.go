package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/pkg/option"
)

// ErrReminderNotFound is returned when a remote reminder ID no longer exists.
var ErrReminderNotFound = errors.New("reminder not found")

var ErrRemindersAccessDenied = errors.New(
	"reminders access denied — enable in System Settings → Privacy & Security → Reminders",
)

// SyncStore is the persistence contract used by SyncEngine.
type SyncStore interface {
	List() ([]*domain.Todo, error)
	Put(todo *domain.Todo) error
	PurgeTombstones(cutoff time.Time, lastPushAt time.Time) error
}

// SyncMeta is the metadata contract used by SyncEngine.
type SyncMeta interface {
	GetSyncState() (domain.SyncState, error)
	SaveSyncState(domain.SyncState) error
}

// ReminderList is a list exposed by the remote Reminders account.
type ReminderList struct {
	ID, Name string
}

// RemoteReminder is the adapter-neutral view of a reminder received from Reminders.
type RemoteReminder struct {
	ID, ListID, Title, Notes string
	DueDate                  option.Option[time.Time]
	Completed                bool
	CompletionDate           option.Option[time.Time]
}

// CreateReminderParams describes a request to create a remote reminder.
type CreateReminderParams struct {
	ListID, Title, Notes string
	DueDate              option.Option[time.Time]
}

// UpdateReminderParams distinguishes omitted fields from fields being cleared.
type UpdateReminderParams struct {
	ID             string
	Title, Notes   option.Option[string]
	ListID         option.Option[string]
	DueDate        option.Option[option.Option[time.Time]]
	Completed      option.Option[bool]
	CompletionDate option.Option[option.Option[time.Time]]
}

// RemindersGateway is the port SyncEngine uses to read and mutate remote reminders.
type RemindersGateway interface {
	Permissions() (string, error)
	ListLists() ([]ReminderList, error)
	EnsureList(name string) (ReminderList, error)
	Pull(listIDs []string) ([]RemoteReminder, error)
	Create(CreateReminderParams) (string, error)
	Update(UpdateReminderParams) error
	Delete(id string) error
}

func snapshotFromTodo(t *domain.Todo) domain.TodoSnapshot {
	return domain.TodoSnapshot{
		Title:       t.Title,
		Description: t.Description,
		Deadline:    t.Deadline,
		ListID:      t.ListID.ValueOr(""),
		Done:        t.Done,
		CompletedAt: t.CompletedAt,
		Deleted:     t.Deleted,
	}
}

func applySnapshot(t *domain.Todo, s domain.TodoSnapshot) {
	t.Title = s.Title
	t.Description = s.Description
	t.Deadline = s.Deadline
	if s.ListID != "" {
		t.ListID = option.Some(s.ListID)
	}
	t.Done = s.Done
	t.CompletedAt = s.CompletedAt
}

func snapshotFromReminder(r RemoteReminder) domain.TodoSnapshot {
	return domain.TodoSnapshot{
		Title:       r.Title,
		Description: r.Notes,
		Deadline:    r.DueDate,
		ListID:      r.ListID,
		Done:        r.Completed,
		CompletedAt: r.CompletionDate,
	}
}

func applyField(t *domain.Todo, field string, remote domain.TodoSnapshot) {
	switch field {
	case "title":
		t.Title = remote.Title
	case "description":
		t.Description = remote.Description
	case "deadline":
		t.Deadline = remote.Deadline
	case "listID":
		t.ListID = option.Some(remote.ListID)
	case "done":
		t.Done = remote.Done
	case "completedAt":
		t.CompletedAt = remote.CompletedAt
	case "deleted":
		if !remote.Deleted {
			t.Deleted = false
			t.DeletedAt = option.None[time.Time]()
		}
	}
}

func snapshotJSON(s domain.TodoSnapshot) (string, error) {
	data, err := json.Marshal(s)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func parseSnapshot(s string) (domain.TodoSnapshot, error) {
	if s == "" {
		return domain.TodoSnapshot{}, nil
	}
	var state domain.TodoSnapshot
	if err := json.Unmarshal([]byte(s), &state); err != nil {
		return domain.TodoSnapshot{}, err
	}
	return state, nil
}

// diffSnapshots returns the names of fields that differ between a and b.
func diffSnapshots(a, b domain.TodoSnapshot) map[string]struct{} {
	diff := make(map[string]struct{})
	if a.Title != b.Title {
		diff["title"] = struct{}{}
	}
	if a.Description != b.Description {
		diff["description"] = struct{}{}
	}
	if !equalOptionTime(a.Deadline, b.Deadline) {
		diff["deadline"] = struct{}{}
	}
	if a.ListID != b.ListID {
		diff["listID"] = struct{}{}
	}
	if a.Done != b.Done {
		diff["done"] = struct{}{}
	}
	if !equalOptionTime(a.CompletedAt, b.CompletedAt) {
		diff["completedAt"] = struct{}{}
	}
	if a.Deleted != b.Deleted {
		diff["deleted"] = struct{}{}
	}
	return diff
}

func equalOptionTime(a, b option.Option[time.Time]) bool {
	aValue, aPresent := a.Get()
	bValue, bPresent := b.Get()
	return aPresent == bPresent && (!aPresent || aValue.Equal(bValue))
}

type Syncer interface {
	FullSync() error
	PushChanges() error
	PullChanges() error
}

type SyncEngine struct {
	store              SyncStore
	meta               SyncMeta
	gateway            RemindersGateway
	clock              Clock
	defaultListName    string
	tombstoneRetention time.Duration
}

func NewSyncEngine(
	store SyncStore,
	meta SyncMeta,
	gateway RemindersGateway,
	clock Clock,
	defaultListName string,
	tombstoneRetention time.Duration,
) *SyncEngine {
	return &SyncEngine{
		store:              store,
		meta:               meta,
		gateway:            gateway,
		clock:              clock,
		defaultListName:    defaultListName,
		tombstoneRetention: tombstoneRetention,
	}
}

func (e *SyncEngine) needsPush(t *domain.Todo) (bool, error) {
	if !t.ReminderID.IsPresent() && !t.Deleted {
		return true, nil
	}
	local, err := parseSnapshot(t.LocalSnapshot)
	if err != nil {
		return false, fmt.Errorf("parse local snapshot for todo %q: %w", t.ID, err)
	}
	return len(diffSnapshots(local, snapshotFromTodo(t))) > 0, nil
}

// PushChanges pushes locally-changed todos to Reminders.
func (e *SyncEngine) PushChanges() error {
	state, err := e.meta.GetSyncState()
	if err != nil {
		return err
	}
	todos, err := e.store.List()
	if err != nil {
		return err
	}

	for _, t := range todos {
		if t.Deleted && !t.ReminderID.IsPresent() {
			continue
		}
		needsPush, checkErr := e.needsPush(t)
		if checkErr != nil {
			return checkErr
		}
		if !needsPush {
			continue
		}
		if err = e.pushTodo(t); err != nil {
			return err
		}
	}

	state.LastPushAt = e.clock.Now()
	err = e.meta.SaveSyncState(state)
	if err != nil {
		return err
	}
	now := e.clock.Now()
	return e.store.PurgeTombstones(now.Add(-e.tombstoneRetention), now)
}

func (e *SyncEngine) pushTodo(t *domain.Todo) error {
	if t.Deleted {
		return e.pushDeleted(t)
	}
	if t.ReminderID.IsPresent() {
		return e.pushUpdate(t)
	}
	return e.pushCreate(t)
}

func (e *SyncEngine) pushDeleted(t *domain.Todo) error {
	if err := e.gateway.Delete(t.ReminderID.ValueOr("")); err != nil {
		return err
	}
	return e.saveSnapshots(t)
}

func (e *SyncEngine) pushCreate(t *domain.Todo) error {
	rid, err := e.createReminder(t)
	if err != nil {
		return err
	}
	t.ReminderID = option.Some(rid)
	return e.saveSnapshots(t)
}

func (e *SyncEngine) pushUpdate(t *domain.Todo) error {
	err := e.gateway.Update(e.fullUpdateParams(t))
	if errors.Is(err, ErrReminderNotFound) {
		rid, createErr := e.createReminder(t)
		if createErr != nil {
			return createErr
		}
		t.ReminderID = option.Some(rid)
	} else if err != nil {
		return err
	}
	return e.saveSnapshots(t)
}

func (e *SyncEngine) saveSnapshots(t *domain.Todo) error {
	st := snapshotFromTodo(t)
	snapshot, err := snapshotJSON(st)
	if err != nil {
		return fmt.Errorf("encode todo %q snapshot: %w", t.ID, err)
	}
	t.RemoteSnapshot = snapshot
	t.LocalSnapshot = snapshot
	return e.store.Put(t)
}

func (e *SyncEngine) resolveListID(t *domain.Todo) (string, error) {
	if t.ListID.IsPresent() {
		return t.ListID.ValueOr(""), nil
	}
	list, err := e.gateway.EnsureList(t.ListName)
	if err != nil {
		return "", err
	}
	t.ListID = option.Some(list.ID)
	return list.ID, nil
}

func (e *SyncEngine) createReminder(t *domain.Todo) (string, error) {
	listID, err := e.resolveListID(t)
	if err != nil {
		return "", err
	}
	return e.gateway.Create(CreateReminderParams{
		ListID:  listID,
		Title:   t.Title,
		Notes:   t.Description,
		DueDate: t.Deadline,
	})
}

func (e *SyncEngine) fullUpdateParams(t *domain.Todo) UpdateReminderParams {
	return UpdateReminderParams{
		ID:             t.ReminderID.ValueOr(""),
		Title:          option.Some(t.Title),
		Notes:          option.Some(t.Description),
		ListID:         t.ListID,
		DueDate:        option.Some(t.Deadline),
		Completed:      option.Some(t.Done),
		CompletionDate: option.Some(t.CompletedAt),
	}
}

// PullChanges fetches Reminders state and merges it into the local store.
func (e *SyncEngine) PullChanges() error {
	state, err := e.meta.GetSyncState()
	if err != nil {
		return err
	}
	lists, err := e.gateway.ListLists()
	if err != nil {
		return err
	}
	nameByID := map[string]string{}
	for _, l := range lists {
		nameByID[l.ID] = l.Name
	}
	defaultListID := ""
	for _, l := range lists {
		if l.Name == e.defaultListName {
			defaultListID = l.ID
		}
	}
	if defaultListID == "" {
		var list ReminderList
		list, err = e.gateway.EnsureList(e.defaultListName)
		if err != nil {
			return err
		}
		defaultListID = list.ID
		nameByID[defaultListID] = e.defaultListName
	}

	todos, err := e.store.List()
	if err != nil {
		return err
	}
	tracked := map[string]bool{defaultListID: true}
	for _, t := range todos {
		if t.ListID.IsPresent() {
			tracked[t.ListID.ValueOr("")] = true
		}
	}
	trackedIDs := make([]string, 0, len(tracked))
	for id := range tracked {
		trackedIDs = append(trackedIDs, id)
	}

	reminders, err := e.gateway.Pull(trackedIDs)
	if err != nil {
		return err
	}
	now := e.clock.Now()
	if mergeErr := e.mergeReminders(todos, reminders, nameByID, now); mergeErr != nil {
		return mergeErr
	}
	state.LastPullAt = now
	return e.meta.SaveSyncState(state)
}

func (e *SyncEngine) mergeReminders(
	todos []*domain.Todo,
	reminders []RemoteReminder,
	nameByID map[string]string,
	now time.Time,
) error {
	byID := map[string]RemoteReminder{}
	for _, r := range reminders {
		byID[r.ID] = r
	}
	for _, t := range todos {
		if !t.ReminderID.IsPresent() {
			continue
		}
		rid := t.ReminderID.ValueOr("")
		r, found := byID[rid]
		if !found {
			if err := e.tombstoneIfUnchanged(t, now); err != nil {
				return err
			}
			continue
		}
		if err := e.mergeFields(t, r, nameByID, now); err != nil {
			return err
		}
		delete(byID, rid)
	}
	for _, r := range byID {
		if err := e.importReminder(r, nameByID, now); err != nil {
			return err
		}
	}
	return nil
}

func (e *SyncEngine) tombstoneIfUnchanged(t *domain.Todo, now time.Time) error {
	prevLocal, err := parseSnapshot(t.LocalSnapshot)
	if err != nil {
		return fmt.Errorf("parse local snapshot for todo %q: %w", t.ID, err)
	}
	local := snapshotFromTodo(t)
	if t.Deleted || len(diffSnapshots(prevLocal, local)) > 0 {
		return nil
	}
	t.Deleted = true
	t.DeletedAt = option.Some(now)
	t.UpdatedAt = now
	st := snapshotFromTodo(t)
	snapshot, err := snapshotJSON(st)
	if err != nil {
		return fmt.Errorf("encode todo %q snapshot: %w", t.ID, err)
	}
	t.RemoteSnapshot = snapshot
	t.LocalSnapshot = snapshot
	return e.store.Put(t)
}

func (e *SyncEngine) mergeFields(t *domain.Todo, r RemoteReminder, nameByID map[string]string, now time.Time) error {
	prevRemote, err := parseSnapshot(t.RemoteSnapshot)
	if err != nil {
		return fmt.Errorf("parse remote snapshot for todo %q: %w", t.ID, err)
	}
	prevLocal, err := parseSnapshot(t.LocalSnapshot)
	if err != nil {
		return fmt.Errorf("parse local snapshot for todo %q: %w", t.ID, err)
	}
	remote := snapshotFromReminder(r)
	local := snapshotFromTodo(t)
	remoteChanged := diffSnapshots(prevRemote, remote)
	localChanged := diffSnapshots(prevLocal, local)
	for field := range remoteChanged {
		if _, changedLocally := localChanged[field]; !changedLocally {
			applyField(t, field, remote)
		}
	}
	if name, ok := nameByID[remote.ListID]; ok && t.ListName == "" {
		t.ListName = name
	}
	t.UpdatedAt = now
	t.RemoteSnapshot, err = snapshotJSON(remote)
	if err != nil {
		return fmt.Errorf("encode remote snapshot for todo %q: %w", t.ID, err)
	}
	t.LocalSnapshot, err = snapshotJSON(snapshotFromTodo(t))
	if err != nil {
		return fmt.Errorf("encode local snapshot for todo %q: %w", t.ID, err)
	}
	return e.store.Put(t)
}

func (e *SyncEngine) importReminder(r RemoteReminder, nameByID map[string]string, now time.Time) error {
	todo := domain.NewTodo(r.Title)
	todo.ReminderID = option.Some(r.ID)
	todo.ListID = option.Some(r.ListID)
	todo.ListName = nameByID[r.ListID]
	if todo.ListName == "" {
		todo.ListName = "Reminders"
	}
	todo.CreatedAt = now
	todo.UpdatedAt = now
	state := snapshotFromReminder(r)
	applySnapshot(todo, state)
	remoteSnapshot, err := snapshotJSON(state)
	if err != nil {
		return fmt.Errorf("encode remote snapshot for imported todo %q: %w", todo.ID, err)
	}
	localSnapshot, err := snapshotJSON(snapshotFromTodo(todo))
	if err != nil {
		return fmt.Errorf("encode local snapshot for imported todo %q: %w", todo.ID, err)
	}
	todo.RemoteSnapshot = remoteSnapshot
	todo.LocalSnapshot = localSnapshot
	return e.store.Put(todo)
}

// FullSync pulls remote changes first, then pushes local ones.
func (e *SyncEngine) FullSync() error {
	if err := e.PullChanges(); err != nil {
		return err
	}
	return e.PushChanges()
}
