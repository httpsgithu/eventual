package eventual

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSingle(t *testing.T) {
	t.Parallel()
	const (
		timeUntilSet = 100 * time.Millisecond
		v1           = "1"
		v2           = "2"
		v3           = "3"
		v4           = "4"
	)

	v := NewValue[string]()
	go func() {
		time.Sleep(timeUntilSet)
		v.Set(v1)
	}()

	shortTimeoutCtx, cancel := context.WithTimeout(context.Background(), timeUntilSet/2)
	defer cancel()

	_, err := v.Get(shortTimeoutCtx)
	require.Error(t, err, "Get with short timeout should have timed out")

	result, err := v.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, v1, result)

	v.Set(v2)
	result, err = v.Get(DontWait)
	require.NoError(t, err, "Get with expired context should have succeeded")
	require.Equal(t, v2, result)

	require.Zero(t, len(v.(*value[string]).waiters), "value should have no remaining waiters")

	v.Reset()
	_, err = v.Get(shortTimeoutCtx)
	require.Error(t, err, "Get with short timeout should have timed out after reset")

	go func() {
		time.Sleep(timeUntilSet)
		v.Set(v3)
	}()
	result, err = v.Get(context.Background())
	require.NoError(t, err, "Get after reset should have succeeded")
	require.Equal(t, v3, result)

	v.Reset()
	go func() {
		time.Sleep(timeUntilSet)
		v.Reset()
		v.Set(v4)
	}()
	result, err = v.Get(context.Background())
	require.NoError(t, err, "Get from before reset should have succeeded")
	require.Equal(t, v4, result)
}

// func TestExpiring(t *testing.T) {
// 	t.Parallel()
// 	v := NewValue[int]()

// 	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
// 	defer cancel()

// 	_, err := v.Get(ctx)
// 	require.Error(t, err, "Get before Set should return error")
// }

func TestNoSet(t *testing.T) {
	t.Parallel()
	v := NewValue[string]()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	_, err := v.Get(ctx)
	require.Error(t, err, "Get before Set should return error")
}

func TestCancel(t *testing.T) {
	t.Parallel()
	v := NewValue[string]()

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err := v.Get(ctx)
	require.Error(t, err, "Get should respect context cancellation")
}

func TestConcurrent(t *testing.T) {
	t.Parallel()
	const concurrency = 200

	var (
		v                  = NewValue[string]()
		setStart           = make(chan struct{})
		setGroup, getGroup sync.WaitGroup
	)

	go func() {
		// Do some concurrent setting to make sure that works.
		for i := 0; i < concurrency; i++ {
			setGroup.Add(1)
			go func() {
				// Coordinate on setStart to run all goroutines at once.
				<-setStart
				v.Set("some value")
				setGroup.Done()
			}()
		}
		close(setStart)
	}()

	failureChan := make(chan string, concurrency)
	for i := 0; i < concurrency; i++ {
		getGroup.Add(1)
		go func() {
			defer getGroup.Done()
			r, err := v.Get(context.Background())
			if err != nil {
				failureChan <- err.Error()
			} else if r != "some value" {
				failureChan <- fmt.Sprintf("wrong result: %s", r)
			}
		}()
	}
	getGroup.Wait()
	close(failureChan)

	failures := map[string]int{}
	for failure := range failureChan {
		failures[failure]++
	}
	for msg, count := range failures {
		t.Logf("%d failures with message '%s'", count, msg)
	}
	if len(failures) > 0 {
		t.FailNow()
	}

	// Ensure all Set calls returned.
	setGroup.Wait()
}

func TestSetExpiring(t *testing.T) {
	v := NewValue[string]()
	v.SetExpiring("hi", time.Now().Add(50*time.Millisecond))
	r, err := v.Get(DontWait)
	require.NoError(t, err)
	require.EqualValues(t, "hi", r)
	time.Sleep(50 * time.Millisecond)
	_, err = v.Get(DontWait)
	require.Error(t, err)
}

func TestGetOrSetExpiring(t *testing.T) {
	numSets := 0
	v := NewValue[string]()
	r, err := v.GetOrSetExpiring(time.Now().Add(50*time.Millisecond), func() (string, error) {
		return "", errors.New("i'm failing")
	})
	require.Error(t, err)
	r, err = v.GetOrSetExpiring(time.Now().Add(50*time.Millisecond), func() (string, error) {
		numSets++
		return "hi", nil
	})
	require.NoError(t, err)
	require.EqualValues(t, "hi", r)
	time.Sleep(100 * time.Millisecond)
	_, err = v.Get(DontWait)
	require.Error(t, err)
	for i := 0; i < 2; i++ {
		r, err = v.GetOrSetExpiring(time.Now().Add(50*time.Millisecond), func() (string, error) {
			numSets++
			return "hi2", nil
		})
		require.NoError(t, err)
		require.Equal(t, "hi2", r)
		require.Equal(t, 2, numSets)
	}
}

func TestWithDefault(t *testing.T) {
	t.Parallel()
	const (
		timeUntilSet = 100 * time.Millisecond
		defaultValue = "default value"
		initialValue = "initial value"
	)

	v := WithDefault(defaultValue)
	go func() {
		time.Sleep(timeUntilSet)
		v.Set(initialValue)
	}()

	shortTimeoutCtx, cancel := context.WithTimeout(context.Background(), timeUntilSet/2)
	defer cancel()

	result, err := v.Get(shortTimeoutCtx)
	require.NoError(t, err, "Get with short timeout should have gotten no error")
	require.Equal(t, defaultValue, result, "Get with short timeout should have gotten default value")

	result, err = v.Get(context.Background())
	require.NoError(t, err)
	require.Equal(t, initialValue, result)
}

func BenchmarkGet(b *testing.B) {
	v := NewValue[string]()
	v.Set("foo")
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		v.Get(ctx)
	}
}
