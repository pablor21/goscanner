package types

import (
	"go/ast"

	"golang.org/x/tools/go/packages"
)

type CommentPlacement int

const (
	CommentPlacementUnknown CommentPlacement = iota
	CommentPlacementAbove
	CommentPlacementInline
	CommentPlacementPackage
	CommentPlacementImports
	CommentPlacementFile
)

func (cp CommentPlacement) String() string {
	return [...]string{"unknown", "above", "inline", "package", "imports", "file"}[cp]
}

func (cp CommentPlacement) MarshalJSON() ([]byte, error) {
	return []byte(`"` + cp.String() + `"`), nil
}

func (cp *CommentPlacement) UnmarshalJSON(data []byte) error {
	str := string(data)
	switch str {
	case `"above"`:
		*cp = CommentPlacementAbove
	case `"inline"`:
		*cp = CommentPlacementInline
	case `"package"`:
		*cp = CommentPlacementPackage
	case `"imports"`:
		*cp = CommentPlacementImports
	case `"file"`:
		*cp = CommentPlacementFile
	default:
		*cp = CommentPlacementAbove
	}
	return nil
}

// represents a comment associated with a Go code element
type Comment struct {
	ID    string           `json:"id,omitempty"`
	Text  string           `json:"text,omitempty"`
	Place CommentPlacement `json:"placement,omitempty"`
}

func NewComment(text string, place CommentPlacement) Comment {
	// generate a unique ID for the comment

	return Comment{
		ID:    "",
		Text:  text,
		Place: place,
	}
}

// represents a Go source file
type File struct {
	Filename    string    `json:"filename,omitempty"`
	CommentsCol []Comment `json:"comments,omitempty"`
	pkg         *Package  `json:"-"`
}

func NewFileFromAst(file *ast.File, path string) *File {
	filePath := path
	if filePath == "" && file != nil && file.Name != nil {
		filePath = file.Name.Name + ".go"
	}
	return &File{
		Filename:    filePath,
		CommentsCol: []Comment{},
		pkg:         nil,
	}
}

func NewFile(filename, comments string) *File {
	return &File{
		Filename:    filename,
		CommentsCol: []Comment{},
		pkg:         nil,
	}
}

func (f *File) SetPackage(p *Package) {
	f.pkg = p
}

func (f *File) Package() *Package {
	return f.pkg
}

func (f *File) AddComments(comments ...Comment) {
	f.CommentsCol = append(f.CommentsCol, comments...)
}

// represents a Go package
type Package struct {
	Name          string               `json:"name,omitempty"`
	Path          string               `json:"path,omitempty"`
	Imports       []string             `json:"imports,omitempty"`
	module        *Module              `json:"-"`
	Files         map[string]*File     `json:"files,omitempty"`
	pkg           *packages.Package    `json:"-"`
	namedComments map[string][]Comment // maps object IDs to comments
	CommentsCol   []Comment            `json:"comments,omitempty"`
}

func NewPackage(pkg *packages.Package) *Package {
	p := &Package{
		Name:          pkg.Name,
		Path:          pkg.PkgPath,
		Imports:       []string{},
		Files:         make(map[string]*File),
		namedComments: make(map[string][]Comment),
		CommentsCol:   []Comment{},
		module:        nil,
	}
	p.SetPkg(pkg)
	return p
}

func (p *Package) SetModule(m *Module) {
	p.module = m
}

func (p *Package) Module() *Module {
	return p.module
}

func (p *Package) Package() *packages.Package {
	return p.pkg
}

func (p *Package) AddFiles(f ...*File) {
	for _, file := range f {
		file.pkg = p
		p.Files[file.Filename] = file
	}
}

func (p *Package) AddComments(objID string, comments []Comment) {
	p.namedComments[objID] = append(p.namedComments[objID], comments...)
	if objID == "#PACKAGE_DOC" {
		p.CommentsCol = append(p.CommentsCol, comments...)
	}
}

func (p *Package) SetPkg(pkg *packages.Package) {
	p.pkg = pkg
	p.Name = pkg.Name
	p.Path = pkg.PkgPath
	// p.Files = make(map[string]*File)
	// set the imports
	for importPath := range pkg.Imports {
		p.Imports = append(p.Imports, importPath)
	}
}

// reprresents a Go module
type Module struct {
	Path     string             `json:"path,omitempty"`
	Version  string             `json:"version,omitempty"`
	Dir      string             `json:"dir,omitempty"`
	Packages map[string]Package `json:"packages,omitempty"`
}

func NewModule(path, version, dir string) *Module {
	return &Module{
		Path:     path,
		Version:  version,
		Dir:      dir,
		Packages: make(map[string]Package),
	}
}

func (m *Module) AddPackage(p *Package) {
	p.module = m
	m.Packages[p.Path] = *p
}

func (m *Module) GetPackage(path string) *Package {
	if pkg, exists := m.Packages[path]; exists {
		return &pkg
	}
	return nil
}

func (m *Module) AddPackages(pkgs ...*Package) {
	for _, p := range pkgs {
		m.AddPackage(p)
	}
}
