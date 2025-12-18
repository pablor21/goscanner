package scanner

import (
	"fmt"
	"go/ast"
	"go/doc"
	"go/types"
	"slices"
	"strings"

	"github.com/pablor21/goscanner/logger"
	gct "github.com/pablor21/goscanner/types"
	"golang.org/x/tools/go/packages"
)

// BasicTypes is a list of Go basic types (as per go/types.BasicKind)
var BasicTypes = []string{
	"bool",
	"byte",
	"complex64",
	"complex128",
	"error",
	"float32",
	"float64",
	"int",
	"int8",
	"int16",
	"int32",
	"int64",
	"rune",
	"string",
	"uint",
	"uint8",
	"uint16",
	"uint32",
	"uint64",
	"uintptr",
	"interface{}",
	"slice",
	"any",
	"comparable",
	"error",
}

// type TypeCollection map[string]gct.Type
// type ValueCollection map[string]gct.ValueType
// type PackageCollection map[string]*gct.Package

// TypeResolver is an interface for resolving types and managing type information
type TypeResolver interface {
	// ResolveType resolves a types.Type to a TypeInfo
	ResolveType(t types.Type) gct.Type
	// GetCannonicalName returns the canonical name of a type
	GetCannonicalName(t types.Type) string
	// ProcessPackage processes a package to extract and cache type information (scans the entire package)
	ProcessPackage(pkg *packages.Package) error
	// GetTypeInfos returns all resolved TypeInfo objects
	GetTypeInfos() TypeCollection
	// GetValueInfos returns all resolved ValueType objects
	GetValueInfos() ValueCollection
	// GetPackageInfos returns all loaded package information
	GetPackageInfos() PackageCollection
}

type defaultTypeResolver struct {
	types        TypeCollection
	values       ValueCollection // All resolved ValueType objects
	packages     PackageCollection
	ignoredTypes map[string]struct{}
	docTypes     map[string]*doc.Type         // All discovered doc types
	docFuncs     map[string]*doc.Func         // Add this field
	pkgs         map[string]*packages.Package // All loaded packages
	loadedPkgs   map[string]bool              // Track what's been processed
	basicTypes   TypeCollection               // Cache of basic types to avoid duplication
	currentPkg   *gct.Package                 // Currently processing package
	confg        *Config
	logger       logger.Logger

	anonymousCounter int

	scannedPackages map[string]bool // packages being scanned (depth 0)
	typeDepths      map[string]int  // memoize calculated depths
	currentDepth    int             // current processing depth
}

func newDefaultTypeResolver(confg *Config, log logger.Logger) *defaultTypeResolver {
	tr := &defaultTypeResolver{
		types:            TypeCollection{},
		values:           ValueCollection{},
		packages:         PackageCollection{},
		ignoredTypes:     make(map[string]struct{}),
		docTypes:         make(map[string]*doc.Type),
		docFuncs:         make(map[string]*doc.Func),
		pkgs:             make(map[string]*packages.Package),
		basicTypes:       TypeCollection{},
		loadedPkgs:       make(map[string]bool),
		scannedPackages:  make(map[string]bool),
		typeDepths:       make(map[string]int),
		confg:            confg,
		logger:           log,
		anonymousCounter: 0,
	}
	if tr.logger == nil {
		tr.logger = logger.NewDefaultLogger()
	}

	tr.logger.SetTag("TypeResolver")

	// spin up basic types cache
	for _, basicTypeName := range BasicTypes {
		basicType := gct.NewBasicTypeInfo(basicTypeName, basicTypeName, gct.TypeKindBasic)
		tr.basicTypes[basicTypeName] = basicType
	}

	return tr
}

func (r *defaultTypeResolver) GetTypeInfos() TypeCollection {
	return r.types
}

func (r *defaultTypeResolver) GetValueInfos() ValueCollection {
	return r.values
}

func (r *defaultTypeResolver) GetPackageInfos() PackageCollection {
	return r.packages
}

func (r *defaultTypeResolver) GetCannonicalName(t types.Type) string {
	if t == nil {
		return ""
	}

	// For named types with type parameters, return just the base name
	// if namedType, ok := t.(*types.Named); ok && namedType.TypeParams() != nil && namedType.TypeParams().Len() > 0 {
	// 	obj := namedType.Obj()
	// 	if obj.Pkg() != nil {
	// 		return obj.Pkg().Path() + "." + obj.Name()
	// 	}
	// 	return obj.Name()
	// }

	name := types.TypeString(t, func(pkg *types.Package) string {
		return pkg.Path()
	})

	return name
}

// GetBaseName returns the clean name without type parameters for generics
func (r *defaultTypeResolver) GetBaseName(t types.Type) string {
	if t == nil {
		return ""
	}

	switch namedType := t.(type) {
	case *types.Named:
		obj := namedType.Obj()
		if obj.Pkg() != nil {
			return obj.Pkg().Path() + "." + obj.Name()
		}
		return obj.Name()
	default:
		// For non-named types, use canonical name
		return r.GetCannonicalName(t)
	}
}

func (r *defaultTypeResolver) ProcessPackage(pkg *packages.Package) error {
	// Set the current package being processed

	// create the pkg object
	pkgInfo := gct.NewPackage(pkg)
	r.packages[pkg.PkgPath] = pkgInfo
	r.extractAst(pkgInfo)
	r.currentPkg = pkgInfo
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
	for _, value := range docPkg.Consts {
		for _, name := range value.Names {
			obj := pkg.Types.Scope().Lookup(name)
			r.ParseValue(obj)
		}
	}

	// Variables
	for _, value := range docPkg.Vars {
		for _, name := range value.Names {
			obj := pkg.Types.Scope().Lookup(name)
			r.ParseValue(obj)
		}
	}

	// Types + factory functions
	for _, docType := range docPkg.Types {
		typeCanonical := pkg.PkgPath + "." + docType.Name
		r.docTypes[typeCanonical] = docType

		// Factory functions associated by go/doc
		for _, typeFunc := range docType.Funcs {
			funcCanonical := pkg.PkgPath + "." + typeFunc.Name
			r.docFuncs[funcCanonical] = typeFunc
		}

		// Resolve the actual type via go/types
		obj := pkg.Types.Scope().Lookup(docType.Name)
		if obj == nil {
			continue
		}

		// If the type has associated constants, treat it as an enum
		if len(docType.Consts) > 0 {
			r.resolveGoType(obj.Type(), gct.TypeKindEnum)
		} else {
			r.ResolveType(obj.Type())
		}

		// parse consts (for enum-like types)
		for _, constDecl := range docType.Consts {
			for _, name := range constDecl.Names {
				obj := pkg.Types.Scope().Lookup(name)
				r.ParseValue(obj)
			}
		}
	}

	// Package-level functions (go/types)
	for _, name := range pkg.Types.Scope().Names() {
		obj := pkg.Types.Scope().Lookup(name)

		f, ok := obj.(*types.Func)
		if !ok {
			continue
		}

		canonical := pkg.PkgPath + "." + f.Name()
		res := r.makeFunctionInfo(canonical, f)
		if res != nil {
			r.cache(res)
		}
	}

	return nil
}

// func (r *defaultTypeResolver) ProcessPackage(pkg *packages.Package) error {
// 	// Set the current package being processed
// 	r.currentPkg = pkg

// 	// 1. Extract doc.Type information
// 	docPkg, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath, doc.AllMethods|doc.AllDecls)
// 	if err != nil {
// 		return err
// 	}

// 	r.packages[pkg.PkgPath] = pkg
// 	r.loadedPkgs[pkg.PkgPath] = true

// 	// Store function documentation
// 	for _, docFunc := range docPkg.Funcs {
// 		canonicalName := pkg.PkgPath + "." + docFunc.Name
// 		r.docFuncs[canonicalName] = docFunc
// 	}

// 	// Process types
// 	for _, docType := range docPkg.Types {
// 		canonicalName := pkg.PkgPath + "." + docType.Name

// 		// After extracting docPkg.Funcs, also check type-associated functions
// 		// (this is important for "factory functions" that return types)
// 		//
// 		// Go's doc package associates functions that return exactly one custom type
// 		// with that type rather than treating them as package-level functions.
// 		// See: src/go/doc/reader.go readFunc() - when numResultTypes == 1,
// 		// the function is added to typ.funcs and early returns, skipping r.funcs.
// 		// This causes functions like "func F() CustomType" to not appear in docPkg.Funcs.
// 		for _, typeFunc := range docType.Funcs {
// 			r.docFuncs[canonicalName] = typeFunc
// 		}

// 		r.docTypes[canonicalName] = docType
// 		// resolve type to cache it
// 		t := pkg.Types.Scope().Lookup(docType.Name)
// 		if t == nil {
// 			continue
// 		}

// 		if _, ok := t.(*types.Const); ok {
// 			continue
// 		}

// 		r.ResolveType(t.Type())
// 	}

// 	// Process package-level functions
// 	for _, name := range pkg.Types.Scope().Names() {
// 		obj := pkg.Types.Scope().Lookup(name)
// 		if f, ok := obj.(*types.Func); ok {
// 			canonicalName := pkg.PkgPath + "." + f.Name()
// 			// Process standalone functions
// 			res := r.makeFunctionInfo(canonicalName, f)
// 			if res != nil {
// 				r.cache(res)
// 			}
// 		}
// 	}

// 	return nil

// }

// parseConst creates a ValueType for a constant object
func (r *defaultTypeResolver) ParseValue(obj types.Object) *gct.Value {
	if obj == nil {
		return nil
	}
	c, ok := obj.(*types.Const)
	if !ok {
		return nil
	}

	// first resolve the underlying type
	t := r.resolveGoType(c.Type(), gct.TypeKindEnum)

	if t == nil {
		return nil
	}

	// id := r.GetCannonicalName(c.Type())

	// Create the canonical ID for the constant
	canonical := r.currentPkg.Path + "." + c.Name()

	// check if the type is a NamedType (enum)
	if named, ok := t.(*gct.EnumInfo); ok {
		val := gct.NewConstValue(canonical, c, r.currentPkg, named.TypeRef.TypeRef())
		// Set package info for AST comment access
		val.SetPackageInfo(r.currentPkg)
		named.AddValues(val)
		// r.values[canonical] = val
		return val
	} else {
		val := gct.NewConstValue(canonical, c, r.currentPkg, t)
		// Set package info for AST comment access
		val.SetPackageInfo(r.currentPkg)
		r.values[canonical] = val
		return val
	}

	// canonical := r.currentPkg.Path + "." + c.Name()

	// r.values[canonical] = val

}

// ResolveType resolves a Go type to TypeInfo
func (r *defaultTypeResolver) ResolveType(t types.Type) gct.Type {
	if t == nil {
		return nil
	}

	return r.resolveGoType(t, "")
}

// resolveGoType handles Go type objects
func (r *defaultTypeResolver) resolveGoType(t types.Type, forceKind gct.TypeKind) gct.Type {
	// check for nil
	if t == nil {
		return nil
	}

	// if ptr, ok := t.(*types.Pointer); ok {
	// 	r.logger.Error(fmt.Sprintf("invalid type %v, pointers must be deferenced before processing", ptr))
	// 	return nil
	// }

	// normalize untyped types
	t = r.normalizeUntyped(t)
	typeName := r.GetCannonicalName(t)

	// Handle special predeclared types (error, comparable) as basic types
	if typeName == "error" || typeName == "comparable" {
		if basicType, exists := r.basicTypes[typeName]; exists {
			return basicType
		}
	}

	// check if its a basic type
	if basicType, exists := r.basicTypes[typeName]; exists {
		return basicType
	}

	var ti gct.Type
	// Check cache first
	if ti, exists := r.types[typeName]; exists {
		return ti
	}

	// Determine package
	// var pkgPath string
	// if named, ok := t.(*types.Named); ok {
	// 	pkgPath = named.Obj().Pkg().Path()
	// }

	r.logger.Debug(fmt.Sprintf("Resolving Go type: %v, %v", t, typeName))

	// First, unwrap named types to get the underlying type
	var namedType *types.Named
	var obj types.Object
	var docType *doc.Type

	if named, ok := t.(*types.Named); ok {
		namedType = named
		obj = named.Obj()
		docType = r.docTypes[typeName]
		t = named.Underlying()
	}

	// Now handle the underlying type (works for both named and unnamed)
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
		ti = r.makeStruct(typeName, gt, namedType, obj, docType, forceKind)

	case *types.Alias:
		ti = r.makeAlias(typeName, gt, forceKind)

	case *types.Map:
		ti = r.makeMap(typeName, gt, namedType, obj, docType, forceKind)

	default:
		r.logger.Warn(fmt.Sprintf("Unsupported type encountered: %s (%T)", t.String(), t))
	}

	// Note: We don't call Load() here to avoid circular dependency deadlocks.
	// Types will load lazily when their details are actually accessed.
	if ti != nil {
		// Calculate and set depth using the dedicated function
		typeDepth := r.calculateTypeDepth(ti, nil)
		ti.SetDepth(typeDepth)

	}

	return ti
}

// COMMENTED: UNUSED
// createBasicType creates a BasicTypeInfo from a types.Basic
// func (r *defaultTypeResolver) createBasicType(basicType *types.Basic) gct.BasicTypeInfo {
// 	return r.createBasicTypeFromName(basicType.String())
// }

// COMMENTED: UNUSED
// createBasicTypeFromName creates a BasicTypeInfo from a type name
// func (r *defaultTypeResolver) createBasicTypeFromName(typeName string) gct.BasicTypeInfo {
// 	return gct.NewBasicTypeInfo(typeName, gct.TypeKindBasic)
// }

func (r *defaultTypeResolver) makeNamedBasicInfo(id string, basicType *types.Basic, obj types.Object) *gct.NamedTypeInfo {
	if basicType == nil {
		return nil
	}
	underlying := r.ResolveType(basicType)
	if underlying != nil {

		// loader
		loader := func(ti gct.Type) error {
			if namedType, ok := ti.(*gct.NamedTypeInfo); ok {
				namedType.Methods = r.extractMethodInfos(ti)
			}
			return nil
		}

		tRef := gct.NewTypeRef(underlying.Id(), 0, underlying)
		res := r.makeNamedTypeInfo(id, underlying.Kind(), obj, r.docTypes[id], loader)
		res.TypeRef = tRef
		res.SetPackageInfo(r.currentPkg)
		return res
	}

	return nil
}

func (r *defaultTypeResolver) makeNamedTypeInfo(id string, kind gct.TypeKind, obj types.Object, docType *doc.Type, detailsLoader gct.DetailsLoaderFn) *gct.NamedTypeInfo {
	res := gct.NewNamedTypeInfo(id, kind, obj, docType, r.currentPkg, detailsLoader)
	// Set package info for AST comment access
	res.SetPackageInfo(r.currentPkg)
	return res

}

func (r *defaultTypeResolver) makeEnumTypeInfo(id string, basicType *types.Basic, obj types.Object) *gct.EnumInfo {
	if basicType == nil {
		return nil
	}
	underlying := r.ResolveType(basicType)
	if underlying != nil {
		// loader
		loader := func(ti gct.Type) error {
			if namedType, ok := ti.(*gct.NamedTypeInfo); ok {
				namedType.Methods = r.extractMethodInfos(ti)
			}
			return nil
		}
		res := gct.NewEnum(id, underlying, obj, r.docTypes[id], r.currentPkg, loader)
		if res != nil {
			// Set package info for AST comment access
			res.SetPackageInfo(r.currentPkg)
		}
		return res
	}
	return nil
}

// makeFunctionInfo creates a FunctionTypeInfo for function types
func (r *defaultTypeResolver) makeFunctionInfo(id string, obj types.Object) *gct.FunctionTypeInfo {
	// check if we should export this function
	if !r.shouldExportType(obj, gct.TypeKindFunction) {
		r.logger.Debug(fmt.Sprintf("Skipping unexported function: %s", id))
		return nil
	}

	docFunc := r.docFuncs[id]

	// create the function type entry
	functionType := gct.NewFunctionInfo(id, obj, docFunc, r.currentPkg, nil)

	// Extract parameters and returns if we have a function object
	if obj != nil {
		var sig *types.Signature
		var ok bool

		// Handle both direct signatures and named function types
		if sig, ok = obj.Type().(*types.Signature); !ok {
			// For named function types, get the underlying signature
			if namedType, isNamed := obj.Type().(*types.Named); isNamed {
				if underlying, isUnderlying := namedType.Underlying().(*types.Signature); isUnderlying {
					sig = underlying
					ok = true
				}
			}
		}

		if ok && sig != nil {
			// check if variadic
			if sig.Variadic() {
				functionType.IsVariadic = true
			}

			// parameters
			params := sig.Params()
			for i := 0; i < params.Len(); i++ {
				paramVar := params.At(i)
				paramType, pointers := r.deferPtr(paramVar.Type())
				ref := r.ResolveType(paramType)
				if ref == nil {
					continue
				}
				paramTypeRef := gct.NewTypeRef(ref.Id(), pointers, ref)

				paramInfo := gct.NewParameterInfo(paramVar, paramTypeRef, functionType)

				// check if it's variadic parameter (last param of a variadic function)
				if functionType.IsVariadic && i == params.Len()-1 {
					paramInfo.IsVariadic = true
				}

				functionType.Parameters = append(functionType.Parameters, paramInfo)
			}

			// returns
			for resultVar := range sig.Results().Variables() {
				resultType, pointers := r.deferPtr(resultVar.Type())
				ref := r.ResolveType(resultType)
				if ref == nil {
					continue
				}
				resRef := gct.NewTypeRef(ref.Id(), pointers, ref)

				retInfo := gct.NewReturnInfo(resultVar, resRef, functionType)

				functionType.Returns = append(functionType.Returns, retInfo)

			}
		}
	}

	return functionType
}

func (r *defaultTypeResolver) makeNamedFunction(id string, namedType *types.Named, obj types.Object) *gct.NamedFunctionInfo {
	if namedType == nil || obj == nil {
		return nil
	}

	// check if we should export this function
	if !r.shouldExportType(obj, gct.TypeKindFunction) {
		r.logger.Debug(fmt.Sprintf("Skipping unexported function: %s", id))
		return nil
	}

	// the loader lazy loads the function details when needed
	loader := func(ti gct.Type) error {
		if namedFunc, ok := ti.(*gct.NamedFunctionInfo); ok {
			namedFunc.Methods = r.extractMethodInfos(ti)
		}
		return nil
	}
	fn := r.makeFunctionInfo(id, obj)
	if fn == nil {
		return nil
	}
	res := gct.NewNamedFunctionInfo(id, obj, fn, r.docTypes[id], r.currentPkg, nil, loader)
	return res

}

// extractMethodInfos extracts method information for a given type
func (r *defaultTypeResolver) extractMethodInfos(parent gct.Type) []*gct.MethodInfo {
	if parent == nil {
		return nil
	}

	namedParent, ok := parent.(gct.NamedType)
	if !ok {
		return nil
	}

	var methods []*gct.MethodInfo

	// Method set for pointer type (*T) - includes methods with both T and *T receivers
	pointerType := types.NewPointer(parent.Object().Type())
	pointerMethodSet := types.NewMethodSet(pointerType)
	r.logger.Debug(fmt.Sprintf("Type %s has %d methods", parent.Id(), pointerMethodSet.Len()))
	for method := range pointerMethodSet.Methods() {
		methodObj := method.Obj()

		methodInfo := r.makeMethodInfo(methodObj, namedParent)
		if methodInfo != nil {
			methods = append(methods, methodInfo)
		}
	}

	// Sort methods by name for consistency
	slices.SortFunc(methods, func(a, b *gct.MethodInfo) int {
		return strings.Compare(a.Name(), b.Name())
	})

	return methods
}

func (r *defaultTypeResolver) makeMethodInfo(methodObj types.Object, parent gct.NamedType) *gct.MethodInfo {
	if methodObj == nil {
		return nil
	}
	// check if its pointer receiver
	isPointerReceiver := false
	isVariadic := false

	methodInfo := gct.NewMethodInfo(methodObj, parent)

	// check if we should export this method
	if !r.shouldExportType(methodObj, gct.TypeKindMethod) {
		r.logger.Debug(fmt.Sprintf("Skipping unexported method: %s.%s", parent.Id(), methodObj.Name()))
		return nil
	}

	var promotedFrom gct.Type = nil
	// extract signature details
	if sig, ok := methodObj.Type().(*types.Signature); ok {
		if recv := sig.Recv(); recv != nil {
			_, isPointerReceiver = recv.Type().(*types.Pointer)

			// Get receiver type (strip pointer if present)
			recvType := recv.Type()
			if ptr, ok := recvType.(*types.Pointer); ok {
				recvType = ptr.Elem()
			}

			// Compare receiver type with parent type
			parentType := parent.Object().Type()
			if !types.Identical(recvType, parentType) {
				// Method is promoted! Receiver type is the source
				promotedFrom = r.ResolveType(recvType)
				methodInfo.SetPromotedFrom(promotedFrom)
			}
		}
		// check if variadic
		if sig.Variadic() {
			isVariadic = true
		}

		// parameters
		params := sig.Params()
		for i := 0; i < params.Len(); i++ {
			paramVar := params.At(i)
			// paramTypeRef := NewTypeRefFromTypes(paramVar.Type(), parent.Package())

			paramType, pointers := r.deferPtr(paramVar.Type())
			ref := r.ResolveType(paramType)
			if ref == nil {
				continue
			}
			paramTypeRef := gct.NewTypeRef(ref.Id(), pointers, ref)

			paramInfo := gct.NewParameterInfo(paramVar, paramTypeRef, methodInfo.FunctionTypeInfo)

			// check if it's variadic parameter (last param of a variadic method)
			if isVariadic && i == params.Len()-1 {
				paramInfo.IsVariadic = true
			}

			methodInfo.Parameters = append(methodInfo.Parameters, paramInfo)
		}

		//results
		for resultVar := range sig.Results().Variables() {

			resultType, pointers := r.deferPtr(resultVar.Type())
			ref := r.ResolveType(resultType)
			if ref == nil {
				continue
			}
			resRef := gct.NewTypeRef(ref.Id(), pointers, ref)
			methodInfo.Returns = append(methodInfo.Returns, gct.NewReturnInfo(resultVar, resRef, methodInfo.FunctionTypeInfo))
		}
	}

	methodInfo.IsPointerReceiver = isPointerReceiver
	methodInfo.IsVariadic = isVariadic

	// Set package info to load comments
	methodInfo.SetPackageInfo(r.currentPkg)

	return methodInfo
}

func (r *defaultTypeResolver) makeNamedStruct(id string, namedType *types.Named, obj types.Object) *gct.StructTypeInfo {
	if namedType == nil {
		return nil
	}

	// Create a placeholder struct type first to break circular dependencies
	structType := gct.NewStructTypeInfo(
		id,
		obj,
		r.docTypes[id],
		r.currentPkg,
		nil, // Will be set below
	)

	// Register in cache early to prevent infinite recursion on self-referencing types
	r.types[id] = structType

	// resolve the underlying struct type (this may refer back to the named type itself)
	underlying, ok := namedType.Underlying().(*types.Struct)
	if !ok {
		// remove from cache if we couldn't resolve
		delete(r.types, id)
		return nil
	}

	loader := func(ti gct.Type) error {
		for i := 0; i < underlying.NumFields(); i++ {
			field := underlying.Field(i)
			fieldType, pointers := r.deferPtr(field.Type())
			ref := r.ResolveType(fieldType)
			if ref == nil {
				continue
			}

			fieldTypeRef := gct.NewTypeRef(ref.Id(), pointers, ref)

			// If the field is embedded, we'll add its fields as promoted
			if field.Embedded() {
				// Load the embedded type to get its fields
				if loadable, ok := ref.(gct.Loadable); ok {
					loadable.Load()
				}

				// Get the embedded struct's fields if it's a struct
				if embeddedStructType, ok := ref.(*gct.StructTypeInfo); ok {
					// Add each field from the embedded struct as a promoted field
					for _, embeddedField := range embeddedStructType.GetFields() {
						promotedField := gct.NewFieldInfo(
							ti.Id()+"#"+embeddedField.Name(),
							ti,
							embeddedField.Object(),
							nil, // no separate doc for promoted fields
							r.currentPkg,
							embeddedField.TypeRef,
							embeddedField.GetTag(),
							ref, // promoted from this embedded type
							nil,
						)
						promotedField.SetPackageInfo(r.currentPkg)
						structType.Fields = append(structType.Fields, promotedField)
					}
				}
				// Don't add the embedded field itself, only its promoted fields
				continue
			}

			// Regular field (not embedded)
			fieldInfo := gct.NewFieldInfo(
				ti.Id()+"#"+field.Name(),
				ti,
				field,
				r.docTypes[field.Id()],
				r.currentPkg,
				fieldTypeRef,
				underlying.Tag(i),
				nil, // not promoted
				nil,
			)

			fieldInfo.SetPackageInfo(r.currentPkg)
			structType.Fields = append(structType.Fields, fieldInfo)
		}

		structType.Methods = r.extractMethodInfos(ti)
		return nil
	}

	// Set the loader on the struct type
	structType.SetDetailsLoader(loader)

	// Set package info to load comments
	structType.SetPackageInfo(r.currentPkg)

	return structType
}

func (r *defaultTypeResolver) makeNamedInterface(id string, namedType types.Type, obj types.Object) *gct.InterfaceTypeInfo {
	{
		if namedType == nil {
			return nil
		}

		// Create a placeholder interface type first to break circular dependencies
		interfaceType := gct.NewInterfaceTypeInfo(
			id,
			obj,
			r.docTypes[id],
			r.currentPkg,
			nil, // Will be set below
		)

		// Register in cache early to prevent infinite recursion on self-referencing types
		r.types[id] = interfaceType

		var underlying *types.Interface

		if nt, ok := namedType.(*types.Named); !ok {
			// resolve the underlying interface type (this may refer back to the named type itself)
			underlying, ok = nt.Underlying().(*types.Interface)
			if !ok {
				// remove from cache if we couldn't resolve
				delete(r.types, id)
				return nil
			}
		} else {
			// unnamed interface type
			underlying, ok = namedType.(*types.Interface)
			if !ok {
				// remove from cache if we couldn't resolve
				delete(r.types, id)
				return nil
			}
		}

		loader := func(ti gct.Type) error {
			// methods
			for i := 0; i < underlying.NumMethods(); i++ {
				method := underlying.Method(i)
				methodInfo := r.makeMethodInfo(method, ti.(gct.NamedType))
				if methodInfo != nil {
					interfaceType.Methods = append(interfaceType.Methods, methodInfo)
				}
			}

			return nil
		}

		// Set the loader on the interface type
		interfaceType.SetDetailsLoader(loader)

		// Set package info to load comments
		interfaceType.SetPackageInfo(r.currentPkg)

		return interfaceType
	}
}

/**
 * ALIAS TYPES
**/
func (r *defaultTypeResolver) makeBasicAlias(id string, aliasType *types.Alias, obj types.Object) *gct.BasicAliasTypeInfo {
	if aliasType == nil {
		return nil
	}

	// resolve the underlying type
	underlyingType, pointerCount := r.deferPtr(aliasType.Underlying())
	ref := r.ResolveType(underlyingType)
	if ref == nil {
		return nil
	}

	// create the typeref
	typeRef := gct.NewTypeRef(ref.Id(), pointerCount, ref)

	// create the alias type entry
	aliasTypeInfo := gct.NewBasicAliasTypeInfo(id, typeRef, obj, r.docTypes[id], r.currentPkg)

	// Set package info for AST comment access
	aliasTypeInfo.SetPackageInfo(r.currentPkg)

	return aliasTypeInfo
}

/**
 * CHANNEL TYPES
**/

func (r *defaultTypeResolver) makeBasicChannel(_ string, t types.Type) *gct.BasicChannelTypeInfo {
	if t == nil {
		return nil
	}

	var elemType types.Type
	var dir gct.ChannelDirection

	switch ut := t.Underlying().(type) {
	case *types.Chan:
		elemType = ut.Elem()
		switch ut.Dir() {
		case types.SendRecv:
			dir = gct.ChanDirBoth
		case types.SendOnly:
			dir = gct.ChanDirSend
		case types.RecvOnly:
			dir = gct.ChanDirRecv
		}
	default:
		return nil // not a channel type
	}

	// get the element type reference
	elemType, pointerCount := r.deferPtr(elemType)
	ref := r.ResolveType(elemType)
	if ref == nil {
		return nil
	}

	// create the typeref
	typeRef := gct.NewTypeRef(ref.Id(), pointerCount, ref)

	// create the channel type entry
	channelType := gct.NewBasicChannelTypeInfo(
		"chan",
		typeRef,
		dir,
	)

	return channelType
}

// makeNamedChannel creates a NamedChannelTypeInfo for channel named types
func (r *defaultTypeResolver) makeNamedChannel(id string, t types.Type, obj types.Object) *gct.NamedChannelTypeInfo {
	var elemType types.Type

	var dir gct.ChannelDirection

	switch ut := t.Underlying().(type) {
	case *types.Chan:
		elemType = ut.Elem()
		switch ut.Dir() {
		case types.SendRecv:
			dir = gct.ChanDirBoth
		case types.SendOnly:
			dir = gct.ChanDirSend
		case types.RecvOnly:
			dir = gct.ChanDirRecv
		}
	default:
		return nil // not a channel type
	}

	// Create a placeholder channel type first to break circular dependencies
	channelType := gct.NewNamedChannelTypeInfo(
		id,
		obj,
		r.docTypes[id],
		r.currentPkg,
		nil, // Will be set below
		dir,
		nil, // Will be set below
	)

	// Register in cache early to prevent infinite recursion on self-referencing types
	r.types[id] = channelType

	// Resolve the element type (this may refer back to the named type itself)
	elemType, pointerCount := r.deferPtr(elemType)
	ref := r.ResolveType(elemType)
	if ref == nil {
		// remove from cache if we couldn't resolve
		delete(r.types, id)
		return nil
	}

	// create the typeref
	typeRef := gct.NewTypeRef(ref.Id(), pointerCount, ref)

	// Set the typeref on the channel type
	channelType.ElementReference = typeRef

	loader := func(ti gct.Type) error {
		channelType.Methods = r.extractMethodInfos(ti)
		return nil
	}

	// Set the loader on the channel type
	channelType.SetDetailsLoader(loader)

	// Set package info to load comments
	channelType.SetPackageInfo(r.currentPkg)

	return channelType
}

/**
 * COLLECTON TYPES
**/

// makeBasicCollection creates a BasicCollectionTypeInfo for slice or array types
func (r *defaultTypeResolver) makeBasicCollection(_ string, t types.Type) *gct.BasicCollectionTypeInfo {
	if t == nil {
		return nil
	}

	var size int64
	kind := gct.TypeKindSlice
	var elemType types.Type

	switch ut := t.Underlying().(type) {
	case *types.Slice:
		elemType = ut.Elem()
	case *types.Array:
		elemType = ut.Elem()
	default:
		return nil // not a collection type
	}

	// get the element type reference
	elemType, pointerCount := r.deferPtr(elemType)
	ref := r.ResolveType(elemType)
	if ref == nil {
		return nil
	}

	// create the typeref
	typeRef := gct.NewTypeRef(ref.Id(), pointerCount, ref)

	// Determine simple type name based on kind
	simpleName := "slice"
	if kind == gct.TypeKindArray {
		simpleName = "array"
	}

	// create the collection type entry
	collectionType := gct.NewBasicCollectionTypeInfo(simpleName, kind, typeRef, size)

	return collectionType
}

// makeNamedCollection creates a NamedCollectionTypeInfo for slice or array named types
func (r *defaultTypeResolver) makeNamedCollection(id string, namedType *types.Named, obj types.Object) *gct.NamedCollectionTypeInfo {

	var size int64
	kind := gct.TypeKindSlice
	var elemType types.Type

	switch ut := namedType.Underlying().(type) {
	case *types.Slice:
		elemType = ut.Elem()
	case *types.Array:
		elemType = ut.Elem()
	default:
		return nil // not a collection type
	}

	// if it's an array type
	if arrayType, ok := namedType.Underlying().(*types.Array); ok {
		size = arrayType.Len()
		kind = gct.TypeKindArray
	}

	// Create a placeholder collection type first to break circular dependencies
	collectionType := gct.NewNamedCollectionTypeInfo(
		id,
		kind,
		obj,
		r.currentPkg,
		r.docTypes[id],
		nil, // Will be set below
		size,
		nil, // Will be set below
	)

	// Register in cache early to prevent infinite recursion on self-referencing types
	r.types[id] = collectionType

	// Resolve the element type (this may refer back to the named type itself)
	// Example: type SelfRef []SelfRef
	elemType, pointerCount := r.deferPtr(elemType)
	ref := r.ResolveType(elemType)
	if ref == nil {
		// remove from cache if we couldn't resolve
		delete(r.types, id)
		return nil
	}

	// create the typeref
	typeRef := gct.NewTypeRef(ref.Id(), pointerCount, ref)

	// Set the typeref on the collection type
	collectionType.ElementReference = typeRef

	loader := func(ti gct.Type) error {
		collectionType.Methods = r.extractMethodInfos(ti)
		return nil
	}

	// Set the loader on the collection type
	collectionType.SetDetailsLoader(loader)

	return collectionType
}

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

// func (r *defaultTypeResolver) isBasicName(typeName string) bool {
// 	return slices.Contains(BasicTypes, typeName)
// }

// Determine if a type is in a scanned package
func (r *defaultTypeResolver) isScannedPackage(pkgPath string) bool {
	return r.scannedPackages[pkgPath]
}

// Calculate depth for a type
func (r *defaultTypeResolver) calculateTypeDepth(ti gct.Type, referencedFrom gct.Type) int {
	typeID := ti.Id()

	// Check if already calculated
	if depth, exists := r.typeDepths[typeID]; exists {
		return depth
	}

	// Extract package path from type ID
	pkgPath := extractPackagePath(typeID) // helper to parse package from canonical ID

	// If type is in a scanned package, depth is 0
	if r.isScannedPackage(pkgPath) {
		r.typeDepths[typeID] = 0
		return 0
	}

	// Otherwise, depth is 1 + depth of the type that referenced it
	if referencedFrom == nil {
		// Shouldn't happen, but default to 1
		r.typeDepths[typeID] = 1
		return 1
	}

	referrerDepth := referencedFrom.Depth()
	depth := referrerDepth + 1

	r.typeDepths[typeID] = depth
	return depth
}

func extractPackagePath(typeID string) string {
	parts := strings.Split(typeID, ".")
	if len(parts) < 2 {
		return ""
	}
	return strings.Join(parts[:len(parts)-1], ".")
}

// shouldExportType determines if a type should be exported based on visibility settings
func (r *defaultTypeResolver) shouldExportType(obj types.Object, kind gct.TypeKind) bool {
	if obj == nil {
		return false
	}

	exported := obj.Exported()
	r.logger.Debug(fmt.Sprintf("Checking visibility for %s (%s): exported=%v", obj.Name(), kind, exported))

	switch kind {
	case gct.TypeKindStruct, gct.TypeKindInterface:
		result := exported || r.confg.TypeVisibility.Has(VisibilityLevelUnexported)
		return result
	case gct.TypeKindMethod, gct.TypeKindFunction:
		result := exported || r.confg.MethodVisibility.Has(VisibilityLevelUnexported)
		return result
	case gct.TypeKindField:
		result := exported || r.confg.FieldVisibility.Has(VisibilityLevelUnexported)
		return result
	case gct.TypeKindEnum:
		result := exported || r.confg.EnumVisibility.Has(VisibilityLevelUnexported)
		return result
	default:
		result := exported || r.confg.TypeVisibility.Has(VisibilityLevelUnexported)
		return result
	}
}

// func (r *defaultTypeResolver) isBasicType(t types.Type) bool {
// 	// Check for actual basic types (*types.Basic)
// 	if _, ok := t.(*types.Basic); ok {
// 		return true
// 	}

// 	// Check for special predeclared types that should be treated as basic
// 	// even though they might be *types.Named or *types.Interface
// 	typeName := t.String()
// 	if typeName == "error" || typeName == "comparable" {
// 		return true
// 	}

// 	// For named types, check if underlying is basic or if name is in basic list
// 	if named, ok := t.(*types.Named); ok {
// 		// Check if the underlying type is basic
// 		if _, isBasic := named.Underlying().(*types.Basic); isBasic {
// 			return true
// 		}
// 		// Check if it's one of the special named types treated as basic
// 		return r.isBasicName(typeName)
// 	}

// 	return false
// }

func (r *defaultTypeResolver) normalizeUntyped(t types.Type) types.Type {
	if basic, ok := t.Underlying().(*types.Basic); ok {
		if basic.Info()&types.IsUntyped != 0 {
			switch basic.Kind() {
			case types.UntypedInt:
				return types.Typ[types.Int]
			case types.UntypedFloat:
				return types.Typ[types.Float64]
			case types.UntypedRune:
				return types.Typ[types.Rune]
			case types.UntypedString:
				return types.Typ[types.String]
			case types.UntypedBool:
				return types.Typ[types.Bool]
			}
		}
	}
	return t
}

// createAnonymousTypeName generates a unique name for anonymous types
// func (r *defaultTypeResolver) createAnonymousTypeName(kind TypeKind) string {
// 	r.anonymousCounter++
// 	var typeName string
// 	switch kind {
// 	case TypeKindInterface:
// 		typeName = "interface"
// 	case TypeKindStruct:
// 		typeName = "struct"
// 	default:
// 		typeName = "type"
// 	}
// 	return fmt.Sprintf("__anonymous_%s_%d__", typeName, r.anonymousCounter)
// }

// cache stores a type in the resolver's cache if appropriate
func (r *defaultTypeResolver) cache(ti gct.Type) gct.Type {
	if ti == nil {
		return nil
	}
	shouldCache := false
	switch ti.(type) {
	case *gct.BasicAliasTypeInfo:
		shouldCache = true
	case gct.NamedType:
		shouldCache = true // named types should be cached
	}

	if shouldCache {
		r.types[ti.Id()] = ti
	}
	// _, ok := ti.(gct.NamedType)
	// if !ok {
	// 	// function types can also be stored
	// 	_, ok = ti.(*gct.FunctionTypeInfo)

	// }
	// if ok {
	// 	r.types[ti.Id()] = ti
	// }
	return ti
}

// extractComments extracts comments for all declarations from parsed AST files
func (r *defaultTypeResolver) extractAst(pkgInfo *gct.Package) error {
	pkg := pkgInfo.Package()
	//comments := make(map[string]string)

	for i, file := range pkg.Syntax {
		// add the file to the package info
		var filePath string
		if i < len(pkg.CompiledGoFiles) {
			filePath = pkg.CompiledGoFiles[i]
		}
		f := gct.NewFileFromAst(file, filePath)
		// Pass both AST and file path to the constructor (update NewFileFromAst to accept path)
		pkgInfo.AddFiles(f)
		if file.Doc != nil {
			pkgLevelComment := strings.TrimSpace(file.Doc.Text())
			if pkgLevelComment != "" {
				// Store or process the package-level comment as needed.
				// For example, you might want to add it to pkgInfo with a special key:
				pkgInfo.AddComments("#PACKAGE_DOC", []gct.Comment{gct.NewComment(pkgLevelComment, gct.CommentPlacementPackage)})
			}

			f.AddComments(gct.NewComment(pkgLevelComment, gct.CommentPlacementPackage))

		}

		f.AddComments(gct.NewComment(strings.Join(extractCommentsBetweenPackageAndImports(file, pkg.Fset), "\n"), gct.CommentPlacementFile))

		for _, decl := range file.Decls {
			switch d := decl.(type) {
			case *ast.GenDecl:
				// Handle const, var, type declarations
				for _, spec := range d.Specs {
					switch s := spec.(type) {
					case *ast.ValueSpec:
						// Constants and variables
						comment := extractComment(s.Doc, s.Comment, d.Doc)
						for _, name := range s.Names {
							pkgInfo.AddComments(name.Name, comment)
						}
					case *ast.TypeSpec:
						// Type declarations
						comment := extractComment(s.Doc, s.Comment, d.Doc)
						pkgInfo.AddComments(s.Name.Name, comment)

						// Extract struct field comments
						if structType, ok := s.Type.(*ast.StructType); ok {
							for _, field := range structType.Fields.List {
								fieldComment := extractComment(field.Doc, field.Comment, nil)
								for _, fieldName := range field.Names {
									// comments[s.Name.Name+"."+fieldName.Name] = fieldComment
									pkgInfo.AddComments(s.Name.Name+"."+fieldName.Name, fieldComment)
								}
							}
						}

						// Extract interface method comments
						if interfaceType, ok := s.Type.(*ast.InterfaceType); ok {
							for _, method := range interfaceType.Methods.List {
								methodComment := extractComment(method.Doc, method.Comment, nil)
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
					recvType := getTypeName(d.Recv.List[0].Type)
					funcName = recvType + "." + funcName
				}
				comment = strings.TrimSpace(comment)
				if comment != "" {
					pkgInfo.AddComments(funcName, []gct.Comment{gct.NewComment(comment, gct.CommentPlacementAbove)})
				}
			}
		}
	}

	return nil
}
