package scanner

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/pablor21/goscanner/logger"
	gstypes "github.com/pablor21/goscanner/types"
)

// test basic types
func TestTypeResolver_resolveBasicType(t *testing.T) {
	r := NewDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	// Define test cases for basic types based on the go/types package
	tests := []struct {
		goType   types.Type
		wantKind gstypes.TypeKind
	}{
		{types.Typ[types.Bool], gstypes.TypeKindBasic},
		{types.Typ[types.Byte], gstypes.TypeKindBasic},
		{types.Typ[types.Complex64], gstypes.TypeKindBasic},
		{types.Typ[types.Complex128], gstypes.TypeKindBasic},
		{types.Universe.Lookup("error").Type(), gstypes.TypeKindBasic},
		{types.Universe.Lookup("rune").Type(), gstypes.TypeKindBasic},
		{types.Universe.Lookup("comparable").Type(), gstypes.TypeKindBasic},
		{types.Typ[types.Float32], gstypes.TypeKindBasic},
		{types.Typ[types.Float64], gstypes.TypeKindBasic},
		{types.Typ[types.Int], gstypes.TypeKindBasic},
		{types.Typ[types.Int8], gstypes.TypeKindBasic},
		{types.Typ[types.Int16], gstypes.TypeKindBasic},
		{types.Typ[types.Int32], gstypes.TypeKindBasic},
		{types.Typ[types.Int64], gstypes.TypeKindBasic},
		{types.Typ[types.Rune], gstypes.TypeKindBasic},
		{types.Typ[types.String], gstypes.TypeKindBasic},
		{types.Typ[types.Uint], gstypes.TypeKindBasic},
		{types.Typ[types.Uint8], gstypes.TypeKindBasic},
		{types.Typ[types.Uint16], gstypes.TypeKindBasic},
		{types.Typ[types.Uint32], gstypes.TypeKindBasic},
		{types.Typ[types.Uint64], gstypes.TypeKindBasic},
		{types.Typ[types.Uintptr], gstypes.TypeKindBasic},
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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name           string
		wantKind       gstypes.TypeKind
		wantUnderlying string
	}{
		{"MyError", gstypes.TypeKindInterface, "error"},
		{"MyInt", gstypes.TypeKindBasic, "int"},
		{"MyString", gstypes.TypeKindBasic, "string"},
		{"MyBool", gstypes.TypeKindBasic, "bool"},
		{"MyFloat", gstypes.TypeKindBasic, "float64"},
		{"MyComplex", gstypes.TypeKindBasic, "complex128"},
		{"MyByte", gstypes.TypeKindBasic, "byte"}, // byte is an alias for uint8
		{"MyRune", gstypes.TypeKindBasic, "rune"}, // rune is an alias for int32
		{"MyUintptr", gstypes.TypeKindBasic, "uintptr"},
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
			if basic, ok := got.(*gstypes.Basic); ok {
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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name      string
		wantKind  gstypes.TypeKind
		wantDepth int
		wantElem  string
	}{
		{"MyIntPtr", gstypes.TypeKindPointer, 1, "test.MyInt"},
		{"MyIntPtrPtr", gstypes.TypeKindPointer, 2, "test.MyInt"},
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

			if ptr, ok := got.(*gstypes.Pointer); ok {
				if ptr.Depth() != tt.wantDepth {
					t.Errorf("Expected depth %d, got %d", tt.wantDepth, ptr.Depth())
				}
				if ptr.Elem() == nil {
					t.Error("Expected element type, got nil")
				} else if ptr.Elem().Id() != tt.wantElem {
					t.Errorf("Expected element %s, got %s", tt.wantElem, ptr.Elem().Id())
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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name       string
		wantKind   gstypes.TypeKind
		wantLength int64
		wantElem   string
	}{
		{"MyStringSlice", gstypes.TypeKindSlice, -1, "test.MyString"},
		{"MyStringArray", gstypes.TypeKindArray, 5, "test.MyString"},
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

			if slice, ok := got.(*gstypes.Slice); ok {
				if slice.Len() != tt.wantLength {
					t.Errorf("Expected length %d, got %d", tt.wantLength, slice.Len())
				}
				if slice.Elem() == nil {
					t.Error("Expected element type, got nil")
				} else if slice.Elem().Id() != tt.wantElem {
					t.Errorf("Expected element %s, got %s", tt.wantElem, slice.Elem().Id())
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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name     string
		wantKind gstypes.TypeKind
		wantKey  string
		wantElem string
	}{
		{"MyStringIntMap", gstypes.TypeKindMap, "test.MyString", "test.MyInt"},
		{"MyIntStringMap", gstypes.TypeKindMap, "test.MyInt", "test.MyString"},
		{"MyInterfaceMap", gstypes.TypeKindMap, "interface{}", "interface{}"},
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

			if m, ok := got.(*gstypes.Map); ok {
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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name     string
		wantKind gstypes.TypeKind
		wantElem string
		wantDir  gstypes.ChannelDirection
	}{
		{"MyIntChan", gstypes.TypeKindChan, "test.MyInt", gstypes.ChanDirBoth},
		{"MyIntRecvChan", gstypes.TypeKindChan, "test.MyInt", gstypes.ChanDirRecv},
		{"MyIntSendChan", gstypes.TypeKindChan, "test.MyInt", gstypes.ChanDirSend},
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

			if ch, ok := got.(*gstypes.Chan); ok {
				if ch.Elem() == nil {
					t.Error("Expected element type, got nil")
				} else if ch.Elem().Id() != tt.wantElem {
					t.Errorf("Expected element %s, got %s", tt.wantElem, ch.Elem().Id())
				}

				if ch.Dir() != tt.wantDir {
					t.Errorf("Expected direction %v, got %v", tt.wantDir, ch.Dir())
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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name       string
		wantKind   gstypes.TypeKind
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
		{"MyFunc", gstypes.TypeKindFunction,
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
		{"NamedFunc", gstypes.TypeKindFunction,
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

			if fn, ok := got.(*gstypes.Function); ok {
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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
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
	if got.Kind() != gstypes.TypeKindInterface {
		t.Errorf("resolveType(MyEmptyInterface) kind = %v, want %v", got.Kind(), gstypes.TypeKindInterface)
	}

	iface, ok := got.(*gstypes.Interface)
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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	tests := []struct {
		name       string
		wantKind   gstypes.TypeKind
		wantFields []struct {
			name string
			typ  string
		}
		wantMethods []string
	}{
		{"MyStruct", gstypes.TypeKindStruct,
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
			if strct, ok := got.(*gstypes.Struct); ok {
				// Load fields and methods
				if err := strct.Load(); err != nil {
					t.Fatalf("Failed to load struct: %v", err)
				}

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
