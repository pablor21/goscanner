package scannernew

import (
	"strings"

	"golang.org/x/tools/go/packages"
)

// PackageGlob represents a package glob pattern
type PackageGlob struct {
	Pattern   string
	Recursive bool
	ModPath   string
	PkgPath   string
}

// ParseGlob parses a glob pattern and returns a PackageGlob
func ParseGlob(pattern string) *PackageGlob {
	glob := &PackageGlob{
		Pattern: pattern,
	}

	// Check if pattern contains **/ for recursive scanning or ends with /**
	glob.Recursive = strings.Contains(pattern, "**/") || strings.HasSuffix(pattern, "/**")

	// Determine if it's a relative or absolute package path
	if strings.HasPrefix(pattern, "./") || strings.HasPrefix(pattern, "../") {
		glob.PkgPath = pattern
	} else {
		glob.ModPath = pattern
		glob.PkgPath = pattern
	}

	return glob
}

// ExpandGlob expands a glob pattern to concrete package paths
func (g *PackageGlob) ExpandGlob() ([]string, error) {
	var pkgs []string

	if strings.HasPrefix(g.Pattern, "./") || strings.HasPrefix(g.Pattern, "../") {
		pkgs = g.expandRelativeGlob()
	} else {
		pkgs = g.expandModuleGlob()
	}

	return pkgs, nil
}

// expandRelativeGlob handles patterns like ./** or ../pkg/**
func (g *PackageGlob) expandRelativeGlob() []string {
	var pkgs []string

	if strings.HasSuffix(g.Pattern, "/...") {
		pkgs = append(pkgs, g.Pattern)
	} else if strings.HasSuffix(g.Pattern, "/**") {
		baseDir := strings.TrimSuffix(g.Pattern, "/**")
		pkgs = append(pkgs, baseDir+"/...")
	} else if g.Recursive {
		baseDir := strings.TrimSuffix(g.Pattern, "/**")
		pkgs = append(pkgs, baseDir+"/...")
	} else {
		pkgs = append(pkgs, g.Pattern)
	}

	return pkgs
}

// expandModuleGlob handles patterns like github.com/mod/package/**
func (g *PackageGlob) expandModuleGlob() []string {
	var pkgs []string

	if g.Recursive {
		basePattern := strings.TrimSuffix(g.Pattern, "/**")
		if strings.Contains(basePattern, "*") {
			pkgs = g.expandWildcardPattern(basePattern)
		} else {
			pkgs = append(pkgs, basePattern+"/...")
		}
	} else {
		if strings.Contains(g.Pattern, "*") {
			pkgs = g.expandWildcardPattern(g.Pattern)
		} else {
			pkgs = append(pkgs, g.Pattern)
		}
	}

	return pkgs
}

// expandWildcardPattern expands patterns with * wildcards
func (g *PackageGlob) expandWildcardPattern(pattern string) []string {
	var pkgs []string

	if strings.Contains(pattern, "**/") {
		expanded := strings.ReplaceAll(pattern, "**/", "")
		pkgs = append(pkgs, expanded+"/...")
	} else {
		pkgs = append(pkgs, pattern)
	}

	return pkgs
}

// LoadPackages loads packages matching the glob pattern
func (g *PackageGlob) LoadPackages(mode ScanMode) ([]*packages.Package, error) {
	patterns, err := g.ExpandGlob()
	if err != nil {
		return nil, err
	}

	var loadMode packages.LoadMode

	// Always need basic package info
	loadMode = packages.NeedName | packages.NeedFiles | packages.NeedCompiledGoFiles | packages.NeedImports

	// Add modes based on ScanMode flags
	if mode.Has(ScanModeTypes) {
		loadMode |= packages.NeedTypes | packages.NeedTypesInfo
	}

	if mode.Has(ScanModeMethods) || mode.Has(ScanModeFields) || mode.Has(ScanModeDocs) || mode.Has(ScanModeComments) {
		loadMode |= packages.NeedSyntax
	}

	if mode.Has(ScanModeDocs) || mode.Has(ScanModeComments) {
		// Load dependencies with their syntax and types so we can extract their docs
		loadMode |= packages.NeedDeps | packages.NeedImports
	}

	config := &packages.Config{
		Mode: loadMode,
		// Tests: true, // Uncomment if you want to include test files
	}

	return packages.Load(config, patterns...)
}

// GlobScanner handles package discovery
type GlobScanner struct{}

func NewGlobScanner() *GlobScanner {
	return &GlobScanner{}
}

// ScanPackages scans packages matching the provided patterns
func (s *GlobScanner) ScanPackages(mode ScanMode, patterns ...string) ([]*packages.Package, error) {
	var allPackages []*packages.Package

	for _, pattern := range patterns {
		glob := ParseGlob(pattern)
		pkgs, err := glob.LoadPackages(mode)
		if err != nil {
			return nil, err
		}
		allPackages = append(allPackages, pkgs...)
	}

	return allPackages, nil
}
