package types

import (
	"go/doc"
	"go/types"

	"golang.org/x/tools/go/packages"
)

// BasicTypeInfo represents a basic type entry
type BasicTypeInfo struct {
	ID              string     `json:"id,omitempty"`
	DisplayName     string     `json:"name,omitempty"`
	TypeKind        TypeKind   `json:"kind,omitempty"`
	Comments        []string   `json:"comments,omitempty"`
	VisibilityLevel Visibility `json:"visibility,omitempty"`
	// lock to protect lazy loading
	// loadOnce      sync.Once
	obj types.Object
	doc *doc.Type
	pkg *packages.Package
	// detailsLoader DetailsLoaderFn
}

// NewBasicTypeInfo creates a new BasicTypeInfo
func NewBasicTypeInfo(id string, kind TypeKind) BasicTypeInfo {

	return BasicTypeInfo{
		ID:          id,
		DisplayName: id,
		TypeKind:    kind,
		// VisibilityLevel: VisibilityExported,
	}
}

// Gets the ID of the type entry
// Implements Type#Id
func (b BasicTypeInfo) Id() string { return b.ID }

// Gets the name of the type entry
// Implements Type#Name
func (b BasicTypeInfo) Name() string { return b.DisplayName }

// Gets the kind of the type entry
// Implements Type#Kind
func (b BasicTypeInfo) Kind() TypeKind { return b.TypeKind }

// Gets the packages.Package
// Implements Type#Package
func (b BasicTypeInfo) Package() *packages.Package { return b.pkg }

// Gets the go/doc.Type associated with the type entry
// Implements Type#Type
func (b BasicTypeInfo) Type() *doc.Type { return b.doc }

// Gets the Object associated with the type entry
// Implements Type#Object
func (b BasicTypeInfo) Object() types.Object { return b.obj }

// Gets the visibility level of the type entry
// Implements Type#Visibility
func (b BasicTypeInfo) Visibility() Visibility {
	return b.VisibilityLevel
}

// Gets the package path of the type entry
// Implements Type#PackagePath
func (b BasicTypeInfo) PackagePath() string {
	if b.pkg != nil {
		return b.pkg.PkgPath
	}
	return ""
}

// CollectionTypeEntry represents a or array type entry
type BasicCollectionTypeInfo struct {
	BasicTypeInfo
	Size             int64         `json:"size,omitempty"` // for array, slice has no size
	ElementReference TypeReference `json:"elementType,omitempty"`
}

// NewBasicCollectionTypeInfo creates a new BasicCollectionTypeInfo
func NewBasicCollectionTypeInfo(id string, kind TypeKind, elementType TypeReference, size int64) *BasicCollectionTypeInfo {

	return &BasicCollectionTypeInfo{
		BasicTypeInfo:    NewBasicTypeInfo(id, kind),
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
	BasicTypeInfo
	ElementReference TypeReference    `json:"elementType,omitempty"`
	ChannelDir       ChannelDirection `json:"channelDirection,omitempty"`
}

// NewBasicChannelTypeInfo creates a new BasicChannelTypeInfo
func NewBasicChannelTypeInfo(id string, kind TypeKind, elementType TypeReference, chanDir ChannelDirection) *BasicChannelTypeInfo {

	return &BasicChannelTypeInfo{
		BasicTypeInfo:    NewBasicTypeInfo(id, kind),
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
