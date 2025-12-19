package scanner

import (
	"context"
	"os"
	"runtime"
	"runtime/pprof"
	"testing"
)

// TestProfile_CPUParallelScan runs a CPU profile of parallel scanning
func TestProfile_CPUParallelScan(t *testing.T) {
	if os.Getenv("PROFILE") != "1" {
		t.Skip("Skipping profile test. Set PROFILE=1 to enable")
	}

	f, err := os.Create("cpu_parallel.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	// Run scan multiple times
	config := NewDefaultConfig()
	config.MaxConcurrency = runtime.NumCPU()
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error"

	for i := 0; i < 10; i++ {
		s := NewScanner()
		_, err := s.ScanWithConfig(config)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestProfile_CPUSequentialScan runs a CPU profile of sequential scanning
func TestProfile_CPUSequentialScan(t *testing.T) {
	if os.Getenv("PROFILE") != "1" {
		t.Skip("Skipping profile test. Set PROFILE=1 to enable")
	}

	f, err := os.Create("cpu_sequential.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	// Run scan multiple times
	config := NewDefaultConfig()
	config.MaxConcurrency = 1
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error"

	for i := 0; i < 10; i++ {
		s := NewScanner()
		_, err := s.ScanWithConfig(config)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// TestProfile_MemoryParallelScan runs a memory profile of parallel scanning
func TestProfile_MemoryParallelScan(t *testing.T) {
	if os.Getenv("PROFILE") != "1" {
		t.Skip("Skipping profile test. Set PROFILE=1 to enable")
	}

	config := NewDefaultConfig()
	config.MaxConcurrency = runtime.NumCPU()
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error"

	// Run scan multiple times
	for i := 0; i < 10; i++ {
		s := NewScanner()
		_, err := s.ScanWithConfig(config)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Force GC and capture memory profile
	runtime.GC()

	f, err := os.Create("mem_parallel.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.WriteHeapProfile(f); err != nil {
		t.Fatal(err)
	}
}

// TestProfile_MemorySequentialScan runs a memory profile of sequential scanning
func TestProfile_MemorySequentialScan(t *testing.T) {
	if os.Getenv("PROFILE") != "1" {
		t.Skip("Skipping profile test. Set PROFILE=1 to enable")
	}

	config := NewDefaultConfig()
	config.MaxConcurrency = 1
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error"

	// Run scan multiple times
	for i := 0; i < 10; i++ {
		s := NewScanner()
		_, err := s.ScanWithConfig(config)
		if err != nil {
			t.Fatal(err)
		}
	}

	// Force GC and capture memory profile
	runtime.GC()

	f, err := os.Create("mem_sequential.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.WriteHeapProfile(f); err != nil {
		t.Fatal(err)
	}
}

// TestProfile_Goroutines profiles goroutine usage
func TestProfile_Goroutines(t *testing.T) {
	if os.Getenv("PROFILE") != "1" {
		t.Skip("Skipping profile test. Set PROFILE=1 to enable")
	}

	config := NewDefaultConfig()
	config.MaxConcurrency = runtime.NumCPU()
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error"

	s := NewScanner()

	// Start scan in background
	done := make(chan error)
	go func() {
		_, err := s.ScanWithConfig(config)
		done <- err
	}()

	// Capture goroutine profile while running
	f, err := os.Create("goroutine.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.Lookup("goroutine").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}

	// Wait for completion
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

// TestProfile_TypeResolutionHotPath identifies hot paths in type resolution
func TestProfile_TypeResolutionHotPath(t *testing.T) {
	if os.Getenv("PROFILE") != "1" {
		t.Skip("Skipping profile test. Set PROFILE=1 to enable")
	}

	f, err := os.Create("cpu_typeresolution.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.StartCPUProfile(f); err != nil {
		t.Fatal(err)
	}
	defer pprof.StopCPUProfile()

	config := NewDefaultConfig()
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error"
	ctx := NewScanningContext(context.Background(), config)

	scanner := NewGlobScanner()
	pkgs, err := scanner.ScanPackages(ScanModeFull, config.Packages...)
	if err != nil {
		t.Fatal(err)
	}

	resolver := NewDefaultTypeResolver(config, ctx.Logger)

	// Profile just the type resolution phase
	for i := 0; i < 100; i++ {
		for _, pkg := range pkgs {
			workerCtx := ctx.WithPackage(nil)
			if err := resolver.ProcessPackage(workerCtx, pkg); err != nil {
				t.Fatal(err)
			}
		}
	}
}

// TestProfile_Mutex profiles lock contention
func TestProfile_Mutex(t *testing.T) {
	if os.Getenv("PROFILE") != "1" {
		t.Skip("Skipping profile test. Set PROFILE=1 to enable")
	}

	// Enable mutex profiling
	runtime.SetMutexProfileFraction(1)
	defer runtime.SetMutexProfileFraction(0)

	config := NewDefaultConfig()
	config.MaxConcurrency = runtime.NumCPU()
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error"

	// Run scans to accumulate mutex contention data
	for i := 0; i < 5; i++ {
		s := NewScanner()
		_, err := s.ScanWithConfig(config)
		if err != nil {
			t.Fatal(err)
		}
	}

	f, err := os.Create("mutex.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.Lookup("mutex").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}
}

// TestProfile_Block profiles blocking operations
func TestProfile_Block(t *testing.T) {
	if os.Getenv("PROFILE") != "1" {
		t.Skip("Skipping profile test. Set PROFILE=1 to enable")
	}

	// Enable block profiling
	runtime.SetBlockProfileRate(1)
	defer runtime.SetBlockProfileRate(0)

	config := NewDefaultConfig()
	config.MaxConcurrency = runtime.NumCPU()
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error"

	// Run scans to accumulate blocking data
	for i := 0; i < 5; i++ {
		s := NewScanner()
		_, err := s.ScanWithConfig(config)
		if err != nil {
			t.Fatal(err)
		}
	}

	f, err := os.Create("block.prof")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	if err := pprof.Lookup("block").WriteTo(f, 0); err != nil {
		t.Fatal(err)
	}
}
