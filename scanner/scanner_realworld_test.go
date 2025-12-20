package scanner

import (
	"fmt"
	"runtime"
	"testing"
	"time"
)

// TestRealWorld_SmallProject tests performance on small project (6 packages)
func TestRealWorld_SmallProject(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world test in short mode")
	}

	runRealWorldTest(t, "Small Project (starwars)", []string{"../examples/starwars/..."})
}

// TestRealWorld_StdlibHTTP tests performance on stdlib http package
func TestRealWorld_StdlibHTTP(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world test in short mode")
	}

	runRealWorldTest(t, "Stdlib net/http", []string{"net/http"})
}

// TestRealWorld_StdlibEncoding tests performance on stdlib encoding packages
func TestRealWorld_StdlibEncoding(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world test in short mode")
	}

	runRealWorldTest(t, "Stdlib encoding/*", []string{"encoding/..."})
}

// TestRealWorld_GolangToolsPackages tests on a larger real codebase
func TestRealWorld_GolangToolsPackages(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping real-world test in short mode")
	}

	runRealWorldTest(t, "golang.org/x/tools/go/packages", []string{"golang.org/x/tools/go/packages/..."})
}

func runRealWorldTest(t *testing.T, name string, packages []string) {
	concurrencyLevels := []int{1, 2, 4, 8, runtime.NumCPU()}
	results := make(map[int]Result)

	fmt.Printf("\n=== Testing: %s ===\n", name)

	for _, concurrency := range concurrencyLevels {
		config := NewDefaultConfig()
		config.MaxConcurrency = concurrency
		config.Packages = packages
		config.LogLevel = "error"

		var totalDuration time.Duration
		var typeCount, pkgCount int
		runs := 3

		for i := 0; i < runs; i++ {
			start := time.Now()
			s := NewScanner()
			result, err := s.ScanWithConfig(config)
			duration := time.Since(start)

			if err != nil {
				t.Fatalf("Scan failed with concurrency=%d: %v", concurrency, err)
			}

			totalDuration += duration
			typeCount = result.Types.Len()
			pkgCount = result.Packages.Len()
		}

		avgDuration := totalDuration / time.Duration(runs)
		results[concurrency] = Result{
			Concurrency: concurrency,
			Duration:    avgDuration,
			TypeCount:   typeCount,
			PkgCount:    pkgCount,
		}
	}

	// Print results
	baseline := results[1].Duration
	fmt.Printf("\n%-15s %-12s %-12s %-10s %-10s\n", "Concurrency", "Duration", "Types", "Packages", "Speedup")
	fmt.Printf("%s\n", "-------------------------------------------------------------------")

	for _, concurrency := range concurrencyLevels {
		r := results[concurrency]
		speedup := float64(baseline) / float64(r.Duration)
		fmt.Printf("%-15d %-12s %-12d %-10d %.2fx\n",
			r.Concurrency,
			r.Duration.Round(time.Millisecond),
			r.TypeCount,
			r.PkgCount,
			speedup,
		)
	}

	// Verify correctness: all runs should find same number of types
	for _, r := range results {
		if r.TypeCount != results[1].TypeCount {
			t.Errorf("Type count mismatch: concurrency=%d found %d types, expected %d",
				r.Concurrency, r.TypeCount, results[1].TypeCount)
		}
	}
}

type Result struct {
	Concurrency int
	Duration    time.Duration
	TypeCount   int
	PkgCount    int
}

// BenchmarkRealWorld_Scaling measures speedup with different concurrency levels
func BenchmarkRealWorld_Scaling(b *testing.B) {
	packages := []string{"../examples/starwars/..."}
	concurrencyLevels := []int{1, 2, 4, 8}

	for _, concurrency := range concurrencyLevels {
		b.Run(fmt.Sprintf("Concurrency_%d", concurrency), func(b *testing.B) {
			config := NewDefaultConfig()
			config.MaxConcurrency = concurrency
			config.Packages = packages
			config.LogLevel = "error"

			b.ResetTimer()
			b.ReportAllocs()

			for i := 0; i < b.N; i++ {
				s := NewScanner()
				_, err := s.ScanWithConfig(config)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
