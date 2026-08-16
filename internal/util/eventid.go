package util

import "time"

func PackEventID(epoch, now time.Time, counterBits uint, counter uint64) uint64 {
	ms := max(now.Sub(epoch).Milliseconds(), 0)
	return uint64(ms)<<counterBits | counter&(1<<counterBits-1)
}

func EventIDTime(epoch time.Time, counterBits uint, id uint64) time.Time {
	return epoch.Add(time.Duration(id>>counterBits) * time.Millisecond)
}
