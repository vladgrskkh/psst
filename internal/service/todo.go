package service

import (
	"errors"
	"strings"
	"time"

	"github.com/vladgrskkh/psst/internal/domain"
	"github.com/vladgrskkh/psst/pkg/option"
)

var (
	ErrNotFound       = errors.New("todo not found")
	ErrInvalidTitle   = errors.New("title must not be empty")
	ErrDeadlineInPast = errors.New("deadline must be in the future")
	ErrInvalidList    = errors.New("list name must not be empty")
)

// TodoStore is the persistence contract used by TodoService.
type TodoStore interface {
	Get(id string) (*domain.Todo, error)
	List() ([]*domain.Todo, error)
	Put(todo *domain.Todo) error
}

// Clock supplies timestamps to application services.
type Clock interface {
	Now() time.Time
}

type TodoService struct {
	store TodoStore
	clock Clock
}

func NewTodoService(store TodoStore, clock Clock) *TodoService {
	return &TodoService{store: store, clock: clock}
}

type CreateTodoParams struct {
	Title       string
	Description string
	Deadline    option.Option[time.Time]
	ListName    string
}

func (s *TodoService) Create(p CreateTodoParams) (*domain.Todo, error) {
	title := strings.TrimSpace(p.Title)
	list := strings.TrimSpace(p.ListName)
	if err := validate(title, list, p.Deadline, s.clock.Now()); err != nil {
		return nil, err
	}
	now := s.clock.Now()
	todo := domain.NewTodo(title)
	todo.Description = strings.TrimSpace(p.Description)
	todo.Deadline = p.Deadline
	todo.ListName = list
	todo.CreatedAt = now
	todo.UpdatedAt = now
	if err := s.store.Put(todo); err != nil {
		return nil, err
	}
	return todo, nil
}

type UpdateTodoParams struct {
	Title       option.Option[string]
	Description option.Option[string]
	Deadline    option.Option[option.Option[time.Time]]
	ListName    option.Option[string]
}

func (s *TodoService) Update(id string, p UpdateTodoParams) (*domain.Todo, error) {
	todo, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	if p.Title.IsPresent() {
		todo.Title = strings.TrimSpace(p.Title.ValueOr(""))
	}
	if p.Description.IsPresent() {
		todo.Description = strings.TrimSpace(p.Description.ValueOr(""))
	}
	if p.Deadline.IsPresent() {
		todo.Deadline = p.Deadline.ValueOr(option.None[time.Time]())
	}
	if p.ListName.IsPresent() {
		todo.ListName = strings.TrimSpace(p.ListName.ValueOr(""))
		todo.ListID = option.None[string]()
	}
	err = validate(todo.Title, todo.ListName, todo.Deadline, s.clock.Now())
	if err != nil {
		return nil, err
	}
	todo.UpdatedAt = s.clock.Now()
	err = s.store.Put(todo)
	if err != nil {
		return nil, err
	}
	return todo, nil
}

func (s *TodoService) ToggleDone(id string) (*domain.Todo, error) {
	todo, err := s.store.Get(id)
	if err != nil {
		return nil, err
	}
	now := s.clock.Now()
	todo.Done = !todo.Done
	if todo.Done {
		todo.CompletedAt = option.Some(now)
	} else {
		todo.CompletedAt = option.None[time.Time]()
	}
	todo.UpdatedAt = now
	err = s.store.Put(todo)
	if err != nil {
		return nil, err
	}
	return todo, nil
}

func (s *TodoService) Delete(id string) error {
	todo, err := s.store.Get(id)
	if err != nil {
		return err
	}
	now := s.clock.Now()
	todo.Deleted = true
	todo.DeletedAt = option.Some(now)
	todo.UpdatedAt = now
	return s.store.Put(todo)
}

func (s *TodoService) List(filter string) ([]*domain.Todo, error) {
	all, err := s.store.List()
	if err != nil {
		return nil, err
	}
	filter = strings.ToLower(strings.TrimSpace(filter))
	out := make([]*domain.Todo, 0, len(all))
	for _, t := range all {
		if t.Deleted {
			continue
		}
		if filter != "" && !strings.Contains(strings.ToLower(t.Title), filter) &&
			!strings.Contains(strings.ToLower(t.Description), filter) {
			continue
		}
		out = append(out, t)
	}
	return out, nil
}

func validate(title, list string, deadline option.Option[time.Time], now time.Time) error {
	if title == "" {
		return ErrInvalidTitle
	}
	if list == "" {
		return ErrInvalidList
	}
	if deadline.IsPresent() && !deadline.ValueOr(time.Time{}).After(now) {
		return ErrDeadlineInPast
	}
	return nil
}
