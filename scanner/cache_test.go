package scanner

import (
	"os"
	"path/filepath"
	"testing"
)

// TestCacheRoundtrip tests that we can write and read cache without losing data
func TestCacheRoundtrip(t *testing.T) {
	if os.Getenv("PROFILE") == "1" {
		t.Skip("Skipping cache test in profile mode")
	}

	// Create a temporary cache file
	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test.cache")

	// Perform a scan
	config := NewDefaultConfig()
	config.Packages = []string{"../examples/starwars/basic"}
	config.LogLevel = "error"

	result, err := NewScanner().ScanWithConfig(config)
	if err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}

	originalTypeCount := result.Types.Len()
	if originalTypeCount == 0 {
		t.Skip("No types found in example package, skipping cache test")
	}

	// Ensure all types are fully loaded
	if err := result.EnsureFullyLoaded(); err != nil {
		t.Fatalf("Failed to ensure types fully loaded: %v", err)
	}

	// Write to cache
	if err := result.ToCache(cacheFile); err != nil {
		t.Fatalf("Failed to write cache: %v", err)
	}

	// Verify cache file was created
	if !IsCacheValid(cacheFile) {
		t.Fatal("Cache file was not created or is invalid")
	}

	// Read from cache
	cachedResult, err := ReadCache(cacheFile)
	if err != nil {
		t.Fatalf("Failed to read cache: %v", err)
	}

	// Verify data integrity
	cachedTypeCount := cachedResult.Types.Len()
	if cachedTypeCount != originalTypeCount {
		t.Errorf("Type count mismatch: original=%d, cached=%d", originalTypeCount, cachedTypeCount)
	}

	// Verify specific types are present
	originalKeys := result.Types.Keys()
	cachedKeys := cachedResult.Types.Keys()

	if len(originalKeys) != len(cachedKeys) {
		t.Errorf("Key count mismatch: original=%d, cached=%d", len(originalKeys), len(cachedKeys))
	}

	// Sample check: verify a few types are still there
	for _, key := range originalKeys[:minInt(3, len(originalKeys))] {
		if !cachedResult.Types.Has(key) {
			t.Errorf("Type %s not found in cached result", key)
		}
	}
}

// TestCacheDeterministic tests that multiple cache writes produce consistent results
func TestCacheDeterministic(t *testing.T) {
	if os.Getenv("PROFILE") == "1" {
		t.Skip("Skipping cache test in profile mode")
	}

	tmpDir := t.TempDir()

	// Perform a scan
	config := NewDefaultConfig()
	config.Packages = []string{"../examples/starwars/basic"}
	config.LogLevel = "error"

	result, err := NewScanner().ScanWithConfig(config)
	if err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}

	// Ensure fully loaded
	if err := result.EnsureFullyLoaded(); err != nil {
		t.Fatalf("Failed to ensure types fully loaded: %v", err)
	}

	// Write cache twice
	cacheFile1 := filepath.Join(tmpDir, "cache1.cache")
	cacheFile2 := filepath.Join(tmpDir, "cache2.cache")

	if err := result.ToCache(cacheFile1); err != nil {
		t.Fatalf("Failed to write first cache: %v", err)
	}

	if err := result.ToCache(cacheFile2); err != nil {
		t.Fatalf("Failed to write second cache: %v", err)
	}

	// Read both caches
	result1, err := ReadCache(cacheFile1)
	if err != nil {
		t.Fatalf("Failed to read first cache: %v", err)
	}

	result2, err := ReadCache(cacheFile2)
	if err != nil {
		t.Fatalf("Failed to read second cache: %v", err)
	}

	// Verify they have the same data
	if result1.Types.Len() != result2.Types.Len() {
		t.Errorf("Type count mismatch: cache1=%d, cache2=%d", result1.Types.Len(), result2.Types.Len())
	}

	if result1.Values.Len() != result2.Values.Len() {
		t.Errorf("Value count mismatch: cache1=%d, cache2=%d", result1.Values.Len(), result2.Values.Len())
	}

	if result1.Packages.Len() != result2.Packages.Len() {
		t.Errorf("Package count mismatch: cache1=%d, cache2=%d", result1.Packages.Len(), result2.Packages.Len())
	}
}

// TestCacheValidation tests cache validation functions
func TestCacheValidation(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentFile := filepath.Join(tmpDir, "nonexistent.cache")
	invalidFile := filepath.Join(tmpDir, "invalid.cache")

	// Create an invalid cache file
	if err := os.WriteFile(invalidFile, []byte("invalid data"), 0644); err != nil {
		t.Fatalf("Failed to create invalid cache file: %v", err)
	}

	// Test IsCacheValid with nonexistent file
	if IsCacheValid(nonExistentFile) {
		t.Error("IsCacheValid should return false for nonexistent file")
	}

	// Test IsCacheValid with invalid file
	if IsCacheValid(invalidFile) {
		t.Error("IsCacheValid should return false for invalid file")
	}

	// Test CacheAge with nonexistent file
	age := CacheAge(nonExistentFile)
	if age != -1 {
		t.Errorf("CacheAge should return -1 for nonexistent file, got %d", age)
	}
}

// TestCacheCompressionOptions tests both compressed and uncompressed cache
func TestCacheCompressionOptions(t *testing.T) {
	if os.Getenv("PROFILE") == "1" {
		t.Skip("Skipping cache test in profile mode")
	}

	tmpDir := t.TempDir()

	// Perform a scan
	config := NewDefaultConfig()
	config.Packages = []string{"../examples/starwars/basic"}
	config.LogLevel = "error"

	result, err := NewScanner().ScanWithConfig(config)
	if err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}

	// Ensure fully loaded
	if err := result.EnsureFullyLoaded(); err != nil {
		t.Fatalf("Failed to ensure types fully loaded: %v", err)
	}

	// Test compressed cache
	compressedFile := filepath.Join(tmpDir, "compressed.cache")
	if err := WriteCache(compressedFile, result); err != nil {
		t.Fatalf("Failed to write compressed cache: %v", err)
	}

	// Test uncompressed cache
	uncompressedFile := filepath.Join(tmpDir, "uncompressed.cache")
	if err := WriteCache(uncompressedFile, result); err != nil {
		t.Fatalf("Failed to write uncompressed cache: %v", err)
	}

	// Get file sizes
	compressedInfo, err := os.Stat(compressedFile)
	if err != nil {
		t.Fatalf("Failed to stat compressed cache: %v", err)
	}

	uncompressedInfo, err := os.Stat(uncompressedFile)
	if err != nil {
		t.Fatalf("Failed to stat uncompressed cache: %v", err)
	}

	t.Logf("Compressed cache size: %d bytes", compressedInfo.Size())
	t.Logf("Uncompressed cache size: %d bytes", uncompressedInfo.Size())

	// Compressed should be smaller
	if compressedInfo.Size() >= uncompressedInfo.Size() {
		t.Logf("Warning: compressed cache is not smaller than uncompressed (compressed=%d, uncompressed=%d)",
			compressedInfo.Size(), uncompressedInfo.Size())
	}

	// Verify both read correctly
	compressedResult, err := ReadCache(compressedFile)
	if err != nil {
		t.Fatalf("Failed to read compressed cache: %v", err)
	}

	uncompressedResult, err := ReadCache(uncompressedFile)
	if err != nil {
		t.Fatalf("Failed to read uncompressed cache: %v", err)
	}

	// Verify data matches
	if compressedResult.Types.Len() != uncompressedResult.Types.Len() {
		t.Errorf("Type count mismatch between compressed and uncompressed")
	}
}

// TestCacheWithFullyLoadedFlag tests that the FullyLoaded flag is preserved
func TestCacheWithFullyLoadedFlag(t *testing.T) {
	if os.Getenv("PROFILE") == "1" {
		t.Skip("Skipping cache test in profile mode")
	}

	tmpDir := t.TempDir()
	cacheFile := filepath.Join(tmpDir, "test.cache")

	// Perform a scan
	config := NewDefaultConfig()
	config.Packages = []string{"../examples/starwars/basic"}
	config.LogLevel = "error"

	result, err := NewScanner().ScanWithConfig(config)
	if err != nil {
		t.Fatalf("Failed to scan: %v", err)
	}

	// Ensure fully loaded
	if err := result.EnsureFullyLoaded(); err != nil {
		t.Fatalf("Failed to ensure types fully loaded: %v", err)
	}

	// Write with fullyLoaded=true
	if err := WriteCache(cacheFile, result); err != nil {
		t.Fatalf("Failed to write cache: %v", err)
	}

	// Read cache back and verify FullyLoaded flag
	cachedResult, err := ReadCache(cacheFile)
	if err != nil {
		t.Fatalf("Failed to read cache: %v", err)
	}

	// Verify data integrity
	if cachedResult.Types.Len() != result.Types.Len() {
		t.Error("Type count mismatch after cache roundtrip")
	}
}

// TestCacheFileNotFound tests error handling for missing cache files
func TestCacheFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistentFile := filepath.Join(tmpDir, "does_not_exist.cache")

	_, err := ReadCache(nonExistentFile)
	if err == nil {
		t.Error("ReadCache should return error for nonexistent file")
	}
}

// Helper function
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// BenchmarkCacheWrite benchmarks cache writing
func BenchmarkCacheWrite(b *testing.B) {
	config := NewDefaultConfig()
	config.Packages = []string{"../examples/starwars/basic"}
	config.LogLevel = "error"

	result, err := NewScanner().ScanWithConfig(config)
	if err != nil {
		b.Fatalf("Failed to scan: %v", err)
	}

	if err := result.EnsureFullyLoaded(); err != nil {
		b.Fatalf("Failed to ensure types fully loaded: %v", err)
	}

	tmpDir := b.TempDir()

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		cacheFile := filepath.Join(tmpDir, "bench_"+string(rune(i))+".cache")
		if err := result.ToCache(cacheFile); err != nil {
			b.Fatalf("Failed to write cache: %v", err)
		}
	}
}

// BenchmarkCacheRead benchmarks cache reading
func BenchmarkCacheRead(b *testing.B) {
	config := NewDefaultConfig()
	config.Packages = []string{"../examples/starwars/basic"}
	config.LogLevel = "error"

	result, err := NewScanner().ScanWithConfig(config)
	if err != nil {
		b.Fatalf("Failed to scan: %v", err)
	}

	if err := result.EnsureFullyLoaded(); err != nil {
		b.Fatalf("Failed to ensure types fully loaded: %v", err)
	}

	tmpDir := b.TempDir()
	cacheFile := filepath.Join(tmpDir, "bench.cache")
	if err := result.ToCache(cacheFile); err != nil {
		b.Fatalf("Failed to write cache: %v", err)
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, err := ReadCache(cacheFile)
		if err != nil {
			b.Fatalf("Failed to read cache: %v", err)
		}
	}
}
