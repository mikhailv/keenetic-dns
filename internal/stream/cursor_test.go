package stream

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCursor_CarriesItsTime(t *testing.T) {
	at := time.Date(2026, time.August, 15, 12, 34, 56, 789_000_000, time.UTC)
	c := NewCursor(at, 42)

	assert.Equal(t, at, c.Time())
	assert.Equal(t, uint64(42), uint64(c)&(1<<cursorCounterBits-1), "the counter occupies the low bits")
}

func TestCursor_Increases(t *testing.T) {
	base := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)

	first := NewCursor(base, 1)
	second := NewCursor(base, 2)
	later := NewCursor(base.Add(time.Millisecond), 0)

	assert.Less(t, uint64(first), uint64(second))
	assert.Less(t, uint64(second), uint64(later), "a later millisecond outranks any counter within an earlier one")
}

func TestCursor_CounterWraps(t *testing.T) {
	at := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)

	wrapped := NewCursor(at, 1<<cursorCounterBits)
	assert.Equal(t, at, wrapped.Time(), "wrapping must not carry into the timestamp")
	assert.Equal(t, NewCursor(at, 0), wrapped)
}

func TestCursor_TimestampFits(t *testing.T) {
	const msBits = 64 - cursorCounterBits

	now := NewCursor(time.Now(), 0)
	assert.Equal(t, time.Now().UnixMilli()/1000, now.Time().Unix(), "today round-trips")

	years := float64(uint64(1)<<msBits-1) / 1000 / 86400 / 365.25
	assert.Greater(t, years, 500.0, "the layout must not expire in service")
}

func TestBuffered_CursorsAlwaysIncrease(t *testing.T) {
	s := NewBufferedStream[testEntry](8)

	var last Cursor
	for i := range 1000 {
		s.Append(testEntry{})
		res := s.QueryBackward(^Cursor(0), 1, nil)
		require.Len(t, res.Items, 1, "append %d", i)
		require.Greater(t, uint64(res.LastCursor), uint64(last), "cursor went backwards at append %d", i)
		last = res.LastCursor
	}
}

type testEntry struct {
	Cursor Cursor
}

func (e *testEntry) SetCursor(c Cursor) { e.Cursor = c }
