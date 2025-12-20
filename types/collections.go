package types

import "sync"

// SyncMap is a generic goroutine-safe map with read-write mutex protection
type SyncMap[K comparable, V any] struct {
	mu     sync.RWMutex
	values map[K]V
}

// Get retrieves a value by key. Returns the value and a boolean indicating if the key exists.
func (m *SyncMap[K, V]) Get(key K) (V, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	val, exists := m.values[key]
	return val, exists
}

// Set stores a value with the given key.
func (m *SyncMap[K, V]) Set(key K, val V) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values[key] = val
}

// Has checks if a key exists in the map.
func (m *SyncMap[K, V]) Has(key K) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.values[key]
	return exists
}

// Delete removes a key from the map.
func (m *SyncMap[K, V]) Delete(key K) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.values, key)
}

// Keys returns a slice of all keys in the map.
func (m *SyncMap[K, V]) Keys() []K {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]K, 0, len(m.values))
	for k := range m.values {
		keys = append(keys, k)
	}
	return keys
}

// Values returns a slice of all values in the map.
func (m *SyncMap[K, V]) Values() []V {
	m.mu.RLock()
	defer m.mu.RUnlock()
	values := make([]V, 0, len(m.values))
	for _, v := range m.values {
		values = append(values, v)
	}
	return values
}

// Len returns the number of items in the map.
func (m *SyncMap[K, V]) Len() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.values)
}

// Clear removes all items from the map.
func (m *SyncMap[K, V]) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.values = make(map[K]V)
}

// Range calls the given function for each key-value pair in the map.
// If the function returns false, iteration stops.
func (m *SyncMap[K, V]) Range(fn func(key K, value V) bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for k, v := range m.values {
		if !fn(k, v) {
			break
		}
	}
}

// GetOrSet returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The bool return value indicates whether the value was loaded (true) or stored (false).
func (m *SyncMap[K, V]) GetOrSet(key K, val V) (V, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, exists := m.values[key]; exists {
		return existing, true
	}
	m.values[key] = val
	return val, false
}

// CompareAndSwap swaps the old value for the new value if the current value equals old.
// Returns true if the swap was performed.
func (m *SyncMap[K, V]) CompareAndSwap(key K, old, new V, compareFn func(V, V) bool) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if current, exists := m.values[key]; exists && compareFn(current, old) {
		m.values[key] = new
		return true
	}
	return false
}

// NewSyncMap creates a new goroutine-safe map.
func NewSyncMap[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		values: make(map[K]V),
	}
}

// SyncSlice is a generic goroutine-safe slice with read-write mutex protection
type SyncSlice[T any] struct {
	mu     sync.RWMutex
	values []T
}

// Get retrieves a value by index. Panics if index is out of range.
func (s *SyncSlice[T]) Get(index int) T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.values[index]
}

// Set updates a value at the given index. Panics if index is out of range.
func (s *SyncSlice[T]) Set(index int, val T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values[index] = val
}

// Append adds one or more values to the end of the slice.
func (s *SyncSlice[T]) Append(vals ...T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = append(s.values, vals...)
}

// Len returns the number of items in the slice.
func (s *SyncSlice[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.values)
}

// Clear removes all items from the slice.
func (s *SyncSlice[T]) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.values = s.values[:0]
}

// Range calls the given function for each index-value pair in the slice.
// If the function returns false, iteration stops.
func (s *SyncSlice[T]) Range(fn func(index int, value T) bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	for i, v := range s.values {
		if !fn(i, v) {
			break
		}
	}
}

// Slice returns a copy of the underlying slice.
func (s *SyncSlice[T]) Slice() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]T, len(s.values))
	copy(result, s.values)
	return result
}

// Filter returns a new slice containing only values for which the predicate returns true.
func (s *SyncSlice[T]) Filter(predicate func(T) bool) []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]T, 0)
	for _, v := range s.values {
		if predicate(v) {
			result = append(result, v)
		}
	}
	return result
}

// NewSyncSlice creates a new goroutine-safe slice.
func NewSyncSlice[T any]() *SyncSlice[T] {
	return &SyncSlice[T]{
		values: make([]T, 0),
	}
}

// SyncCounter is a goroutine-safe counter
type SyncCounter struct {
	mu      sync.Mutex
	counter map[string]int
}

// Increment increases the counter for the given key and returns the new value.
func (c *SyncCounter) Increment(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter[key]++
	return c.counter[key]
}

// Get returns the current value for the given key.
func (c *SyncCounter) Get(key string) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.counter[key]
}

// Reset sets the counter for the given key to zero.
func (c *SyncCounter) Reset(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter[key] = 0
}

// Clear removes all counters.
func (c *SyncCounter) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.counter = make(map[string]int)
}

// NewSyncCounter creates a new goroutine-safe counter.
func NewSyncCounter() *SyncCounter {
	return &SyncCounter{
		counter: make(map[string]int),
	}
}
