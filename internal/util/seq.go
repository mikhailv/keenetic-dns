package util

import (
	"iter"
	"slices"
)

func SeqToSlice[T any](initCapacity int, seq iter.Seq[T]) []T {
	return slices.AppendSeq(make([]T, 0, initCapacity), seq)
}

func Seq2ToSlice[T any](initCapacity int, seq iter.Seq2[T, error]) ([]T, error) {
	res := make([]T, 0, initCapacity)
	for v, err := range seq {
		if err != nil {
			return nil, err
		}
		res = append(res, v)
	}
	return res, nil
}
