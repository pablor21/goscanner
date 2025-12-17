package types

import (
	"go/doc"
	"go/types"
	"sync"
)

// NamedTypeInfo represents a named type entry
// e.g., struct, interface, or type alias
type NamedTypeInfo struct {
	*BasicTypeInfo
	*TypeRef
	PkgPath string `json:"packagePath,omitempty"`
	// CommentCol []Comment `json:"comments,omitempty"`
	// Underlying TypeReference `json:",inline,omitempty"`
	IsNamed bool          `json:"isNamed"`
	Methods []*MethodInfo `json:"methods,omitempty"`
	// lock to protect lazy loading
	loadOnce      sync.Once
	detailsLoader DetailsLoaderFn
}

// NewNamedTypeInfo creates a new NamedTypeInfo
// Usually you would use specific constructors like NewStructTypeInfo or NewInterfaceTypeInfo
func NewNamedTypeInfo(id string, kind TypeKind, obj types.Object, docType *doc.Type, pkg *Package, detailsLoader DetailsLoaderFn) *NamedTypeInfo {
	pkgPath := ""
	if pkg != nil {
		pkgPath = pkg.Path
	}

	// get the visibility level
	visibility := VisibilityExported
	if obj != nil && !obj.Exported() {
		visibility = VisibilityUnexported
	}

	bti := NewBasicTypeInfo(id, obj.Name(), kind)
	bti.Description = obj.String()
	bti.pkg = pkg
	bti.obj = obj
	bti.doc = docType
	bti.VisibilityLevel = visibility

	return &NamedTypeInfo{
		BasicTypeInfo: bti,
		IsNamed:       true,
		PkgPath:       pkgPath,
		loadOnce:      sync.Once{},
		detailsLoader: detailsLoader,
	}
}

// // Gets the package path of the type entry
// // Implements NamedType#PackagePath
// func (n *NamedTypeInfo) PackagePath() string {
// 	return n.PkgPath
// }

// // Gets the packages.Package
// // Implements NamedType#Package
// func (n *NamedTypeInfo) Package() *packages.Package {
// 	return n.pkg
// }

// Gets the documentation comments of the type entry
// Implements NamedType#Comments
func (n *NamedTypeInfo) Comments() []Comment {
	// trigger lazy load
	_ = n.Load()
	return n.CommentCol
}

// // Gets the go/doc.Type associated with the type entry
// // Implements NamedType#Type
// func (n *NamedTypeInfo) Type() *doc.Type {
// 	return n.doc
// }

// // Gets the Object associated with the type entry
// // Implements NamedType#Object
// func (n *NamedTypeInfo) Object() types.Object {
// 	// trigger lazy load
// 	return n.obj
// }

// Load loads the details of the named type entry
// Implements Loadable#Load
func (n *NamedTypeInfo) Load() error {
	var loadErr error
	n.loadOnce.Do(func() {

		// load comments and other details
		n.loadComments()

		if n.detailsLoader != nil {
			loadErr = n.detailsLoader(n)
		}

		if loadErr != nil {
			return
		}

		// load methods
		for _, method := range n.Methods {
			_ = method.Load() // ignore error for now
		}
	})
	return loadErr
}

func (n *NamedTypeInfo) SetDetailsLoader(loader DetailsLoaderFn) {
	n.detailsLoader = loader
}

// InterfaceTypeInfo represents an interface type entry
type InterfaceTypeInfo struct {
	*NamedTypeInfo
	Methods []*MethodInfo `json:"methods,omitempty"`
}

// NewInterfaceTypeInfo creates a new InterfaceTypeInfo
func NewInterfaceTypeInfo(id string, obj types.Object, docType *doc.Type, pkg *Package, detailsLoader DetailsLoaderFn) *InterfaceTypeInfo {
	return &InterfaceTypeInfo{
		NamedTypeInfo: NewNamedTypeInfo(id, TypeKindInterface, obj, docType, pkg, detailsLoader),
		Methods:       []*MethodInfo{},
	}
}

type FieldInfo struct {
	NamedTypeInfo
	*TypeRef `json:",inline,omitempty"`
	// Where the field was promoted from (if applicable)
	PromotedFromId string `json:"promotedFrom,omitempty"`
	Tag            string `json:"tag,omitempty"`
	promotedFrom   Type   `json:"-"`
}

func NewFieldInfo(id string, parent Type, obj types.Object, docType *doc.Type, pkg *Package, typeRef *TypeRef, tag string, promotedFrom Type, detailsLoader DetailsLoaderFn) *FieldInfo {
	promotedFromId := ""
	if promotedFrom != nil {
		promotedFromId = promotedFrom.Id()
	}
	fi := &FieldInfo{
		NamedTypeInfo:  *NewNamedTypeInfo(id, TypeKindField, obj, docType, pkg, detailsLoader),
		TypeRef:        typeRef,
		PromotedFromId: promotedFromId,
		Tag:            tag,
		promotedFrom:   promotedFrom,
	}
	fi.commentId = parent.Name() + "." + fi.DisplayName
	return fi
}

func (f *FieldInfo) PromotedFrom() Type {
	return f.promotedFrom
}

func (f *FieldInfo) IsPromoted() bool {
	return f.promotedFrom != nil
}

func (f *FieldInfo) GetTag() string {
	return f.Tag
}

func (f *FieldInfo) SetPromotedFrom(t Type) {
	f.promotedFrom = t
	if t != nil {
		f.PromotedFromId = t.Id()
	}
}

// StructTypeInfo represents a struct type entry
type StructTypeInfo struct {
	*NamedTypeInfo
	Methods []*MethodInfo `json:"methods,omitempty"`
	Fields  []*FieldInfo  `json:"fields,omitempty"`
}

// NewStructTypeInfo creates a new StructTypeInfo
func NewStructTypeInfo(id string, obj types.Object, docType *doc.Type, pkg *Package, detailsLoader DetailsLoaderFn) *StructTypeInfo {
	return &StructTypeInfo{
		NamedTypeInfo: NewNamedTypeInfo(id, TypeKindStruct, obj, docType, pkg, detailsLoader),
		Methods:       []*MethodInfo{},
		Fields:        []*FieldInfo{},
	}
}

func (s *StructTypeInfo) GetFields() []*FieldInfo {
	return s.Fields
}

// NamedCollectionTypeInfo represents a named collection type entry
//
// examples:
//
//	type MySlice []int
//	type MyArray [5]string
type NamedCollectionTypeInfo struct {
	*NamedTypeInfo
	Size             int64         `json:"size,omitempty"`
	ElementReference TypeReference `json:"elementType,omitempty"`
}

// NewNamedCollectionTypeInfo creates a new NamedCollectionTypeInfo
func NewNamedCollectionTypeInfo(id string, kind TypeKind, obj types.Object, pkg *Package, doc *doc.Type, elementType TypeReference, size int64, detailsLoader DetailsLoaderFn) *NamedCollectionTypeInfo {

	n := &NamedCollectionTypeInfo{
		NamedTypeInfo:    NewNamedTypeInfo(id, kind, obj, doc, pkg, detailsLoader),
		Size:             size,
		ElementReference: elementType,
	}
	return n
}

// ElementType returns the element type reference
// Implements HasElementType#ElementType
func (n *NamedCollectionTypeInfo) ElementType() TypeReference {
	return n.ElementReference
}

type NamedChannelTypeInfo struct {
	NamedTypeInfo
	ElementReference TypeReference    `json:"elementType,omitempty"`
	ChannelDir       ChannelDirection `json:"channelDirection,omitempty"`
}

// NewNamedChannelTypeInfo creates a new NamedChannelTypeInfo
func NewNamedChannelTypeInfo(id string, obj types.Object, doc *doc.Type, pkg *Package, elementType TypeReference, chanDir ChannelDirection, detailsLoader DetailsLoaderFn) *NamedChannelTypeInfo {

	return &NamedChannelTypeInfo{
		NamedTypeInfo:    *NewNamedTypeInfo(id, TypeKindChannel, obj, doc, pkg, detailsLoader),
		ElementReference: elementType,
		ChannelDir:       chanDir,
	}
}

// ElementType returns the element type reference
// Implements HasElementType#ElementType
func (b *NamedChannelTypeInfo) ElementType() TypeReference {
	return b.ElementReference
}

// ChannelDirection returns the channel direction
// Implements ChannelTypeInfo#ChannelDirection
func (b *NamedChannelTypeInfo) ChannelDirection() ChannelDirection {
	return b.ChannelDir
}
