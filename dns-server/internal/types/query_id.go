package types

import (
	"strconv"
	"time"

	"github.com/mikhailv/keenetic-dns/internal/util"
)

type QueryID uint64

var queryIDEpoch = time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)

const queryIDCounterBits uint = 24

func NewQueryID(now time.Time, counter uint64) QueryID {
	return QueryID(util.PackEventID(queryIDEpoch, now, queryIDCounterBits, counter))
}

func (id QueryID) Time() time.Time {
	return queryIDEpoch.Add(time.Duration(id>>queryIDCounterBits) * time.Millisecond)
}

func (id QueryID) String() string {
	return strconv.FormatUint(uint64(id), 36)
}

func (id QueryID) MarshalText() ([]byte, error) {
	return []byte(id.String()), nil
}

func (id *QueryID) UnmarshalText(b []byte) error {
	v, err := strconv.ParseUint(string(b), 36, 64)
	if err != nil {
		return err
	}
	*id = QueryID(v)
	return nil
}
