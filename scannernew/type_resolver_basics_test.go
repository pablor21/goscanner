package scannernew

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/pablor21/goscanner/logger"
	"github.com/pablor21/goscanner/typesnew"
)

// test basic types
func TestTypeResolver_resolveBasicType(t *testing.T) {
	r := NewDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	// Define test cases for basic types based on the go/types package
	tests := []struct {
		goType   types.Type
		wantKind typesnew.TypeKind
	}{
		{types.Typ[types.Bool], typesnew.TypeKindBasic},
		{types.Typ[types.Byte], typesnew.TypeKindBasic},
		{types.Typ[types.Complex64], typesnew.TypeKindBasic},
		{types.Typ[types.Complex128], typesnew.TypeKindBasic},
		{types.Universe.Lookup("error").Type(), typesnew.TypeKindBasic},
		{types.Universe.Lookup("rune").Type(), typesnew.TypeKindBasic},
		{types.Universe.Lookup("comparable").Type(), typesnew.TypeKindBasic},
		{types.Typ[types.Float32], typesnew.TypeKindBasic},
		{types.Typ[types.Float64], typesnew.TypeKindBasic},
		{types.Typ[types.Int], typesnew.TypeKindBasic},
		{types.Typ[types.Int8], typesnew.TypeKindBasic},
		{types.Typ[types.Int16], typesnew.TypeKindBasic},
		{types.Typ[types.Int32], typesnew.TypeKindBasic},
		{types.Typ[types.Int64], typesnew.TypeKindBasic},
		{types.Typ[types.Rune], typesnew.TypeKindBasic},
		{types.Typ[types.String], typesnew.TypeKindBasic},
		{types.Typ[types.Uint], typesnew.TypeKindBasic},
		{types.Typ[types.Uint8], typesnew.TypeKindBasic},
		{types.Typ[types.Uint16], typesnew.TypeKindBasic},
		{types.Typ[types.Uint32], typesnew.TypeKindBasic},
		{types.Typ[types.Uint64], typesnew.TypeKindBasic},
		{types.Typ[types.Uintptr], typesnew.TypeKindBasic},
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

	type MyError error
	type MyInt int
	type MyString string
	type MyBool bool
	type MyFloat float64
	type MyComplex complex128
	type MyByte byte
	type MyRune rune
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

	r := NewDefaultTypeResolver(NewDefaultConfig(), l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name           string
		wantKind       typesnew.TypeKind
		wantUnderlying string
	}{
		{"MyError", typesnew.TypeKindInterface, "error"},
		{"MyInt", typesnew.TypeKindBasic, "int"},
		{"MyString", typesnew.TypeKindBasic, "string"},
		{"MyBool", typesnew.TypeKindBasic, "bool"},
		{"MyFloat", typesnew.TypeKindBasic, "float64"},
		{"MyComplex", typesnew.TypeKindBasic, "complex128"},
		{"MyByte", typesnew.TypeKindBasic, "byte"}, // byte is an alias for uint8
		{"MyRune", typesnew.TypeKindBasic, "rune"}, // rune is an alias for int32
		{"MyUintptr", typesnew.TypeKindBasic, "uintptr"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := pkg.Scope().Lookup(tt.name)
			if obj == nil {
				t.Fatalf("Type %s not found", tt.name)
			}

			got := r.ResolveType(obj.Type())
			if got == nil {
				t.Errorf("resolveType(%v) = nil", tt.name)
				return
			}

			if got.Kind() != tt.wantKind {
				t.Errorf("resolveType(%v) kind = %v, want %v", tt.name, got.Kind(), tt.wantKind)
			}

			if !got.IsNamed() {
				t.Errorf("Expected named type for %v, got unnamed", tt.name)
			}

			// Check for underlying type
			if basic, ok := got.(*typesnew.Basic); ok {
				if basic.Underlying() == nil {
					t.Errorf("Expected underlying type, got nil")
				} else if basic.Underlying().Id() != tt.wantUnderlying {
					t.Errorf("Expected underlying type %s, got %s", tt.wantUnderlying, basic.Underlying().Id())
				}
			}
		})
	}
}

func TestTypeResolver_resolvePointerTypes(t *testing.T) {
	src := `
	package test
	
	type MyInt int
	type MyIntPtr *MyInt
	type MyIntPtrPtr **MyInt
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
	r := NewDefaultTypeResolver(NewDefaultConfig(), l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name      string
		wantKind  typesnew.TypeKind
		wantDepth int
		wantElem  string
	}{
		{"MyIntPtr", typesnew.TypeKindPointer, 1, "test.MyInt"},
		{"MyIntPtrPtr", typesnew.TypeKindPointer, 2, "test.MyInt"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := pkg.Scope().Lookup(tt.name)
			if obj == nil {
				t.Fatalf("Type %s not found", tt.name)
			}

			got := r.ResolveType(obj.Type())
			if got == nil {
				t.Errorf("resolveType(%v) = nil", tt.name)
				return
			}

			if got.Kind() != tt.wantKind {
				t.Errorf("resolveType(%v) kind = %v, want %v", tt.name, got.Kind(), tt.wantKind)
			}

			if ptr, ok := got.(*typesnew.Pointer); ok {
				if ptr.Depth() != tt.wantDepth {
					t.Errorf("Expected depth %d, got %d", tt.wantDepth, ptr.Depth())
				}
				if ptr.Element() == nil {
					t.Error("Expected element type, got nil")
				} else if ptr.Element().Id() != tt.wantElem {
					t.Errorf("Expected element %s, got %s", tt.wantElem, ptr.Element().Id())
				}
			} else {
				t.Errorf("Expected Pointer type, got %T", got)
			}
		})
	}
}

func TestTypeResolver_resolveSliceTypes(t *testing.T) {
	src := `
	package test
	
	type MyString string
	type MyStringSlice []MyString
	type MyStringArray [5]MyString
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
	r := NewDefaultTypeResolver(NewDefaultConfig(), l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name       string
		wantKind   typesnew.TypeKind
		wantLength int64
		wantElem   string
	}{
		{"MyStringSlice", typesnew.TypeKindSlice, -1, "test.MyString"},
		{"MyStringArray", typesnew.TypeKindArray, 5, "test.MyString"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := pkg.Scope().Lookup(tt.name)
			if obj == nil {
				t.Fatalf("Type %s not found", tt.name)
			}

			got := r.ResolveType(obj.Type())
			if got == nil {
				t.Errorf("resolveType(%v) = nil", tt.name)
				return
			}

			if got.Kind() != tt.wantKind {
				t.Errorf("resolveType(%v) kind = %v, want %v", tt.name, got.Kind(), tt.wantKind)
			}

			if slice, ok := got.(*typesnew.Slice); ok {
				if slice.Len() != tt.wantLength {
					t.Errorf("Expected length %d, got %d", tt.wantLength, slice.Len())
				}
				if slice.Element() == nil {
					t.Error("Expected element type, got nil")
				} else if slice.Element().Id() != tt.wantElem {
					t.Errorf("Expected element %s, got %s", tt.wantElem, slice.Element().Id())
				}
			} else {
				t.Errorf("Expected Slice type, got %T", got)
			}
		})
	}
}

func TestTypeResolver_resolveMapTypes(t *testing.T) {
	src := `
	package test
	
	type MyString string
	type MyInt int
	type MyStringIntMap map[MyString]MyInt
	type MyIntStringMap map[MyInt]MyString
	type MyInterfaceMap map[interface{}]interface{}
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
	r := NewDefaultTypeResolver(NewDefaultConfig(), l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name     string
		wantKind typesnew.TypeKind
		wantKey  string
		wantElem string
	}{
		{"MyStringIntMap", typesnew.TypeKindMap, "test.MyString", "test.MyInt"},
		{"MyIntStringMap", typesnew.TypeKindMap, "test.MyInt", "test.MyString"},
		{"MyInterfaceMap", typesnew.TypeKindMap, "interface{}", "interface{}"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := pkg.Scope().Lookup(tt.name)
			if obj == nil {
				t.Fatalf("Type %s not found", tt.name)
			}

			got := r.ResolveType(obj.Type())
			if got == nil {
				t.Errorf("resolveType(%v) = nil", tt.name)
				return
			}

			if got.Kind() != tt.wantKind {
				t.Errorf("resolveType(%v) kind = %v, want %v", tt.name, got.Kind(), tt.wantKind)
			}

			if m, ok := got.(*typesnew.Map); ok {
				if m.Key() == nil {
					t.Error("Expected key type, got nil")
				} else if m.Key().Id() != tt.wantKey {
					t.Errorf("Expected key %s, got %s", tt.wantKey, m.Key().Id())
				}

				if m.Value() == nil {
					t.Error("Expected value type, got nil")
				} else if m.Value().Id() != tt.wantElem {
					t.Errorf("Expected value %s, got %s", tt.wantElem, m.Value().Id())
				}
			} else {
				t.Errorf("Expected Map type, got %T", got)
			}
		})
	}
}

func TestTypeResolver_resolveChanTypes(t *testing.T) {
	src := `
	package test
	
	type MyInt int
	type MyIntChan chan MyInt
	type MyIntRecvChan <-chan MyInt
	type MyIntSendChan chan<- MyInt
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
	r := NewDefaultTypeResolver(NewDefaultConfig(), l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name     string
		wantKind typesnew.TypeKind
		wantElem string
		wantDir  typesnew.ChannelDirection
	}{
		{"MyIntChan", typesnew.TypeKindChan, "test.MyInt", typesnew.ChanDirBoth},
		{"MyIntRecvChan", typesnew.TypeKindChan, "test.MyInt", typesnew.ChanDirRecv},
		{"MyIntSendChan", typesnew.TypeKindChan, "test.MyInt", typesnew.ChanDirSend},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := pkg.Scope().Lookup(tt.name)
			if obj == nil {
				t.Fatalf("Type %s not found", tt.name)
			}

			got := r.ResolveType(obj.Type())
			if got == nil {
				t.Errorf("resolveType(%v) = nil", tt.name)
				return
			}

			if got.Kind() != tt.wantKind {
				t.Errorf("resolveType(%v) kind = %v, want %v", tt.name, got.Kind(), tt.wantKind)
			}

			if ch, ok := got.(*typesnew.Chan); ok {
				if ch.Element() == nil {
					t.Error("Expected element type, got nil")
				} else if ch.Element().Id() != tt.wantElem {
					t.Errorf("Expected element %s, got %s", tt.wantElem, ch.Element().Id())
				}

				if ch.Direction() != tt.wantDir {
					t.Errorf("Expected direction %v, got %v", tt.wantDir, ch.Direction())
				}
			} else {
				t.Errorf("Expected Chan type, got %T", got)
			}
		})
	}
}

func TestTypeResolver_resolveFunctions(t *testing.T) {
	src := `
	package test
	
	type MyInt int
	
	func MyFunc(a MyInt, b int) (string, error) {
		return "", nil
	}

	type NamedFunc func(x int) bool
	func (f NamedFunc) DoSomething(y string) int {
		return 0
	}
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
	r := NewDefaultTypeResolver(NewDefaultConfig(), l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name       string
		wantKind   typesnew.TypeKind
		parameters []struct {
			name string
			typ  string
		}
		results []struct {
			name string
			typ  string
		}
		named bool
	}{
		{"MyFunc", typesnew.TypeKindFunction,
			[]struct {
				name string
				typ  string
			}{
				{"a", "test.MyInt"},
				{"b", "int"},
			},
			[]struct {
				name string
				typ  string
			}{
				{"", "string"},
				{"", "error"},
			},
			false,
		},
		{"NamedFunc", typesnew.TypeKindFunction,
			[]struct {
				name string
				typ  string
			}{
				{"x", "int"},
			},
			[]struct {
				name string
				typ  string
			}{
				{"", "bool"},
			},
			true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := pkg.Scope().Lookup(tt.name)
			if obj == nil {
				t.Fatalf("Type %s not found", tt.name)
			}

			got := r.ResolveType(obj.Type())
			if got == nil {
				t.Errorf("resolveType(%v) = nil", tt.name)
				return
			}

			if got.Kind() != tt.wantKind {
				t.Errorf("resolveType(%v) kind = %v, want %v", tt.name, got.Kind(), tt.wantKind)
			}

			if got.IsNamed() != tt.named {
				t.Errorf("Expected named=%v, got %v", tt.named, got.IsNamed())
			}

			if fn, ok := got.(*typesnew.Function); ok {
				if len(fn.Parameters()) != len(tt.parameters) {
					t.Errorf("Expected %d parameters, got %d", len(tt.parameters), len(fn.Parameters()))
				} else {
					for i, param := range fn.Parameters() {
						if param.Name() != tt.parameters[i].name {
							t.Errorf("Expected parameter name %s, got %s", tt.parameters[i].name, param.Name())
						}
						if param.Type().Id() != tt.parameters[i].typ {
							t.Errorf("Expected parameter type %s, got %s", tt.parameters[i].typ, param.Type().Id())
						}
					}
				}

				if len(fn.Results()) != len(tt.results) {
					t.Errorf("Expected %d results, got %d", len(tt.results), len(fn.Results()))
				} else {
					for i, result := range fn.Results() {
						if result.Name() != tt.results[i].name {
							t.Errorf("Expected result name %s, got %s", tt.results[i].name, result.Name())
						}
						if result.Type().Id() != tt.results[i].typ {
							t.Errorf("Expected result type %s, got %s", tt.results[i].typ, result.Type().Id())
						}
					}
				}

			} else {
				t.Errorf("Expected Function type, got %T", got)
			}
		})
	}
}

func TestTypeResolver_resolveEmptyInterface(t *testing.T) {
	src := `
	package test
	
	type MyEmptyInterface interface{}
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
	r := NewDefaultTypeResolver(NewDefaultConfig(), l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	obj := pkg.Scope().Lookup("MyEmptyInterface")
	if obj == nil {
		t.Fatalf("Type MyEmptyInterface not found")
	}

	got := r.ResolveType(obj.Type())
	if got == nil {
		t.Errorf("resolveType(MyEmptyInterface) = nil")
		return
	}
	if got.Kind() != typesnew.TypeKindInterface {
		t.Errorf("resolveType(MyEmptyInterface) kind = %v, want %v", got.Kind(), typesnew.TypeKindInterface)
	}

	iface, ok := got.(*typesnew.Interface)
	if !ok {
		t.Errorf("Expected Interface type, got %T", got)
		return
	}

	if len(iface.Methods()) != 0 {
		t.Errorf("Expected 0 methods, got %d", len(iface.Methods()))
	}
}

func TestTypeResolver_testMakeStruct(t *testing.T) {
	src := `
	package test
	
	type MyStruct struct {
		Field1 int
		Field2 string
	}
		func (s MyStruct) Method1() {}
		func (s *MyStruct) Method2() {}
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
	r := NewDefaultTypeResolver(NewDefaultConfig(), l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name       string
		wantKind   typesnew.TypeKind
		wantFields []struct {
			name string
			typ  string
		}
		wantMethods []string
	}{
		{"MyStruct", typesnew.TypeKindStruct,
			[]struct {
				name string
				typ  string
			}{
				{"Field1", "int"},
				{"Field2", "string"},
			},
			[]string{"Method1", "Method2"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := pkg.Scope().Lookup(tt.name)
			if obj == nil {
				t.Fatalf("Type %s not found", tt.name)
			}

			got := r.ResolveType(obj.Type())
			if got == nil {
				t.Errorf("resolveType(%v) = nil", tt.name)
				return
			}

			if got.Kind() != tt.wantKind {
				t.Errorf("resolveType(%v) kind = %v, want %v", tt.name, got.Kind(), tt.wantKind)
			}
			if strct, ok := got.(*typesnew.Struct); ok {
				if len(strct.Fields()) != len(tt.wantFields) {
					t.Errorf("Expected %d fields, got %d", len(tt.wantFields), len(strct.Fields()))
				} else {
					for i, field := range strct.Fields() {
						if field.Name() != tt.wantFields[i].name {
							t.Errorf("Expected field name %s, got %s", tt.wantFields[i].name, field.Name())
						}
						if field.Type().Id() != tt.wantFields[i].typ {
							t.Errorf("Expected field type %s, got %s", tt.wantFields[i].typ, field.Type().Id())
						}
					}
				}

				if len(strct.Methods()) != len(tt.wantMethods) {
					t.Errorf("Expected %d methods, got %d", len(tt.wantMethods), len(strct.Methods()))
				} else {
					for i, method := range strct.Methods() {
						if method.Name() != tt.wantMethods[i] {
							t.Errorf("Expected method name %s, got %s", tt.wantMethods[i], method.Name())
						}
					}
				}

			} else {
				t.Errorf("Expected Struct type, got %T", got)
			}
		})
	}
}
