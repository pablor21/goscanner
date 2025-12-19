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

// Test generic types with non-struct/interface origins
func TestTypeResolver_GenericOriginTypes(t *testing.T) {
	src := `
	package test

	// CASE 1: Generic slice type
	type GenericSlice[T any] []T

	func (g GenericSlice[T]) Len() int {
		return len(g)
	}

	func (g GenericSlice[T]) Append(val T) GenericSlice[T] {
		return append(g, val)
	}

	type StringSlice = GenericSlice[string]

	// CASE 2: Generic map type
	type GenericMap[K comparable, V any] map[K]V

	func (g GenericMap[K, V]) Get(k K) V {
		return g[k]
	}

	func (g GenericMap[K, V]) Set(k K, v V) {
		g[k] = v
	}

	type StringIntMap = GenericMap[string, int]

	// CASE 3: Generic channel type
	type GenericChan[T any] chan T

	func (g GenericChan[T]) Send(val T) {
		g <- val
	}

	type IntChan = GenericChan[int]

	// CASE 4: Generic function type wrapper
	type GenericFunc[T, U any] struct {
		Fn func(T) U
	}

	func (g GenericFunc[T, U]) Call(val T) U {
		return g.Fn(val)
	}

	type StringToIntFunc = GenericFunc[string, int]

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
	r.currentPkg = gstypes.NewPackage("test", "test", nil)
	r.currentPkg.SetLogger(l)

	t.Run("GenericSlice_DirectInstantiation", func(t *testing.T) {
		obj := pkg.Scope().Lookup("StringSlice")
		if obj == nil {
			t.Fatal("StringSlice not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*gstypes.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Check origin
		if ig.Origin() == nil {
			t.Fatal("Origin is nil")
		}

		if ig.Origin().Id() != "test.GenericSlice" {
			t.Errorf("Origin ID = %v, want test.GenericSlice", ig.Origin().Id())
		}

		// Origin should be a slice type
		originSlice, ok := ig.Origin().(*gstypes.Slice)
		if !ok {
			t.Fatalf("Origin should be Slice, got %T", ig.Origin())
		}

		// Load the origin to get methods
		if err := originSlice.Load(); err != nil {
			t.Fatalf("Failed to load origin: %v", err)
		}

		// Should have methods
		methods := originSlice.Methods()
		if len(methods) != 2 {
			t.Errorf("Expected 2 methods (Len, Append), got %d", len(methods))
		}

		// Check type arguments
		typeArgs := ig.TypeArgs()
		if len(typeArgs) != 1 {
			t.Fatalf("Expected 1 type argument, got %d", len(typeArgs))
		}

		if typeArgs[0].Type.Id() != "string" {
			t.Errorf("Type arg = %v, want string", typeArgs[0].Type.Id())
		}

		// Check that the element type of origin slice is a type parameter
		originElem := originSlice.Elem()
		if originElem == nil {
			t.Fatal("Origin slice element is nil")
		}

		if originElem.Kind() != gstypes.TypeKindTypeParameter {
			t.Errorf("Origin slice element kind = %v, want TypeKindTypeParameter", originElem.Kind())
		}
	})

	t.Run("GenericMap_DirectInstantiation", func(t *testing.T) {
		obj := pkg.Scope().Lookup("StringIntMap")
		if obj == nil {
			t.Fatal("StringIntMap not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*gstypes.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Origin should be a map type
		originMap, ok := ig.Origin().(*gstypes.Map)
		if !ok {
			t.Fatalf("Origin should be Map, got %T", ig.Origin())
		}

		// Load the origin to get methods
		if err := originMap.Load(); err != nil {
			t.Fatalf("Failed to load origin: %v", err)
		}

		// Should have methods
		methods := originMap.Methods()
		if len(methods) != 2 {
			t.Errorf("Expected 2 methods (Get, Set), got %d", len(methods))
		}

		// Check type arguments
		typeArgs := ig.TypeArgs()
		if len(typeArgs) != 2 {
			t.Fatalf("Expected 2 type arguments, got %d", len(typeArgs))
		}

		if typeArgs[0].Param != "K" || typeArgs[0].Type.Id() != "string" {
			t.Errorf("First type arg: param=%v type=%v, want K/string", typeArgs[0].Param, typeArgs[0].Type.Id())
		}

		if typeArgs[1].Param != "V" || typeArgs[1].Type.Id() != "int" {
			t.Errorf("Second type arg: param=%v type=%v, want V/int", typeArgs[1].Param, typeArgs[1].Type.Id())
		}
	})

	t.Run("GenericChan_DirectInstantiation", func(t *testing.T) {
		obj := pkg.Scope().Lookup("IntChan")
		if obj == nil {
			t.Fatal("IntChan not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*gstypes.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// Origin should be a channel type
		originChan, ok := ig.Origin().(*gstypes.Chan)
		if !ok {
			t.Fatalf("Origin should be Chan, got %T", ig.Origin())
		}

		// Load the origin to get methods
		if err := originChan.Load(); err != nil {
			t.Fatalf("Failed to load origin: %v", err)
		}

		// Should have methods
		methods := originChan.Methods()
		if len(methods) != 1 {
			t.Errorf("Expected 1 method (Send), got %d", len(methods))
		}
	})

	t.Run("GenericFunc_DirectInstantiation", func(t *testing.T) {
		obj := pkg.Scope().Lookup("StringToIntFunc")
		if obj == nil {
			t.Fatal("StringToIntFunc not found")
		}

		got := r.ResolveType(obj.Type())
		if got == nil {
			t.Fatal("ResolveType returned nil")
		}

		// Should be InstantiatedGeneric
		ig, ok := got.(*gstypes.InstantiatedGeneric)
		if !ok {
			t.Fatalf("Expected InstantiatedGeneric, got %T", got)
		}

		// This wraps a struct with a function field, so origin should be struct
		originStruct, ok := ig.Origin().(*gstypes.Struct)
		if !ok {
			t.Fatalf("Origin should be Struct (function wrapper), got %T", ig.Origin())
		}

		if err := originStruct.Load(); err != nil {
			t.Fatalf("Failed to load origin: %v", err)
		}

		// Should have the Call method
		methods := originStruct.Methods()
		if len(methods) != 1 {
			t.Errorf("Expected 1 method (Call), got %d", len(methods))
		}

		// Should have the Fn field
		fields := originStruct.Fields()
		if len(fields) != 1 {
			t.Errorf("Expected 1 field (Fn), got %d", len(fields))
		}
	})
}
