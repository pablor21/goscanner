package goscanner

import (
	"fmt"
	"go/ast"
	"go/types"
	"time"

	"github.com/pablor21/gonnotation"
	"golang.org/x/tools/go/packages"
)

type Scanner interface {
	AddProcessor(processor Processor)
	SetProcessors(processors []Processor)
	Scan() (*ScanningResult, error)
	ScanWithConfig(config *Config) (*ScanningResult, error)
	ScanWithContext(ctx *ScanningContext) (*ScanningResult, error)
	GetTypeResolver() TypeResolver
}

type DefaultScanner struct {
	Processors   []Processor
	Context      *ScanningContext
	TypeResolver TypeResolver
}

func NewScanner() *DefaultScanner {
	return &DefaultScanner{
		TypeResolver: newDefaultTypeResolver(ScanModeBasic),
		Processors:   []Processor{},
	}
}

func (s *DefaultScanner) AddProcessor(processor Processor) {
	s.Processors = append(s.Processors, processor)
}

func (s *DefaultScanner) SetProcessors(processors []Processor) {
	s.Processors = processors
}

func (s *DefaultScanner) Scan() (ret *ScanningResult, err error) {
	return s.ScanWithConfig(NewDefaultConfig())
}

func (s *DefaultScanner) ScanWithConfig(config *Config) (ret *ScanningResult, err error) {
	if config == nil {
		return s.Scan()
	}
	// init the scanning context with the provided configuration
	ctx := NewScanningContext(config)
	return s.ScanWithContext(ctx)
}

func (s *DefaultScanner) ScanWithContext(ctx *ScanningContext) (ret *ScanningResult, err error) {

	// start timer and log start message
	ctx.Logger.Info("Starting scan...")
	totalPackages := 0
	now := time.Now()
	memoryUsage := RSS()
	defer func() {
		ctx.Logger.Info(fmt.Sprintf("Scan completed in %v, found %d types, accross %d packages, memory usage: %dKB", time.Since(now), len(ctx.typesCache), totalPackages, memoryUsage))
	}()

	if ctx == nil || ctx.Config == nil {
		panic(`No scanning context provided or config invalid!`)
	}
	// Initialize the scanning result
	s.Context = ctx

	// determine the scanning mode based on the provided configuration (get the maximum depth of the scan)
	for _, processor := range s.Processors {
		if processor.ScanMode() > ctx.ScanMode {
			ctx.ScanMode = processor.ScanMode()
		}
	}

	// create the glob pattern based on the provided configuration
	scanner := NewGlobScanner()
	pkgs, err := scanner.ScanPackages(ctx.ScanMode, ctx.Config.Packages...)
	if err != nil {
		return nil, err
	}

	// set the scanmode in the type resolver
	s.TypeResolver = newDefaultTypeResolver(ctx.ScanMode)

	// process the packages and generate the scanning result
	for _, pkg := range pkgs {
		// scan the package for types
		err := s.ScanTypes(pkg)
		if err != nil {
			return nil, err
		}
	}

	totalPackages = len(pkgs)
	// Calculate memory usage in MB
	memoryUsage = (RSS() - memoryUsage)

	ret = &ScanningResult{
		Types: s.TypeResolver.GetTypeInfos(),
	}

	// Finalize the scanning result
	//s.Context.Logger.Info(fmt.Sprintf("Done, found %d types, accross %d packages", len(s.Context.typesCache), len(pkgs)))

	// Return the scanning result and any errors encountered
	return ret, err
}

func (s *DefaultScanner) GetTypeResolver() TypeResolver {
	return s.TypeResolver
}

func (s *DefaultScanner) ScanTypes(pkg *packages.Package) error {
	// if pkg == nil || len(pkg.Syntax) == 0 {
	// 	return nil
	// }
	// // Use doc.NewFromFiles - only exported declarations
	// docPkg, err := doc.NewFromFiles(pkg.Fset, pkg.Syntax, pkg.PkgPath)
	// if err != nil {
	// 	return err
	// }

	// // Process the types in the package
	// for _, t := range docPkg.Types {
	// 	// Skip unexported types (defensive check)
	// 	if t == nil || !isExported(t.Name) {
	// 		continue
	// 	}

	// 	// Get the type object from the package scope
	// 	obj := pkg.Types.Scope().Lookup(t.Name)
	// 	if obj == nil || obj.Type() == nil {
	// 		continue
	// 	}

	// 	// Use TypeResolver to resolve the type
	// 	typeInfo := s.TypeResolver.ResolveType(obj.Type())
	// 	if typeInfo != nil {
	// 		s.Context.typesCache[typeInfo.GetCannonicalName()] = typeInfo
	// 		s.Context.Logger.Debug(fmt.Sprintf("resolved type: %s", typeInfo.GetCannonicalName()))
	// 	}
	// }

	// Check package scope for defined types
	// if pkg.Types != nil && pkg.Types.Scope() != nil {
	// 	for _, name := range pkg.Types.Scope().Names() {
	// 		obj := pkg.Types.Scope().Lookup(name)
	// 		if obj != nil {
	// 			s.Context.Logger.Info(fmt.Sprintf("found type: %s", obj.Name()))
	// 		}
	// 	}
	// }

	// // Check TypesInfo.Defs for type definitions
	// if pkg.TypesInfo != nil && pkg.TypesInfo.Defs != nil {
	// 	for ident, obj := range pkg.TypesInfo.Defs {
	// 		if obj != nil && ident != nil {
	// 			// TODO: Process the type definition
	// 			_ = obj
	// 			_ = ident
	// 		}
	// 	}
	// }

	return s.TypeResolver.ProcessPackage(pkg)
}

// func (s *DefaultScanner) ProcessType(t *doc.Type, pkg *packages.Package) {
// 	// Skip unexported types (defensive check)
// 	if t == nil || pkg == nil || !isExported(t.Name) {
// 		// TODO: Handle the case where the type is unexported AND the configuration allows for it
// 		return
// 	}

// 	s.Context.Logger.Debug(fmt.Sprintf("found type: %s, in package %s", t.Name, pkg.PkgPath))

// 	// // Parse annotations and comments from the type's documentation
// 	// annotations := gonnotation.ParseAnnotationsFromText(t.Doc)
// 	// // Parse comments from the type's documentation
// 	// comments := parseComments(t.Doc)

// 	// switch obj.Type().Underlying().(type) {
// 	// case *types.Struct:

// 	// 	// Create loader function for lazy loading struct details
// 	// 	loader := func() (*DetailedTypeInfo, error) {
// 	// 		return s.loadTypeDetails(obj, pkg)
// 	// 	}

// 	// 	info := NewTypeInfo(t.Name, pkg.PkgPath, comments, annotations, loader)
// 	// 	s.Context.typesCache[info.GetCannonicalName()] = info

// 	// case *types.Interface:
// 	// 	// Create loader function for lazy loading interface details
// 	// 	loader := func() (*DetailedTypeInfo, error) {
// 	// 		return s.loadInterfaceDetails(obj, pkg)
// 	// 	}

// 	// 	info := NewInterfaceInfo(t.Name, pkg.PkgPath, comments, annotations, loader)
// 	// 	s.Context.typesCache[info.GetCannonicalName()] = info

// 	// case *types.Basic:
// 	// 	// process the basic type
// 	// 	// ...
// 	// default:
// 	// 	// handle other types as needed
// 	// 	// ...
// 	// }
// }

// loadTypeDetails loads detailed structural information for a struct type
func (s *DefaultScanner) loadTypeDetails(obj types.Object, pkg *packages.Package) (*DetailedTypeInfo, error) {
	details := &DetailedTypeInfo{}

	s.Context.Logger.Info(fmt.Sprintf("Loading struct details for type: %s", obj.Name()))
	// Only load if we actually need the details (based on scan mode)
	if s.Context.ScanMode.Has(ScanModeFields) {
		if (obj.Type()) == nil {
			return details, nil
		}

		// Load struct fields
		if structType, ok := obj.Type().Underlying().(*types.Struct); ok {
			// Find the struct declaration in AST to get field tags and comments
			structDecl := s.findStructDecl(obj.Name(), pkg)

			for i := 0; i < structType.NumFields(); i++ {
				field := structType.Field(i)
				if !isExported(field.Name()) {
					continue
				}

				// Process field type and determine if it's a pointer
				fieldType, isPointer := s.processFieldType(field.Type())

				// Get field tags and annotations
				var annotations []gonnotation.Annotation
				if structDecl != nil && i < len(structDecl.Fields.List) {
					fieldDecl := structDecl.Fields.List[i]

					// Parse annotations from field comments
					if fieldDecl.Doc != nil {
						annotations = gonnotation.ParseAnnotationsFromText(fieldDecl.Doc.Text())
					}

					// Also check for annotations in struct tags
					if fieldDecl.Tag != nil {
						tagAnnotations := s.parseTagAnnotations(fieldDecl.Tag.Value)
						annotations = append(annotations, tagAnnotations...)
					}
				}

				fieldInfo := FieldInfo{
					Name:        field.Name(),
					TypeRef:     fieldType.GetCannonicalName(),
					TypeKind:    fieldType.GetKind(),
					IsPointer:   isPointer,
					Annotations: annotations,
				}
				details.Fields = append(details.Fields, fieldInfo)
			}
		}
	}

	if s.Context.ScanMode.Has(ScanModeMethods) {
		// Load methods - TODO: implement method scanning
		details.Methods = []MethodInfo{}
	}

	return details, nil
}

// findStructDecl finds the AST declaration for a struct type
func (s *DefaultScanner) findStructDecl(typeName string, pkg *packages.Package) *ast.StructType {
	for _, file := range pkg.Syntax {
		for _, decl := range file.Decls {
			if genDecl, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range genDecl.Specs {
					if typeSpec, ok := spec.(*ast.TypeSpec); ok {
						if typeSpec.Name.Name == typeName {
							if structType, ok := typeSpec.Type.(*ast.StructType); ok {
								return structType
							}
						}
					}
				}
			}
		}
	}
	return nil
}

// processFieldType converts a types.Type to TypeInfo and determines if it's a pointer
func (s *DefaultScanner) processFieldType(t types.Type) (TypeInfo, bool) {

	// Check if it's a pointer
	if ptr, ok := t.(*types.Pointer); ok {
		// It's a pointer, get the underlying type
		underlyingTypeInfo := s.convertTypeToTypeInfo(ptr.Elem())
		return underlyingTypeInfo, true
	}

	// Not a pointer
	typeInfo := s.convertTypeToTypeInfo(t)
	return typeInfo, false
}

// convertTypeToTypeInfo converts a types.Type to our TypeInfo interface
func (s *DefaultScanner) convertTypeToTypeInfo(t types.Type) TypeInfo {
	if t == nil {
		return nil
	}
	switch typ := t.(type) {
	case *types.Named:
		// Named type (custom struct, interface, etc.)
		obj := typ.Obj()
		annotations := []gonnotation.Annotation{} // TODO: get from cache if available

		loader := func() (*DetailedTypeInfo, error) {
			// Lazy loader for this type
			return &DetailedTypeInfo{}, nil
		}

		// Handle nil package (e.g., built-in types)
		var pkgPath string
		if obj.Pkg() != nil {
			pkgPath = obj.Pkg().Path()
		}

		return NewNamedTypeInfo(TypeKind(typ.String()), obj.Name(), pkgPath, []string{}, annotations, loader)

	case *types.Basic:
		// Basic type (int, string, bool, etc.)
		return NewNamedTypeInfo(TypeKindField, typ.Name(), "", []string{}, []gonnotation.Annotation{}, nil)

	case *types.Slice:
		// Slice type
		elementType := s.convertTypeToTypeInfo(typ.Elem())
		loader := func() (*DetailedTypeInfo, error) {
			return &DetailedTypeInfo{
				SliceFlag:   true,
				ElementType: elementType,
			}, nil
		}
		return NewNamedTypeInfo(TypeKindSlice, typ.Elem().String(), "", []string{}, []gonnotation.Annotation{}, loader)
	case *types.Array:
		// Array type
		elementType := s.convertTypeToTypeInfo(typ.Elem())
		loader := func() (*DetailedTypeInfo, error) {
			return &DetailedTypeInfo{
				SliceFlag:   true,
				ElementType: elementType,
			}, nil
		}
		return NewNamedTypeInfo(TypeKindArray, typ.Elem().String(), "", []string{}, []gonnotation.Annotation{}, loader)
	case *types.Map:
		// Map type
		keyType := s.convertTypeToTypeInfo(typ.Key())
		valueType := s.convertTypeToTypeInfo(typ.Elem())
		loader := func() (*DetailedTypeInfo, error) {
			return &DetailedTypeInfo{
				MapFlag:   true,
				KeyType:   keyType,
				ValueType: valueType,
			}, nil
		}
		return NewNamedTypeInfo(TypeKindMap, "map["+typ.Key().String()+"]"+typ.Elem().String(), "", []string{}, []gonnotation.Annotation{}, loader)
	case *types.Chan:
		// Channel type
		elementType := s.convertTypeToTypeInfo(typ.Elem())
		loader := func() (*DetailedTypeInfo, error) {
			return &DetailedTypeInfo{
				ChanFlag:    true,
				ChanDir:     int(typ.Dir()),
				ElementType: elementType,
			}, nil
		}
		return NewNamedTypeInfo(TypeKindChannel, "chan "+typ.Elem().String(), "", []string{}, []gonnotation.Annotation{}, loader)
	case *types.Signature:
		// Function type
		return NewNamedTypeInfo(TypeKindFunction, typ.String(), "", []string{}, []gonnotation.Annotation{}, nil)
	default:
		// Fallback for other types
		return NewNamedTypeInfo(TypeKindVariable, t.String(), "", []string{}, []gonnotation.Annotation{}, nil)
	}
}

// parseTagAnnotations extracts annotations from struct field tags
func (s *DefaultScanner) parseTagAnnotations(tagValue string) []gonnotation.Annotation {
	var annotations []gonnotation.Annotation

	// // Remove quotes from tag value
	// if len(tagValue) >= 2 && tagValue[0] == '`' && tagValue[len(tagValue)-1] == '`' {
	// 	tagValue = tagValue[1 : len(tagValue)-1]
	// }

	// // Parse struct tag using reflection
	// tag := reflect.StructTag(tagValue)

	// // Look for common annotation-like tags
	// annotationTags := []string{"json", "xml", "yaml", "validate", "db", "orm"}

	// for _, tagName := range annotationTags {
	// 	if tagVal := tag.Get(tagName); tagVal != "" {
	// 		// Convert struct tag to annotation format
	// 		// e.g., json:"name,omitempty" -> @json(name="name", omitempty=true)
	// 		parts := strings.Split(tagVal, ",")

	// 		annotation := gonnotation.Annotation{
	// 			Name:   tagName,
	// 			Values: make(map[string]interface{}),
	// 		}

	// 		if len(parts) > 0 && parts[0] != "" {
	// 			annotation.Values["name"] = parts[0]
	// 		}

	// 		// Add flags as boolean values
	// 		for i := 1; i < len(parts); i++ {
	// 			if parts[i] != "" {
	// 				annotation.Values[parts[i]] = true
	// 			}
	// 		}

	// 		annotations = append(annotations, annotation)
	// 	}
	// }

	return annotations
}

// loadInterfaceDetails loads detailed information for an interface type
func (s *DefaultScanner) loadInterfaceDetails(obj types.Object, pkg *packages.Package) (*DetailedTypeInfo, error) {
	details := &DetailedTypeInfo{}

	if s.Context.ScanMode.Has(ScanModeMethods) {
		// Load interface methods - TODO: implement
		details.Methods = []MethodInfo{}
	}

	return details, nil
}
