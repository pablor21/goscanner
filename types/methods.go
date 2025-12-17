package types

import (
	"go/doc"
	"go/types"
	"sync"

	"golang.org/x/tools/go/packages"
)

// FunctionTypeInfo represents a function entry (commonly used for package-level functions)
type FunctionTypeInfo struct {
	BasicTypeInfo
	IsVariadic bool             `json:"variadic,omitempty"`
	Parameters []*ParameterInfo `json:"parameters,omitempty"`
	Returns    []*ReturnInfo    `json:"returns,omitempty"`
	CommentCol []string         `json:"comments,omitempty"`
	doc        *doc.Func
	// lock to protect lazy loading
	loadOnce      sync.Once
	detailsLoader DetailsLoaderFn
}

func NewFunctionInfo(id string, obj types.Object, funcDoc *doc.Func, pkg *packages.Package) *FunctionTypeInfo {
	displayName := id
	if obj != nil {
		displayName = obj.Name()
	}

	return &FunctionTypeInfo{
		BasicTypeInfo: BasicTypeInfo{
			ID:          id,
			DisplayName: displayName,
			TypeKind:    TypeKindFunction,
			obj:         obj,
			pkg:         pkg,
		},
		doc:           funcDoc,
		detailsLoader: nil,
	}
}

// // NewFunctionInfo creates a FunctionTypeInfo for named function types using doc.Type
// func NewFunctionInfo(id string, obj types.Object, docType *doc.Type, pkg *packages.Package) *FunctionTypeInfo {
// 	displayName := id
// 	if obj != nil {
// 		displayName = obj.Name()
// 	}

// 	functionInfo := &FunctionTypeInfo{
// 		BasicTypeInfo: BasicTypeInfo{
// 			ID:          id,
// 			DisplayName: displayName,
// 			TypeKind:    TypeKindFunction,
// 			obj:         obj,
// 			pkg:         pkg,
// 		},
// 		doc:           nil, // No doc.Func for named types
// 		detailsLoader: nil,
// 	}

// 	// Extract comments from doc.Type
// 	if docType != nil {
// 		functionInfo.CommentCol = ExtractComments(docType.Doc)
// 	}

// 	return functionInfo
// }

// Load loads the function details (comments, parameters, returns)
// Implements Loadable#Load
func (fi *FunctionTypeInfo) Load() error {
	var loadErr error
	fi.loadOnce.Do(func() {
		// load comments and other details
		if fi.doc != nil {
			fi.CommentCol = ExtractComments(fi.doc.Doc)
		}

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
	fn         *FunctionTypeInfo
	IsVariadic bool             `json:"variadic,omitempty"`
	Parameters []*ParameterInfo `json:"parameters,omitempty"`
	Returns    []*ReturnInfo    `json:"returns,omitempty"`
}

func NewNamedFunctionInfo(id string, obj types.Object, fn *FunctionTypeInfo, docType *doc.Type, pkg *packages.Package, loader DetailsLoaderFn) *NamedFunctionInfo {
	fi := &NamedFunctionInfo{
		NamedTypeInfo: NewNamedTypeInfo(id, TypeKindFunction, obj, docType, pkg, loader),
		fn:            fn,
	}
	fi.Parameters = fi.fn.Parameters
	fi.Returns = fi.fn.Returns
	fi.IsVariadic = fi.fn.IsVariadic
	return fi
}

func (fi *NamedFunctionInfo) UnderlyingFn() *FunctionTypeInfo {
	return fi.fn
}

func (fi *NamedFunctionInfo) Load() error {
	var loadErr error
	fi.loadOnce.Do(func() {
		// load the comments
		if fi.doc != nil {
			fi.CommentCol = ExtractComments(fi.doc.Doc)
		}

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
	receiver          NamedType        `json:"-"`
	ReceiverInfo      *BasicTypeInfo   `json:"receiver,omitempty"`
	IsPointerReceiver bool             `json:"isPointerReceiver,omitempty"`
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
		FunctionTypeInfo:  NewFunctionInfo(parent.Id()+"#"+obj.Name(), obj, doc, parent.Package()),
		receiver:          parent,
		ReceiverInfo:      parent.GetBasicInfo(),
		IsPointerReceiver: false,
	}
	m.TypeKind = TypeKindMethod

	return m
}

func (m *MethodInfo) Receiver() NamedType { return m.receiver }

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
