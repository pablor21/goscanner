package typesnew

import (
	"go/doc"
	"go/types"
	"sync"
)

// TypeKind represents the kind of a type
type TypeKind string

const (
	TypeKindStruct    TypeKind = "struct"
	TypeKindInterface TypeKind = "interface"
	TypeKindFunction  TypeKind = "function"
	TypeKindMethod    TypeKind = "method"
	TypeKindField     TypeKind = "field"
	TypeKindBasic     TypeKind = "basic"
	TypeKindAlias     TypeKind = "alias"
	TypeKindPointer   TypeKind = "pointer"
	TypeKindSlice     TypeKind = "slice"
	TypeKindArray     TypeKind = "array"
	TypeKindMap       TypeKind = "map"
	TypeKindChan      TypeKind = "chan"
	TypeKindEnum      TypeKind = "enum"
	TypeKindConstant  TypeKind = "constant"
	TypeKindVariable  TypeKind = "variable"
	TypeKindUnknown   TypeKind = ""
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

	// Doc returns the go/doc.Type documentation for this type
	Doc() *doc.Type

	// Comments returns the documentation comments for this type
	Comments() []Comment

	// SetPackage sets the package for this type
	SetPackage(pkg *Package)

	// Serializable implements
	Serializable

	// Loadable implements
	Loadable
}

type TypesCol map[string]Type

func (c TypesCol) Get(id string) Type {
	return c[id]
}

func (c TypesCol) Set(id string, t Type) {
	c[id] = t
}

func (c TypesCol) Has(id string) bool {
	_, exists := c[id]
	return exists
}

func (c TypesCol) Delete(id string) {
	delete(c, id)
}

func (c TypesCol) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}

func (c TypesCol) Values() []Type {
	values := make([]Type, 0, len(c))
	for _, v := range c {
		values = append(values, v)
	}
	return values
}

func (c TypesCol) Len() int {
	return len(c)
}

func (c TypesCol) Clear() {
	for k := range c {
		delete(c, k)
	}
}

func (c TypesCol) Serialize() any {
	for _, t := range c {
		_ = t.Serialize()
	}
	return nil
}

func NewTypesCol() TypesCol {
	return make(TypesCol)
}

// baseType contains common fields for all types
type baseType struct {
	id             string
	name           string
	kind           TypeKind
	pkg            *Package
	obj            types.Object
	docType        *doc.Type
	comments       []Comment
	loader         LoaderFn
	loadOnce       sync.Once
	commentId      string
	commentsLoaded bool
}

// newBaseType creates a new base type
func newBaseType(id string, name string, kind TypeKind) baseType {
	return baseType{
		id:        id,
		name:      name,
		commentId: name,
		kind:      kind,
		comments:  []Comment{},
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
	if b.commentId == "" {
		b.commentId = b.name
	}
	// load comments from AST if available, otherwise from doc
	if b.pkg != nil && len(b.pkg.comments[b.commentId]) > 0 {
		// Use AST comments (includes both doc and inline)
		b.comments = append(b.comments, b.pkg.comments[b.commentId]...)
	} else if b.docType != nil {
		// Fallback to go/doc comments
		commentText := ExtractComments(b.docType.Doc)
		if commentText != "" {
			b.comments = append(b.comments, NewComment(commentText, CommentPlacementAbove))
		}
	}
	b.commentsLoaded = true
}
