package goscanner

import (
	"fmt"
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
		TypeResolver: newDefaultTypeResolver(ScanModeBasic),
		Processors:   []Processor{},
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
	memoryUsage := RSS()
	defer func() {
		ctx.Logger.Info(fmt.Sprintf("Scan completed in %v, found %d types, accross %d packages, memory usage: %dKB", time.Since(now), len(s.TypeResolver.GetTypeInfos()), totalPackages, memoryUsage))
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
	s.TypeResolver = newDefaultTypeResolver(ctx.ScanMode)

	// process the packages and generate the scanning result
	for _, pkg := range pkgs {
		// scan the package for types
		err := s.ScanTypes(pkg)
		if err != nil {
			return nil, err
		}
	}

	totalPackages = len(pkgs)
	// Calculate memory usage in MB
	memoryUsage = (RSS() - memoryUsage)

	ret = &ScanningResult{
		Types: s.TypeResolver.GetTypeInfos(),
	}

	// trigger the lazy loading of each type
	for _, t := range ret.Types {
		t.Load()
	}

	// Return the scanning result and any errors encountered
	return ret, err
}

func (s *DefaultScanner) GetTypeResolver() TypeResolver {
	return s.TypeResolver
}

func (s *DefaultScanner) ScanTypes(pkg *packages.Package) error {
	return s.TypeResolver.ProcessPackage(pkg)
}
