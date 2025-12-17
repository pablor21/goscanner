package scanner

import (
	"fmt"
	"runtime"
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
		// TypeResolver: newDefaultTypeResolver(ScanModeBasic, nil),
		Processors: []Processor{},
	}
}

func (s *DefaultScanner) AddProcessor(processor Processor) {
	s.Processors = append(s.Processors, processor)
}

func (s *DefaultScanner) SetProcessors(processors []Processor) {
	s.Processors = processors
}

func (s *DefaultScanner) Scan() (ret *ScanningResult, err error) {
	return s.ScanWithConfig(NewDefaultConfig())
}

func (s *DefaultScanner) ScanWithConfig(config *Config) (ret *ScanningResult, err error) {
	if config == nil {
		return s.Scan()
	}
	// init the scanning context with the provided configuration
	ctx := NewScanningContext(config)
	return s.ScanWithContext(ctx)
}

func (s *DefaultScanner) ScanWithContext(ctx *ScanningContext) (ret *ScanningResult, err error) {

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
		ctx.Logger.Info(fmt.Sprintf("Scan completed in %v, found %d types, across %d packages, memory usage: %dKB", time.Since(now), len(s.TypeResolver.GetTypeInfos()), totalPackages, memoryUsage))
	}()

	if ctx == nil || ctx.Config == nil {
		panic(`No scanning context provided or config invalid!`)
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
	s.TypeResolver = newDefaultTypeResolver(ctx.Config, ctx.Logger)

	// process the packages and generate the scanning result
	for _, pkg := range pkgs {
		// scan the package for types
		err := s.ScanTypes(pkg)
		if err != nil {
			return nil, err
		}
	}

	totalPackages = len(pkgs)

	ret = &ScanningResult{
		Types:    s.TypeResolver.GetTypeInfos(),
		Values:   s.TypeResolver.GetValueInfos(),
		Packages: s.TypeResolver.GetPackageInfos(),
		//BasicTypes:        s.TypeResolver.GetBasicTypes(),
		// GenericParamTypes: s.TypeResolver.GetGenericParamTypes(),
	}

	// trigger the lazy loading of each type
	for _, t := range ret.Types {
		if loadable, ok := t.(interface{ Load() error }); ok {
			err := loadable.Load()
			if err != nil {
				// just log the error but continue processing other types
				ctx.Logger.Error(fmt.Sprintf("Failed to load type %s: %v", t.Id(), err))
			}
		}
	}

	// Return the scanning result and any errors encountered
	return ret, err
}

// // filterGenericArtifacts removes generic instantiation artifacts that contain unresolved type parameters
// func (s *DefaultScanner) filterGenericArtifacts(types map[string]TypeInfo) map[string]TypeInfo {
// 	filtered := make(map[string]TypeInfo)
// 	for name, typeInfo := range types {
// 		// Filter out map/slice/array types with unresolved generic parameters
// 		if s.containsUnresolvedGenerics(name) {
// 			continue
// 		}
// 		filtered[name] = typeInfo
// 	}
// 	return filtered
// }

// // containsUnresolvedGenerics checks if a type name contains unresolved generic parameters
// func (s *DefaultScanner) containsUnresolvedGenerics(typeName string) bool {
// 	// Check for patterns like "map[string]pkg.T" where T is likely a generic parameter
// 	if strings.Contains(typeName, "map[") || strings.Contains(typeName, "[]") || strings.Contains(typeName, "[") {
// 		// Check for single letter generic parameters after package paths
// 		genericPatterns := []string{".T", ".U", ".K", ".V", ".A", ".S", ".M", ".P", ".Q"}
// 		for _, pattern := range genericPatterns {
// 			if strings.Contains(typeName, pattern) {
// 				// Check if it's followed by ] or ) or end of string (indicating it's a generic parameter)
// 				idx := strings.Index(typeName, pattern)
// 				if idx >= 0 && idx+len(pattern) < len(typeName) {
// 					nextChar := typeName[idx+len(pattern)]
// 					if nextChar == ']' || nextChar == ')' || nextChar == '>' || nextChar == ',' || nextChar == ' ' {
// 						return true
// 					}
// 				}
// 				if idx >= 0 && idx+len(pattern) == len(typeName) {
// 					return true
// 				}
// 			}
// 		}
// 	}
// 	return false
// }

func (s *DefaultScanner) GetTypeResolver() TypeResolver {
	return s.TypeResolver
}

func (s *DefaultScanner) ScanTypes(pkg *packages.Package) error {
	return s.TypeResolver.ProcessPackage(pkg)
}
