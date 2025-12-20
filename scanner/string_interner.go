package scanner

import "sync"

// StringInterner provides thread-safe string interning to reduce memory allocations
// by ensuring only one copy of each unique string is kept in memory
type StringInterner struct {
	mu      sync.RWMutex
	strings map[string]string
}

// NewStringInterner creates a new string interner
func NewStringInterner() *StringInterner {
	return &StringInterner{
		strings: make(map[string]string, 1000), // Pre-allocate for common case
	}
}

// Intern returns a canonical representation of the string.
// If the string already exists in the pool, returns the existing copy.
// Otherwise, adds it to the pool and returns it.
func (si *StringInterner) Intern(s string) string {
	// Fast path: read lock for existing strings
	si.mu.RLock()
	if interned, exists := si.strings[s]; exists {
		si.mu.RUnlock()
		return interned
	}
	si.mu.RUnlock()

	// Slow path: write lock to add new string
	si.mu.Lock()
	defer si.mu.Unlock()

	// Double-check after acquiring write lock (another goroutine might have added it)
	if interned, exists := si.strings[s]; exists {
		return interned
	}

	// Add to pool
	si.strings[s] = s
	return s
}

// Len returns the number of unique strings in the pool
func (si *StringInterner) Len() int {
	si.mu.RLock()
	defer si.mu.RUnlock()
	return len(si.strings)
}

// Clear removes all strings from the pool
func (si *StringInterner) Clear() {
	si.mu.Lock()
	defer si.mu.Unlock()
	si.strings = make(map[string]string, 1000)
}
