package types

import (
	"go/types"
)

type ConstantKind string

const (
	ConstantKindBool    ConstantKind = "bool"
	ConstantKindInt     ConstantKind = "int"     // All integer types (e.g., int, int8, uint, rune, etc.)
	ConstantKindFloat   ConstantKind = "float"   // float32, float64
	ConstantKindComplex ConstantKind = "complex" // complex64, complex128
	ConstantKindString  ConstantKind = "string"
)

// ConstantInfo represents a constant entry
type ConstantInfo struct {
	*BasicTypeInfo
	Kind  ConstantKind `json:"kind,omitempty"`
	Value any          `json:"value,omitempty"`
}

func NewConstantInfo(id string, obj types.Object, value any, pkg *Package) *ConstantInfo {
	displayName := id
	if obj != nil {
		displayName = obj.Name()
	}

	var kind ConstantKind
	switch obj.Type().Underlying().(type) {
	case *types.Basic:
		basicType := obj.Type().Underlying().(*types.Basic)
		switch basicType.Kind() {
		case types.Bool:
			kind = ConstantKindBool
		case types.Int, types.Int8, types.Int16, types.Rune, types.Int64,
			types.Uint, types.Uint8, types.Uint16, types.Uint32, types.Uint64, types.Uintptr:
			kind = ConstantKindInt
		case types.Float32, types.Float64:
			kind = ConstantKindFloat
		case types.Complex64, types.Complex128:
			kind = ConstantKindComplex
		case types.String:
			kind = ConstantKindString
		default:
			kind = ConstantKindString // Fallback to string for other basic kinds
		}
	default:
		kind = ConstantKindString // Fallback to string for non-basic types
	}

	return &ConstantInfo{
		BasicTypeInfo: &BasicTypeInfo{
			ID:          id,
			DisplayName: displayName,
			TypeKind:    TypeKindConstant,
			obj:         obj,
			pkg:         pkg,
		},
		Kind:  kind,
		Value: value,
	}
}
