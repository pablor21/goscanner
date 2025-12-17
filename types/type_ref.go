package types

type TypeRef struct {
	// *BasicTypeEntry
	PointerFlag  bool `json:"isPointer,omitempty"`
	PointerCount int  `json:"pointerIndirections,omitempty"`
	// Actual type entry being referenced
	reference Type `json:"-"`
	// This field is only informational to provide easy access to basic type info and avoid serialization issues
	BasicEntryInfo *typeReferencePublicInfo `json:"type,inline,omitempty"`
}

// typeReferencePublicInfo represents public information about a type reference for serialization
// purposes ONLY
type typeReferencePublicInfo struct {
	BasicTypeInfo
	IsPointer           bool `json:"isPointer,omitempty"`
	PointerIndirections int  `json:"pointerIndirections,omitempty"`

	ElementTypeInfo *typeReferencePublicInfo `json:"valueType,omitempty"`

	KeyTypeInfo *typeReferencePublicInfo `json:"keyType,omitempty"`

	ChanDir ChannelDirection `json:"channelDirection,omitempty"`
}

func makePublicTypeRefInfo(ref TypeReference) *typeReferencePublicInfo {
	// Handle nil reference gracefully
	if ref == nil || ref.TypeRef() == nil {
		return &typeReferencePublicInfo{
			BasicTypeInfo: BasicTypeInfo{
				ID:          "",
				DisplayName: "nil",
			},
		}
	}

	rt := ref.TypeRef()
	basicInfo := NewBasicTypeInfo(rt.Id(), rt.Kind())
	basicInfo.DisplayName = rt.Name()
	basicInfo.VisibilityLevel = rt.Visibility()

	switch t := ref.TypeRef().(type) {
	case *NamedChannelTypeInfo:
		elemInfo := makePublicTypeRefInfo(t.ElementType())
		elemInfo.IsPointer = t.ElementType().IsPointer()
		elemInfo.PointerIndirections = t.ElementType().PointerIndirections()
		return &typeReferencePublicInfo{
			BasicTypeInfo:       basicInfo,
			ElementTypeInfo:     elemInfo,
			ChanDir:             t.ChannelDirection(),
			IsPointer:           ref.IsPointer(),
			PointerIndirections: ref.PointerIndirections(),
		}
	case HasElementType:
		elemType := t.ElementType()
		if elemType == nil {
			return &typeReferencePublicInfo{
				BasicTypeInfo: BasicTypeInfo{
					ID:          t.Id(),
					DisplayName: t.Name(),
				},
			}
		}
		elemInfo := makePublicTypeRefInfo(elemType)
		elemInfo.IsPointer = elemType.IsPointer()
		elemInfo.PointerIndirections = elemType.PointerIndirections()

		return &typeReferencePublicInfo{
			BasicTypeInfo:   basicInfo,
			ElementTypeInfo: elemInfo,
			// IsPointer:           ref.IsPointer(),
			// PointerIndirections: ref.PointerIndirections(),
		}
	case HasKeyType:
		keyInfo := makePublicTypeRefInfo(t.KeyType())
		keyInfo.IsPointer = t.KeyType().IsPointer()
		keyInfo.PointerIndirections = t.KeyType().PointerIndirections()
		return &typeReferencePublicInfo{
			BasicTypeInfo:       basicInfo,
			KeyTypeInfo:         keyInfo,
			IsPointer:           ref.IsPointer(),
			PointerIndirections: ref.PointerIndirections(),
		}

	default:

		return &typeReferencePublicInfo{
			BasicTypeInfo: basicInfo,
			// IsPointer:           ref.IsPointer(),
			// PointerIndirections: ref.PointerIndirections(),
		}
	}
	// if basic, ok := ref.TypeRef().(*BasicTypeEntry); ok {
	// 	return &typeReferencePublicInfo{BasicTypeEntry: basic}
	// } else if complex, ok := ref.TypeRef().(*ComplexTypeEntry); ok {
	// 	basicInfo := &BasicTypeEntry{
	// 		ID:          complex.ID,
	// 		DisplayName: complex.DisplayName,
	// 		TypeKind:    complex.TypeKind,
	// 	}
	// 	return &typeReferencePublicInfo{BasicTypeEntry: basicInfo}
	// } else if collection, ok := ref.TypeRef().(*CollectionTypeEntry); ok {
	// 	basicInfo := &BasicTypeEntry{
	// 		ID:          collection.ID,
	// 		DisplayName: collection.DisplayName,
	// 		TypeKind:    collection.TypeKind,
	// 	}

	// 	r := &typeReferencePublicInfo{BasicTypeEntry: basicInfo, ElementTypeInfo: makePublicTypeRefInfo(collection.ElementReference)}
	// 	r.ElementTypeInfo.IsPointer = collection.ElementReference.IsPointer()
	// 	r.ElementTypeInfo.PointerIndirections = collection.ElementReference.PointerIndirections()
	// 	return r
	// }
	// return nil
}

func NewTypeRef(refId string, pointerIndirections int, reference Type) *TypeRef {
	ref := &TypeRef{
		PointerFlag:  pointerIndirections > 0,
		PointerCount: pointerIndirections,
		reference:    reference,
	}

	// Only create public info if we have a valid reference
	if reference != nil {
		ref.BasicEntryInfo = makePublicTypeRefInfo(ref)
	} else {
		// Create a placeholder for nil references
		ref.BasicEntryInfo = &typeReferencePublicInfo{
			BasicTypeInfo: BasicTypeInfo{
				ID:          refId,
				DisplayName: "unresolved",
				// VisibilityLevel: VisibilityLevelExported,
			},
		}
	}

	return ref
}

// TypeRefId returns the ID of the referenced type
// Implements TypeReference#TypeRefId
func (tr *TypeRef) TypeRefId() string {
	return tr.reference.Id()
}

// TypeRef returns the referenced type entry
// Implements TypeReference#TypeRef
func (tr *TypeRef) TypeRef() Type {
	return tr.reference
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
	if tr.reference != nil {
		if loadable, ok := tr.reference.(Loadable); ok {
			return loadable.Load()
		}
	}
	return nil
}
