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
type Value[V comparable] interface {
	// Set this Value.
	Set(value V)

	// Set this Value, expiring at the given time.
	SetExpiring(value V, expiration time.Time)

	// Reset clears the currently set value, reverting to the same state as if the Eventual had just
	// been created.
	Reset()

	// Get waits for the value to be set. If the context expires first, an error will be returned.
	//
	// This function will return immediately when called with an expired context. In this case, the
	// value will be returned only if it has already been set; otherwise the context error will be
	// returned. For convenience, see DontWait.
	Get(context.Context) (V, error)

	// Gets the stored value, or if none available, runs the given func, stores the value as an expiring value,
	// and returns the result. If func() returns an error, nothing is stored and the error is returned to caller.
	GetOrSetExpiring(expiration time.Time, getter func() (V, error)) (V, error)
}

// NewValue creates a new value.
func NewValue[V comparable]() Value[V] {
	return &value[V]{}
}

// WithDefault creates a new value that returns the given defaultValue if a real value isn't
// available in time.
func WithDefault[V comparable](defaultValue V) Value[V] {
	return &value[V]{defaultValue: defaultValue}
}

type value[V comparable] struct {
	m            sync.Mutex
	v            V
	zeroValue    V
	defaultValue V
	expiration   time.Time
	set          bool
	waiters      []chan V
}

func (v *value[V]) Set(i V) {
	v.SetExpiring(i, time.Now().Add(tenYears))
}

func (v *value[V]) SetExpiring(i V, t time.Time) {
	v.m.Lock()
	v.doSetExpiring(i, t)
	v.m.Unlock()
}

func (v *value[V]) doSetExpiring(i V, t time.Time) {
	v.v = i
	if !v.set {
		// This is our first time setting, inform anyone who is waiting
		for _, waiter := range v.waiters {
			waiter <- i
		}
		v.waiters = make([]chan V, 0)
		v.expiration = t
		v.set = true
	}
}

func (v *value[V]) Reset() {
	v.m.Lock()
	v.v = v.zeroValue
	v.expiration = time.Time{}
	v.set = false
	v.m.Unlock()
}

func (v *value[V]) Get(ctx context.Context) (V, error) {
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
	waiter := make(chan V, 1)
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

func (v *value[V]) GetOrSetExpiring(t time.Time, getter func() (V, error)) (V, error) {
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
