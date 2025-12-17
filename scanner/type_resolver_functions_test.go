package scanner

import (
	"go/types"
	"testing"

	"github.com/pablor21/goscanner/logger"
	. "github.com/pablor21/goscanner/types"
)

func TestMakeFunctionInfo(t *testing.T) {
	r := newDefaultTypeResolver(NewDefaultConfig(), logger.NewDefaultLogger())

	// Create a mock function type
	funcType := types.NewSignatureType(nil, nil, nil, nil, nil, false)

	// Create a mock object for the function
	funcObj := types.NewFunc(0, nil, "TestFunction", funcType)

	// Call makeFunctionInfo
	funcInfo := r.makeFunctionInfo("TestFunction", funcObj)
	if funcInfo == nil {
		t.Fatalf("Expected non-nil FunctionTypeInfo")
	}

	// Validate basic properties
	if funcInfo.ID != "TestFunction" {
		t.Errorf("Expected ID 'TestFunction', got '%s'", funcInfo.ID)
	}
	if funcInfo.DisplayName != "TestFunction" {
		t.Errorf("Expected DisplayName 'TestFunction', got '%s'", funcInfo.DisplayName)
	}
	if funcInfo.TypeKind != TypeKindFunction {
		t.Errorf("Expected TypeKind Function, got '%s'", funcInfo.TypeKind)
	}
}
