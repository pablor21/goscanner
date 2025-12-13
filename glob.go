package goscanner

import (
	"go/build"
	"path/filepath"
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
		// Relative path - convert to absolute
		glob.PkgPath = pattern
	} else {
		// Absolute module path
		glob.ModPath = pattern
		glob.PkgPath = pattern
	}

	return glob
}

// ExpandGlob expands a glob pattern to concrete package paths
func (g *PackageGlob) ExpandGlob() ([]string, error) {
	var packages []string

	if strings.HasPrefix(g.Pattern, "./") || strings.HasPrefix(g.Pattern, "../") {
		// Handle relative paths
		packages = g.expandRelativeGlob()
	} else {
		// Handle module paths
		packages = g.expandModuleGlob()
	}

	return packages, nil
}

// expandRelativeGlob handles patterns like ./** or ../pkg/**
func (g *PackageGlob) expandRelativeGlob() []string {
	var packages []string

	if strings.HasSuffix(g.Pattern, "/...") {
		// Pattern like ../... or ./... - use it directly
		packages = append(packages, g.Pattern)
	} else if strings.HasSuffix(g.Pattern, "/**") {
		// Pattern like ./** or ../** - convert to /... for Go packages
		baseDir := strings.TrimSuffix(g.Pattern, "/**")
		packages = append(packages, baseDir+"/...")
	} else if g.Recursive {
		// Pattern like ./** - scan current directory recursively
		baseDir := strings.TrimSuffix(g.Pattern, "/**")
		packages = append(packages, baseDir+"/...")
	} else {
		// Pattern like ./package - scan specific directory
		packages = append(packages, g.Pattern)
	}

	return packages
}

// expandModuleGlob handles patterns like github.com/mod/package/**
func (g *PackageGlob) expandModuleGlob() []string {
	var packages []string

	if g.Recursive {
		// Remove /** and add /... for go packages tool
		basePattern := strings.TrimSuffix(g.Pattern, "/**")

		// Handle wildcards in the pattern
		if strings.Contains(basePattern, "*") {
			// Pattern like github.com/mod/**/pac*age
			packages = g.expandWildcardPattern(basePattern)
		} else {
			// Simple recursive pattern like github.com/mod/package
			packages = append(packages, basePattern+"/...")
		}
	} else {
		// Non-recursive pattern
		if strings.Contains(g.Pattern, "*") {
			// Pattern with wildcards like github.com/mod/pac*age
			packages = g.expandWildcardPattern(g.Pattern)
		} else {
			// Exact package like github.com/mod/package
			packages = append(packages, g.Pattern)
		}
	}

	return packages
}

// expandWildcardPattern expands patterns with * wildcards
func (g *PackageGlob) expandWildcardPattern(pattern string) []string {
	var packages []string

	// For now, let the packages.Load handle the wildcard expansion
	// The packages tool can handle patterns like github.com/mod/...
	// For complex wildcards, we might need to implement custom matching

	if strings.Contains(pattern, "**/") {
		// Convert **/ to /... for packages tool
		expanded := strings.ReplaceAll(pattern, "**/", "")
		packages = append(packages, expanded+"/...")
	} else {
		// Simple wildcards - let packages.Load handle them
		packages = append(packages, pattern)
	}

	return packages
}

// LoadPackages loads packages matching the glob pattern
func (g *PackageGlob) LoadPackages(mode ScanMode) ([]*packages.Package, error) {
	patterns, err := g.ExpandGlob()
	if err != nil {
		return nil, err
	}

	var loadMode packages.LoadMode

	// Always need basic package info
	loadMode = packages.NeedName

	// Add modes based on ScanMode flags
	if mode.Has(ScanModeTypes) {
		loadMode |= packages.NeedTypes
	}

	if mode.Has(ScanModeMethods) || mode.Has(ScanModeFields) || mode.Has(ScanModeDocs) || mode.Has(ScanModeAnnotations) {
		// Need syntax tree for detailed analysis
		loadMode |= packages.NeedSyntax | packages.NeedFiles
	}

	// Only add heavy TypesInfo if we need method/field details
	if mode.Has(ScanModeMethods) || mode.Has(ScanModeFields) {
		loadMode |= packages.NeedTypesInfo
	}
	cfg := &packages.Config{
		Mode: loadMode,
		// ParseFile: func(fset *token.FileSet, filename string, src []byte) (*ast.File, error) {
		// 	return parser.ParseFile(fset, filename, src, parser.ParseComments)
		// },
	}

	var allPackages []*packages.Package
	for _, pattern := range patterns {
		// Resolve relative paths to actual package import paths
		resolvedPattern, err := g.resolvePattern(pattern)
		if err != nil {
			return nil, err
		}

		pkgs, err := packages.Load(cfg, resolvedPattern)
		if err != nil {
			return nil, err
		}
		allPackages = append(allPackages, pkgs...)
	}

	return allPackages, nil
}

// resolvePattern resolves relative patterns to actual package import paths
func (g *PackageGlob) resolvePattern(pattern string) (string, error) {
	if !strings.HasPrefix(pattern, "./") && !strings.HasPrefix(pattern, "../") {
		// Not a relative pattern, return as is
		return pattern, nil
	}

	// For relative patterns, use the pattern as-is since packages.Load
	// can handle relative paths like ./... when run from the correct directory
	return pattern, nil
}

// MatchesPattern checks if a package path matches the glob pattern
func (g *PackageGlob) MatchesPattern(pkgPath string) bool {
	pattern := g.Pattern

	// Handle relative patterns like ../main, ./internal, etc.
	if strings.HasPrefix(pattern, "../") || strings.HasPrefix(pattern, "./") {
		// Extract the package name part
		parts := strings.Split(strings.TrimPrefix(pattern, "../"), "/")
		if len(parts) > 0 {
			packageName := parts[len(parts)-1]
			// Check if the pkgPath ends with this package name
			if strings.HasSuffix(pkgPath, "/"+packageName) || strings.HasSuffix(pkgPath, packageName) {
				return true
			}
		}
	}

	// Convert glob pattern to simple matching
	if strings.HasSuffix(pattern, "/**") {
		// Recursive pattern - check if pkgPath starts with base
		base := strings.TrimSuffix(pattern, "/**")

		// Handle relative base patterns
		if strings.HasPrefix(base, "../") || strings.HasPrefix(base, "./") {
			// For relative patterns, check if the package path contains the base pattern
			baseName := strings.TrimPrefix(base, "../")
			baseName = strings.TrimPrefix(baseName, "./")
			if baseName == "" {
				return true // Match everything for ./** or ../**
			}
			return strings.Contains(pkgPath, baseName)
		}

		return strings.HasPrefix(pkgPath, base)
	}

	// Handle wildcards
	if strings.Contains(pattern, "*") {
		// For relative patterns with wildcards, try to match the end of the path
		if strings.HasPrefix(pattern, "../") || strings.HasPrefix(pattern, "./") {
			cleanPattern := strings.TrimPrefix(pattern, "../")
			cleanPattern = strings.TrimPrefix(cleanPattern, "./")
			matched, _ := filepath.Match(cleanPattern, filepath.Base(pkgPath))
			if matched {
				return true
			}
			// Also try matching against the full path
			matched, _ = filepath.Match(cleanPattern, pkgPath)
			return matched
		}

		matched, _ := filepath.Match(pattern, pkgPath)
		return matched
	}

	// Exact match
	if pattern == pkgPath {
		return true
	}

	// For relative patterns, also check if it matches the package name
	if strings.HasPrefix(pattern, "../") || strings.HasPrefix(pattern, "./") {
		packageName := strings.TrimPrefix(pattern, "../")
		packageName = strings.TrimPrefix(packageName, "./")
		return strings.HasSuffix(pkgPath, "/"+packageName) || pkgPath == packageName
	}

	return false
}

// MatchesPackageName checks if a package name matches the glob pattern
func (g *PackageGlob) MatchesPackageName(packageName string) bool {
	pattern := g.Pattern

	// Handle relative patterns like ./main, ../main
	if strings.HasPrefix(pattern, "../") || strings.HasPrefix(pattern, "./") {
		// Extract the package name part from the pattern
		cleanPattern := strings.TrimPrefix(pattern, "../")
		cleanPattern = strings.TrimPrefix(cleanPattern, "./")

		// If the clean pattern matches the package name exactly, it's a match
		if cleanPattern == packageName {
			return true
		}
	}

	// For non-relative patterns, check direct match
	if pattern == packageName {
		return true
	}

	return false
}

// GlobScanner provides utility functions for package glob operations
type GlobScanner struct{}

// NewGlobScanner creates a new glob scanner
func NewGlobScanner() *GlobScanner {
	return &GlobScanner{}
}

// ScanPackages scans packages matching one or more glob patterns
// Patterns starting with ! are treated as exclusions
func (gs *GlobScanner) ScanPackages(mode ScanMode, patterns ...string) ([]*packages.Package, error) {
	var includePatterns []string
	var excludePatterns []string

	// Separate include and exclude patterns
	for _, pattern := range patterns {
		if strings.HasPrefix(pattern, "!") {
			// Exclusion pattern - remove the ! prefix
			excludePattern := strings.TrimPrefix(pattern, "!")
			excludePatterns = append(excludePatterns, excludePattern)
		} else {
			// Include pattern
			includePatterns = append(includePatterns, pattern)
		}
	}

	var allPackages []*packages.Package

	// Load packages for include patterns
	for _, pattern := range includePatterns {
		glob := ParseGlob(pattern)
		pkgs, err := glob.LoadPackages(mode)
		if err != nil {
			return nil, err
		}
		allPackages = append(allPackages, pkgs...)
	} // Filter out excluded packages
	var filteredPackages []*packages.Package
	for _, pkg := range allPackages {
		excluded := false

		// Check if package matches any exclusion pattern
		for _, excludePattern := range excludePatterns {
			excludeGlob := ParseGlob(excludePattern)
			if excludeGlob.MatchesPattern(pkg.PkgPath) || excludeGlob.MatchesPackageName(pkg.Name) {
				excluded = true
				break
			}
		}

		if !excluded {
			filteredPackages = append(filteredPackages, pkg)
		}
	}

	// Remove duplicates from remaining packages
	seen := make(map[string]bool)
	var uniquePackages []*packages.Package
	for _, pkg := range filteredPackages {
		if !seen[pkg.PkgPath] {
			seen[pkg.PkgPath] = true
			uniquePackages = append(uniquePackages, pkg)
		}
	}

	return uniquePackages, nil
}

// GetCurrentModulePath returns the current module path
func (gs *GlobScanner) GetCurrentModulePath() (string, error) {
	// Try to get from go.mod in current directory
	ctx := build.Default
	pkg, err := ctx.ImportDir(".", build.FindOnly)
	if err != nil {
		return "", err
	}
	return pkg.ImportPath, nil
}
