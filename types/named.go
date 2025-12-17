package types

import (
	"go/doc"
	"go/types"
	"sync"

	"golang.org/x/tools/go/packages"
)

// NamedTypeInfo represents a named type entry
// e.g., struct, interface, or type alias
type NamedTypeInfo struct {
	BasicTypeInfo
	*TypeRef
	PkgPath    string   `json:"packagePath,omitempty"`
	CommentCol []string `json:"comments,omitempty"`
	// Underlying TypeReference `json:",inline,omitempty"`
	IsNamed bool          `json:"isNamed"`
	Methods []*MethodInfo `json:"methods,omitempty"`
	// lock to protect lazy loading
	loadOnce      sync.Once
	detailsLoader DetailsLoaderFn
}

// NewNamedTypeInfo creates a new NamedTypeInfo
// Usually you would use specific constructors like NewStructTypeInfo or NewInterfaceTypeInfo
func NewNamedTypeInfo(id string, kind TypeKind, obj types.Object, docType *doc.Type, pkg *packages.Package, detailsLoader DetailsLoaderFn) *NamedTypeInfo {
	pkgPath := ""
	if pkg != nil {
		pkgPath = pkg.PkgPath
	}

	// get the visibility level
	visibility := VisibilityExported
	if obj != nil && !obj.Exported() {
		visibility = VisibilityUnexported
	}

	bti := NewBasicTypeInfo(id, kind)
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
func (n *NamedTypeInfo) Comments() []string {
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
		// load comments
		if n.doc != nil {
			n.CommentCol = ExtractComments(n.doc.Doc)
		}
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

func (n *NamedTypeInfo) GetBasicInfo() *BasicTypeInfo {
	return &n.BasicTypeInfo
}

// InterfaceTypeInfo represents an interface type entry
type InterfaceTypeInfo struct {
	*NamedTypeInfo
	Methods []*MethodInfo `json:"methods,omitempty"`
}

// NewInterfaceTypeInfo creates a new InterfaceTypeInfo
func NewInterfaceTypeInfo(id string, obj types.Object, docType *doc.Type, pkg *packages.Package, detailsLoader DetailsLoaderFn) *InterfaceTypeInfo {
	return &InterfaceTypeInfo{
		NamedTypeInfo: NewNamedTypeInfo(id, TypeKindInterface, obj, docType, pkg, detailsLoader),
		Methods:       []*MethodInfo{},
	}
}

// StructTypeInfo represents a struct type entry
type StructTypeInfo struct {
	*NamedTypeInfo
	Methods []*MethodInfo `json:"methods,omitempty"`
	Fields  []*FieldInfo  `json:"fields,omitempty"`
}

// NewStructTypeInfo creates a new StructTypeInfo
func NewStructTypeInfo(id string, obj types.Object, docType *doc.Type, pkg *packages.Package, detailsLoader DetailsLoaderFn) *StructTypeInfo {
	return &StructTypeInfo{
		NamedTypeInfo: NewNamedTypeInfo(id, TypeKindStruct, obj, docType, pkg, detailsLoader),
		Methods:       []*MethodInfo{},
		Fields:        []*FieldInfo{},
	}
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
func NewNamedCollectionTypeInfo(id string, kind TypeKind, obj types.Object, pkg *packages.Package, doc *doc.Type, elementType TypeReference, size int64, detailsLoader DetailsLoaderFn) *NamedCollectionTypeInfo {

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
func NewNamedChannelTypeInfo(id string, kind TypeKind, obj types.Object, doc *doc.Type, pkg *packages.Package, elementType TypeReference, chanDir ChannelDirection, detailsLoader DetailsLoaderFn) *NamedChannelTypeInfo {

	return &NamedChannelTypeInfo{
		NamedTypeInfo:    *NewNamedTypeInfo(id, kind, obj, doc, pkg, detailsLoader),
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
