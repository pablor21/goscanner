package scanner

import (
	"fmt"
	"go/ast"
	"go/token"
	"go/types"
	"runtime"
	"strings"
	"syscall"

	gct "github.com/pablor21/goscanner/types"
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
func GetTypeKind(t types.Type) gct.TypeKind {
	switch actual := t.(type) {
	case *types.Pointer:
		return GetTypeKind(actual.Elem())
	case *types.TypeParam:
		return gct.TypeKindGenericParam
	case *types.Named:
		underlying := actual.Underlying()
		switch underlying.(type) {
		case *types.Struct:
			return gct.TypeKindStruct
		case *types.Interface:
			return gct.TypeKindInterface
		default:
			return gct.TypeKindBasic
		}
	case *types.Struct:
		return gct.TypeKindStruct
	case *types.Interface:
		return gct.TypeKindInterface
	case *types.Slice:
		return gct.TypeKindSlice
	case *types.Array:
		return gct.TypeKindArray
	case *types.Map:
		return gct.TypeKindMap
	case *types.Chan:
		return gct.TypeKindChannel
	case *types.Basic:
		return gct.TypeKindBasic
	default:
		return gct.TypeKindBasic
	}
}

func IsConstant(obj types.Object) bool {
	_, ok := obj.(*types.Const)
	return ok
}

func IsVariable(obj types.Object) bool {
	_, ok := obj.(*types.Var)
	return ok
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

func extractCommentsBetweenPackageAndImports(file *ast.File, fset *token.FileSet) []string {
	var results []string
	pkgEnd := file.Package
	var firstImportPos token.Pos
	var firstDeclPos token.Pos
	if len(file.Decls) > 0 {
		firstDeclPos = file.Decls[0].Pos()
	} else {
		firstDeclPos = token.NoPos
	}

	for _, decl := range file.Decls {
		if gen, ok := decl.(*ast.GenDecl); ok && gen.Tok == token.IMPORT {
			firstImportPos = gen.Pos()
			break
		}
	}
	// Use the earlier of firstImportPos or firstDeclPos (if both exist)
	stopPos := token.NoPos
	if firstImportPos != token.NoPos && firstDeclPos != token.NoPos {
		if firstImportPos < firstDeclPos {
			stopPos = firstImportPos
		} else {
			stopPos = firstDeclPos
		}
	} else if firstImportPos != token.NoPos {
		stopPos = firstImportPos
	} else if firstDeclPos != token.NoPos {
		stopPos = firstDeclPos
	}

	for _, cg := range file.Comments {
		if cg.Pos() > pkgEnd && (stopPos == token.NoPos || cg.End() < stopPos) {
			// If this comment is directly attached to the first declaration, skip it
			attached := false
			if firstDeclPos != token.NoPos && fset != nil {
				commentEndLine := fset.Position(cg.End()).Line
				declLine := fset.Position(firstDeclPos).Line
				if declLine == commentEndLine+1 {
					attached = true
				}
			}
			if !attached {
				results = append(results, strings.TrimSpace(cg.Text()))
			}
		}
	}
	return results
}

// extractComment combines doc comments and inline comments
func extractComment(doc, comment, parentDoc *ast.CommentGroup) []gct.Comment {
	var parts []gct.Comment

	// Add doc comment (above the declaration)
	if doc != nil {
		if text := strings.TrimSpace(doc.Text()); text != "" {
			parts = append(parts, gct.NewComment(text, gct.CommentPlacementAbove))
		}
	}

	// Add inline comment (after the declaration)
	if comment != nil {
		if text := strings.TrimSpace(comment.Text()); text != "" {
			parts = append(parts, gct.NewComment(text, gct.CommentPlacementInline))
		}
	}

	// Fallback to parent doc if no specific comments
	if len(parts) == 0 && parentDoc != nil {
		if text := strings.TrimSpace(parentDoc.Text()); text != "" {
			parts = append(parts, gct.NewComment(text, gct.CommentPlacementAbove))
		}
	}

	return parts
}

// getTypeName extracts the type name from an expression
func getTypeName(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return getTypeName(t.X)
	case *ast.SelectorExpr:
		return getTypeName(t.X) + "." + t.Sel.Name
	default:
		return ""
	}
}
