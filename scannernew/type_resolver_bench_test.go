package scannernew

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"testing"

	"github.com/pablor21/goscanner/logger"
	"golang.org/x/tools/go/packages"
)

// BenchmarkTypeResolver_ResolveComplexPackage benchmarks the type resolver on a realistic package
func BenchmarkTypeResolver_ResolveComplexPackage(b *testing.B) {
	// Use the existing starwars example as benchmark input
	src := `
	package test
	
	import "net/http"
	
	type Handler interface {
		Handle(w http.ResponseWriter, r *http.Request)
	}
	
	type Server struct {
		Mux *http.ServeMux
		Handler Handler
		Config map[string]interface{}
		Data []string
	}
	
	func (s *Server) Start() error { return nil }
	func (s *Server) Stop() error { return nil }
	func (s *Server) Process(data []byte) (string, error) { return "", nil }
	
	type Config struct {
		Port int
		Host string
		TLS bool
	}
	
	func NewServer(cfg Config) *Server { return nil }
	func ProcessRequest(h Handler, data []byte) error { return nil }
	`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		b.Fatal(err)
	}

	typesConfig := &types.Config{Importer: importer.Default()}
	typesPkg, err := typesConfig.Check("test", fset, []*ast.File{file}, nil)
	if err != nil {
		b.Fatal(err)
	}

	pkg := &packages.Package{
		PkgPath: "test",
		Name:    "test",
		Fset:    fset,
		Syntax:  []*ast.File{file},
		Types:   typesPkg,
	}

	config := NewDefaultConfig()
	log := logger.NewDefaultLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := NewDefaultTypeResolver(config, log)
		r.ProcessPackage(pkg)
	}
}

// BenchmarkTypeResolver_StringConcatenation isolates string building performance
func BenchmarkTypeResolver_StringConcatenation(b *testing.B) {
	src := `
	package test
	
	type Example struct {
		Field1 string
		Field2 int
		Field3 bool
		Field4 []string
		Field5 map[string]interface{}
	}
	
	func (e *Example) Method1() {}
	func (e *Example) Method2() {}
	func (e *Example) Method3() {}
	`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		b.Fatal(err)
	}

	typesConfig := &types.Config{}
	typesPkg, err := typesConfig.Check("test", fset, []*ast.File{file}, nil)
	if err != nil {
		b.Fatal(err)
	}

	pkg := &packages.Package{
		PkgPath: "test",
		Name:    "test",
		Fset:    fset,
		Syntax:  []*ast.File{file},
		Types:   typesPkg,
	}

	config := NewDefaultConfig()
	log := logger.NewDefaultLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := NewDefaultTypeResolver(config, log)
		r.ProcessPackage(pkg)
	}
}

// BenchmarkTypeResolver_ExternalPackageDoc benchmarks external doc loading
func BenchmarkTypeResolver_ExternalPackageDoc(b *testing.B) {
	src := `
	package test
	
	import "net/http"
	
	type MyHandler http.Handler
	type MyServeMux http.ServeMux
	type MyRequest http.Request
	type MyResponse http.ResponseWriter
	`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		b.Fatal(err)
	}

	typesConfig := &types.Config{Importer: importer.Default()}
	typesPkg, err := typesConfig.Check("test", fset, []*ast.File{file}, nil)
	if err != nil {
		b.Fatal(err)
	}

	pkg := &packages.Package{
		PkgPath: "test",
		Name:    "test",
		Fset:    fset,
		Syntax:  []*ast.File{file},
		Types:   typesPkg,
	}

	config := NewDefaultConfig()
	log := logger.NewDefaultLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := NewDefaultTypeResolver(config, log)
		r.ProcessPackage(pkg)
	}
}

// BenchmarkTypeResolver_GetCanonicalName benchmarks the closure allocation issue
func BenchmarkTypeResolver_GetCanonicalName(b *testing.B) {
	src := `
	package test
	
	type A struct { B *B }
	type B struct { C *C }
	type C struct { D *D }
	type D struct { E *E }
	type E struct { F *F }
	type F struct { A *A }
	`

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "test.go", src, parser.ParseComments)
	if err != nil {
		b.Fatal(err)
	}

	typesConfig := &types.Config{}
	typesPkg, err := typesConfig.Check("test", fset, []*ast.File{file}, nil)
	if err != nil {
		b.Fatal(err)
	}

	pkg := &packages.Package{
		PkgPath: "test",
		Name:    "test",
		Fset:    fset,
		Syntax:  []*ast.File{file},
		Types:   typesPkg,
	}

	config := NewDefaultConfig()
	log := logger.NewDefaultLogger()

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		r := NewDefaultTypeResolver(config, log)
		r.ProcessPackage(pkg)
	}
}
