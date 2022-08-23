package eventual

import (
	"context"
	"sync"
	"time"
)

// Map is a map of eventual values, meaning that callers wishing to access the value block until it is
// available.
type Map[K comparable, V comparable] interface {
	// Set the Value at key.
	Set(key K, value V)

	// Set the Value at key, expiring at the given time.
	SetExpiring(key K, value V, expiration time.Time)

	// Reset clears the currently set value at key, reverting to the same state as if the Eventual had just
	// been created.
	Reset(key K)

	// Get waits for the value to be set. If the context expires first, an error will be returned.
	//
	// This function will return immediately when called with an expired context. In this case, the
	// value will be returned only if it has already been set; otherwise the context error will be
	// returned. For convenience, see DontWait.
	Get(ctx context.Context, key K) (V, error)

	// Gets the stored value at key, or if none available, runs the given func, stores the value as an expiring value,
	// and returns the result. If func() returns an error, nothing is stored and the error is returned to caller.
	GetOrSetExpiring(key K, expiration time.Time, getter func() (V, error)) (V, error)
}

type emap[K comparable, V comparable] struct {
	m  map[K]Value[V]
	mx sync.Mutex
}

func NewMap[K comparable, V comparable]() Map[K, V] {
	return &emap[K, V]{
		m: make(map[K]Value[V]),
	}
}
func (m *emap[K, V]) Set(key K, value V) {
	v := m.getValue(key)
	v.Set(value)
}

func (m *emap[K, V]) SetExpiring(key K, value V, expiration time.Time) {
	v := m.getValue(key)
	v.SetExpiring(value, expiration)
}

func (m *emap[K, V]) Reset(key K) {
	v := m.getValue(key)
	v.Reset()
}

func (m *emap[K, V]) Get(ctx context.Context, key K) (V, error) {
	v := m.getValue(key)
	return v.Get(ctx)
}

func (m *emap[K, V]) GetOrSetExpiring(key K, expiration time.Time, getter func() (V, error)) (V, error) {
	v := m.getValue(key)
	return v.GetOrSetExpiring(expiration, getter)
}

func (m *emap[K, V]) getValue(key K) Value[V] {
	m.mx.Lock()
	defer m.mx.Unlock()

	result := m.m[key]
	if result == nil {
		result = NewValue[V]()
		m.m[key] = result
	}

	return result
}
