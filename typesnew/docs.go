package typesnew

import (
	"strings"

	"github.com/pablor21/goscanner/logger"
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

const (
	PackageCommentID = "#PACKAGE_DOCS"
)

func (cp CommentPlacement) String() string {
	return [...]string{"unknown", "above", "inline", "package", "imports", "file"}[cp]
}

func (cp *CommentPlacement) FromString(str string) {
	switch str {
	case "above":
		*cp = CommentPlacementAbove
	case "inline":
		*cp = CommentPlacementInline
	case "package":
		*cp = CommentPlacementPackage
	case "imports":
		*cp = CommentPlacementImports
	case "file":
		*cp = CommentPlacementFile
	default:
		*cp = CommentPlacementAbove
	}
}

func (cp CommentPlacement) MarshalJSON() ([]byte, error) {
	return []byte(`"` + cp.String() + `"`), nil
}

func (cp *CommentPlacement) UnmarshalJSON(data []byte) error {
	str := string(data)
	cp.FromString(str[1 : len(str)-1])
	return nil
}

// Comment represents a comment associated with a Go code element
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

// Module represents a Go module
type Module struct {
	path     string
	version  string
	packages []*Package
}

// NewModule creates a new module
func NewModule(path string, version string) *Module {
	return &Module{
		path:     path,
		version:  version,
		packages: []*Package{},
	}
}

func (m *Module) Path() string {
	return m.path
}

func (m *Module) Version() string {
	return m.version
}

func (m *Module) Packages() []*Package {
	return m.packages
}

func (m *Module) AddPackage(pkg *Package) {
	m.packages = append(m.packages, pkg)
}

// Package represents a Go package
type Package struct {
	path        string
	name        string
	files       []*File
	types       []Type
	pkgComments []Comment
	comments    map[string][]Comment // key is type/function/field name, value is comments
	pkg         *packages.Package    // the original go/packages.Package
	logger      logger.Logger
}

// NewPackage creates a new package
func NewPackage(path string, name string, pkg *packages.Package) *Package {
	return &Package{
		path:     path,
		name:     name,
		files:    []*File{},
		types:    []Type{},
		comments: make(map[string][]Comment),
		pkg:      pkg,
	}
}

func (p *Package) Path() string {
	return p.path
}

func (p *Package) Name() string {
	return p.name
}

func (p *Package) Files() []*File {
	return p.files
}

func (p *Package) Types() []Type {
	return p.types
}

func (p *Package) AddFile(file *File) {
	p.files = append(p.files, file)
}

func (p *Package) AddType(t Type) {
	p.types = append(p.types, t)
}

func (p *Package) GetComments(name string) []Comment {
	return p.comments[name]
}

func (p *Package) SetComments(name string, comments []Comment) {
	if name == PackageCommentID {
		p.pkgComments = comments
		return
	}
	if p.comments == nil {
		p.comments = make(map[string][]Comment)
	}
	p.comments[name] = comments
}

func (p *Package) GoPackage() *packages.Package {
	return p.pkg
}

// File represents a Go source file
type File struct {
	path     string
	name     string
	comments []Comment // file-level comments
}

// NewFile creates a new file
func NewFile(path string, name string) *File {
	return &File{
		path:     path,
		name:     name,
		comments: []Comment{},
	}
}

func (f *File) Path() string {
	return f.path
}

func (f *File) Name() string {
	return f.name
}

func (f *File) Comments() []Comment {
	return f.comments
}

func (f *File) SetComments(comments []Comment) {
	f.comments = comments
}

func ExtractComments(doc string) string {
	if doc == "" {
		return ""
	}
	var lines []string

	for c := range strings.SplitSeq(doc, "\n") {
		trimmed := strings.TrimSpace(c)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	return strings.Join(lines, "\n")
}
