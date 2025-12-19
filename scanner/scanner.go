package scanner

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
	"time"

	"golang.org/x/tools/go/packages"
)

type Scanner interface {
	AddProcessor(processor Processor)
	SetProcessors(processors []Processor)
	Scan() (*ScanningResult, error)
	ScanWithConfig(config *Config) (*ScanningResult, error)
	ScanWithContext(ctx *ScanningContext) (*ScanningResult, error)
	GetTypeResolver() TypeResolver
}

type DefaultScanner struct {
	Processors   []Processor
	Context      *ScanningContext
	TypeResolver TypeResolver
}

func NewScanner() *DefaultScanner {
	return &DefaultScanner{
		Processors: []Processor{},
	}
}

func (s *DefaultScanner) AddProcessor(processor Processor) {
	s.Processors = append(s.Processors, processor)
}

func (s *DefaultScanner) SetProcessors(processors []Processor) {
	s.Processors = processors
}

func (s *DefaultScanner) Scan() (*ScanningResult, error) {
	return s.ScanWithConfig(NewDefaultConfig())
}

func (s *DefaultScanner) ScanWithConfig(config *Config) (*ScanningResult, error) {
	if config == nil {
		return s.Scan()
	}
	// init the scanning context with the provided configuration
	ctx := NewScanningContext(context.Background(), config)
	return s.ScanWithContext(ctx)
}

func (s *DefaultScanner) ScanWithContext(ctx *ScanningContext) (*ScanningResult, error) {
	// start timer and log start message
	ctx.Logger.Info("Starting scan...")
	totalPackages := 0
	now := time.Now()
	var m1, m2 runtime.MemStats
	var memoryUsage uint64

	runtime.GC()
	runtime.ReadMemStats(&m1)

	defer func() {
		runtime.GC()
		runtime.ReadMemStats(&m2)
		memoryUsage = (m2.Alloc - m1.Alloc) / 1024 // in KB
		ctx.Logger.Info(fmt.Sprintf("Scan completed in %v, found %d types, across %d packages, memory usage: %dKB", time.Since(now), s.TypeResolver.GetTypes().Len(), totalPackages, memoryUsage))
	}()

	if ctx == nil || ctx.Config == nil {
		panic("No scanning context provided or config invalid")
	}
	// Initialize the scanning result
	s.Context = ctx

	// determine the scanning mode based on the provided configuration (get the maximum depth of the scan)
	for _, processor := range s.Processors {
		if processor.ScanMode() > ctx.ScanMode {
			ctx.ScanMode = processor.ScanMode()
		}
	}
	// create the glob pattern based on the provided configuration
	scanner := NewGlobScanner()
	pkgs, err := scanner.ScanPackages(ctx.ScanMode, ctx.Config.Packages...)
	if err != nil {
		return nil, err
	}

	// set the scanmode in the type resolver
	s.TypeResolver = NewDefaultTypeResolver(ctx.Config, ctx.Logger)

	// Register dependency packages so we can load their docs when needed
	visited := make(map[string]bool)
	var registerDeps func(*packages.Package)
	registerDeps = func(pkg *packages.Package) {
		if pkg == nil || visited[pkg.PkgPath] {
			return
		}
		visited[pkg.PkgPath] = true

		// Register this package in the type resolver's package map
		s.TypeResolver.(*defaultTypeResolver).pkgs.Set(pkg.PkgPath, pkg)

		// Recursively register dependencies
		for _, dep := range pkg.Imports {
			registerDeps(dep)
		}
	}

	// Register all packages and their dependencies
	for _, pkg := range pkgs {
		registerDeps(pkg)
	}

	// Process packages in parallel using worker pool
	// Number of workers = configured max_concurrency (0 means CPU cores)
	numWorkers := ctx.Config.MaxConcurrency
	if numWorkers <= 0 {
		numWorkers = runtime.NumCPU()
	}
	if len(pkgs) < numWorkers {
		numWorkers = len(pkgs)
	}

	var wg sync.WaitGroup
	pkgChan := make(chan *packages.Package, len(pkgs))
	errChan := make(chan error, len(pkgs))

	// Start worker goroutines
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()
			for pkg := range pkgChan {
				// Each worker gets its own context copy with the package
				workerCtx := ctx.WithPackage(nil) // Reset to clean state for this package
				if err := s.TypeResolver.ProcessPackage(workerCtx, pkg); err != nil {
					errChan <- fmt.Errorf("worker %d failed to process %s: %w", workerID, pkg.PkgPath, err)
					return
				}
			}
		}(i)
	}

	// Send packages to workers
	for _, pkg := range pkgs {
		pkgChan <- pkg
	}
	close(pkgChan)

	// Wait for all workers to complete
	wg.Wait()
	close(errChan)

	// Check for errors
	if len(errChan) > 0 {
		return nil, <-errChan
	}

	totalPackages = len(pkgs)

	result := &ScanningResult{
		Types:    s.TypeResolver.GetTypes(),
		Values:   s.TypeResolver.GetValues(),
		Packages: s.TypeResolver.GetPackages(),
	}

	// Trigger lazy loading of all types in parallel
	// Keep loading until no new types are discovered
	// (Loading a type can trigger resolution of new types like field types)
	// Use worker pool to limit concurrency and handle dynamic type discovery
	loadedTypes := sync.Map{} // Thread-safe map for tracking loaded types
	maxRetries := 3

	for {
		// Get all type IDs that haven't been loaded yet
		var typeIDs []string
		for _, t := range result.Types.Values() {
			if _, loaded := loadedTypes.Load(t.Id()); !loaded {
				typeIDs = append(typeIDs, t.Id())
			}
		}

		if len(typeIDs) == 0 {
			break // No new types to load
		}

		// Sort to ensure deterministic loading order
		sort.Strings(typeIDs)

		// Parallel type loading with worker pool
		numWorkers := ctx.Config.MaxConcurrency
		if numWorkers <= 0 {
			numWorkers = runtime.NumCPU() * 2 // More workers for I/O-bound loading
		}
		if len(typeIDs) < numWorkers {
			numWorkers = len(typeIDs)
		}

		var wg sync.WaitGroup
		typeChan := make(chan string, len(typeIDs))
		errChan := make(chan error, len(typeIDs))

		// Start worker goroutines for type loading
		for i := 0; i < numWorkers; i++ {
			wg.Add(1)
			go func(workerID int) {
				defer wg.Done()
				for id := range typeChan {
					// Retry mechanism for failed loads
					var loadErr error
					for attempt := 0; attempt < maxRetries; attempt++ {
						t, exists := result.Types.Get(id)
						if !exists {
							break // Type disappeared, skip it
						}

						loadErr = t.Load()
						if loadErr == nil {
							loadedTypes.Store(id, true)
							break // Success
						}

						// Log retry attempts
						if attempt < maxRetries-1 {
							ctx.Logger.Debug(fmt.Sprintf("Retry %d/%d loading type %s: %v", attempt+1, maxRetries, id, loadErr))
						}
					}

					if loadErr != nil {
						errChan <- fmt.Errorf("failed to load type %s after %d attempts: %w", id, maxRetries, loadErr)
						ctx.Logger.Error(fmt.Sprintf("Failed to load type %s: %v", id, loadErr))
					}
				}
			}(i)
		}

		// Send type IDs to workers
		for _, id := range typeIDs {
			typeChan <- id
		}
		close(typeChan)

		// Wait for all workers to complete
		wg.Wait()
		close(errChan)

		// Log any errors (non-fatal, continue processing)
		for err := range errChan {
			ctx.Logger.Debug(err.Error())
		}
	}

	// Return the scanning result and any errors encountered
	return result, nil
}

func (s *DefaultScanner) GetTypeResolver() TypeResolver {
	return s.TypeResolver
}

func (s *DefaultScanner) ScanTypes(pkg *packages.Package) error {
	return s.TypeResolver.ProcessPackage(s.Context, pkg)
}
