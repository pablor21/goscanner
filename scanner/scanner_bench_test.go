package scanner

import (
	"context"
	"runtime"
	"testing"

	"golang.org/x/tools/go/packages"
)

// BenchmarkPackageProcessing_Sequential measures baseline sequential performance
func BenchmarkPackageProcessing_Sequential(b *testing.B) {
	benchmarkPackageProcessing(b, 1)
}

// BenchmarkPackageProcessing_Parallel2 measures performance with 2 workers
func BenchmarkPackageProcessing_Parallel2(b *testing.B) {
	benchmarkPackageProcessing(b, 2)
}

// BenchmarkPackageProcessing_Parallel4 measures performance with 4 workers
func BenchmarkPackageProcessing_Parallel4(b *testing.B) {
	benchmarkPackageProcessing(b, 4)
}

// BenchmarkPackageProcessing_Parallel8 measures performance with 8 workers
func BenchmarkPackageProcessing_Parallel8(b *testing.B) {
	benchmarkPackageProcessing(b, 8)
}

// BenchmarkPackageProcessing_ParallelCPU measures performance with CPU count workers
func BenchmarkPackageProcessing_ParallelCPU(b *testing.B) {
	benchmarkPackageProcessing(b, runtime.NumCPU())
}

func benchmarkPackageProcessing(b *testing.B, maxConcurrency int) {
	// Load test packages once
	scanner := NewGlobScanner()
	pkgs, err := scanner.ScanPackages(ScanModeFull, "../examples/starwars/...")
	if err != nil {
		b.Fatalf("Failed to scan packages: %v", err)
	}

	if len(pkgs) == 0 {
		b.Skip("No packages found to benchmark")
	}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		config := NewDefaultConfig()
		config.MaxConcurrency = maxConcurrency
		ctx := NewScanningContext(context.Background(), config)

		s := NewScanner()
		s.TypeResolver = NewDefaultTypeResolver(config, ctx.Logger)

		// Register all packages
		visited := make(map[string]bool)
		var registerDeps func(*packages.Package)
		registerDeps = func(pkg *packages.Package) {
			if pkg == nil || visited[pkg.PkgPath] {
				return
			}
			visited[pkg.PkgPath] = true
			s.TypeResolver.(*defaultTypeResolver).pkgs.Set(pkg.PkgPath, pkg)
			for _, dep := range pkg.Imports {
				registerDeps(dep)
			}
		}

		for _, pkg := range pkgs {
			registerDeps(pkg)
		}

		// Process packages (this is what we're benchmarking)
		for _, pkg := range pkgs {
			workerCtx := ctx.WithPackage(nil)
			if err := s.TypeResolver.ProcessPackage(workerCtx, pkg); err != nil {
				b.Fatalf("Failed to process package: %v", err)
			}
		}
	}
}

// BenchmarkTypeLoading_Sequential measures baseline sequential type loading
func BenchmarkTypeLoading_Sequential(b *testing.B) {
	benchmarkTypeLoading(b, 1)
}

// BenchmarkTypeLoading_Parallel2 measures type loading with 2 workers
func BenchmarkTypeLoading_Parallel2(b *testing.B) {
	benchmarkTypeLoading(b, 2)
}

// BenchmarkTypeLoading_Parallel4 measures type loading with 4 workers
func BenchmarkTypeLoading_Parallel4(b *testing.B) {
	benchmarkTypeLoading(b, 4)
}

// BenchmarkTypeLoading_Parallel8 measures type loading with 8 workers
func BenchmarkTypeLoading_Parallel8(b *testing.B) {
	benchmarkTypeLoading(b, 8)
}

// BenchmarkTypeLoading_ParallelCPU measures type loading with CPU count workers
func BenchmarkTypeLoading_ParallelCPU(b *testing.B) {
	benchmarkTypeLoading(b, runtime.NumCPU()*2)
}

func benchmarkTypeLoading(b *testing.B, maxConcurrency int) {
	// Setup: scan packages and process types once
	config := NewDefaultConfig()
	config.MaxConcurrency = maxConcurrency
	ctx := NewScanningContext(context.Background(), config)

	scanner := NewGlobScanner()
	pkgs, err := scanner.ScanPackages(ScanModeFull, "../examples/starwars/...")
	if err != nil {
		b.Fatalf("Failed to scan packages: %v", err)
	}

	if len(pkgs) == 0 {
		b.Skip("No packages found to benchmark")
	}

	s := NewScanner()
	s.TypeResolver = NewDefaultTypeResolver(config, ctx.Logger)

	// Register and process packages
	visited := make(map[string]bool)
	var registerDeps func(*packages.Package)
	registerDeps = func(pkg *packages.Package) {
		if pkg == nil || visited[pkg.PkgPath] {
			return
		}
		visited[pkg.PkgPath] = true
		s.TypeResolver.(*defaultTypeResolver).pkgs.Set(pkg.PkgPath, pkg)
		for _, dep := range pkg.Imports {
			registerDeps(dep)
		}
	}

	for _, pkg := range pkgs {
		registerDeps(pkg)
	}

	for _, pkg := range pkgs {
		workerCtx := ctx.WithPackage(nil)
		if err := s.TypeResolver.ProcessPackage(workerCtx, pkg); err != nil {
			b.Fatalf("Failed to process package: %v", err)
		}
	}

	types := s.TypeResolver.GetTypes()
	if types.Len() == 0 {
		b.Skip("No types found to benchmark")
	}

	b.ResetTimer()
	b.ReportAllocs()

	// Benchmark type loading
	for i := 0; i < b.N; i++ {
		for _, t := range types.Values() {
			if err := t.Load(); err != nil {
				b.Errorf("Failed to load type %s: %v", t.Id(), err)
			}
		}
	}
}

// BenchmarkFullScan_Sequential measures complete scan with sequential processing
func BenchmarkFullScan_Sequential(b *testing.B) {
	benchmarkFullScan(b, 1)
}

// BenchmarkFullScan_Parallel measures complete scan with parallel processing
func BenchmarkFullScan_Parallel(b *testing.B) {
	benchmarkFullScan(b, runtime.NumCPU())
}

func benchmarkFullScan(b *testing.B, maxConcurrency int) {
	config := NewDefaultConfig()
	config.MaxConcurrency = maxConcurrency
	config.Packages = []string{"../examples/starwars/..."}
	config.LogLevel = "error" // Suppress logs during benchmarking

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		s := NewScanner()
		result, err := s.ScanWithConfig(config)
		if err != nil {
			b.Fatalf("Scan failed: %v", err)
		}
		if result.Types.Len() == 0 {
			b.Fatal("No types found")
		}
	}
}

// BenchmarkMemoryUsage measures memory usage during scanning
func BenchmarkMemoryUsage(b *testing.B) {
	config := NewDefaultConfig()
	config.Packages = []string{"../examples/starwars/..."}

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		s := NewScanner()
		result, err := s.ScanWithConfig(config)
		if err != nil {
			b.Fatalf("Scan failed: %v", err)
		}
		if result.Types.Len() == 0 {
			b.Fatal("No types found")
		}

		// Force GC to measure retained memory
		runtime.GC()
	}
}
