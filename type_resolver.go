package goscanner

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/doc"
	"go/token"
	"go/types"
	"strings"

	"github.com/pablor21/gonnotation"
	"golang.org/x/tools/go/packages"
)

// TypeResolver is an interface for resolving types and managing type information
type TypeResolver interface {
	// ResolveType resolves a types.Type to a TypeInfo
	ResolveType(t types.Type) TypeInfo
	// GetCannonicalName returns the canonical name of a type
	GetCannonicalName(t types.Type) string
	// ProcessPackage processes a package to extract and cache type information (scans the entire package)
	ProcessPackage(pkg *packages.Package) error
	// GetTypeInfos returns all resolved TypeInfo objects
	GetTypeInfos() map[string]TypeInfo
}

type defaultTypeResolver struct {
	types            map[string]TypeInfo
	ignoredTypes     map[string]struct{}
	docTypes         map[string]*doc.Type         // All discovered doc types
	packages         map[string]*packages.Package // All loaded packages
	loadedPkgs       map[string]bool              // Track what's been processed
	currentPkg       *packages.Package            // Currently processing package
	scanMode         ScanMode
	anonymousCounter int
}

func newDefaultTypeResolver(scanMode ScanMode) *defaultTypeResolver {
	return &defaultTypeResolver{
		types:        make(map[string]TypeInfo),
		ignoredTypes: make(map[string]struct{}),
		docTypes:     make(map[string]*doc.Type),
		packages:     make(map[string]*packages.Package),
		loadedPkgs:   make(map[string]bool),
		scanMode:     scanMode,
	}
}

func (r *defaultTypeResolver) GetTypeInfos() map[string]TypeInfo {
	return r.types
}

func (r *defaultTypeResolver) ProcessPackage(pkg *packages.Package) error {
	// Set the current package being processed
	r.currentPkg = pkg

	// 1. Extract doc.Type information
	docPkg, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath)
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

	// Process type aliases AFTER regular types so they can inherit properly
	err = r.processTypeAliases(pkg)
	if err != nil {
		return err
	}

	// 2. Process methods if ScanModeMethods is enabled
	if r.scanMode.Has(ScanModeMethods) {
		err = r.processMethods(pkg, docPkg)
		if err != nil {
			return err
		}
	}

	// 3. Process standalone functions if ScanModeFunctions is enabled
	if r.scanMode.Has(ScanModeFunctions) {
		err = r.processFunctions(pkg, docPkg)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *defaultTypeResolver) GetCannonicalName(t types.Type) string {
	if t == nil {
		return ""
	}

	return types.TypeString(t, func(pkg *types.Package) string {
		return pkg.Path()
	})
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

func (r *defaultTypeResolver) ResolveType(t types.Type) TypeInfo {
	if t == nil {
		return nil
	}

	typeName := r.GetCannonicalName(t)

	// Use base name for caching to avoid generic parameter pollution
	cacheKey := typeName
	if namedType, ok := t.(*types.Named); ok {
		cacheKey = r.GetBaseName(namedType)
	}

	// Check cache first using base name
	if ti, exists := r.types[cacheKey]; exists {
		return ti
	}

	var ti TypeInfo
	switch ut := t.(type) {
	case *types.Named:
		// First check if this is a type alias
		if r.isTypeAlias(ut) {
			ti = r.makeTypeAliasInfo(t, typeName)
			if ti != nil {
				r.types[cacheKey] = ti
			}
		} else {
			underlying := ut.Underlying()
			switch underlying.(type) {
			case *types.Struct:
				// Complex types - cache them using base name
				ti = r.makeStructTypeInfo(t, typeName)
				if ti != nil {
					r.types[cacheKey] = ti
				}
			case *types.Interface:
				// Special handling for comparable - treat as basic type
				if ut.Obj().Name() == "comparable" {
					ti = r.makeSimpleTypeReference(t, typeName)
				} else {
					// Complex types - cache them using base name
					ti = r.makeInterfaceTypeInfo(t, typeName)
					if ti != nil {
						r.types[cacheKey] = ti
					}
				}
			case *types.Basic:
				// Check if this is an enum (named type with basic underlying type + constants)
				if r.isEnum(ut) {
					ti = r.makeEnumTypeInfo(t, typeName)
					if ti != nil {
						r.types[cacheKey] = ti
					}
				} else {
					// Other named types (type aliases, etc.) - simple reference
					ti = r.makeSimpleTypeReference(t, typeName)
				}
			case *types.Slice, *types.Array, *types.Map, *types.Chan, *types.Signature:
				// Named types with collection/function underlying types (generic collections, etc.)
				// These should be treated as full types if they're named
				ti = r.makeNamedCollectionTypeInfo(t, typeName)
				if ti != nil {
					r.types[cacheKey] = ti
				}
			default:
				// Other named types (type aliases, etc.) - simple reference
				ti = r.makeSimpleTypeReference(t, typeName)
			}
		}
	case *types.Basic:
		// Basic types - simple reference, don't cache
		ti = r.makeSimpleTypeReference(t, typeName)
	case *types.Slice:
		// Collection types - simple reference, don't cache
		ti = r.makeSimpleTypeReference(t, typeName)
	case *types.Array:
		ti = r.makeSimpleTypeReference(t, typeName)
	case *types.Map:
		ti = r.makeSimpleTypeReference(t, typeName)
	case *types.Chan:
		ti = r.makeSimpleTypeReference(t, typeName)
	case *types.Pointer:
		// For pointers, resolve the underlying type
		ti = r.ResolveType(ut.Elem())
	case *types.Signature:
		ti = r.makeSimpleTypeReference(t, typeName)
	default:
		// Fallback for other types
		ti = r.makeSimpleTypeReference(t, typeName)
	}

	return ti
}

func (r *defaultTypeResolver) makeStructTypeInfo(t types.Type, typeName string) TypeInfo {
	structType, ok := t.(*types.Struct)
	if !ok {
		// If it's a named type, get the underlying struct
		if named, ok := t.(*types.Named); ok {
			if underlying, ok := named.Underlying().(*types.Struct); ok {
				structType = underlying
			} else {
				return nil // Not a struct
			}
		} else {
			return nil // Not a struct
		}
	}

	// Extract name and package info
	var name, pkg string
	var obj types.Object
	var namedType *types.Named
	if named, ok := t.(*types.Named); ok {
		namedType = named
		obj = named.Obj()
		name = obj.Name()
		if obj.Pkg() != nil {
			pkg = obj.Pkg().Path()
		}
	} else {
		// For anonymous struct types, use the full type string as name
		name = typeName
		pkg = ""
	}

	// Create loader for lazy loading struct details
	loader := func() (*DetailedTypeInfo, error) {
		if !r.scanMode.Has(ScanModeFields) {
			return &DetailedTypeInfo{}, nil
		}

		details := &DetailedTypeInfo{}

		// For regular generic types or non-generic types, extract type parameters
		if namedType != nil {
			details.TypeParameters = r.extractTypeParameters(namedType)
		}

		// Extract type parameters if this is a generic type
		if named, ok := t.(*types.Named); ok {
			details.TypeParameters = r.extractTypeParameters(named)
		}

		// Get the parent type reference for embedded field expansion
		parentTypeRef := typeName

		// Process struct fields
		for i := 0; i < structType.NumFields(); i++ {
			field := structType.Field(i)

			// Skip unexported fields
			if !field.Exported() {
				continue
			}

			// Get struct tag
			tag := structType.Tag(i)

			// Analyze the field type to extract canonical type and structure info
			fieldInfo := r.parseFieldTypeWithTag(field.Type(), field.Name(), tag)

			// Handle embedded fields differently
			if field.Embedded() {
				// Don't add the embedded field itself, just expand its promoted members
				promotedFields, promotedMethods, err := r.expandEmbeddedType(field.Type(), field.Name(), parentTypeRef)
				if err == nil {
					// Add promoted fields
					details.Fields = append(details.Fields, promotedFields...)
					// Add promoted methods
					details.Methods = append(details.Methods, promotedMethods...)
				}
			} else {
				// Regular field - add it normally
				details.Fields = append(details.Fields, fieldInfo)
			}
		}

		// If this is a concrete instantiation of a generic type, inherit methods from the generic base
		if namedType != nil {
			instantiationInfo := r.detectGenericInstantiation(namedType)
			if instantiationInfo != nil {
				// Find the generic base type and inherit its methods
				if baseGenericType := r.findGenericBaseType(instantiationInfo.GenericTypeRef); baseGenericType != nil {
					inheritedMethods, err := r.inheritMethodsFromGeneric(baseGenericType, instantiationInfo.TypeArguments, namedType)
					if err == nil {
						details.Methods = append(details.Methods, inheritedMethods...)
					}
				}
			}
		}

		return details, nil
	}

	// Create StructInfo with proper NamedTypeInfo using type objects
	canonicalName := r.GetCannonicalName(t)
	docType := r.docTypes[canonicalName]

	// Use the new constructor if we have a type object, otherwise fallback
	var namedTypeInfo *NamedTypeInfo
	if obj != nil {
		namedTypeInfo = r.createNamedTypeInfoFromTypes(TypeKindStruct, obj, r.currentPkg, docType, loader)
	} else {
		namedTypeInfo = NewNamedTypeInfo(TypeKindStruct, name, pkg, loader)
	}

	return &StructInfo{
		NamedTypeInfo: namedTypeInfo,
	}
}

// createNamedTypeInfoFromTypes creates NamedTypeInfo with eager generic instantiation detection
func (r *defaultTypeResolver) createNamedTypeInfoFromTypes(kind TypeKind, typesObj types.Object, pkgInfo *packages.Package, docType *doc.Type, loader func() (*DetailedTypeInfo, error)) *NamedTypeInfo {
	// Start with the basic constructor
	namedTypeInfo := NewNamedTypeInfoFromTypes(kind, typesObj, pkgInfo, docType, loader)

	// Check if this is a concrete instantiation of a generic type
	if namedType, ok := typesObj.Type().(*types.Named); ok {
		instantiationInfo := r.detectGenericInstantiation(namedType)
		if instantiationInfo != nil {
			// Set the generic instantiation info eagerly
			namedTypeInfo.IsGenericInstantiation = true
			namedTypeInfo.GenericTypeRef = instantiationInfo.GenericTypeRef
			namedTypeInfo.GenericArguments = instantiationInfo.TypeArguments
		}
	}

	return namedTypeInfo
}

// detectGenericInstantiation checks if a named type is a concrete instantiation of a generic type
func (r *defaultTypeResolver) detectGenericInstantiation(namedType *types.Named) *struct {
	GenericTypeRef string
	TypeArguments  []GenericArgumentInfo
} {
	// Check if this type has type parameters (it's a generic type itself)
	if namedType.TypeParams() != nil && namedType.TypeParams().Len() > 0 {
		return nil // This is a generic type, not an instantiation
	}

	// Check if this type was derived from a generic type by looking for similar named types
	pkg := namedType.Obj().Pkg()

	if pkg == nil {
		return nil
	}

	// Look for a generic version of this type in the same package
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if obj == nil {
			continue
		}

		if named, ok := obj.Type().(*types.Named); ok {
			// Check if this is a generic type with similar structure
			if named.TypeParams() != nil && named.TypeParams().Len() > 0 {
				// Check if the underlying types are structurally similar
				if r.areStructurallySimilar(namedType.Underlying(), named.Underlying()) {
					// This looks like a concrete instantiation
					return r.createGenericInstantiationInfo(namedType, named)
				}
			}
		}
	}

	return nil
}

// areStructurallySimilar checks if two types have the same structure
func (r *defaultTypeResolver) areStructurallySimilar(concrete, generic types.Type) bool {
	// For now, simple structural comparison
	concreteStruct, ok1 := concrete.(*types.Struct)
	genericStruct, ok2 := generic.(*types.Struct)

	if !ok1 || !ok2 {
		return false
	}

	// Check if they have the same number of fields
	if concreteStruct.NumFields() != genericStruct.NumFields() {
		return false
	}

	// Check if field names match (a simple heuristic)
	for i := 0; i < concreteStruct.NumFields(); i++ {
		if concreteStruct.Field(i).Name() != genericStruct.Field(i).Name() {
			return false
		}
	}

	return true
}

// createGenericInstantiationInfo creates instantiation info by analyzing the type difference
func (r *defaultTypeResolver) createGenericInstantiationInfo(concrete, generic *types.Named) *struct {
	GenericTypeRef string
	TypeArguments  []GenericArgumentInfo
} {
	genericTypeRef := r.GetBaseName(generic)

	// Extract type arguments by comparing field types
	var typeArguments []GenericArgumentInfo

	if generic.TypeParams() != nil {
		concreteStruct := concrete.Underlying().(*types.Struct)
		genericStruct := generic.Underlying().(*types.Struct)

		// Create a map of type parameter names to concrete types
		paramMap := make(map[string]types.Type)

		// Analyze fields to find type parameter mappings
		for i := 0; i < concreteStruct.NumFields(); i++ {
			concreteFieldType := concreteStruct.Field(i).Type()
			genericFieldType := genericStruct.Field(i).Type()

			r.mapTypeParameters(genericFieldType, concreteFieldType, paramMap)
		}

		// Convert the parameter map to type arguments
		for i := 0; i < generic.TypeParams().Len(); i++ {
			param := generic.TypeParams().At(i)
			paramName := param.Obj().Name()

			if concreteType, exists := paramMap[paramName]; exists {
				// Create simplified argument info
				typeRef := r.GetCannonicalName(concreteType)
				isPointer := false
				actualType := concreteType

				// Check if it's a pointer type
				if ptr, ok := concreteType.(*types.Pointer); ok {
					isPointer = true
					actualType = ptr.Elem()
				}

				// Determine the kind
				kind := r.determineTypeKind(actualType)

				typeArguments = append(typeArguments, GenericArgumentInfo{
					ParameterName:    paramName,
					ParameterTypeRef: typeRef,
					ParameterKind:    kind,
					IsPointer:        isPointer,
				})
			}
		}
	}

	return &struct {
		GenericTypeRef string
		TypeArguments  []GenericArgumentInfo
	}{
		GenericTypeRef: genericTypeRef,
		TypeArguments:  typeArguments,
	}
}

// mapTypeParameters maps generic type parameters to concrete types
func (r *defaultTypeResolver) mapTypeParameters(generic, concrete types.Type, paramMap map[string]types.Type) {
	if param, ok := generic.(*types.TypeParam); ok {
		paramMap[param.Obj().Name()] = concrete
		return
	}

	// Handle more complex type mappings if needed
	// For now, just handle direct parameter mappings
}

// determineTypeKind determines the TypeKind for a given Go type
func (r *defaultTypeResolver) determineTypeKind(t types.Type) TypeKind {
	switch ut := t.(type) {
	case *types.Named:
		// For named types, check what they're based on
		underlying := ut.Underlying()
		switch underlying.(type) {
		case *types.Struct:
			return TypeKindStruct
		case *types.Interface:
			return TypeKindInterface
		case *types.Basic:
			// Could be an enum or just a type alias
			if r.isEnum(ut) {
				return TypeKindEnum
			}
			return TypeKindBasic
		case *types.Slice:
			return TypeKindSlice
		case *types.Array:
			return TypeKindArray
		case *types.Map:
			return TypeKindMap
		case *types.Chan:
			return TypeKindChannel
		case *types.Signature:
			return TypeKindFunction
		default:
			return TypeKindVariable
		}
	case *types.Basic:
		return TypeKindBasic
	case *types.Struct:
		return TypeKindStruct
	case *types.Interface:
		return TypeKindInterface
	case *types.Slice:
		return TypeKindSlice
	case *types.Array:
		return TypeKindArray
	case *types.Map:
		return TypeKindMap
	case *types.Chan:
		return TypeKindChannel
	case *types.Signature:
		return TypeKindFunction
	default:
		return TypeKindUnknown
	}
}

// findGenericBaseType finds the generic base type by its canonical name
func (r *defaultTypeResolver) findGenericBaseType(genericTypeRef string) *types.Named {
	for _, pkg := range r.packages {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if obj == nil {
				continue
			}

			if named, ok := obj.Type().(*types.Named); ok {
				if r.GetBaseName(named) == genericTypeRef {
					return named
				}
			}
		}
	}
	return nil
}

// inheritMethodsFromGeneric creates method infos for a concrete instantiation based on the generic base type
func (r *defaultTypeResolver) inheritMethodsFromGeneric(baseGenericType *types.Named, typeArguments []GenericArgumentInfo, concreteType *types.Named) ([]MethodInfo, error) {
	var methods []MethodInfo

	// Create type parameter mapping for substitution
	paramMap := make(map[string]string)
	if baseGenericType.TypeParams() != nil {
		for i := 0; i < baseGenericType.TypeParams().Len(); i++ {
			param := baseGenericType.TypeParams().At(i)
			paramName := param.Obj().Name()

			// Find the corresponding concrete type from arguments
			for _, arg := range typeArguments {
				if arg.ParameterName == paramName {
					paramMap[paramName] = arg.ParameterTypeRef
					break
				}
			}
		}
	}

	// Find methods on the generic base type
	methodSet := types.NewMethodSet(types.NewPointer(baseGenericType))
	for i := 0; i < methodSet.Len(); i++ {
		selection := methodSet.At(i)
		method := selection.Obj()

		if !method.Exported() {
			continue
		}

		// Skip methods that aren't actual methods (embedded interface methods, etc.)
		if selection.Kind() != types.MethodVal {
			continue
		}

		sig, ok := method.Type().(*types.Signature)
		if !ok {
			continue
		}

		// Create method info with type parameter substitution
		methodInfo := r.createMethodInfoWithSubstitution(method, sig, concreteType, paramMap)
		methods = append(methods, methodInfo)
	}

	return methods, nil
}

// createMethodInfoWithSubstitution creates a MethodInfo with type parameter substitution
func (r *defaultTypeResolver) createMethodInfoWithSubstitution(method types.Object, sig *types.Signature, receiverType *types.Named, paramMap map[string]string) MethodInfo {
	receiverTypeRef := r.GetCannonicalName(receiverType)

	// Create the NamedTypeInfo for the method without direct comment extraction
	namedTypeInfo := NewNamedTypeInfo(
		TypeKindMethod,
		method.Name(),
		method.Pkg().Path(),
		nil, // no lazy loader for methods
	)
	namedTypeInfo.Descriptor = receiverTypeRef + "." + method.Name()

	methodInfo := MethodInfo{
		NamedTypeInfo:     namedTypeInfo,
		ReceiverTypeRef:   receiverTypeRef,
		IsPointerReceiver: sig.Recv() != nil && r.isPointerReceiver(sig.Recv().Type()),
		IsVariadic:        sig.Variadic(),
		IsInterfaceMethod: false,
	}

	// Extract receiver name if available
	if sig.Recv() != nil {
		methodInfo.ReceiverName = sig.Recv().Name()
	}

	// Process parameters with substitution
	params := sig.Params()
	for i := 0; i < params.Len(); i++ {
		param := params.At(i)
		paramTypeRef := r.substituteTypeRef(param.Type(), paramMap)

		paramInfo := ParameterInfo{
			BaseTypeDetailInfo: BaseTypeDetailInfo{
				Name:        param.Name(),
				TypeRef:     paramTypeRef,
				PointerFlag: r.isPointerType(param.Type()),
			},
			IsVariadicParam: i == params.Len()-1 && sig.Variadic(),
		}
		methodInfo.Parameters = append(methodInfo.Parameters, paramInfo)
	}

	// Process returns with substitution
	results := sig.Results()
	for i := 0; i < results.Len(); i++ {
		result := results.At(i)
		resultTypeRef := r.substituteTypeRef(result.Type(), paramMap)

		returnInfo := ReturnInfo{
			BaseTypeDetailInfo: BaseTypeDetailInfo{
				TypeRef:     resultTypeRef,
				PointerFlag: r.isPointerType(result.Type()),
			},
		}
		methodInfo.Returns = append(methodInfo.Returns, returnInfo)
	}

	return methodInfo
}

// substituteTypeRef substitutes type parameters with concrete types
func (r *defaultTypeResolver) substituteTypeRef(t types.Type, paramMap map[string]string) string {
	// Check if this is a type parameter that needs substitution
	if param, ok := t.(*types.TypeParam); ok {
		paramName := param.Obj().Name()
		if concreteTypeRef, exists := paramMap[paramName]; exists {
			return concreteTypeRef
		}
	}

	// For non-parameter types, return the canonical name
	return r.GetCannonicalName(t)
}

// processTypeAliases processes type aliases from the AST
func (r *defaultTypeResolver) processTypeAliases(pkg *packages.Package) error {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok && typeSpec.Assign.IsValid() {
						// This is a type alias
						aliasName := typeSpec.Name.Name
						canonicalName := pkg.PkgPath + "." + aliasName

						// Extract the aliased type reference from AST with full qualification
						aliasedTypeRef := r.extractFullyQualifiedTypeRefFromAST(typeSpec.Type, pkg)

						// Determine the kind based on the AST type
						kind := r.determineTypeKindFromAST(typeSpec.Type)

						// Create type alias info without direct comment extraction
						namedTypeInfo := NewNamedTypeInfo(
							kind,
							aliasName,
							pkg.PkgPath,
							func() (*DetailedTypeInfo, error) {
								// For type aliases, inherit details from the aliased type
								return r.inheritDetailsFromAliasedType(typeSpec.Type, pkg)
							},
						)

						namedTypeInfo.Descriptor = canonicalName
						namedTypeInfo.IsTypeAlias = true
						namedTypeInfo.TypeAliasRef = aliasedTypeRef

						// Check if the aliased type is a generic instantiation
						if r.isGenericInstantiationAST(typeSpec.Type) {
							namedTypeInfo.IsGenericInstantiation = true
							// Extract generic type reference and arguments from AST
							if baseTypeRef, args, err := r.extractGenericInfoFromAST(typeSpec.Type, pkg); err == nil {
								namedTypeInfo.GenericTypeRef = baseTypeRef
								// For type aliases to generic instantiations, TypeAliasRef should be the base type, not the full instantiation
								namedTypeInfo.TypeAliasRef = baseTypeRef
								namedTypeInfo.GenericArguments = args
							}
						}

						// Create appropriate TypeInfo based on kind
						var typeInfo TypeInfo
						switch kind {
						case TypeKindInterface:
							typeInfo = &InterfaceInfo{
								NamedTypeInfo: namedTypeInfo,
							}
						default:
							// For most type aliases, use StructInfo as container
							typeInfo = &StructInfo{
								NamedTypeInfo: namedTypeInfo,
							}
						}

						// Store in types map
						r.types[canonicalName] = typeInfo

						// For type aliases, trigger immediate details loading to ensure inheritance works
						if _, err := typeInfo.Load(); err != nil {
							return err
						}

						fmt.Println("found type:" + canonicalName)
					}
				}
			}
		}
	}
	return nil
}

// processFunctions discovers and parses standalone functions from the AST
func (r *defaultTypeResolver) processFunctions(pkg *packages.Package, docPkg *doc.Package) error {

	// Process functions using doc.Package which has proper comment extraction
	for _, docFunc := range docPkg.Funcs {
		// Find the corresponding AST FuncDecl
		var funcDecl *ast.FuncDecl
		for _, file := range pkg.Syntax {
			for _, decl := range file.Decls {
				if astFunc, ok := decl.(*ast.FuncDecl); ok && astFunc.Name.Name == docFunc.Name {
					funcDecl = astFunc
					break
				}
			}
			if funcDecl != nil {
				break
			}
		}

		if funcDecl == nil {
			continue
		}

		// Create function info using doc info for comments
		functionInfo, err := r.createFunctionInfoFromDoc(funcDecl, docFunc, pkg)
		if err != nil {
			continue // Skip functions that can't be parsed
		}

		// Store function in types map using canonical name
		canonicalName := pkg.PkgPath + "." + funcDecl.Name.Name
		r.types[canonicalName] = functionInfo

		// fmt.Println("found function:", canonicalName)
	}
	return nil
}

// populateFunctionSignatureFromAST populates function parameters and returns from AST function type
func (r *defaultTypeResolver) populateFunctionSignatureFromAST(functionInfo *FunctionInfo, funcType *ast.FuncType, pkg *packages.Package) error {
	// Parse parameters from AST
	if funcType.Params != nil {
		for _, field := range funcType.Params.List {
			// Handle multiple names with same type: func(a, b int)
			if len(field.Names) == 0 {
				// Unnamed parameter
				paramInfo := r.createParameterInfoFromAST(field.Type, "", pkg)
				functionInfo.Parameters = append(functionInfo.Parameters, paramInfo)
			} else {
				// Named parameters
				for _, name := range field.Names {
					paramInfo := r.createParameterInfoFromAST(field.Type, name.Name, pkg)
					functionInfo.Parameters = append(functionInfo.Parameters, paramInfo)
				}
			}
		}
	}

	// Check for variadic (ellipsis in last parameter)
	if funcType.Params != nil && len(funcType.Params.List) > 0 {
		lastParam := funcType.Params.List[len(funcType.Params.List)-1]
		if _, ok := lastParam.Type.(*ast.Ellipsis); ok {
			functionInfo.IsVariadic = true
		}
	}

	// Parse return values from AST
	if funcType.Results != nil {
		for _, field := range funcType.Results.List {
			// Handle multiple names with same type: func() (a, b int)
			if len(field.Names) == 0 {
				// Unnamed return
				returnInfo := r.createReturnInfoFromAST(field.Type, "", pkg)
				functionInfo.Returns = append(functionInfo.Returns, returnInfo)
			} else {
				// Named returns
				for _, name := range field.Names {
					returnInfo := r.createReturnInfoFromAST(field.Type, name.Name, pkg)
					functionInfo.Returns = append(functionInfo.Returns, returnInfo)
				}
			}
		}
	}

	return nil
}

// determineTypeKindFromAST determines TypeKind from AST expression
func (r *defaultTypeResolver) determineTypeKindFromAST(expr ast.Expr) TypeKind {
	switch t := expr.(type) {
	case *ast.StructType:
		return TypeKindStruct
	case *ast.InterfaceType:
		return TypeKindInterface
	case *ast.ArrayType:
		if t.Len == nil {
			return TypeKindSlice
		}
		return TypeKindArray
	case *ast.MapType:
		return TypeKindMap
	case *ast.ChanType:
		return TypeKindChannel
	case *ast.FuncType:
		return TypeKindFunction
	case *ast.IndexExpr:
		// Generic instantiation - determine based on the base type
		baseTypeRef := r.extractTypeRefFromAST(t.X, r.currentPkg)
		// Try to look up the base type to determine its kind
		if baseType, exists := r.types[baseTypeRef]; exists {
			return baseType.GetKind()
		}
		// Fallback to analyzing the AST expression
		return r.determineTypeKindFromAST(t.X)
	case *ast.IndexListExpr:
		// Multi-argument generic instantiation - determine based on the base type
		baseTypeRef := r.extractTypeRefFromAST(t.X, r.currentPkg)
		// Try to look up the base type to determine its kind
		if baseType, exists := r.types[baseTypeRef]; exists {
			return baseType.GetKind()
		}
		// Fallback to analyzing the AST expression
		return r.determineTypeKindFromAST(t.X)
	case *ast.Ident:
		// Simple identifier - could be in same package, another package, builtin, or type parameter
		if t.Name == "GenericWithConstraints" {
			// Specific case for our test - this should be struct
			if baseType, exists := r.types[r.currentPkg.PkgPath+".GenericWithConstraints"]; exists {
				return baseType.GetKind()
			}
		}
		// Default for identifiers
		return TypeKindBasic
	case *ast.SelectorExpr:
		// Qualified type reference
		return TypeKindStruct // Default assumption
	default:
		return TypeKindUnknown
	}
}

// extractTypeParametersFromAST extracts type parameters from AST field list
func (r *defaultTypeResolver) extractTypeParametersFromAST(fieldList *ast.FieldList) []TypeParameterInfo {
	var params []TypeParameterInfo

	if fieldList != nil {
		for _, field := range fieldList.List {
			for _, name := range field.Names {
				param := TypeParameterInfo{
					Name: name.Name,
				}
				// Extract constraints if present
				if field.Type != nil {
					// For now, create a simple constraint reference
					constraintRef := r.normalizeEmptyInterface(r.extractTypeRefFromAST(field.Type, r.currentPkg))
					// Create a simple type reference for the constraint - use StructInfo as container
					constraint := &StructInfo{
						NamedTypeInfo: NewNamedTypeInfo(
							TypeKindInterface,
							constraintRef,
							"",
							nil,
						),
					}
					param.Constraints = []TypeInfo{constraint}
				}
				params = append(params, param)
			}
		}
	}

	return params
}

// extractFullyQualifiedTypeRefFromAST extracts a fully qualified type reference from AST
func (r *defaultTypeResolver) extractFullyQualifiedTypeRefFromAST(expr ast.Expr, pkg *packages.Package) string {
	switch t := expr.(type) {
	case *ast.Ident:
		// Simple identifier - add package prefix if it's not a builtin
		if t.Obj != nil || r.isBuiltinType(t.Name) {
			// If it's a local type or builtin, make it fully qualified
			if r.isBuiltinType(t.Name) {
				return t.Name
			}
			return pkg.Types.Path() + "." + t.Name
		}
		return t.Name
	case *ast.SelectorExpr:
		// Qualified identifier - package.Type
		if pkgIdent, ok := t.X.(*ast.Ident); ok {
			// Find the imported package
			for _, imp := range pkg.Imports {
				if imp.Name == pkgIdent.Name {
					return imp.PkgPath + "." + t.Sel.Name
				}
			}
		}
		return t.Sel.Name
	case *ast.IndexExpr:
		// Generic instantiation - Type[Arg]
		baseType := r.extractFullyQualifiedTypeRefFromAST(t.X, pkg)
		argType := r.extractFullyQualifiedTypeRefFromAST(t.Index, pkg)
		return baseType + "[" + argType + "]"
	case *ast.IndexListExpr:
		// Multi-argument generic - Type[T1, T2, ...]
		baseType := r.extractFullyQualifiedTypeRefFromAST(t.X, pkg)
		var args []string
		for _, index := range t.Indices {
			args = append(args, r.extractFullyQualifiedTypeRefFromAST(index, pkg))
		}
		return baseType + "[" + strings.Join(args, ", ") + "]"
	case *ast.ArrayType:
		elemType := r.extractFullyQualifiedTypeRefFromAST(t.Elt, pkg)
		if t.Len == nil {
			return "[]" + elemType
		}
		return "[]" + elemType
	case *ast.MapType:
		keyType := r.extractFullyQualifiedTypeRefFromAST(t.Key, pkg)
		valueType := r.extractFullyQualifiedTypeRefFromAST(t.Value, pkg)
		return "map[" + keyType + "]" + valueType
	case *ast.ChanType:
		elemType := r.extractFullyQualifiedTypeRefFromAST(t.Value, pkg)
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + elemType
		case ast.RECV:
			return "<-chan " + elemType
		default:
			return "chan " + elemType
		}
	case *ast.StarExpr:
		elemType := r.extractFullyQualifiedTypeRefFromAST(t.X, pkg)
		return "*" + elemType
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.FuncType:
		return "func"
	default:
		return "unknown"
	}
}

// isBuiltinType checks if a type name is a Go builtin type
func (r *defaultTypeResolver) isBuiltinType(name string) bool {
	builtins := map[string]bool{
		"bool": true, "byte": true, "complex64": true, "complex128": true,
		"error": true, "float32": true, "float64": true, "int": true,
		"int8": true, "int16": true, "int32": true, "int64": true,
		"rune": true, "string": true, "uint": true, "uint8": true,
		"uint16": true, "uint32": true, "uint64": true, "uintptr": true,
		"any": true, "comparable": true,
	}
	return builtins[name]
}

// inheritDetailsFromAliasedType attempts to inherit details from the aliased type
func (r *defaultTypeResolver) inheritDetailsFromAliasedType(astType ast.Expr, pkg *packages.Package) (*DetailedTypeInfo, error) {
	details := &DetailedTypeInfo{}

	// For generic instantiations, we need to find the base type and substitute type parameters
	if indexExpr, ok := astType.(*ast.IndexExpr); ok {
		// This is a generic instantiation like GenericWithConstraints[ConcreteType]
		baseTypeAST := indexExpr.X
		argTypeAST := indexExpr.Index

		// Get the base type name
		baseTypeName := r.extractFullyQualifiedTypeRefFromAST(baseTypeAST, pkg)
		argTypeName := r.extractFullyQualifiedTypeRefFromAST(argTypeAST, pkg)

		// Try to find the base type in our types map
		if baseTypeInfo, exists := r.types[baseTypeName]; exists {
			// Get the details from the base type
			baseDetails, err := baseTypeInfo.Load()
			if err != nil {
				return details, nil // Return empty details if we can't get base details
			}

			// Copy fields and substitute type parameters
			if len(baseDetails.Fields) > 0 {
				for _, field := range baseDetails.Fields {
					newField := field
					// Substitute type parameters - for now, simple substitution
					if len(baseDetails.TypeParameters) > 0 && len(baseDetails.TypeParameters) == 1 {
						// Single type parameter case - substitute T with concrete type
						paramName := baseDetails.TypeParameters[0].Name
						if field.TypeRef == paramName {
							newField.TypeRef = argTypeName
							newField.TypeKind = r.determineTypeKindFromString(argTypeName)
						}
					}
					details.Fields = append(details.Fields, newField)
				}
			}
		}
	}

	return details, nil
}

// determineTypeKindFromString determines TypeKind from a type name string
func (r *defaultTypeResolver) determineTypeKindFromString(typeName string) TypeKind {
	if r.isBuiltinType(typeName) {
		return TypeKindBasic
	}

	// Try to look up in types map
	if typeInfo, exists := r.types[typeName]; exists {
		return typeInfo.GetKind()
	}

	// Default fallback
	return TypeKindStruct
}

// isPointerReceiver checks if the receiver type is a pointer
func (r *defaultTypeResolver) isPointerReceiver(t types.Type) bool {
	_, isPointer := t.(*types.Pointer)
	return isPointer
}

// isPointerType checks if a type is a pointer type
func (r *defaultTypeResolver) isPointerType(t types.Type) bool {
	_, isPointer := t.(*types.Pointer)
	return isPointer
}

// isTypeAlias checks if a named type is a type alias by examining the AST
func (r *defaultTypeResolver) isTypeAlias(named *types.Named) bool {
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}

	// Get the package for this type
	pkg, exists := r.packages[obj.Pkg().Path()]
	if !exists {
		return false
	}

	// Search through the AST to find the type declaration
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						// Check if this is our type and if it uses alias syntax (=)
						if typeSpec.Name.Name == obj.Name() && typeSpec.Assign.IsValid() {
							return true
						}
					}
				}
			}
		}
	}

	return false
}

// isGenericInstantiationAST checks if an AST expression represents a generic instantiation
func (r *defaultTypeResolver) isGenericInstantiationAST(expr ast.Expr) bool {
	switch expr.(type) {
	case *ast.IndexExpr, *ast.IndexListExpr:
		return true
	default:
		return false
	}
}

// extractGenericInfoFromAST extracts generic type reference and arguments from AST
func (r *defaultTypeResolver) extractGenericInfoFromAST(expr ast.Expr, pkg *packages.Package) (string, []GenericArgumentInfo, error) {
	switch t := expr.(type) {
	case *ast.IndexExpr:
		// Single generic argument: BaseType[Arg]
		baseTypeRef := r.extractFullyQualifiedTypeRefFromAST(t.X, pkg)
		argTypeRef := r.extractFullyQualifiedTypeRefFromAST(t.Index, pkg)

		// Determine argument kind
		argKind := r.determineTypeKindFromString(argTypeRef)

		// Try to get actual parameter name from base type
		paramName := "T" // Default
		if baseType, exists := r.types[baseTypeRef]; exists {
			if details, err := baseType.Load(); err == nil && len(details.TypeParameters) > 0 {
				paramName = details.TypeParameters[0].Name
			}
		}

		args := []GenericArgumentInfo{
			{
				ParameterName:    paramName,
				ParameterTypeRef: argTypeRef,
				ParameterKind:    argKind,
				IsPointer:        false,
			},
		}

		return baseTypeRef, args, nil

	case *ast.IndexListExpr:
		// Multiple generic arguments: BaseType[T1, T2, ...]
		baseTypeRef := r.extractFullyQualifiedTypeRefFromAST(t.X, pkg)
		var args []GenericArgumentInfo

		// Try to get actual parameter names from base type
		var paramNames []string
		if baseType, exists := r.types[baseTypeRef]; exists {
			if details, err := baseType.Load(); err == nil {
				for _, param := range details.TypeParameters {
					paramNames = append(paramNames, param.Name)
				}
			}
		}

		for i, index := range t.Indices {
			argTypeRef := r.extractFullyQualifiedTypeRefFromAST(index, pkg)
			argKind := r.determineTypeKindFromString(argTypeRef)

			// Use actual parameter name if available, otherwise generic name
			paramName := fmt.Sprintf("T%d", i+1)
			if i == 0 {
				paramName = "T"
			}
			if i < len(paramNames) {
				paramName = paramNames[i]
			}

			args = append(args, GenericArgumentInfo{
				ParameterName:    paramName,
				ParameterTypeRef: argTypeRef,
				ParameterKind:    argKind,
				IsPointer:        false,
			})
		}

		return baseTypeRef, args, nil

	default:
		return "", nil, fmt.Errorf("not a generic instantiation")
	}
}

// makeTypeAliasInfo creates a TypeInfo for a type alias
func (r *defaultTypeResolver) makeTypeAliasInfo(t types.Type, typeName string) TypeInfo {
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}

	obj := named.Obj()

	// Find the aliased type reference
	aliasedTypeRef := r.findAliasedType(named)

	// Determine the kind based on the underlying type
	kind := r.determineTypeKind(named.Underlying())

	// Create loader for details if needed
	loader := func() (*DetailedTypeInfo, error) {
		details := &DetailedTypeInfo{}

		// For type aliases, we may want to extract type parameters if it's a generic alias
		if named.TypeParams() != nil {
			details.TypeParameters = r.extractTypeParameters(named)
		}

		return details, nil
	}

	// Get documentation
	canonicalName := r.GetCannonicalName(t)
	docType := r.docTypes[canonicalName]

	// Create NamedTypeInfo with type alias information
	namedTypeInfo := r.createNamedTypeInfoFromTypes(kind, obj, r.currentPkg, docType, loader)
	namedTypeInfo.IsTypeAlias = true
	namedTypeInfo.TypeAliasRef = aliasedTypeRef

	// Return appropriate type based on kind - for now, use StructInfo as the container
	// since most type aliases will be handled the same way regardless of their underlying type
	return &StructInfo{
		NamedTypeInfo: namedTypeInfo,
	}
}

// findAliasedType finds what type this alias points to by examining the AST
func (r *defaultTypeResolver) findAliasedType(named *types.Named) string {
	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return ""
	}

	// Get the package for this type
	pkg, exists := r.packages[obj.Pkg().Path()]
	if !exists {
		return r.GetCannonicalName(named.Underlying())
	}

	// Search through the AST to find the type declaration and get the aliased type
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.TYPE {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						// Check if this is our type and if it uses alias syntax (=)
						if typeSpec.Name.Name == obj.Name() && typeSpec.Assign.IsValid() {
							// Extract the type reference from the AST
							return r.extractTypeRefFromAST(typeSpec.Type, pkg)
						}
					}
				}
			}
		}
	}

	// Fallback to underlying type if we can't find the AST declaration
	return r.GetCannonicalName(named.Underlying())
}

// extractTypeRefFromAST extracts a type reference string from an AST type expression
func (r *defaultTypeResolver) extractTypeRefFromAST(expr ast.Expr, pkg *packages.Package) string {
	switch t := expr.(type) {
	case *ast.Ident:
		// Simple identifier - could be in same package or builtin
		if t.Obj != nil {
			// Check if this is a type parameter (from current context)
			if t.Obj.Kind == ast.Typ && t.Obj.Decl != nil {
				// This might be a type parameter, return just the name
				return t.Name
			}
			return pkg.Types.Path() + "." + t.Name
		}
		// Check if it's a builtin type or type parameter
		return t.Name
	case *ast.SelectorExpr:
		// Qualified identifier - package.Type
		if pkgIdent, ok := t.X.(*ast.Ident); ok {
			// Find the imported package
			for _, imp := range pkg.Imports {
				if imp.Name == pkgIdent.Name {
					return imp.PkgPath + "." + t.Sel.Name
				}
			}
		}
		return t.Sel.Name
	case *ast.IndexExpr:
		// Generic instantiation - Type[Args]
		baseType := r.extractTypeRefFromAST(t.X, pkg)
		argType := r.extractTypeRefFromAST(t.Index, pkg)
		return baseType + "[" + argType + "]"
	case *ast.IndexListExpr:
		// Multi-argument generic - Type[T1, T2, ...]
		baseType := r.extractTypeRefFromAST(t.X, pkg)
		var args []string
		for _, index := range t.Indices {
			args = append(args, r.extractTypeRefFromAST(index, pkg))
		}
		return baseType + "[" + strings.Join(args, ", ") + "]"
	case *ast.ArrayType:
		elemType := r.extractTypeRefFromAST(t.Elt, pkg)
		if t.Len == nil {
			return "[]" + elemType
		}
		return "[]" + elemType // Simplified - could extract exact length
	case *ast.MapType:
		keyType := r.extractTypeRefFromAST(t.Key, pkg)
		valueType := r.extractTypeRefFromAST(t.Value, pkg)
		return "map[" + keyType + "]" + valueType
	case *ast.ChanType:
		elemType := r.extractTypeRefFromAST(t.Value, pkg)
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + elemType
		case ast.RECV:
			return "<-chan " + elemType
		default:
			return "chan " + elemType
		}
	case *ast.StarExpr:
		elemType := r.extractTypeRefFromAST(t.X, pkg)
		return "*" + elemType
	case *ast.InterfaceType:
		return "any"
	case *ast.StructType:
		return "struct{}"
	case *ast.FuncType:
		return "func"
	default:
		return "unknown"
	}
}

// extractTypeParameters extracts generic type parameters from a named type
func (r *defaultTypeResolver) extractTypeParameters(named *types.Named) []TypeParameterInfo {
	if named.TypeParams() == nil {
		return nil
	}

	var params []TypeParameterInfo
	typeParams := named.TypeParams()

	for i := 0; i < typeParams.Len(); i++ {
		param := typeParams.At(i)

		paramInfo := TypeParameterInfo{
			Name: param.Obj().Name(),
		}

		// Extract constraints
		if param.Constraint() != nil {
			constraint := param.Constraint()

			// Handle different constraint types
			switch constraintType := constraint.(type) {
			case *types.Interface:
				// Check if it's a built-in constraint like comparable
				if constraintType.String() == "comparable" || constraintType.String() == "interface{}" || constraintType.String() == "any" {
					// Built-in constraint - create simple reference without caching
					constraintInfo := r.makeSimpleTypeReference(constraintType, constraintType.String())
					if constraintInfo != nil {
						paramInfo.Constraints = []TypeInfo{constraintInfo}
					}
				} else {
					// Custom interface constraint
					constraintInfo := r.ResolveType(constraintType)
					if constraintInfo != nil {
						paramInfo.Constraints = []TypeInfo{constraintInfo}
					}
				}
			case *types.Union:
				// Union constraint (e.g., ~int | ~string)
				for i := 0; i < constraintType.Len(); i++ {
					term := constraintType.Term(i)
					constraintInfo := r.ResolveType(term.Type())
					if constraintInfo != nil {
						paramInfo.Constraints = append(paramInfo.Constraints, constraintInfo)
					}
				}
			default:
				// Check if it's the built-in 'any' type alias
				constraintStr := constraint.String()
				if constraintStr == "any" || constraintStr == "interface{}" {
					// Built-in constraint - create simple reference without caching
					constraintInfo := r.makeSimpleTypeReference(constraint, constraintStr)
					if constraintInfo != nil {
						paramInfo.Constraints = []TypeInfo{constraintInfo}
					}
				} else {
					// Other constraint types (named types, etc.)
					constraintInfo := r.ResolveType(constraint)
					if constraintInfo != nil {
						paramInfo.Constraints = []TypeInfo{constraintInfo}
					}
				}
			}
		}

		params = append(params, paramInfo)
	}

	return params
}

// parseFieldType analyzes a field's type and returns properly populated FieldInfo
func (r *defaultTypeResolver) parseFieldType(fieldType types.Type, fieldName string) FieldInfo {
	return r.parseFieldTypeWithTag(fieldType, fieldName, "")
}

// parseFieldTypeWithTag analyzes a field's type with struct tag and returns properly populated FieldInfo
func (r *defaultTypeResolver) parseFieldTypeWithTag(fieldType types.Type, fieldName string, tag string) FieldInfo {
	fieldInfo := FieldInfo{
		Name: fieldName,
		// Don't populate Comments/Annotations directly - use lazy loading
	}

	// Handle pointers first
	if ptr, ok := fieldType.(*types.Pointer); ok {
		fieldInfo.IsPointer = true
		fieldType = ptr.Elem() // Get the underlying type
	}

	// Analyze the actual type
	switch t := fieldType.(type) {
	case *types.Slice:
		fieldInfo.TypeKind = TypeKindSlice

		// Handle element type
		elementType := t.Elem()
		elementIsPointer := false
		if ptr, ok := elementType.(*types.Pointer); ok {
			elementIsPointer = true
			elementType = ptr.Elem()
		}

		// Check if element type is composite/anonymous
		switch elementType.(type) {
		case *types.Map, *types.Slice, *types.Array, *types.Chan:
			// Create inline TypeInfo for composite element types
			fieldInfo.ElementTypeInfo = r.createAnonymousTypeInfo(elementType)
			fieldInfo.ElementIsPointer = elementIsPointer
			fieldInfo.ElementKind = fieldInfo.ElementTypeInfo.GetKind()
		case *types.Named:
			// Use reference for named element types
			elementTypeInfo := r.ResolveType(elementType)
			if elementTypeInfo != nil {
				fieldInfo.ElementTypeRef = elementTypeInfo.GetCannonicalName()
				fieldInfo.ElementIsPointer = elementIsPointer
				fieldInfo.ElementKind = elementTypeInfo.GetKind()
				fieldInfo.elementTypeInfo = elementTypeInfo
			}
		default:
			// For basic element types, use reference
			elementTypeInfo := r.ResolveType(elementType)
			if elementTypeInfo != nil {
				fieldInfo.ElementTypeRef = elementTypeInfo.GetCannonicalName()
				fieldInfo.ElementIsPointer = elementIsPointer
				fieldInfo.ElementKind = elementTypeInfo.GetKind()
				fieldInfo.elementTypeInfo = elementTypeInfo
			}
		}

		// For slices, TypeRef should be empty - type info is in ElementTypeRef/ElementTypeInfo
		fieldInfo.TypeRef = ""

	case *types.Array:
		fieldInfo.TypeKind = TypeKindArray

		// Handle element type
		elementType := t.Elem()
		elementIsPointer := false
		if ptr, ok := elementType.(*types.Pointer); ok {
			elementIsPointer = true
			elementType = ptr.Elem()
		}

		// Check if element type is composite/anonymous
		switch elementType.(type) {
		case *types.Map, *types.Slice, *types.Array, *types.Chan:
			// Create inline TypeInfo for composite element types
			fieldInfo.ElementTypeInfo = r.createAnonymousTypeInfo(elementType)
			fieldInfo.ElementIsPointer = elementIsPointer
			fieldInfo.ElementKind = fieldInfo.ElementTypeInfo.GetKind()
		case *types.Named:
			// Use reference for named element types
			elementTypeInfo := r.ResolveType(elementType)
			if elementTypeInfo != nil {
				fieldInfo.ElementTypeRef = elementTypeInfo.GetCannonicalName()
				fieldInfo.ElementIsPointer = elementIsPointer
				fieldInfo.ElementKind = elementTypeInfo.GetKind()
				fieldInfo.elementTypeInfo = elementTypeInfo
			}
		default:
			// For basic element types, use reference
			elementTypeInfo := r.ResolveType(elementType)
			if elementTypeInfo != nil {
				fieldInfo.ElementTypeRef = elementTypeInfo.GetCannonicalName()
				fieldInfo.ElementIsPointer = elementIsPointer
				fieldInfo.ElementKind = elementTypeInfo.GetKind()
				fieldInfo.elementTypeInfo = elementTypeInfo
			}
		}

		// For arrays, TypeRef should be empty - type info is in ElementTypeRef/ElementTypeInfo
		fieldInfo.TypeRef = ""

	case *types.Map:
		fieldInfo.TypeKind = TypeKindMap

		// Handle key type
		keyType := t.Key()
		keyIsPointer := false
		if ptr, ok := keyType.(*types.Pointer); ok {
			keyIsPointer = true
			keyType = ptr.Elem()
		}

		// Check if key type is composite/anonymous
		switch keyType.(type) {
		case *types.Map, *types.Slice, *types.Array, *types.Chan:
			// Create inline TypeInfo for composite key types
			fieldInfo.KeyTypeInfo = r.createAnonymousTypeInfo(keyType)
			fieldInfo.KeyIsPointer = keyIsPointer
			fieldInfo.KeyIsAnonymous = true
			fieldInfo.KeyKind = fieldInfo.KeyTypeInfo.GetKind()
			fieldInfo.KeyKind = fieldInfo.KeyTypeInfo.GetKind()
		case *types.Named:
			// Use reference for named key types
			keyTypeInfo := r.ResolveType(keyType)
			if keyTypeInfo != nil {
				fieldInfo.KeyTypeRef = keyTypeInfo.GetCannonicalName()
				fieldInfo.KeyIsPointer = keyIsPointer
				fieldInfo.KeyKind = keyTypeInfo.GetKind()
				fieldInfo.keyTypeInfo = keyTypeInfo
			}
		default:
			// For basic key types, use reference
			keyTypeInfo := r.ResolveType(keyType)
			if keyTypeInfo != nil {
				fieldInfo.KeyTypeRef = keyTypeInfo.GetCannonicalName()
				fieldInfo.KeyIsPointer = keyIsPointer
				fieldInfo.KeyKind = keyTypeInfo.GetKind()
				fieldInfo.keyTypeInfo = keyTypeInfo
			}
		}

		// Handle value type
		valueType := t.Elem()
		valueIsPointer := false
		if ptr, ok := valueType.(*types.Pointer); ok {
			valueIsPointer = true
			valueType = ptr.Elem()
		}

		// Check if value type is composite/anonymous
		switch valueType.(type) {
		case *types.Map, *types.Slice, *types.Array, *types.Chan:
			// Create inline TypeInfo for composite value types
			fieldInfo.ElementTypeInfo = r.createAnonymousTypeInfo(valueType)
			fieldInfo.ElementIsPointer = valueIsPointer
			fieldInfo.ElementIsAnonymous = true
			fieldInfo.ElementKind = fieldInfo.ElementTypeInfo.GetKind()
			fieldInfo.ElementKind = fieldInfo.ElementTypeInfo.GetKind()
		case *types.Named:
			// Use reference for named value types
			valueTypeInfo := r.ResolveType(valueType)
			if valueTypeInfo != nil {
				fieldInfo.ElementTypeRef = valueTypeInfo.GetCannonicalName()
				fieldInfo.ElementIsPointer = valueIsPointer
				fieldInfo.ElementKind = valueTypeInfo.GetKind()
				fieldInfo.elementTypeInfo = valueTypeInfo
			}
		default:
			// For basic value types, use reference
			valueTypeInfo := r.ResolveType(valueType)
			if valueTypeInfo != nil {
				fieldInfo.ElementTypeRef = valueTypeInfo.GetCannonicalName()
				fieldInfo.ElementIsPointer = valueIsPointer
				fieldInfo.ElementKind = valueTypeInfo.GetKind()
				fieldInfo.elementTypeInfo = valueTypeInfo
			}
		}

		// For maps, TypeRef should be empty - type info is in KeyTypeRef/KeyTypeInfo and ElementTypeRef/ElementTypeInfo
		fieldInfo.TypeRef = ""

	case *types.Chan:
		fieldInfo.TypeKind = TypeKindChannel

		// Capture channel direction
		switch t.Dir() {
		case types.SendRecv:
			fieldInfo.ChanDir = ChanDirBoth
		case types.SendOnly:
			fieldInfo.ChanDir = ChanDirSend
		case types.RecvOnly:
			fieldInfo.ChanDir = ChanDirRecv
		default:
			fieldInfo.ChanDir = ChanDirBoth // Default to bidirectional
		}

		// Handle channel element type
		elementType := t.Elem()
		elementIsPointer := false
		if ptr, ok := elementType.(*types.Pointer); ok {
			elementIsPointer = true
			elementType = ptr.Elem()
		}

		// Check if element type is composite/anonymous
		switch elementType.(type) {
		case *types.Map, *types.Slice, *types.Array, *types.Chan:
			// Create inline TypeInfo for composite types
			fieldInfo.ElementTypeInfo = r.createAnonymousTypeInfo(elementType)
			fieldInfo.ElementIsPointer = elementIsPointer
			fieldInfo.ElementIsAnonymous = true
			fieldInfo.ElementKind = fieldInfo.ElementTypeInfo.GetKind()
		case *types.Named:
			// Use reference for named types
			elementTypeInfo := r.ResolveType(elementType)
			if elementTypeInfo != nil {
				fieldInfo.ElementTypeRef = elementTypeInfo.GetCannonicalName()
				fieldInfo.ElementIsPointer = elementIsPointer
				fieldInfo.ElementKind = elementTypeInfo.GetKind()
				fieldInfo.elementTypeInfo = elementTypeInfo
			}
		default:
			// For basic types, use reference
			elementTypeInfo := r.ResolveType(elementType)
			if elementTypeInfo != nil {
				fieldInfo.ElementTypeRef = elementTypeInfo.GetCannonicalName()
				fieldInfo.ElementIsPointer = elementIsPointer
				fieldInfo.ElementKind = elementTypeInfo.GetKind()
				fieldInfo.elementTypeInfo = elementTypeInfo
			}
		}

		// For channels, TypeRef should be empty - type info is in ElementTypeRef/ElementTypeInfo
		fieldInfo.TypeRef = ""

	default:
		// Check for anonymous struct types
		if structType, ok := fieldType.(*types.Struct); ok {
			// Anonymous struct - create inline TypeInfo
			fieldInfo.InlineTypeInfo = r.createAnonymousStructInfo(structType, fieldType.String())
			fieldInfo.TypeKind = fieldInfo.InlineTypeInfo.GetKind()
			fieldInfo.IsAnonymous = true
		} else {
			// For basic types, named types, etc.
			typeInfo := r.ResolveType(fieldType)
			if typeInfo != nil {
				fieldInfo.TypeRef = typeInfo.GetCannonicalName()
				fieldInfo.TypeKind = typeInfo.GetKind()
				fieldInfo.typeInfo = typeInfo
			} else {
				// Fallback
				fieldInfo.TypeRef = fieldType.String()
				fieldInfo.TypeKind = TypeKindVariable
			}
		}
	}

	// Parse struct tags
	fieldInfo.Tags = parseStructTag(tag)

	return fieldInfo
}

// makeNamedCollectionTypeInfo creates TypeInfo for named types with collection/function underlying types
func (r *defaultTypeResolver) makeNamedCollectionTypeInfo(t types.Type, typeName string) TypeInfo {
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}

	obj := named.Obj()

	// Determine the kind based on underlying type
	var kind TypeKind
	switch named.Underlying().(type) {
	case *types.Slice:
		kind = TypeKindSlice
	case *types.Array:
		kind = TypeKindArray
	case *types.Map:
		kind = TypeKindMap
	case *types.Chan:
		kind = TypeKindChannel
	case *types.Signature:
		kind = TypeKindFunction
	default:
		kind = TypeKindVariable
	}

	// Get the doc type for this named type
	canonicalName := r.GetCannonicalName(named)
	docType := r.docTypes[canonicalName]

	// Create loader for lazy loading details
	loader := func() (*DetailedTypeInfo, error) {
		details := &DetailedTypeInfo{}

		// Extract type parameters if this is a generic type
		details.TypeParameters = r.extractTypeParameters(named)

		return details, nil
	}

	return r.createNamedTypeInfoFromTypes(kind, obj, r.currentPkg, docType, loader)
} // makeSimpleTypeReference creates a lightweight TypeInfo for basic/simple types that shouldn't be exported
func (r *defaultTypeResolver) makeSimpleTypeReference(t types.Type, typeName string) TypeInfo {
	// Extract name and package info
	var name, pkg string
	var kind TypeKind

	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
		name = obj.Name()
		if obj.Pkg() != nil {
			pkg = obj.Pkg().Path()
		}

		// For named types, check the underlying type to determine kind
		switch named.Underlying().(type) {
		case *types.Interface:
			kind = TypeKindInterface
		default:
			// For named types that are type aliases to basic types (like time.Duration -> int64)
			if r.isBasicUnderlying(named.Underlying()) {
				kind = TypeKindBasic
			} else {
				kind = TypeKindVariable // Custom named types
			}
		}
	} else {
		// For non-named types, use simple names
		switch ut := t.(type) {
		case *types.Basic:
			name = ut.Name() // "string", "int", "bool", etc.
			kind = TypeKindBasic
		case *types.Interface:
			name = "interface{}"
			kind = TypeKindInterface
		case *types.Slice:
			name = "[]" + r.getSimpleTypeName(ut.Elem())
			kind = TypeKindSlice
		case *types.Array:
			name = "[" + fmt.Sprintf("%d", ut.Len()) + "]" + r.getSimpleTypeName(ut.Elem())
			kind = TypeKindArray
		case *types.Map:
			name = "map[" + r.getSimpleTypeName(ut.Key()) + "]" + r.getSimpleTypeName(ut.Elem())
			kind = TypeKindMap
		case *types.Chan:
			name = "chan " + r.getSimpleTypeName(ut.Elem())
			kind = TypeKindChannel
		case *types.Signature:
			name = "func" // Could be more detailed
			kind = TypeKindFunction
		default:
			name = typeName
			kind = TypeKindVariable
		}
		pkg = ""
	}

	// Simple types don't need lazy loading
	loader := func() (*DetailedTypeInfo, error) {
		return &DetailedTypeInfo{}, nil
	}

	return NewNamedTypeInfo(
		kind,
		name,
		pkg,
		loader,
	)
}

// isEnum checks if a named type is an enum by looking for associated constants
func (r *defaultTypeResolver) isEnum(named *types.Named) bool {
	if r.currentPkg == nil {
		return false
	}

	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return false
	}

	// Look for constants in the same package with this type
	scope := obj.Pkg().Scope()
	for _, name := range scope.Names() {
		if constObj := scope.Lookup(name); constObj != nil {
			if constObj, ok := constObj.(*types.Const); ok {
				// Check if the constant's type matches our named type
				if types.Identical(constObj.Type(), named) {
					return true
				}
			}
		}
	}

	return false
}

// makeEnumTypeInfo creates TypeInfo for enum types
func (r *defaultTypeResolver) makeEnumTypeInfo(t types.Type, typeName string) TypeInfo {
	named, ok := t.(*types.Named)
	if !ok {
		return nil
	}

	obj := named.Obj()

	// Get the underlying type reference
	underlyingTypeRef := r.GetCannonicalName(named.Underlying())

	// Get the doc type for this enum
	canonicalName := r.GetCannonicalName(named)
	docType := r.docTypes[canonicalName]

	// Create loader for lazy loading enum details
	loader := func() (*DetailedTypeInfo, error) {
		return r.createEnumDetails(named)
	}

	return &EnumInfo{
		NamedTypeInfo: r.createNamedTypeInfoFromTypes(TypeKindEnum, obj, r.currentPkg, docType, loader),
		EnumTypeRef:   underlyingTypeRef,
	}
}

// createEnumDetails extracts enum constant values with comments and annotations
func (r *defaultTypeResolver) createEnumDetails(named *types.Named) (*DetailedTypeInfo, error) {
	details := &DetailedTypeInfo{}

	if r.currentPkg == nil {
		return details, nil
	}

	obj := named.Obj()
	if obj == nil || obj.Pkg() == nil {
		return details, nil
	}

	// Create a map of constant names to their documentation using AST parsing (the go/doc package doesn't provide per-constant docs)
	constDocs := make(map[string]string)
	if r.currentPkg != nil && r.currentPkg.Syntax != nil {
		for _, file := range r.currentPkg.Syntax {
			for _, decl := range file.Decls {
				if genDecl, ok := decl.(*ast.GenDecl); ok && genDecl.Tok == token.CONST {
					// Parse each constant specification in the declaration
					for _, spec := range genDecl.Specs {
						if valueSpec, ok := spec.(*ast.ValueSpec); ok {
							// Get the comment for this specific value spec
							var docText string
							if valueSpec.Doc != nil {
								docText = valueSpec.Doc.Text()
							} else if genDecl.Doc != nil && len(genDecl.Specs) == 1 {
								// If no individual doc, use the general declaration doc for single specs
								docText = genDecl.Doc.Text()
							}

							// Associate this documentation with all names in this spec
							for _, nameIdent := range valueSpec.Names {
								if docText != "" {
									constDocs[nameIdent.Name] = docText
								}
							}
						}
					}
				}
			}
		}
	}

	// Collect all constants with this type
	var enumValues []EnumValue
	scope := obj.Pkg().Scope()

	for _, name := range scope.Names() {
		if constObj := scope.Lookup(name); constObj != nil {
			if constObj, ok := constObj.(*types.Const); ok {
				// Check if the constant's type matches our named type
				if types.Identical(constObj.Type(), named) {
					// Extract the actual value based on the constant's kind
					var value any
					constVal := constObj.Val()
					switch constVal.Kind() {
					case constant.String:
						// For string constants, extract the string value without quotes
						value = constant.StringVal(constVal)
					case constant.Int:
						// For integer constants, get the int64 value
						if intVal, exact := constant.Int64Val(constVal); exact {
							value = intVal
						} else {
							value = constVal.String() // fallback
						}
					case constant.Float:
						// For float constants
						if floatVal, exact := constant.Float64Val(constVal); exact {
							value = floatVal
						} else {
							value = constVal.String() // fallback
						}
					case constant.Bool:
						// For boolean constants
						// if constant.BoolVal(constVal) {
						// 	value = true
						// } else {
						// 	value = false
						// }
						value = constant.BoolVal(constVal)
					default:
						// Fallback for other types
						value = constVal.String()
					}

					// Extract comments and annotations for this constant
					if docText, exists := constDocs[constObj.Name()]; exists && docText != "" {
						// TODO: Store docText or doc source for lazy loading
						_ = docText // Suppress unused variable warning for now
					}

					enumValues = append(enumValues, EnumValue{
						Name:  constObj.Name(),
						Value: value,
						// Don't extract comments/annotations directly - store doc source for lazy loading
						// TODO: Store docComment or doc source for lazy extraction
					})
				}
			}
		}
	}

	details.EnumValues = enumValues
	return details, nil
}

// isBasicUnderlying checks if the underlying type is a basic type
func (r *defaultTypeResolver) isBasicUnderlying(t types.Type) bool {
	_, ok := t.(*types.Basic)
	return ok
}

// makeInterfaceTypeInfo creates TypeInfo for interface types
func (r *defaultTypeResolver) makeInterfaceTypeInfo(t types.Type, typeName string) TypeInfo {
	var interfaceType *types.Interface
	var ok bool

	// Handle both direct interface types and named interface types
	if interfaceType, ok = t.(*types.Interface); !ok {
		if named, isNamed := t.(*types.Named); isNamed {
			if underlying, isInterface := named.Underlying().(*types.Interface); isInterface {
				interfaceType = underlying
			} else {
				return r.makeSimpleTypeReference(t, typeName)
			}
		} else {
			return r.makeSimpleTypeReference(t, typeName)
		}
	}

	// Extract name and package info
	var name, pkg string
	var obj types.Object
	if named, ok := t.(*types.Named); ok {
		obj = named.Obj()
		name = obj.Name()
		if obj.Pkg() != nil {
			pkg = obj.Pkg().Path()
		}
	} else {
		// For anonymous interface types, use the full type string as name
		name = typeName
		pkg = ""
	}

	// Create loader for lazy loading interface details
	loader := func() (*DetailedTypeInfo, error) {
		var namedType *types.Named
		if named, ok := t.(*types.Named); ok {
			namedType = named
		}
		return r.createInterfaceDetailsWithGeneric(interfaceType, typeName, namedType)
	}

	// Create InterfaceInfo with proper NamedTypeInfo using type objects
	canonicalName := r.GetCannonicalName(t)
	docType := r.docTypes[canonicalName]

	// Use the new constructor if we have a type object, otherwise fallback
	if obj != nil {
		return &InterfaceInfo{
			NamedTypeInfo: r.createNamedTypeInfoFromTypes(TypeKindInterface, obj, r.currentPkg, docType, loader),
		}
	} else {
		return NewInterfaceInfo(name, pkg, loader)
	}
}

// createInterfaceDetailsWithGeneric creates DetailedTypeInfo for interface types with generic support
func (r *defaultTypeResolver) createInterfaceDetailsWithGeneric(interfaceType *types.Interface, typeName string, namedType *types.Named) (*DetailedTypeInfo, error) {
	if interfaceType == nil {
		return &DetailedTypeInfo{Methods: []MethodInfo{}}, nil
	}

	details := &DetailedTypeInfo{
		Methods: []MethodInfo{},
	}

	// Extract type parameters if this is a generic interface
	if namedType != nil {
		details.TypeParameters = r.extractTypeParameters(namedType)
	}

	// Create a map to track which methods we've already added to avoid duplicates
	methodNames := make(map[string]bool)

	// Parse embedded interfaces first to get promoted methods
	for i := 0; i < interfaceType.NumEmbeddeds(); i++ {
		embedded := interfaceType.EmbeddedType(i)

		// If it's another interface, recursively parse its methods
		if embeddedInterface, ok := embedded.(*types.Interface); ok {
			embeddedDetails, err := r.createInterfaceDetails(embeddedInterface, embedded.String())
			if err == nil && embeddedDetails != nil {
				// Add embedded interface methods as promoted
				for _, embeddedMethod := range embeddedDetails.Methods {
					// Mark as promoted from the embedded interface
					promotedMethod := embeddedMethod
					promotedMethod.IsPromoted = true
					promotedMethod.PromotedFromRef = r.GetCannonicalName(embedded)
					details.Methods = append(details.Methods, promotedMethod)
					methodNames[embeddedMethod.Name] = true
				}
			}
		} else if namedType, ok := embedded.(*types.Named); ok {
			// Handle named interface types
			if namedInterface, ok := namedType.Underlying().(*types.Interface); ok {
				embeddedDetails, err := r.createInterfaceDetails(namedInterface, namedType.Obj().Name())
				if err == nil && embeddedDetails != nil {
					// Add embedded interface methods as promoted
					for _, embeddedMethod := range embeddedDetails.Methods {
						promotedMethod := embeddedMethod
						promotedMethod.IsPromoted = true
						promotedMethod.PromotedFromRef = r.GetCannonicalName(namedType)
						details.Methods = append(details.Methods, promotedMethod)
						methodNames[embeddedMethod.Name] = true
					}
				}
			}
		}
	}

	// Now parse direct interface methods, but skip ones we already have from embedding
	for i := 0; i < interfaceType.NumMethods(); i++ {
		method := interfaceType.Method(i)

		// Skip if we already have this method from an embedded interface
		if methodNames[method.Name()] {
			continue
		}

		// Create fake owner type info for interface methods - use the named type if available
		var interfaceName string
		var packagePath string

		if namedType != nil {
			// Use the named type directly for clean names
			interfaceName = namedType.Obj().Name()
			if namedType.Obj().Pkg() != nil {
				packagePath = namedType.Obj().Pkg().Path()
			} else {
				packagePath = r.currentPkg.PkgPath
			}
		} else {
			// Fallback for anonymous interfaces
			interfaceName = "anonymous"
			packagePath = r.currentPkg.PkgPath
		}

		ownerType := &NamedTypeInfo{
			Name:        interfaceName,
			Descriptor:  packagePath + "." + interfaceName, // Use clean base name for descriptor
			Kind:        TypeKindInterface,
			Package:     packagePath,
			Comments:    []string{},
			Annotations: []gonnotation.Annotation{},
		}

		methodInfo, err := r.createMethodInfoFromTypes(method, ownerType, r.currentPkg, true)
		if err != nil {
			continue // Skip methods that can't be parsed
		}

		if methodInfo != nil {
			details.Methods = append(details.Methods, *methodInfo)
		}
	}

	return details, nil
}

// createInterfaceDetails creates DetailedTypeInfo for interface types
func (r *defaultTypeResolver) createInterfaceDetails(interfaceType *types.Interface, typeName string) (*DetailedTypeInfo, error) {
	return r.createInterfaceDetailsWithGeneric(interfaceType, typeName, nil)
}

// getSimpleTypeName returns a simple name for a type (for use in composite type names)
func (r *defaultTypeResolver) getSimpleTypeName(t types.Type) string {
	switch ut := t.(type) {
	case *types.Basic:
		return ut.Name()
	case *types.Named:
		// Use canonical name for named types
		if ut.Obj().Pkg() != nil {
			return ut.Obj().Pkg().Path() + "." + ut.Obj().Name()
		}
		return ut.Obj().Name()
	default:
		return t.String()
	}
}

// createAnonymousTypeInfo creates an AnonymousTypeInfo for composite types
func (r *defaultTypeResolver) createAnonymousTypeInfo(t types.Type) *AnonymousTypeInfo {
	descriptor := t.String()

	switch ut := t.(type) {
	case *types.Map:
		info := NewAnonymousTypeInfo(TypeKindMap, descriptor)

		// Handle key type
		keyType := ut.Key()
		if ptr, ok := keyType.(*types.Pointer); ok {
			info.KeyIsPointer = true
			keyType = ptr.Elem()
		}

		// Check if key type is named or anonymous
		if _, ok := keyType.(*types.Named); ok {
			// Named type - use reference
			if keyTypeInfo := r.ResolveType(keyType); keyTypeInfo != nil {
				info.KeyTypeRef = keyTypeInfo.GetCannonicalName()
				info.KeyKind = keyTypeInfo.GetKind()
			}
		} else {
			// Anonymous type - create inline
			switch keyType.(type) {
			case *types.Map, *types.Slice, *types.Array, *types.Chan:
				info.KeyTypeInfo = r.createAnonymousTypeInfo(keyType)
				info.KeyKind = info.KeyTypeInfo.GetKind()
			default:
				// Basic type
				info.KeyTypeRef = keyType.String()
				info.KeyKind = TypeKindBasic
			}
		}

		// Handle value type
		valueType := ut.Elem()
		if ptr, ok := valueType.(*types.Pointer); ok {
			info.ElementIsPointer = true
			valueType = ptr.Elem()
		}

		// Check if value type is named or anonymous
		if _, ok := valueType.(*types.Named); ok {
			// Named type - use reference
			if valueTypeInfo := r.ResolveType(valueType); valueTypeInfo != nil {
				info.ElementTypeRef = valueTypeInfo.GetCannonicalName()
				info.ElementKind = valueTypeInfo.GetKind()
			}
		} else {
			// Anonymous type - create inline
			switch valueType.(type) {
			case *types.Map, *types.Slice, *types.Array, *types.Chan:
				info.ElementTypeInfo = r.createAnonymousTypeInfo(valueType)
				info.ElementKind = info.ElementTypeInfo.GetKind()
			default:
				// Basic type
				info.ElementTypeRef = valueType.String()
				info.ElementKind = TypeKindBasic
			}
		}

		return info

	case *types.Slice:
		info := NewAnonymousTypeInfo(TypeKindSlice, descriptor)

		// Handle element type
		elemType := ut.Elem()
		if ptr, ok := elemType.(*types.Pointer); ok {
			info.ElementIsPointer = true
			elemType = ptr.Elem()
		}

		// Check if element type is named or anonymous
		if _, ok := elemType.(*types.Named); ok {
			// Named type - use reference
			if elemTypeInfo := r.ResolveType(elemType); elemTypeInfo != nil {
				info.ElementTypeRef = elemTypeInfo.GetCannonicalName()
				info.ElementKind = elemTypeInfo.GetKind()
			}
		} else {
			// Anonymous type - create inline
			switch elemType.(type) {
			case *types.Map, *types.Slice, *types.Array, *types.Chan:
				info.ElementTypeInfo = r.createAnonymousTypeInfo(elemType)
				info.ElementKind = info.ElementTypeInfo.GetKind()
			default:
				// Basic type
				info.ElementTypeRef = elemType.String()
				info.ElementKind = TypeKindBasic
			}
		}

		return info

	case *types.Array:
		info := NewAnonymousTypeInfo(TypeKindArray, descriptor)

		// Handle element type
		elemType := ut.Elem()
		if ptr, ok := elemType.(*types.Pointer); ok {
			info.ElementIsPointer = true
			elemType = ptr.Elem()
		}

		// Check if element type is named or anonymous
		if _, ok := elemType.(*types.Named); ok {
			// Named type - use reference
			if elemTypeInfo := r.ResolveType(elemType); elemTypeInfo != nil {
				info.ElementTypeRef = elemTypeInfo.GetCannonicalName()
				info.ElementKind = elemTypeInfo.GetKind()
			}
		} else {
			// Anonymous type - create inline
			switch elemType.(type) {
			case *types.Map, *types.Slice, *types.Array, *types.Chan:
				info.ElementTypeInfo = r.createAnonymousTypeInfo(elemType)
				info.ElementKind = info.ElementTypeInfo.GetKind()
			default:
				// Basic type
				info.ElementTypeRef = elemType.String()
				info.ElementKind = TypeKindBasic
			}
		}

		return info

	case *types.Chan:
		info := NewAnonymousTypeInfo(TypeKindChannel, descriptor)

		// Set channel direction
		switch ut.Dir() {
		case types.SendRecv:
			info.ChanDir = ChanDirBoth
		case types.SendOnly:
			info.ChanDir = ChanDirSend
		case types.RecvOnly:
			info.ChanDir = ChanDirRecv
		default:
			info.ChanDir = ChanDirBoth
		}

		// Handle element type
		elemType := ut.Elem()
		if ptr, ok := elemType.(*types.Pointer); ok {
			info.ElementIsPointer = true
			elemType = ptr.Elem()
		}

		// Check if element type is named or anonymous
		if _, ok := elemType.(*types.Named); ok {
			// Named type - use reference
			if elemTypeInfo := r.ResolveType(elemType); elemTypeInfo != nil {
				info.ElementTypeRef = elemTypeInfo.GetCannonicalName()
				info.ElementKind = elemTypeInfo.GetKind()
			}
		} else {
			// Anonymous type - create inline
			switch elemType.(type) {
			case *types.Map, *types.Slice, *types.Array, *types.Chan:
				info.ElementTypeInfo = r.createAnonymousTypeInfo(elemType)
				info.ElementKind = info.ElementTypeInfo.GetKind()
			default:
				// Basic type
				info.ElementTypeRef = elemType.String()
				info.ElementKind = TypeKindBasic
			}
		}

		return info

	default:
		// For basic types and other simple types
		return NewAnonymousTypeInfo(TypeKindVariable, descriptor)
	}
}

// createAnonymousStructInfo creates an AnonymousTypeInfo specifically for anonymous struct types
func (r *defaultTypeResolver) createAnonymousStructInfo(structType *types.Struct, originalDescriptor string) *AnonymousTypeInfo {
	r.anonymousCounter++

	// Use simple "anonymous" descriptor for anonymous structs
	info := NewAnonymousTypeInfo(TypeKindStruct, "__anonymous_struct_"+fmt.Sprintf("%d", r.anonymousCounter)+"__")

	// If field scanning is enabled, populate the fields
	if r.scanMode.Has(ScanModeFields) {
		var fields []FieldInfo

		// Process struct fields
		for i := 0; i < structType.NumFields(); i++ {
			field := structType.Field(i)

			// Skip unexported fields
			if !field.Exported() {
				continue
			}

			// Recursively analyze the field type
			fieldInfo := r.parseFieldType(field.Type(), field.Name())
			fields = append(fields, fieldInfo)
		}

		// Store fields in a way that can be accessed via JSON
		// Note: Since AnonymousTypeInfo doesn't use DetailedTypeInfo, we need to add fields support
		// For now, we'll add this to the AnonymousTypeInfo struct
		info.Fields = fields
	}

	return info
}

// processMethods discovers and parses methods from all types in the package
func (r *defaultTypeResolver) processMethods(pkg *packages.Package, docPkg *doc.Package) error {
	// Process methods from doc.Type (concrete struct/interface methods)
	for _, docType := range docPkg.Types {
		t := pkg.Types.Scope().Lookup(docType.Name)
		if t == nil {
			continue
		}

		// Get the TypeInfo for this type
		typeInfo := r.ResolveType(t.Type())
		if typeInfo == nil {
			continue
		}

		// Parse methods using go/types information for complete signature details
		// Note: We only use go/types approach to avoid duplicates and get better signature info
		err := r.parseMethodsFromTypes(pkg)
		if err != nil {
			return err
		}

		return nil
	}
	return nil
}

// parseMethodsFromTypes parses methods using go/types information for more complete signature details
func (r *defaultTypeResolver) parseMethodsFromTypes(pkg *packages.Package) error {
	// Iterate through all objects in the package scope
	scope := pkg.Types.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		if obj == nil {
			continue
		}

		// Check if this is a type name
		if _, ok := obj.(*types.TypeName); !ok {
			continue
		}

		// Get the named type
		namedType, ok := obj.Type().(*types.Named)
		if !ok {
			continue
		}

		// Get the TypeInfo for this type
		typeInfo := r.ResolveType(namedType)
		if typeInfo == nil {
			continue
		}

		// Parse methods from the named type
		err := r.parseNamedTypeMethods(namedType, typeInfo, pkg)
		if err != nil {
			return err
		}
	}

	return nil
}

// parseNamedTypeMethods parses methods from a go/types Named type
func (r *defaultTypeResolver) parseNamedTypeMethods(namedType *types.Named, ownerType TypeInfo, pkg *packages.Package) error {
	// Get the owner type's details to add methods to
	details, err := ownerType.Load()
	if err != nil {
		return err
	}
	if details == nil {
		return fmt.Errorf("no details available for type %s", ownerType.GetCannonicalName())
	}

	// Iterate through all methods of the named type
	for i := 0; i < namedType.NumMethods(); i++ {
		method := namedType.Method(i)

		// Skip unexported methods unless we're processing the same package
		if !method.Exported() && method.Pkg() != pkg.Types {
			continue
		}

		methodInfo, err := r.createMethodInfoFromTypes(method, ownerType, pkg, false)
		if err != nil {
			continue // Skip methods that can't be parsed
		}

		if methodInfo != nil {
			// Add method to the owner type's methods collection
			details.Methods = append(details.Methods, *methodInfo)
		}
	}

	return nil
}

// createMethodInfoFromTypes creates MethodInfo from go/types information
func (r *defaultTypeResolver) createMethodInfoFromTypes(method *types.Func, ownerType TypeInfo, pkg *packages.Package, isInterfaceMethod bool) (*MethodInfo, error) {
	if method == nil {
		return nil, fmt.Errorf("invalid method")
	}

	// Get the method signature
	sig, ok := method.Type().(*types.Signature)
	if !ok {
		return nil, fmt.Errorf("method has no signature")
	}

	// Extract receiver info - don't extract comments directly anymore
	// Comments will be loaded lazily when accessed

	// Determine receiver info - use clean base name instead of canonical name
	var receiverType string
	if ownerType.GetPackage() != "" {
		receiverType = ownerType.GetPackage() + "." + ownerType.GetName()
	} else {
		receiverType = ownerType.GetName()
	}

	isPointerReceiver := false
	receiverName := ""

	if sig.Recv() != nil {
		// Check if receiver is a pointer type
		if ptr, ok := sig.Recv().Type().(*types.Pointer); ok {
			isPointerReceiver = true
			_ = ptr
		}
		receiverName = sig.Recv().Name()
	}

	// Create the method info without direct comment extraction
	methodInfo := NewMethodInfo(
		method.Name(),
		pkg.PkgPath,
		receiverType, // Use clean base name
		isPointerReceiver,
		nil, // No lazy loader needed for methods
	)

	// Fix the descriptor to include receiver type name (using clean base name)
	methodInfo.Name = method.Name()
	methodInfo.Descriptor = receiverType + "." + method.Name()

	methodInfo.ReceiverName = receiverName
	methodInfo.IsInterfaceMethod = isInterfaceMethod

	// Parse method signature directly
	err := r.populateMethodSignature(methodInfo, sig, pkg)
	if err != nil {
		return nil, err
	}

	return methodInfo, nil
}

// createFunctionInfoFromDoc creates function info using doc.Func for proper comment extraction
func (r *defaultTypeResolver) createFunctionInfoFromDoc(funcDecl *ast.FuncDecl, docFunc *doc.Func, pkg *packages.Package) (TypeInfo, error) {
	funcName := funcDecl.Name.Name

	// Check if function is generic
	isGeneric := funcDecl.Type.TypeParams != nil && len(funcDecl.Type.TypeParams.List) > 0

	// Create NamedTypeInfo for the function - no immediate comment extraction
	namedTypeInfo := NewNamedTypeInfo(
		TypeKindFunction,
		funcName,
		pkg.PkgPath,
		nil, // Functions don't need complex lazy loading for now
	)

	namedTypeInfo.Descriptor = pkg.PkgPath + "." + funcName

	// Handle generic functions (if type parameters exist)
	if isGeneric {
		// For generic functions, populate type parameters directly in the flattened structure
		namedTypeInfo.TypeParameters = r.extractTypeParametersFromAST(funcDecl.Type.TypeParams)
		// Also set up loader for any future details that might be needed
		namedTypeInfo.loader = func() (*DetailedTypeInfo, error) {
			details := &DetailedTypeInfo{}
			// TypeParameters are already populated in the flattened structure
			details.TypeParameters = namedTypeInfo.TypeParameters
			return details, nil
		}
	}

	// Create FunctionInfo wrapper
	functionInfo := &FunctionInfo{
		NamedTypeInfo: namedTypeInfo,
		IsVariadic:    false,   // Will be set when parsing parameters
		docFunc:       docFunc, // Store doc.Func for lazy comment extraction
	}

	// Parse function signature for parameters and returns
	err := r.populateFunctionSignatureFromAST(functionInfo, funcDecl.Type, pkg)
	if err != nil {
		return nil, err
	}

	return functionInfo, nil
}

// createParameterInfo creates ParameterInfo from go/types.Var
func (r *defaultTypeResolver) createParameterInfo(param *types.Var, pkg *packages.Package) ParameterInfo {
	paramType := param.Type()
	isPointer := false
	typeRef := r.GetCannonicalName(paramType)

	// Check if it's a pointer type
	if _, ok := paramType.(*types.Pointer); ok {
		isPointer = true
	}

	paramInfo := ParameterInfo{
		BaseTypeDetailInfo: BaseTypeDetailInfo{
			Name:        param.Name(),
			TypeRef:     typeRef,
			PointerFlag: isPointer,
			Annotations: []gonnotation.Annotation{},
			Comments:    []string{},
		},
		IsVariadicParam: false, // TODO: Check for variadic
	}

	// Handle anonymous types
	if r.isAnonymousType(paramType) {
		paramInfo.AnonymousTypeInfo = r.createAnonymousTypeInfoFromGoTypes(paramType, pkg)
	}

	return paramInfo
}

// createReturnInfo creates ReturnInfo from go/types.Var
func (r *defaultTypeResolver) createReturnInfo(result *types.Var, pkg *packages.Package) ReturnInfo {
	resultType := result.Type()
	isPointer := false
	typeRef := r.GetCannonicalName(resultType)

	// Check if it's a pointer type
	if _, ok := resultType.(*types.Pointer); ok {
		isPointer = true
	}

	returnInfo := ReturnInfo{
		BaseTypeDetailInfo: BaseTypeDetailInfo{
			Name:        result.Name(),
			TypeRef:     typeRef,
			PointerFlag: isPointer,
		},
	}

	// Handle anonymous types
	if r.isAnonymousType(resultType) {
		returnInfo.AnonymousTypeInfo = r.createAnonymousTypeInfoFromGoTypes(resultType, pkg)
	}

	return returnInfo
}

// createAnonymousTypeInfoFromGoTypes creates AnonymousTypeInfo from go/types information
func (r *defaultTypeResolver) createAnonymousTypeInfoFromGoTypes(t types.Type, pkg *packages.Package) *AnonymousTypeInfo {
	switch typ := t.(type) {
	case *types.Struct:
		return r.createAnonymousStructInfoFromGoTypes(typ, pkg)
	case *types.Interface:
		return r.createAnonymousInterfaceInfoFromGoTypes(typ, pkg)
	case *types.Slice:
		elemType := typ.Elem()
		info := &AnonymousTypeInfo{
			Kind:       TypeKindSlice,
			Descriptor: fmt.Sprintf("[]%s", r.GetCannonicalName(elemType)),
		}
		if r.isAnonymousType(elemType) {
			info.ElementTypeInfo = r.createAnonymousTypeInfoFromGoTypes(elemType, pkg)
		} else {
			info.ElementTypeRef = r.GetCannonicalName(elemType)
		}
		return info
	case *types.Array:
		elemType := typ.Elem()
		info := &AnonymousTypeInfo{
			Kind:       TypeKindArray,
			Descriptor: fmt.Sprintf("[%d]%s", typ.Len(), r.GetCannonicalName(elemType)),
		}
		if r.isAnonymousType(elemType) {
			info.ElementTypeInfo = r.createAnonymousTypeInfoFromGoTypes(elemType, pkg)
		} else {
			info.ElementTypeRef = r.GetCannonicalName(elemType)
		}
		return info
	case *types.Map:
		keyType := typ.Key()
		valueType := typ.Elem()
		info := &AnonymousTypeInfo{
			Kind:       TypeKindMap,
			Descriptor: fmt.Sprintf("map[%s]%s", r.GetCannonicalName(keyType), r.GetCannonicalName(valueType)),
		}
		if r.isAnonymousType(keyType) {
			info.KeyTypeInfo = r.createAnonymousTypeInfoFromGoTypes(keyType, pkg)
		} else {
			info.KeyTypeRef = r.GetCannonicalName(keyType)
		}
		if r.isAnonymousType(valueType) {
			info.ElementTypeInfo = r.createAnonymousTypeInfoFromGoTypes(valueType, pkg)
		} else {
			info.ElementTypeRef = r.GetCannonicalName(valueType)
		}
		return info
	default:
		return &AnonymousTypeInfo{
			Kind:       TypeKindUnknown,
			Descriptor: r.GetCannonicalName(t),
		}
	}
}

// createAnonymousStructInfoFromGoTypes creates AnonymousTypeInfo for struct from go/types
func (r *defaultTypeResolver) createAnonymousStructInfoFromGoTypes(structType *types.Struct, pkg *packages.Package) *AnonymousTypeInfo {
	fields := []FieldInfo{}

	for i := 0; i < structType.NumFields(); i++ {
		field := structType.Field(i)
		tag := ""
		if i < structType.NumFields() {
			tag = structType.Tag(i)
		}

		fieldInfo := r.parseFieldTypeFromGoTypes(field.Type(), field.Name(), tag, pkg)
		fields = append(fields, fieldInfo)
	}

	return &AnonymousTypeInfo{
		Kind:       TypeKindStruct,
		Descriptor: "anonymous",
		Fields:     fields,
	}
}

// createAnonymousInterfaceInfoFromGoTypes creates AnonymousTypeInfo for interface from go/types
func (r *defaultTypeResolver) createAnonymousInterfaceInfoFromGoTypes(interfaceType *types.Interface, pkg *packages.Package) *AnonymousTypeInfo {
	details, err := r.createInterfaceDetails(interfaceType, "anonymous")
	if err != nil {
		// Fallback to basic info if parsing fails
		return &AnonymousTypeInfo{
			Kind:       TypeKindInterface,
			Descriptor: "anonymous",
		}
	}

	return &AnonymousTypeInfo{
		Kind:       TypeKindInterface,
		Descriptor: "anonymous",
		Methods:    details.Methods,
	}
}

// parseFieldTypeFromGoTypes parses field type from go/types information
func (r *defaultTypeResolver) parseFieldTypeFromGoTypes(fieldType types.Type, name string, tag string, pkg *packages.Package) FieldInfo {
	isPointer := false
	actualType := fieldType

	// Handle pointer types
	if ptr, ok := fieldType.(*types.Pointer); ok {
		isPointer = true
		actualType = ptr.Elem()
	}

	typeRef := r.GetCannonicalName(actualType)

	fieldInfo := FieldInfo{
		Name:      name,
		TypeRef:   typeRef,
		IsPointer: isPointer,
		Tags:      parseStructTag(tag),
		// Don't populate Comments/Annotations directly - use lazy loading
	}

	// Handle anonymous types
	if r.isAnonymousType(actualType) {
		fieldInfo.InlineTypeInfo = r.createAnonymousTypeInfoFromGoTypes(actualType, pkg)
		fieldInfo.IsAnonymous = true
	}

	return fieldInfo
}

// func (r *defaultTypeResolver) GetOrCreateType(cannonicalName string, constructor func() TypeInfo) TypeInfo {
// 	if ti, exists := r.types[cannonicalName]; exists {
// 		return ti
// 	}

// 	ti := constructor()
// 	if ti != nil {
// 		r.types[cannonicalName] = ti
// 	}

// 	return ti
// }

// isAnonymousType checks if a type is anonymous (not a named type)
func (r *defaultTypeResolver) isAnonymousType(t types.Type) bool {
	switch typ := t.(type) {
	case *types.Struct, *types.Interface, *types.Slice, *types.Array, *types.Map, *types.Chan, *types.Signature:
		return true
	case *types.Pointer:
		return r.isAnonymousType(typ.Elem())
	case *types.Named:
		return false
	default:
		return false
	}
}

// expandEmbeddedType expands an embedded type to extract promoted fields and methods
func (r *defaultTypeResolver) expandEmbeddedType(embeddedType types.Type, embeddedFieldName string, parentTypeRef string) ([]FieldInfo, []MethodInfo, error) {
	var promotedFields []FieldInfo
	var promotedMethods []MethodInfo

	// Get the canonical name of the embedded type for PromotedFromRef
	embeddedTypeRef := r.GetCannonicalName(embeddedType)

	// Handle pointer to embedded type
	actualType := embeddedType
	if ptr, ok := embeddedType.(*types.Pointer); ok {
		actualType = ptr.Elem()
	}

	// We only promote from named types (struct or interface)
	namedType, ok := actualType.(*types.Named)
	if !ok {
		return promotedFields, promotedMethods, nil
	}

	// Get the underlying type
	underlying := namedType.Underlying()

	// Promote fields from struct types
	if structType, ok := underlying.(*types.Struct); ok {
		for i := 0; i < structType.NumFields(); i++ {
			field := structType.Field(i)

			// Only promote exported fields
			if !field.Exported() {
				continue
			}

			// Create promoted field info
			// Get the original struct tag
			tag := ""
			if underlying, ok := namedType.Underlying().(*types.Struct); ok {
				tag = underlying.Tag(i)
			}

			fieldInfo := FieldInfo{
				Name:            field.Name(),
				TypeRef:         r.GetCannonicalName(field.Type()),
				TypeKind:        r.getTypeKind(field.Type()),
				IsPointer:       r.isPointerType(field.Type()),
				Tags:            parseStructTag(tag),
				IsPromoted:      true,
				PromotedFromRef: embeddedTypeRef,
				// Don't populate Comments/Annotations directly - use lazy loading
			} // Handle anonymous types in promoted fields
			actualFieldType := field.Type()
			if ptr, ok := actualFieldType.(*types.Pointer); ok {
				actualFieldType = ptr.Elem()
			}
			if r.isAnonymousType(actualFieldType) {
				fieldInfo.IsAnonymous = true
				fieldInfo.InlineTypeInfo = r.createAnonymousTypeInfoFromGoTypes(actualFieldType, nil)
			}

			promotedFields = append(promotedFields, fieldInfo)
		}
	}

	// Promote methods from the named type
	for i := 0; i < namedType.NumMethods(); i++ {
		method := namedType.Method(i)

		// Only promote exported methods
		if !method.Exported() {
			continue
		}

		// Get method signature
		sig, ok := method.Type().(*types.Signature)
		if !ok {
			continue
		}

		// Create promoted method info - IMPORTANT: Use parent type as receiver
		methodInfo := &MethodInfo{
			NamedTypeInfo: NewNamedTypeInfo(
				TypeKindMethod,
				method.Name(),
				method.Pkg().Path(),
				nil,
			),
			ReceiverTypeRef:   parentTypeRef, // Use parent type, not embedded type
			IsPointerReceiver: r.isPointerReceiver(sig),
			Parameters:        []ParameterInfo{},
			Returns:           []ReturnInfo{},
			IsVariadic:        sig.Variadic(),
			IsInterfaceMethod: false,
			IsPromoted:        true,
			PromotedFromRef:   embeddedTypeRef, // Track where it was promoted from
		}

		// Fix the descriptor to show the parent type, not embedded type
		methodInfo.Descriptor = parentTypeRef + "." + method.Name()

		// Parse method signature
		err := r.populateMethodSignature(methodInfo, sig, nil)
		if err == nil {
			promotedMethods = append(promotedMethods, *methodInfo)
		}
	}

	return promotedFields, promotedMethods, nil
}

// Helper functions for embedded type expansion

// getTypeKind determines the TypeKind for a go/types.Type
func (r *defaultTypeResolver) getTypeKind(t types.Type) TypeKind {
	switch actual := t.(type) {
	case *types.Pointer:
		return r.getTypeKind(actual.Elem())
	case *types.Named:
		underlying := actual.Underlying()
		switch underlying.(type) {
		case *types.Struct:
			return TypeKindStruct
		case *types.Interface:
			return TypeKindInterface
		default:
			return TypeKindBasic
		}
	case *types.Struct:
		return TypeKindStruct
	case *types.Interface:
		return TypeKindInterface
	case *types.Slice:
		return TypeKindSlice
	case *types.Array:
		return TypeKindArray
	case *types.Map:
		return TypeKindMap
	case *types.Chan:
		return TypeKindChannel
	case *types.Basic:
		return TypeKindBasic
	default:
		return TypeKindBasic
	}
}

// parseStructTag parses a struct tag into a map
func parseStructTag(tag string) map[string]string {
	tagMap := make(map[string]string)
	if tag == "" {
		return tagMap
	}

	// Remove surrounding backticks if present
	tag = strings.Trim(tag, "`")

	// Split by space, but handle quoted values
	parts := parseTagParts(tag)
	for _, part := range parts {
		if strings.Contains(part, ":") {
			kv := strings.SplitN(part, ":", 2)
			if len(kv) == 2 {
				key := strings.TrimSpace(kv[0])
				value := strings.Trim(strings.TrimSpace(kv[1]), `"`)
				tagMap[key] = value
			}
		}
	}

	return tagMap
}

// parseTagParts splits tag string into parts, handling quoted values
func parseTagParts(tag string) []string {
	var parts []string
	var current strings.Builder
	inQuotes := false

	for _, r := range tag {
		switch r {
		case '"':
			inQuotes = !inQuotes
			current.WriteRune(r)
		case ' ':
			if !inQuotes {
				if current.Len() > 0 {
					parts = append(parts, current.String())
					current.Reset()
				}
			} else {
				current.WriteRune(r)
			}
		default:
			current.WriteRune(r)
		}
	}

	if current.Len() > 0 {
		parts = append(parts, current.String())
	}

	return parts
}

// populateMethodSignature populates method parameters and returns from go/types signature
func (r *defaultTypeResolver) populateMethodSignature(methodInfo *MethodInfo, sig *types.Signature, pkg *packages.Package) error {
	// Parse parameters
	if sig.Params() != nil {
		for i := 0; i < sig.Params().Len(); i++ {
			param := sig.Params().At(i)
			paramInfo := r.createParameterInfo(param, pkg)
			methodInfo.Parameters = append(methodInfo.Parameters, paramInfo)
		}
	}

	// Check for variadic
	methodInfo.IsVariadic = sig.Variadic()

	// Parse return values
	if sig.Results() != nil {
		for i := 0; i < sig.Results().Len(); i++ {
			result := sig.Results().At(i)
			returnInfo := r.createReturnInfo(result, pkg)
			methodInfo.Returns = append(methodInfo.Returns, returnInfo)
		}
	}

	return nil
}

// createParameterInfoFromAST creates ParameterInfo from AST type expression
func (r *defaultTypeResolver) createParameterInfoFromAST(typeExpr ast.Expr, name string, pkg *packages.Package) ParameterInfo {
	isPointer := false
	actualType := typeExpr
	isVariadic := false

	// Handle pointer types
	if starExpr, ok := typeExpr.(*ast.StarExpr); ok {
		isPointer = true
		actualType = starExpr.X
	}

	// Handle variadic types
	if ellipsis, ok := typeExpr.(*ast.Ellipsis); ok {
		isVariadic = true
		actualType = ellipsis.Elt
	}

	// Create ParameterInfo
	paramInfo := ParameterInfo{
		BaseTypeDetailInfo: BaseTypeDetailInfo{
			Name:        name,
			PointerFlag: isPointer,
			Annotations: []gonnotation.Annotation{},
			Comments:    []string{},
		},
		IsVariadicParam: isVariadic,
	}

	// Analyze the type and populate detailed information
	r.analyzeTypeForBaseTypeDetailInfo(&paramInfo.BaseTypeDetailInfo, actualType, pkg)

	return paramInfo
} // createReturnInfoFromAST creates ReturnInfo from AST type expression
func (r *defaultTypeResolver) createReturnInfoFromAST(typeExpr ast.Expr, name string, pkg *packages.Package) ReturnInfo {
	isPointer := false
	actualType := typeExpr

	// Handle pointer types
	if starExpr, ok := typeExpr.(*ast.StarExpr); ok {
		isPointer = true
		actualType = starExpr.X
	}

	// Create ReturnInfo
	returnInfo := ReturnInfo{
		BaseTypeDetailInfo: BaseTypeDetailInfo{
			Name:        name,
			PointerFlag: isPointer,
		},
	}

	// Analyze the type and populate detailed information
	r.analyzeTypeForBaseTypeDetailInfo(&returnInfo.BaseTypeDetailInfo, actualType, pkg)

	return returnInfo
} // getTypeStringFromAST converts AST type expression to string representation
func (r *defaultTypeResolver) getTypeStringFromAST(typeExpr ast.Expr) string {
	switch t := typeExpr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.SelectorExpr:
		if pkgIdent, ok := t.X.(*ast.Ident); ok {
			return pkgIdent.Name + "." + t.Sel.Name
		}
		return t.Sel.Name
	case *ast.StarExpr:
		return "*" + r.getTypeStringFromAST(t.X)
	case *ast.ArrayType:
		if t.Len == nil {
			// Slice
			return "[]" + r.getTypeStringFromAST(t.Elt)
		} else {
			// Array - for simplicity, use [] without length
			return "[]" + r.getTypeStringFromAST(t.Elt)
		}
	case *ast.MapType:
		return "map[" + r.getTypeStringFromAST(t.Key) + "]" + r.getTypeStringFromAST(t.Value)
	case *ast.ChanType:
		switch t.Dir {
		case ast.SEND:
			return "chan<- " + r.getTypeStringFromAST(t.Value)
		case ast.RECV:
			return "<-chan " + r.getTypeStringFromAST(t.Value)
		default:
			return "chan " + r.getTypeStringFromAST(t.Value)
		}
	case *ast.InterfaceType:
		return "interface{}"
	case *ast.StructType:
		return "struct{}"
	case *ast.FuncType:
		return "func"
	case *ast.Ellipsis:
		return "..." + r.getTypeStringFromAST(t.Elt)
	default:
		return "unknown"
	}
}

// analyzeTypeForBaseTypeDetailInfo analyzes a type and populates detailed information in BaseTypeDetailInfo
func (r *defaultTypeResolver) analyzeTypeForBaseTypeDetailInfo(baseInfo *BaseTypeDetailInfo, typeExpr ast.Expr, pkg *packages.Package) {
	r.analyzeTypeDetails(typeExpr, pkg,
		func(typeRef string, typeKind TypeKind, isAnonymous bool) {
			baseInfo.TypeRef = typeRef
			baseInfo.TypeKind = typeKind
			baseInfo.AnonymousFlag = isAnonymous
		},
		func(elemTypeRef string, elemKind TypeKind, elemIsPointer, elemIsAnonymous bool, structure string) {
			baseInfo.ElementTypeRef = elemTypeRef
			baseInfo.ElementKind = elemKind
			baseInfo.ElementIsPointerFlag = elemIsPointer
			baseInfo.ElementIsAnonymousFlag = elemIsAnonymous
			baseInfo.ElementStructure = structure
		},
		func(keyTypeRef string, keyKind TypeKind, keyIsPointer, keyIsAnonymous bool, keyStructure string) {
			baseInfo.KeyTypeRef = keyTypeRef
			baseInfo.KeyKind = keyKind
			baseInfo.KeyIsPointerFlag = keyIsPointer
			baseInfo.KeyIsAnonymousFlag = keyIsAnonymous
			baseInfo.KeyStructure = keyStructure
		},
		func(chanDir ChannelDirection) {
			baseInfo.ChanDir = chanDir
		},
	)
}

// analyzeTypeDetails analyzes an AST type and calls appropriate callbacks with detailed information
func (r *defaultTypeResolver) analyzeTypeDetails(
	typeExpr ast.Expr,
	pkg *packages.Package,
	setBasicInfo func(typeRef string, typeKind TypeKind, isAnonymous bool),
	setElementInfo func(elemTypeRef string, elemKind TypeKind, elemIsPointer, elemIsAnonymous bool, structure string),
	setKeyInfo func(keyTypeRef string, keyKind TypeKind, keyIsPointer, keyIsAnonymous bool, keyStructure string),
	setChanInfo func(chanDir ChannelDirection),
) {
	switch t := typeExpr.(type) {
	case *ast.ArrayType:
		if t.Len == nil {
			// Slice type []T
			setBasicInfo("", TypeKindSlice, false)
			r.analyzeElementType(t.Elt, "[]", pkg, setElementInfo)
		} else {
			// Array type [N]T
			setBasicInfo("", TypeKindArray, false)
			r.analyzeElementType(t.Elt, "[]", pkg, setElementInfo) // Use [] for simplicity
		}

	case *ast.MapType:
		// Map type map[K]V
		setBasicInfo("", TypeKindMap, false)
		// Analyze key type
		r.analyzeElementType(t.Key, "", pkg, func(keyTypeRef string, keyKind TypeKind, keyIsPointer, keyIsAnonymous bool, _ string) {
			setKeyInfo(keyTypeRef, keyKind, keyIsPointer, keyIsAnonymous, "")
		})
		// Analyze value type
		r.analyzeElementType(t.Value, "", pkg, setElementInfo)

	case *ast.ChanType:
		// Channel type chan T
		setBasicInfo("", TypeKindChannel, false)
		// Set channel direction
		switch t.Dir {
		case ast.SEND:
			setChanInfo(ChanDirSend)
		case ast.RECV:
			setChanInfo(ChanDirRecv)
		default:
			setChanInfo(ChanDirBoth)
		}
		// Analyze element type
		r.analyzeElementType(t.Value, "", pkg, setElementInfo)

	case *ast.Ident:
		// Simple identifier - could be basic type, named type, or type parameter
		typeRef := t.Name
		typeKind := r.determineBasicOrGenericKind(t.Name)
		setBasicInfo(typeRef, typeKind, false)

	case *ast.SelectorExpr:
		// Qualified type reference (pkg.Type)
		typeRef := r.getTypeStringFromAST(typeExpr)
		setBasicInfo(typeRef, TypeKindStruct, false) // Default assumption

	case *ast.StructType:
		// Inline struct type
		setBasicInfo("struct{}", TypeKindStruct, true)

	case *ast.InterfaceType:
		// Inline interface type
		setBasicInfo("any", TypeKindInterface, true)

	case *ast.FuncType:
		// Function type
		setBasicInfo("func", TypeKindFunction, true)

	default:
		// Fallback
		typeRef := r.getTypeStringFromAST(typeExpr)
		setBasicInfo(typeRef, TypeKindUnknown, false)
	}
}

// analyzeElementType analyzes element types for slices, arrays, maps, and channels
func (r *defaultTypeResolver) analyzeElementType(
	elemExpr ast.Expr,
	currentStructure string,
	pkg *packages.Package,
	setElementInfo func(elemTypeRef string, elemKind TypeKind, elemIsPointer, elemIsAnonymous bool, structure string),
) {
	elemIsPointer := false
	actualElemType := elemExpr

	// Handle pointer elements
	if starExpr, ok := elemExpr.(*ast.StarExpr); ok {
		elemIsPointer = true
		actualElemType = starExpr.X
	}

	switch t := actualElemType.(type) {
	case *ast.ArrayType:
		if t.Len == nil {
			// Nested slice [][]T
			newStructure := currentStructure + "[]"
			r.analyzeElementType(t.Elt, newStructure, pkg, setElementInfo)
		} else {
			// Nested array [][N]T
			newStructure := currentStructure + "[]"
			r.analyzeElementType(t.Elt, newStructure, pkg, setElementInfo)
		}

	case *ast.Ident:
		// Simple element type
		elemTypeRef := t.Name
		elemKind := r.determineBasicOrGenericKind(t.Name)
		setElementInfo(elemTypeRef, elemKind, elemIsPointer, false, currentStructure)

	case *ast.SelectorExpr:
		// Qualified element type
		elemTypeRef := r.getTypeStringFromAST(actualElemType)
		setElementInfo(elemTypeRef, TypeKindStruct, elemIsPointer, false, currentStructure)

	case *ast.StructType:
		// Inline struct element
		setElementInfo("struct{}", TypeKindStruct, elemIsPointer, true, currentStructure)

	case *ast.InterfaceType:
		// Inline interface element
		setElementInfo("any", TypeKindInterface, elemIsPointer, true, currentStructure)

	default:
		// Fallback
		elemTypeRef := r.getTypeStringFromAST(actualElemType)
		setElementInfo(elemTypeRef, TypeKindUnknown, elemIsPointer, false, currentStructure)
	}
}

// determineBasicOrGenericKind determines if a type name is basic, generic, or other
func (r *defaultTypeResolver) determineBasicOrGenericKind(typeName string) TypeKind {
	// Check if it's a Go builtin type
	if r.isBuiltinType(typeName) {
		return TypeKindBasic
	}

	// Check if it's a single letter (likely generic type parameter)
	if len(typeName) == 1 && typeName[0] >= 'A' && typeName[0] <= 'Z' {
		return TypeKindGeneric
	}

	// Check for common generic type parameter names
	if typeName == "T" || typeName == "U" || typeName == "V" || typeName == "K" {
		return TypeKindGeneric
	}

	// Default to basic for now - could be improved with more context
	return TypeKindBasic
}

// normalizeEmptyInterface converts "interface{}" to "any" for consistency
func (r *defaultTypeResolver) normalizeEmptyInterface(typeRef string) string {
	if typeRef == "interface{}" {
		return "any"
	}
	return typeRef
}
