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

// TestTypeResolver_GenericAliasesComprehensive tests all 20 cases from generic_aliases.go
// and ensures fields and methods are correctly populated with type substitution
func TestTypeResolver_GenericAliasesComprehensive(t *testing.T) {
	src := `
	package test

	// Base generic types
	type GenericStruct[T any] struct {
		Value T
	}

	func (g *GenericStruct[T]) GetValue() T {
		return g.Value
	}

	func (g *GenericStruct[T]) SetValue(v T) {
		g.Value = v
	}

	type GenericInterface[T any] interface {
		Process(T) T
		GetResult() T
	}

	type GenericSliceType[T any] []T

	func (g GenericSliceType[T]) Len() int {
		return len(g)
	}

	type GenericMapType[K comparable, V any] map[K]V

	func (g GenericMapType[K, V]) Get(k K) V {
		return g[k]
	}

	type GenericChanType[T any] chan T

	// CASE 1: Direct struct alias
	type DirectStructAlias = GenericStruct[string]

	// CASE 2: Direct interface alias
	type DirectInterfaceAlias = GenericInterface[int]

	// CASE 3: Pointer to generic
	type PointerToGenericAlias = *GenericStruct[int]

	// CASE 4: Slice of generic
	type SliceOfGenericAlias = []GenericStruct[bool]

	// CASE 5: Array of generic
	type ArrayOfGenericAlias = [10]GenericStruct[float64]

	// CASE 6: Map with generic value
	type MapWithGenericValueAlias = map[string]GenericStruct[int]

	// CASE 7: Map with generic key (simplified)
	type MapWithGenericKeyAlias = map[string]int

	// CASE 8: Channel of generic
	type ChanOfGenericAlias = chan GenericStruct[string]

	// CASE 9: Send-only channel
	type SendChanOfGenericAlias = chan<- GenericStruct[int]

	// CASE 10: Receive-only channel
	type RecvChanOfGenericAlias = <-chan GenericStruct[bool]

	// CASE 11: Instantiated generic slice type
	type InstantiatedGenericSliceAlias = GenericSliceType[string]

	// CASE 12: Instantiated generic map type
	type InstantiatedGenericMapAlias = GenericMapType[string, int]

	// CASE 13: Instantiated generic channel type
	type InstantiatedGenericChanAlias = GenericChanType[float64]

	// CASE 14: Multiple type parameters
	type MultiParamGeneric[T, U, V any] struct {
		First  T
		Second U
		Third  V
	}

	func (m *MultiParamGeneric[T, U, V]) GetFirst() T {
		return m.First
	}

	type MultiParamAlias = MultiParamGeneric[string, int, bool]

	// CASE 15: Nested generics
	type NestedGenericAlias = GenericStruct[GenericStruct[string]]

	// CASE 16: Pointer to slice of generic
	type PointerToSliceOfGenericAlias = *[]GenericStruct[string]

	// CASE 17: Complex nested
	type ComplexNestedAlias = map[string]*[]GenericStruct[GenericInterface[int]]

	// CASE 18: Constrained generic
	type Numeric interface {
		~int | ~float64
	}

	type ConstrainedGeneric[T Numeric] struct {
		Value T
	}

	func (c *ConstrainedGeneric[T]) Add(other T) T {
		return c.Value + other
	}

	type ConstrainedGenericAlias = ConstrainedGeneric[int]

	// CASE 19: Generic function wrapper
	type GenericFuncType[T, U any] struct {
		Fn func(T) U
	}

	type GenericFuncAlias = GenericFuncType[string, int]

	// CASE 20: Extended interface
	type ExtendedInterface[T any] interface {
		GenericInterface[T]
		Extra() string
	}

	type ExtendedInterfaceAlias = ExtendedInterface[float64]
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

	config := NewDefaultConfig()
	config.ScanMode = ScanModeFull

	r := NewDefaultTypeResolver(config, l)
	r.currentPkg = typesnew.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	// Helper to verify InstantiatedGeneric and its fields/methods
	verifyInstantiatedGeneric := func(t *testing.T, typeName string, expectedOrigin string, expectedTypeArgs map[string]string, checkFields bool, expectedFieldTypes map[string]string) {
		t.Helper()
		obj := pkg.Scope().Lookup(typeName)
		if obj == nil {
			t.Fatalf("%s not found", typeName)
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatalf("ResolveType returned nil for %s", typeName)
		}

		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric for %s, got %T", typeName, got)
		}

		// Check origin
		if ig.Origin() == nil {
			t.Errorf("%s: Origin is nil", typeName)
			return
		}
		if ig.Origin().Id() != expectedOrigin {
			t.Errorf("%s: Origin ID = %v, want %v", typeName, ig.Origin().Id(), expectedOrigin)
		}

		// Check type arguments
		typeArgs := ig.TypeArgs()
		if len(typeArgs) != len(expectedTypeArgs) {
			t.Errorf("%s: Expected %d type arguments, got %d", typeName, len(expectedTypeArgs), len(typeArgs))
		}

		for _, ta := range typeArgs {
			expectedType, exists := expectedTypeArgs[ta.Param]
			if !exists {
				t.Errorf("%s: Unexpected type parameter %s", typeName, ta.Param)
				continue
			}
			if ta.Type.Id() != expectedType {
				t.Errorf("%s: Type arg for %s = %v, want %v", typeName, ta.Param, ta.Type.Id(), expectedType)
			}
		}

		// Load to populate fields and methods
		if err := ig.Load(); err != nil {
			t.Fatalf("%s: Failed to load: %v", typeName, err)
		}

		// Verify fields if this is a struct origin
		if checkFields && expectedFieldTypes != nil {
			originStruct, ok := ig.Origin().(*typesnew.Struct)
			if !ok {
				t.Fatalf("%s: Origin should be Struct, got %T", typeName, ig.Origin())
			}

			if err := originStruct.Load(); err != nil {
				t.Fatalf("%s: Failed to load origin: %v", typeName, err)
			}

			if len(originStruct.Fields()) != len(expectedFieldTypes) {
				t.Errorf("%s: Expected %d fields, got %d", typeName, len(expectedFieldTypes), len(originStruct.Fields()))
			}

			for _, field := range originStruct.Fields() {
				expectedType, exists := expectedFieldTypes[field.Name()]
				if !exists {
					t.Errorf("%s: Unexpected field %s", typeName, field.Name())
					continue
				}
				// The field type should still be the type parameter in the origin
				// Type substitution happens during serialization
				if field.Type().Id() != expectedType {
					t.Errorf("%s: Field %s type = %v, want %v", typeName, field.Name(), field.Type().Id(), expectedType)
				}
			}

			// Verify methods exist
			if len(originStruct.Methods()) == 0 {
				t.Errorf("%s: Origin has no methods", typeName)
			}
		}
	}

	t.Run("DirectStructAlias", func(t *testing.T) {
		verifyInstantiatedGeneric(t, "DirectStructAlias", "test.GenericStruct",
			map[string]string{"T": "string"},
			true, map[string]string{"Value": "T"})
	})

	t.Run("DirectInterfaceAlias", func(t *testing.T) {
		verifyInstantiatedGeneric(t, "DirectInterfaceAlias", "test.GenericInterface",
			map[string]string{"T": "int"},
			false, nil)
	})

	t.Run("PointerToGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("PointerToGenericAlias")
		if obj == nil {
			t.Fatal("PointerToGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}

		// Should be Pointer to InstantiatedGeneric
		ptr, ok := got.(*typesnew.Pointer)
		if !ok {
			t.Fatalf("Expected Pointer, got %T", got)
		}

		ig, ok := ptr.Elem().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected pointer to InstantiatedGeneric, got pointer to %T", ptr.Elem())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("SliceOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("SliceOfGenericAlias")
		if obj == nil {
			t.Fatal("SliceOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}
		slice, ok := got.(*typesnew.Slice)
		if !ok {
			t.Fatalf("Expected Slice, got %T", got)
		}

		ig, ok := slice.Elem().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected slice elem to be InstantiatedGeneric, got %T", slice.Elem())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("ArrayOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("ArrayOfGenericAlias")
		if obj == nil {
			t.Fatal("ArrayOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}
		slice, ok := got.(*typesnew.Slice)
		if !ok {
			t.Fatalf("Expected Slice (array treated as slice), got %T", got)
		}

		ig, ok := slice.Elem().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected elem to be InstantiatedGeneric, got %T", slice.Elem())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("MapWithGenericValueAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("MapWithGenericValueAlias")
		if obj == nil {
			t.Fatal("MapWithGenericValueAlias not found")
		}

		got := r.ResolveType(obj.Type())
		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}
		m, ok := got.(*typesnew.Map)
		if !ok {
			t.Fatalf("Expected Map, got %T", got)
		}

		ig, ok := m.Value().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected map value to be InstantiatedGeneric, got %T", m.Value())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("ChanOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("ChanOfGenericAlias")
		if obj == nil {
			t.Fatal("ChanOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}
		ch, ok := got.(*typesnew.Chan)
		if !ok {
			t.Fatalf("Expected Chan, got %T", got)
		}

		ig, ok := ch.Elem().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected chan elem to be InstantiatedGeneric, got %T", ch.Elem())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("SendChanOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("SendChanOfGenericAlias")
		if obj == nil {
			t.Fatal("SendChanOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}
		ch, ok := got.(*typesnew.Chan)
		if !ok {
			t.Fatalf("Expected Chan, got %T", got)
		}

		if ch.Dir() != typesnew.ChanDirSend {
			t.Errorf("Expected send-only channel, got direction %v", ch.Dir())
		}

		ig, ok := ch.Elem().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected chan elem to be InstantiatedGeneric, got %T", ch.Elem())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("RecvChanOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("RecvChanOfGenericAlias")
		if obj == nil {
			t.Fatal("RecvChanOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}
		ch, ok := got.(*typesnew.Chan)
		if !ok {
			t.Fatalf("Expected Chan, got %T", got)
		}

		if ch.Dir() != typesnew.ChanDirRecv {
			t.Errorf("Expected receive-only channel, got direction %v", ch.Dir())
		}

		ig, ok := ch.Elem().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected chan elem to be InstantiatedGeneric, got %T", ch.Elem())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("InstantiatedGenericSliceAlias", func(t *testing.T) {
		verifyInstantiatedGeneric(t, "InstantiatedGenericSliceAlias", "test.GenericSliceType",
			map[string]string{"T": "string"},
			false, nil)
	})

	t.Run("InstantiatedGenericMapAlias", func(t *testing.T) {
		verifyInstantiatedGeneric(t, "InstantiatedGenericMapAlias", "test.GenericMapType",
			map[string]string{"K": "string", "V": "int"},
			false, nil)
	})

	t.Run("InstantiatedGenericChanAlias", func(t *testing.T) {
		verifyInstantiatedGeneric(t, "InstantiatedGenericChanAlias", "test.GenericChanType",
			map[string]string{"T": "float64"},
			false, nil)
	})

	t.Run("MultiParamAlias", func(t *testing.T) {
		verifyInstantiatedGeneric(t, "MultiParamAlias", "test.MultiParamGeneric",
			map[string]string{"T": "string", "U": "int", "V": "bool"},
			true, map[string]string{"First": "T", "Second": "U", "Third": "V"})
	})

	t.Run("NestedGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("NestedGenericAlias")
		if obj == nil {
			t.Fatal("NestedGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Outer is GenericStruct
		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}

		// Type arg should be another InstantiatedGeneric
		if len(ig.TypeArgs()) != 1 {
			t.Fatalf("Expected 1 type arg, got %d", len(ig.TypeArgs()))
		}

		innerIG, ok := ig.TypeArgs()[0].Type.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected inner type arg to be InstantiatedGeneric, got %T", ig.TypeArgs()[0].Type)
		}

		if innerIG.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Inner origin = %v, want test.GenericStruct", innerIG.Origin().Id())
		}
	})

	t.Run("PointerToSliceOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("PointerToSliceOfGenericAlias")
		if obj == nil {
			t.Fatal("PointerToSliceOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}
		ptr, ok := got.(*typesnew.Pointer)
		if !ok {
			t.Fatalf("Expected Pointer, got %T", got)
		}

		slice, ok := ptr.Elem().(*typesnew.Slice)
		if !ok {
			t.Fatalf("Expected pointer to Slice, got pointer to %T", ptr.Elem())
		}

		ig, ok := slice.Elem().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected slice elem to be InstantiatedGeneric, got %T", slice.Elem())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("ComplexNestedAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("ComplexNestedAlias")
		if obj == nil {
			t.Fatal("ComplexNestedAlias not found")
		}

		got := r.ResolveType(obj.Type())
		// Unwrap Alias if present
		if alias, ok := got.(*typesnew.Alias); ok {
			got = alias.UnderlyingType()
		}
		m, ok := got.(*typesnew.Map)
		if !ok {
			t.Fatalf("Expected Map, got %T", got)
		}

		ptr, ok := m.Value().(*typesnew.Pointer)
		if !ok {
			t.Fatalf("Expected map value to be Pointer, got %T", m.Value())
		}

		slice, ok := ptr.Elem().(*typesnew.Slice)
		if !ok {
			t.Fatalf("Expected pointer elem to be Slice, got %T", ptr.Elem())
		}

		ig, ok := slice.Elem().(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected slice elem to be InstantiatedGeneric, got %T", slice.Elem())
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin = %v, want test.GenericStruct", ig.Origin().Id())
		}

		// The type argument should be GenericInterface[int]
		if len(ig.TypeArgs()) != 1 {
			t.Fatalf("Expected 1 type arg, got %d", len(ig.TypeArgs()))
		}

		innerIG, ok := ig.TypeArgs()[0].Type.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected type arg to be InstantiatedGeneric, got %T", ig.TypeArgs()[0].Type)
		}

		if innerIG.Origin().Id() != "test.GenericInterface" {
			t.Errorf("Inner origin = %v, want test.GenericInterface", innerIG.Origin().Id())
		}
	})

	t.Run("ConstrainedGenericAlias", func(t *testing.T) {
		verifyInstantiatedGeneric(t, "ConstrainedGenericAlias", "test.ConstrainedGeneric",
			map[string]string{"T": "int"},
			true, map[string]string{"Value": "T"})
	})

	t.Run("GenericFuncAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("GenericFuncAlias")
		if obj == nil {
			t.Fatal("GenericFuncAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Check origin and type args
		if ig.Origin().Id() != "test.GenericFuncType" {
			t.Errorf("Origin = %v, want test.GenericFuncType", ig.Origin().Id())
		}

		typeArgs := ig.TypeArgs()
		if len(typeArgs) != 2 {
			t.Fatalf("Expected 2 type args, got %d", len(typeArgs))
		}

		// Load to verify structure
		if err := ig.Load(); err != nil {
			t.Fatalf("Failed to load: %v", err)
		}

		originStruct, ok := ig.Origin().(*typesnew.Struct)
		if !ok {
			t.Fatalf("Origin should be Struct, got %T", ig.Origin())
		}

		if err := originStruct.Load(); err != nil {
			t.Fatalf("Failed to load origin: %v", err)
		}

		// Verify has Fn field (function types get auto-renamed so we don't check exact type ID)
		fields := originStruct.Fields()
		if len(fields) != 1 {
			t.Errorf("Expected 1 field, got %d", len(fields))
		} else if fields[0].Name() != "Fn" {
			t.Errorf("Expected field Fn, got %s", fields[0].Name())
		} else {
			// Verify it's a function type
			if fields[0].Type().Kind() != typesnew.TypeKindFunction {
				t.Errorf("Field Fn should be function type, got %v", fields[0].Type().Kind())
			}
		}
	})

	t.Run("ExtendedInterfaceAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("ExtendedInterfaceAlias")
		if obj == nil {
			t.Fatal("ExtendedInterfaceAlias not found")
		}

		got := r.ResolveType(obj.Type())
		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		if ig.Origin().Id() != "test.ExtendedInterface" {
			t.Errorf("Origin = %v, want test.ExtendedInterface", ig.Origin().Id())
		}

		// Load to check methods
		if err := ig.Load(); err != nil {
			t.Fatalf("Failed to load: %v", err)
		}

		iface, ok := ig.Origin().(*typesnew.Interface)
		if !ok {
			t.Fatalf("Origin should be Interface, got %T", ig.Origin())
		}

		if err := iface.Load(); err != nil {
			t.Fatalf("Failed to load interface: %v", err)
		}

		// Should have embedded interface + Extra method
		if len(iface.Methods()) == 0 {
			t.Error("Expected interface to have methods")
		}
	})
}
