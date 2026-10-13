// Package option provides a generic value that can be present or absent.
package option

import (
	"bytes"
	"encoding/json"
)

// Option represents a value that may be absent. Its zero value is None.
// None marshals as JSON null unless the containing field uses omitzero.
type Option[T any] struct {
	value   T
	present bool
}

// Some returns an Option containing value.
func Some[T any](value T) Option[T] {
	return Option[T]{value: value, present: true}
}

// None returns an absent Option.
func None[T any]() Option[T] {
	return Option[T]{}
}

// Get returns the value and whether it is present.
func (o Option[T]) Get() (T, bool) {
	return o.value, o.present
}

// IsPresent reports whether the Option contains a value.
func (o Option[T]) IsPresent() bool {
	return o.present
}

// ValueOr returns the contained value or fallback when absent.
func (o Option[T]) ValueOr(fallback T) T {
	if o.present {
		return o.value
	}
	return fallback
}

// IsZero reports whether the Option is absent, allowing omitzero JSON tags.
func (o Option[T]) IsZero() bool {
	return !o.present
}

// MarshalJSON encodes None as null and Some(value) as value.
func (o Option[T]) MarshalJSON() ([]byte, error) {
	if !o.present {
		return []byte("null"), nil
	}
	return json.Marshal(o.value)
}

// UnmarshalJSON decodes null as None and any other valid JSON value as Some.
func (o *Option[T]) UnmarshalJSON(data []byte) error {
	if bytes.Equal(bytes.TrimSpace(data), []byte("null")) {
		var zero T
		o.value = zero
		o.present = false
		return nil
	}
	if err := json.Unmarshal(data, &o.value); err != nil {
		return err
	}
	o.present = true
	return nil
}
