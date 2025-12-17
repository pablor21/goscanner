package types

import (
	"go/doc"
	"go/types"
	"sync"
)

// FunctionTypeInfo represents a function entry (commonly used for package-level functions)
type FunctionTypeInfo struct {
	*BasicTypeInfo
	IsVariadic bool             `json:"variadic,omitempty"`
	Parameters []*ParameterInfo `json:"parameters,omitempty"`
	Returns    []*ReturnInfo    `json:"returns,omitempty"`
	doc        *doc.Func
	// lock to protect lazy loading
	loadOnce      sync.Once
	detailsLoader DetailsLoaderFn
}

func NewFunctionInfo(id string, obj types.Object, funcDoc *doc.Func, pkg *Package, promotedFrom Type) *FunctionTypeInfo {
	displayName := id
	if obj != nil {
		displayName = obj.Name()
	}

	return &FunctionTypeInfo{
		BasicTypeInfo: &BasicTypeInfo{
			ID:          id,
			DisplayName: displayName,
			TypeKind:    TypeKindFunction,
			Description: obj.String(),
			obj:         obj,
			pkg:         pkg,
		},
		doc:           funcDoc,
		detailsLoader: nil,
	}
}

// Load loads the function details (comments, parameters, returns)
// Implements Loadable#Load
func (fi *FunctionTypeInfo) Load() error {
	var loadErr error
	fi.loadOnce.Do(func() {
		// load comments and other details
		fi.loadComments()

		if fi.detailsLoader != nil {
			loadErr = fi.detailsLoader(fi)
		}
	})
	return loadErr
}

func (fi *FunctionTypeInfo) SetDetailsLoader(loader DetailsLoaderFn) {
	fi.detailsLoader = loader
}

// NamedFunctionInfo represents a named type with function underlying type
type NamedFunctionInfo struct {
	*NamedTypeInfo
	fn             *FunctionTypeInfo
	IsVariadic     bool             `json:"variadic,omitempty"`
	Parameters     []*ParameterInfo `json:"parameters,omitempty"`
	Returns        []*ReturnInfo    `json:"returns,omitempty"`
	PromotedFromId string           `json:"promotedFrom,omitempty"`
	promotedFrom   Type             `json:"-"`
}

func NewNamedFunctionInfo(id string, obj types.Object, fn *FunctionTypeInfo, docType *doc.Type, pkg *Package, promotedFrom Type, loader DetailsLoaderFn) *NamedFunctionInfo {
	fi := &NamedFunctionInfo{
		NamedTypeInfo: NewNamedTypeInfo(id, TypeKindFunction, obj, docType, pkg, loader),
		fn:            fn,
	}
	fi.Parameters = fi.fn.Parameters
	fi.Returns = fi.fn.Returns
	fi.IsVariadic = fi.fn.IsVariadic

	promotedFromId := ""
	if promotedFrom != nil {
		promotedFromId = promotedFrom.Id()
	}
	fi.PromotedFromId = promotedFromId
	return fi
}

func (fi *NamedFunctionInfo) UnderlyingFn() *FunctionTypeInfo {
	return fi.fn
}

func (fi *NamedFunctionInfo) PromotedFrom() Type {
	return fi.promotedFrom
}

func (fi *NamedFunctionInfo) IsPromoted() bool {
	return fi.promotedFrom != nil
}

func (fi *NamedFunctionInfo) SetPromotedFrom(t Type) {
	fi.promotedFrom = t
}

func (fi *NamedFunctionInfo) Load() error {
	var loadErr error
	fi.loadOnce.Do(func() {
		// load comments
		fi.loadComments()

		// load named type details
		if fi.detailsLoader != nil {
			loadErr = fi.detailsLoader(fi)
		}

		if loadErr != nil {
			return
		}

		// load function signature details
		if fi.fn != nil {
			loadErr = fi.fn.Load()
		}
	})
	return loadErr
}

// MethodInfo represents a method entry associated with a named type
type MethodInfo struct {
	*FunctionTypeInfo
	receiver NamedType `json:"-"`
	// ReceiverInfo      *BasicTypeInfo   `json:"receiver,omitempty"`
	IsPointerReceiver bool             `json:"isPointerReceiver,omitempty"`
	PromotedFromId    string           `json:"promotedFrom,omitempty"`
	promotedFrom      Type             `json:"-"`
	Parameters        []*ParameterInfo `json:"parameters,omitempty"`
}

func NewMethodInfo(obj types.Object, parent NamedType) *MethodInfo {
	// search the doc.Func from parent.Type().Methods
	var doc *doc.Func
	if parent.Type() != nil {
		for _, m := range parent.Type().Methods {
			if m.Name == obj.Name() {
				doc = m
				break
			}
		}
	}

	m := &MethodInfo{
		FunctionTypeInfo: NewFunctionInfo(parent.Id()+"#"+obj.Name(), obj, doc, parent.Package(), nil),
		receiver:         parent,
		// ReceiverInfo:      parent.GetBasicInfo(),
		IsPointerReceiver: false,
	}
	m.TypeKind = TypeKindMethod
	m.commentId = parent.GetBasicInfo().DisplayName + "." + obj.Name()

	return m
}

func (m *MethodInfo) Receiver() NamedType { return m.receiver }

func (fi *MethodInfo) PromotedFrom() Type {
	return fi.promotedFrom
}

func (fi *MethodInfo) IsPromoted() bool {
	return fi.promotedFrom != nil
}

func (fi *MethodInfo) SetPromotedFrom(t Type) {
	fi.promotedFrom = t
	if t != nil {
		fi.PromotedFromId = t.Id()
	}
}

type ParameterInfo struct {
	parent     *FunctionTypeInfo
	*TypeRef   `json:",inline,omitempty"`
	Name       string `json:"name,omitempty"`
	IsVariadic bool   `json:"variadic,omitempty"`
}

func NewParameterInfo(obj *types.Var, t TypeReference, parent *FunctionTypeInfo) *ParameterInfo {
	return &ParameterInfo{
		parent:  parent,
		Name:    obj.Name(),
		TypeRef: t.(*TypeRef),
	}
}

func (p *ParameterInfo) Type() TypeReference {
	return p.TypeRef
}

type ReturnInfo struct {
	*TypeRef `json:",inline,omitempty"`
	Name     string `json:"name,omitempty"`
	// TypeInfo *typeReferencePublicInfo `json:"type,omitempty"`
}

func NewReturnInfo(obj *types.Var, t TypeReference, parent *FunctionTypeInfo) *ReturnInfo {
	return &ReturnInfo{
		Name:    obj.Name(),
		TypeRef: t.(*TypeRef),
	}
}

func (r *ReturnInfo) Type() TypeReference {
	return r.TypeRef
}
