package scannernew

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/types"
	"reflect"
	"strings"

	"github.com/pablor21/goscanner/logger"
	"github.com/pablor21/goscanner/typesnew"
	"golang.org/x/tools/go/packages"
)

// TypeResolver interface for resolving types and managing type information
type TypeResolver interface {
	// ResolveType resolves a types.Type to a typesnew.Type
	ResolveType(t types.Type) typesnew.Type
	// GetCanonicalName returns the canonical name of a type
	GetCanonicalName(t types.Type) string
	// ProcessPackage processes a package to extract and cache type information
	ProcessPackage(pkg *packages.Package) error
	// GetTypes returns all resolved types
	GetTypes() *typesnew.TypesCol[typesnew.Type]
	// GetValues returns all resolved values (constants/variables)
	GetValues() *typesnew.TypesCol[*typesnew.Value]
	// GetPackages returns all loaded packages
	GetPackages() *typesnew.TypesCol[*typesnew.Package]
}

type defaultTypeResolver struct {
	types          *typesnew.TypesCol[typesnew.Type]     // All resolved types
	values         *typesnew.TypesCol[*typesnew.Value]   // All resolved values (constants/variables)
	packages       *typesnew.TypesCol[*typesnew.Package] // All loaded packages
	ignoredTypes   map[string]struct{}                   // Types to ignore
	docTypes       map[string]*doc.Type                  // Documentation for types
	docFuncs       map[string]*doc.Func                  // Documentation for functions
	pkgs           map[string]*packages.Package          // Raw go/packages
	loadedPkgs     map[string]bool                       // Track processed packages
	basicTypes     map[string]typesnew.Type              // Cache of basic types
	currentPkg     *typesnew.Package                     // Currently processing package
	unnamedCounter map[string]int                        // Counter for unnamed types per kind
	unnamedIDs     map[string]string                     // Map structure -> ID for unnamed types
	config         *Config
	logger         logger.Logger
}

// NewDefaultTypeResolver creates a new type resolver
func NewDefaultTypeResolver(config *Config, log logger.Logger) *defaultTypeResolver {
	if log == nil {
		log = logger.NewDefaultLogger()
	}

	tr := &defaultTypeResolver{
		types:          typesnew.NewTypesCol[typesnew.Type](),
		values:         typesnew.NewTypesCol[*typesnew.Value](),
		packages:       typesnew.NewTypesCol[*typesnew.Package](),
		ignoredTypes:   make(map[string]struct{}),
		docTypes:       make(map[string]*doc.Type),
		docFuncs:       make(map[string]*doc.Func),
		pkgs:           make(map[string]*packages.Package),
		loadedPkgs:     make(map[string]bool),
		basicTypes:     make(map[string]typesnew.Type),
		unnamedCounter: make(map[string]int),
		config:         config,
		logger:         log,
	}

	tr.logger.SetTag("TypeResolver")

	// Initialize basic types cache
	tr.initBasicTypes()

	return tr
}

// initBasicTypes creates cached basic type instances
func (r *defaultTypeResolver) initBasicTypes() {
	for _, basicTypeName := range typesnew.BasicTypes {
		basicType := typesnew.NewBasic(basicTypeName, basicTypeName)
		r.basicTypes[basicTypeName] = basicType
	}
}

// generateUnnamedID generates a unique ID for unnamed composite types
func (r *defaultTypeResolver) generateUnnamedID(kind string) string {
	r.unnamedCounter[kind]++
	return fmt.Sprintf("__unnamed_%s__%d__", kind, r.unnamedCounter[kind])
}

func (r *defaultTypeResolver) GetTypes() *typesnew.TypesCol[typesnew.Type] {
	return r.types
}

func (r *defaultTypeResolver) GetValues() *typesnew.TypesCol[*typesnew.Value] {
	return r.values
}

func (r *defaultTypeResolver) GetPackages() *typesnew.TypesCol[*typesnew.Package] {
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

	name := types.TypeString(t, func(pkg *types.Package) string {
		return pkg.Path()
	})

	return name
}

// getPackageInfo returns the package info for the given object
// If obj is nil or has no package, returns currentPkg
func (r *defaultTypeResolver) getPackageInfo(obj types.Object) *typesnew.Package {
	if obj != nil && obj.Pkg() != nil {
		pkgPath := obj.Pkg().Path()
		if pkgInfo, exists := r.packages.Get(pkgPath); exists {
			return pkgInfo
		}
		// If package not in our cache, create it
		pkgInfo := typesnew.NewPackage(pkgPath, obj.Pkg().Name(), nil)
		pkgInfo.SetLogger(r.logger)
		r.packages.Set(pkgPath, pkgInfo)
		return pkgInfo
	}
	return r.currentPkg
}

// loadExternalPackageDoc loads documentation for an external package if not already loaded
func (r *defaultTypeResolver) loadExternalPackageDoc(pkgPath string, obj types.Object) *doc.Type {
	// Don't try to load if we don't have the object
	if obj == nil || obj.Pkg() == nil {
		return nil
	}

	// Build the canonical type name
	typeName := pkgPath + "." + obj.Name()

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
	if pkg.Syntax != nil && len(pkg.Syntax) > 0 {
		docPkg, err := doc.NewFromFiles(
			pkg.Fset,
			pkg.Syntax,
			pkg.PkgPath,
			doc.AllMethods|doc.AllDecls,
		)
		if err != nil {
			r.logger.Debug(fmt.Sprintf("Failed to extract docs from external package %s: %v", pkgPath, err))
			return nil
		}

		// Cache the doc types from this package
		for _, docType := range docPkg.Types {
			canonical := pkgPath + "." + docType.Name
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
	pkgInfo := typesnew.NewPackage(pkg.PkgPath, pkg.Name, pkg)
	pkgInfo.SetLogger(r.logger)
	r.packages.Set(pkg.PkgPath, pkgInfo)
	r.currentPkg = pkgInfo

	// Extract comments from AST
	if err := r.extractComments(pkgInfo, pkg); err != nil {
		r.logger.Warn(fmt.Sprintf("Failed to extract comments: %v", err))
	}

	// Extract documentation
	docPkg, err := doc.NewFromFiles(
		pkg.Fset,
		pkg.Syntax,
		pkg.PkgPath,
		doc.AllMethods|doc.AllDecls,
	)
	if err != nil {
		return err
	}

	r.pkgs[pkg.PkgPath] = pkg
	r.loadedPkgs[pkg.PkgPath] = true

	// Package-level functions documentation
	for _, docFunc := range docPkg.Funcs {
		canonical := pkg.PkgPath + "." + docFunc.Name
		r.docFuncs[canonical] = docFunc
	}

	// Constants
	if r.config.ScanMode.Has(ScanModeConsts) {
		for _, value := range docPkg.Consts {
			for _, name := range value.Names {
				obj := pkg.Types.Scope().Lookup(name)
				r.parseValue(obj)
			}
		}
	}

	// Variables
	if r.config.ScanMode.Has(ScanModeVariables) {
		for _, value := range docPkg.Vars {
			for _, name := range value.Names {
				obj := pkg.Types.Scope().Lookup(name)
				r.parseValue(obj)
			}
		}
	}

	// Types + associated functions
	if r.config.ScanMode.Has(ScanModeTypes) {
		for _, docType := range docPkg.Types {
			typeCanonical := pkg.PkgPath + "." + docType.Name
			r.docTypes[typeCanonical] = docType

			// Factory functions associated with the type
			for _, typeFunc := range docType.Funcs {
				funcCanonical := pkg.PkgPath + "." + typeFunc.Name
				r.docFuncs[funcCanonical] = typeFunc
			}

			// Resolve the actual type
			obj := pkg.Types.Scope().Lookup(docType.Name)
			if obj == nil {
				continue
			}

			// If the type has constants, treat it as an enum
			if r.config.ScanMode.Has(ScanModeEnums) && len(docType.Consts) > 0 {
				r.resolveGoType(obj.Type(), typesnew.TypeKindEnum)
			} else {
				r.ResolveType(obj.Type())
			}

			// Parse enum constants
			if r.config.ScanMode.Has(ScanModeEnums) {
				for _, constDecl := range docType.Consts {
					for _, name := range constDecl.Names {
						obj := pkg.Types.Scope().Lookup(name)
						r.parseValue(obj)
					}
				}
			}
		}
	}

	// Package-level functions
	if r.config.ScanMode.Has(ScanModeFunctions) {
		for _, name := range pkg.Types.Scope().Names() {
			obj := pkg.Types.Scope().Lookup(name)
			if f, ok := obj.(*types.Func); ok {
				// Skip methods - they have a receiver and are handled by their parent struct/interface
				sig, ok := f.Type().(*types.Signature)
				if !ok || sig.Recv() != nil {
					continue
				}

				canonical := pkg.PkgPath + "." + f.Name()
				// makeFunction already caches it, no need to cache again
				fn := r.makeFunction(canonical, sig, nil, f, nil, typesnew.TypeKindFunction)
				if fn != nil {
					// Set structure to the full signature
					fn.SetStructure(sig.String())
				}
			}
		}
	}

	return nil
}

// isNilType checks if a Type interface contains a nil concrete pointer
func isNilType(t typesnew.Type) bool {
	if t == nil {
		return true
	}
	// Use reflection to check if the interface contains a nil pointer
	v := reflect.ValueOf(t)
	return v.Kind() == reflect.Ptr && v.IsNil()
}

// ResolveType resolves a Go type to typesnew.Type
func (r *defaultTypeResolver) ResolveType(t types.Type) typesnew.Type {
	if t == nil {
		return nil
	}

	return r.resolveGoType(t, "")
}

// resolveGoType handles Go type objects with optional kind forcing
func (r *defaultTypeResolver) resolveGoType(t types.Type, forceKind typesnew.TypeKind) typesnew.Type {
	if t == nil {
		return nil
	}

	// Normalize untyped types
	t = r.normalizeUntyped(t)
	typeName := r.GetCanonicalName(t)

	// Handle predeclared types (error, comparable) as basic types
	if typeName == "error" || typeName == "comparable" {
		if basicType, exists := r.basicTypes[typeName]; exists {
			return basicType
		}
	}

	// Check if it's a basic type
	if basicType, exists := r.basicTypes[typeName]; exists {
		return basicType
	}

	// Check cache
	if ti, exists := r.types.Get(typeName); exists {
		return ti
	}

	r.logger.Debug(fmt.Sprintf("Resolving Go type: %v", typeName))

	// Unwrap named types to get the underlying type
	var namedType *types.Named
	var obj types.Object
	var docType *doc.Type

	// check if it's a named type first
	if named, ok := t.(*types.Named); ok {
		namedType = named
		obj = named.Obj()
		docType = r.docTypes[typeName]

		// If docType is nil and this is from an external package, try to load it
		if docType == nil && obj != nil && obj.Pkg() != nil {
			pkgPath := obj.Pkg().Path()
			// Only try to load if this is not the current package we're processing
			if r.currentPkg == nil || pkgPath != r.currentPkg.Path() {
				docType = r.loadExternalPackageDoc(pkgPath, obj)
			}
		}

		t = named.Underlying()
	}

	// Handle the underlying type
	var ti typesnew.Type

	switch gt := t.(type) {
	case *types.Basic:
		ti = r.makeBasic(typeName, gt, namedType, obj, docType, forceKind)

	case *types.Pointer:
		ti = r.makePointer(typeName, gt, namedType, obj, docType, forceKind)

	case *types.Slice, *types.Array:
		ti = r.makeCollection(typeName, gt, namedType, obj, docType, forceKind)

	case *types.Signature:
		ti = r.makeFunction(typeName, gt, namedType, obj, docType, forceKind)

	case *types.Chan:
		ti = r.makeChannel(typeName, gt, namedType, obj, docType, forceKind)

	case *types.Interface:
		ti = r.makeInterface(typeName, gt, namedType, obj, docType, forceKind)

	case *types.Struct:
		ti = r.makeStruct(typeName, gt, namedType, obj, docType)

	case *types.Alias:
		ti = r.makeAlias(typeName, gt, forceKind)

	case *types.Map:
		ti = r.makeMap(typeName, gt, namedType, obj, docType, forceKind)

	default:
		r.logger.Warn(fmt.Sprintf("Unsupported type: %s (%T)", t.String(), t))
	}

	if ti != nil {
		// Check if the interface contains a nil pointer
		if isNilType(ti) {
			r.logger.Warn(fmt.Sprintf("Type resolution returned typed nil for: %s", typeName))
			return nil
		}
	}

	return ti
}

// cache stores a type in the resolver's cache
func (r *defaultTypeResolver) cache(t typesnew.Type) {
	if t == nil {
		return
	}
	r.types.Set(t.Id(), t)
}

// setupCommonTypeFields sets common fields on a type (package, object, doc, goType)
func (r *defaultTypeResolver) setupCommonTypeFields(t typesnew.Type, obj types.Object, docType *doc.Type, goType types.Type) {
	t.SetPackage(r.getPackageInfo(obj))
	if obj != nil {
		t.SetObject(obj)
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
	docType *doc.Type,
	forceKind typesnew.TypeKind,
) *typesnew.Basic {
	// If it's not named, return the cached basic type
	if namedType == nil {
		// Use the basic type name as the key
		basicTypeName := basicType.String()
		if cached, exists := r.basicTypes[basicTypeName]; exists {
			return cached.(*typesnew.Basic)
		}
		// Shouldn't happen, but create if missing
		basic := typesnew.NewBasic(basicTypeName, basicTypeName)
		r.basicTypes[basicTypeName] = basic
		return basic
	}

	// Named basic type (like `type MyInt int`)
	// Create a new Basic type with underlying pointing to cached basic type
	basicTypeName := basicType.String()
	cachedBasic, exists := r.basicTypes[basicTypeName]
	if !exists {
		// Create cached basic if missing
		cachedBasic = typesnew.NewBasic(basicTypeName, basicTypeName)
		r.basicTypes[basicTypeName] = cachedBasic
	}

	// Create the named basic type
	namedBasic := typesnew.NewBasic(id, obj.Name())
	namedBasic.SetObject(obj)
	namedBasic.SetUnderlying(cachedBasic)
	namedBasic.SetPackage(r.getPackageInfo(obj))

	// Set the object and doc
	if obj != nil {
		// Store obj via loader to avoid direct exposure
		namedBasic.SetLoader(func(t typesnew.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, obj, docType, t)
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
	forceKind typesnew.TypeKind,
) *typesnew.Pointer {
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
		r.logger.Warn(fmt.Sprintf("Failed to resolve pointer element type: %v", elemType))
		return nil
	}

	// Create pointer type with depth
	ptr := typesnew.NewPointer(typeID, simpleName, elem, depth)
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
	forceKind typesnew.TypeKind,
) *typesnew.Slice {
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
	var elem typesnew.Type
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
			r.logger.Warn(fmt.Sprintf("Failed to resolve collection element type: %v", elemType))
			return nil
		}

		// Create element pointer if needed
		if pointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			ptr := typesnew.NewPointer(ptrID, ptrID, elem, pointerDepth)
			ptr.SetGoType(originalElemType) // Use original, not unwrapped
			elem = ptr
		}
	}

	if elem == nil {
		r.logger.Warn(fmt.Sprintf("Failed to resolve collection element type: %v", elemType))
		return nil
	}

	// Check if it's an array
	var slice *typesnew.Slice
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
		slice = typesnew.NewArray(typeID, simpleName, elem, arrType.Len())
	} else {
		// Create slice type
		slice = typesnew.NewSlice(typeID, simpleName, elem)
	}
	r.setupCommonTypeFields(slice, obj, docType, collType)

	// Set loader for named types
	if namedType != nil && obj != nil {
		slice.SetLoader(func(t typesnew.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, obj, docType, t)
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
	forceKind typesnew.TypeKind,
) *typesnew.Map {

	// Get key and value types
	keyType := mapType.Key()
	valueType := mapType.Elem()

	// Resolve key type
	var key typesnew.Type
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
			r.logger.Warn(fmt.Sprintf("Failed to resolve map key type: %v", keyType))
			return nil
		}
		if keyPointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			ptr := typesnew.NewPointer(ptrID, ptrID, key, keyPointerDepth)
			ptr.SetGoType(originalKeyType) // Use original, not unwrapped
			key = ptr
		}
	}
	if key == nil {
		r.logger.Warn(fmt.Sprintf("Failed to resolve map key type: %v", keyType))
		return nil
	}

	// Resolve value type
	var value typesnew.Type
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
			r.logger.Warn(fmt.Sprintf("Failed to resolve map value type: %v", valueType))
			return nil
		}
		if valuePointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			ptr := typesnew.NewPointer(ptrID, ptrID, value, valuePointerDepth)
			ptr.SetGoType(originalValueType) // Use original, not unwrapped
			value = ptr
		}
	}
	if value == nil {
		r.logger.Warn(fmt.Sprintf("Failed to resolve map value type: %v", valueType))
		return nil
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
	mapT := typesnew.NewMap(typeID, simpleName, key, value)
	r.setupCommonTypeFields(mapT, obj, docType, mapType)

	// Set loader for named types
	if namedType != nil && obj != nil {
		mapT.SetLoader(func(t typesnew.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, obj, docType, t)
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
	forceKind typesnew.TypeKind,
) *typesnew.Chan {
	// Get element type
	elemType := chanType.Elem()

	// Determine direction
	var direction typesnew.ChannelDirection
	switch chanType.Dir() {
	case types.SendRecv:
		direction = typesnew.ChanDirBoth
	case types.SendOnly:
		direction = typesnew.ChanDirSend
	case types.RecvOnly:
		direction = typesnew.ChanDirRecv
	}

	// Resolve element type
	var elem typesnew.Type
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
			r.logger.Warn(fmt.Sprintf("Failed to resolve channel element type: %v", elemType))
			return nil
		}
		if pointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			ptr := typesnew.NewPointer(ptrID, ptrID, elem, pointerDepth)
			ptr.SetGoType(originalElemType) // Use original, not unwrapped
			elem = ptr
		}
	}
	if elem == nil {
		r.logger.Warn(fmt.Sprintf("Failed to resolve channel element type: %v", elemType))
		return nil
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
	ch := typesnew.NewChan(typeID, simpleName, elem, direction)
	r.setupCommonTypeFields(ch, obj, docType, chanType)

	// Set loader for named types
	if namedType != nil && obj != nil {
		ch.SetLoader(func(t typesnew.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, obj, docType, t)
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
	obj types.Object,
	docType *doc.Type,
	parent typesnew.Type,
) ([]*typesnew.Method, error) {
	methods := []*typesnew.Method{}

	for method := range namedType.Methods() {

		// Check if method should be exported
		if !r.shouldExport(method) {
			r.logger.Debug(fmt.Sprintf("Skipping unexported %s method: %s.%s", parent.Kind(), parent.Id(), method.Name()))
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
		m := typesnew.NewMethod(methodID, method.Name(), parent, isPointerReceiver)
		m.SetPackage(r.getPackageInfo(method))
		m.SetStructure(sig.String())

		// Process signature
		parameters, results := r.processSignature(sig)
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

// processSignature processes a function signature and returns parameters and results
// This is a helper function used by both functions and methods to avoid code duplication
func (r *defaultTypeResolver) processSignature(sig *types.Signature) ([]*typesnew.Parameter, []*typesnew.Result) {
	var parameters []*typesnew.Parameter
	var results []*typesnew.Result

	// Process parameters
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		paramVar := params.At(i)
		paramType, pointerDepth := r.deferPtr(paramVar.Type())
		paramTypeResolved := r.ResolveType(paramType)
		if paramTypeResolved == nil {
			continue
		}

		var finalParamType typesnew.Type = paramTypeResolved
		if pointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			finalParamType = typesnew.NewPointer(ptrID, ptrID, paramTypeResolved, pointerDepth)
			finalParamType.SetGoType(types.NewPointer(paramType))
		}

		isVariadic := sig.Variadic() && i == params.Len()-1
		param := typesnew.NewParameter(paramVar.Name(), finalParamType, isVariadic)
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

		var finalResultType typesnew.Type = resultTypeResolved
		if pointerDepth > 0 {
			ptrID := r.generateUnnamedID("pointer")
			finalResultType = typesnew.NewPointer(ptrID, ptrID, resultTypeResolved, pointerDepth)
			finalResultType.SetGoType(types.NewPointer(resultType))
		}

		result := typesnew.NewResult(resultVar.Name(), finalResultType)
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
	forceKind typesnew.TypeKind,
) *typesnew.Function {
	// Create function type
	fn := typesnew.NewFunction(id, id)
	r.setupCommonTypeFields(fn, obj, docType, sig)

	// Process signature using helper
	parameters, results := r.processSignature(sig)
	for _, p := range parameters {
		fn.AddParameter(p)
	}
	for _, r := range results {
		fn.AddResult(r)
	}

	// Set loader for named types
	if namedType != nil && obj != nil {
		fn.SetLoader(func(t typesnew.Type) error {
			// Load methods if needed
			if r.config.ScanMode.Has(ScanModeMethods) {
				m, err := r.extractMethods(namedType, obj, docType, t)
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
	if forceKind != typesnew.TypeKindMethod {
		r.cache(fn)
	}
	return fn
}

// makeAlias creates an Alias type
func (r *defaultTypeResolver) makeAlias(
	id string,
	aliasType *types.Alias,
	forceKind typesnew.TypeKind,
) *typesnew.Alias {
	// Get the underlying type
	underlyingType := aliasType.Underlying()

	// Use deferPtr to handle pointers in the underlying type
	underlyingType, pointerDepth := r.deferPtr(underlyingType)

	// Resolve the underlying type
	underlying := r.ResolveType(underlyingType)
	if underlying == nil {
		r.logger.Warn(fmt.Sprintf("Failed to resolve alias underlying type: %v", underlyingType))
		return nil
	}

	// Create pointer wrapper if needed
	var finalUnderlying typesnew.Type = underlying
	if pointerDepth > 0 {
		ptrID := r.generateUnnamedID("pointer")
		finalUnderlying = typesnew.NewPointer(ptrID, ptrID, underlying, pointerDepth)
		finalUnderlying.SetGoType(types.NewPointer(underlyingType))
	}

	// Create alias type
	alias := typesnew.NewAlias(id, id, finalUnderlying)
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
	forceKind typesnew.TypeKind,
) *typesnew.Interface {
	// Create interface type
	iface := typesnew.NewInterface(id, id)
	r.setupCommonTypeFields(iface, obj, docType, interfaceType)

	// Register in cache early to prevent infinite recursion on self-referencing types
	r.cache(iface)

	// Get the underlying interface type
	var underlying *types.Interface
	if namedType != nil {
		var ok bool
		underlying, ok = namedType.Underlying().(*types.Interface)
		if !ok {
			// Remove from cache if we couldn't resolve
			r.types.Delete(id)
			r.logger.Warn(fmt.Sprintf("Failed to resolve interface underlying type: %v", namedType))
			return nil
		}
	} else {
		// Unnamed interface type
		underlying = interfaceType
	}
	// Set loader to extract methods lazily
	iface.SetLoader(func(t typesnew.Type) error {

		// Extract methods if needed
		if r.config.ScanMode.Has(ScanModeMethods) {
			methods := []*typesnew.Method{}
			for method := range underlying.Methods() {

				// Check if method should be exported
				if !r.shouldExport(method) {
					r.logger.Debug(fmt.Sprintf("Skipping unexported interface method: %s.%s", id, method.Name()))
					continue
				}

				// Get method signature
				sig, ok := method.Type().(*types.Signature)
				if !ok {
					continue
				}

				// Create method directly - ID is interface#methodName
				methodID := id + "#" + method.Name()
				m := typesnew.NewMethod(methodID, method.Name(), iface, false)
				m.SetPackage(r.getPackageInfo(method))
				m.SetStructure(sig.String())

				// Process signature using helper
				parameters, results := r.processSignature(sig)
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
		}

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
) *typesnew.Struct {
	// Determine name based on whether this is a named or unnamed struct
	var name string
	if obj != nil {
		name = obj.Name()
	} else {
		name = id
	}

	// Create struct type
	strct := typesnew.NewStruct(id, name)
	r.setupCommonTypeFields(strct, obj, docType, nil)

	// Register in cache early to prevent infinite recursion on self-referencing types
	r.cache(strct)

	// Get the underlying struct type
	var underlying *types.Struct
	if namedType != nil {
		var ok bool
		underlying, ok = namedType.Underlying().(*types.Struct)
		if !ok {
			// Remove from cache if we couldn't resolve
			r.types.Delete(id)
			r.logger.Warn(fmt.Sprintf("Failed to resolve struct underlying type: %v", namedType))
			return nil
		}
	} else {
		// Unnamed struct type
		underlying = structType
	}

	// Set loader to extract fields and methods lazily
	strct.SetLoader(func(t typesnew.Type) error {
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

				// Create pointer wrapper if needed
				var finalFieldType typesnew.Type = fieldTypeResolved
				if pointerDepth > 0 {
					ptrID := r.generateUnnamedID("pointer")
					finalFieldType = typesnew.NewPointer(ptrID, ptrID, fieldTypeResolved, pointerDepth)
					finalFieldType.SetGoType(types.NewPointer(fieldType))
				}

				// Handle embedded fields
				if field.Embedded() {
					// Load the embedded type to get its fields
					if err := finalFieldType.Load(); err != nil {
						r.logger.Warn(fmt.Sprintf("Failed to load embedded type %s: %v", finalFieldType.Id(), err))
					}

					// Get the embedded struct's fields if it's a struct
					if embeddedStruct, ok := fieldTypeResolved.(*typesnew.Struct); ok {
						// Add each field from the embedded struct as a promoted field
						for _, embeddedField := range embeddedStruct.Fields() {
							fieldID := id + "#" + embeddedField.Name()
							promotedField := typesnew.NewField(fieldID, embeddedField.Name(), embeddedField.Type(), embeddedField.Tag(), false, strct)
							promotedField.SetPromotedFrom(finalFieldType)
							strct.AddField(promotedField)
						}
					}
					// Don't add the embedded field itself, only its promoted fields
					continue
				}

				// Regular field (not embedded)
				fieldID := id + "#" + field.Name()
				f := typesnew.NewField(fieldID, field.Name(), finalFieldType, underlying.Tag(i), false, strct)

				strct.AddField(f)
			}
		}

		// Extract methods if needed
		if r.config.ScanMode.Has(ScanModeMethods) && namedType != nil {
			methods, err := r.extractMethods(namedType, obj, docType, strct)
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
func (r *defaultTypeResolver) makeEnum(
	id string,
	basicType *types.Basic,
	namedType *types.Named,
	obj types.Object,
	docType *doc.Type,
	forceKind typesnew.TypeKind,
) *typesnew.Enum {
	if namedType == nil {
		r.logger.Warn("Cannot create enum from unnamed type")
		return nil
	}

	// Get the underlying basic type
	basicTypeName := basicType.String()
	cachedBasic, exists := r.basicTypes[basicTypeName]
	if !exists {
		// Create cached basic if missing
		cachedBasic = typesnew.NewBasic(basicTypeName, basicTypeName)
		r.basicTypes[basicTypeName] = cachedBasic
	}

	// Create enum type
	enum := typesnew.NewEnum(id, obj.Name(), cachedBasic)
	enum.SetPackage(r.getPackageInfo(obj))

	// Set loader to extract enum values lazily
	enum.SetLoader(func(t typesnew.Type) error {
		// Enum values will be added by ProcessPackage when it encounters constants
		// associated with this type via docType.Consts
		return nil
	})

	// Cache and return
	r.cache(enum)
	return enum
}

// parseValue creates a Value (constant or variable)
func (r *defaultTypeResolver) parseValue(obj types.Object) typesnew.Type {
	if obj == nil {
		return nil
	}

	// Get canonical name
	id := r.GetCanonicalName(obj.Type())
	if obj.Pkg() != nil {
		id = obj.Pkg().Path() + "." + obj.Name()
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
	var finalValueType typesnew.Type = valueTypeResolved
	if pointerDepth > 0 {
		ptrID := r.generateUnnamedID("pointer")
		finalValueType = typesnew.NewPointer(ptrID, ptrID, valueTypeResolved, pointerDepth)
		finalValueType.SetGoType(types.NewPointer(valueType))
	}

	var value typesnew.Type

	switch v := obj.(type) {
	case *types.Const:
		// Get the constant value
		constVal := v.Val()
		var goValue any
		if constVal != nil {
			goValue = constVal.String()
			// Try to parse as specific type based on the underlying type
			// This is a simplified version; in practice, you might want more sophisticated parsing
		}

		value = typesnew.NewConstant(id, obj.Name(), finalValueType, goValue)

	case *types.Var:
		value = typesnew.NewVariable(id, obj.Name(), finalValueType)

	default:
		r.logger.Warn(fmt.Sprintf("Unsupported value type: %T", obj))
		return nil
	}

	if value != nil {
		value.SetPackage(r.getPackageInfo(obj))
		r.cache(value)
	}

	return value
}

// handleGenerics is a placeholder for future generics handling
// This will be complex and requires special treatment for:
// - Type parameters
// - Type constraints
// - Instantiated generic types
// - Generic function signatures
func (r *defaultTypeResolver) handleGenerics(t types.Type) typesnew.Type {
	// TODO: Implement generics handling
	// For now, we'll just log that we encountered a generic type
	r.logger.Debug(fmt.Sprintf("Generics not yet fully supported: %v", t))
	return nil
}

// shouldExport checks if an object should be exported based on its name
func (r *defaultTypeResolver) shouldExport(obj types.Object) bool {
	if obj == nil {
		return false
	}
	return obj.Exported()
}

// extractComments extracts comments for all declarations from parsed AST files
func (r *defaultTypeResolver) extractComments(pkgInfo *typesnew.Package, pkg *packages.Package) error {
	for _, file := range pkg.Syntax {
		// Extract package-level comments
		if file.Doc != nil {
			pkgLevelComment := strings.TrimSpace(file.Doc.Text())
			if pkgLevelComment != "" {
				pkgInfo.AddComments("#PACKAGE_DOC", []typesnew.Comment{typesnew.NewComment(pkgLevelComment, typesnew.CommentPlacementPackage)})
			}
		}

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
						r.logger.Debug(fmt.Sprintf("Type %s: doc=%v, comment=%v, parent=%v, extracted=%d comments",
							s.Name.Name,
							s.Doc != nil,
							s.Comment != nil,
							d.Doc != nil,
							len(comment)))
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
					funcName = recvType + "." + funcName
				}
				comment = strings.TrimSpace(comment)
				if comment != "" {
					pkgInfo.AddComments(funcName, []typesnew.Comment{typesnew.NewComment(comment, typesnew.CommentPlacementAbove)})
				}
			}
		}
	}

	return nil
}

// extractComment combines doc comments and inline comments
func (r *defaultTypeResolver) extractComment(doc, comment, parentDoc *ast.CommentGroup) []typesnew.Comment {
	var parts []typesnew.Comment

	// Add doc comment (above the declaration)
	if doc != nil {
		if text := strings.TrimSpace(doc.Text()); text != "" {
			parts = append(parts, typesnew.NewComment(text, typesnew.CommentPlacementAbove))
		}
	} else if parentDoc != nil {
		// Use parent doc if this spec has no doc comment of its own
		if text := strings.TrimSpace(parentDoc.Text()); text != "" {
			parts = append(parts, typesnew.NewComment(text, typesnew.CommentPlacementAbove))
		}
	}

	// Add inline comment (after the declaration)
	if comment != nil {
		if text := strings.TrimSpace(comment.Text()); text != "" {
			parts = append(parts, typesnew.NewComment(text, typesnew.CommentPlacementInline))
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
		return r.getTypeName(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}
