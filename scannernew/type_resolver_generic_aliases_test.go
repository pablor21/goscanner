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

func TestTypeResolver_GenericAliases(t *testing.T) {
	src := `
	package test

	// Base generic struct with methods
	type GenericStruct[T any] struct {
		Value T
	}

	func (g *GenericStruct[T]) GetValue() T {
		return g.Value
	}

	// Base generic interface
	type GenericInterface[T any] interface {
		Process(T) T
	}

	// CASE 1: Direct alias to instantiated generic struct
	type DirectStructAlias = GenericStruct[string]

	// CASE 2: Direct alias to instantiated generic interface
	type DirectInterfaceAlias = GenericInterface[int]

	// CASE 3: Alias to pointer to instantiated generic
	type PointerToGenericAlias = *GenericStruct[int]

	// CASE 4: Alias to slice of instantiated generic
	type SliceOfGenericAlias = []GenericStruct[bool]

	// CASE 5: Alias to array of instantiated generic
	type ArrayOfGenericAlias = [10]GenericStruct[float64]

	// CASE 6: Alias to map with instantiated generic as value
	type MapWithGenericValueAlias = map[string]GenericStruct[int]

	// CASE 7: Alias to channel of instantiated generic
	type ChanOfGenericAlias = chan GenericStruct[string]

	// CASE 8: Multiple type parameters
	type MultiParamGeneric[T, U any] struct {
		First  T
		Second U
	}

	type MultiParamAlias = MultiParamGeneric[string, int]

	// CASE 9: Nested generics
	type NestedGenericAlias = GenericStruct[GenericStruct[string]]

	// CASE 10: Generic with constraints
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

	t.Run("DirectStructAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("DirectStructAlias")
		if obj == nil {
			t.Fatal("DirectStructAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Check origin
		if ig.Origin() == nil {
			t.Error("Origin is nil")
		} else if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin ID = %v, want test.GenericStruct", ig.Origin().Id())
		}

		// Check type arguments
		typeArgs := ig.TypeArgs()
		if len(typeArgs) != 1 {
			t.Fatalf("Expected 1 type argument, got %d", len(typeArgs))
		}

		if typeArgs[0].Param != "T" {
			t.Errorf("Type param name = %v, want T", typeArgs[0].Param)
		}

		if typeArgs[0].Type.Id() != "string" {
			t.Errorf("Type arg = %v, want string", typeArgs[0].Type.Id())
		}

		// Load to get fields and methods
		if err := ig.Load(); err != nil {
			t.Fatalf("Failed to load: %v", err)
		}

		// Check that origin has the field
		originStruct, ok := ig.Origin().(*typesnew.Struct)
		if !ok {
			t.Fatalf("Origin should be Struct, got %T", ig.Origin())
		}

		if err := originStruct.Load(); err != nil {
			t.Fatalf("Failed to load origin: %v", err)
		}

		if len(originStruct.Fields()) != 1 {
			t.Errorf("Origin should have 1 field, got %d", len(originStruct.Fields()))
		}

		if len(originStruct.Methods()) != 1 {
			t.Errorf("Origin should have 1 method, got %d", len(originStruct.Methods()))
		}
	})

	t.Run("DirectInterfaceAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("DirectInterfaceAlias")
		if obj == nil {
			t.Fatal("DirectInterfaceAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Check origin is interface
		if ig.Origin() == nil {
			t.Error("Origin is nil")
		} else if ig.Origin().Kind() != typesnew.TypeKindInterface {
			t.Errorf("Origin kind = %v, want interface", ig.Origin().Kind())
		}

		// Check type arguments
		typeArgs := ig.TypeArgs()
		if len(typeArgs) != 1 {
			t.Fatalf("Expected 1 type argument, got %d", len(typeArgs))
		}

		if typeArgs[0].Type.Id() != "int" {
			t.Errorf("Type arg = %v, want int", typeArgs[0].Type.Id())
		}
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

		// Should be Alias type pointing to a pointer
		alias, ok := got.(*typesnew.Alias)
		if !ok {
			t.Fatalf("Expected Alias, got %T", got)
		}

		// The underlying should be a pointer
		underlying := alias.UnderlyingType()
		if underlying == nil {
			t.Fatal("Underlying is nil")
		}

		ptr, ok := underlying.(*typesnew.Pointer)
		if !ok {
			t.Fatalf("Expected underlying to be Pointer, got %T", underlying)
		}

		// Pointer element should be InstantiatedGeneric
		elem := ptr.Elem()
		ig, ok := elem.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected pointer element to be InstantiatedGeneric, got %T", elem)
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin ID = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("SliceOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("SliceOfGenericAlias")
		if obj == nil {
			t.Fatal("SliceOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be Alias type pointing to a slice
		alias, ok := got.(*typesnew.Alias)
		if !ok {
			t.Fatalf("Expected Alias, got %T", got)
		}

		// The underlying should be a slice
		underlying := alias.UnderlyingType()
		if underlying == nil {
			t.Fatal("Underlying is nil")
		}

		slice, ok := underlying.(*typesnew.Slice)
		if !ok {
			t.Fatalf("Expected underlying to be Slice, got %T", underlying)
		}

		// Slice element should be InstantiatedGeneric
		elem := slice.Elem()
		ig, ok := elem.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected slice element to be InstantiatedGeneric, got %T", elem)
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin ID = %v, want test.GenericStruct", ig.Origin().Id())
		}

		typeArgs := ig.TypeArgs()
		if len(typeArgs) != 1 || typeArgs[0].Type.Id() != "bool" {
			t.Errorf("Expected type arg bool, got %v", typeArgs)
		}
	})

	t.Run("ArrayOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("ArrayOfGenericAlias")
		if obj == nil {
			t.Fatal("ArrayOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be Alias type pointing to an array
		alias, ok := got.(*typesnew.Alias)
		if !ok {
			t.Fatalf("Expected Alias, got %T", got)
		}

		// The underlying should be a slice (arrays are represented as slices with length)
		underlying := alias.UnderlyingType()
		if underlying == nil {
			t.Fatal("Underlying is nil")
		}

		arr, ok := underlying.(*typesnew.Slice)
		if !ok {
			t.Fatalf("Expected underlying to be Slice (array), got %T", underlying)
		}

		// Check array length
		if arr.Len() != 10 {
			t.Errorf("Array length = %d, want 10", arr.Len())
		}

		// Array element should be InstantiatedGeneric
		elem := arr.Elem()
		ig, ok := elem.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected array element to be InstantiatedGeneric, got %T", elem)
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin ID = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("MapWithGenericValueAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("MapWithGenericValueAlias")
		if obj == nil {
			t.Fatal("MapWithGenericValueAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be Alias type pointing to a map
		alias, ok := got.(*typesnew.Alias)
		if !ok {
			t.Fatalf("Expected Alias, got %T", got)
		}

		// The underlying should be a map
		underlying := alias.UnderlyingType()
		if underlying == nil {
			t.Fatal("Underlying is nil")
		}

		m, ok := underlying.(*typesnew.Map)
		if !ok {
			t.Fatalf("Expected underlying to be Map, got %T", underlying)
		}

		// Map value should be InstantiatedGeneric
		value := m.Value()
		ig, ok := value.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected map value to be InstantiatedGeneric, got %T", value)
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin ID = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("ChanOfGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("ChanOfGenericAlias")
		if obj == nil {
			t.Fatal("ChanOfGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be Alias type pointing to a channel
		alias, ok := got.(*typesnew.Alias)
		if !ok {
			t.Fatalf("Expected Alias, got %T", got)
		}

		// The underlying should be a channel
		underlying := alias.UnderlyingType()
		if underlying == nil {
			t.Fatal("Underlying is nil")
		}

		ch, ok := underlying.(*typesnew.Chan)
		if !ok {
			t.Fatalf("Expected underlying to be Chan, got %T", underlying)
		}

		// Channel element should be InstantiatedGeneric
		elem := ch.Elem()
		ig, ok := elem.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected channel element to be InstantiatedGeneric, got %T", elem)
		}

		if ig.Origin().Id() != "test.GenericStruct" {
			t.Errorf("Origin ID = %v, want test.GenericStruct", ig.Origin().Id())
		}
	})

	t.Run("MultiParamAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("MultiParamAlias")
		if obj == nil {
			t.Fatal("MultiParamAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Check type arguments
		typeArgs := ig.TypeArgs()
		if len(typeArgs) != 2 {
			t.Fatalf("Expected 2 type arguments, got %d", len(typeArgs))
		}

		if typeArgs[0].Param != "T" || typeArgs[0].Type.Id() != "string" {
			t.Errorf("First type arg: param=%v type=%v, want T/string", typeArgs[0].Param, typeArgs[0].Type.Id())
		}

		if typeArgs[1].Param != "U" || typeArgs[1].Type.Id() != "int" {
			t.Errorf("Second type arg: param=%v type=%v, want U/int", typeArgs[1].Param, typeArgs[1].Type.Id())
		}
	})

	t.Run("NestedGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("NestedGenericAlias")
		if obj == nil {
			t.Fatal("NestedGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// The type argument should itself be an InstantiatedGeneric
		typeArgs := ig.TypeArgs()
		if len(typeArgs) != 1 {
			t.Fatalf("Expected 1 type argument, got %d", len(typeArgs))
		}

		nestedIG, ok := typeArgs[0].Type.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected nested InstantiatedGeneric, got %T", typeArgs[0].Type)
		}

		nestedTypeArgs := nestedIG.TypeArgs()
		if len(nestedTypeArgs) != 1 || nestedTypeArgs[0].Type.Id() != "string" {
			t.Errorf("Nested type arg should be string, got %v", nestedTypeArgs)
		}
	})

	t.Run("ConstrainedGenericAlias", func(t *testing.T) {
		obj := pkg.Scope().Lookup("ConstrainedGenericAlias")
		if obj == nil {
			t.Fatal("ConstrainedGenericAlias not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*typesnew.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Load to get methods
		if err := ig.Load(); err != nil {
			t.Fatalf("Failed to load: %v", err)
		}

		// Check that origin has the method
		originStruct, ok := ig.Origin().(*typesnew.Struct)
		if !ok {
			t.Fatalf("Origin should be Struct, got %T", ig.Origin())
		}

		if err := originStruct.Load(); err != nil {
			t.Fatalf("Failed to load origin: %v", err)
		}

		if len(originStruct.Methods()) != 1 {
			t.Errorf("Origin should have 1 method, got %d", len(originStruct.Methods()))
		}

		// The type parameter should have the Numeric constraint
		typeParams := originStruct.TypeParams()
		if len(typeParams) != 1 {
			t.Fatalf("Expected 1 type parameter, got %d", len(typeParams))
		}

		constraint := typeParams[0].Constraint()
		if constraint == nil {
			t.Error("Constraint should not be nil")
		} else if constraint.Id() != "test.Numeric" {
			t.Errorf("Constraint ID = %v, want test.Numeric", constraint.Id())
		}
	})
}
