package util

type LazyMap[K comparable, V any] map[K]V

func (m *LazyMap[K, V]) Set(key K, value V) {
	if *m == nil {
		*m = map[K]V{}
	}
	(*m)[key] = value
}
