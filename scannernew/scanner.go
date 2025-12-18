package scannernew

import (
	"fmt"
	"runtime"
	"sort"
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
	ctx := NewScanningContext(config)
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
		s.TypeResolver.(*defaultTypeResolver).pkgs[pkg.PkgPath] = pkg

		// Recursively register dependencies
		for _, dep := range pkg.Imports {
			registerDeps(dep)
		}
	}

	// Register all packages and their dependencies
	for _, pkg := range pkgs {
		registerDeps(pkg)
	}

	// process the packages and generate the scanning result
	for _, pkg := range pkgs {
		// scan the package for types
		err := s.ScanTypes(pkg)
		if err != nil {
			return nil, err
		}
	}

	totalPackages = len(pkgs)

	result := &ScanningResult{
		Types:    s.TypeResolver.GetTypes(),
		Values:   s.TypeResolver.GetValues(),
		Packages: s.TypeResolver.GetPackages(),
	}

	// Trigger lazy loading of all types
	// Keep loading until no new types are discovered
	// (Loading a type can trigger resolution of new types like field types)
	// Sort IDs to ensure deterministic iteration order
	loadedTypes := make(map[string]bool)
	for {
		// Get all type IDs and sort them for deterministic order
		var typeIDs []string
		for _, t := range result.Types.Values() {
			if !loadedTypes[t.Id()] {
				typeIDs = append(typeIDs, t.Id())
			}
		}

		if len(typeIDs) == 0 {
			break // No new types to load
		}

		// Sort to ensure deterministic loading order
		sort.Strings(typeIDs)

		for _, id := range typeIDs {
			t, exists := result.Types.Get(id)
			if !exists {
				continue
			}
			loadedTypes[id] = true

			if err := t.Load(); err != nil {
				ctx.Logger.Error(fmt.Sprintf("Failed to load type %s: %v", t.Id(), err))
			}
		}
	}

	// Return the scanning result and any errors encountered
	return result, nil
}

func (s *DefaultScanner) GetTypeResolver() TypeResolver {
	return s.TypeResolver
}

func (s *DefaultScanner) ScanTypes(pkg *packages.Package) error {
	return s.TypeResolver.ProcessPackage(pkg)
}
