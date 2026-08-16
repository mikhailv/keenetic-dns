package chunkstore

import (
	"fmt"
	"time"
)

type Timestamp uint32

func (t Timestamp) Time() time.Time {
	return time.Unix(int64(t), 0)
}

type TimeRange struct {
	_     struct{}  `cbor:",toarray"`
	Start Timestamp `json:"start"`
	End   Timestamp `json:"end"`
}

func (s TimeRange) IsZero() bool {
	return s == (TimeRange{})
}

func (s TimeRange) StartTime() time.Time {
	return s.Start.Time()
}

func (s TimeRange) EndTime() time.Time {
	return s.End.Time()
}

func (s TimeRange) InRange(t Timestamp) bool {
	return s.Start <= t && t <= s.End
}

func (s TimeRange) Intersects(other TimeRange) bool {
	return s.Start <= other.End && s.End >= other.Start
}

func (s TimeRange) Valid() bool {
	return s.Start <= s.End && s.End > 0
}

func (s TimeRange) String() string {
	return fmt.Sprintf("%d-%d", s.Start, s.End)
}
