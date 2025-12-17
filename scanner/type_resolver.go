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

type TypeCollection map[string]gct.Type
type ValueCollection map[string]gct.ValueType
type PackageCollection map[string]*gct.Package

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
	types            TypeCollection
	values           ValueCollection // All resolved ValueType objects
	packages         PackageCollection
	ignoredTypes     map[string]struct{}
	docTypes         map[string]*doc.Type         // All discovered doc types
	docFuncs         map[string]*doc.Func         // Add this field
	pkgs             map[string]*packages.Package // All loaded packages
	loadedPkgs       map[string]bool              // Track what's been processed
	basicTypes       TypeCollection               // Cache of basic types to avoid duplication
	currentPkg       *packages.Package            // Currently processing package
	confg            *Config
	logger           logger.Logger
	anonymousCounter int
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
		basicType := gct.NewBasicTypeInfo(basicTypeName, gct.TypeKindBasic)
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
	r.currentPkg = pkg

	// create the pkg object
	pkgInfo := gct.NewPackage(pkg)
	r.packages[pkg.PkgPath] = pkgInfo
	r.extractAst(pkgInfo)

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
			r.ParseValue(obj, pkg)
		}
	}

	// Variables
	for _, value := range docPkg.Vars {
		for _, name := range value.Names {
			obj := pkg.Types.Scope().Lookup(name)
			r.ParseValue(obj, pkg)
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

		// parse consts (for enum-like types)
		for _, constDecl := range docType.Consts {
			for _, name := range constDecl.Names {
				obj := pkg.Types.Scope().Lookup(name)
				r.ParseValue(obj, pkg)
			}
		}

		// Resolve the actual type via go/types
		obj := pkg.Types.Scope().Lookup(docType.Name)
		if obj == nil {
			continue
		}

		r.ResolveType(obj.Type())
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
func (r *defaultTypeResolver) ParseValue(obj types.Object, pkg *packages.Package) *gct.Value {
	if obj == nil {
		return nil
	}
	c, ok := obj.(*types.Const)
	if !ok {
		return nil
	}
	canonical := pkg.PkgPath + "." + c.Name()
	val := gct.NewConstValue(canonical, c, pkg, r.ResolveType(c.Type()))
	r.values[canonical] = val
	return val
}

// ResolveType resolves a Go type to TypeInfo
func (r *defaultTypeResolver) ResolveType(t types.Type) gct.Type {
	if t == nil {
		return nil
	}

	return r.resolveGoType(t)
}

// resolveGoType handles Go type objects
func (r *defaultTypeResolver) resolveGoType(t types.Type) gct.Type {
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

	// check if its a basic type
	if basicType, exists := r.basicTypes[typeName]; exists {
		return basicType
	}

	var ti gct.Type
	// Check cache first
	if ti, exists := r.types[typeName]; exists {
		return ti
	}

	r.logger.Debug(fmt.Sprintf("Resolving Go type: %v, %v", t, typeName))

	switch gt := t.(type) {
	// No cached types for basic types or slices

	case *types.Basic:
		// get the real basic type

		r.logger.Warn(fmt.Sprintf("Loading basic type %v, this seems an error...", gt.Name()))
	case *types.Slice, *types.Array:
		res := r.makeBasicCollection(typeName, gt)
		if res != nil {
			ti = res
		}
	case *types.Signature:
		res := r.makeFunctionInfo(r.GetCannonicalName(gt), nil)
		if res != nil {
			r.cache(res)
			ti = res
		}

	// all these types can be named types so it will cache the results
	case *types.Named:
		// check for basic named types first
		if r.isBasicType(gt) {
			r.logger.Warn(fmt.Sprintf("Loading named basic type %v, this seems an error...", gt.Obj().Name()))
			break
		}

		// Handle based on underlying type
		underlying := gt.Underlying()
		switch ut := underlying.(type) {
		case *types.Pointer:
			// Create a placeholder named type first to break circular dependencies
			id := r.GetCannonicalName(gt)
			res := r.makeNamedTypeInfo(id, gct.TypeKindPointer, gt.Obj(), r.docTypes[id], r.pkgs[id], nil)

			// Register in cache early to prevent infinite recursion on self-referencing types
			if res != nil {
				r.cache(res)
			}

			// resolve the pointer target (this may refer back to the named type)
			realType, pointers := r.deferPtr(ut)
			un := r.ResolveType(realType)
			if un != nil {
				tRef := gct.NewTypeRef(un.Id(), pointers, un)
				res.TypeRef = tRef
				ti = res
			} else {
				// delete the placeholder if we couldn't resolve the target
				delete(r.types, id)
			}
		case *types.Basic:

			// check for constants
			if IsConstant(gt.Obj()) {
				// This is a named constant like: const Pi = 3.14159
				// Handle constant creation here
			} else {
				res := r.makeNamedBasicInfo(typeName, ut, gt.Obj())
				if res != nil {
					// check if its a constant
					r.cache(res)
					ti = res
				}
			}
		case *types.Slice, *types.Array:
			res := r.makeNamedCollection(typeName, gt, gt.Obj())
			if res != nil {
				r.cache(res)
				ti = res
			}
		case *types.Signature:
			res := r.makeNamedFunction(typeName, gt, gt.Obj())
			if res != nil {
				r.cache(res)
				ti = res
			}

		default:
			// Handle unsupported named types
			r.logger.Warn(fmt.Sprintf("Unsupported named type %s with underlying %T", gt.String(), ut))
		}

	default:
		r.logger.Warn(fmt.Sprintf("Unsupported type encountered: %s (%T)", t.String(), t))
	}

	// trigger lazy loading of type details
	if ti != nil {
		if loadable, ok := ti.(gct.Loadable); ok {
			err := loadable.Load()
			if err != nil {
				r.logger.Error(fmt.Sprintf("Failed to load type details for %s: %v", ti.Id(), err))
			}
		}
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
		res := r.makeNamedTypeInfo(id, underlying.Kind(), obj, r.docTypes[id], r.pkgs[id], loader)
		res.TypeRef = tRef
		return res
	}

	return nil
}

func (r *defaultTypeResolver) makeNamedTypeInfo(id string, kind gct.TypeKind, obj types.Object, docType *doc.Type, pkg *packages.Package, detailsLoader gct.DetailsLoaderFn) *gct.NamedTypeInfo {
	return gct.NewNamedTypeInfo(id, kind, obj, docType, pkg, detailsLoader)

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
	functionType := gct.NewFunctionInfo(id, obj, docFunc, r.pkgs[id])

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
	res := gct.NewNamedFunctionInfo(id, obj, fn, r.docTypes[id], r.pkgs[id], loader)
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

	if sig, ok := methodObj.Type().(*types.Signature); ok {
		if recv := sig.Recv(); recv != nil {
			_, isPointerReceiver = recv.Type().(*types.Pointer)
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

	return methodInfo
}

/**
 * COLLECTON TYPES
**/

// makeBasicCollection creates a BasicCollectionTypeInfo for slice or array types
func (r *defaultTypeResolver) makeBasicCollection(id string, t types.Type) *gct.BasicCollectionTypeInfo {
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

	// create the collection type entry
	collectionType := gct.NewBasicCollectionTypeInfo(id, kind, typeRef, size)

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
		r.pkgs[id],
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

func (r *defaultTypeResolver) isBasicName(typeName string) bool {
	return slices.Contains(BasicTypes, typeName)
}

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

func (r *defaultTypeResolver) isBasicType(t types.Type) bool {
	_, ok := t.(*types.Basic)
	if !ok {
		_, ok := t.(*types.Named)
		if ok {
			// check if it's one of the special named types treated as basic types
			return r.isBasicName(t.String())
		}
	}
	return ok
}

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
	_, ok := ti.(gct.NamedType)
	if !ok {
		// function types can also be stored
		_, ok = ti.(*gct.FunctionTypeInfo)
	}
	if ok {
		r.types[ti.Id()] = ti
	}
	return ti
}

// extractComments extracts comments for all declarations from parsed AST files
func (r *defaultTypeResolver) extractAst(pkgInfo *gct.Package) error {
	pkg := pkgInfo.Package()
	//comments := make(map[string]string)

	for _, file := range pkg.Syntax {
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

// extractComment combines doc comments and inline comments
func extractComment(doc, comment, parentDoc *ast.CommentGroup) []gct.Comment {
	var parts []gct.Comment

	// Add doc comment (above the declaration)
	if doc != nil {
		if text := strings.TrimSpace(doc.Text()); text != "" {
			parts = append(parts, gct.NewComment(text, gct.CommentPlacementAbove))
		}
	}

	// Add inline comment (after the declaration)
	if comment != nil {
		if text := strings.TrimSpace(comment.Text()); text != "" {
			parts = append(parts, gct.NewComment(text, gct.CommentPlacementInline))
		}
	}

	// Fallback to parent doc if no specific comments
	if len(parts) == 0 && parentDoc != nil {
		if text := strings.TrimSpace(parentDoc.Text()); text != "" {
			parts = append(parts, gct.NewComment(text, gct.CommentPlacementAbove))
		}
	}

	return parts
}

// getTypeName extracts the type name from an expression
func getTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return getTypeName(t.X)
	case *ast.SelectorExpr:
		return getTypeName(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}
