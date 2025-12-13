package goscanner

import (
	"fmt"
	"go/doc"
	"go/types"

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
	types        map[string]TypeInfo
	ignoredTypes map[string]struct{}
	docTypes     map[string]*doc.Type         // All discovered doc types
	packages     map[string]*packages.Package // All loaded packages
	loadedPkgs   map[string]bool              // Track what's been processed
	scanMode     ScanMode
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

		// Process struct fields
		for i := 0; i < structType.NumFields(); i++ {
			field := structType.Field(i)

			// Skip unexported fields
			if !field.Exported() {
				continue
			}

			// Analyze the field type to extract canonical type and structure info
			fieldInfo := r.parseFieldType(field.Type(), field.Name())

			details.Fields = append(details.Fields, fieldInfo)
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

// makeInterfaceTypeInfo creates TypeInfo for interface types (placeholder for now)
func (r *defaultTypeResolver) makeInterfaceTypeInfo(t types.Type, typeName string) TypeInfo {
	// TODO: Implement proper interface handling
	return r.makeSimpleTypeReference(t, typeName)
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
	// Use simple "anonymous" descriptor for anonymous structs
	info := NewAnonymousTypeInfo(TypeKindStruct, "anonymous")

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
