package types

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
