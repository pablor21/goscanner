package scanner

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/types"
	"reflect"
	"strings"

	"github.com/pablor21/goscanner/logger"
	"golang.org/x/tools/go/packages"

	gstypes "github.com/pablor21/goscanner/types"
)

// TypeResolver interface for resolving types and managing type information
type TypeResolver interface {
	// ResolveType resolves a types.Type to a types.Type
	ResolveType(t types.Type) gstypes.Type
	// GetCanonicalName returns the canonical name of a type
	GetCanonicalName(t types.Type) string
	// ProcessPackage processes a package to extract and cache type information
	ProcessPackage(pkg *packages.Package) error
	// GetTypes returns all resolved types
	GetTypes() *gstypes.TypesCol[gstypes.Type]
	// GetValues returns all resolved values (constants/variables)
	GetValues() *gstypes.TypesCol[*gstypes.Value]
	// GetPackages returns all loaded packages
	GetPackages() *gstypes.TypesCol[*gstypes.Package]
}

type defaultTypeResolver struct {
	types            *gstypes.TypesCol[gstypes.Type]     // All resolved types
	values           *gstypes.TypesCol[*gstypes.Value]   // All resolved values (constants/variables)
	packages         *gstypes.TypesCol[*gstypes.Package] // All loaded packages
	ignoredTypes     map[string]struct{}                 // Types to ignore
	docTypes         map[string]*doc.Type                // Documentation for types
	docFuncs         map[string]*doc.Func                // Documentation for functions
	docPackages      map[string]*doc.Package             // Cached doc.Package by package path
	pkgs             map[string]*packages.Package        // Raw go/packages
	loadedPkgs       map[string]bool                     // Track processed packages
	packageDistances map[string]int                      // Track distance for each package from scanned packages
	basicTypes       map[string]gstypes.Type             // Cache of basic types
	currentPkg       *gstypes.Package                    // Currently processing package
	resolvingPkg     string                              // Package path being resolved (for distance calculation)
	unnamedCounter   map[string]int                      // Counter for unnamed types per kind
	qualifier        types.Qualifier                     // Cached qualifier function for GetCanonicalName
	config           *Config
	logger           logger.Logger
}

// NewDefaultTypeResolver creates a new type resolver
func NewDefaultTypeResolver(config *Config, log logger.Logger) *defaultTypeResolver {
	if log == nil {
		log = logger.NewDefaultLogger()
	}

	tr := &defaultTypeResolver{
		types:            gstypes.NewTypesCol[gstypes.Type](),
		values:           gstypes.NewTypesCol[*gstypes.Value](),
		packages:         gstypes.NewTypesCol[*gstypes.Package](),
		ignoredTypes:     make(map[string]struct{}),
		docTypes:         make(map[string]*doc.Type),
		docFuncs:         make(map[string]*doc.Func),
		docPackages:      make(map[string]*doc.Package),
		pkgs:             make(map[string]*packages.Package),
		loadedPkgs:       make(map[string]bool),
		packageDistances: make(map[string]int),
		basicTypes:       make(map[string]gstypes.Type),
		unnamedCounter:   make(map[string]int),
		qualifier: func(pkg *types.Package) string {
			return pkg.Path()
		},
		config: config,
		logger: log,
	}

	tr.logger.SetTag("TypeResolver")

	// Initialize basic types cache
	tr.initBasicTypes()

	return tr
}

// initBasicTypes creates cached basic type instances
func (r *defaultTypeResolver) initBasicTypes() {
	for _, basicTypeName := range gstypes.BasicTypes {
		basicType := gstypes.NewBasic(basicTypeName, basicTypeName)
		r.basicTypes[basicTypeName] = basicType
	}
}

// generateUnnamedID generates a unique ID for unnamed composite types
func (r *defaultTypeResolver) generateUnnamedID(kind string) string {
	r.unnamedCounter[kind]++
	return fmt.Sprintf("__unnamed_%s__%d__", kind, r.unnamedCounter[kind])
}

func (r *defaultTypeResolver) GetTypes() *gstypes.TypesCol[gstypes.Type] {
	return r.types
}

func (r *defaultTypeResolver) GetValues() *gstypes.TypesCol[*gstypes.Value] {
	return r.values
}

func (r *defaultTypeResolver) GetPackages() *gstypes.TypesCol[*gstypes.Package] {
	return r.packages
}

func (r *defaultTypeResolver) GetCanonicalName(t types.Type) string {
	if t == nil {
		return ""
	}

	// If it's a basic type, return its name directly
	if basic, ok := t.(*types.Basic); ok {
		return basic.Name()
	}

	// For named types, check if it's an instantiated generic first (has type arguments)
	// If it is, use the full name with type arguments (e.g., GenericStruct[int])
	// Otherwise, if it's a generic type definition (has type parameters), return the base name
	if named, ok := t.(*types.Named); ok {
		// Check for type arguments first (instantiated generic like GenericStruct[int])
		if named.TypeArgs() != nil && named.TypeArgs().Len() > 0 {
			// This is an instantiated generic, use TypeString to get full name with args
			name := types.TypeString(t, r.qualifier)
			return name
		}
		// Check for type parameters (generic type definition like GenericStruct[T])
		if named.TypeParams() != nil && named.TypeParams().Len() > 0 {
			obj := named.Obj()
			if obj.Pkg() != nil {
				return obj.Pkg().Path() + "." + obj.Name()
			}
			return obj.Name()
		}
	}

	name := types.TypeString(t, r.qualifier)

	return name
}

// getPackageInfo returns the package info for the given object
// If obj is nil or has no package, returns currentPkg
func (r *defaultTypeResolver) getPackageInfo(obj types.Object) *gstypes.Package {
	if obj != nil && obj.Pkg() != nil {
		pkgPath := obj.Pkg().Path()
		if pkgInfo, exists := r.packages.Get(pkgPath); exists {
			return pkgInfo
		}

		// Check if this is an external package and if we should parse its files
		isExternal := r.currentPkg != nil && pkgPath != r.currentPkg.Path()
		shouldParseFiles := isExternal &&
			r.config.ExternalPackagesOptions != nil &&
			r.config.ExternalPackagesOptions.ParseFiles

		var rawPkg *packages.Package
		if shouldParseFiles {
			// Load the external package with AST to extract comments and files
			rawPkg = r.loadExternalPackage(pkgPath)
		}

		// Create package info
		pkgInfo := gstypes.NewPackage(pkgPath, obj.Pkg().Name(), rawPkg)
		pkgInfo.SetLogger(r.logger)
		r.packages.Set(pkgPath, pkgInfo)

		// Calculate distance for this package (use minimum distance if already exists)
		refPkg := r.resolvingPkg
		if refPkg == "" && r.currentPkg != nil {
			refPkg = r.currentPkg.Path()
		}

		newDistance := 1 // default distance
		if refPkg != "" {
			if refDist, ok := r.packageDistances[refPkg]; ok {
				// External package is one step further than the package that references it
				newDistance = refDist + 1
			}
		}

		// Update distance if this is a shorter path or first time seeing this package
		if existingDist, exists := r.packageDistances[pkgPath]; !exists || newDistance < existingDist {
			r.packageDistances[pkgPath] = newDistance
		}

		// Extract comments and files if we loaded the AST
		if rawPkg != nil && len(rawPkg.Syntax) > 0 {
			if err := r.extractComments(pkgInfo, rawPkg); err != nil {
				r.logger.Warnf("Failed to extract comments for external package %s: %v", pkgPath, err)
			}
			// Store the raw package for later use
			r.pkgs[pkgPath] = rawPkg
		}

		return pkgInfo
	}
	return r.currentPkg
}

// getPackageForObj returns the raw packages.Package for the given object
func (r *defaultTypeResolver) getPackageForObj(obj types.Object) *packages.Package {
	if obj != nil && obj.Pkg() != nil {
		pkgPath := obj.Pkg().Path()
		if pkg, exists := r.pkgs[pkgPath]; exists {
			return pkg
		}
	}
	return nil
}

// getModuleRelativePath converts an OS path to a module-relative path
func (r *defaultTypeResolver) getModuleRelativePath(osPath string, pkgPath string) string {
	if osPath == "" || pkgPath == "" {
		return osPath
	}

	// Extract filename from OS path
	fileName := osPath
	if idx := strings.LastIndex(osPath, "/"); idx >= 0 {
		fileName = osPath[idx+1:]
	}

	// Combine package path + filename
	var sb strings.Builder
	sb.WriteString(pkgPath)
	sb.WriteString("/")
	sb.WriteString(fileName)
	return sb.String()
}

// loadExternalPackage loads an external package with its AST for comment extraction
func (r *defaultTypeResolver) loadExternalPackage(pkgPath string) *packages.Package {
	// Check if already loaded
	if pkg, exists := r.pkgs[pkgPath]; exists {
		return pkg
	}

	r.logger.Debugf("Loading external package with AST: %s", pkgPath)

	// Load package with AST (NeedSyntax includes NeedTypes and NeedImports)
	cfg := &packages.Config{
		Mode: packages.NeedName | packages.NeedFiles | packages.NeedImports |
			packages.NeedTypes | packages.NeedSyntax | packages.NeedTypesInfo,
	}

	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		r.logger.Warnf("Failed to load external package %s: %v", pkgPath, err)
		return nil
	}

	if len(pkgs) == 0 {
		r.logger.Warnf("No packages found for %s", pkgPath)
		return nil
	}

	if len(pkgs[0].Errors) > 0 {
		r.logger.Warnf("Errors loading package %s: %v", pkgPath, pkgs[0].Errors)
	}

	return pkgs[0]
}

// loadExternalPackageDoc loads documentation for an external package if not already loaded
func (r *defaultTypeResolver) loadExternalPackageDoc(pkgPath string, obj types.Object) *doc.Type {
	// Don't try to load if we don't have the object
	if obj == nil || obj.Pkg() == nil {
		return nil
	}

	// Build the canonical type name
	var sb strings.Builder
	sb.WriteString(pkgPath)
	sb.WriteString(".")
	sb.WriteString(obj.Name())
	typeName := sb.String()

	// If we've already loaded this package's docs, return from cache
	if r.loadedPkgs[pkgPath] {
		return r.docTypes[typeName] // Return cached value (may be nil if type has no docs)
	}

	// Mark as loaded to prevent re-attempting
	r.loadedPkgs[pkgPath] = true

	// Try to get the package from our packages map (if it was loaded with dependencies)
	pkg, exists := r.pkgs[pkgPath]
	if !exists {
		// Package not available, can't load docs
		return nil
	}

	// Extract documentation from the package
	if len(pkg.Syntax) > 0 {
		// Check if we already have the doc.Package cached
		docPkg, cached := r.docPackages[pkgPath]
		if !cached {
			// Not cached - parse and cache it
			var err error
			docPkg, err = doc.NewFromFiles(
				pkg.Fset,
				pkg.Syntax,
				pkg.PkgPath,
				doc.AllMethods|doc.AllDecls,
			)
			if err != nil {
				r.logger.Debugf("Failed to extract docs from external package %s: %v", pkgPath, err)
				return nil
			}
			r.docPackages[pkgPath] = docPkg
		}

		// Cache the doc types from this package
		for _, docType := range docPkg.Types {
			var sb strings.Builder
			sb.WriteString(pkgPath)
			sb.WriteString(".")
			sb.WriteString(docType.Name)
			canonical := sb.String()
			r.docTypes[canonical] = docType
		}

		// Return the specific docType for this object
		return r.docTypes[typeName]
	}

	return nil
}

// ProcessPackage processes a package to extract type information
func (r *defaultTypeResolver) ProcessPackage(pkg *packages.Package) error {
	// Create package info
	pkgInfo := gstypes.NewPackage(pkg.PkgPath, pkg.Name, pkg)
	pkgInfo.SetLogger(r.logger)
	r.packages.Set(pkg.PkgPath, pkgInfo)
	r.currentPkg = pkgInfo

	// Mark this package as scanned (distance 0)
	r.packageDistances[pkg.PkgPath] = 0

	// Extract comments from AST
	if err := r.extractComments(pkgInfo, pkg); err != nil {
		r.logger.Warnf("Failed to extract comments: %v", err)
	}

	// Extract documentation - check cache first
	docPkg, cached := r.docPackages[pkg.PkgPath]
	if !cached {
		var err error
		docPkg, err = doc.NewFromFiles(
			pkg.Fset,
			pkg.Syntax,
			pkg.PkgPath,
			doc.AllMethods|doc.AllDecls,
		)
		if err != nil {
			return err
		}
		r.docPackages[pkg.PkgPath] = docPkg
	}

	r.pkgs[pkg.PkgPath] = pkg
	r.loadedPkgs[pkg.PkgPath] = true

	// Cache scope for efficiency
	scope := pkg.Types.Scope()

	// Package-level functions documentation
	for _, docFunc := range docPkg.Funcs {
		var sb strings.Builder
		sb.WriteString(pkg.PkgPath)
		sb.WriteString(".")
		sb.WriteString(docFunc.Name)
		canonical := sb.String()
		r.docFuncs[canonical] = docFunc
	}

	// Types + associated functions
	if r.config.ScanMode.Has(ScanModeTypes) {
		for _, docType := range docPkg.Types {
			var sb strings.Builder
			sb.WriteString(pkg.PkgPath)
			sb.WriteString(".")
			sb.WriteString(docType.Name)
			typeCanonical := sb.String()
			r.docTypes[typeCanonical] = docType

			// Factory functions associated with the type
			for _, typeFunc := range docType.Funcs {
				var sb strings.Builder
				sb.WriteString(pkg.PkgPath)
				sb.WriteString(".")
				sb.WriteString(typeFunc.Name)
				funcCanonical := sb.String()
				r.docFuncs[funcCanonical] = typeFunc
			}

			// Resolve the actual type
			obj := scope.Lookup(docType.Name)
			if obj == nil {
				continue
			}

			r.ResolveType(obj.Type())

			// Parse constants associated with this type
			if r.config.ScanMode.Has(ScanModeConsts) {
				for _, constDecl := range docType.Consts {
					for _, name := range constDecl.Names {
						obj := scope.Lookup(name)
						r.parseValue(obj, constDecl)
					}
				}
			}
		}
	}

	// Process type aliases (they don't appear in docPkg.Types)
	if r.config.ScanMode.Has(ScanModeTypes) {
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			// Check if it's a type name (TypeName objects represent type declarations)
			if typeName, ok := obj.(*types.TypeName); ok {
				// Check if it's a type alias (not already processed via docPkg.Types)
				if _, isAlias := typeName.Type().(*types.Alias); isAlias {
					// Resolve the alias type
					r.ResolveType(typeName.Type())
				}
			}
		}
	}

	// Constants
	if r.config.ScanMode.Has(ScanModeConsts) {
		for _, value := range docPkg.Consts {
			for _, name := range value.Names {
				obj := scope.Lookup(name)
				r.parseValue(obj, value)
			}
		}
	}

	// Variables
	if r.config.ScanMode.Has(ScanModeVariables) {
		for _, value := range docPkg.Vars {
			for _, name := range value.Names {
				obj := scope.Lookup(name)
				r.parseValue(obj, value)
			}
		}
	}

	// Package-level functions
	if r.config.ScanMode.Has(ScanModeFunctions) {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if f, ok := obj.(*types.Func); ok {
				// Skip methods - they have a receiver and are handled by their parent struct/interface
				sig, ok := f.Type().(*types.Signature)
				if !ok || sig.Recv() != nil {
					continue
				}

				var sb strings.Builder
				sb.WriteString(pkg.PkgPath)
				sb.WriteString(".")
				sb.WriteString(f.Name())
				canonical := sb.String()

				// Get doc for this function
				docFunc := r.docFuncs[canonical]

				// makeFunction already caches it, no need to cache again
				fn := r.makeFunction(canonical, sig, nil, f, nil, gstypes.TypeKindFunction)
				if fn != nil {
					// Set documentation from doc.Func if available
					if docFunc != nil {
						// Create a doc.Type wrapper to use SetDoc
						docType := &doc.Type{Doc: docFunc.Doc}
						fn.SetDoc(docType)
					}
					// Set structure to the full signature
					fn.SetStructure(sig.String())
				}
			}
		}
	}

	return nil
}

// isNilType checks if a Type interface contains a nil concrete pointer
func isNilType(t gstypes.Type) bool {
	if t == nil {
		return true
	}
	// Use reflection to check if the interface contains a nil pointer
	v := reflect.ValueOf(t)
	return v.Kind() == reflect.Pointer && v.IsNil()
}

// ResolveType handles Go type objects
func (r *defaultTypeResolver) ResolveType(t types.Type) gstypes.Type {
	if t == nil {
		return nil
	}

	// Quick path: check caches first
	if cached := r.checkCaches(t); cached != nil {
		return cached
	}

	r.logger.Debugf("Resolving Go type: %v", r.GetCanonicalName(t))

	// Handle special cases (aliases to generics, instantiated generics)
	if special := r.handleSpecialCases(t); special != nil {
		return special
	}

	// Unwrap and resolve the underlying type
	return r.resolveUnderlyingType(t)
}

// checkCaches checks for cached types (normalized, predeclared, basic)
func (r *defaultTypeResolver) checkCaches(t types.Type) gstypes.Type {
	// Normalize untyped types
	t = r.normalizeUntyped(t)
	typeName := r.GetCanonicalName(t)

	// Check main type cache first (for all named types)
	if ti, exists := r.types.Get(typeName); exists {
		return ti
	}

	// Handle predeclared types (error, comparable) as basic types
	if typeName == "error" || typeName == "comparable" {
		if basicType, exists := r.basicTypes[typeName]; exists {
			return basicType
		}
	}

	// Check if it's a basic type (only for unnamed basic types)
	if basicType, exists := r.basicTypes[typeName]; exists {
		return basicType
	}

	return nil
}

// handleSpecialCases handles type aliases to generics and instantiated generics
func (r *defaultTypeResolver) handleSpecialCases(t types.Type) gstypes.Type {
	typeName := r.GetCanonicalName(t)

	// Check if it's a type alias for an instantiated generic
	// e.g., type A = List[int]
	// Direct aliases to instantiated generics are detected here.
	// Compound aliases (e.g., type A = *List[int], []List[int], etc.) are handled by
	// makeAlias, which creates an Alias wrapper around the compound type.
	if alias, ok := t.(*types.Alias); ok {
		rhsType := alias.Rhs()
		if named, ok := rhsType.(*types.Named); ok && named.TypeArgs() != nil && named.TypeArgs().Len() > 0 {
			// The alias is for an instantiated generic
			origin := r.ResolveType(named.Origin())
			typeArgs := r.extractTypeArgumentsWithParams(named.Origin(), named.TypeArgs())
			return r.makeInstantiatedGeneric(typeName, origin, typeArgs)
		}
	}

	// Check if it's a named type with type arguments (instantiated generic)
	if named, ok := t.(*types.Named); ok && named.TypeArgs() != nil && named.TypeArgs().Len() > 0 {
		// This is an instantiated generic like List[int]
		origin := r.ResolveType(named.Origin())
		typeArgs := r.extractTypeArgumentsWithParams(named.Origin(), named.TypeArgs())
		return r.makeInstantiatedGeneric(typeName, origin, typeArgs)
	}

	return nil
}

// resolveUnderlyingType unwraps named types and resolves the underlying type
func (r *defaultTypeResolver) resolveUnderlyingType(t types.Type) gstypes.Type {
	typeName := r.GetCanonicalName(t)
	var namedType *types.Named
	var obj types.Object
	var docType *doc.Type

	// Unwrap named types to get the underlying type
	if named, ok := t.(*types.Named); ok {
		namedType = named
		obj = named.Obj()
		docType = r.docTypes[typeName]

		// Set resolving package context for nested type resolution
		// Only set if not already in a resolving context (don't override parent context)
		shouldSetContext := r.resolvingPkg == "" && obj.Pkg() != nil
		if shouldSetContext {
			oldResolvingPkg := r.resolvingPkg
			r.resolvingPkg = obj.Pkg().Path()
			defer func() {
				r.resolvingPkg = oldResolvingPkg
			}()
		}

		// If docType is nil and this is from an external package, try to load it
		if docType == nil && obj.Pkg() != nil {
			pkgPath := obj.Pkg().Path()
			// Only try to load if this is not the current package we're processing
			if r.currentPkg == nil || pkgPath != r.currentPkg.Path() {
				docType = r.loadExternalPackageDoc(pkgPath, obj)
			}
		}

		t = named.Underlying()
	}

	// Handle the underlying type
	var ti gstypes.Type

	switch gt := t.(type) {
	case *types.Basic:
		ti = r.makeBasic(typeName, gt, namedType, obj)

	case *types.Pointer:
		ti = r.makePointer(typeName, gt, namedType, obj, docType)

	case *types.Slice, *types.Array:
		ti = r.makeCollection(typeName, gt, namedType, obj, docType)

	case *types.Signature:
		ti = r.makeFunction(typeName, gt, namedType, obj, docType, gstypes.TypeKindFunction)

	case *types.Chan:
		ti = r.makeChannel(typeName, gt, namedType, obj, docType)

	case *types.Interface:
		ti = r.makeInterface(typeName, gt, namedType, obj, docType)

	case *types.Struct:
		ti = r.makeStruct(typeName, gt, namedType, obj, docType)

	case *types.Alias:
		ti = r.makeAlias(typeName, gt)

	case *types.Map:
		ti = r.makeMap(typeName, gt, namedType, obj, docType)

	case *types.TypeParam:
		ti = r.makeTypeParameter(typeName, gt)

	case *types.Union:
		ti = r.makeUnion(typeName, gt)

	default:
		r.logger.Warnf("Unsupported type: %s (%T)", t.String(), t)
	}

	if ti != nil {
		// Check if the interface contains a nil pointer
		if isNilType(ti) {
			r.logger.Warnf("Type resolution returned typed nil for: %s", typeName)
			return nil
		}
	}

	return ti
}

// cache stores a type in the resolver's cache
func (r *defaultTypeResolver) cache(t gstypes.Type) {
	if t == nil || t.Id() == "" {
		return
	}
	// Cache named types and instantiated generics (even if they report IsNamed() as false)
	if !t.IsNamed() {
		// Allow InstantiatedGeneric to be cached even if it's not technically "named"
		if _, ok := t.(*gstypes.InstantiatedGeneric); !ok {
			return
		}
	}
	r.types.Set(t.Id(), t)
}

// setupCommonTypeFields sets common fields on a type (package, object, doc, goType, files, exported, distance)
func (r *defaultTypeResolver) setupCommonTypeFields(t gstypes.Type, obj types.Object, docType *doc.Type, goType types.Type) {
	pkgInfo := r.getPackageInfo(obj)
	t.SetPackage(pkgInfo)

	// Set distance from packageDistances map
	if pkgInfo != nil {
		if dist, exists := r.packageDistances[pkgInfo.Path()]; exists {
			t.SetDistance(dist)
		} else {
			// If not in map, default to distance 999
			t.SetDistance(999)
		}
	}

	if obj != nil {
		t.SetObject(obj)
		// Set whether the type is exported
		t.SetExported(obj.Exported())
		// Set the file where this type is defined
		if obj.Pos().IsValid() {
			pkg := r.getPackageForObj(obj)
			if pkg != nil {
				pos := pkg.Fset.Position(obj.Pos())
				if pos.Filename != "" {
					// Convert OS path to module-relative path
					modulePath := r.getModuleRelativePath(pos.Filename, obj.Pkg().Path())
					t.SetFiles([]string{modulePath})
				}
			}
		}
	}
	if docType != nil {
		t.SetDoc(docType)
	}
	if goType != nil {
		t.SetGoType(goType)
	}
}

// normalizeUntyped converts untyped constants to their typed equivalents
func (r *defaultTypeResolver) normalizeUntyped(t types.Type) types.Type {
	if basic, ok := t.(*types.Basic); ok {
		switch basic.Kind() {
		case types.UntypedBool:
			return types.Typ[types.Bool]
		case types.UntypedInt:
			return types.Typ[types.Int]
		case types.UntypedRune:
			return types.Typ[types.Rune]
		case types.UntypedFloat:
			return types.Typ[types.Float64]
		case types.UntypedComplex:
			return types.Typ[types.Complex128]
		case types.UntypedString:
			return types.Typ[types.String]
		case types.UntypedNil:
			return types.Typ[types.UnsafePointer]
		}
	}
	return t
}

// makeBasic creates a Basic type
func (r *defaultTypeResolver) makeBasic(
	id string,
	basicType *types.Basic,
	namedType *types.Named,
	obj types.Object,
) *gstypes.Basic {
	// If it's not named, return the cached basic type
	if namedType == nil {
		// Use the basic type name as the key
		basicTypeName := basicType.String()
		if cached, exists := r.basicTypes[basicTypeName]; exists {
			return cached.(*gstypes.Basic)
		}
		// Shouldn't happen, but create if missing
		basic := gstypes.NewBasic(basicTypeName, basicTypeName)
		r.basicTypes[basicTypeName] = basic
		return basic
	}

	// Named basic type (like `type MyInt int`)
	// Create a new Basic type with underlying pointing to cached basic type
	basicTypeName := basicType.String()
	cachedBasic, exists := r.basicTypes[basicTypeName]
	if !exists {
		// Create cached basic if missing
		cachedBasic = gstypes.NewBasic(basicTypeName, basicTypeName)
		r.basicTypes[basicTypeName] = cachedBasic
	}

	// Create the named basic type
	namedBasic := gstypes.NewBasic(id, obj.Name())
	namedBasic.SetUnderlying(cachedBasic)

	// Set common fields including distance
	r.setupCommonTypeFields(namedBasic, obj, nil, namedType)

	// Set the object and doc
	if obj != nil {
		// Store obj via loader to avoid direct exposure
		namedBasic.SetLoader(func(t gstypes.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, t)
				if err != nil {
					return err
				}
				t.AddMethods(m...)

			}
			return nil
		})
	}

	// Cache and return
	r.cache(namedBasic)
	return namedBasic
}

// makePointer creates a Pointer type
func (r *defaultTypeResolver) makePointer(
	id string,
	ptrType *types.Pointer,
	namedType *types.Named,
	obj types.Object,
	docType *doc.Type,
	// forceKind gstypes.TypeKind,
) *gstypes.Pointer {
	// Named types: id=canonical name, name=simple name
	// Unnamed types: id=generated ID, name=generated ID
	var typeID, simpleName string
	if namedType != nil {
		typeID = id
		simpleName = obj.Name()
	} else {
		typeID = r.generateUnnamedID("pointer")
		simpleName = typeID
	}

	// Calculate pointer depth
	elemType, depth := r.deferPtr(ptrType)

	// Resolve the element type (the type being pointed to)
	elem := r.ResolveType(elemType)
	if elem == nil {
		r.logger.Warnf("Failed to resolve pointer element type: %v", elemType)
		return nil
	}

	// Create pointer type with depth
	ptr := gstypes.NewPointer(typeID, simpleName, elem, depth)
	r.setupCommonTypeFields(ptr, obj, docType, ptrType)

	// Set loader for named types
	// Only cache NAMED types
	if namedType != nil {
		r.types.Set(id, ptr)
	}
	return ptr
}

// deferPtr calculates pointer depth by unwrapping nested pointers
// Returns the element type and the pointer depth
func (r *defaultTypeResolver) deferPtr(t types.Type) (types.Type, int) {
	count := 0
	for {
		ptr, ok := t.(*types.Pointer)
		if !ok {
			break
		}
		count++
		t = ptr.Elem()
	}
	return t, count
}

// makeCollection creates a Slice type for both slices and arrays
func (r *defaultTypeResolver) makeCollection(
	id string,
	collType types.Type,
	namedType *types.Named,
	obj types.Object,
	docType *doc.Type,
	// forceKind gstypes.TypeKind,
) *gstypes.Slice {
	var elemType types.Type

	switch ut := collType.(type) {
	case *types.Slice:
		elemType = ut.Elem()
	case *types.Array:
		elemType = ut.Elem()
	default:
		return nil
	}

	// Check if element is a named type first
	var elem gstypes.Type
	if _, ok := elemType.(*types.Named); ok {
		// For named types, resolve directly without unwrapping
		elem = r.ResolveType(elemType)
	} else {
		// For unnamed types, use deferPtr to handle nested pointers
		originalElemType := elemType // Save before unwrapping
		var pointerDepth int
		elemType, pointerDepth = r.deferPtr(elemType)

		// Resolve the underlying element type
		elem = r.ResolveType(elemType)
		if elem == nil {
			r.logger.Warnf("Failed to resolve collection element type: %v", elemType)
			return nil
		}

		// Create element pointer if needed
		if pointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			ptr := gstypes.NewPointer(ptrID, ptrID, elem, pointerDepth)
			ptr.SetGoType(originalElemType) // Use original, not unwrapped
			// Set package based on named type context or current package
			if namedType != nil && obj != nil {
				ptr.SetPackage(r.getPackageInfo(obj))
			} else {
				ptr.SetPackage(r.currentPkg)
			}
			elem = ptr
		}
	}

	if elem == nil {
		r.logger.Warnf("Failed to resolve collection element type: %v", elemType)
		return nil
	}

	// For unnamed element types, set package to parent's package (except basic types)
	if !elem.IsNamed() {
		if _, isBasic := elem.(*gstypes.Basic); !isBasic {
			if namedType != nil && obj != nil {
				elem.SetPackage(r.getPackageInfo(obj))
			}
		}
	}

	// Check if it's an array
	var slice *gstypes.Slice
	// Named types: id=canonical name, name=simple name
	// Unnamed types: id=generated ID, name=generated ID
	var typeID, simpleName string
	if namedType != nil {
		typeID = id
		simpleName = obj.Name()
	} else {
		typeID = r.generateUnnamedID("slice")
		simpleName = typeID
	}
	if arrType, ok := collType.(*types.Array); ok {
		// Create array type with length
		slice = gstypes.NewArray(typeID, simpleName, elem, arrType.Len())
	} else {
		// Create slice type
		slice = gstypes.NewSlice(typeID, simpleName, elem)
	}
	r.setupCommonTypeFields(slice, obj, docType, collType)
	// For unnamed collections, set package based on context
	if namedType == nil {
		slice.SetPackage(r.currentPkg)
	}

	// Set loader for named types
	if namedType != nil && obj != nil {
		slice.SetLoader(func(t gstypes.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, t)
				if err != nil {
					return err
				}
				t.AddMethods(m...)

			}
			return nil
		})
	}

	// Only cache NAMED types
	if namedType != nil {
		r.types.Set(id, slice)
	}
	return slice
}

// makeMap creates a Map type
func (r *defaultTypeResolver) makeMap(
	id string,
	mapType *types.Map,
	namedType *types.Named,
	obj types.Object,
	docType *doc.Type,
	// forceKind gstypes.TypeKind,
) *gstypes.Map {

	// Get key and value types
	keyType := mapType.Key()
	valueType := mapType.Elem()

	// Resolve key type
	var key gstypes.Type
	if _, ok := keyType.(*types.Named); ok {
		// For named types, resolve directly
		key = r.ResolveType(keyType)
	} else {
		// For unnamed types, use deferPtr
		originalKeyType := keyType // Save before unwrapping
		var keyPointerDepth int
		keyType, keyPointerDepth = r.deferPtr(keyType)
		key = r.ResolveType(keyType)
		if key == nil {
			r.logger.Warnf("Failed to resolve map key type: %v", keyType)
			return nil
		}
		if keyPointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			ptr := gstypes.NewPointer(ptrID, ptrID, key, keyPointerDepth)
			ptr.SetGoType(originalKeyType) // Use original, not unwrapped
			// Set package based on named type context or current package
			if namedType != nil && obj != nil {
				ptr.SetPackage(r.getPackageInfo(obj))
			} else {
				ptr.SetPackage(r.currentPkg)
			}
			key = ptr
		}
	}
	if key == nil {
		r.logger.Warnf("Failed to resolve map key type: %v", keyType)
		return nil
	}

	// Resolve value type
	var value gstypes.Type
	if _, ok := valueType.(*types.Named); ok {
		// For named types, resolve directly
		value = r.ResolveType(valueType)
	} else {
		// For unnamed types, use deferPtr
		originalValueType := valueType // Save before unwrapping
		var valuePointerDepth int
		valueType, valuePointerDepth = r.deferPtr(valueType)
		value = r.ResolveType(valueType)
		if value == nil {
			r.logger.Warnf("Failed to resolve map value type: %v", valueType)
			return nil
		}
		if valuePointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			ptr := gstypes.NewPointer(ptrID, ptrID, value, valuePointerDepth)
			ptr.SetGoType(originalValueType) // Use original, not unwrapped
			// Set package based on named type context or current package
			if namedType != nil && obj != nil {
				ptr.SetPackage(r.getPackageInfo(obj))
			} else {
				ptr.SetPackage(r.currentPkg)
			}
			value = ptr
		}
	}
	if value == nil {
		r.logger.Warnf("Failed to resolve map value type: %v", valueType)
		return nil
	}

	// For unnamed key/value types, set package to parent's package (except basic types)
	if !key.IsNamed() {
		if _, isBasic := key.(*gstypes.Basic); !isBasic {
			if namedType != nil && obj != nil {
				key.SetPackage(r.getPackageInfo(obj))
			}
		}
	}
	if !value.IsNamed() {
		if _, isBasic := value.(*gstypes.Basic); !isBasic {
			if namedType != nil && obj != nil {
				value.SetPackage(r.getPackageInfo(obj))
			}
		}
	}

	// Create map type
	// Named types: id=canonical name, name=simple name
	// Unnamed types: id=generated ID, name=generated ID
	var typeID, simpleName string
	if namedType != nil {
		typeID = id
		simpleName = obj.Name()
	} else {
		typeID = r.generateUnnamedID("map")
		simpleName = typeID
	}
	mapT := gstypes.NewMap(typeID, simpleName, key, value)
	r.setupCommonTypeFields(mapT, obj, docType, mapType)
	// For unnamed maps, set package based on context
	if namedType == nil {
		mapT.SetPackage(r.currentPkg)
	}

	// Set loader for named types
	if namedType != nil && obj != nil {
		mapT.SetLoader(func(t gstypes.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, t)
				if err != nil {
					return err
				}
				t.AddMethods(m...)

			}
			return nil
		})
	}

	// Only cache NAMED types
	if namedType != nil {
		r.types.Set(id, mapT)
	}
	return mapT
}

// makeChannel creates a Chan type
func (r *defaultTypeResolver) makeChannel(
	id string,
	chanType *types.Chan,
	namedType *types.Named,
	obj types.Object,
	docType *doc.Type,
	// forceKind gstypes.TypeKind,
) *gstypes.Chan {
	// Get element type
	elemType := chanType.Elem()

	// Determine direction
	var direction gstypes.ChannelDirection
	switch chanType.Dir() {
	case types.SendRecv:
		direction = gstypes.ChanDirBoth
	case types.SendOnly:
		direction = gstypes.ChanDirSend
	case types.RecvOnly:
		direction = gstypes.ChanDirRecv
	}

	// Resolve element type
	var elem gstypes.Type
	if _, ok := elemType.(*types.Named); ok {
		// For named types, resolve directly
		elem = r.ResolveType(elemType)
	} else {
		// For unnamed types, use deferPtr
		originalElemType := elemType // Save before unwrapping
		var pointerDepth int
		elemType, pointerDepth = r.deferPtr(elemType)
		elem = r.ResolveType(elemType)
		if elem == nil {
			r.logger.Warnf("Failed to resolve channel element type: %v", elemType)
			return nil
		}
		if pointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			ptr := gstypes.NewPointer(ptrID, ptrID, elem, pointerDepth)
			ptr.SetGoType(originalElemType) // Use original, not unwrapped
			// Set package based on named type context or current package
			if namedType != nil && obj != nil {
				ptr.SetPackage(r.getPackageInfo(obj))
			} else {
				ptr.SetPackage(r.currentPkg)
			}
			elem = ptr
		}
	}
	if elem == nil {
		r.logger.Warnf("Failed to resolve channel element type: %v", elemType)
		return nil
	}

	// For unnamed element types, set package to parent's package (except basic types)
	if !elem.IsNamed() {
		if _, isBasic := elem.(*gstypes.Basic); !isBasic {
			// Inherit package from parent if parent is named, otherwise use current
			if namedType != nil && obj != nil {
				elem.SetPackage(r.getPackageInfo(obj))
			}
		}
	}

	// Create channel type
	// Named types: id=canonical name, name=simple name
	// Unnamed types: id=generated ID, name=generated ID
	var typeID, simpleName string
	if namedType != nil {
		typeID = id
		simpleName = obj.Name()
	} else {
		typeID = r.generateUnnamedID("chan")
		simpleName = typeID
	}
	ch := gstypes.NewChan(typeID, simpleName, elem, direction)
	r.setupCommonTypeFields(ch, obj, docType, chanType)
	// For unnamed channels, set package based on context
	if namedType == nil {
		ch.SetPackage(r.currentPkg)
	}

	// Set loader for named types
	if namedType != nil && obj != nil {
		ch.SetLoader(func(t gstypes.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, t)
				if err != nil {
					return err
				}
				t.AddMethods(m...)

			}
			return nil
		})
	}

	// Only cache NAMED types
	if namedType != nil {
		r.types.Set(id, ch)
	}
	return ch

}

// extractMethods extracts methods from a named type and adds them to the TypeWithMethods
func (r *defaultTypeResolver) extractMethods(
	namedType *types.Named,
	parent gstypes.Type,
) ([]*gstypes.Method, error) {
	methods := make([]*gstypes.Method, 0, namedType.NumMethods())

	for method := range namedType.Methods() {

		// Check if method should be exported
		if !r.shouldExport(method) {
			r.logger.Debugf("Skipping unexported %s method: %s.%s", parent.Kind(), parent.Id(), method.Name())
			continue
		}

		// Get method signature
		sig, ok := method.Type().(*types.Signature)
		if !ok {
			continue
		}

		// Determine if it's a pointer receiver
		isPointerReceiver := false
		if recv := sig.Recv(); recv != nil {
			_, isPointerReceiver = recv.Type().(*types.Pointer)
		}

		// Create method - ID is struct#methodName
		methodID := parent.Id() + "#" + method.Name()
		m := gstypes.NewMethod(methodID, method.Name(), parent, isPointerReceiver)
		m.SetPackage(r.getPackageInfo(method))
		m.SetDistance(parent.Distance())
		m.SetStructure(sig.String())

		// Process signature
		parameters, results := r.processSignature(sig, parent.Package())
		for _, p := range parameters {
			m.AddParameter(p)
		}
		for _, r := range results {
			m.AddResult(r)
		}

		// Set object and doc
		m.SetObject(method)
		methods = append(methods, m)

	}

	return methods, nil

}

// setUnnamedTypePackages recursively sets the package for all unnamed types in a type tree
func (r *defaultTypeResolver) setUnnamedTypePackages(t gstypes.Type, pkg *gstypes.Package) {
	if t == nil || pkg == nil || t.IsNamed() {
		return
	}

	// Skip basic types - they have no package
	if _, isBasic := t.(*gstypes.Basic); isBasic {
		return
	}

	// Set package on this unnamed type
	t.SetPackage(pkg)

	// Recursively fix nested types based on type kind
	switch typed := t.(type) {
	case *gstypes.Pointer:
		r.setUnnamedTypePackages(typed.Elem(), pkg)
	case *gstypes.Slice:
		r.setUnnamedTypePackages(typed.Elem(), pkg)
	case *gstypes.Chan:
		r.setUnnamedTypePackages(typed.Elem(), pkg)
	case *gstypes.Map:
		r.setUnnamedTypePackages(typed.Key(), pkg)
		r.setUnnamedTypePackages(typed.Value(), pkg)
	case *gstypes.Struct:
		for _, field := range typed.Fields() {
			r.setUnnamedTypePackages(field.Type(), pkg)
		}
	case *gstypes.Interface:
		for _, method := range typed.Methods() {
			for _, param := range method.Parameters() {
				r.setUnnamedTypePackages(param.Type(), pkg)
			}
			for _, result := range method.Results() {
				r.setUnnamedTypePackages(result.Type(), pkg)
			}
		}
	case *gstypes.Function:
		for _, param := range typed.Parameters() {
			r.setUnnamedTypePackages(param.Type(), pkg)
		}
		for _, result := range typed.Results() {
			r.setUnnamedTypePackages(result.Type(), pkg)
		}
	}
}

// processSignature processes a function signature and returns parameters and results
// This is a helper function used by both functions and methods to avoid code duplication
// pkgContext is the package to assign to unnamed types (nil means use currentPkg)
func (r *defaultTypeResolver) processSignature(sig *types.Signature, pkgContext *gstypes.Package) ([]*gstypes.Parameter, []*gstypes.Result) {
	var parameters []*gstypes.Parameter
	var results []*gstypes.Result

	// Process parameters
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		paramVar := params.At(i)
		paramType, pointerDepth := r.deferPtr(paramVar.Type())
		paramTypeResolved := r.ResolveType(paramType)
		if paramTypeResolved == nil {
			continue
		}

		// For unnamed parameter types, set package to context package (except basic types)
		if !paramTypeResolved.IsNamed() {
			// Recursively fix package for this type and all nested unnamed types
			if pkgContext != nil {
				r.setUnnamedTypePackages(paramTypeResolved, pkgContext)
			} else {
				r.setUnnamedTypePackages(paramTypeResolved, r.currentPkg)
			}
		}

		var finalParamType gstypes.Type = paramTypeResolved
		if pointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			finalParamType = gstypes.NewPointer(ptrID, ptrID, paramTypeResolved, pointerDepth)
			finalParamType.SetGoType(types.NewPointer(paramType))
			if pkgContext != nil {
				finalParamType.SetPackage(pkgContext)
			} else {
				finalParamType.SetPackage(r.currentPkg)
			}
		}

		isVariadic := sig.Variadic() && i == params.Len()-1
		param := gstypes.NewParameter(paramVar.Name(), finalParamType, isVariadic)
		parameters = append(parameters, param)
	}

	// Process results
	resultVars := sig.Results()
	for resultVar := range resultVars.Variables() {
		resultVar := resultVar
		resultType, pointerDepth := r.deferPtr(resultVar.Type())
		resultTypeResolved := r.ResolveType(resultType)
		if resultTypeResolved == nil {
			continue
		}

		// For unnamed result types, set package to context package (except basic types)
		if !resultTypeResolved.IsNamed() {
			// Recursively fix package for this type and all nested unnamed types
			if pkgContext != nil {
				r.setUnnamedTypePackages(resultTypeResolved, pkgContext)
			} else {
				r.setUnnamedTypePackages(resultTypeResolved, r.currentPkg)
			}
		}

		var finalResultType gstypes.Type = resultTypeResolved
		if pointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			finalResultType = gstypes.NewPointer(ptrID, ptrID, resultTypeResolved, pointerDepth)
			finalResultType.SetGoType(types.NewPointer(resultType))
			if pkgContext != nil {
				finalResultType.SetPackage(pkgContext)
			} else {
				finalResultType.SetPackage(r.currentPkg)
			}
		}

		result := gstypes.NewResult(resultVar.Name(), finalResultType)
		results = append(results, result)
	}

	return parameters, results
}

// makeFunction creates a Function type
func (r *defaultTypeResolver) makeFunction(
	id string,
	sig *types.Signature,
	namedType *types.Named,
	obj types.Object,
	docType *doc.Type,
	forceKind gstypes.TypeKind,
) *gstypes.Function {
	// Determine ID based on whether this is a named or unnamed function
	var typeID, simpleName string
	if obj != nil {
		// Named function or package-level function: use provided id
		typeID = id
		simpleName = obj.Name()
	} else {
		// Unnamed/anonymous function: generate ID
		typeID = r.generateUnnamedID("function")
		simpleName = typeID
	}

	// Create function type
	fn := gstypes.NewFunction(typeID, simpleName)
	r.setupCommonTypeFields(fn, obj, docType, sig)

	// Extract type parameters if this is a generic function
	if sig.TypeParams() != nil && sig.TypeParams().Len() > 0 {
		typeParams := r.extractTypeParameters(sig.TypeParams(), typeID)
		for _, tp := range typeParams {
			fn.AddTypeParam(tp)
		}
	}

	// Process signature using helper
	parameters, results := r.processSignature(sig, r.currentPkg)
	for _, p := range parameters {
		fn.AddParameter(p)
	}
	for _, r := range results {
		fn.AddResult(r)
	}

	// Set loader for named types
	if namedType != nil && obj != nil {
		fn.SetLoader(func(t gstypes.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, t)
				if err != nil {
					return err
				}
				t.AddMethods(m...)

			}
			return nil
		})
	}

	// Only cache functions, not methods
	// Methods are stored in their parent struct/interface, not in the global types collection
	if forceKind != gstypes.TypeKindMethod {
		r.cache(fn)
	}
	return fn
}

// makeAlias creates an Alias type
func (r *defaultTypeResolver) makeAlias(
	id string,
	aliasType *types.Alias,
	// forceKind types.TypeKind,
) *gstypes.Alias {
	// Get the underlying type
	underlyingType := aliasType.Underlying()

	// Use deferPtr to handle pointers in the underlying type
	underlyingType, pointerDepth := r.deferPtr(underlyingType)

	// Resolve the underlying type
	underlying := r.ResolveType(underlyingType)
	if underlying == nil {
		r.logger.Warnf("Failed to resolve alias underlying type: %v", underlyingType)
		return nil
	}

	// Create pointer wrapper if needed
	var finalUnderlying gstypes.Type = underlying
	if pointerDepth > 0 {
		ptrID := r.generateUnnamedID("pointer")
		finalUnderlying = gstypes.NewPointer(ptrID, ptrID, underlying, pointerDepth)
		finalUnderlying.SetGoType(types.NewPointer(underlyingType))
	}

	// Create alias type
	alias := gstypes.NewAlias(id, id, finalUnderlying)
	// Get package from the alias type's object
	if aliasType.Obj() != nil {
		alias.SetPackage(r.getPackageInfo(aliasType.Obj()))
	} else {
		alias.SetPackage(r.currentPkg)
	}

	// Cache and return
	r.cache(alias)
	return alias
}

// makeInterface creates an Interface type
func (r *defaultTypeResolver) makeInterface(
	id string,
	interfaceType *types.Interface,
	namedType *types.Named,
	obj types.Object,
	docType *doc.Type,
	// forceKind types.TypeKind,
) *gstypes.Interface {
	// Determine ID based on whether this is a named or unnamed interface
	var typeID, simpleName string
	if obj != nil {
		// Named interface: use provided id
		typeID = id
		simpleName = obj.Name()
	} else {
		// Unnamed/anonymous interface: generate ID
		typeID = r.generateUnnamedID("interface")
		simpleName = typeID
	}

	// Create interface type
	iface := gstypes.NewInterface(typeID, simpleName)
	r.setupCommonTypeFields(iface, obj, docType, interfaceType)

	// Extract type parameters if this is a generic interface
	if namedType != nil && namedType.TypeParams() != nil && namedType.TypeParams().Len() > 0 {
		typeParams := r.extractTypeParameters(namedType.TypeParams(), typeID)
		for _, tp := range typeParams {
			iface.AddTypeParam(tp)
		}
	}

	// Register in cache early to prevent infinite recursion on self-referencing types
	r.cache(iface)

	// Get the underlying interface type
	var underlying *types.Interface
	if namedType != nil {
		var ok bool
		underlying, ok = namedType.Underlying().(*types.Interface)
		if !ok {
			// Remove from cache if we couldn't resolve
			r.types.Delete(typeID)
			r.logger.Warnf("Failed to resolve interface underlying type: %v", namedType)
			return nil
		}
	} else {
		// Unnamed interface type
		underlying = interfaceType
	}
	// Set loader to extract methods lazily
	iface.SetLoader(func(t gstypes.Type) error {
		// Extract embedded interfaces
		for i := 0; i < underlying.NumEmbeddeds(); i++ {
			embeddedType := underlying.EmbeddedType(i)
			embeddedResolved := r.ResolveType(embeddedType)
			if embeddedResolved != nil {
				iface.AddEmbed(embeddedResolved)

				// Promote methods from embedded interface using Go types to get instantiated types
				// For instantiated generics, unwrap to get the underlying interface
				underlyingEmbedded := embeddedType
				if namedEmbedded, ok := embeddedType.(*types.Named); ok {
					underlyingEmbedded = namedEmbedded.Underlying()
				}

				if embeddedIfaceType, ok := underlyingEmbedded.(*types.Interface); ok {
					for j := 0; j < embeddedIfaceType.NumMethods(); j++ {
						embeddedMethod := embeddedIfaceType.Method(j)

						// Check if method should be exported
						if !r.shouldExport(embeddedMethod) {
							continue
						}

						sig, ok := embeddedMethod.Type().(*types.Signature)
						if !ok {
							continue
						}

						// Create promoted method
						promotedMethodID := typeID + "#" + embeddedMethod.Name()
						promotedMethod := gstypes.NewMethod(
							promotedMethodID,
							embeddedMethod.Name(),
							iface,
							false,
						)
						promotedMethod.SetPackage(r.getPackageInfo(embeddedMethod))
						promotedMethod.SetDistance(iface.Distance())
						promotedMethod.SetPromotedFrom(embeddedResolved)
						promotedMethod.SetStructure(sig.String())

						// Process signature
						parameters, results := r.processSignature(sig, iface.Package())
						for _, p := range parameters {
							promotedMethod.AddParameter(p)
						}
						for _, res := range results {
							promotedMethod.AddResult(res)
						}

						iface.AddMethods(promotedMethod)
					}
				}
			}
		}

		// Extract explicit methods
		var methods []*gstypes.Method
		for i := 0; i < underlying.NumExplicitMethods(); i++ {
			method := underlying.ExplicitMethod(i)

			// Check if method should be exported
			if !r.shouldExport(method) {
				r.logger.Debugf("Skipping unexported interface method: %s.%s", typeID, method.Name())
				continue
			}

			// Get method signature
			sig, ok := method.Type().(*types.Signature)
			if !ok {
				continue
			}

			// Create method directly - ID is interface#methodName
			methodID := typeID + "#" + method.Name()
			m := gstypes.NewMethod(methodID, method.Name(), iface, false)
			m.SetPackage(r.getPackageInfo(method))
			m.SetDistance(iface.Distance())
			m.SetStructure(sig.String())

			// Process signature using helper
			parameters, results := r.processSignature(sig, iface.Package())
			for _, p := range parameters {
				m.AddParameter(p)
			}
			for _, r := range results {
				m.AddResult(r)
			}
			// Set object and doc
			m.SetObject(method)
			methods = append(methods, m)
		}
		iface.AddMethods(methods...)

		return nil
	})

	return iface
}

// makeStruct creates a Struct type
func (r *defaultTypeResolver) makeStruct(
	id string,
	structType *types.Struct,
	namedType *types.Named,
	obj types.Object,
	docType *doc.Type,
) *gstypes.Struct {
	// Determine ID and name based on whether this is a named or unnamed struct
	var typeID, name string
	if obj != nil {
		// Named struct: use provided id
		typeID = id
		name = obj.Name()
	} else {
		// Unnamed/anonymous struct: generate ID
		typeID = r.generateUnnamedID("struct")
		name = typeID
	}

	// Create struct type
	strct := gstypes.NewStruct(typeID, name)
	r.setupCommonTypeFields(strct, obj, docType, nil)

	// Extract type parameters if this is a generic struct
	if namedType != nil && namedType.TypeParams() != nil && namedType.TypeParams().Len() > 0 {
		typeParams := r.extractTypeParameters(namedType.TypeParams(), typeID)
		for _, tp := range typeParams {
			strct.AddTypeParam(tp)
		}
	}

	// Register in cache early to prevent infinite recursion on self-referencing types
	r.cache(strct)

	// Get the underlying struct type
	var underlying *types.Struct
	if namedType != nil {
		var ok bool
		underlying, ok = namedType.Underlying().(*types.Struct)
		if !ok {
			// Remove from cache if we couldn't resolve
			r.types.Delete(typeID)
			r.logger.Warnf("Failed to resolve struct underlying type: %v", namedType)
			return nil
		}
	} else {
		// Unnamed struct type
		underlying = structType
	}

	// Set loader to extract fields and methods lazily
	strct.SetLoader(func(t gstypes.Type) error {
		// Set resolving package context for nested type resolution
		oldResolvingPkg := r.resolvingPkg
		if strct.Package() != nil {
			r.resolvingPkg = strct.Package().Path()
		}
		defer func() {
			r.resolvingPkg = oldResolvingPkg
		}()

		// Extract fields if needed
		if r.config.ScanMode.Has(ScanModeFields) {
			for i := 0; i < underlying.NumFields(); i++ {
				field := underlying.Field(i)

				// Use deferPtr for field type
				fieldType, pointerDepth := r.deferPtr(field.Type())
				fieldTypeResolved := r.ResolveType(fieldType)
				if fieldTypeResolved == nil {
					continue
				}

				// For unnamed field types, set package to struct's package (except basic types)
				if !fieldTypeResolved.IsNamed() {
					// Skip basic types - they are predeclared and have no package
					if _, isBasic := fieldTypeResolved.(*gstypes.Basic); !isBasic {
						fieldTypeResolved.SetPackage(strct.Package())
					}
				}

				// Create pointer wrapper if needed
				var finalFieldType gstypes.Type = fieldTypeResolved
				if pointerDepth > 0 {
					ptrID := r.generateUnnamedID("pointer")
					finalFieldType = gstypes.NewPointer(ptrID, ptrID, fieldTypeResolved, pointerDepth)
					finalFieldType.SetGoType(types.NewPointer(fieldType))
					finalFieldType.SetPackage(strct.Package())
					finalFieldType.SetDistance(strct.Distance())
				}

				// If this is an embedded field, track it separately and promote fields/methods
				if field.Embedded() {
					// Add to embeds list instead of fields
					strct.AddEmbed(finalFieldType)

					// For embedded types, extract fields/methods from the Go type to get instantiated types
					var embeddedGoType types.Type = fieldType

					// Get the underlying struct type from Go
					var embeddedStructType *types.Struct
					if named, ok := embeddedGoType.(*types.Named); ok {
						if st, ok := named.Underlying().(*types.Struct); ok {
							embeddedStructType = st
						}
					} else if st, ok := embeddedGoType.(*types.Struct); ok {
						embeddedStructType = st
					}

					if embeddedStructType != nil {
						// Promote fields from the embedded struct using the Go type
						for j := 0; j < embeddedStructType.NumFields(); j++ {
							embeddedField := embeddedStructType.Field(j)

							// Skip if this is itself an embedded field
							if embeddedField.Embedded() {
								continue
							}

							// Resolve the field type from Go
							embeddedFieldType, embeddedPointerDepth := r.deferPtr(embeddedField.Type())
							embeddedFieldTypeResolved := r.ResolveType(embeddedFieldType)
							if embeddedFieldTypeResolved == nil {
								continue
							}

							// Create pointer wrapper if needed
							var finalEmbeddedFieldType gstypes.Type = embeddedFieldTypeResolved
							if embeddedPointerDepth > 0 {
								ptrID := r.generateUnnamedID("pointer")
								finalEmbeddedFieldType = gstypes.NewPointer(ptrID, ptrID, embeddedFieldTypeResolved, embeddedPointerDepth)
							}

							promotedFieldID := id + "#" + embeddedField.Name()
							promotedField := gstypes.NewField(promotedFieldID, embeddedField.Name(), finalEmbeddedFieldType, embeddedStructType.Tag(j), false, strct)
							promotedField.SetDistance(strct.Distance())
							promotedField.SetPromotedFrom(finalFieldType)
							strct.AddField(promotedField)
						}

						// Promote methods from the embedded type using Go types
						if namedEmbedded, ok := embeddedGoType.(*types.Named); ok {
							for k := 0; k < namedEmbedded.NumMethods(); k++ {
								embeddedMethod := namedEmbedded.Method(k)

								// Check if method should be exported
								if !r.shouldExport(embeddedMethod) {
									continue
								}

								sig, ok := embeddedMethod.Type().(*types.Signature)
								if !ok {
									continue
								}

								// Create promoted method
								promotedMethodID := id + "#" + embeddedMethod.Name()
								isPointerReceiver := false
								if sig.Recv() != nil {
									_, isPointerReceiver = sig.Recv().Type().(*types.Pointer)
								}
								promotedMethod := gstypes.NewMethod(
									promotedMethodID,
									embeddedMethod.Name(),
									strct,
									isPointerReceiver,
								)
								promotedMethod.SetPackage(r.getPackageInfo(embeddedMethod))
								promotedMethod.SetDistance(strct.Distance())

								// Process signature
								parameters, results := r.processSignature(sig, strct.Package())
								for _, p := range parameters {
									promotedMethod.AddParameter(p)
								}
								for _, res := range results {
									promotedMethod.AddResult(res)
								}

								strct.AddMethods(promotedMethod)
							}
						}
					}
				} else {
					// Regular field (not embedded)
					fieldID := typeID + "#" + field.Name()
					f := gstypes.NewField(fieldID, field.Name(), finalFieldType, underlying.Tag(i), false, strct)
					f.SetPackage(strct.Package())
					f.SetDistance(strct.Distance())
					f.SetObject(field)
					strct.AddField(f)
				}
			}
		}

		// Extract methods if needed
		if r.config.ScanMode.Has(ScanModeMethods) && namedType != nil {
			methods, err := r.extractMethods(namedType, strct)
			if err != nil {
				return err
			}
			// add methods to struct
			strct.AddMethods(methods...)
		}

		return nil
	})

	return strct
}

// makeEnum creates an Enum type from a named type with associated constants
// func (r *defaultTypeResolver) makeEnum(
// 	id string,
// 	basicType *types.Basic,
// 	namedType *types.Named,
// 	obj types.Object,
// 	// docType *doc.Type,
// 	// forceKind types.TypeKind,
// ) *types.Enum {
// 	if namedType == nil {
// 		r.logger.Warn("Cannot create enum from unnamed type")
// 		return nil
// 	}

// 	// Get the underlying basic type
// 	basicTypeName := basicType.String()
// 	cachedBasic, exists := r.basicTypes[basicTypeName]
// 	if !exists {
// 		// Create cached basic if missing
// 		cachedBasic = types.NewBasic(basicTypeName, basicTypeName)
// 		r.basicTypes[basicTypeName] = cachedBasic
// 	}

// 	// Create enum type
// 	enum := types.NewEnum(id, obj.Name(), cachedBasic)
// 	enum.SetPackage(r.getPackageInfo(obj))

// 	// Set loader to extract enum values lazily
// 	enum.SetLoader(func(t types.Type) error {
// 		// Enum values will be added by ProcessPackage when it encounters constants
// 		// associated with this type via docType.Consts
// 		return nil
// 	})

// 	// Cache and return
// 	r.cache(enum)
// 	return enum
// }

// parseValue creates a Value (constant or variable)
func (r *defaultTypeResolver) parseValue(obj types.Object, docValue *doc.Value) gstypes.Type {
	if obj == nil {
		return nil
	}

	// Build canonical name for the value
	var id string
	if obj.Pkg() != nil {
		var sb strings.Builder
		sb.WriteString(obj.Pkg().Path())
		sb.WriteString(".")
		sb.WriteString(obj.Name())
		id = sb.String()
	} else {
		id = obj.Name()
	}

	// Check cache
	if cached, exists := r.types.Get(id); exists {
		return cached
	}

	// Resolve the value's type
	valueType, pointerDepth := r.deferPtr(obj.Type())
	valueTypeResolved := r.ResolveType(valueType)
	if valueTypeResolved == nil {
		return nil
	}

	// Create pointer wrapper if needed
	var finalValueType gstypes.Type = valueTypeResolved
	if pointerDepth > 0 {
		ptrID := r.generateUnnamedID("pointer")
		finalValueType = gstypes.NewPointer(ptrID, ptrID, valueTypeResolved, pointerDepth)
		finalValueType.SetGoType(types.NewPointer(valueType))
		// Unnamed pointer for value uses current package
		finalValueType.SetPackage(r.currentPkg)
	}

	var value *gstypes.Value

	switch v := obj.(type) {
	case *types.Const:
		value = gstypes.NewConstant(id, obj.Name(), finalValueType, v.Val())
	case *types.Var:
		value = gstypes.NewVariable(id, obj.Name(), finalValueType)

	default:
		r.logger.Warnf("Unsupported value type: %T", obj)
		return nil
	}

	if value != nil {
		value.SetPackage(r.getPackageInfo(obj))
		value.SetObject(obj)

		// Set documentation if available
		if docValue != nil && docValue.Doc != "" {
			// Create a doc.Type wrapper to use SetDoc
			docType := &doc.Type{Doc: docValue.Doc}
			value.SetDoc(docType)
		}

		r.values.Set(id, value)

		// Load the value to trigger comment loading
		if err := value.Load(); err != nil {
			r.logger.Warnf("Failed to load value %s: %v", id, err)
		}
	}

	return value
}

// makeTypeParameter creates a TypeParameter type
func (r *defaultTypeResolver) makeTypeParameter(id string, typeParam *types.TypeParam) *gstypes.TypeParameter {
	// Get the constraint type
	constraintType := typeParam.Constraint()

	// For type constraints like `M map[string][]int` or `S struct{ Name string }`,
	// Go wraps them in an unnamed interface. We need to extract the embedded type.
	if iface, ok := constraintType.(*types.Interface); ok && iface.NumEmbeddeds() == 1 && iface.NumExplicitMethods() == 0 {
		// This is an interface with a single embedded type and no methods
		// Extract the embedded type as the actual constraint
		embeddedType := iface.EmbeddedType(0)
		constraintType = embeddedType
	}

	// Resolve the constraint - this will create the full structure
	constraint := r.ResolveType(constraintType)

	// Force load the constraint to ensure its structure is populated
	if constraint != nil {
		if err := constraint.Load(); err != nil {
			r.logger.Warnf("Failed to load constraint for type parameter %s: %v", id, err)
		}
	}

	tp := gstypes.NewTypeParameter(id, typeParam.Obj().Name(), typeParam.Index(), constraint)
	tp.SetPackage(r.getPackageInfo(typeParam.Obj()))
	tp.SetObject(typeParam.Obj())

	// Type parameters are not cached globally, they're part of their parent type
	return tp
}

// makeUnion creates a Union type
func (r *defaultTypeResolver) makeUnion(id string, union *types.Union) *gstypes.Union {
	terms := make([]*gstypes.UnionTerm, union.Len())

	for i := 0; i < union.Len(); i++ {
		term := union.Term(i)
		termType := r.ResolveType(term.Type())
		if termType == nil {
			r.logger.Warnf("Failed to resolve union term type: %v", term.Type())
			continue
		}
		terms[i] = gstypes.NewUnionTerm(termType, term.Tilde())
	}

	u := gstypes.NewUnion(id, id, terms)
	u.SetPackage(r.currentPkg)

	// Unions are not cached globally, they're part of constraints
	return u
}

// makeInstantiatedGeneric creates an InstantiatedGeneric type
func (r *defaultTypeResolver) makeInstantiatedGeneric(id string, origin gstypes.Type, typeArgs []gstypes.TypeArgument) *gstypes.InstantiatedGeneric {
	// Extract simple name from id (last part after .)
	name := id
	if lastDot := strings.LastIndex(id, "."); lastDot >= 0 {
		name = id[lastDot+1:]
	}

	ig := gstypes.NewInstantiatedGeneric(id, name, origin, typeArgs)
	ig.SetPackage(origin.Package())

	// Cache instantiated generics
	r.cache(ig)
	return ig
}

// extractTypeArgumentsWithParams extracts type arguments with parameter names and indices
func (r *defaultTypeResolver) extractTypeArgumentsWithParams(originType *types.Named, typeList *types.TypeList) []gstypes.TypeArgument {
	typeArgs := make([]gstypes.TypeArgument, typeList.Len())

	// Get type parameters from the origin type
	typeParams := originType.TypeParams()

	for i := 0; i < typeList.Len(); i++ {
		paramName := ""
		if typeParams != nil && i < typeParams.Len() {
			paramName = typeParams.At(i).Obj().Name()
		}

		typeArgs[i] = gstypes.TypeArgument{
			Param: paramName,
			Index: i,
			Type:  r.ResolveType(typeList.At(i)),
		}
	}
	return typeArgs
}

// extractTypeArgsFromInstantiation extracts type arguments from a named type that's based on a generic
// For cases like: type MyList GenericList[string]
// where namedType is MyList and originType is GenericList
// extractTypeParameters extracts type parameters from a TypeParamList and adds them to a type
func (r *defaultTypeResolver) extractTypeParameters(typeParamList *types.TypeParamList, parentID string) []*gstypes.TypeParameter {
	if typeParamList == nil || typeParamList.Len() == 0 {
		return nil
	}

	typeParams := make([]*gstypes.TypeParameter, typeParamList.Len())
	for i := 0; i < typeParamList.Len(); i++ {
		typeParam := typeParamList.At(i)
		// Type parameter ID is scoped to parent: ParentType.T
		tpID := parentID + "." + typeParam.Obj().Name()
		typeParams[i] = r.makeTypeParameter(tpID, typeParam)
	}
	return typeParams
}

// shouldExport checks if an object should be exported based on visibility configuration
func (r *defaultTypeResolver) shouldExport(obj types.Object) bool {
	if obj == nil {
		return true
	}

	// Determine if this is from an external package
	isExternal := obj.Pkg() != nil && r.currentPkg != nil && obj.Pkg().Path() != r.currentPkg.Path()

	// Get the appropriate visibility setting
	var visibility VisibilityLevel
	if isExternal && r.config.ExternalPackagesOptions != nil {
		visibility = r.config.ExternalPackagesOptions.Visibility
	} else {
		visibility = r.config.Visibility
	}

	// Check visibility
	if obj.Exported() {
		return visibility.Has(VisibilityLevelExported)
	} else {
		return visibility.Has(VisibilityLevelUnexported)
	}
}

// extractComments extracts comments for all declarations from parsed AST files
// extractCommentsBetweenPackageAndImports extracts comments between package declaration and first import/declaration
func (r *defaultTypeResolver) extractCommentsBetweenPackageAndImports(file *ast.File, pkg *packages.Package) []string {
	var results []string
	pkgEnd := file.Package
	var firstImportPos, firstDeclPos ast.Node

	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == 2 { // token.IMPORT = 2
			firstImportPos = gen
			break
		}
	}

	if len(file.Decls) > 0 {
		firstDeclPos = file.Decls[0]
	}

	// Use the earlier of firstImportPos or firstDeclPos
	var stopPos ast.Node
	if firstImportPos != nil && firstDeclPos != nil {
		if firstImportPos.Pos() < firstDeclPos.Pos() {
			stopPos = firstImportPos
		} else {
			stopPos = firstDeclPos
		}
	} else if firstImportPos != nil {
		stopPos = firstImportPos
	} else if firstDeclPos != nil {
		stopPos = firstDeclPos
	}

	for _, cg := range file.Comments {
		if cg.Pos() > pkgEnd && (stopPos == nil || cg.End() < stopPos.Pos()) {
			// Check if comment is attached to first declaration
			attached := false
			if firstDeclPos != nil && pkg.Fset != nil {
				commentEndLine := pkg.Fset.Position(cg.End()).Line
				declLine := pkg.Fset.Position(firstDeclPos.Pos()).Line
				if declLine == commentEndLine+1 {
					attached = true
				}
			}
			if !attached {
				results = append(results, strings.TrimSpace(cg.Text()))
			}
		}
	}
	return results
}

func (r *defaultTypeResolver) extractComments(pkgInfo *gstypes.Package, pkg *packages.Package) error {
	for i, file := range pkg.Syntax {
		// Determine file path
		var osPath string
		if i < len(pkg.CompiledGoFiles) {
			osPath = pkg.CompiledGoFiles[i]
		}

		// Extract filename from path
		fileName := osPath
		if idx := strings.LastIndex(osPath, "/"); idx >= 0 {
			fileName = osPath[idx+1:]
		}

		// Convert to module-relative path
		modulePath := r.getModuleRelativePath(osPath, pkg.PkgPath)

		// Create File object
		fileInfo := gstypes.NewFile(modulePath, fileName)

		// Extract package-level comments
		if file.Doc != nil {
			pkgLevelComment := strings.TrimSpace(file.Doc.Text())
			if pkgLevelComment != "" {
				pkgInfo.AddComments(gstypes.PackageCommentID, []gstypes.Comment{gstypes.NewComment(pkgLevelComment, gstypes.CommentPlacementPackage)})
				fileInfo.SetComments([]gstypes.Comment{gstypes.NewComment(pkgLevelComment, gstypes.CommentPlacementPackage)})
			}
		}

		// Extract file-level comments (between package and imports)
		fileComments := r.extractCommentsBetweenPackageAndImports(file, pkg)
		if len(fileComments) > 0 {
			fileInfo.AddComments(gstypes.NewComment(strings.Join(fileComments, "\n"), gstypes.CommentPlacementFile))
		}

		// Add file to package
		pkgInfo.AddFile(fileInfo)

		// Extract declarations
		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				// Handle const, var, type declarations
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						// Constants and variables
						comment := r.extractComment(s.Doc, s.Comment, d.Doc)
						for _, name := range s.Names {
							pkgInfo.AddComments(name.Name, comment)
						}
					case *ast.TypeSpec:
						// Type declarations
						comment := r.extractComment(s.Doc, s.Comment, d.Doc)

						pkgInfo.AddComments(s.Name.Name, comment)

						// Extract struct field comments
						if structType, ok := s.Type.(*ast.StructType); ok {
							for _, field := range structType.Fields.List {
								fieldComment := r.extractComment(field.Doc, field.Comment, nil)
								for _, fieldName := range field.Names {
									pkgInfo.AddComments(s.Name.Name+"."+fieldName.Name, fieldComment)
								}
							}
						}

						// Extract interface method comments
						if interfaceType, ok := s.Type.(*ast.InterfaceType); ok {
							for _, method := range interfaceType.Methods.List {
								methodComment := r.extractComment(method.Doc, method.Comment, nil)
								for _, methodName := range method.Names {
									pkgInfo.AddComments(s.Name.Name+"."+methodName.Name, methodComment)
								}
							}
						}
					}
				}
			case *ast.FuncDecl:
				// Functions and methods
				comment := ""
				if d.Doc != nil {
					comment = strings.TrimSpace(d.Doc.Text())
				}
				funcName := d.Name.Name
				if d.Recv != nil && len(d.Recv.List) > 0 {
					// Method: extract receiver type
					recvType := r.getTypeName(d.Recv.List[0].Type)
					var sb strings.Builder
					sb.WriteString(recvType)
					sb.WriteString(".")
					sb.WriteString(funcName)
					funcName = sb.String()
				}
				comment = strings.TrimSpace(comment)
				if comment != "" {
					pkgInfo.AddComments(funcName, []gstypes.Comment{gstypes.NewComment(comment, gstypes.CommentPlacementAbove)})
				}
			}
		}
	}

	return nil
}

// extractComment combines doc comments and inline comments
func (r *defaultTypeResolver) extractComment(doc, comment, parentDoc *ast.CommentGroup) []gstypes.Comment {
	var parts []gstypes.Comment

	// Add doc comment (above the declaration)
	if doc != nil {
		if text := strings.TrimSpace(doc.Text()); text != "" {
			parts = append(parts, gstypes.NewComment(text, gstypes.CommentPlacementAbove))
		}
	} else if parentDoc != nil {
		// Use parent doc if this spec has no doc comment of its own
		if text := strings.TrimSpace(parentDoc.Text()); text != "" {
			parts = append(parts, gstypes.NewComment(text, gstypes.CommentPlacementAbove))
		}
	}

	// Add inline comment (after the declaration)
	if comment != nil {
		if text := strings.TrimSpace(comment.Text()); text != "" {
			parts = append(parts, gstypes.NewComment(text, gstypes.CommentPlacementInline))
		}
	}

	return parts
}

// getTypeName extracts the type name from an expression
func (r *defaultTypeResolver) getTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return r.getTypeName(t.X)
	case *ast.SelectorExpr:
		var sb strings.Builder
		sb.WriteString(r.getTypeName(t.X))
		sb.WriteString(".")
		sb.WriteString(t.Sel.Name)
		return sb.String()
	default:
		return ""
	}
}
