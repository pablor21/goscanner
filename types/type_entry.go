package types

import (
	"go/doc"
	"go/types"
	"strings"
	"sync"
)

// MethodInfo represents information about a method
type MethodInfo struct {
	Name       string           `json:"name,omitempty"`
	Parameters []*ParameterInfo `json:"parameters,omitempty"`
	Results    []*ResultInfo    `json:"results,omitempty"`
	Docs       []string         `json:"comments,omitempty"`
	Variadic   bool             `json:"variadic,omitempty"`
	Receiver   *TypeRef         `json:"receiver,omitempty"`
}

// ParameterInfo represents information about a method/function parameter
type ParameterInfo struct {
	Name     string `json:"name,omitempty"`
	Variadic bool   `json:"variadic,omitempty"`
	TypeRef  `json:",inline"`
}

// ResultInfo represents information about a method/function result
type ResultInfo struct {
	Name    string `json:"name,omitempty"`
	TypeRef `json:",inline"`
}

// FieldInfo represents information about a struct field
type FieldInfo struct {
	Name     string   `json:"name,omitempty"`
	Docs     []string `json:"comments,omitempty"`
	Embedded bool     `json:"embedded,omitempty"`
	TypeRef  `json:",inline"`
}

// BasicTypeEntry represents a basic type entry
type BasicTypeEntry struct {
	ID                 string    `json:"id,omitempty"`
	DisplayName        string    `json:"name,omitempty"`
	PackageDisplayName string    `json:"package,omitempty"`
	TypeKind           TypeKind  `json:"kind,omitempty"`
	PointerFlag        bool      `json:"isPointer,omitempty"`
	RefID              string    `json:"refId,omitempty"`
	Reference          TypeEntry `json:"-"`
	Comments           []string  `json:"comments,omitempty"`
	// lock to protect lazy loading
	loadOnce      sync.Once
	obj           types.Object
	doc           *doc.Type
	detailsLoader DetailsLoaderFn
}

func NewBasicTypeEntry(id string, kind TypeKind) *BasicTypeEntry {
	displayName := id
	packageDisplayName := ""
	idParts := strings.Split(id, ".")
	if len(idParts) > 1 {
		packageDisplayName = strings.Join(idParts[:len(idParts)-1], ".")
		displayName = idParts[len(idParts)-1]
	} else {
		displayName = idParts[0]
	}

	return &BasicTypeEntry{
		ID:                 id,
		DisplayName:        displayName,
		PackageDisplayName: packageDisplayName,
		TypeKind:           kind,
		PointerFlag:        false,
		RefID:              "",
		loadOnce:           sync.Once{},
		Reference:          nil,
		Comments:           nil,
	}
}

func (b *BasicTypeEntry) Id() string {
	return b.ID
}
func (b *BasicTypeEntry) Name() string {
	return b.DisplayName
}
func (b *BasicTypeEntry) PackagePath() string {
	return b.PackageDisplayName
}
func (b *BasicTypeEntry) Kind() TypeKind {
	return b.TypeKind
}
func (b *BasicTypeEntry) IsPointer() bool {
	return b.PointerFlag
}
func (b *BasicTypeEntry) TypeRefId() string {
	return b.RefID
}
func (b *BasicTypeEntry) TypeRef() TypeEntry {
	return b.Reference
}
func (b *BasicTypeEntry) Docs() []string {
	return b.Comments
}
func (b *BasicTypeEntry) Load() error {
	var loadErr error
	b.loadOnce.Do(func() {
		// extract comments from doc.Type if available
		b.loadComments()
		// invoke details loader if provided
		if b.detailsLoader != nil {
			loadErr = b.detailsLoader(b)
		}
	})
	return loadErr
}

func (b *BasicTypeEntry) loadComments() {
	if b.doc != nil {
		var comments []string
		if b.doc.Doc != "" {
			comments = append(comments, strings.Split(b.doc.Doc, "\n")...)
		}
		b.Comments = comments
	}
}

// ComplexTypeEntry represents a complex type entry (structs, interfaces, etc.)
type ComplexTypeEntry struct {
	BasicTypeEntry
	Methods       []*MethodInfo `json:"methods,omitempty"`
	Fields        []*FieldInfo  `json:"fields,omitempty"`
	TypeReference TypeReference `json:"typeRef,omitempty"`
}

func NewComplexTypeEntry(id string, kind TypeKind, obj types.Object, docType *doc.Type, loader DetailsLoaderFn) *ComplexTypeEntry {
	displayName := id
	packageDisplayName := ""
	idParts := strings.Split(id, ".")
	if len(idParts) > 1 {
		packageDisplayName = strings.Join(idParts[:len(idParts)-1], ".")
		displayName = idParts[len(idParts)-1]
	} else {
		displayName = idParts[0]
	}
	//basic := NewBasicTypeEntry(id, kind)
	// basic.obj = obj
	// basic.doc = docType
	// basic.detailsLoader = loader
	return &ComplexTypeEntry{
		BasicTypeEntry: BasicTypeEntry{
			ID:                 id,
			DisplayName:        displayName,
			PackageDisplayName: packageDisplayName,
			TypeKind:           kind,
			PointerFlag:        false,
			RefID:              "",
			Reference:          nil,
			Comments:           nil,
			loadOnce:           sync.Once{},
			obj:                obj,
			doc:                docType,
			detailsLoader:      loader,
		},
		Methods:       []*MethodInfo{},
		Fields:        []*FieldInfo{},
		TypeReference: nil,
	}
}

func (c *ComplexTypeEntry) TypeRef() TypeReference {
	if c.TypeReference != nil {
		return c.TypeReference
	}
	return nil
}

func (c *ComplexTypeEntry) Load() error {
	var loadErr error
	c.loadOnce.Do(func() {
		// load comments
		c.loadComments()
		// Load complex type details here
		if c.detailsLoader != nil {
			loadErr = c.detailsLoader(c)
		}
	})
	return loadErr
}

// CollectionTypeEntry represents a or array type entry
type CollectionTypeEntry struct {
	BasicTypeEntry
	ElementReference TypeReference `json:"elementType,omitempty"`
}

func NewCollectionTypeEntry(id string, kind TypeKind, elementType TypeReference, obj types.Object, docType *doc.Type, loader DetailsLoaderFn) *CollectionTypeEntry {
	displayName := id
	packageDisplayName := ""
	idParts := strings.Split(id, ".")
	if len(idParts) > 1 {
		packageDisplayName = strings.Join(idParts[:len(idParts)-1], ".")
		displayName = idParts[len(idParts)-1]
	} else {
		displayName = idParts[0]
	}
	return &CollectionTypeEntry{
		BasicTypeEntry: BasicTypeEntry{
			ID:                 id,
			DisplayName:        displayName,
			PackageDisplayName: packageDisplayName,
			TypeKind:           kind,
			PointerFlag:        false,
			RefID:              "",
			Reference:          nil,
			Comments:           nil,
			obj:                obj,
			loadOnce:           sync.Once{},
			doc:                docType,
			detailsLoader:      loader,
		},
		ElementReference: elementType,
	}
}
