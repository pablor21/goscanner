package scanner

import (
	"context"
	"go/types"

	gstypes "github.com/pablor21/goscanner/types"

	"github.com/pablor21/goscanner/logger"
)

// ScanningContext holds the configuration and state information for the scanning process
// It embeds context.Context to support cancellation and deadlines while adding
// scanner-specific state like current package information.
type ScanningContext struct {
	context.Context                       // Embed standard context for cancellation/timeout
	Config          *Config               // Scanner configuration
	Logger          logger.Logger         // Logger instance
	ScanMode        ScanMode              // Scanning mode
	typesCache      map[string]types.Type // Legacy cache (consider deprecating)
	ignoredTypes    map[string]struct{}   // Types to ignore

	// Package-specific context (set per-package during scanning)
	currentPkg   *gstypes.Package // Currently processing package
	resolvingPkg string           // Package path being resolved (for distance calculation)
}

// NewScanningContext creates a new scanning context from the root context
func NewScanningContext(ctx context.Context, config *Config) *ScanningContext {
	if ctx == nil {
		ctx = context.Background()
	}
	logger.SetupLogger(config.LogLevel)
	return &ScanningContext{
		Context:      ctx,
		Config:       config,
		ScanMode:     config.ScanMode,
		Logger:       logger.NewDefaultLogger(),
		typesCache:   make(map[string]types.Type),
		ignoredTypes: make(map[string]struct{}),
	}
}

// WithPackage returns a new context with the current package set
// This allows each goroutine to have its own package context
func (sc *ScanningContext) WithPackage(pkg *gstypes.Package) *ScanningContext {
	newCtx := *sc // Shallow copy
	newCtx.currentPkg = pkg
	if pkg != nil && sc.resolvingPkg == "" {
		newCtx.resolvingPkg = pkg.Path()
	}
	return &newCtx
}

// WithResolvingPackage returns a new context with the resolving package path set
func (sc *ScanningContext) WithResolvingPackage(pkgPath string) *ScanningContext {
	newCtx := *sc // Shallow copy
	newCtx.resolvingPkg = pkgPath
	return &newCtx
}

// CurrentPackage returns the currently processing package
func (sc *ScanningContext) CurrentPackage() *gstypes.Package {
	return sc.currentPkg
}

// ResolvingPackage returns the package path being resolved
func (sc *ScanningContext) ResolvingPackage() string {
	return sc.resolvingPkg
}
