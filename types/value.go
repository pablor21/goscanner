package types

import (
	"go/constant"
	"go/types"
)

type Value struct {
	BasicTypeInfo
	ConstValue    any            `json:"value,omitempty"`
	parent        Type           `json:"-"`
	valueType     Type           // underlying type of the value entry
	ValueTypeInfo *BasicTypeInfo `json:"type,omitempty"`
}

func NewValue(id string, obj types.Object, pkg *Package, valueType Type) *Value {
	switch v := obj.(type) {
	case *types.Const:
		return NewConstValue(id, v, pkg, valueType)
	case *types.Var:
		return NewVarValue(id, v, pkg, valueType)
	default:
		return nil
	}
}

func NewConstValue(id string, obj *types.Const, pkg *Package, valueType Type) *Value {
	var constVal any
	constVal = obj.Val()
	if obj.Val().Kind() == constant.String {
		constVal = constant.StringVal(obj.Val())
	}

	return &Value{
		BasicTypeInfo: BasicTypeInfo{
			ID:          id,
			DisplayName: obj.Name(),
			TypeKind:    TypeKindConstant,
			commentId:   obj.Name(),
			Description: obj.String(),
			obj:         obj,
			pkg:         pkg,
		},
		ConstValue:    constVal,
		ValueTypeInfo: NewBasicTypeInfo(valueType.Id(), valueType.Name(), valueType.Kind()),
	}
}

func NewVarValue(id string, obj *types.Var, pkg *Package, valueType Type) *Value {
	return &Value{
		BasicTypeInfo: BasicTypeInfo{
			ID:          id,
			DisplayName: obj.Name(),
			Description: obj.String(),
			TypeKind:    TypeKindVariable,
			commentId:   id,
			obj:         obj,
			pkg:         pkg,
		},
		ConstValue:    nil, // Variables do not have a constant value
		ValueTypeInfo: NewBasicTypeInfo(valueType.Id(), valueType.Name(), valueType.Kind()),
	}
}

// Value returns the constant value of the value entry
// implements ValueType#Value
func (v *Value) Value() any {
	return v.ConstValue
}

// Parent returns the parent type of the value entry (enum or variable)
// implements ValueType#Parent
func (v *Value) Parent() Type {
	return v.parent
}

// ValueType returns the underlying type of the value entry
// implements ValueType#ValueType
func (v *Value) ValueType() Type {
	return v.valueType
}
