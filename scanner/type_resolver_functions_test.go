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

// Test simple function with no parameters or returns
func TestTypeResolver_SimpleFunctionNoParamsNoReturns(t *testing.T) {
	src := `
	package test
	
	func SimpleFunc() {}
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

	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	obj := pkg.Scope().Lookup("SimpleFunc")
	if obj == nil {
		t.Fatal("Function SimpleFunc not found")
	}

	funcInfo := r.makeFunctionInfo("test.SimpleFunc", obj)
	if funcInfo == nil {
		t.Fatal("Expected non-nil FunctionTypeInfo")
	}

	if funcInfo.Kind() != TypeKindFunction {
		t.Errorf("Expected TypeKind Function, got %v", funcInfo.Kind())
	}

	if len(funcInfo.Parameters) != 0 {
		t.Errorf("Expected 0 parameters, got %d", len(funcInfo.Parameters))
	}

	if len(funcInfo.Returns) != 0 {
		t.Errorf("Expected 0 returns, got %d", len(funcInfo.Returns))
	}

	if funcInfo.IsVariadic {
		t.Error("Expected IsVariadic to be false")
	}
}

// Test function with basic parameters
func TestTypeResolver_FunctionWithBasicParameters(t *testing.T) {
	src := `
	package test
	
	func Add(a int, b int) {}
	func Concat(x string, y bool, z float64) {}
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

	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	// Test Add function (2 int parameters)
	obj := pkg.Scope().Lookup("Add")
	funcInfo := r.makeFunctionInfo("test.Add", obj)

	if len(funcInfo.Parameters) != 2 {
		t.Errorf("Add: Expected 2 parameters, got %d", len(funcInfo.Parameters))
	}

	if len(funcInfo.Parameters) >= 2 {
		if funcInfo.Parameters[0].Name != "a" {
			t.Errorf("Add: Expected parameter name 'a', got '%s'", funcInfo.Parameters[0].Name)
		}
		if funcInfo.Parameters[0].TypeRef.TypeRefId() != "int" {
			t.Errorf("Add: Expected parameter type 'int', got '%s'", funcInfo.Parameters[0].TypeRef.TypeRefId())
		}
	}

	// Test Concat function (3 different types)
	obj = pkg.Scope().Lookup("Concat")
	funcInfo = r.makeFunctionInfo("test.Concat", obj)

	if len(funcInfo.Parameters) != 3 {
		t.Errorf("Concat: Expected 3 parameters, got %d", len(funcInfo.Parameters))
	}

	expectedTypes := []string{"string", "bool", "float64"}
	expectedNames := []string{"x", "y", "z"}

	for i, param := range funcInfo.Parameters {
		if i >= len(expectedNames) {
			break
		}
		if param.Name != expectedNames[i] {
			t.Errorf("Concat param %d: Expected name '%s', got '%s'", i, expectedNames[i], param.Name)
		}
		if param.TypeRef.TypeRefId() != expectedTypes[i] {
			t.Errorf("Concat param %d: Expected type '%s', got '%s'", i, expectedTypes[i], param.TypeRef.TypeRefId())
		}
	}
}

// Test function with return values
func TestTypeResolver_FunctionWithReturns(t *testing.T) {
	src := `
	package test
	
	func GetInt() int { return 0 }
	func GetMultiple() (int, string, error) { return 0, "", nil }
	func GetNamed() (count int, name string) { return 0, "" }
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

	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	// Test single return
	obj := pkg.Scope().Lookup("GetInt")
	funcInfo := r.makeFunctionInfo("test.GetInt", obj)

	if len(funcInfo.Returns) != 1 {
		t.Errorf("GetInt: Expected 1 return, got %d", len(funcInfo.Returns))
	}

	if len(funcInfo.Returns) >= 1 {
		if funcInfo.Returns[0].TypeRef.TypeRefId() != "int" {
			t.Errorf("GetInt: Expected return type 'int', got '%s'", funcInfo.Returns[0].TypeRef.TypeRefId())
		}
	}

	// Test multiple returns
	obj = pkg.Scope().Lookup("GetMultiple")
	funcInfo = r.makeFunctionInfo("test.GetMultiple", obj)

	if len(funcInfo.Returns) != 3 {
		t.Errorf("GetMultiple: Expected 3 returns, got %d", len(funcInfo.Returns))
	}

	expectedTypes := []string{"int", "string", "error"}
	for i, ret := range funcInfo.Returns {
		if i >= len(expectedTypes) {
			break
		}
		if ret.TypeRef.TypeRefId() != expectedTypes[i] {
			t.Errorf("GetMultiple return %d: Expected type '%s', got '%s'", i, expectedTypes[i], ret.TypeRef.TypeRefId())
		}
	}

	// Test named returns
	obj = pkg.Scope().Lookup("GetNamed")
	funcInfo = r.makeFunctionInfo("test.GetNamed", obj)

	if len(funcInfo.Returns) != 2 {
		t.Errorf("GetNamed: Expected 2 returns, got %d", len(funcInfo.Returns))
	}

	expectedNames := []string{"count", "name"}
	expectedTypes = []string{"int", "string"}

	for i, ret := range funcInfo.Returns {
		if i >= len(expectedNames) {
			break
		}
		if ret.Name != expectedNames[i] {
			t.Errorf("GetNamed return %d: Expected name '%s', got '%s'", i, expectedNames[i], ret.Name)
		}
		if ret.TypeRef.TypeRefId() != expectedTypes[i] {
			t.Errorf("GetNamed return %d: Expected type '%s', got '%s'", i, expectedTypes[i], ret.TypeRef.TypeRefId())
		}
	}
}

// Test variadic functions
func TestTypeResolver_VariadicFunctions(t *testing.T) {
	src := `
	package test
	
	func Sum(numbers ...int) int { return 0 }
	func Printf(format string, args ...interface{}) {}
	func Multi(a int, b string, rest ...float64) {}
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

	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	// Test single variadic parameter
	obj := pkg.Scope().Lookup("Sum")
	funcInfo := r.makeFunctionInfo("test.Sum", obj)

	if !funcInfo.IsVariadic {
		t.Error("Sum: Expected IsVariadic to be true")
	}

	if len(funcInfo.Parameters) != 1 {
		t.Errorf("Sum: Expected 1 parameter, got %d", len(funcInfo.Parameters))
	}

	if len(funcInfo.Parameters) >= 1 {
		if !funcInfo.Parameters[0].IsVariadic {
			t.Error("Sum: Expected parameter to be variadic")
		}
		if funcInfo.Parameters[0].Name != "numbers" {
			t.Errorf("Sum: Expected parameter name 'numbers', got '%s'", funcInfo.Parameters[0].Name)
		}
	}

	// Test variadic with multiple parameters
	obj = pkg.Scope().Lookup("Multi")
	funcInfo = r.makeFunctionInfo("test.Multi", obj)

	if !funcInfo.IsVariadic {
		t.Error("Multi: Expected IsVariadic to be true")
	}

	if len(funcInfo.Parameters) != 3 {
		t.Errorf("Multi: Expected 3 parameters, got %d", len(funcInfo.Parameters))
	}

	if len(funcInfo.Parameters) >= 3 {
		// First two should not be variadic
		if funcInfo.Parameters[0].IsVariadic {
			t.Error("Multi: First parameter should not be variadic")
		}
		if funcInfo.Parameters[1].IsVariadic {
			t.Error("Multi: Second parameter should not be variadic")
		}
		// Last should be variadic
		if !funcInfo.Parameters[2].IsVariadic {
			t.Error("Multi: Last parameter should be variadic")
		}
	}
}

// Test functions with pointer parameters and returns
func TestTypeResolver_FunctionWithPointers(t *testing.T) {
	src := `
	package test
	
	type MyStruct struct {
		Value int
	}
	
	func GetPointer(s *MyStruct) *int { return nil }
	func DoublePointer(p **string) ***bool { return nil }
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

	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	// Test single pointer
	obj := pkg.Scope().Lookup("GetPointer")
	funcInfo := r.makeFunctionInfo("test.GetPointer", obj)

	if len(funcInfo.Parameters) != 1 {
		t.Errorf("GetPointer: Expected 1 parameter, got %d", len(funcInfo.Parameters))
	}

	if len(funcInfo.Parameters) >= 1 {
		if funcInfo.Parameters[0].TypeRef.PointerDepth() != 1 {
			t.Errorf("GetPointer param: Expected pointer depth 1, got %d", funcInfo.Parameters[0].TypeRef.PointerDepth())
		}
		if funcInfo.Parameters[0].TypeRef.TypeRefId() != "test.MyStruct" {
			t.Errorf("GetPointer param: Expected type 'test.MyStruct', got '%s'", funcInfo.Parameters[0].TypeRef.TypeRefId())
		}
	}

	if len(funcInfo.Returns) >= 1 {
		if funcInfo.Returns[0].TypeRef.PointerDepth() != 1 {
			t.Errorf("GetPointer return: Expected pointer depth 1, got %d", funcInfo.Returns[0].TypeRef.PointerDepth())
		}
		if funcInfo.Returns[0].TypeRef.TypeRefId() != "int" {
			t.Errorf("GetPointer return: Expected type 'int', got '%s'", funcInfo.Returns[0].TypeRef.TypeRefId())
		}
	}

	// Test multiple pointer levels
	obj = pkg.Scope().Lookup("DoublePointer")
	funcInfo = r.makeFunctionInfo("test.DoublePointer", obj)

	if len(funcInfo.Parameters) >= 1 {
		if funcInfo.Parameters[0].TypeRef.PointerDepth() != 2 {
			t.Errorf("DoublePointer param: Expected pointer depth 2, got %d", funcInfo.Parameters[0].TypeRef.PointerDepth())
		}
	}

	if len(funcInfo.Returns) >= 1 {
		if funcInfo.Returns[0].TypeRef.PointerDepth() != 3 {
			t.Errorf("DoublePointer return: Expected pointer depth 3, got %d", funcInfo.Returns[0].TypeRef.PointerDepth())
		}
	}
}

// Test functions with slice, map, channel parameters
// func TestTypeResolver_FunctionWithComplexTypes(t *testing.T) {
// 	src := `
// 	package test

// 	func ProcessSlice(items []string) []int { return nil }
// 	func UseMap(data map[string]int) map[int]bool { return nil }
// 	func SendChan(ch chan int) <-chan string { return nil }
// 	func MixedTypes(s []int, m map[string]bool, c chan float64) {}
// 	`

// 	fset := token.NewFileSet()
// 	file, err := parser.ParseFile(fset, "test.go", src, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	cfg := &types.Config{}
// 	pkg, err := cfg.Check("test", fset, []*ast.File{file}, nil)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

// 	// Test slice parameters and returns
// 	obj := pkg.Scope().Lookup("ProcessSlice")
// 	funcInfo := r.makeFunctionInfo("test.ProcessSlice", obj)

// 	if len(funcInfo.Parameters) >= 1 {
// 		paramType := funcInfo.Parameters[0].TypeRef.TypeRef()
// 		if paramType.Kind() != TypeKindSlice {
// 			t.Errorf("ProcessSlice param: Expected slice type, got %v", paramType.Kind())
// 		}
// 	}

// 	if len(funcInfo.Returns) >= 1 {
// 		retType := funcInfo.Returns[0].TypeRef.TypeRef()
// 		if retType.Kind() != TypeKindSlice {
// 			t.Errorf("ProcessSlice return: Expected slice type, got %v", retType.Kind())
// 		}
// 	}

// 	// Test mixed complex types
// 	obj = pkg.Scope().Lookup("MixedTypes")
// 	funcInfo = r.makeFunctionInfo("test.MixedTypes", obj)

// 	if len(funcInfo.Parameters) != 3 {
// 		t.Errorf("MixedTypes: Expected 3 parameters, got %d", len(funcInfo.Parameters))
// 	}

// 	expectedKinds := []TypeKind{TypeKindSlice, TypeKindUnknown, TypeKindUnknown} // map and chan might not be fully resolved
// 	for i, param := range funcInfo.Parameters {
// 		if i >= len(expectedKinds) || i >= 1 {
// 			break
// 		}
// 		if param.TypeRef.TypeRef() != nil && expectedKinds[i] != TypeKindUnknown {
// 			if param.TypeRef.TypeRef().Kind() != expectedKinds[i] {
// 				t.Errorf("MixedTypes param %d: Expected kind %v, got %v", i, expectedKinds[i], param.TypeRef.TypeRef().Kind())
// 			}
// 		}
// 	}
// }

// // Test named function types
// func TestTypeResolver_NamedFunctionTypes(t *testing.T) {
// 	src := `
// 	package test

// 	type Handler func(int) string
// 	type Transformer func(string) (string, error)
// 	type Callback func(...interface{}) bool
// 	`

// 	fset := token.NewFileSet()
// 	file, err := parser.ParseFile(fset, "test.go", src, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	cfg := &types.Config{}
// 	pkg, err := cfg.Check("test", fset, []*ast.File{file}, nil)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

// 	// Test Handler type
// 	obj := pkg.Scope().Lookup("Handler")
// 	funcType := r.ResolveType(obj.Type())

// 	if funcType == nil {
// 		t.Fatal("Handler: Expected non-nil type")
// 	}

// 	if funcType.Kind() != TypeKindFunction {
// 		t.Errorf("Handler: Expected function kind, got %v", funcType.Kind())
// 	}

// 	namedFunc, ok := funcType.(*NamedFunctionInfo)
// 	if !ok {
// 		t.Fatal("Handler: Expected NamedFunctionInfo type")
// 	}

// 	if len(namedFunc.Parameters) != 1 {
// 		t.Errorf("Handler: Expected 1 parameter, got %d", len(namedFunc.Parameters))
// 	}

// 	if len(namedFunc.Returns) != 1 {
// 		t.Errorf("Handler: Expected 1 return, got %d", len(namedFunc.Returns))
// 	}

// 	// Test variadic named function
// 	obj = pkg.Scope().Lookup("Callback")
// 	funcType = r.ResolveType(obj.Type())

// 	namedFunc, ok = funcType.(*NamedFunctionInfo)
// 	if ok && !namedFunc.IsVariadic {
// 		t.Error("Callback: Expected IsVariadic to be true")
// 	}
// }

// Test function methods on named types
func TestTypeResolver_FunctionTypeMethods(t *testing.T) {
	src := `
	package test
	
	type Handler func(int) string
	
	func (h Handler) Invoke(x int) string {
		return h(x)
	}
	
	func (h *Handler) InvokePtr(x int) string {
		return (*h)(x)
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

	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	obj := pkg.Scope().Lookup("Handler")
	funcType := r.ResolveType(obj.Type())

	if funcType == nil {
		t.Fatal("Expected non-nil type")
	}

	// Load methods
	if loadable, ok := funcType.(Loadable); ok {
		loadable.Load()
	}

	namedFunc, ok := funcType.(*NamedFunctionInfo)
	if !ok {
		t.Fatal("Expected NamedFunctionInfo type")
	}

	if len(namedFunc.Methods) == 0 {
		t.Error("Expected methods on Handler type, got 0")
	}

	// Check for both value and pointer receiver methods
	foundInvoke := false
	foundInvokePtr := false

	for _, method := range namedFunc.Methods {
		if method.Name() == "Invoke" {
			foundInvoke = true
			if method.IsPointerReceiver {
				t.Error("Invoke should have value receiver, not pointer")
			}
		}
		if method.Name() == "InvokePtr" {
			foundInvokePtr = true
			if !method.IsPointerReceiver {
				t.Error("InvokePtr should have pointer receiver")
			}
		}
	}

	if !foundInvoke {
		t.Error("Expected to find Invoke method")
	}
	if !foundInvokePtr {
		t.Error("Expected to find InvokePtr method")
	}
}

// Test unexported functions
// func TestTypeResolver_UnexportedFunctions(t *testing.T) {
// 	src := `
// 	package test

// 	func ExportedFunc() {}
// 	func unexportedFunc() {}
// 	`

// 	fset := token.NewFileSet()
// 	file, err := parser.ParseFile(fset, "test.go", src, 0)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	cfg := &types.Config{}
// 	pkg, err := cfg.Check("test", fset, []*ast.File{file}, nil)
// 	if err != nil {
// 		t.Fatal(err)
// 	}

// 	// Test with default config (should skip unexported)
// 	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

// 	obj := pkg.Scope().Lookup("ExportedFunc")
// 	funcInfo := r.makeFunctionInfo("test.ExportedFunc", obj)
// 	if funcInfo == nil {
// 		t.Error("ExportedFunc should be resolved")
// 	}

// 	obj = pkg.Scope().Lookup("unexportedFunc")
// 	funcInfo = r.makeFunctionInfo("test.unexportedFunc", obj)
// 	if funcInfo != nil {
// 		t.Error("unexportedFunc should not be resolved with default config")
// 	}

// 	// Test with config that includes unexported
// 	config := NewDefaultConfig()
// 	config.MethodVisibility = VisibilityLevelAll
// 	r = newDefaultTypeResolver(config, logger.NewDefaultLogger())

// 	obj = pkg.Scope().Lookup("unexportedFunc")
// 	funcInfo = r.makeFunctionInfo("test.unexportedFunc", obj)
// 	if funcInfo == nil {
// 		t.Error("unexportedFunc should be resolved with VisibilityLevelAll")
// 	}
// }
