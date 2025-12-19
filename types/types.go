package types

import (
	"go/doc"
	"go/types"
	"sync"
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

// TypeKind represents the kind of a type
type TypeKind string

const (
	TypeKindStruct        TypeKind = "struct"
	TypeKindInterface     TypeKind = "interface"
	TypeKindFunction      TypeKind = "function"
	TypeKindMethod        TypeKind = "method"
	TypeKindField         TypeKind = "field"
	TypeKindBasic         TypeKind = "basic"
	TypeKindAlias         TypeKind = "alias"
	TypeKindPointer       TypeKind = "pointer"
	TypeKindSlice         TypeKind = "slice"
	TypeKindArray         TypeKind = "array"
	TypeKindMap           TypeKind = "map"
	TypeKindChan          TypeKind = "chan"
	TypeKindEnum          TypeKind = "enum"
	TypeKindConstant      TypeKind = "constant"
	TypeKindVariable      TypeKind = "variable"
	TypeKindTypeParameter TypeKind = "type_parameter"
	TypeKindUnion         TypeKind = "union"
	TypeKindInstantiated  TypeKind = "instantiated"
	TypeKindUnknown       TypeKind = ""
)

// ChannelDirection represents the direction of a channel
type ChannelDirection string

const (
	ChanDirBoth ChannelDirection = "both" // chan T (bidirectional)
	ChanDirSend ChannelDirection = "send" // chan<- T (send-only)
	ChanDirRecv ChannelDirection = "recv" // <-chan T (receive-only)
)

type LoaderFn func(Type) error

// Serializable represents a type that can be serialized
type Serializable interface {
	// Serialize returns a serializable representation of this type
	Serialize() any
}

type Loadable interface {
	// Load lazily loads the type details
	Load() error

	// SetLoader sets the loader function
	SetLoader(loader func(Type) error)
}

type HasMethods interface {
	// Methods returns the methods of this type
	Methods() []*Method

	// AddMethod adds methods to this type
	AddMethods(methods ...*Method)
}

// Type is the base interface that all types implement
type Type interface {
	// Id returns the canonical identifier for this type
	Id() string

	// Name returns the display name of this type
	Name() string

	// Kind returns the kind of this type
	Kind() TypeKind

	// IsNamed returns true if this is a named type
	IsNamed() bool

	// Package returns the package this type belongs to
	Package() *Package

	// Object returns the go/types.Object associated with this type
	Object() types.Object
	// SetObject sets the go/types.Object for this type
	SetObject(obj types.Object)

	// Doc returns the go/doc.Type documentation for this type
	Doc() *doc.Type

	// SetObject sets the go/types.Object for this type
	SetDoc(docType *doc.Type)

	// Comments returns the documentation comments for this type
	Comments() []Comment

	// SetPackage sets the package for this type
	SetPackage(pkg *Package)

	// Files returns the files where this type is defined
	Files() []string

	// SetFiles sets the files where this type is defined
	SetFiles(files []string)

	// Exported returns true if this type is exported
	Exported() bool

	// SetExported sets whether this type is exported
	SetExported(exported bool)

	// Distance returns the distance from scanned packages
	Distance() int

	// SetDistance sets the distance from scanned packages
	SetDistance(distance int)

	// SetGoType sets the original go/types.Type (used for unnamed types)
	SetGoType(t types.Type)

	// GoType returns the original go/types.Type (used for unnamed types)
	GoType() types.Type

	// Serializable implements
	Serializable

	// Loadable implements
	Loadable

	// HasMethods implements
	HasMethods
}

// TypesCol is a specialized SyncMap for Serializable types with string keys.
// It embeds SyncMap and adds a Serialize method.
type TypesCol[T Serializable] struct {
	*SyncMap[string, T]
}

// Serialize converts the collection to a serializable map format.
func (c *TypesCol[T]) Serialize() any {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := make(map[string]any, len(c.values))
	for id, t := range c.values {
		result[id] = t.Serialize()
	}
	return result
}

// NewTypesCol creates a new TypesCol instance.
func NewTypesCol[T Serializable]() *TypesCol[T] {
	return &TypesCol[T]{
		SyncMap: NewSyncMap[string, T](),
	}
}

// baseType contains common fields for all types
type baseType struct {
	id             string
	name           string
	kind           TypeKind
	pkg            *Package
	obj            types.Object
	goType         types.Type // Original go/types.Type for structure (used for unnamed types)
	docType        *doc.Type
	comments       []Comment
	methods        []*Method
	loader         LoaderFn
	loadOnce       sync.Once
	commentId      string
	commentsLoaded bool
	files          []string // Files where this type is defined
	exported       bool     // Whether this type is exported
	distance       int      // Distance from scanned packages (0 = in scanned package, 1 = direct dependency, etc.)
}

// newBaseType creates a new base type
func newBaseType(id string, name string, kind TypeKind) baseType {
	return baseType{
		id:        id,
		name:      name,
		commentId: name,
		kind:      kind,
		comments:  []Comment{},
		methods:   []*Method{},
		loadOnce:  sync.Once{},
	}
}

// Id returns the canonical identifier
func (b *baseType) Id() string {
	return b.id
}

// Name returns the display name
func (b *baseType) Name() string {
	return b.name
}

// Kind returns the type kind
func (b *baseType) Kind() TypeKind {
	return b.kind
}

// IsNamed returns true if this type has an associated Object
func (b *baseType) IsNamed() bool {
	return b.obj != nil
}

// Package returns the package
func (b *baseType) Package() *Package {
	return b.pkg
}

// Object returns the go/types.Object
func (b *baseType) Object() types.Object {
	return b.obj
}

// Doc returns the go/doc.Type
func (b *baseType) Doc() *doc.Type {
	return b.docType
}

// Comments returns the documentation comments
func (b *baseType) Comments() []Comment {
	return b.comments
}

// SetPackage sets the package
func (b *baseType) SetPackage(pkg *Package) {
	b.pkg = pkg
	b.commentsLoaded = false
	// clean previous comments
	b.comments = []Comment{}
}

// Files returns the files where this type is defined
func (b *baseType) Files() []string {
	return b.files
}

// SetFiles sets the files where this type is defined
func (b *baseType) SetFiles(files []string) {
	b.files = files
}

// Exported returns true if this type is exported
func (b *baseType) Exported() bool {
	return b.exported
}

// SetExported sets whether this type is exported
func (b *baseType) SetExported(exported bool) {
	b.exported = exported
}

// Distance returns the distance from scanned packages
func (b *baseType) Distance() int {
	return b.distance
}

// SetDistance sets the distance from scanned packages
func (b *baseType) SetDistance(distance int) {
	b.distance = distance
}

// SetObject sets the go/types.Object
func (b *baseType) SetObject(obj types.Object) {
	b.obj = obj
}

// SetDoc sets the go/doc.Type
func (b *baseType) SetDoc(docType *doc.Type) {
	b.docType = docType
}

// SetGoType sets the original go/types.Type
func (b *baseType) SetGoType(t types.Type) {
	b.goType = t
}

// GoType returns the original go/types.Type
func (b *baseType) GoType() types.Type {
	return b.goType
}

func (b *baseType) Methods() []*Method {
	return b.methods
}

func (b *baseType) AddMethods(methods ...*Method) {
	if b.methods == nil {
		b.methods = []*Method{}
	}
	b.methods = append(b.methods, methods...)
}

// Load lazily loads type details using the loader function
func (b *baseType) Load() error {
	var err error
	b.loadOnce.Do(func() {
		b.loadComments(false)
		if b.loader != nil {
			err = b.loader(nil) // will be called with actual Type implementation
		}
	})
	return err
}

// SetLoader sets the loader function
func (b *baseType) SetLoader(loader func(Type) error) {
	b.loader = loader
}

func (b *baseType) loadComments(force bool) {
	if b.commentsLoaded && !force {
		return
	}
	// clean previous comments
	b.comments = []Comment{}

	if b.pkg == nil {
		b.commentsLoaded = true
		return
	}

	if b.commentId == "" {
		b.commentId = b.name
	}

	// load comments from AST if available, otherwise from doc
	if b.pkg != nil {
		pkgComments := b.pkg.GetComments(b.commentId)
		if len(pkgComments) > 0 {
			// Use AST comments (includes both doc and inline)
			b.comments = append(b.comments, pkgComments...)
		}
	}

	// fallback to go/doc comments
	if len(b.comments) == 0 && b.docType != nil {
		// Fallback to go/doc comments
		commentText := ExtractComments(b.docType.Doc)
		if commentText != "" {
			b.comments = append(b.comments, NewComment(commentText, CommentPlacementAbove))
		}
	}
	b.commentsLoaded = true
}
