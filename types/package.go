package types

import "golang.org/x/tools/go/packages"

type CommentPlacement int

const (
	CommentPlacementAbove CommentPlacement = iota + 1
	CommentPlacementInline
)

func (cp CommentPlacement) String() string {
	return [...]string{"above", "inline"}[cp-1]
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
	default:
		*cp = CommentPlacementAbove
	}
	return nil
}

// represents a comment associated with a Go code element
type Comment struct {
	Text  string           `json:"text,omitempty"`
	Place CommentPlacement `json:"placement,omitempty"`
}

func NewComment(text string, place CommentPlacement) Comment {
	return Comment{
		Text:  text,
		Place: place,
	}
}

// represents a Go source file
type File struct {
	Filename string   `json:"filename,omitempty"`
	pkg      *Package `json:"-"`
}

func NewFile(filename, comments string) *File {
	return &File{
		Filename: filename,
		pkg:      nil,
	}
}

// represents a Go package
type Package struct {
	Name     string               `json:"name,omitempty"`
	Path     string               `json:"path,omitempty"`
	Imports  []string             `json:"imports,omitempty"`
	module   *Module              `json:"-"`
	Files    []*File              `json:"files,omitempty"`
	pkg      *packages.Package    `json:"-"`
	Comments map[string][]Comment `json:"comments,omitempty"`
}

func NewPackage(pkg *packages.Package) *Package {
	p := &Package{
		Name:     pkg.Name,
		Path:     pkg.PkgPath,
		Imports:  []string{},
		Files:    []*File{},
		Comments: make(map[string][]Comment),
		module:   nil,
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
		p.Files = append(p.Files, file)
	}
}

func (p *Package) AddComments(objID string, comments []Comment) {
	p.Comments[objID] = append(p.Comments[objID], comments...)
}

func (p *Package) SetPkg(pkg *packages.Package) {
	p.pkg = pkg
	p.Name = pkg.Name
	p.Path = pkg.PkgPath
	p.Files = []*File{}
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
