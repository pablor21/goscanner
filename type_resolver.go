package goscanner

import (
	"fmt"
	"go/ast"
	"go/doc"
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

func (r *defaultTypeResolver) loadPackage(pkgPath string) (*packages.Package, error) {
	if r, exists := r.packages[pkgPath]; exists {
		return r, nil
	}

	var loadMode packages.LoadMode

	// Always need basic package info
	loadMode = packages.NeedName

	// Add modes based on ScanMode flags
	if r.scanMode.Has(ScanModeTypes) {
		loadMode |= packages.NeedTypes
	}

	if r.scanMode.Has(ScanModeMethods) || r.scanMode.Has(ScanModeFields) || r.scanMode.Has(ScanModeDocs) || r.scanMode.Has(ScanModeAnnotations) {
		// Need syntax tree for detailed analysis
		loadMode |= packages.NeedSyntax | packages.NeedFiles
	}

	// Only add heavy TypesInfo if we need method/field details
	if r.scanMode.Has(ScanModeMethods) || r.scanMode.Has(ScanModeFields) {
		loadMode |= packages.NeedTypesInfo
	}

	cfg := &packages.Config{
		Mode: loadMode,
	}
	pkgs, err := packages.Load(cfg, pkgPath)
	if err != nil {
		return nil, err
	}
	if len(pkgs) == 0 {
		return nil, nil
	}
	pkg := pkgs[0]

	// Extract doc.Types from this package
	docPkg, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath)
	if err == nil {
		for _, docType := range docPkg.Types {
			canonicalName := pkgPath + "." + docType.Name
			r.docTypes[canonicalName] = docType
		}
	}

	r.packages[pkgPath] = pkg
	r.loadedPkgs[pkgPath] = true
	return pkg, nil
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
		fmt.Println("found type:" + canonicalName)
		r.ResolveType(t.Type())
	}

	// 2. Process methods if ScanModeMethods is enabled
	if r.scanMode.Has(ScanModeMethods) {
		err = r.processMethods(pkg, docPkg)
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

func (r *defaultTypeResolver) ResolveType(t types.Type) TypeInfo {
	if t == nil {
		return nil
	}

	typeName := r.GetCannonicalName(t)

	// Check cache first
	if ti, exists := r.types[typeName]; exists {
		return ti
	}

	var ti TypeInfo
	switch ut := t.(type) {
	case *types.Named:
		underlying := ut.Underlying()
		switch underlying.(type) {
		case *types.Struct:
			// Complex types - cache them
			ti = r.makeStructTypeInfo(t, typeName)
			if ti != nil {
				r.types[typeName] = ti
			}
		case *types.Interface:
			// Complex types - cache them
			ti = r.makeInterfaceTypeInfo(t, typeName) // TODO: implement proper interface handling
			if ti != nil {
				r.types[typeName] = ti
			}
		default:
			// Other named types (type aliases, etc.) - simple reference
			ti = r.makeSimpleTypeReference(t, typeName)
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
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
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

		return details, nil
	}

	// Create StructInfo with proper NamedTypeInfo
	return &StructInfo{
		NamedTypeInfo: NewNamedTypeInfo(
			TypeKindStruct,
			name,
			pkg,
			[]string{},                 // No comments from types.Type
			[]gonnotation.Annotation{}, // No annotations from types.Type
			loader,
		),
	}
}

// parseFieldType analyzes a field's type and returns properly populated FieldInfo
func (r *defaultTypeResolver) parseFieldType(fieldType types.Type, fieldName string) FieldInfo {
	return r.parseFieldTypeWithTag(fieldType, fieldName, "")
}

// parseFieldTypeWithTag analyzes a field's type with struct tag and returns properly populated FieldInfo
func (r *defaultTypeResolver) parseFieldTypeWithTag(fieldType types.Type, fieldName string, tag string) FieldInfo {
	fieldInfo := FieldInfo{
		Name:        fieldName,
		Annotations: []gonnotation.Annotation{}, // TODO: Get from tags/comments when available
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

// makeSimpleTypeReference creates a lightweight TypeInfo for basic/simple types that shouldn't be exported
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
		[]string{},                 // No comments for simple types
		[]gonnotation.Annotation{}, // No annotations for simple types
		loader,
	)
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
	if named, ok := t.(*types.Named); ok {
		obj := named.Obj()
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
		return r.createInterfaceDetails(interfaceType, typeName)
	}

	// Create InterfaceInfo with proper NamedTypeInfo
	return NewInterfaceInfo(
		name,
		pkg,
		[]string{},                 // No comments from types.Type
		[]gonnotation.Annotation{}, // No annotations from types.Type
		loader,
	)
}

// createInterfaceDetails creates DetailedTypeInfo for interface types
func (r *defaultTypeResolver) createInterfaceDetails(interfaceType *types.Interface, typeName string) (*DetailedTypeInfo, error) {
	if interfaceType == nil {
		return &DetailedTypeInfo{Methods: []MethodInfo{}}, nil
	}

	details := &DetailedTypeInfo{
		Methods: []MethodInfo{},
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
		} // Create fake owner type info for interface methods - use just the interface name, not full canonical name
		var interfaceName string
		var packagePath string

		// Extract clean interface name and package
		if len(typeName) > 0 && strings.Contains(typeName, ".") {
			parts := strings.Split(typeName, ".")
			interfaceName = parts[len(parts)-1] // Get just the interface name
			packagePath = strings.Join(parts[:len(parts)-1], ".")
		} else {
			interfaceName = typeName
			packagePath = r.currentPkg.PkgPath
		}

		ownerType := &NamedTypeInfo{
			Name:        interfaceName,
			Descriptor:  typeName, // Use full canonical name for descriptor
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

	return details, nil // Parse embedded interfaces

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

// drillToFinalType recursively drills down through composite types to find the final non-composite type
// Returns: (finalType, structure, isPointer)
// Examples:
//
//	[][][][]*OtherStruct -> (OtherStruct, "[][][][]", true)
//	[5][10]string -> (string, "[5][10]", false)
//	chan *[]User -> (User, "[]", true) with channel info handled separately
//	map[string]*[][]User -> (User, "[][]", true) for map values, key handled separately
func (r *defaultTypeResolver) drillToFinalType(t types.Type) (types.Type, string, bool) {
	var structure string
	isPointer := false
	current := t

	for {
		switch ct := current.(type) {
		case *types.Pointer:
			isPointer = true
			current = ct.Elem()
		case *types.Slice:
			structure += "[]"
			current = ct.Elem()
		case *types.Array:
			structure += fmt.Sprintf("[%d]", ct.Len())
			current = ct.Elem()
		case *types.Map:
			// For maps, drill through the value type only
			// Key handling is done separately in map-specific logic
			current = ct.Elem()
		case *types.Chan:
			// For channels, drill through the element type
			current = ct.Elem()
		default:
			// We've reached a non-composite type
			return current, structure, isPointer
		}
	}
}

// drillToFinalKeyType handles map keys, which can also be composite
// Returns: (finalKeyType, keyStructure, isPointer)
func (r *defaultTypeResolver) drillToFinalKeyType(t types.Type) (types.Type, string, bool) {
	return r.drillToFinalType(t)
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

// resolveTypeForAnonymous resolves a type that might be used in anonymous contexts
// Returns TypeInfo for named types, or creates AnonymousTypeInfo for composite types
func (r *defaultTypeResolver) resolveTypeForAnonymous(t types.Type) TypeInfo {
	// For named types, use the regular resolver (this will return a reference-based TypeInfo)
	if _, ok := t.(*types.Named); ok {
		return r.ResolveType(t)
	}

	// For composite types, create anonymous TypeInfo only if they're truly composite
	switch t.(type) {
	case *types.Map, *types.Slice, *types.Array, *types.Chan:
		return r.createAnonymousTypeInfo(t)
	default:
		// For basic types, create a simple reference (not full inline details)
		return r.makeSimpleTypeReference(t, t.String())
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
	details, err := ownerType.GetDetails()
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

	// Parse comments and annotations (would need AST lookup for full comments)
	comments := []string{}
	annotations := []gonnotation.Annotation{}

	// Determine receiver info
	receiverType := ownerType.GetCannonicalName()
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

	// Create the method info and directly parse signature
	methodInfo := NewMethodInfo(
		method.Name(),
		pkg.PkgPath,
		comments,
		annotations,
		receiverType,
		isPointerReceiver,
		nil, // No lazy loader needed for methods
	)

	// Fix the descriptor to include receiver type name
	methodInfo.NamedTypeInfo.Name = method.Name()
	methodInfo.NamedTypeInfo.Descriptor = receiverType + "." + method.Name()

	methodInfo.ReceiverName = receiverName
	methodInfo.IsInterfaceMethod = isInterfaceMethod

	// Parse method signature directly
	err := r.populateMethodSignature(methodInfo, sig, pkg)
	if err != nil {
		return nil, err
	}

	return methodInfo, nil
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
		Name:        param.Name(),
		TypeRef:     typeRef,
		IsPointer:   isPointer,
		IsVariadic:  false, // TODO: Check for variadic
		Annotations: []gonnotation.Annotation{},
		Comments:    []string{},
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
		Name:      result.Name(),
		TypeRef:   typeRef,
		IsPointer: isPointer,
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
		Name:        name,
		TypeRef:     typeRef,
		IsPointer:   isPointer,
		Tags:        parseStructTag(tag),
		Annotations: []gonnotation.Annotation{},
		Comments:    []string{},
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
				Annotations:     []gonnotation.Annotation{},
				Comments:        []string{},
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
				[]string{},
				[]gonnotation.Annotation{},
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
		methodInfo.NamedTypeInfo.Descriptor = parentTypeRef + "." + method.Name()

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

// isPointerType checks if a type is a pointer
func (r *defaultTypeResolver) isPointerType(t types.Type) bool {
	_, ok := t.(*types.Pointer)
	return ok
}

// isPointerReceiver checks if a method has a pointer receiver
func (r *defaultTypeResolver) isPointerReceiver(sig *types.Signature) bool {
	if sig.Recv() == nil {
		return false
	}
	_, ok := sig.Recv().Type().(*types.Pointer)
	return ok
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
} // parseJsonTag extracts the JSON tag value for convenience
func parseJsonTag(tag string) string {
	tagMap := parseStructTag(tag)
	if jsonTag, exists := tagMap["json"]; exists {
		// Remove omitempty and other options, just return the name
		parts := strings.Split(jsonTag, ",")
		if len(parts) > 0 && parts[0] != "-" {
			return parts[0]
		}
	}
	return ""
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

// populateMethodSignatureFromAST populates method parameters and returns from AST function type
func (r *defaultTypeResolver) populateMethodSignatureFromAST(methodInfo *MethodInfo, funcType *ast.FuncType, pkg *packages.Package) error {
	// Parse parameters from AST
	if funcType.Params != nil {
		for _, field := range funcType.Params.List {
			// Handle multiple names with same type: func(a, b int)
			if len(field.Names) == 0 {
				// Unnamed parameter
				paramInfo := r.createParameterInfoFromAST(field.Type, "", pkg)
				methodInfo.Parameters = append(methodInfo.Parameters, paramInfo)
			} else {
				// Named parameters
				for _, name := range field.Names {
					paramInfo := r.createParameterInfoFromAST(field.Type, name.Name, pkg)
					methodInfo.Parameters = append(methodInfo.Parameters, paramInfo)
				}
			}
		}
	}

	// Check for variadic (ellipsis in last parameter)
	if funcType.Params != nil && len(funcType.Params.List) > 0 {
		lastParam := funcType.Params.List[len(funcType.Params.List)-1]
		if _, ok := lastParam.Type.(*ast.Ellipsis); ok {
			methodInfo.IsVariadic = true
		}
	}

	// Parse return values from AST
	if funcType.Results != nil {
		for _, field := range funcType.Results.List {
			// Handle multiple names with same type: func() (a, b int)
			if len(field.Names) == 0 {
				// Unnamed return
				returnInfo := r.createReturnInfoFromAST(field.Type, "", pkg)
				methodInfo.Returns = append(methodInfo.Returns, returnInfo)
			} else {
				// Named returns
				for _, name := range field.Names {
					returnInfo := r.createReturnInfoFromAST(field.Type, name.Name, pkg)
					methodInfo.Returns = append(methodInfo.Returns, returnInfo)
				}
			}
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

	// Get type reference - for AST we need to construct the type string
	typeRef := r.getTypeStringFromAST(actualType)

	paramInfo := ParameterInfo{
		Name:        name,
		TypeRef:     typeRef,
		IsPointer:   isPointer,
		IsVariadic:  isVariadic,
		Annotations: []gonnotation.Annotation{},
		Comments:    []string{},
	}

	return paramInfo
}

// createReturnInfoFromAST creates ReturnInfo from AST type expression
func (r *defaultTypeResolver) createReturnInfoFromAST(typeExpr ast.Expr, name string, pkg *packages.Package) ReturnInfo {
	isPointer := false
	actualType := typeExpr

	// Handle pointer types
	if starExpr, ok := typeExpr.(*ast.StarExpr); ok {
		isPointer = true
		actualType = starExpr.X
	}

	// Get type reference - for AST we need to construct the type string
	typeRef := r.getTypeStringFromAST(actualType)

	return ReturnInfo{
		Name:      name,
		TypeRef:   typeRef,
		IsPointer: isPointer,
	}
}

// getTypeStringFromAST converts AST type expression to string representation
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
			return "[]" + r.getTypeStringFromAST(t.Elt)
		}
		return "[]" + r.getTypeStringFromAST(t.Elt)
	case *ast.MapType:
		return "map[" + r.getTypeStringFromAST(t.Key) + "]" + r.getTypeStringFromAST(t.Value)
	case *ast.ChanType:
		return "chan " + r.getTypeStringFromAST(t.Value)
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

// }
