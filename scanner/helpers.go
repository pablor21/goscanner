package scanner

import (
	"fmt"
	"go/types"
	"runtime"
	"strings"
	"syscall"

	. "github.com/pablor21/goscanner/types"
)

// isExported reports whether name is an exported Go symbol
// (that is, whether it begins with an upper-case letter).
func IsExported(name string) bool {
	// return true
	return name != "" && name[0] >= 'A' && name[0] <= 'Z'
}

func MemUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc // bytes allocated and still in use
}

func RSS() uint64 {
	var stat syscall.Rusage
	err := syscall.Getrusage(syscall.RUSAGE_SELF, &stat)
	if err != nil {
		fmt.Println(fmt.Errorf("cannot get memory usage: %s", err))
		return 0
	}
	return uint64(stat.Maxrss) * 1024 // RSS in bytes
}

// a comment is any that does not starts with a @
func ParseComments(doc string) []string {
	if doc == "" {
		return nil
	}

	// Split the documentation text into lines
	lines := strings.Split(doc, "\n")
	if len(lines) == 0 {
		return nil
	}

	var comments []string
	for _, line := range lines {
		// Trim leading and trailing whitespace
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		comments = append(comments, line)

		// if !strings.HasPrefix(line, "@") {
		// 	comments = append(comments, line)
		// }
	}
	if len(comments) == 0 {
		return nil
	}

	return comments
}

// Helper functions for embedded type expansion

// getTypeKind determines the TypeKind for a go/types.Type
func GetTypeKind(t types.Type) TypeKind {
	switch actual := t.(type) {
	case *types.Pointer:
		return GetTypeKind(actual.Elem())
	case *types.TypeParam:
		return TypeKindGenericParam
	case *types.Named:
		underlying := actual.Underlying()
		switch underlying.(type) {
		case *types.Struct:
			return TypeKindStruct
		case *types.Interface:
			return TypeKindInterface
		default:
			return TypeKindBasic
		}
	case *types.Struct:
		return TypeKindStruct
	case *types.Interface:
		return TypeKindInterface
	case *types.Slice:
		return TypeKindSlice
	case *types.Array:
		return TypeKindArray
	case *types.Map:
		return TypeKindMap
	case *types.Chan:
		return TypeKindChannel
	case *types.Basic:
		return TypeKindBasic
	default:
		return TypeKindBasic
	}
}

// isPointerReceiver checks if the receiver type is a pointer
func IsPointerReceiver(t types.Type) bool {
	_, isPointer := t.(*types.Pointer)
	return isPointer
}

// isPointerType checks if a type is a pointer type
func IsPointerType(t types.Type) bool {
	_, isPointer := t.(*types.Pointer)
	return isPointer
}
