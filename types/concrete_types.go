// Package types defines concrete implementations of various Go types
// such as Basic, Pointer, Slice, Map, Alias, Function, Interface, Struct, Enum, and Value.
// Each type implements the Type interface and provides methods for serialization
// and lazy loading of additional details.
// Usually these types correspond to go/types types but are designed for easier
// serialization and documentation extraction.
package types

import (
	"go/doc"
)

// serializeTypeRef serializes a type as a reference (basic info only)
func serializeTypeRef(t Type) any {
	if t == nil {
		return nil
	}

	// For unnamed types, we need full serialization since they won't appear in the global types registry
	// Named types can be just a reference since they're in the cache
	if !t.IsNamed() {
		return t.Serialize()
	}

	// For InstantiatedGeneric, include full serialization with origin and typeArgs
	if ig, ok := t.(*InstantiatedGeneric); ok {
		return ig.Serialize()
	}

	// For named types, return minimal reference (they're in the global registry)
	return map[string]any{
		"id":   t.Id(),
		"kind": t.Kind(),
	}
}

// serializeTypeOrID returns either a full type object (for complex types like anonymous structs)
// or a minimal reference with id and kind (for named types)
func serializeTypeOrID(t Type) any {
	if t == nil {
		return nil
	}

	// Use the same logic as serializeTypeRef
	return serializeTypeRef(t)
}

// Old: just ID string
// return t.Id()

// Old full reference (commented out)
// return serializeTypeRef(t)

// func getPackagePath(t Type) string {
// 	if t.Package() != nil {
// 		return t.Package().Path()
// 	}
// 	return ""
// }

// Basic represents a basic/primitive type (int, string, bool, etc.)
// For named basic types like `type MyInt int`, the underlying field points to the cached basic type
type Basic struct {
	baseType
	underlying Type // For named basic types, points to the primitive basic type
}

// NewBasic creates a new basic type
func NewBasic(id string, name string) *Basic {
	return &Basic{
		baseType: newBaseType(id, name, TypeKindBasic),
	}
}

// Underlying returns the underlying type (for named basic types)
func (b *Basic) Underlying() Type {
	return b.underlying
}

// SetUnderlying sets the underlying type
func (b *Basic) SetUnderlying(t Type) {
	b.underlying = t
}

func (b *Basic) Serialize() any {
	var underlyingSerialized any
	if b.underlying != nil {
		underlyingSerialized = b.underlying.Serialize()
	}

	return &SerializedBasic{
		SerializedType: b.serializeBase(),
		Underlying:     underlyingSerialized,
	}
}

func (b *Basic) Load() error {
	var err error
	b.loadOnce.Do(func() {
		b.loadComments(false)
		if b.loader != nil {
			err = b.loader(b)
		}
		// Load underlying type if it exists
		if err == nil && b.underlying != nil {
			err = b.underlying.Load()
		}
	})
	return err
}

// Pointer represents a pointer type
type Pointer struct {
	baseType
	elem  Type
	depth int // pointer depth (1 for *, 2 for **, etc.)
}

// NewPointer creates a new pointer type
func NewPointer(id string, name string, elem Type, depth int) *Pointer {
	return &Pointer{
		baseType: newBaseType(id, name, TypeKindPointer),
		elem:     elem,
		depth:    depth,
	}
}

func (p *Pointer) Elem() Type {
	return p.elem
}

func (p *Pointer) Depth() int {
	return p.depth
}

func (p *Pointer) Serialize() any {
	// Avoid calling Load() here to prevent reentrancy deadlocks
	var elemSerialized any
	if p.elem != nil {
		if p.elem.IsNamed() {
			elemSerialized = serializeTypeRef(p.elem)
		} else {
			elemSerialized = p.elem.Serialize()
		}
	}

	structure := p.name
	if p.obj != nil && p.obj.Type() != nil {
		structure = p.obj.Type().Underlying().String()
	} else if p.goType != nil {
		structure = p.goType.String()
	}

	return &SerializedPointer{
		SerializedType: p.serializeBase(),
		Element:        elemSerialized,
		Depth:          p.depth,
		Structure:      structure,
	}
}

func (p *Pointer) Load() error {
	var err error
	p.loadOnce.Do(func() {
		p.loadComments(false)
		if p.loader != nil {
			err = p.loader(p)
		}
		// Don't load element - causes deadlock on circular types
	})
	return err
}

// Slice represents a slice or array type
type Slice struct {
	baseType
	elem Type
	len  int64 // length for arrays, -1 for slices
}

// NewSlice creates a new slice type
func NewSlice(id string, name string, elem Type) *Slice {
	return &Slice{
		baseType: newBaseType(id, name, TypeKindSlice),
		elem:     elem,
		len:      -1,
	}
}

// NewArray creates a new array type
func NewArray(id string, name string, elem Type, length int64) *Slice {
	return &Slice{
		baseType: newBaseType(id, name, TypeKindArray),
		elem:     elem,
		len:      length,
	}
}

func (s *Slice) Elem() Type {
	return s.elem
}

func (s *Slice) Len() int64 {
	return s.len
}

func (s *Slice) IsArray() bool {
	return s.len >= 0
}

func (s *Slice) Serialize() any {
	// Avoid calling Load() here to prevent reentrancy deadlocks

	var elemSerialized any
	if s.elem != nil {
		// If element is a named type, serialize only basic info (reference)
		if s.elem.IsNamed() {
			elemSerialized = serializeTypeRef(s.elem)
		} else {
			elemSerialized = s.elem.Serialize()
		}
	}

	structure := s.name
	if s.obj != nil && s.obj.Type() != nil {
		structure = s.obj.Type().Underlying().String()
	} else if s.goType != nil {
		structure = s.goType.String()
	}

	return &SerializedSlice{
		SerializedType: s.serializeBase(),
		Element:        elemSerialized,
		Length:         s.len,
		Structure:      structure,
	}
}

func (s *Slice) Load() error {
	var err error
	s.loadOnce.Do(func() {
		s.loadComments(false)
		if s.loader != nil {
			err = s.loader(s)
		}
		// Load the element type
		if err == nil && s.elem != nil {
			err = s.elem.Load()
		}
	})
	return err
}

// Chan represents a channel type
type Chan struct {
	baseType
	elem Type
	dir  ChannelDirection
}

// NewChan creates a new channel type
func NewChan(id string, name string, elem Type, dir ChannelDirection) *Chan {
	return &Chan{
		baseType: newBaseType(id, name, TypeKindChan),
		elem:     elem,
		dir:      dir,
	}
}

func (c *Chan) Elem() Type {
	return c.elem
}

func (c *Chan) Dir() ChannelDirection {
	return c.dir
}

func (c *Chan) Serialize() any {
	// Avoid calling Load() here to prevent reentrancy deadlocks
	var elemSerialized any
	if c.elem != nil {
		if c.elem.IsNamed() {
			elemSerialized = serializeTypeRef(c.elem)
		} else {
			elemSerialized = c.elem.Serialize()
		}
	}

	structure := c.name
	if c.obj != nil && c.obj.Type() != nil {
		structure = c.obj.Type().Underlying().String()
	} else if c.goType != nil {
		structure = c.goType.String()
	}

	return &SerializedChan{
		SerializedType: c.serializeBase(),
		Element:        elemSerialized,
		Direction:      c.dir,
		Structure:      structure,
	}
}

func (c *Chan) Load() error {
	var err error
	c.loadOnce.Do(func() {
		c.loadComments(false)
		if c.loader != nil {
			err = c.loader(c)
		}
		// Load the element type
		if err == nil && c.elem != nil {
			err = c.elem.Load()
		}
	})
	return err
}

// Map represents a map type
type Map struct {
	baseType
	key   Type
	value Type
}

// NewMap creates a new map type
func NewMap(id string, name string, key Type, value Type) *Map {
	return &Map{
		baseType: newBaseType(id, name, TypeKindMap),
		key:      key,
		value:    value,
	}
}

func (m *Map) Key() Type {
	return m.key
}

func (m *Map) Value() Type {
	return m.value
}

func (m *Map) Serialize() any {
	// Avoid calling Load() here to prevent reentrancy deadlocks

	var keySerialized any
	if m.key != nil {
		// If key is a named type, serialize only basic info (reference)
		if m.key.IsNamed() {
			keySerialized = serializeTypeRef(m.key)
		} else {
			keySerialized = m.key.Serialize()
		}
	}

	var valueSerialized any
	if m.value != nil {
		// If value is a named type, serialize only basic info (reference)
		if m.value.IsNamed() {
			valueSerialized = serializeTypeRef(m.value)
		} else {
			valueSerialized = m.value.Serialize()
		}
	}

	structure := m.name
	if m.obj != nil && m.obj.Type() != nil {
		structure = m.obj.Type().Underlying().String()
	} else if m.goType != nil {
		structure = m.goType.String()
	}

	return &SerializedMap{
		SerializedType: m.serializeBase(),
		Key:            keySerialized,
		Value:          valueSerialized,
		Structure:      structure,
	}
}

func (m *Map) Load() error {
	var err error
	m.loadOnce.Do(func() {
		if m.loader != nil {
			err = m.loader(m)
		}
		// Load key type
		if err == nil && m.key != nil {
			err = m.key.Load()
		}
		// Load value type
		if err == nil && m.value != nil {
			err = m.value.Load()
		}
	})
	return err
}

// Alias represents a type alias
type Alias struct {
	baseType
	underlying Type
}

// NewAlias creates a new alias type
func NewAlias(id string, name string, underlying Type) *Alias {
	return &Alias{
		baseType:   newBaseType(id, name, TypeKindAlias),
		underlying: underlying,
	}
}

func (a *Alias) UnderlyingType() Type {
	return a.underlying
}

func (a *Alias) Serialize() any {
	// Avoid calling Load() here to prevent reentrancy deadlocks
	var underlyingSerialized any
	if a.underlying != nil {
		underlyingSerialized = a.underlying.Serialize()
	}

	return &SerializedAlias{
		SerializedType: a.serializeBase(),
		Underlying:     underlyingSerialized,
	}
}

func (a *Alias) Load() error {
	var err error
	a.loadOnce.Do(func() {
		a.loadComments(false)
		if a.loader != nil {
			err = a.loader(a)
		}
		// Load underlying type
		if err == nil && a.underlying != nil {
			err = a.underlying.Load()
		}
	})
	return err
}

// Parameter represents a function/method parameter
type Parameter struct {
	name       string
	paramType  Type
	isVariadic bool
}

// NewParameter creates a new parameter
func NewParameter(name string, paramType Type, isVariadic bool) *Parameter {
	return &Parameter{
		name:       name,
		paramType:  paramType,
		isVariadic: isVariadic,
	}
}

func (p *Parameter) Name() string {
	return p.name
}

func (p *Parameter) Type() Type {
	return p.paramType
}

func (p *Parameter) IsVariadic() bool {
	return p.isVariadic
}

// Result represents a function/method return value
type Result struct {
	name       string
	resultType Type
}

// NewResult creates a new result
func NewResult(name string, resultType Type) *Result {
	return &Result{
		name:       name,
		resultType: resultType,
	}
}

func (r *Result) Name() string {
	return r.name
}

func (r *Result) Type() Type {
	return r.resultType
}

// Function represents a function/signature type
type Function struct {
	baseType
	params     []*Parameter
	results    []*Result
	isVariadic bool
	docFunc    *doc.Func        // for package-level functions
	structure  string           // full signature string
	typeParams []*TypeParameter // type parameters for generic functions
}

// NewFunction creates a new function type
func NewFunction(id string, name string) *Function {
	return &Function{
		baseType:   newBaseType(id, name, TypeKindFunction),
		params:     []*Parameter{},
		results:    []*Result{},
		typeParams: []*TypeParameter{},
	}
}

func (f *Function) Parameters() []*Parameter {
	return f.params
}

func (f *Function) Results() []*Result {
	return f.results
}

func (f *Function) IsVariadic() bool {
	return f.isVariadic
}

func (f *Function) AddParameter(param *Parameter) {
	f.params = append(f.params, param)
	if param.IsVariadic() {
		f.isVariadic = true
	}
}

func (f *Function) AddResult(result *Result) {
	f.results = append(f.results, result)
}

func (f *Function) DocFunc() *doc.Func {
	return f.docFunc
}

func (f *Function) SetDocFunc(docFunc *doc.Func) {
	f.docFunc = docFunc
}

func (f *Function) SetStructure(structure string) {
	f.structure = structure
}

func (f *Function) TypeParams() []*TypeParameter {
	return f.typeParams
}

func (f *Function) AddTypeParam(tp *TypeParameter) {
	f.typeParams = append(f.typeParams, tp)
}

func (f *Function) Serialize() any {
	// Removed Load call as per requirement
	params := make([]*SerializedParameter, len(f.params))
	for i, p := range f.params {
		params[i] = &SerializedParameter{
			Name:       p.name,
			Type:       serializeTypeOrID(p.paramType),
			IsVariadic: p.isVariadic,
		}
		// Old full serialization logic (commented out)
		// var paramTypeSerialized any
		// if p.paramType != nil {
		// 	if p.paramType.IsNamed() {
		// 		paramTypeSerialized = serializeTypeRef(p.paramType)
		// 	} else {
		// 		paramTypeSerialized = p.paramType.Serialize()
		// 	}
		// }
	}

	results := make([]*SerializedResult, len(f.results))
	for i, r := range f.results {
		results[i] = &SerializedResult{
			Name: r.name,
			Type: serializeTypeOrID(r.resultType),
		}
		// Old full serialization logic (commented out)
		// var resultTypeSerialized any
		// if r.resultType != nil {
		// 	if r.resultType.IsNamed() {
		// 		resultTypeSerialized = serializeTypeRef(r.resultType)
		// 	} else {
		// 		resultTypeSerialized = r.resultType.Serialize()
		// 	}
		// }
	}

	typeParams := make([]*SerializedTypeParameter, len(f.typeParams))
	for i, tp := range f.typeParams {
		typeParams[i] = tp.Serialize().(*SerializedTypeParameter)
	}

	return &SerializedFunction{
		SerializedType: f.serializeBase(),
		Parameters:     params,
		Results:        results,
		IsVariadic:     f.isVariadic,
		Structure:      f.structure,
		TypeParams:     typeParams,
	}
}

func (f *Function) Load() error {
	var err error
	f.loadOnce.Do(func() {
		f.loadComments(false)
		if f.loader != nil {
			err = f.loader(f)
		}
		// Load parameter types
		if err == nil {
			for _, p := range f.params {
				if p.paramType != nil {
					err = p.paramType.Load()
					if err != nil {
						return
					}
				}
			}
		}
		// Load result types
		if err == nil {
			for _, r := range f.results {
				if r.resultType != nil {
					err = r.resultType.Load()
					if err != nil {
						return
					}
				}
			}
		}
	})
	return err
}

// Interface represents an interface type
type Interface struct {
	baseType
	embeds     []Type           // embedded types
	typeParams []*TypeParameter // type parameters for generic interfaces
}

// NewInterface creates a new interface type
func NewInterface(id string, name string) *Interface {
	return &Interface{
		baseType:   newBaseType(id, name, TypeKindInterface),
		typeParams: []*TypeParameter{},
	}
}

func (i *Interface) Serialize() any {
	// Avoid calling Load() here to prevent reentrancy deadlocks

	embeds := make([]any, len(i.embeds))
	for idx, e := range i.embeds {
		embeds[idx] = serializeTypeRef(e)
	}

	methods := make([]*SerializedMethod, len(i.methods))
	for idx, m := range i.methods {
		methods[idx] = m.Serialize().(*SerializedMethod)
	}

	typeParams := make([]*SerializedTypeParameter, len(i.typeParams))
	for idx, tp := range i.typeParams {
		typeParams[idx] = tp.Serialize().(*SerializedTypeParameter)
	}

	return &SerializedInterface{
		SerializedType: i.serializeBase(),
		Embeds:         embeds,
		Methods:        methods,
		TypeParams:     typeParams,
	}
}

func (i *Interface) AddEmbed(embed Type) {
	i.embeds = append(i.embeds, embed)
}

func (i *Interface) Embeds() []Type {
	return i.embeds
}

func (i *Interface) TypeParams() []*TypeParameter {
	return i.typeParams
}

func (i *Interface) AddTypeParam(tp *TypeParameter) {
	i.typeParams = append(i.typeParams, tp)
}

func (i *Interface) Load() error {
	var err error
	i.loadOnce.Do(func() {
		i.loadComments(false)
		if i.loader != nil {
			err = i.loader(i)
		}
	})
	return err
}

// Struct represents a struct type
type Struct struct {
	baseType
	embeds     []Type // embedded types
	fields     []*Field
	typeParams []*TypeParameter // type parameters for generic structs
}

// NewStruct creates a new struct type
func NewStruct(id string, name string) *Struct {
	return &Struct{
		baseType:   newBaseType(id, name, TypeKindStruct),
		fields:     []*Field{},
		typeParams: []*TypeParameter{},
	}
}

func (s *Struct) Fields() []*Field {
	return s.fields
}

func (s *Struct) AddField(field *Field) {
	s.fields = append(s.fields, field)
	field.parent = s
	field.pkg = s.pkg
}

func (s *Struct) AddEmbed(embed Type) {
	s.embeds = append(s.embeds, embed)
}

func (s *Struct) Embeds() []Type {
	return s.embeds
}

func (s *Struct) TypeParams() []*TypeParameter {
	return s.typeParams
}

func (s *Struct) AddTypeParam(tp *TypeParameter) {
	s.typeParams = append(s.typeParams, tp)
}

func (s *Struct) Serialize() any {
	// Avoid calling Load() here to prevent reentrancy deadlocks

	embeds := make([]any, len(s.embeds))
	for i, e := range s.embeds {
		embeds[i] = serializeTypeRef(e)
	}

	fields := make([]*SerializedField, len(s.fields))
	for i, f := range s.fields {
		fields[i] = f.Serialize().(*SerializedField)
	}

	methods := make([]*SerializedMethod, len(s.methods))
	for i, m := range s.methods {
		methods[i] = m.Serialize().(*SerializedMethod)
	}

	typeParams := make([]*SerializedTypeParameter, len(s.typeParams))
	for i, tp := range s.typeParams {
		typeParams[i] = tp.Serialize().(*SerializedTypeParameter)
	}

	return &SerializedStruct{
		SerializedType: s.serializeBase(),
		Embeds:         embeds,
		Fields:         fields,
		Methods:        methods,
		TypeParams:     typeParams,
	}
}

func (s *Struct) Load() error {
	var err error
	s.loadOnce.Do(func() {
		s.loadComments(false)
		if s.loader != nil {
			err = s.loader(s)
		}
		// Don't load fields/methods - causes deadlock on circular types
	})
	return err
}

// Value represents a constant or variable
type Value struct {
	baseType
	value     any  // the actual constant/variable value
	valueType Type // the type of this value
	parent    Type // parent type (for enum values)
}

// NewConstant creates a new constant value
func NewConstant(id string, name string, valueType Type, value any) *Value {
	return &Value{
		baseType:  newBaseType(id, name, TypeKindConstant),
		value:     value,
		valueType: valueType,
	}
}

// NewVariable creates a new variable value
func NewVariable(id string, name string, valueType Type) *Value {
	return &Value{
		baseType:  newBaseType(id, name, TypeKindVariable),
		valueType: valueType,
	}
}

func (v *Value) Value() any {
	return v.value
}

func (v *Value) ValueType() Type {
	return v.valueType
}

func (v *Value) Parent() Type {
	return v.parent
}

func (v *Value) SetParent(parent Type) {
	v.parent = parent
}

func (v *Value) Serialize() any {
	parentID := ""
	if v.parent != nil {
		parentID = v.parent.Id()
	}

	var valueTypeSerialized any
	if v.valueType != nil {
		valueTypeSerialized = serializeTypeRef(v.valueType)
	}

	return &SerializedValue{
		SerializedType: v.serializeBase(),
		Value:          v.value,
		ValueType:      valueTypeSerialized,
		Parent:         parentID,
	}
}

func (v *Value) Load() error {
	var err error
	v.loadOnce.Do(func() {
		v.loadComments(false)
		if v.loader != nil {
			err = v.loader(v)
		}
		// Load the value type
		if err == nil && v.valueType != nil {
			err = v.valueType.Load()
		}
	})
	return err
}

// TypeParameter represents a generic type parameter (e.g., T in List[T])
type TypeParameter struct {
	baseType
	index      int  // Position in type parameter list (0-based)
	constraint Type // The constraint type (any, comparable, custom interface, or union)
}

// NewTypeParameter creates a new type parameter
func NewTypeParameter(id string, name string, index int, constraint Type) *TypeParameter {
	return &TypeParameter{
		baseType:   newBaseType(id, name, TypeKindTypeParameter),
		index:      index,
		constraint: constraint,
	}
}

func (tp *TypeParameter) Index() int {
	return tp.index
}

func (tp *TypeParameter) Constraint() Type {
	return tp.constraint
}

func (tp *TypeParameter) Serialize() any {
	return &SerializedTypeParameter{
		SerializedType: tp.serializeBase(),
		Index:          tp.index,
		Constraint:     serializeTypeOrID(tp.constraint),
	}
	// Old: full constraint serialization (commented out)
	// var constraintSerialized any
	// if tp.constraint != nil {
	// 	// Always serialize the full constraint structure to show what it is
	// 	constraintSerialized = tp.constraint.Serialize()
	// }
}

func (tp *TypeParameter) Load() error {
	var err error
	tp.loadOnce.Do(func() {
		tp.loadComments(false)
		if tp.loader != nil {
			err = tp.loader(tp)
		}
		if err == nil && tp.constraint != nil {
			err = tp.constraint.Load()
		}
	})
	return err
}

// UnionTerm represents a single term in a union constraint
type UnionTerm struct {
	typ           Type
	approximation bool // true for ~T, false for T
}

func NewUnionTerm(typ Type, approximation bool) *UnionTerm {
	return &UnionTerm{
		typ:           typ,
		approximation: approximation,
	}
}

func (ut *UnionTerm) Type() Type {
	return ut.typ
}

func (ut *UnionTerm) Approximation() bool {
	return ut.approximation
}

// Union represents a union constraint (e.g., int | string | ~float64)
type Union struct {
	baseType
	terms []*UnionTerm
}

// NewUnion creates a new union type
func NewUnion(id string, name string, terms []*UnionTerm) *Union {
	return &Union{
		baseType: newBaseType(id, name, TypeKindUnion),
		terms:    terms,
	}
}

func (u *Union) Terms() []*UnionTerm {
	return u.terms
}

func (u *Union) Serialize() any {
	serializedTerms := make([]SerializedUnionTerm, len(u.terms))
	for i, term := range u.terms {
		serializedTerms[i] = SerializedUnionTerm{
			Type:          serializeTypeOrID(term.typ),
			Approximation: term.approximation,
		}
	}

	return &SerializedUnion{
		SerializedType: u.serializeBase(),
		Terms:          serializedTerms,
	}
}

func (u *Union) Load() error {
	var err error
	u.loadOnce.Do(func() {
		u.loadComments(false)
		if u.loader != nil {
			err = u.loader(u)
		}
		// Load all term types
		if err == nil {
			for _, term := range u.terms {
				if term.typ != nil {
					if loadErr := term.typ.Load(); loadErr != nil {
						err = loadErr
						return
					}
				}
			}
		}
	})
	return err
}

// InstantiatedGeneric represents a generic type with concrete type arguments
// (e.g., List[int] where List[T] is the generic definition)
// TypeArgument represents a type argument for an instantiated generic
type TypeArgument struct {
	Param string // The type parameter name (e.g., "T", "K", "V")
	Index int    // The position in the type parameter list
	Type  Type   // The concrete type being substituted
}

type InstantiatedGeneric struct {
	baseType
	origin   Type           // The base generic type (e.g., List[T])
	typeArgs []TypeArgument // The concrete type arguments with parameter info
}

// NewInstantiatedGeneric creates a new instantiated generic type
func NewInstantiatedGeneric(id string, name string, origin Type, typeArgs []TypeArgument) *InstantiatedGeneric {
	return &InstantiatedGeneric{
		baseType: newBaseType(id, name, TypeKindInstantiated),
		origin:   origin,
		typeArgs: typeArgs,
	}
}

func (ig *InstantiatedGeneric) Origin() Type {
	return ig.origin
}

func (ig *InstantiatedGeneric) TypeArgs() []TypeArgument {
	return ig.typeArgs
}

func (ig *InstantiatedGeneric) Serialize() any {
	serializedArgs := make([]any, len(ig.typeArgs))
	for i, arg := range ig.typeArgs {
		serializedArgs[i] = map[string]any{
			"param": arg.Param,
			"index": arg.Index,
			"type":  serializeTypeOrID(arg.Type),
		}
	}

	// Build type parameter substitution map
	typeSubstitutions := make(map[string]any)
	for _, arg := range ig.typeArgs {
		if arg.Type != nil {
			typeSubstitutions[arg.Param] = serializeTypeOrID(arg.Type)
		}
	}

	// Get origin ID
	originID := ""
	if ig.origin != nil {
		originID = ig.origin.Id()
	}

	result := map[string]any{
		"id":       ig.id,
		"kind":     ig.kind,
		"typeArgs": serializedArgs,
		"origin":   originID,
	}

	// Include base serialization fields (name, package, etc.)
	baseData := ig.serializeBase()
	result["name"] = baseData.Name
	result["named"] = baseData.IsNamed
	if baseData.Package != "" {
		result["package"] = baseData.Package
	}
	if len(baseData.Comments) > 0 {
		result["comments"] = baseData.Comments
	}

	// Copy fields and methods from origin
	if ig.origin != nil {
		switch origin := ig.origin.(type) {
		case *Struct:
			if fields := origin.Fields(); len(fields) > 0 {
				serializedFields := make([]any, len(fields))
				for i, f := range fields {
					fieldData := f.Serialize()
					// Substitute type parameters in field
					ig.substituteTypes(fieldData, typeSubstitutions)
					serializedFields[i] = fieldData
				}
				result["fields"] = serializedFields
			}
			if embeds := origin.Embeds(); len(embeds) > 0 {
				serializedEmbeds := make([]any, len(embeds))
				for i, e := range embeds {
					embedData := serializeTypeOrID(e)
					// Substitute if embed is a type parameter
					if embedMap, ok := embedData.(map[string]any); ok {
						if embedMap["kind"] == "type_parameter" {
							if embedID, ok := embedMap["id"].(string); ok {
								if concrete, exists := typeSubstitutions[embedID]; exists {
									embedData = concrete
								}
							}
						}
					}
					serializedEmbeds[i] = embedData
				}
				result["embeds"] = serializedEmbeds
			}
			if methods := origin.Methods(); len(methods) > 0 {
				serializedMethods := make([]any, len(methods))
				for i, m := range methods {
					methodData := m.Serialize()
					// Substitute type parameters in method
					ig.substituteTypes(methodData, typeSubstitutions)
					serializedMethods[i] = methodData
				}
				result["methods"] = serializedMethods
			}
		case *Interface:
			if methods := origin.Methods(); len(methods) > 0 {
				serializedMethods := make([]any, len(methods))
				for i, m := range methods {
					methodData := m.Serialize()
					// Substitute type parameters in method
					ig.substituteTypes(methodData, typeSubstitutions)
					serializedMethods[i] = methodData
				}
				result["methods"] = serializedMethods
			}
			if embeds := origin.Embeds(); len(embeds) > 0 {
				serializedEmbeds := make([]any, len(embeds))
				for i, e := range embeds {
					embedData := serializeTypeOrID(e)
					// Substitute if embed is a type parameter
					if embedMap, ok := embedData.(map[string]any); ok {
						if embedMap["kind"] == "type_parameter" {
							if embedID, ok := embedMap["id"].(string); ok {
								if concrete, exists := typeSubstitutions[embedID]; exists {
									embedData = concrete
								}
							}
						}
					}
					serializedEmbeds[i] = embedData
				}
				result["embeds"] = serializedEmbeds
			}
		case *Slice:
			// Include element type with substitution
			if elem := origin.Elem(); elem != nil {
				elemData := serializeTypeOrID(elem)
				// Substitute if element is a type parameter
				if elemMap, ok := elemData.(map[string]any); ok {
					if elemMap["kind"] == "type_parameter" {
						if elemID, ok := elemMap["id"].(string); ok {
							if concrete, exists := typeSubstitutions[elemID]; exists {
								elemData = concrete
							}
						}
					}
				}
				result["elem"] = elemData
			}
			// Include length for arrays
			if origin.Len() > 0 {
				result["len"] = origin.Len()
			}
			// Include methods
			if methods := origin.Methods(); len(methods) > 0 {
				serializedMethods := make([]any, len(methods))
				for i, m := range methods {
					methodData := m.Serialize()
					ig.substituteTypes(methodData, typeSubstitutions)
					serializedMethods[i] = methodData
				}
				result["methods"] = serializedMethods
			}
		case *Map:
			// Include key and value types with substitution
			if key := origin.Key(); key != nil {
				keyData := serializeTypeOrID(key)
				if keyMap, ok := keyData.(map[string]any); ok {
					if keyMap["kind"] == "type_parameter" {
						if keyID, ok := keyMap["id"].(string); ok {
							if concrete, exists := typeSubstitutions[keyID]; exists {
								keyData = concrete
							}
						}
					}
				}
				result["key"] = keyData
			}
			if value := origin.Value(); value != nil {
				valueData := serializeTypeOrID(value)
				if valueMap, ok := valueData.(map[string]any); ok {
					if valueMap["kind"] == "type_parameter" {
						if valueID, ok := valueMap["id"].(string); ok {
							if concrete, exists := typeSubstitutions[valueID]; exists {
								valueData = concrete
							}
						}
					}
				}
				result["value"] = valueData
			}
			// Include methods
			if methods := origin.Methods(); len(methods) > 0 {
				serializedMethods := make([]any, len(methods))
				for i, m := range methods {
					methodData := m.Serialize()
					ig.substituteTypes(methodData, typeSubstitutions)
					serializedMethods[i] = methodData
				}
				result["methods"] = serializedMethods
			}
		case *Chan:
			// Include element type with substitution
			if elem := origin.Elem(); elem != nil {
				elemData := serializeTypeOrID(elem)
				if elemMap, ok := elemData.(map[string]any); ok {
					if elemMap["kind"] == "type_parameter" {
						if elemID, ok := elemMap["id"].(string); ok {
							if concrete, exists := typeSubstitutions[elemID]; exists {
								elemData = concrete
							}
						}
					}
				}
				result["elem"] = elemData
			}
			// Include direction
			result["dir"] = origin.Dir()
			// Include methods
			if methods := origin.Methods(); len(methods) > 0 {
				serializedMethods := make([]any, len(methods))
				for i, m := range methods {
					methodData := m.Serialize()
					ig.substituteTypes(methodData, typeSubstitutions)
					serializedMethods[i] = methodData
				}
				result["methods"] = serializedMethods
			}
		case *Basic:
			// Include underlying type
			if underlying := origin.Underlying(); underlying != nil {
				result["underlying"] = serializeTypeOrID(underlying)
			}
			// Include methods
			if methods := origin.Methods(); len(methods) > 0 {
				serializedMethods := make([]any, len(methods))
				for i, m := range methods {
					methodData := m.Serialize()
					ig.substituteTypes(methodData, typeSubstitutions)
					serializedMethods[i] = methodData
				}
				result["methods"] = serializedMethods
			}
		case *Pointer:
			// Include element type with substitution
			if elem := origin.Elem(); elem != nil {
				elemData := serializeTypeOrID(elem)
				if elemMap, ok := elemData.(map[string]any); ok {
					if elemMap["kind"] == "type_parameter" {
						if elemID, ok := elemMap["id"].(string); ok {
							if concrete, exists := typeSubstitutions[elemID]; exists {
								elemData = concrete
							}
						}
					}
				}
				result["elem"] = elemData
			}
			result["depth"] = origin.Depth()
		case *Function:
			// Include parameters and results with substitution
			if params := origin.Parameters(); len(params) > 0 {
				serializedParams := make([]any, len(params))
				for i, p := range params {
					paramData := map[string]any{
						"name": p.Name(),
						"type": serializeTypeOrID(p.Type()),
					}
					ig.substituteTypes(paramData, typeSubstitutions)
					serializedParams[i] = paramData
				}
				result["parameters"] = serializedParams
			}
			if results := origin.Results(); len(results) > 0 {
				serializedResults := make([]any, len(results))
				for i, r := range results {
					resultData := map[string]any{
						"name": r.Name(),
						"type": serializeTypeOrID(r.Type()),
					}
					ig.substituteTypes(resultData, typeSubstitutions)
					serializedResults[i] = resultData
				}
				result["results"] = serializedResults
			}
		}
	}

	return result
}

// substituteTypes recursively replaces type parameters with concrete types in serialized data
func (ig *InstantiatedGeneric) substituteTypes(data any, substitutions map[string]any) {
	switch v := data.(type) {
	case map[string]any:
		// Check if this is a type parameter reference
		if v["kind"] == "type_parameter" {
			if typeID, ok := v["id"].(string); ok {
				if concrete, exists := substitutions[typeID]; exists {
					// Replace the entire type reference with the concrete type
					if concreteMap, ok := concrete.(map[string]any); ok {
						for k, val := range concreteMap {
							v[k] = val
						}
					}
					return
				}
			}
		}
		// Recursively process nested structures
		for _, val := range v {
			ig.substituteTypes(val, substitutions)
		}
	case []any:
		for _, item := range v {
			ig.substituteTypes(item, substitutions)
		}
	}
}

func (ig *InstantiatedGeneric) Load() error {
	var err error
	ig.loadOnce.Do(func() {
		ig.loadComments(false)
		if ig.loader != nil {
			err = ig.loader(ig)
		}
		// Load origin and type args
		if err == nil && ig.origin != nil {
			err = ig.origin.Load()
		}
		if err == nil {
			for _, arg := range ig.typeArgs {
				if arg.Type != nil {
					if loadErr := arg.Type.Load(); loadErr != nil {
						err = loadErr
						return
					}
				}
			}
		}
	})
	return err
}

// // Enum represents an enum type (named type with associated constants)
// type Enum struct {
// 	baseType
// 	underlying Type     // the underlying type (usually int, string, etc.)
// 	values     []*Value // the constant values that belong to this enum
// }

// // NewEnum creates a new enum type
// func NewEnum(id string, name string, underlying Type) *Enum {
// 	return &Enum{
// 		baseType:   newBaseType(id, name, TypeKindEnum),
// 		underlying: underlying,
// 		values:     []*Value{},
// 	}
// }

// func (e *Enum) Underlying() Type {
// 	return e.underlying
// }

// func (e *Enum) Values() []*Value {
// 	return e.values
// }

// func (e *Enum) AddValue(value *Value) {
// 	value.SetParent(e)
// 	e.values = append(e.values, value)
// }

// func (e *Enum) Serialize() any {
// 	if err := e.Load(); err != nil && e.pkg != nil && e.pkg.logger != nil {
// 		e.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", e.id, err))
// 	}
// 	var underlyingSerialized any
// 	if e.underlying != nil {
// 		underlyingSerialized = e.underlying.Serialize()
// 	}

// 	values := make([]*SerializedValue, len(e.values))
// 	for i, v := range e.values {
// 		values[i] = v.Serialize().(*SerializedValue)
// 	}

// 	return &SerializedEnum{
// 		SerializedType: e.serializeBase(),
// 		Underlying:     underlyingSerialized,
// 		Values:         values,
// 	}
// }

// func (e *Enum) Load() error {
// 	var err error
// 	e.loadOnce.Do(func() {
// 		e.loadComments(false)
// 		if e.loader != nil {
// 			err = e.loader(e)
// 		}
// 		// Load underlying type
// 		if err == nil && e.underlying != nil {
// 			err = e.underlying.Load()
// 		}
// 		// Load all values
// 		if err == nil {
// 			for _, v := range e.values {
// 				err = v.Load()
// 				if err != nil {
// 					return
// 				}
// 			}
// 		}
// 	})
// 	return err
// }
