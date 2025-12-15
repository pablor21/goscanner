package scanner

import (
	"fmt"
	"go/doc"
	"go/types"
	"slices"

	"github.com/pablor21/goscanner/logger"
	// use the types as a local alias
	. "github.com/pablor21/goscanner/types"
	"golang.org/x/tools/go/packages"
)

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

type TypeCollection map[string]TypeEntry

// TypeResolver is an interface for resolving types and managing type information
type TypeResolver interface {
	// ResolveType resolves a types.Type to a TypeInfo
	ResolveType(t types.Type) TypeEntry
	// GetCannonicalName returns the canonical name of a type
	GetCannonicalName(t types.Type) string
	// ProcessPackage processes a package to extract and cache type information (scans the entire package)
	ProcessPackage(pkg *packages.Package) error
	// GetTypeInfos returns all resolved TypeInfo objects
	GetTypeInfos() TypeCollection
}

type defaultTypeResolver struct {
	types            TypeCollection
	ignoredTypes     map[string]struct{}
	docTypes         map[string]*doc.Type         // All discovered doc types
	packages         map[string]*packages.Package // All loaded packages
	loadedPkgs       map[string]bool              // Track what's been processed
	currentPkg       *packages.Package            // Currently processing package
	confg            *Config
	logger           logger.Logger
	anonymousCounter int
}

func newDefaultTypeResolver(confg *Config, log logger.Logger) *defaultTypeResolver {
	tr := &defaultTypeResolver{
		types:            make(map[string]TypeEntry),
		ignoredTypes:     make(map[string]struct{}),
		docTypes:         make(map[string]*doc.Type),
		packages:         make(map[string]*packages.Package),
		loadedPkgs:       make(map[string]bool),
		confg:            confg,
		logger:           log,
		anonymousCounter: 0,
	}
	if tr.logger == nil {
		tr.logger = logger.NewDefaultLogger()
	}

	tr.logger.SetTag("TypeResolver")

	return tr
}

func (r *defaultTypeResolver) GetTypeInfos() TypeCollection {
	return r.types
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

	// 1. Extract doc.Type information
	docPkg, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath, doc.AllMethods|doc.AllDecls)
	if err != nil {
		return err
	}

	r.packages[pkg.PkgPath] = pkg
	r.loadedPkgs[pkg.PkgPath] = true

	for _, docType := range docPkg.Types {
		canonicalName := pkg.PkgPath + "." + docType.Name
		r.docTypes[canonicalName] = docType
		// resolve type to cache it
		t := pkg.Types.Scope().Lookup(docType.Name)
		if t == nil {
			continue
		}
		r.ResolveType(t.Type())

	}
	return nil

	// 	// Skip generic type aliases - they will be handled in processTypeAliases
	// 	if alias, ok := t.Type().(*types.Alias); ok {
	// 		// Check if this is a generic alias by seeing if the type string contains type parameters
	// 		aliasTypeString := types.TypeString(alias, func(pkg *types.Package) string { return pkg.Path() })
	// 		if strings.Contains(aliasTypeString, "[") && strings.Contains(aliasTypeString, "]") {
	// 			continue
	// 		}
	// 	}

	// 	// Only resolve the base type, not generic instantiations
	// 	// Check if this is a generic type by examining if it has type parameters
	// 	if namedType, ok := t.Type().(*types.Named); ok {
	// 		// For generic types, process normally - GetCannonicalName should handle the naming
	// 		r.ResolveType(namedType)
	// 	} else {
	// 		// Non-named type (basic, interface{}, etc.) - resolve normally
	// 		r.ResolveType(t.Type())
	// 	}
	// } // Process type aliases AFTER regular types so they can inherit properly
	// err = r.processTypeAliases(pkg)
	// if err != nil {
	// 	return err
	// }
}

// ResolveType resolves a Go type to TypeInfo
func (r *defaultTypeResolver) ResolveType(t types.Type) TypeEntry {
	if t == nil {
		return nil
	}

	return r.resolveGoType(t)
}

// resolveGoType handles Go type objects
func (r *defaultTypeResolver) resolveGoType(t types.Type) TypeEntry {
	// check for nil
	if t == nil {
		return nil
	}

	if ptr, ok := t.(*types.Pointer); ok {
		r.logger.Error(fmt.Sprintf("invalid type %v, pointers must be deferenced before processing", ptr))
		return nil
	}

	typeName := r.GetCannonicalName(t)
	r.logger.Debug(fmt.Sprintf("Resolving Go type: %v, %v", t, typeName))

	var ti TypeEntry
	// Check cache first
	if ti, exists := r.types[typeName]; exists {
		return ti
	}

	// check for pointer types
	switch gt := t.(type) {
	// No cached types for basic types or slices
	case *types.Basic:
		res := r.createBasicType(gt)
		if res != nil {
			ti = res
		}
	case *types.Slice:
		res := r.createSliceInfo(gt, nil, nil)
		if res != nil {
			ti = res
		}
	// all these types can be named types so it will cache the results
	case *types.Named:
		// check for basic named types first
		if r.isBasicType(gt) {
			res := r.createBasicTypeFromName(gt.Obj().Name())
			if res != nil {
				ti = res
			}
			break
		}

		// Handle based on underlying type
		underlying := gt.Underlying()
		switch ut := underlying.(type) {
		case *types.Basic:
			// Named basic type like time.Duration
			res := r.createBasicTypeFromName(r.GetCannonicalName(gt))
			if res != nil {
				ti = res
			}
		case *types.Struct:
			// Named struct type
			res := r.createStructInfo(ut, gt.Obj(), gt)
			if res != nil {
				// Cache if appropriate - use the actual canonical name from the created type
				if r.shouldStoreInMainRegistry(res) {
					r.types[res.Id()] = res
				}
				ti = res
			}
		case *types.Slice:
			// Named slice type
			res := r.createSliceInfo(ut, gt.Obj(), gt)
			if res != nil {
				// Cache if appropriate - use the actual canonical name from the created type
				if r.shouldStoreInMainRegistry(res) {
					r.types[res.Id()] = res
				}
				ti = res
			}
		default:
			// Handle unsupported named types gracefully
			r.logger.Warn(fmt.Sprintf("Unsupported named type %s with underlying %T", gt.String(), ut))
		}

	default:
		r.logger.Warn(fmt.Sprintf("Unsupported type encountered: %s (%T)", t.String(), t))
	}

	// trigger lazy loading of type details
	if ti != nil {

		err := ti.Load()
		if err != nil {
			r.logger.Error(fmt.Sprintf("Failed to load type details for %s: %v", ti.Id(), err))
		}
	}

	return ti

	// case *types.Slice:
	// 	return r.makeSliceTypeInfo(goType, typeName)
	// case *types.Array:
	// 	return r.makeArrayTypeInfo(goType, typeName)
	// case *types.Map:
	// 	return r.makeMapTypeInfo(goType, typeName)
	// case *types.Chan:
	// 	return r.makeChannelTypeInfo(goType, typeName)
	// case *types.Pointer:
	// 	return r.ResolveType(ut.Elem()) // Delegate to main entry point
	// case *types.Signature:
	// 	return r.createNamedTypeInfo(TypeKindFunction, typeName, "", typeName)
	// case *types.Interface:
	// 	// Handle anonymous interfaces properly
	// 	if interfaceType, ok := goType.(*types.Interface); ok {
	// 		if interfaceType.NumMethods() > 0 {
	// 			return r.createAnonymousTypeInfo(interfaceType, r.currentPkg.Types.Path())
	// 		} else {
	// 			return r.createBuiltinInterfaceTypeInfo("any")
	// 		}
	// 	}
	// 	return r.createBuiltinInterfaceTypeInfo("any")
	// default:
	// 	return r.createNamedTypeInfo(TypeKindVariable, typeName, "", typeName)
	// }
}

func (r *defaultTypeResolver) createBasicType(basicType *types.Basic) *BasicTypeEntry {
	return r.createBasicTypeFromName(basicType.String())
}

func (r *defaultTypeResolver) createBasicTypeFromName(typeName string) *BasicTypeEntry {
	if typeName == "" {
		return nil
	}
	return NewBasicTypeEntry(typeName, TypeKindBasic)
}

// func (r *defaultTypeResolver) createComplexType(namedType *types.Named, obj types.Object) TypeEntry {
// 	if namedType == nil || obj == nil {
// 		return nil
// 	}
// 	switch ut := namedType.Underlying().(type) {
// 	case *types.Struct:
// 		res := r.createStructInfo(ut, obj, namedType)
// 		if res != nil {
// 			// Cache if appropriate - use the actual canonical name from the created type
// 			if r.shouldStoreInMainRegistry(res) {
// 				r.types[res.Id()] = res
// 			}
// 			return res
// 		}

// 	//case *types.Interface:
// 	// return r.createNamedInterfaceTypeInfo(ut, obj)
// 	// case *types.Slice:
// 	// 	r := r.createSliceInfo(ut, obj, namedType)
// 	// 	if r != nil {
// 	// 		return r
// 	// 	}
// 	default:
// 		// Handle unsupported types gracefully
// 		r.logger.Warn(fmt.Sprintf("Unsupported type for %s with underlying %T", namedType.String(), ut))
// 		return nil
// 	}
// 	return nil
// }

// createNamedType creates a NamedTypeInfo from a types.Named
// func (r *defaultTypeResolver) createNamedType(namedType *types.Named, obj types.Object) *ComplexTypeEntry {
// 	if namedType == nil {
// 		return nil
// 	}
// 	switch ut := namedType.Underlying().(type) {
// 	case *types.Struct:
// 		return r.createNamedStructTypeInfo(ut, obj, namedType)
// 	case *types.Interface:
// 		return r.createNamedInterfaceTypeInfo(ut, obj)
// 	default:
// 		r.logger.Debug(fmt.Sprintf("Creating named type info for %s with underlying %T", namedType.String(), ut))
// 	}
// 	return nil
// }

func (r *defaultTypeResolver) createSliceInfo(sliceType *types.Slice, obj types.Object, namedType *types.Named) *CollectionTypeEntry {
	if sliceType == nil {
		return nil
	}

	// get the element type reference
	elemType, pointerCount := r.deferPtr(sliceType.Elem())
	ref := r.ResolveType(elemType)
	if ref == nil {
		return nil
	}

	// create the typeref
	typeRef := NewTypeRef(ref.Id(), pointerCount, ref)

	// determine the slice type name - use namedType if available, otherwise use canonical name
	var sliceTypeName string
	var docType *doc.Type
	if namedType != nil {
		sliceTypeName = r.GetCannonicalName(namedType)
		docType = r.docTypes[sliceTypeName]
	} else {
		// for unnamed slices like []int, use the slice type itself
		sliceTypeName = "slice"
		docType = nil
	}

	// create the collection type entry
	collectionType := NewCollectionTypeEntry(sliceTypeName, TypeKindSlice, typeRef, obj, docType, nil)

	return collectionType
}

func (r *defaultTypeResolver) createStructInfo(structType *types.Struct, obj types.Object, namedType *types.Named) *ComplexTypeEntry {
	if structType == nil {
		return nil
	}

	// check if we should export this struct
	if !r.shouldExportType(obj, TypeKindStruct) {
		r.logger.Debug(fmt.Sprintf("Skipping unexported struct: %s", r.GetCannonicalName(structType)))
		return nil
	}

	// the loader lazy loads the struct details when needed
	loader := func(ti TypeEntry) error {

		if ct, ok := ti.(*ComplexTypeEntry); ok {

			// Extract fields
			if r.confg.ScanMode.Has(ScanModeFields) {
				ct.Methods = []*MethodInfo{}
				ct.Fields = []*FieldInfo{}
				for field := range structType.Fields() {
					if !r.shouldExportType(field, TypeKindField) {
						r.logger.Debug(fmt.Sprintf("Skipping unexported field: %s:%s", ti.Id(), field.Name()))
						continue
					}

					// get the typeref, first deference the type if it's a pointer
					fieldType, pointerCount := r.deferPtr(field.Type())
					ref := r.ResolveType(fieldType)
					if ref == nil {
						continue
					}

					// // create the typeref
					typeRef := NewTypeRef(ref.Id(), pointerCount, ref)

					if typeRef == nil {
						continue
					}

					// // load the referenced type details
					// err := typeRef.Load()
					// if err != nil {
					// 	r.logger.Error(fmt.Sprintf("Failed to load type details for field type %s: %v", typeRef.TypeRefId(), err))
					// }

					// typeRef.RefID = r.GetCannonicalName(fieldType)
					// typeRef.PointerFlag = pointerCount > 0
					// typeRef.PointerCount = pointerCount
					// typeRef.Reference = r.ResolveType(field.Type())
					// typeRef.RefKind = ref.Kind()

					// create the field info
					fieldInfo := FieldInfo{
						Name:    field.Name(),
						TypeRef: *typeRef,
					}

					ct.Fields = append(ct.Fields, &fieldInfo)
				}

				// // Method set for pointer type (*T) - includes methods with both T and *T receivers
				// pointerType := types.NewPointer(namedType)
				// pointerMethodSet := types.NewMethodSet(pointerType)
				// r.logger.Debug(fmt.Sprintf("Struct %s has %d methods", obj.Name(), pointerMethodSet.Len()))
				// for method := range pointerMethodSet.Methods() {
				// 	methodObj := method.Obj()

				// 	// check if its pointer receiver
				// 	isPointerReceiver := false
				// 	if sig, ok := methodObj.Type().(*types.Signature); ok {
				// 		if recv := sig.Recv(); recv != nil {
				// 			_, isPointerReceiver = recv.Type().(*types.Pointer)
				// 		}
				// 	}

				// 	methodInfo := r.createMethodInfo(methodObj, ti.GetCannonicalName(), isPointerReceiver, false)
				// 	if methodInfo != nil {
				// 		details.Methods = append(details.Methods, *methodInfo)
				// 		methodInfo.Load()
				// 	}
				// }

			}
		}

		return nil
	}

	// tInfo := NewNamedTypeInfoFromTypes(TypeKindStruct, obj, r.currentPkg, r.docTypes[r.GetCannonicalName(structType)], loader)
	tInfo := NewComplexTypeEntry(r.GetCannonicalName(namedType), TypeKindStruct, obj, r.docTypes[r.GetCannonicalName(structType)], loader)
	return tInfo
}

// func (r *defaultTypeResolver) createNamedInterfaceTypeInfo(interfaceType *types.Interface, obj types.Object) *NamedTypeInfo {
// 	if interfaceType == nil {
// 		return nil
// 	}

// 	if !r.shouldExportType(obj, TypeKindInterface) {
// 		r.logger.Debug(fmt.Sprintf("Skipping unexported interface: %s", r.GetCannonicalName(interfaceType)))
// 		return nil
// 	}

// 	loader := func(ti TypeInfo) (*DetailedTypeInfo, error) {
// 		details := &DetailedTypeInfo{}

// 		// Extract methods
// 		if r.confg.ScanMode.Has(ScanModeMethods) {
// 			r.logger.Debug(fmt.Sprintf("Interface %s has %d methods", obj.Name(), interfaceType.NumMethods()))
// 			for method := range interfaceType.Methods() {
// 				// Interface methods are never pointer receivers (they're just contracts)
// 				methodInfo := r.createMethodInfo(method, ti.GetCannonicalName(), false, true)
// 				if methodInfo != nil {
// 					details.Methods = append(details.Methods, *methodInfo)
// 					methodInfo.Load()
// 				}
// 			}
// 		}

// 		return details, nil
// 	}

// 	tInfo := NewNamedTypeInfoFromTypes(TypeKindInterface, obj, r.currentPkg, r.docTypes[r.GetCannonicalName(interfaceType)], loader)

// 	// For anonymous interfaces, mark as anonymous
// 	if obj == nil || obj.Name() == "" {
// 		tInfo.AnonymousFlag = true
// 	}

// 	return tInfo
// }

// func (r *defaultTypeResolver) createMethodInfo(methodObj types.Object, parentTypeName string, isPointerReceiver bool, isInterfaceMethod bool) *MethodInfo {
// 	if methodObj == nil {
// 		return nil
// 	}

// 	// check if we should export this method
// 	if !r.shouldExportType(methodObj, TypeKindMethod) {
// 		r.logger.Debug(fmt.Sprintf("Skipping unexported method: %s.%s", parentTypeName, methodObj.Name()))
// 		return nil
// 	}

// 	isVariadic := false
// 	if sig, ok := methodObj.Type().(*types.Signature); ok {
// 		// check if it's variadic
// 		if sig.Variadic() {
// 			isVariadic = true
// 		}
// 	}

// 	// the loader will lazy load parameters and returns when needed
// 	loader := func(ti TypeInfo) (*DetailedTypeInfo, error) {
// 		details := &DetailedTypeInfo{}
// 		if mi, ok := ti.(*MethodInfo); ok {

// 			if sig, ok := methodObj.Type().(*types.Signature); ok {
// 				// Parameters
// 				params := sig.Params()
// 				for i := 0; i < params.Len(); i++ {
// 					paramVar := params.At(i)
// 					paramInfo := ParameterInfo{
// 						BaseTypeDetailInfo: BaseTypeDetailInfo{
// 							Name: paramVar.Name(),
// 						},
// 					}
// 					paramInfo.SetTypeRef(r.ResolveType(paramVar.Type()))
// 					mi.Parameters = append(mi.Parameters, paramInfo)
// 				}
// 			}

// 			if sig, ok := methodObj.Type().(*types.Signature); ok {
// 				// Returns
// 				returns := sig.Results()
// 				for i := 0; i < returns.Len(); i++ {
// 					returnVar := returns.At(i)
// 					returnInfo := ReturnInfo{}
// 					returnInfo.SetTypeRef(r.ResolveType(returnVar.Type()))
// 					mi.Returns = append(mi.Returns, returnInfo)
// 				}

// 			}
// 		}

// 		return details, nil
// 	}

// 	methodInfo := NewMethodInfo(methodObj.Name(), "", parentTypeName, isPointerReceiver, loader)
// 	methodInfo.SetTypeRef(r.ResolveType(methodObj.Type()))
// 	methodInfo.IsVariadic = isVariadic
// 	methodInfo.IsInterfaceMethod = isInterfaceMethod

// 	return methodInfo
// }

// createBasicTypeFromName creates a BasicTypeInfo from a type name
// func (r *defaultTypeResolver) createBasicTypeFromName(typeName string) TypeEntry {
// 	if typeName == "" {
// 		return nil
// 	}
// 	// create type info
// 	ti := NewBasicTypeEntry(typeName,TypeKindBasic,, nil)

// 	return ti
// }

// // createBasicType creates a BasicTypeInfo from basic types (string, int, bool, etc.)
// func (r *defaultTypeResolver) createBasicType(basicType types.Object) *NamedTypeInfo {
// 	if basicType == nil {
// 		return nil
// 	}
// 	return NewNamedTypeInfoFromTypes(TypeKindBasic, basicType, r.currentPkg, r.docTypes[r.GetCannonicalName(basicType.Type())], nil)
// }

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

func (r *defaultTypeResolver) shouldExportType(obj types.Object, kind TypeKind) bool {
	if obj == nil {
		return false
	}

	exported := obj.Exported()
	r.logger.Debug(fmt.Sprintf("Checking visibility for %s (%s): exported=%v", obj.Name(), kind, exported))

	switch kind {
	case TypeKindStruct, TypeKindInterface:
		result := exported || r.confg.TypeVisibility.Has(VisibilityLevelUnexported)
		return result
	case TypeKindMethod, TypeKindFunction:
		result := exported || r.confg.MethodVisibility.Has(VisibilityLevelUnexported)
		return result
	case TypeKindField:
		result := exported || r.confg.FieldVisibility.Has(VisibilityLevelUnexported)
		return result
	case TypeKindEnum:
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

// shouldStoreInMainRegistry determines if a type should be stored in the main types registry
// Basic types and generic parameters should NOT be stored (they are primitives, not user-defined types)
func (r *defaultTypeResolver) shouldStoreInMainRegistry(ti TypeEntry) bool {
	if ti == nil {
		return false
	}

	kind := ti.Kind()
	// Only store user-defined types (structs, interfaces, etc.) - NOT basic types or generic params
	return kind != TypeKindBasic && kind != TypeKindGenericParam
}
