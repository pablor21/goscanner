package types

import (
	"fmt"
	"testing"
)

// Benchmark concurrent reads on TypesCol
func BenchmarkTypesCol_ConcurrentReads(b *testing.B) {
	col := NewTypesCol[*Struct]()

	// Pre-populate with test data
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
}

// Benchmark concurrent writes on TypesCol
func BenchmarkTypesCol_ConcurrentWrites(b *testing.B) {
	col := NewTypesCol[*Struct]()

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			s := NewStruct(fmt.Sprintf("type%d", i), fmt.Sprintf("Type%d", i))
			col.Set(s.Id(), s)
			i++
		}
	})
}

// Benchmark mixed read/write operations
func BenchmarkTypesCol_ConcurrentMixed(b *testing.B) {
	col := NewTypesCol[*Struct]()

	// Pre-populate with test data
	for i := 0; i < 500; i++ {
		s := NewStruct(fmt.Sprintf("type%d", i), fmt.Sprintf("Type%d", i))
		col.Set(s.Id(), s)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%10 < 8 { // 80% reads
				key := fmt.Sprintf("type%d", i%500)
				col.Get(key)
			} else { // 20% writes
				s := NewStruct(fmt.Sprintf("type%d", i), fmt.Sprintf("Type%d", i))
				col.Set(s.Id(), s)
			}
			i++
		}
	})
}

// Benchmark sequential operations (baseline)
func BenchmarkTypesCol_Sequential(b *testing.B) {
	col := NewTypesCol[*Struct]()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s := NewStruct(fmt.Sprintf("type%d", i), fmt.Sprintf("Type%d", i))
		col.Set(s.Id(), s)
		col.Get(s.Id())
	}
}

// Benchmark with realistic workload simulation
func BenchmarkTypesCol_RealisticWorkload(b *testing.B) {
	col := NewTypesCol[*Struct]()

	// Simulate package scanning: burst writes, then many reads
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// Write phase (simulating type discovery)
		for j := 0; j < 100; j++ {
			s := NewStruct(fmt.Sprintf("type%d_%d", i, j), fmt.Sprintf("Type%d_%d", i, j))
			col.Set(s.Id(), s)
		}

		// Read phase (simulating type resolution and loading)
		for j := 0; j < 100; j++ {
			key := fmt.Sprintf("type%d_%d", i, j)
			col.Get(key)
		}
	}
}

// Benchmark concurrent type loading
func BenchmarkType_ConcurrentLoad(b *testing.B) {
	// Create a struct with a simple loader
	s := NewStruct("test.MyStruct", "MyStruct")

	s.SetLoader(func(t Type) error {
		// Simulate some work
		for i := 0; i < 100; i++ {
			_ = i * i
		}
		return nil
	})

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			s.Load()
		}
	})
}
