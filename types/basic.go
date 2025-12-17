package types

import (
	"go/doc"
	"go/types"
	"sync"

	"golang.org/x/tools/go/packages"
)

// BasicTypeInfo represents a basic type entry
type BasicTypeInfo struct {
	ID              string     `json:"id,omitempty"`
	DisplayName     string     `json:"name,omitempty"`
	TypeKind        TypeKind   `json:"kind,omitempty"`
	CommentCol      []Comment  `json:"comments,omitempty"`
	VisibilityLevel Visibility `json:"visibility,omitempty"`
	commentId       string     `json:"-"`                     // internal comment ID for lookup
	Description     string     `json:"description,omitempty"` // string representation of the type
	HierarchyDepth  int        `json:"depth,omitempty"`       // depth in the type hierarchy
	// lock to protect lazy loading
	loadOnce      sync.Once
	obj           types.Object
	doc           *doc.Type
	pkg           *Package // Custom package wrapper with AST comments
	detailsLoader DetailsLoaderFn
}

// NewBasicTypeInfo creates a new BasicTypeInfo
func NewBasicTypeInfo(id string, name string, kind TypeKind) *BasicTypeInfo {

	return &BasicTypeInfo{
		ID:          id,
		DisplayName: name,
		TypeKind:    kind,
		commentId:   name,
		// VisibilityLevel: VisibilityExported,
	}
}

// Gets the ID of the type entry
// Implements Type#Id
func (b *BasicTypeInfo) Id() string { return b.ID }

// Gets the name of the type entry
// Implements Type#Name
func (b *BasicTypeInfo) Name() string { return b.DisplayName }

// Gets the kind of the type entry
// Implements Type#Kind
func (b *BasicTypeInfo) Kind() TypeKind { return b.TypeKind }

// Gets the custom Package wrapper
// Implements Type#Package
func (b *BasicTypeInfo) Package() *Package { return b.pkg }

// Gets the documentation comments of the type entry
// Implements Type#Comments
func (b *BasicTypeInfo) Comments() []Comment { return b.CommentCol }

// Gets the depth of the type entry in the type hierarchy
// Implements Type#Depth
func (b *BasicTypeInfo) Depth() int { return b.HierarchyDepth }

// Sets the depth of the type entry in the type hierarchy
func (b *BasicTypeInfo) SetDepth(depth int) {
	b.HierarchyDepth = depth
}

// Gets the basic info of the type entry
// Implements Type#GetBasicInfo
func (b *BasicTypeInfo) GetBasicInfo() *BasicTypeInfo {
	return b
}

func (b *BasicTypeInfo) PkgPackage() *packages.Package {
	if b.Package() != nil {
		return b.Package().pkg
	}
	return nil
}

// Gets the go/doc.Type associated with the type entry
// Implements Type#Type
func (b *BasicTypeInfo) Type() *doc.Type { return b.doc }

// Gets the Object associated with the type entry
// Implements Type#Object
func (b *BasicTypeInfo) Object() types.Object { return b.obj }

// Gets the visibility level of the type entry
// Implements Type#Visibility
func (b *BasicTypeInfo) Visibility() Visibility {
	return b.VisibilityLevel
}

// Gets the package path of the type entry
// Implements Type#PackagePath
func (b *BasicTypeInfo) PackagePath() string {
	if b.pkg != nil {
		return b.PkgPackage().PkgPath
	}
	return ""
}

// SetPackageInfo sets the package info and loads AST comments
func (b *BasicTypeInfo) SetPackageInfo(pkgInfo *Package) {
	b.pkg = pkgInfo
	b.loadComments()
}

func (b *BasicTypeInfo) loadComments() {
	if b.commentId == "" {
		b.commentId = b.DisplayName
	}

	// clean previous comments
	b.CommentCol = []Comment{}

	// load comments from AST if available, otherwise from doc
	if b.pkg != nil && len(b.pkg.namedComments[b.commentId]) > 0 {
		// Use AST comments (includes both doc and inline)
		// for _, comment := range b.pkg.Comments[b.DisplayName] {
		// 	b.CommentCol = append(b.CommentCol, comment)
		// }
		b.CommentCol = append(b.CommentCol, b.pkg.namedComments[b.commentId]...)
	} else if b.doc != nil {
		// Fallback to go/doc comments
		commentText := ExtractComments(b.doc.Doc)
		if commentText != "" {
			b.CommentCol = append(b.CommentCol, NewComment(commentText, CommentPlacementAbove))
		}
	}
}

func (b *BasicTypeInfo) SetDoc(docType *doc.Type) {
	b.doc = docType
	b.loadComments()
}

func (b *BasicTypeInfo) SetObject(obj types.Object) {
	b.obj = obj
}

func (b *BasicTypeInfo) SetVisibility(visibility Visibility) {
	b.VisibilityLevel = visibility
}

func (b *BasicTypeInfo) SetDetailsLoader(loader DetailsLoaderFn) {
	b.detailsLoader = loader
}

func (b *BasicTypeInfo) SetPackage(pkg *Package) {
	b.pkg = pkg
	b.loadComments()
}

func (b *BasicTypeInfo) Load() error {
	var loadErr error
	b.loadOnce.Do(func() {
		// load comments
		b.loadComments()

		if b.detailsLoader != nil {
			loadErr = b.detailsLoader(b)
		}
	})
	return loadErr
}

// basic alias type info
type BasicAliasTypeInfo struct {
	*BasicTypeInfo
	TypeReference `json:"ref,inline"`
	IsAlias       bool `json:"isAlias,omitempty"`
}

// NewBasicAliasTypeInfo creates a new BasicAliasTypeInfo
func NewBasicAliasTypeInfo(id string, underlying TypeReference, obj types.Object, docType *doc.Type, pkg *Package) *BasicAliasTypeInfo {
	displayName := id
	if obj != nil {
		displayName = obj.Name()
	}

	return &BasicAliasTypeInfo{
		BasicTypeInfo: &BasicTypeInfo{
			ID:          id,
			DisplayName: displayName,
			TypeKind:    TypeKindAlias,
			Description: obj.String(),
			obj:         obj,
			pkg:         pkg,
			doc:         docType,
			commentId:   displayName,
		},
		TypeReference: underlying,
		IsAlias:       true,
	}
}

func (b *BasicAliasTypeInfo) UnderlyingType() TypeReference {
	return b.TypeReference
}

// CollectionTypeEntry represents a or array type entry
type BasicCollectionTypeInfo struct {
	*BasicTypeInfo
	Size             int64         `json:"size,omitempty"` // for array, slice has no size
	ElementReference TypeReference `json:"elementType,omitempty"`
}

// NewBasicCollectionTypeInfo creates a new BasicCollectionTypeInfo
func NewBasicCollectionTypeInfo(id string, kind TypeKind, elementType TypeReference, size int64) *BasicCollectionTypeInfo {

	return &BasicCollectionTypeInfo{
		BasicTypeInfo:    NewBasicTypeInfo(id, id, kind),
		Size:             size,
		ElementReference: elementType,
	}
}

// ElementType returns the element type reference
// Implements HasElementType#ElementType
func (b *BasicCollectionTypeInfo) ElementType() TypeReference {
	return b.ElementReference
}

// BasicChannelTypeInfo represents a channel type entry
type BasicChannelTypeInfo struct {
	*BasicTypeInfo
	ElementReference TypeReference    `json:"elementType,omitempty"`
	ChannelDir       ChannelDirection `json:"channelDirection,omitempty"`
}

// NewBasicChannelTypeInfo creates a new BasicChannelTypeInfo
func NewBasicChannelTypeInfo(id string, elementType TypeReference, chanDir ChannelDirection) *BasicChannelTypeInfo {

	return &BasicChannelTypeInfo{
		BasicTypeInfo:    NewBasicTypeInfo(id, id, TypeKindChannel),
		ElementReference: elementType,
		ChannelDir:       chanDir,
	}
}

// ElementType returns the element type reference
// Implements HasElementType#ElementType
func (b *BasicChannelTypeInfo) ElementType() TypeReference {
	return b.ElementReference
}

// ChannelDirection returns the channel direction
// Implements ChannelTypeInfo#ChannelDirection
func (b *BasicChannelTypeInfo) ChannelDirection() ChannelDirection {
	return b.ChannelDir
}

// BasicPointerTypeInfo represents a pointer type entry
type BasicPointerTypeInfo struct {
	*BasicTypeInfo
	ref TypeReference
}

// NewBasicPointerTypeInfo creates a new BasicPointerTypeInfo
func NewBasicPointerTypeInfo(id string, ref TypeReference) *BasicPointerTypeInfo {

	return &BasicPointerTypeInfo{
		BasicTypeInfo: NewBasicTypeInfo(id, id, TypeKindPointer),
		ref:           ref,
	}
}

// ReferencedType returns the referenced type reference
// Implements PointerTypeInfo#ReferencedType
func (b *BasicPointerTypeInfo) ReferencedType() TypeReference {
	return b.ref
}

type BasicMapTypeInfo struct {
	*BasicTypeInfo
	KeyReference   TypeReference `json:"keyType,omitempty"`
	ValueReference TypeReference `json:"valueType,omitempty"`
}

// NewBasicMapTypeInfo creates a new BasicMapTypeInfo
func NewBasicMapTypeInfo(id string, keyType TypeReference, valueType TypeReference) *BasicMapTypeInfo {

	return &BasicMapTypeInfo{
		BasicTypeInfo:  NewBasicTypeInfo(id, id, TypeKindMap),
		KeyReference:   keyType,
		ValueReference: valueType,
	}
}

// KeyType returns the key type reference
// Implements MapTypeInfo#KeyType
func (b *BasicMapTypeInfo) KeyType() TypeReference {
	return b.KeyReference
}

// ValueType returns the value type reference
// Implements MapTypeInfo#ValueType
func (b *BasicMapTypeInfo) ValueType() TypeReference {
	return b.ValueReference
}
