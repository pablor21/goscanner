package types

type TypeRef struct {
	// *BasicTypeEntry
	PointerFlag  bool `json:"isPointer,omitempty"`
	PointerCount int  `json:"pointerIndirections,omitempty"`
	// Actual type entry being referenced
	typeRef TypeEntry `json:"-"`
	// This field is only informational to provide easy access to basic type info and avoid serialization issues
	BasicEntryInfo *typeReferencePublicInfo `json:"type,inline,omitempty"`
}

// typeReferencePublicInfo represents public information about a type reference for serialization
// purposes ONLY
type typeReferencePublicInfo struct {
	*BasicTypeEntry
	IsPointer           bool `json:"isPointer,omitempty"`
	PointerIndirections int  `json:"pointerIndirections,omitempty"`

	ElementTypeInfo *typeReferencePublicInfo `json:"valueType,omitempty"`

	KeyTypeInfo *typeReferencePublicInfo `json:"keyType,omitempty"`

	ChanDir ChannelDirection `json:"channelDirection,omitempty"`
}

func makePublicTypeRefInfo(ref TypeReference) *typeReferencePublicInfo {
	if basic, ok := ref.TypeRef().(*BasicTypeEntry); ok {
		return &typeReferencePublicInfo{BasicTypeEntry: basic}
	} else if complex, ok := ref.TypeRef().(*ComplexTypeEntry); ok {
		basicInfo := &BasicTypeEntry{
			ID:          complex.ID,
			DisplayName: complex.DisplayName,
			TypeKind:    complex.TypeKind,
		}
		return &typeReferencePublicInfo{BasicTypeEntry: basicInfo}
	} else if collection, ok := ref.TypeRef().(*CollectionTypeEntry); ok {
		basicInfo := &BasicTypeEntry{
			ID:          collection.ID,
			DisplayName: collection.DisplayName,
			TypeKind:    collection.TypeKind,
		}

		r := &typeReferencePublicInfo{BasicTypeEntry: basicInfo, ElementTypeInfo: makePublicTypeRefInfo(collection.ElementReference)}
		r.ElementTypeInfo.IsPointer = collection.ElementReference.IsPointer()
		r.ElementTypeInfo.PointerIndirections = collection.ElementReference.PointerIndirections()
		return r
	}
	return nil
}

func NewTypeRef(refId string, pointerIndirections int, reference TypeEntry) *TypeRef {
	ref := &TypeRef{
		PointerFlag:  pointerIndirections > 0,
		PointerCount: pointerIndirections,
		typeRef:      reference,
	}

	ref.BasicEntryInfo = makePublicTypeRefInfo(ref)
	return ref
}

// TypeRefId returns the ID of the referenced type
// Implements TypeReference#TypeRefId
func (tr *TypeRef) TypeRefId() string {
	return tr.typeRef.Id()
}

// TypeRef returns the referenced type entry
// Implements TypeReference#TypeRef
func (tr *TypeRef) TypeRef() TypeEntry {
	return tr.typeRef
}

// IsPointer indicates if the type reference is a pointer
// Implements TypeReference#IsPointer
func (tr *TypeRef) IsPointer() bool {
	return tr.PointerFlag
}

// PointerIndirections returns the number of pointer indirections
// Implements TypeReference#PointerIndirections
func (tr *TypeRef) PointerIndirections() int {
	return tr.PointerCount
}

// Load loads the referenced type entry details
// Implements Loadable#Load
func (tr *TypeRef) Load() error {
	if tr.typeRef != nil {
		return tr.typeRef.Load()
	}
	return nil
}
