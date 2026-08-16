package types

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQueryID_CarriesItsTime(t *testing.T) {
	issued := time.Date(2026, time.August, 15, 12, 34, 56, 789_000_000, time.UTC)
	id := NewQueryID(issued, 42)

	assert.Equal(t, issued, id.Time())
	assert.Equal(t, uint64(42), uint64(id)&(1<<queryIDCounterBits-1), "the counter occupies the low bits")
}

func TestQueryID_OrdersByTime(t *testing.T) {
	base := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)

	first := NewQueryID(base, 1)
	second := NewQueryID(base, 2)
	later := NewQueryID(base.Add(time.Millisecond), 0)

	assert.Less(t, uint64(first), uint64(second))
	assert.Less(t, uint64(second), uint64(later), "a later millisecond outranks any counter within an earlier one")
}

func TestQueryID_CounterWraps(t *testing.T) {
	at := time.Date(2026, time.August, 15, 0, 0, 0, 0, time.UTC)

	last := NewQueryID(at, 1<<queryIDCounterBits-1)
	wrapped := NewQueryID(at, 1<<queryIDCounterBits)

	assert.Equal(t, at, wrapped.Time(), "wrapping must not carry into the timestamp")
	assert.Equal(t, NewQueryID(at, 0), wrapped)
	assert.NotEqual(t, last, wrapped)
}

func TestQueryID_EpochRange(t *testing.T) {
	const msBits = 64 - queryIDCounterBits
	last := queryIDEpoch.Add(time.Duration(uint64(1)<<msBits-1) * time.Millisecond)
	assert.Equal(t, 2060, last.Year(), "40 bits of milliseconds reach ~34.8 years past the epoch")
}

func TestQueryID_MarshalsAsText(t *testing.T) {
	id := NewQueryID(time.Date(2026, time.August, 15, 12, 0, 0, 0, time.UTC), 7)
	require.Greater(t, uint64(id), uint64(1)<<53, "the case that forces text encoding")

	data, err := json.Marshal(struct {
		ID QueryID `json:"id"`
	}{id})
	require.NoError(t, err)
	assert.Contains(t, string(data), `"id":"`+id.String()+`"`)

	var back struct {
		ID QueryID `json:"id"`
	}
	require.NoError(t, json.Unmarshal(data, &back))
	assert.Equal(t, id, back.ID, "round-trips without losing precision")
}
