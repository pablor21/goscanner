package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/pablor21/goscanner/logger"
	. "github.com/pablor21/goscanner/types"
)

// test basic types
func TestTypeResolver_resolveBasicType(t *testing.T) {
	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	// Define test cases for basic types based on the go/types package
	tests := []struct {
		goType   types.Type
		wantKind TypeKind
	}{
		{types.Typ[types.Bool], TypeKindBasic},
		{types.Typ[types.Byte], TypeKindBasic},
		{types.Typ[types.Complex64], TypeKindBasic},
		{types.Typ[types.Complex128], TypeKindBasic},
		{types.Universe.Lookup("error").Type(), TypeKindBasic},
		{types.Universe.Lookup("rune").Type(), TypeKindBasic},
		{types.Universe.Lookup("comparable").Type(), TypeKindBasic},
		{types.Typ[types.Float32], TypeKindBasic},
		{types.Typ[types.Float64], TypeKindBasic},
		{types.Typ[types.Int], TypeKindBasic},
		{types.Typ[types.Int8], TypeKindBasic},
		{types.Typ[types.Int16], TypeKindBasic},
		{types.Typ[types.Int32], TypeKindBasic},
		{types.Typ[types.Int64], TypeKindBasic},
		{types.Typ[types.Rune], TypeKindBasic},
		{types.Typ[types.String], TypeKindBasic},
		{types.Typ[types.Uint], TypeKindBasic},
		{types.Typ[types.Uint8], TypeKindBasic},
		{types.Typ[types.Uint16], TypeKindBasic},
		{types.Typ[types.Uint32], TypeKindBasic},
		{types.Typ[types.Uint64], TypeKindBasic},
		{types.Typ[types.Uintptr], TypeKindBasic},
	}

	for _, tt := range tests {
		t.Run(tt.goType.String(), func(t *testing.T) {
			got := r.ResolveType(tt.goType)
			if got == nil {
				t.Errorf("resolveType(%v) = nil, want %v", tt.goType, tt.wantKind)
				return
			}
			if got.Kind() != tt.wantKind || got.Id() != tt.goType.String() {
				t.Errorf("resolveType(%v) = %v, want kind: %v, name: %v", tt.goType, got.Kind(), tt.wantKind, got.Id())
			}
		})
	}
}

func TestTypeResolver_resolveNamedBasicTypes(t *testing.T) {
	src := `
	package test

	type MyInt int
	type MyString string
	type MyBool bool
	type MyFloat float64
	type MyComplex complex128
	type MyByte byte
	type MyRune rune
	type MyError error
	type MyUintptr uintptr
	type MyFloat32 float32
	type MyFloat64 float64
	type MyInt8 int8
	type MyInt16 int16
	type MyInt32 int32
	type MyInt64 int64
	type MyUint uint
	type MyUint8 uint8
	type MyUint16 uint16
	type MyUint32 uint32
	type MyUint64 uint64
	`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}

	cfg := &types.Config{}
	pkg, err := cfg.Check("test", fset, []*ast.File{file}, nil)
	if err != nil {
		t.Fatal(err)
	}
	l := logger.NewDefaultLogger()
	l.SetLevel(logger.LogLevelDebug)
	r := newDefaultTypeResolver(NewDefaultConfig(), l)

	namedTypes := []string{
		"MyInt",
		"MyString",
		"MyBool",
		"MyFloat",
		"MyComplex",
		"MyByte",
		"MyRune",
		// "MyError", // TODO: add support for error named types
		"MyUintptr",
		"MyFloat32",
		"MyFloat64",
		"MyInt8",
		"MyInt16",
		"MyInt32",
		"MyInt64",
		"MyUint",
		"MyUint8",
		"MyUint16",
		"MyUint32",
		"MyUint64",
	}

	for _, typeName := range namedTypes {
		obj := pkg.Scope().Lookup(typeName)
		namedType := obj.Type()

		got := r.ResolveType(namedType)
		if got == nil {
			t.Errorf("ResolveType(%s) = nil, want type info", typeName)
			continue
		}

		if got.Kind() != TypeKindBasic {
			t.Errorf("ResolveType(%s).GetKind() = %v, want %v", typeName, got.Kind(), TypeKindBasic)
		}

		if got.Id() != "test."+typeName {
			t.Errorf("ResolveType(%s).GetCannonicalName() = %v, want %v", typeName, got.Id(), "test."+typeName)
		}

		if (got.(*NamedTypeInfo)).TypeRefId() != namedType.Underlying().String() {
			t.Errorf("ResolveType(%s).Underlying().GetCannonicalName() = %v, want %v", typeName, (got.(*NamedTypeInfo)).TypeRefId(), namedType.Underlying().String())
		}
	}

	// src := `
	// package test

	// type BasicStruct struct {
	//     Field1 string
	//     Field2 int
	//     Field3 *bool
	// 	Field4 []float64
	// 	Field5 map[string]int
	// 	Field6 chan int
	// 	Field7 interface{}
	// 	Field8 [5]string
	// 	Field9 *BasicStruct
	// 	Field10 [][]int
	// 	field11 string // unexported field
	// }
	// func (bs BasicStruct) Method1() {}
	// func (bs *BasicStruct) Method2(param int) string {
	// 	return ""
	// }
	// // this method should not be counted as it's unexported
	// func (bs BasicStruct) method3() {}

	// type unexportedStruct struct {
	// 	FieldA string
	// }

	// type BasicInterface interface {
	// 	InterfaceMethod1() int
	// 	InterfaceMethod2(param string) error
	// 	privateMethod() // unexported method
	// }

	// type unexportedInterface interface {
	// 	UnexportedInterfaceMethod() bool
	// }
	// `

	// fset := token.NewFileSet()
	// file, err := parser.ParseFile(fset, "test.go", src, 0)
	// if err != nil {
	// 	t.Fatal(err)
	// }

	// cfg := &types.Config{}
	// pkg, err := cfg.Check("test", fset, []*ast.File{file}, nil)
	// if err != nil {
	// 	t.Fatal(err)
	// }
	// l := logger.NewDefaultLogger()
	// l.SetLevel(logger.LogLevelDebug)
	// r := newDefaultTypeResolver(NewDefaultConfig(), l)

	// // Get the type from package scope
	// obj := pkg.Scope().Lookup("BasicStruct")
	// structType := obj.Type()

	// got := r.ResolveType(structType)
	// if got == nil {
	// 	t.Errorf("ResolveType(BasicStruct) = nil, want type info")
	// 	return
	// }

	// if got.Kind() != TypeKindStruct {
	// 	t.Errorf("ResolveType(BasicStruct).GetKind() = %v, want %v", got.Kind(), TypeKindStruct)
	// }

	// if got.Id() != "test.BasicStruct" {
	// 	t.Errorf("ResolveType(BasicStruct).GetCannonicalName() = %v, want %v", got.Id(), "test.BasicStruct")
	// }

	// // resolve type again should return the same instance
	// got2 := r.ResolveType(structType)
	// if got != got2 {
	// 	t.Errorf("ResolveType should be idempotent, got different instances")
	// }

	// // check how many types are cached
	// if len(r.GetTypeInfos()) != 1 { // only BasicStruct should be cached
	// 	t.Errorf("Expected 1 type in cache, got %d", len(r.GetTypeInfos()))
	// }

	// // check how many fields are in the struct
	// structInfo, ok := got.(*ComplexTypeEntry)
	// if !ok {
	// 	t.Errorf("Expected ComplexTypeEntry, got %T", got)
	// 	return
	// }

	// if len(structInfo.Fields) != 10 {
	// 	t.Errorf("Expected 10 fields in BasicStruct, got %d", len(structInfo.Fields))
	// }

	// if len(structInfo.Methods) != 2 {
	// 	t.Errorf("Expected 2 methods in BasicStruct, got %d", len(structInfo.Methods))
	// }

	// // unexportedStruct should not be in the cache
	// unexportedObj := pkg.Scope().Lookup("unexportedStruct")
	// // resolve unexported struct type
	// got = r.ResolveType(unexportedObj.Type())
	// if got != nil {
	// 	t.Errorf("ResolveType(unexportedStruct) = %v, want nil", got)
	// }

	// if len(r.GetTypeInfos()) != 1 { // still only BasicStruct should be cached
	// 	t.Errorf("Expected 1 type in cache after checking unexported struct, got %d", len(r.GetTypeInfos()))
	// }

	// // test interface
	// ifaceObj := pkg.Scope().Lookup("BasicInterface")
	// ifaceType := ifaceObj.Type()

	// ifaceInfo := r.ResolveType(ifaceType)
	// if ifaceInfo == nil {
	// 	t.Errorf("ResolveType(BasicInterface) = nil, want type info")
	// 	return
	// }

	// if ifaceInfo.Kind() != TypeKindInterface {
	// 	t.Errorf("ResolveType(BasicInterface).GetKind() = %v, want %v", ifaceInfo.Kind(), TypeKindInterface)
	// }

	// if ifaceInfo.Id() != "test.BasicInterface" {
	// 	t.Errorf("ResolveType(BasicInterface).GetCannonicalName() = %v, want %v", ifaceInfo.Id(), "test.BasicInterface")
	// }

	// ifaceNamedInfo, ok := ifaceInfo.(*ComplexTypeEntry)
	// if !ok {
	// 	t.Errorf("Expected NamedTypeInfo, got %T", ifaceInfo)
	// 	return
	// }

	// if len(ifaceNamedInfo.Methods) != 2 {
	// 	t.Errorf("Expected 2 methods in BasicInterface, got %d", len(ifaceNamedInfo.Methods))
	// }

	// // unexportedInterface should not be in the cache
	// unexportedIfaceObj := pkg.Scope().Lookup("unexportedInterface")
	// // resolve unexported interface type
	// got = r.ResolveType(unexportedIfaceObj.Type())
	// if got != nil {
	// 	t.Errorf("ResolveType(unexportedInterface) = %v, want nil", got)
	// }

	// if len(r.GetTypeInfos()) != 2 { // still only BasicStruct and BasicInterface should be cached
	// 	t.Errorf("Expected 2 types in cache after checking unexported interface, got %d", len(r.GetTypeInfos()))
	// }
}

// // test structs
// func TestTypeResolver_resolveStructType(t *testing.T) {
// 	r := newDefaultTypeResolver(ScanModeFull, &logger.DefaultLogger{})

// 	tests := []struct {
// 		name         string
// 		setupStruct  func() types.Type
// 		wantKind     TypeKind
// 		wantName     string
// 		shouldCache  bool
// 		expectFields bool
// 	}{
// 		{
// 			name: "simple named struct",
// 			setupStruct: func() types.Type {
// 				structType := types.NewStruct([]*types.Var{
// 					types.NewVar(0, nil, "Field1", types.Typ[types.Int]),
// 					types.NewVar(0, nil, "Field2", types.Typ[types.String]),
// 				}, nil)
// 				return types.NewNamed(types.NewTypeName(0, nil, "MyStruct", nil), structType, nil)
// 			},
// 			wantKind:     TypeKindStruct,
// 			wantName:     "MyStruct",
// 			shouldCache:  true,
// 			expectFields: true,
// 		},
// 		{
// 			name: "empty struct",
// 			setupStruct: func() types.Type {
// 				structType := types.NewStruct([]*types.Var{}, nil)
// 				return types.NewNamed(types.NewTypeName(0, nil, "EmptyStruct", nil), structType, nil)
// 			},
// 			wantKind:     TypeKindStruct,
// 			wantName:     "EmptyStruct",
// 			shouldCache:  true,
// 			expectFields: false,
// 		},
// 		{
// 			name: "struct with pointer fields",
// 			setupStruct: func() types.Type {
// 				structType := types.NewStruct([]*types.Var{
// 					types.NewVar(0, nil, "PtrField", types.NewPointer(types.Typ[types.Int])),
// 					types.NewVar(0, nil, "NormalField", types.Typ[types.String]),
// 				}, nil)
// 				return types.NewNamed(types.NewTypeName(0, nil, "PointerStruct", nil), structType, nil)
// 			},
// 			wantKind:     TypeKindStruct,
// 			wantName:     "PointerStruct",
// 			shouldCache:  true,
// 			expectFields: true,
// 		},
// 		{
// 			name: "struct with embedded field",
// 			setupStruct: func() types.Type {
// 				// Create an embedded struct field (anonymous field)
// 				embeddedStruct := types.NewStruct([]*types.Var{
// 					types.NewVar(0, nil, "EmbeddedField", types.Typ[types.Int]),
// 				}, nil)
// 				embeddedNamed := types.NewNamed(types.NewTypeName(0, nil, "Embedded", nil), embeddedStruct, nil)

// 				structType := types.NewStruct([]*types.Var{
// 					types.NewVar(0, nil, "", embeddedNamed), // Empty name for embedded field
// 					types.NewVar(0, nil, "RegularField", types.Typ[types.String]),
// 				}, nil)
// 				return types.NewNamed(types.NewTypeName(0, nil, "EmbeddingStruct", nil), structType, nil)
// 			},
// 			wantKind:     TypeKindStruct,
// 			wantName:     "EmbeddingStruct",
// 			shouldCache:  true,
// 			expectFields: true,
// 		},
// 		{
// 			name: "struct in package",
// 			setupStruct: func() types.Type {
// 				// Create a package for the struct
// 				pkg := types.NewPackage("example.com/test", "test")
// 				structType := types.NewStruct([]*types.Var{
// 					types.NewVar(0, pkg, "Field1", types.Typ[types.Int]),
// 				}, nil)
// 				return types.NewNamed(types.NewTypeName(0, pkg, "PackageStruct", nil), structType, nil)
// 			},
// 			wantKind:     TypeKindStruct,
// 			wantName:     "example.com/test.PackageStruct",
// 			shouldCache:  true,
// 			expectFields: true,
// 		},
// 	}

// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			// Clear cache between tests to ensure clean state
// 			r.types = make(map[string]TypeInfo)

// 			structType := tt.setupStruct()
// 			got := r.ResolveType(structType)

// 			// Basic checks
// 			if got == nil {
// 				t.Errorf("ResolveType(%v) = nil, want struct type", structType)
// 				return
// 			}

// 			if got.GetKind() != tt.wantKind {
// 				t.Errorf("ResolveType(%v).GetKind() = %v, want %v", structType, got.GetKind(), tt.wantKind)
// 			}

// 			if got.GetCannonicalName() != tt.wantName {
// 				t.Errorf("ResolveType(%v).GetCannonicalName() = %v, want %v", structType, got.GetCannonicalName(), tt.wantName)
// 			}

// 			// Check if type is properly cached
// 			if tt.shouldCache {
// 				cached := r.GetTypeInfos()[got.GetCannonicalName()]
// 				if cached == nil {
// 					t.Errorf("Expected type %v to be cached, but it's not", tt.wantName)
// 				} else if cached != got {
// 					t.Errorf("Cached type is different from resolved type")
// 				}
// 			}

// 			// Test idempotency - resolving the same type should return the same instance
// 			got2 := r.ResolveType(structType)
// 			if got != got2 {
// 				t.Errorf("ResolveType should be idempotent, got different instances")
// 			}

// 			// Test type-specific methods
// 			if !got.IsBasic() == false {
// 				// This might fail depending on implementation, just testing the method exists
// 			}

// 			if got.IsMap() {
// 				t.Errorf("Struct type should not be identified as map")
// 			}

// 			if got.IsSlice() {
// 				t.Errorf("Struct type should not be identified as slice")
// 			}

// 			if got.IsChannel() {
// 				t.Errorf("Struct type should not be identified as channel")
// 			}
// 		})
// 	}
// }

// // Test resolving pointer to struct
// func TestTypeResolver_resolveStructPointerType(t *testing.T) {
// 	r := newDefaultTypeResolver(ScanModeFull, &logger.DefaultLogger{})

// 	// Create a struct type
// 	structType := types.NewStruct([]*types.Var{
// 		types.NewVar(0, nil, "Field1", types.Typ[types.Int]),
// 	}, nil)
// 	namedStruct := types.NewNamed(types.NewTypeName(0, nil, "MyStruct", nil), structType, nil)

// 	// Create pointer to struct
// 	pointerToStruct := types.NewPointer(namedStruct)

// 	got := r.ResolveType(pointerToStruct)
// 	if got == nil {
// 		t.Errorf("ResolveType(pointer to struct) = nil, want type info")
// 		return
// 	}

// 	// The resolver should handle pointer indirection and return the underlying struct type info
// 	// This depends on the implementation - it might return the struct type or a pointer type
// 	if got.GetKind() != TypeKindStruct && got.GetKind() != TypeKindBasic {
// 		t.Errorf("ResolveType(pointer to struct).GetKind() = %v, expected struct or appropriate pointer handling", got.GetKind())
// 	}
// }
