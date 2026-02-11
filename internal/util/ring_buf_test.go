package util

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRingBuf_AddAndGet(t *testing.T) {
	rb := NewRingBuf[int](5)
	assert.Equal(t, 5, rb.Capacity())
	assert.Equal(t, 0, rb.Size())

	rb.Add(10)
	assert.Equal(t, 1, rb.Size())
	assert.Equal(t, 10, rb.Get(0))
}

func TestRingBuf_AddBeyondCapacity(t *testing.T) {
	rb := NewRingBuf[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	assert.Equal(t, 3, rb.Size())

	rb.Add(4)
	rb.Add(5)

	assert.Equal(t, 3, rb.Size())
	assert.Equal(t, 3, rb.Get(0))
	assert.Equal(t, 4, rb.Get(1))
	assert.Equal(t, 5, rb.Get(2))
}

func TestRingBuf_MultipleWrapCycles(t *testing.T) {
	rb := NewRingBuf[int](3)
	for i := 1; i <= 10; i++ {
		rb.Add(i)
	}

	assert.Equal(t, 3, rb.Size())
	assert.Equal(t, []int{8, 9, 10}, rb.Values())
	assert.Equal(t, 8, rb.Get(0))
	assert.Equal(t, 9, rb.Get(1))
	assert.Equal(t, 10, rb.Get(2))
}

func TestRingBuf_CapacityOne(t *testing.T) {
	rb := NewRingBuf[int](1)
	assert.Equal(t, 1, rb.Capacity())
	assert.Equal(t, 0, rb.Size())

	rb.Add(1)
	assert.Equal(t, 1, rb.Size())
	assert.Equal(t, 1, rb.Get(0))

	rb.Add(2)
	assert.Equal(t, 1, rb.Size())
	assert.Equal(t, 2, rb.Get(0))

	rb.Add(3)
	assert.Equal(t, []int{3}, rb.Values())
}

func TestRingBuf_GetZeroValueForTypes(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		rb := NewRingBuf[int](3)
		rb.Add(1)
		assert.Equal(t, 0, rb.Get(-1))
		assert.Equal(t, 0, rb.Get(5))
	})

	t.Run("string", func(t *testing.T) {
		rb := NewRingBuf[string](3)
		rb.Add("hello")
		assert.Equal(t, "", rb.Get(-1))
		assert.Equal(t, "", rb.Get(5))
	})

	t.Run("pointer", func(t *testing.T) {
		rb := NewRingBuf[*int](3)
		v := 42
		rb.Add(&v)
		assert.Nil(t, rb.Get(-1))
		assert.Nil(t, rb.Get(5))
	})

	t.Run("struct", func(t *testing.T) {
		type Item struct {
			Name string
			Val  int
		}
		rb := NewRingBuf[Item](3)
		rb.Add(Item{Name: "test", Val: 1})
		zero := rb.Get(-1)
		assert.Equal(t, "", zero.Name)
		assert.Equal(t, 0, zero.Val)
	})
}

func TestRingBuf_Values(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	assert.Equal(t, []int{1, 2, 3}, rb.Values())
}

func TestRingBuf_ValuesEmpty(t *testing.T) {
	rb := NewRingBuf[int](5)
	assert.Empty(t, rb.Values())
}

func TestRingBuf_ValuesAfterWrap(t *testing.T) {
	rb := NewRingBuf[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	rb.Add(4)
	rb.Add(5)

	assert.Equal(t, []int{3, 4, 5}, rb.Values())
}

func TestRingBuf_ValuesReturnsNewSlice(t *testing.T) {
	rb := NewRingBuf[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	vals1 := rb.Values()
	vals2 := rb.Values()

	vals1[0] = 100

	assert.Equal(t, 100, vals1[0])
	assert.Equal(t, 1, vals2[0])
	assert.Equal(t, 1, rb.Get(0))
}

func TestRingBuf_Slice(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	rb.Add(4)
	rb.Add(5)

	assert.Equal(t, []int{1, 2}, rb.Slice(0, 2))
	assert.Equal(t, []int{3, 4, 5}, rb.Slice(2, 3))
	assert.Equal(t, []int{4, 5}, rb.Slice(3, 10))
}

func TestRingBuf_SliceAfterWrap(t *testing.T) {
	rb := NewRingBuf[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	rb.Add(4)
	rb.Add(5)
	rb.Add(6)

	assert.Equal(t, []int{4, 5}, rb.Slice(0, 2))
}

func TestRingBuf_SliceWrappedBoundary(t *testing.T) {
	rb := NewRingBuf[int](4)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	rb.Add(4)
	rb.Add(5)
	rb.Add(6)

	assert.Equal(t, []int{3, 4, 5, 6}, rb.Values())
	assert.Equal(t, []int{4, 5}, rb.Slice(1, 2))
	assert.Equal(t, []int{5, 6}, rb.Slice(2, 2))
	assert.Equal(t, []int{3, 4, 5, 6}, rb.Slice(0, 10))
}

func TestRingBuf_SliceInvalidParams(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)

	assert.Nil(t, rb.Slice(-1, 2))
	assert.Nil(t, rb.Slice(2, 0))
	assert.Nil(t, rb.Slice(2, -1))
	assert.Equal(t, []int{2}, rb.Slice(1, 10)) // caps count, doesn't return nil
}

func TestRingBuf_SliceAtBoundary(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	assert.Equal(t, []int{1, 2, 3}, rb.Slice(0, 3))
	assert.Equal(t, []int{3}, rb.Slice(2, 1))
	assert.Nil(t, rb.Slice(3, 1))
}

func TestRingBuf_SliceEmptyBuffer(t *testing.T) {
	rb := NewRingBuf[int](5)
	assert.Nil(t, rb.Slice(0, 1))
	assert.Nil(t, rb.Slice(0, 10))
}

func TestRingBuf_SliceReturnsNewSlice(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	slice := rb.Slice(0, 2)
	slice[0] = 100

	assert.Equal(t, 100, slice[0])
	assert.Equal(t, 1, rb.Get(0))
}

func TestRingBuf_Clear(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	rb.Clear()

	assert.Equal(t, 0, rb.Size())
	assert.Equal(t, 0, rb.Get(0))
	assert.Empty(t, rb.Values())

	rb.Add(10)
	assert.Equal(t, 1, rb.Size())
	assert.Equal(t, 10, rb.Get(0))
}

func TestRingBuf_ClearAfterMultipleWraps(t *testing.T) {
	rb := NewRingBuf[int](3)
	for i := 1; i <= 10; i++ {
		rb.Add(i)
	}

	rb.Clear()
	assert.Equal(t, 0, rb.Size())
	assert.Empty(t, rb.Values())

	rb.Add(100)
	assert.Equal(t, 1, rb.Size())
	assert.Equal(t, 100, rb.Get(0))
}

func TestRingBuf_ClearZerosMemory(t *testing.T) {
	type Item struct {
		Data []byte
	}

	rb := NewRingBuf[Item](3)
	rb.Add(Item{Data: []byte{1, 2, 3}})
	rb.Add(Item{Data: []byte{4, 5, 6}})
	rb.Add(Item{Data: []byte{7, 8, 9}})

	rb.Clear()

	for i := range 3 {
		item := rb.buf[i]
		assert.Nil(t, item.Data)
	}
}

func TestRingBuf_IteratorWithStep(t *testing.T) {
	tests := []struct {
		name     string
		from     int
		step     int
		expected []int
	}{
		{"step 1 from 0", 0, 1, []int{1, 2, 3, 4, 5}},
		{"step 2 from 0", 0, 2, []int{1, 3, 5}},
		{"step 3 from 0", 0, 3, []int{1, 4}},
		{"step 1 from 2", 2, 1, []int{3, 4, 5}},
		{"step 2 from 1", 1, 2, []int{2, 4}},
		{"reverse from 4", 4, -1, []int{5, 4, 3, 2, 1}},
		{"reverse step 2 from 4", 4, -2, []int{5, 3, 1}},
		{"reverse step 2 from 3", 3, -2, []int{4, 2}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rb := NewRingBuf[int](5)
			for i := 1; i <= 5; i++ {
				rb.Add(i)
			}

			var vals []int
			for v := range rb.Iterator(tt.from, tt.step) {
				vals = append(vals, v)
			}
			assert.Equal(t, tt.expected, vals)
		})
	}
}

func TestRingBuf_IteratorAfterWrap(t *testing.T) {
	rb := NewRingBuf[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	rb.Add(4)
	rb.Add(5)
	rb.Add(6)

	var vals []int
	for v := range rb.Iterator(0, 1) {
		vals = append(vals, v)
	}
	assert.Equal(t, []int{4, 5, 6}, vals)
}

func TestRingBuf_IteratorReverseAfterWrap(t *testing.T) {
	rb := NewRingBuf[int](3)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	rb.Add(4)
	rb.Add(5)

	var vals []int
	for v := range rb.Iterator(2, -1) {
		vals = append(vals, v)
	}
	assert.Equal(t, []int{5, 4, 3}, vals)
}

func TestRingBuf_IteratorDefaultStep(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)

	var vals []int
	for v := range rb.Iterator(0, 0) {
		vals = append(vals, v)
	}
	assert.Equal(t, []int{1, 2}, vals)
}

func TestRingBuf_IteratorEmpty(t *testing.T) {
	rb := NewRingBuf[int](5)

	var vals []int
	for v := range rb.Iterator(0, 1) {
		vals = append(vals, v)
	}
	assert.Empty(t, vals)
}

func TestRingBuf_IteratorStartBeyondSize(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)

	var vals []int
	for v := range rb.Iterator(5, 1) {
		vals = append(vals, v)
	}
	assert.Empty(t, vals)
}

func TestRingBuf_IteratorNegativeStart(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)

	var vals []int
	for v := range rb.Iterator(-1, 1) {
		vals = append(vals, v)
	}
	assert.Empty(t, vals)
}

func TestRingBuf_IteratorEarlyTermination(t *testing.T) {
	rb := NewRingBuf[int](5)
	rb.Add(1)
	rb.Add(2)
	rb.Add(3)
	rb.Add(4)
	rb.Add(5)

	var vals []int
	for v := range rb.Iterator(0, 1) {
		vals = append(vals, v)
		if v == 3 {
			break
		}
	}
	assert.Equal(t, []int{1, 2, 3}, vals)
}

func TestRingBuf_Struct(t *testing.T) {
	type Item struct {
		Name string
		Val  int
	}

	rb := NewRingBuf[Item](3)
	rb.Add(Item{Name: "a", Val: 1})
	rb.Add(Item{Name: "b", Val: 2})
	rb.Add(Item{Name: "c", Val: 3})
	rb.Add(Item{Name: "d", Val: 4})

	assert.Equal(t, "b", rb.Get(0).Name)
	assert.Equal(t, 3, rb.Get(1).Val)
}

func TestRingBuf_Pointer(t *testing.T) {
	v1 := 10
	v2 := 20
	v3 := 30

	rb := NewRingBuf[*int](3)
	rb.Add(&v1)
	rb.Add(&v2)
	rb.Add(&v3)
	rb.Add(&v2)

	assert.Equal(t, 20, *rb.Get(0))
}

func TestRingBuf_String(t *testing.T) {
	rb := NewRingBuf[string](3)
	rb.Add("hello")
	rb.Add("world")
	rb.Add("test")
	rb.Add("foo")

	assert.Equal(t, []string{"world", "test", "foo"}, rb.Values())
}
