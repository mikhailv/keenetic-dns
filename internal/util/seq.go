package util

import (
	"iter"
	"slices"
)

func SeqToSlice[T any](len int, seq iter.Seq[T]) []T {
	return slices.AppendSeq(make([]T, 0, len), seq)
}
