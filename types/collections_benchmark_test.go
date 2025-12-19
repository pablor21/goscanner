package types

import (
	"fmt"
	"testing"
)

// Benchmark SyncMap concurrent reads
func BenchmarkSyncMap_ConcurrentReads(b *testing.B) {
	m := NewSyncMap[string, int]()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		m.Set(fmt.Sprintf("key%d", i), i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i%1000)
			m.Get(key)
			i++
		}
	})
}

// Benchmark SyncMap concurrent writes
func BenchmarkSyncMap_ConcurrentWrites(b *testing.B) {
	m := NewSyncMap[string, int]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			m.Set(fmt.Sprintf("key%d", i), i)
			i++
		}
	})
}

// Benchmark SyncMap concurrent mixed operations
func BenchmarkSyncMap_ConcurrentMixed(b *testing.B) {
	m := NewSyncMap[string, int]()

	// Pre-populate
	for i := 0; i < 500; i++ {
		m.Set(fmt.Sprintf("key%d", i), i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 < 8 { // 80% reads
				key := fmt.Sprintf("key%d", i%500)
				m.Get(key)
			} else { // 20% writes
				m.Set(fmt.Sprintf("key%d", i), i)
			}
			i++
		}
	})
}

// Benchmark SyncMap GetOrSet
func BenchmarkSyncMap_GetOrSet(b *testing.B) {
	m := NewSyncMap[string, int]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("key%d", i%100)
			m.GetOrSet(key, i)
			i++
		}
	})
}

// Benchmark SyncMap Range
func BenchmarkSyncMap_Range(b *testing.B) {
	m := NewSyncMap[string, int]()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		m.Set(fmt.Sprintf("key%d", i), i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		m.Range(func(key string, value int) bool {
			return true
		})
	}
}

// Benchmark SyncSlice concurrent appends
func BenchmarkSyncSlice_ConcurrentAppends(b *testing.B) {
	s := NewSyncSlice[int]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s.Append(i)
			i++
		}
	})
}

// Benchmark SyncSlice concurrent reads
func BenchmarkSyncSlice_ConcurrentReads(b *testing.B) {
	s := NewSyncSlice[int]()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		s.Append(i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if s.Len() > 0 {
				s.Get(i % s.Len())
			}
			i++
		}
	})
}

// Benchmark SyncSlice Range
func BenchmarkSyncSlice_Range(b *testing.B) {
	s := NewSyncSlice[int]()

	// Pre-populate
	for i := 0; i < 1000; i++ {
		s.Append(i)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Range(func(index int, value int) bool {
			return true
		})
	}
}

// Benchmark SyncCounter concurrent increments
func BenchmarkSyncCounter_ConcurrentIncrements(b *testing.B) {
	c := NewSyncCounter()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("counter%d", i%10)
			c.Increment(key)
			i++
		}
	})
}

// Benchmark SyncCounter Get
func BenchmarkSyncCounter_Get(b *testing.B) {
	c := NewSyncCounter()

	// Pre-populate
	for i := 0; i < 100; i++ {
		c.Increment(fmt.Sprintf("counter%d", i))
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := fmt.Sprintf("counter%d", i%100)
			c.Get(key)
			i++
		}
	})
}

// Benchmark comparison: TypesCol vs raw SyncMap
func BenchmarkTypesCol_vs_SyncMap(b *testing.B) {
	b.Run("TypesCol", func(b *testing.B) {
		col := NewTypesCol[*Struct]()

		// Pre-populate
		for i := 0; i < 1000; i++ {
			s := NewStruct(fmt.Sprintf("type%d", i), fmt.Sprintf("Type%d", i))
			col.Set(s.Id(), s)
		}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := fmt.Sprintf("type%d", i%1000)
				col.Get(key)
				i++
			}
		})
	})

	b.Run("SyncMap", func(b *testing.B) {
		m := NewSyncMap[string, *Struct]()

		// Pre-populate
		for i := 0; i < 1000; i++ {
			s := NewStruct(fmt.Sprintf("type%d", i), fmt.Sprintf("Type%d", i))
			m.Set(s.Id(), s)
		}

		b.ResetTimer()
		b.RunParallel(func(pb *testing.PB) {
			i := 0
			for pb.Next() {
				key := fmt.Sprintf("type%d", i%1000)
				m.Get(key)
				i++
			}
		})
	})
}
