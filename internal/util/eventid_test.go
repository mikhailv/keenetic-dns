package util

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

var testEpoch = time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)

func TestPackEventID_RoundTripsTimeAndCounter(t *testing.T) {
	const bits = 20
	at := time.Date(2026, time.August, 15, 12, 34, 56, 789_000_000, time.UTC)

	id := PackEventID(testEpoch, at, bits, 42)
	assert.Equal(t, at, EventIDTime(testEpoch, bits, id))
	assert.Equal(t, uint64(42), id&(1<<bits-1))
}

func TestPackEventID_OrdersByTime(t *testing.T) {
	const bits = 20
	at := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)

	first := PackEventID(testEpoch, at, bits, 1)
	second := PackEventID(testEpoch, at, bits, 2)
	later := PackEventID(testEpoch, at.Add(time.Millisecond), bits, 0)

	assert.Less(t, first, second)
	assert.Less(t, second, later)
}

func TestPackEventID_CounterWrapsWithoutCarrying(t *testing.T) {
	const bits = 20
	at := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)

	wrapped := PackEventID(testEpoch, at, bits, 1<<bits)
	assert.Equal(t, at, EventIDTime(testEpoch, bits, wrapped))
	assert.Equal(t, PackEventID(testEpoch, at, bits, 0), wrapped)
}

func TestPackEventID_ClampsPreEpoch(t *testing.T) {
	before := time.Date(1970, time.January, 1, 0, 0, 0, 0, time.UTC)
	assert.Zero(t, PackEventID(testEpoch, before, 20, 0))
}
