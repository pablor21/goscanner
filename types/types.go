package types

import (
	"go/doc"
	"go/types"

	"golang.org/x/tools/go/packages"
)

type TypeKind string

const (
	TypeKindStruct        TypeKind = "struct"
	TypeKindEnum          TypeKind = "enum"
	TypeKindField         TypeKind = "field"
	TypeKindInterface     TypeKind = "interface"
	TypeKindFunction      TypeKind = "function"
	TypeKindMethod        TypeKind = "method"
	TypeKindVariable      TypeKind = "variable"
	TypeKindConstant      TypeKind = "constant"
	TypeKindMap           TypeKind = "map"
	TypeKindSlice         TypeKind = "slice"
	TypeKindUnion         TypeKind = "union"
	TypeKindApproximation TypeKind = "approximation"
	TypeKindArray         TypeKind = "array"
	TypeKindChannel       TypeKind = "channel"
	TypeKindBasic         TypeKind = "basic"        // For built-in types like string, int, bool
	TypeKindAlias         TypeKind = "alias"        // For type aliases
	TypeKindNamed         TypeKind = "named"        // For named types with underlying basic types
	TypeKindPointer       TypeKind = "pointer"      // For named types with underlying types
	TypeKindGeneric       TypeKind = "generic"      // For generic types like List[T], Map[K,V], etc.
	TypeKindGenericParam  TypeKind = "genericParam" // For generic type parameters like T, U, etc.
	TypeKindUnknown       TypeKind = "unknown"      // For unrecognized types
)

type ChannelDirection string

const (
	ChanDirBoth ChannelDirection = "both" // chan T (bidirectional)
	ChanDirSend ChannelDirection = "send" // chan<- T (send-only)
	ChanDirRecv ChannelDirection = "recv" // <-chan T (receive-only)
)

type TypeParameterConstraintKind string

const (
	ConstraintKindInterface     TypeParameterConstraintKind = "interface"
	ConstraintKindUnion         TypeParameterConstraintKind = "union"
	ConstraintKindApproximation TypeParameterConstraintKind = "approximation"
)

type Visibility uint

const (
	VisibilityLevelUnknown Visibility = 0
	VisibilityUnexported   Visibility = 1
	VisibilityExported     Visibility = 2
)

// MarshalJSON marshals the Visibility to JSON
func (v Visibility) MarshalJSON() ([]byte, error) {
	switch v {
	case VisibilityExported:
		return []byte(`"exported"`), nil
	case VisibilityUnexported:
		return []byte(`"unexported"`), nil
	default:
		return []byte(`"unknown"`), nil
	}
}

func (v *Visibility) UnmarshalJSON(data []byte) error {
	str := string(data)
	switch str {
	case `"exported"`:
		*v = VisibilityExported
	case `"unexported"`:
		*v = VisibilityUnexported
	default:
		*v = VisibilityLevelUnknown
	}
	return nil
}

func (v Visibility) String() string {
	switch v {
	case VisibilityExported:
		return "exported"
	case VisibilityUnexported:
		return "unexported"
	default:
		return "unknown"
	}
}

// DetailsLoaderFn is a function type for loading type details
type DetailsLoaderFn func(ti Type) error

type Loadable interface {
	Load() error
	SetDetailsLoader(loader DetailsLoaderFn)
}

// Type represents a type entry
type Type interface {
	Id() string
	Name() string
	Kind() TypeKind
	// Gets the package path of the type entry
	PackagePath() string
	// Gets the go/doc.Type associated with the type entry
	Type() *doc.Type
	// Gets the Object associated with the type entry
	Object() types.Object
	// Gets the packages.Package
	Package() *packages.Package
	// Gets the visibility level of the type entry
	Visibility() Visibility
}

type ValueType interface {
	Type
	// Gets the parent type of the value entry (enum or variable)
	Parent() Type
	// Gets the constant value of the value entry
	Value() any
	// Gets the underlying type of the value entry
	ValueType() Type
}

// NamedType represents a named type entry
// e.g., struct, interface, or type alias
type NamedType interface {
	// Embeds TypeInfo (a named type is a type with a package and name)
	Type
	// Gets the documentation comments of the type entry
	Comments() []string
	// Gets the basic info of the named type entry (useful for embedding)
	GetBasicInfo() *BasicTypeInfo
}

// TypeReference represents a reference to a type entry
type TypeReference interface {
	TypeRefId() string
	TypeRef() Type
	IsPointer() bool
	PointerIndirections() int
}

// HasElementType represents types that have an element type
// e.g., slices, arrays, pointers, or maps
type HasElementType interface {
	Type
	ElementType() TypeReference
}

// HasKeyType represents types that have a key type
// e.g., maps
type HasKeyType interface {
	Type
	KeyType() TypeReference
}

// HasMethods represents types that have methods
// e.g., interfaces, structs
type HasMethods interface {
	Methods() []*MethodInfo
}

// HasFields represents types that have fields
// e.g., structs
type HasFields interface {
	Fields() []*FieldInfo
}

type FieldInfo struct {
	NamedTypeInfo
	TypeRef    TypeReference `json:"typeRef,omitempty"`
	IsEmbedded bool          `json:"isEmbedded,omitempty"`
	Tag        string        `json:"tag,omitempty"`
}

func (f *FieldInfo) Type() *doc.Type {
	return f.doc
}

func (f *FieldInfo) TypeReference() TypeReference {
	return f.TypeRef
}

// Load loads the details of the field entry
func (f *FieldInfo) Load() error {
	var loadErr error
	f.loadOnce.Do(func() {
		if f.detailsLoader != nil {
			loadErr = f.detailsLoader(f)
		}
	})
	return loadErr
}
