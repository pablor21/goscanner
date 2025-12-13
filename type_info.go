package goscanner

import (
	"go/ast"
	"go/doc"
	"go/types"
	"sync"

	"github.com/pablor21/gonnotation"
	"golang.org/x/tools/go/packages"
)

type TypeKind string

const (
	TypeKindStruct    TypeKind = "struct"
	TypeKindEnum      TypeKind = "enum"
	TypeKindField     TypeKind = "field"
	TypeKindInterface TypeKind = "interface"
	TypeKindFunction  TypeKind = "function"
	TypeKindMethod    TypeKind = "method"
	TypeKindVariable  TypeKind = "variable"
	TypeKindMap       TypeKind = "map"
	TypeKindSlice     TypeKind = "slice"
	TypeKindArray     TypeKind = "array"
	TypeKindChannel   TypeKind = "channel"
	TypeKindBasic     TypeKind = "basic"   // For built-in types like string, int, bool
	TypeKindGeneric   TypeKind = "generic" // For generic type parameters like T, U, etc.
	TypeKindUnknown   TypeKind = "unknown" // For unrecognized types
)

type ChannelDirection string

const (
	ChanDirBoth ChannelDirection = "both" // chan T (bidirectional)
	ChanDirSend ChannelDirection = "send" // chan<- T (send-only)
	ChanDirRecv ChannelDirection = "recv" // <-chan T (receive-only)
)

// TypeParameterInfo represents a generic type parameter
type TypeParameterInfo struct {
	Name        string     `json:"Name"`
	Constraints []TypeInfo `json:"Constraints,omitempty"` // Can be pointers, interfaces, or other types
}

// GenericArgumentInfo represents a concrete type argument for a generic parameter
type GenericArgumentInfo struct {
	ParameterName    string   `json:"ParameterName"`    // Name of the generic parameter (T, K, V, etc.)
	ParameterTypeRef string   `json:"ParameterTypeRef"` // Reference to the concrete type
	ParameterKind    TypeKind `json:"ParameterKind"`    // Kind of the concrete type
	IsPointer        bool     `json:"IsPointer"`        // Whether the concrete type is a pointer
}

// DetailedTypeInfo contains heavy structural information that's lazy-loaded
type DetailedTypeInfo struct {
	// Generic type parameters
	TypeParameters []TypeParameterInfo `json:"TypeParameters,omitempty"`

	// Structure details for structs
	Fields  []FieldInfo  `json:"Fields,omitempty"`
	Methods []MethodInfo `json:"Methods,omitempty"`

	// Type characteristics
	MapFlag          bool     `json:"IsMap,omitempty"`
	KeyType          TypeInfo `json:"MapKeyType,omitempty"`
	KeyTypePtrFlag   bool     `json:"IsMapKeyPointer,omitempty"`
	ValueType        TypeInfo `json:"ValueType,omitempty"`
	ValueTypePtrFlag bool     `json:"IsValuePointer,omitempty"`
	SliceFlag        bool     `json:"IsSlice,omitempty"`
	ElementType      TypeInfo `json:"ElementType,omitempty"`
	ChanFlag         bool     `json:"IsChan,omitempty"`
	ChanDir          int      `json:"ChanDir,omitempty"`

	// Enum values
	EnumValues []EnumValue `json:"Values,omitempty"`
}

type TypeInfo interface {
	// Always available (eagerly loaded)
	GetKind() TypeKind
	GetName() string
	GetPackage() string
	GetCannonicalName() string
	GetAnnotations() []gonnotation.Annotation
	GetComments() []string

	// New methods for anonymous type support
	IsAnonymous() bool
	GetTypeDescriptor() string

	// Lazy-loaded details
	Load() (*DetailedTypeInfo, error)

	// Convenience methods that may trigger lazy loading
	IsMap() bool
	GetMapKeyType() TypeInfo
	IsMapKeyPointer() bool
	IsSlice() bool
	GetElementType() TypeInfo
	IsElementPointer() bool
	IsBasic() bool
	IsChannel() bool
	IsGeneric() bool
	GetTypeParameters() ([]TypeParameterInfo, error)
}

// TypeDetailInfo represents common interface for detailed type information (parameters, returns, fields)
type TypeDetailInfo interface {
	GetName() string
	GetTypeRef() string
	GetTypeKind() TypeKind
	IsPointer() bool
	IsAnonymous() bool
	IsVariadic() bool // Only meaningful for parameters

	// Element info (for slices, arrays, channels)
	GetElementInfo() (typeRef string, kind TypeKind, isPointer, isAnonymous bool, structure string)

	// Key info (for maps)
	GetKeyInfo() (typeRef string, kind TypeKind, isPointer, isAnonymous bool, structure string)

	// Channel info
	GetChannelDirection() ChannelDirection

	GetAnnotations() []gonnotation.Annotation
	GetComments() []string
}

// BaseTypeDetailInfo provides common implementation for detailed type information
type BaseTypeDetailInfo struct {
	Name          string   `json:"Name,omitempty"`        // Name (parameter/return/field name)
	TypeRef       string   `json:"TypeRef"`               // Type reference
	TypeKind      TypeKind `json:"TypeKind,omitempty"`    // Type kind (slice, map, etc.)
	PointerFlag   bool     `json:"IsPointer"`             // Is pointer type
	AnonymousFlag bool     `json:"IsAnonymous,omitempty"` // Whether the type is anonymous/inline

	// For slices and arrays
	ElementTypeRef         string   `json:"ElementTypeRef,omitempty"`     // Reference to element type
	ElementIsPointerFlag   bool     `json:"ElementIsPointer,omitempty"`   // Whether element type is a pointer
	ElementIsAnonymousFlag bool     `json:"ElementIsAnonymous,omitempty"` // Whether element type is anonymous/inline
	ElementKind            TypeKind `json:"ElementKind,omitempty"`        // Kind of the element type
	ElementStructure       string   `json:"ElementStructure,omitempty"`   // Describes nesting structure: "[]", "[][][]", etc.

	// For maps
	KeyTypeRef         string   `json:"KeyTypeRef,omitempty"`     // Reference to key type
	KeyIsPointerFlag   bool     `json:"KeyIsPointer,omitempty"`   // Whether key type is a pointer
	KeyIsAnonymousFlag bool     `json:"KeyIsAnonymous,omitempty"` // Whether key type is anonymous/inline
	KeyKind            TypeKind `json:"KeyKind,omitempty"`        // Kind of the key type
	KeyStructure       string   `json:"KeyStructure,omitempty"`   // Describes key nesting if composite

	// For channels
	ChanDir ChannelDirection `json:"ChanDir,omitempty"` // Channel direction

	AnonymousTypeInfo *AnonymousTypeInfo       `json:"AnonymousTypeInfo,omitempty"` // For inline types
	Annotations       []gonnotation.Annotation `json:"Annotations,omitempty"`       // Annotations
	Comments          []string                 `json:"Comments,omitempty"`          // Comments
}

// Interface implementation for BaseTypeDetailInfo
func (b *BaseTypeDetailInfo) GetName() string       { return b.Name }
func (b *BaseTypeDetailInfo) GetTypeRef() string    { return b.TypeRef }
func (b *BaseTypeDetailInfo) GetTypeKind() TypeKind { return b.TypeKind }

func (b *BaseTypeDetailInfo) IsPointer() bool   { return b.PointerFlag }
func (b *BaseTypeDetailInfo) IsAnonymous() bool { return b.AnonymousFlag }
func (b *BaseTypeDetailInfo) IsVariadic() bool  { return false } // Base implementation - overridden in ParameterInfo
func (b *BaseTypeDetailInfo) GetElementInfo() (string, TypeKind, bool, bool, string) {
	return b.ElementTypeRef, b.ElementKind, b.ElementIsPointerFlag, b.ElementIsAnonymousFlag, b.ElementStructure
}
func (b *BaseTypeDetailInfo) GetKeyInfo() (string, TypeKind, bool, bool, string) {
	return b.KeyTypeRef, b.KeyKind, b.KeyIsPointerFlag, b.KeyIsAnonymousFlag, b.KeyStructure
}
func (b *BaseTypeDetailInfo) GetChannelDirection() ChannelDirection    { return b.ChanDir }
func (b *BaseTypeDetailInfo) GetAnnotations() []gonnotation.Annotation { return b.Annotations }
func (b *BaseTypeDetailInfo) GetComments() []string                    { return b.Comments }

type NamedTypeInfo struct {
	// Eagerly loaded basic info
	Kind        TypeKind                 `json:"Kind,omitempty"`
	Name        string                   `json:"Name,omitempty"`
	Package     string                   `json:"Package,omitempty"`
	Descriptor  string                   `json:"Descriptor,omitempty"` // Type descriptor
	Annotations []gonnotation.Annotation `json:"Annotations,omitempty"`
	Comments    []string                 `json:"Comments,omitempty"`

	// Generic instantiation info (eagerly loaded)
	IsGenericInstantiation bool                  `json:"IsGenericInstantiation,omitempty"`
	GenericTypeRef         string                `json:"GenericTypeRef,omitempty"`   // Reference to the generic base type
	GenericArguments       []GenericArgumentInfo `json:"GenericArguments,omitempty"` // Concrete type arguments

	// Type alias info (eagerly loaded)
	IsTypeAlias  bool   `json:"IsTypeAlias,omitempty"`  // Whether this is a type alias
	TypeAliasRef string `json:"TypeAliasRef,omitempty"` // Reference to the aliased type

	// Flattened lazy-loaded details
	TypeParameters   []TypeParameterInfo `json:"TypeParameters,omitempty"`
	Fields           []FieldInfo         `json:"Fields,omitempty"`
	Methods          []MethodInfo        `json:"Methods,omitempty"`
	MapFlag          bool                `json:"IsMap,omitempty"`
	KeyType          TypeInfo            `json:"MapKeyType,omitempty"`
	KeyTypePtrFlag   bool                `json:"IsMapKeyPointer,omitempty"`
	ValueType        TypeInfo            `json:"ValueType,omitempty"`
	ValueTypePtrFlag bool                `json:"IsValuePointer,omitempty"`
	SliceFlag        bool                `json:"IsSlice,omitempty"`
	ElementType      TypeInfo            `json:"ElementType,omitempty"`
	ChanFlag         bool                `json:"IsChan,omitempty"`
	ChanDir          int                 `json:"ChanDir,omitempty"`

	// Enum values (flattened from details)
	EnumValues []EnumValue `json:"Values,omitempty"`

	// Lazy loading mechanism (not exported to JSON)
	detailsOnce sync.Once                         `json:"-"`
	detailsErr  error                             `json:"-"`
	loader      func() (*DetailedTypeInfo, error) `json:"-"`
	obj         types.Object                      `json:"-"`
	pkg         *packages.Package                 `json:"-"`
	doc         *doc.Type                         `json:"-"`

	// Comment extraction state
	commentsExtracted bool
}

// NewNamedTypeInfo creates a new type info with eager basic data and lazy details
func NewNamedTypeInfo(kind TypeKind, name string, pkg string, loader func() (*DetailedTypeInfo, error)) *NamedTypeInfo {
	descriptor := name
	if pkg != "" {
		descriptor = pkg + "." + name
	}
	return &NamedTypeInfo{
		Kind:       kind,
		Name:       name,
		Package:    pkg,
		Descriptor: descriptor,
		loader:     loader,
	}
}

// NewNamedTypeInfoFromTypes creates a new type info with type objects for unified comment extraction
func NewNamedTypeInfoFromTypes(kind TypeKind, typesObj types.Object, pkgInfo *packages.Package, docType *doc.Type, loader func() (*DetailedTypeInfo, error)) *NamedTypeInfo {
	name := ""
	pkg := ""
	descriptor := ""

	if typesObj != nil {
		name = typesObj.Name()
		if typesObj.Pkg() != nil {
			pkg = typesObj.Pkg().Path()
			descriptor = pkg + "." + name
		} else {
			descriptor = name
		}
	}

	return &NamedTypeInfo{
		Kind:       kind,
		Name:       name,
		Package:    pkg,
		Descriptor: descriptor,
		obj:        typesObj,
		pkg:        pkgInfo,
		doc:        docType,
		loader:     loader,
		// Comments and Annotations will be lazy-loaded
	}
}

// Eagerly loaded methods (always available)
func (nt *NamedTypeInfo) GetKind() TypeKind {
	return nt.Kind
}

func (nt *NamedTypeInfo) GetName() string {
	return nt.Name
}

func (nt *NamedTypeInfo) GetPackage() string {
	return nt.Package
}

func (nt *NamedTypeInfo) GetCannonicalName() string {
	if nt.GetPackage() == "" {
		return nt.GetName()
	}

	return nt.GetPackage() + "." + nt.GetName()
}

func (nt *NamedTypeInfo) GetAnnotations() []gonnotation.Annotation {
	nt.extractCommentsAndAnnotations()
	return nt.Annotations
}

// extractCommentsAndAnnotations lazily extracts comments and annotations from type objects
func (nt *NamedTypeInfo) extractCommentsAndAnnotations() {
	if nt.commentsExtracted {
		return
	}

	var docString string
	if nt.doc != nil {
		docString = nt.doc.Doc
	}

	nt.Comments = parseComments(docString)
	nt.Annotations = parseAnnotations(docString)
	nt.commentsExtracted = true
}

func (nt *NamedTypeInfo) GetComments() []string {
	nt.extractCommentsAndAnnotations()
	return nt.Comments
}

// Anonymous type support
func (nt *NamedTypeInfo) IsAnonymous() bool {
	return false // Named types are never anonymous
}

func (nt *NamedTypeInfo) GetTypeDescriptor() string {
	return nt.Descriptor // Use the stored descriptor
}

// Lazy loading method
func (nt *NamedTypeInfo) Load() (*DetailedTypeInfo, error) {
	nt.detailsOnce.Do(func() {
		if nt.loader != nil {
			details, err := nt.loader()
			if err != nil {
				nt.detailsErr = err
				return
			}
			// Populate flattened fields from details
			if details != nil {
				nt.TypeParameters = details.TypeParameters
				nt.Fields = details.Fields
				nt.Methods = details.Methods
				nt.MapFlag = details.MapFlag
				nt.KeyType = details.KeyType
				nt.KeyTypePtrFlag = details.KeyTypePtrFlag
				nt.ValueType = details.ValueType
				nt.ValueTypePtrFlag = details.ValueTypePtrFlag
				nt.SliceFlag = details.SliceFlag
				nt.ElementType = details.ElementType
				nt.ChanFlag = details.ChanFlag
				nt.ChanDir = details.ChanDir
				nt.EnumValues = details.EnumValues

				// For enums, ensure all enum values have their comments and annotations loaded
				if nt.Kind == TypeKindEnum && len(details.EnumValues) > 0 {
					for i := range details.EnumValues {
						// Trigger lazy loading of comments and annotations for each enum value
						_ = details.EnumValues[i].GetComments()
						_ = details.EnumValues[i].GetAnnotations()
					}
				}

				// For structs, ensure all field comments and annotations are loaded
				if nt.Kind == TypeKindStruct && len(details.Fields) > 0 {
					for i := range details.Fields {
						// Trigger lazy loading of comments and annotations for each field
						_ = details.Fields[i].GetComments()
						_ = details.Fields[i].GetAnnotations()
					}
				}
			}
		}
	})

	if nt.detailsErr != nil {
		return nil, nt.detailsErr
	}

	// Return a DetailedTypeInfo struct that references the flattened fields
	// for backward compatibility
	return &DetailedTypeInfo{
		TypeParameters:   nt.TypeParameters,
		Fields:           nt.Fields,
		Methods:          nt.Methods,
		MapFlag:          nt.MapFlag,
		KeyType:          nt.KeyType,
		KeyTypePtrFlag:   nt.KeyTypePtrFlag,
		ValueType:        nt.ValueType,
		ValueTypePtrFlag: nt.ValueTypePtrFlag,
		SliceFlag:        nt.SliceFlag,
		ElementType:      nt.ElementType,
		ChanFlag:         nt.ChanFlag,
		ChanDir:          nt.ChanDir,
		EnumValues:       nt.EnumValues, // Include enum values
	}, nil
}

// Convenience methods that trigger lazy loading when needed
func (nt *NamedTypeInfo) IsMap() bool {
	_, err := nt.Load() // Trigger loading
	if err != nil {
		return false
	}
	return nt.MapFlag
}

func (nt *NamedTypeInfo) GetMapKeyType() TypeInfo {
	_, err := nt.Load() // Trigger loading
	if err != nil {
		return nil
	}
	return nt.KeyType
}

func (nt *NamedTypeInfo) IsMapKeyPointer() bool {
	_, err := nt.Load() // Trigger loading
	if err != nil {
		return false
	}
	return nt.KeyTypePtrFlag
}

func (nt *NamedTypeInfo) IsSlice() bool {
	_, err := nt.Load() // Trigger loading
	if err != nil {
		return false
	}
	return nt.SliceFlag
}

func (nt *NamedTypeInfo) GetElementType() TypeInfo {
	_, err := nt.Load() // Trigger loading
	if err != nil {
		return nil
	}
	return nt.ElementType
}

func (nt *NamedTypeInfo) IsElementPointer() bool {
	_, err := nt.Load() // Trigger loading
	if err != nil {
		return false
	}
	return nt.ValueTypePtrFlag
}

func (nt *NamedTypeInfo) IsBasic() bool {
	// Basic types check - this is lightweight, no need for lazy loading
	switch nt.Name {
	case "int", "int8", "int16", "int32", "int64",
		"uint", "uint8", "uint16", "uint32", "uint64",
		"float32", "float64", "complex64", "complex128",
		"byte", "rune", "string", "bool":
		return true
	default:
		return false
	}
}

func (nt *NamedTypeInfo) IsChannel() bool {
	_, err := nt.Load() // Trigger loading
	if err != nil {
		return false
	}
	return nt.ChanFlag
}

func (nt *NamedTypeInfo) IsGeneric() bool {
	_, err := nt.Load() // Trigger loading
	return err == nil && len(nt.TypeParameters) > 0
}

func (nt *NamedTypeInfo) GetTypeParameters() ([]TypeParameterInfo, error) {
	_, err := nt.Load() // Trigger loading
	if err != nil {
		return nil, err
	}
	return nt.TypeParameters, nil
}

// AnonymousTypeInfo represents composite/anonymous types like maps, slices, etc.
type AnonymousTypeInfo struct {
	Kind       TypeKind
	Descriptor string // For display/debug: "map[string][]User", "chan *[]User", etc.

	// For composite structure - use references for named types, inline for anonymous
	ElementTypeRef   string             `json:"ElementTypeRef,omitempty"`   // Reference to named element type
	ElementTypeInfo  *AnonymousTypeInfo `json:"ElementTypeInfo,omitempty"`  // Inline anonymous element type
	ElementIsPointer bool               `json:"ElementIsPointer,omitempty"` // Always include, even if false
	ElementKind      TypeKind           `json:"ElementKind,omitempty"`

	KeyTypeRef   string             `json:"KeyTypeRef,omitempty"`   // Reference to named key type
	KeyTypeInfo  *AnonymousTypeInfo `json:"KeyTypeInfo,omitempty"`  // Inline anonymous key type
	KeyIsPointer bool               `json:"KeyIsPointer,omitempty"` // Always include, even if false
	KeyKind      TypeKind           `json:"KeyKind,omitempty"`

	// Channel direction
	ChanDir ChannelDirection `json:"ChanDir,omitempty"`

	// For anonymous structs
	Fields  []FieldInfo  `json:"Fields,omitempty"`  // Fields for anonymous struct types
	Methods []MethodInfo `json:"Methods,omitempty"` // Methods for anonymous interface types
} // NewAnonymousTypeInfo creates a new anonymous type info
func NewAnonymousTypeInfo(kind TypeKind, descriptor string) *AnonymousTypeInfo {
	return &AnonymousTypeInfo{
		Kind:       kind,
		Descriptor: descriptor,
	}
}

// TypeInfo interface implementation for AnonymousTypeInfo
func (at *AnonymousTypeInfo) GetKind() TypeKind {
	return at.Kind
}

func (at *AnonymousTypeInfo) GetName() string {
	return "" // Anonymous types don't have names
}

func (at *AnonymousTypeInfo) GetPackage() string {
	return "" // Anonymous types don't have packages
}

func (at *AnonymousTypeInfo) GetCannonicalName() string {
	return "" // Anonymous types don't have canonical names
}

func (at *AnonymousTypeInfo) GetAnnotations() []gonnotation.Annotation {
	return nil // Anonymous types don't have annotations
}

func (at *AnonymousTypeInfo) GetComments() []string {
	return nil // Anonymous types don't have comments
}

func (at *AnonymousTypeInfo) IsAnonymous() bool {
	return true
}

func (at *AnonymousTypeInfo) GetTypeDescriptor() string {
	return at.Descriptor
}

func (at *AnonymousTypeInfo) Load() (*DetailedTypeInfo, error) {
	return nil, nil // Anonymous types don't use DetailedTypeInfo
}

// Convenience methods
func (at *AnonymousTypeInfo) IsMap() bool {
	return at.Kind == TypeKindMap
}

func (at *AnonymousTypeInfo) GetMapKeyType() TypeInfo {
	return nil // Not applicable for anonymous types
}

func (at *AnonymousTypeInfo) IsMapKeyPointer() bool {
	return at.KeyIsPointer
}

func (at *AnonymousTypeInfo) IsSlice() bool {
	return at.Kind == TypeKindSlice
}

func (at *AnonymousTypeInfo) GetElementType() TypeInfo {
	return nil // Not applicable for anonymous types
}

func (at *AnonymousTypeInfo) IsElementPointer() bool {
	return at.ElementIsPointer
}

func (at *AnonymousTypeInfo) IsBasic() bool {
	return at.Kind == TypeKindBasic
}

func (at *AnonymousTypeInfo) IsChannel() bool {
	return at.Kind == TypeKindChannel
}

func (at *AnonymousTypeInfo) IsGeneric() bool {
	return false // Anonymous types are not generic
}

func (at *AnonymousTypeInfo) GetTypeParameters() ([]TypeParameterInfo, error) {
	return nil, nil // Anonymous types don't have type parameters
}

// Specialized type structs that embed NamedTypeInfo
type StructInfo struct {
	*NamedTypeInfo
}

// Helper method to get struct-specific details
func (si *StructInfo) GetFields() ([]FieldInfo, error) {
	details, err := si.Load()
	if err != nil || details == nil {
		return nil, err
	}
	return details.Fields, nil
}

func (si *StructInfo) GetMethods() ([]MethodInfo, error) {
	details, err := si.Load()
	if err != nil || details == nil {
		return nil, err
	}
	return details.Methods, nil
}

type FieldInfo struct {
	Name        string   `json:"Name,omitempty"`
	TypeRef     string   `json:"TypeRef,omitempty"`     // JSON-safe reference to TypeInfo
	TypeKind    TypeKind `json:"TypeKind,omitempty"`    // Kind of the field type
	IsPointer   bool     `json:"IsPointer,omitempty"`   // Whether the field type is a pointer
	IsAnonymous bool     `json:"IsAnonymous,omitempty"` // Whether the field type is anonymous/inline

	// For slices and arrays
	ElementTypeRef     string   `json:"ElementTypeRef,omitempty"`     // JSON-safe reference to ElementType - FINAL element type only (never composite)
	ElementIsPointer   bool     `json:"ElementIsPointer,omitempty"`   // Whether element type is a pointer
	ElementIsAnonymous bool     `json:"ElementIsAnonymous,omitempty"` // Whether element type is anonymous/inline
	ElementKind        TypeKind `json:"ElementKind,omitempty"`        // Kind of the element type
	ElementStructure   string   `json:"ElementStructure,omitempty"`   // Describes nesting structure: "[]", "[][][]", "[5][10]", etc.
	elementTypeInfo    TypeInfo
	// Inline type info for anonymous/composite element types
	ElementTypeInfo *AnonymousTypeInfo `json:"ElementTypeInfo,omitempty"`

	// For maps
	KeyTypeRef     string   `json:"KeyTypeRef,omitempty"`     // JSON-safe reference to KeyType
	KeyIsPointer   bool     `json:"KeyIsPointer,omitempty"`   // Whether key type is a pointer
	KeyIsAnonymous bool     `json:"KeyIsAnonymous,omitempty"` // Whether key type is anonymous/inline
	KeyKind        TypeKind `json:"KeyKind,omitempty"`        // Kind of the key type
	KeyStructure   string   `json:"KeyStructure,omitempty"`   // Describes key nesting if composite: "[]", "map[string]", etc.
	keyTypeInfo    TypeInfo
	// Inline type info for anonymous/composite key types
	KeyTypeInfo *AnonymousTypeInfo `json:"KeyTypeInfo,omitempty"`

	// For channels
	ChanDir ChannelDirection `json:"ChanDir,omitempty"` // Channel direction: "both", "send", "recv"

	// Inline type info for anonymous/composite main field types
	InlineTypeInfo *AnonymousTypeInfo `json:"TypeInfo,omitempty"`

	// Struct tags
	Tags map[string]string `json:"Tags,omitempty"` // Parsed tag map: {"json": "name,omitempty", "xml": "name"}

	// Embedded/promoted field tracking
	IsPromoted      bool   `json:"IsPromoted,omitempty"`      // Whether this field is promoted from an embedded type
	PromotedFromRef string `json:"PromotedFromRef,omitempty"` // Full qualified name of the embedded type that promoted this field

	// Comments and annotations (lazy loaded)
	Comments    []string                 `json:"Comments,omitempty"`
	Annotations []gonnotation.Annotation `json:"Annotations,omitempty"`

	// Documentation (not exported to JSON) - lazy loaded
	docField          *ast.Field `json:"-"`
	commentsExtracted bool       `json:"-"`

	typeInfo TypeInfo
}

// GetComments extracts comments from field documentation
func (fi *FieldInfo) GetComments() []string {
	if !fi.commentsExtracted {
		fi.extractFieldCommentsAndAnnotations()
	}
	return fi.Comments
}

// GetAnnotations extracts annotations from field documentation
func (fi *FieldInfo) GetAnnotations() []gonnotation.Annotation {
	if !fi.commentsExtracted {
		fi.extractFieldCommentsAndAnnotations()
	}
	return fi.Annotations
}

// extractFieldCommentsAndAnnotations lazily extracts comments and annotations from field doc
func (fi *FieldInfo) extractFieldCommentsAndAnnotations() {
	if fi.commentsExtracted {
		return
	}

	var docString string
	if fi.docField != nil && fi.docField.Doc != nil {
		docString = fi.docField.Doc.Text()
	}

	fi.Comments = parseComments(docString)
	fi.Annotations = parseAnnotations(docString)
	fi.commentsExtracted = true
}

type MethodInfo struct {
	*NamedTypeInfo

	// Receiver information
	ReceiverTypeRef   string `json:"ReceiverTypeRef"`        // Type that owns this method
	IsPointerReceiver bool   `json:"IsPointerReceiver"`      // true for *T, false for T
	ReceiverName      string `json:"ReceiverName,omitempty"` // receiver variable name

	// Method signature
	Parameters []ParameterInfo `json:"Parameters,omitempty"` // Method parameters
	Returns    []ReturnInfo    `json:"Returns,omitempty"`    // Return values
	IsVariadic bool            `json:"IsVariadic"`           // Has ...args parameter

	// Method context
	IsInterfaceMethod bool `json:"IsInterfaceMethod"` // true if from interface, false if concrete

	// Embedded/promoted method tracking
	IsPromoted      bool   `json:"IsPromoted,omitempty"`      // Whether this method is promoted from an embedded type
	PromotedFromRef string `json:"PromotedFromRef,omitempty"` // Full qualified name of the embedded type that promoted this method
}

// ParameterInfo represents a method or function parameter
type ParameterInfo struct {
	BaseTypeDetailInfo
	IsVariadicParam bool `json:"IsVariadic"` // Is ...args parameter - using different field name to avoid method conflict
}

// Override IsVariadic method for ParameterInfo
func (p *ParameterInfo) IsVariadic() bool {
	return p.IsVariadicParam
}

// ReturnInfo represents a method or function return value
type ReturnInfo struct {
	BaseTypeDetailInfo
}

// FunctionInfo represents a standalone function (not a method)
type FunctionInfo struct {
	*NamedTypeInfo

	// Function signature
	Parameters []ParameterInfo `json:"Parameters,omitempty"` // Function parameters
	Returns    []ReturnInfo    `json:"Returns,omitempty"`    // Return values
	IsVariadic bool            `json:"IsVariadic"`           // Has ...args parameter

	// Documentation (not exported to JSON)
	docFunc *doc.Func `json:"-"`
}

// GetComments extracts comments from function documentation
func (fi *FunctionInfo) GetComments() []string {
	if !fi.commentsExtracted {
		fi.extractFunctionCommentsAndAnnotations()
	}
	return fi.Comments
}

// GetAnnotations extracts annotations from function documentation
func (fi *FunctionInfo) GetAnnotations() []gonnotation.Annotation {
	if !fi.commentsExtracted {
		fi.extractFunctionCommentsAndAnnotations()
	}
	return fi.Annotations
}

// extractFunctionCommentsAndAnnotations lazily extracts comments and annotations from doc.Func
func (fi *FunctionInfo) extractFunctionCommentsAndAnnotations() {
	if fi.commentsExtracted {
		return
	}

	var docString string
	if fi.docFunc != nil {
		docString = fi.docFunc.Doc
	}

	fi.Comments = parseComments(docString)
	fi.Annotations = parseAnnotations(docString)
	fi.commentsExtracted = true
}

type EnumInfo struct {
	*NamedTypeInfo
	EnumTypeRef string `json:"EnumTypeRef,omitempty"`
}

type EnumValue struct {
	Name  string `json:"Name"`
	Value any    `json:"Value"`

	// Comments and annotations (lazy loaded)
	Comments    []string                 `json:"Comments,omitempty"`
	Annotations []gonnotation.Annotation `json:"Annotations,omitempty"`

	// Documentation (not exported to JSON) - lazy loaded
	docComment        *ast.Comment `json:"-"`
	commentsExtracted bool         `json:"-"`
}

// GetComments extracts comments from enum value documentation
func (ev *EnumValue) GetComments() []string {
	if !ev.commentsExtracted {
		ev.extractCommentsAndAnnotations()
	}
	return ev.Comments
}

// GetAnnotations extracts annotations from enum value documentation
func (ev *EnumValue) GetAnnotations() []gonnotation.Annotation {
	if !ev.commentsExtracted {
		ev.extractCommentsAndAnnotations()
	}
	return ev.Annotations
}

// extractCommentsAndAnnotations lazily extracts comments and annotations from enum value doc
func (ev *EnumValue) extractCommentsAndAnnotations() {
	if ev.commentsExtracted {
		return
	}

	var docString string
	if ev.docComment != nil {
		docString = ev.docComment.Text
	}

	ev.Comments = parseComments(docString)
	ev.Annotations = parseAnnotations(docString)
	ev.commentsExtracted = true
}

func NewEnumInfo(name string, pkg string, enumTypeRef string, loader func() (*DetailedTypeInfo, error)) *EnumInfo {
	return &EnumInfo{
		NamedTypeInfo: NewNamedTypeInfo(TypeKindEnum, name, pkg, loader),
		EnumTypeRef:   enumTypeRef,
	}
}

// NewEnumInfoFromTypes creates an EnumInfo using type objects for unified comment extraction
func NewEnumInfoFromTypes(name string, pkg string, enumTypeRef string, typesObj types.Object, pkgInfo *packages.Package, docType *doc.Type, loader func() (*DetailedTypeInfo, error)) *EnumInfo {
	return &EnumInfo{
		NamedTypeInfo: NewNamedTypeInfoFromTypes(TypeKindEnum, typesObj, pkgInfo, docType, loader),
		EnumTypeRef:   enumTypeRef,
	}
}

func (ei *EnumInfo) GetValues() ([]EnumValue, error) {
	details, err := ei.Load()
	if err != nil || details == nil {
		return nil, err
	}
	return details.EnumValues, nil
}

type InterfaceInfo struct {
	*NamedTypeInfo
}

func NewInterfaceInfo(name string, pkg string, loader func() (*DetailedTypeInfo, error)) *InterfaceInfo {
	return &InterfaceInfo{
		NamedTypeInfo: NewNamedTypeInfo(TypeKindInterface, name, pkg, loader),
	}
}

// NewInterfaceInfoFromTypes creates an InterfaceInfo using type objects for unified comment extraction
func NewInterfaceInfoFromTypes(typesObj types.Object, pkgInfo *packages.Package, docType *doc.Type, loader func() (*DetailedTypeInfo, error)) *InterfaceInfo {
	return &InterfaceInfo{
		NamedTypeInfo: NewNamedTypeInfoFromTypes(TypeKindInterface, typesObj, pkgInfo, docType, loader),
	}
}

func (ii *InterfaceInfo) GetMethods() ([]MethodInfo, error) {
	details, err := ii.Load()
	if err != nil || details == nil {
		return nil, err
	}
	return details.Methods, nil
}

func NewFunctionInfo(name string, pkg string, loader func() (*DetailedTypeInfo, error)) *FunctionInfo {
	return &FunctionInfo{
		NamedTypeInfo: NewNamedTypeInfo(TypeKindFunction, name, pkg, loader),
	}
}

func NewMethodInfo(name string, pkg string, receiverTypeRef string, isPointerReceiver bool, loader func() (*DetailedTypeInfo, error)) *MethodInfo {
	// Create descriptor as receiverType.methodName
	descriptor := receiverTypeRef + "." + name

	result := &MethodInfo{
		NamedTypeInfo:     NewNamedTypeInfo(TypeKindMethod, name, pkg, loader),
		ReceiverTypeRef:   receiverTypeRef,
		IsPointerReceiver: isPointerReceiver,
		Parameters:        []ParameterInfo{},
		Returns:           []ReturnInfo{},
		IsVariadic:        false,
		IsInterfaceMethod: false,
	}

	// Fix the descriptor after creation
	result.NamedTypeInfo.Descriptor = descriptor
	return result
}

type VariableInfo struct {
	*NamedTypeInfo
}
