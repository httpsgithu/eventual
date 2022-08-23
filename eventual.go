package eventual

import (
	"context"
	"sync"
	"time"
)

// DontWait is an expired context for use in Value.Get. Using DontWait will cause a Value.Get call
// to return immediately. If the value has not been set, a context.Canceled error will be returned.
var DontWait context.Context

const (
	tenYears = 10 * 365 * 24 * time.Hour
)

func init() {
	var cancel func()
	DontWait, cancel = context.WithCancel(context.Background())
	cancel()
}

// Value is an eventual value, meaning that callers wishing to access the value block until it is
// available.
type Value[T comparable] interface {
	// Set this Value.
	Set(T)

	// Set this Value, expiring at the given time.
	SetExpiring(T, time.Time)

	// Reset clears the currently set value, reverting to the same state as if the Eventual had just
	// been created.
	Reset()

	// Get waits for the value to be set. If the context expires first, an error will be returned.
	//
	// This function will return immediately when called with an expired context. In this case, the
	// value will be returned only if it has already been set; otherwise the context error will be
	// returned. For convenience, see DontWait.
	Get(context.Context) (T, error)

	// Gets the stored value, or if none available, runs the given func, stores the value as an expiring value,
	// and returns the result. If func() returns an error, nothing is stored and the error is returned to caller.
	GetOrSetExpiring(time.Time, func() (T, error)) (T, error)
}

// NewValue creates a new value.
func NewValue[T comparable]() Value[T] {
	return &value[T]{}
}

// WithDefault creates a new value that returns the given defaultValue if a real value isn't
// available in time.
func WithDefault[T comparable](defaultValue T) Value[T] {
	return &value[T]{defaultValue: defaultValue}
}

type value[T comparable] struct {
	m            sync.Mutex
	v            T
	zeroValue    T
	defaultValue T
	expiration   time.Time
	set          bool
	waiters      []chan T
}

func (v *value[T]) Set(i T) {
	v.SetExpiring(i, time.Now().Add(tenYears))
}

func (v *value[T]) SetExpiring(i T, t time.Time) {
	v.m.Lock()
	v.doSetExpiring(i, t)
	v.m.Unlock()
}

func (v *value[T]) doSetExpiring(i T, t time.Time) {
	v.v = i
	if !v.set {
		// This is our first time setting, inform anyone who is waiting
		for _, waiter := range v.waiters {
			waiter <- i
		}
		v.waiters = make([]chan T, 0)
		v.expiration = t
		v.set = true
	}
}

func (v *value[T]) Reset() {
	v.m.Lock()
	v.v = v.zeroValue
	v.expiration = time.Time{}
	v.set = false
	v.m.Unlock()
}

func (v *value[T]) Get(ctx context.Context) (T, error) {
	v.m.Lock()
	if v.set {
		if v.expiration.IsZero() || v.expiration.After(time.Now()) {
			// Value already set, use existing
			_v := v.v
			v.m.Unlock()
			return _v, nil
		}
	}

	// Value not yet set, wait
	waiter := make(chan T, 1)
	v.waiters = append(v.waiters, waiter)
	v.m.Unlock()
	select {
	case _v := <-waiter:
		return _v, nil
	case <-ctx.Done():
		if v.defaultValue != v.zeroValue {
			return v.defaultValue, nil
		}
		return v.defaultValue, ctx.Err()
	}
}

func (v *value[T]) GetOrSetExpiring(t time.Time, getter func() (T, error)) (T, error) {
	v.m.Lock()
	if v.set {
		if v.expiration.IsZero() || v.expiration.After(time.Now()) {
			// Value already set, use existing
			_v := v.v
			v.m.Unlock()
			return _v, nil
		}
	}

	// Value not yet set, get it
	i, err := getter()
	if err != nil {
		v.m.Unlock()
		return v.zeroValue, err
	}
	v.doSetExpiring(i, t)
	v.m.Unlock()
	return i, nil
}
