package types

type TypeKind string

const (
	TypeKindStruct        TypeKind = "struct"
	TypeKindEnum          TypeKind = "enum"
	TypeKindField         TypeKind = "field"
	TypeKindInterface     TypeKind = "interface"
	TypeKindFunction      TypeKind = "function"
	TypeKindMethod        TypeKind = "method"
	TypeKindVariable      TypeKind = "variable"
	TypeKindMap           TypeKind = "map"
	TypeKindSlice         TypeKind = "slice"
	TypeKindUnion         TypeKind = "union"
	TypeKindApproximation TypeKind = "approximation"
	TypeKindArray         TypeKind = "array"
	TypeKindChannel       TypeKind = "channel"
	TypeKindBasic         TypeKind = "basic"        // For built-in types like string, int, bool
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

// DetailsLoaderFn is a function type for loading type details
type DetailsLoaderFn func(ti TypeEntry) error

type Loadable interface {
	Load() error
}

// TypeEntry represents a type entry
type TypeEntry interface {
	Loadable
	Id() string
	Name() string
	Kind() TypeKind
	Docs() []string
}

// NamedTypeEntry represents a named type entry
// e.g., struct, interface, or type alias
type NamedTypeEntry interface {
	Package() string
}

// TypeReference represents a reference to a type entry
type TypeReference interface {
	TypeRefId() string
	TypeRef() TypeEntry
	IsPointer() bool
	PointerIndirections() int
}

// HasElementType represents types that have an element type
// e.g., slices, arrays, pointers, or maps
type HasElementType interface {
	ElementType() TypeReference
}

// HasKeyType represents types that have a key type
// e.g., maps
type HasKeyType interface {
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
