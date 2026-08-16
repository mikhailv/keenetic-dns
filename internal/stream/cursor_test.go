package stream

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	for i := range 1<<cursorCounterBits + 100 {
		s.Append(testEntry{})
		if s.lastCursor <= last {
			t.Fatalf("cursor went backwards at append %d: %016x <= %016x", i, uint64(s.lastCursor), uint64(last))
		}
		last = s.lastCursor
	}
}

func TestBuffered_CursorsIncreaseWhenClockGoesBackwards(t *testing.T) {
	s := NewBufferedStream[testEntry](8)

	s.Append(testEntry{})
	first := s.QueryBackward(^Cursor(0), 1, nil).LastCursor

	s.lastCursor = NewCursor(time.Now().Add(time.Hour), 0)
	ahead := s.lastCursor

	s.Append(testEntry{})
	after := s.QueryBackward(^Cursor(0), 1, nil).LastCursor

	require.Greater(t, uint64(after), uint64(ahead), "an earlier clock must not produce an earlier cursor")
	require.Greater(t, uint64(ahead), uint64(first))
}

type testEntry struct {
	Cursor Cursor
}

func (e *testEntry) SetCursor(c Cursor) { e.Cursor = c }
