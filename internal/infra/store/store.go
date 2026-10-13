// Package store persists todos in a local bbolt database.
package store

import (
	"encoding/json"
	"fmt"
	"sort"
	"time"

	bolt "go.etcd.io/bbolt"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/internal/service"
)

const (
	bucketTodosName  = "todos"
	bucketMetaName   = "meta"
	keySyncStateName = "sync_state"

	dbFileMode = 0o600
)

func Open(path string) (*bolt.DB, error) {
	db, err := bolt.Open(path, dbFileMode, nil)
	if err != nil {
		return nil, fmt.Errorf("open bbolt db: %w", err)
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, name := range [][]byte{[]byte(bucketTodosName), []byte(bucketMetaName)} {
			if _, createErr := tx.CreateBucketIfNotExists(name); createErr != nil {
				return createErr
			}
		}
		return nil
	})
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create buckets: %w", err)
	}
	return db, nil
}

type TodoStore struct {
	db *bolt.DB
}

func NewTodoStore(db *bolt.DB) *TodoStore {
	return &TodoStore{db: db}
}

func (s *TodoStore) Get(id string) (*domain.Todo, error) {
	var todo *domain.Todo
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketTodosName))
		data := b.Get([]byte(id))
		if data == nil {
			return service.ErrNotFound
		}
		var t domain.Todo
		if err := json.Unmarshal(data, &t); err != nil {
			return fmt.Errorf("decode todo %s: %w", id, err)
		}
		todo = &t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return todo, nil
}

func (s *TodoStore) List() ([]*domain.Todo, error) {
	todos := make([]*domain.Todo, 0)
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketTodosName))
		return b.ForEach(func(_, v []byte) error {
			var t domain.Todo
			if err := json.Unmarshal(v, &t); err != nil {
				return fmt.Errorf("decode todo: %w", err)
			}
			todos = append(todos, &t)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(todos, func(i, j int) bool { return todos[i].CreatedAt.Before(todos[j].CreatedAt) })
	return todos, nil
}

func (s *TodoStore) ListByReminderID(rid string) (*domain.Todo, error) {
	todos, err := s.List()
	if err != nil {
		return nil, err
	}
	for _, t := range todos {
		if t.ReminderID.IsPresent() && t.ReminderID.ValueOr("") == rid {
			return t, nil
		}
	}
	return nil, service.ErrNotFound
}

func (s *TodoStore) Put(todo *domain.Todo) error {
	data, err := json.Marshal(todo)
	if err != nil {
		return fmt.Errorf("encode todo: %w", err)
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketTodosName)).Put([]byte(todo.ID), data)
	})
}

func (s *TodoStore) PurgeTombstones(cutoff time.Time, lastPushAt time.Time) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucketTodosName))
		var toDelete [][]byte
		err := b.ForEach(func(k, v []byte) error {
			var t domain.Todo
			if err := json.Unmarshal(v, &t); err != nil {
				return err
			}
			if t.Deleted && t.DeletedAt.IsPresent() &&
				t.DeletedAt.ValueOr(time.Time{}).Before(cutoff) &&
				!t.UpdatedAt.After(lastPushAt) {
				toDelete = append(toDelete, k)
			}
			return nil
		})
		if err != nil {
			return err
		}
		for _, k := range toDelete {
			if delErr := b.Delete(k); delErr != nil {
				return delErr
			}
		}
		return nil
	})
}

func (s *TodoStore) GetSyncState() (domain.SyncState, error) {
	var state domain.SyncState
	err := s.db.View(func(tx *bolt.Tx) error {
		data := tx.Bucket([]byte(bucketMetaName)).Get([]byte(keySyncStateName))
		if data == nil {
			return nil
		}
		return json.Unmarshal(data, &state)
	})
	return state, err
}

func (s *TodoStore) SaveSyncState(state domain.SyncState) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket([]byte(bucketMetaName)).Put([]byte(keySyncStateName), data)
	})
}
