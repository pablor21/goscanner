package types

import (
	"go/doc"
	"go/types"
)

type EnumInfo struct {
	*NamedTypeInfo
	ValuesCol []ValueType `json:"values,omitempty"`
}

func NewEnum(id string, underlying Type, obj types.Object, docType *doc.Type, pkg *Package, loader DetailsLoaderFn) *EnumInfo {
	tRef := NewTypeRef(underlying.Id(), 0, underlying)
	named := NewNamedTypeInfo(id, TypeKindEnum, obj, docType, pkg, loader)
	named.TypeRef = tRef
	en := &EnumInfo{
		NamedTypeInfo: named,
		ValuesCol:     []ValueType{},
	}
	return en
}

func (e *EnumInfo) AddValues(v ...*Value) {
	for _, val := range v {
		val.ID = e.ID + "#" + val.Name()
		e.ValuesCol = append(e.ValuesCol, val)
		val.parent = e
	}

}

func (e *EnumInfo) Values() []ValueType {
	return e.ValuesCol
}
