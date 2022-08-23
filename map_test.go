package eventual

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestMap(t *testing.T) {
	m := NewMap[string, int]()

	m.Set("a", 1)
	m.Set("b", 2)

	a, err := m.Get(DontWait, "a")
	require.NoError(t, err)
	require.Equal(t, 1, a)

	b, err := m.Get(DontWait, "b")
	require.NoError(t, err)
	require.Equal(t, 2, b)

	c, err := m.Get(DontWait, "c")
	require.Error(t, err)
	require.Equal(t, 0, c)
}
