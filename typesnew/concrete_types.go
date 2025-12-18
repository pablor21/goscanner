// Package types defines concrete implementations of various Go types
// such as Basic, Pointer, Slice, Map, Alias, Function, Interface, Struct, Enum, and Value.
// Each type implements the Type interface and provides methods for serialization
// and lazy loading of additional details.
// Usually these types correspond to go/types types but are designed for easier
// serialization and documentation extraction.
package typesnew

import (
	"fmt"
	"go/doc"
)

// serializeTypeRef serializes a type as a reference (basic info only)
func serializeTypeRef(t Type) any {
	return &SerializedType{
		ID:      t.Id(),
		Name:    t.Name(),
		Kind:    t.Kind(),
		IsNamed: t.IsNamed(),
		Package: getPackagePath(t),
	}
}

func getPackagePath(t Type) string {
	if t.Package() != nil {
		return t.Package().Path()
	}
	return ""
}

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
	if err := b.Load(); err != nil && b.pkg != nil && b.pkg.logger != nil {
		b.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", b.id, err))
	}

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

func (p *Pointer) Element() Type {
	return p.elem
}

func (p *Pointer) Depth() int {
	return p.depth
}

func (p *Pointer) Serialize() any {
	if err := p.Load(); err != nil && p.pkg != nil && p.pkg.logger != nil {
		p.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", p.id, err))
	}
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

func (s *Slice) Element() Type {
	return s.elem
}

func (s *Slice) Len() int64 {
	return s.len
}

func (s *Slice) IsArray() bool {
	return s.len >= 0
}

func (s *Slice) Serialize() any {
	if err := s.Load(); err != nil && s.pkg != nil && s.pkg.logger != nil {
		s.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", s.id, err))
	}

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

func (c *Chan) Element() Type {
	return c.elem
}

func (c *Chan) Direction() ChannelDirection {
	return c.dir
}

func (c *Chan) Serialize() any {
	if err := c.Load(); err != nil && c.pkg != nil && c.pkg.logger != nil {
		c.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", c.id, err))
	}
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
	if err := m.Load(); err != nil && m.pkg != nil && m.pkg.logger != nil {
		m.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", m.id, err))
	}

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
	if err := a.Load(); err != nil && a.pkg != nil && a.pkg.logger != nil {
		a.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", a.id, err))
	}
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
	docFunc    *doc.Func // for package-level functions
	structure  string    // full signature string
}

// NewFunction creates a new function type
func NewFunction(id string, name string) *Function {
	return &Function{
		baseType: newBaseType(id, name, TypeKindFunction),
		params:   []*Parameter{},
		results:  []*Result{},
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

func (f *Function) Serialize() any {
	if err := f.Load(); err != nil && f.pkg != nil && f.pkg.logger != nil {
		f.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", f.id, err))
	}
	params := make([]*SerializedParameter, len(f.params))
	for i, p := range f.params {
		var paramTypeSerialized any
		if p.paramType != nil {
			if p.paramType.IsNamed() {
				paramTypeSerialized = serializeTypeRef(p.paramType)
			} else {
				paramTypeSerialized = p.paramType.Serialize()
			}
		}
		params[i] = &SerializedParameter{
			Name:       p.name,
			Type:       paramTypeSerialized,
			IsVariadic: p.isVariadic,
		}
	}

	results := make([]*SerializedResult, len(f.results))
	for i, r := range f.results {
		var resultTypeSerialized any
		if r.resultType != nil {
			if r.resultType.IsNamed() {
				resultTypeSerialized = serializeTypeRef(r.resultType)
			} else {
				resultTypeSerialized = r.resultType.Serialize()
			}
		}
		results[i] = &SerializedResult{
			Name: r.name,
			Type: resultTypeSerialized,
		}
	}

	return &SerializedFunction{
		SerializedType: f.serializeBase(),
		Parameters:     params,
		Results:        results,
		IsVariadic:     f.isVariadic,
		Structure:      f.structure,
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
}

// NewInterface creates a new interface type
func NewInterface(id string, name string) *Interface {
	return &Interface{
		baseType: newBaseType(id, name, TypeKindInterface),
	}
}

func (i *Interface) Serialize() any {
	if err := i.Load(); err != nil && i.pkg != nil && i.pkg.logger != nil {
		i.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", i.id, err))
	}
	methods := make([]*SerializedMethod, len(i.methods))
	for idx, m := range i.methods {
		methods[idx] = m.Serialize().(*SerializedMethod)
	}

	return &SerializedInterface{
		SerializedType: i.serializeBase(),
		Methods:        methods,
	}
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
	fields []*Field
}

// NewStruct creates a new struct type
func NewStruct(id string, name string) *Struct {
	return &Struct{
		baseType: newBaseType(id, name, TypeKindStruct),
		fields:   []*Field{},
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

func (s *Struct) Serialize() any {
	if err := s.Load(); err != nil && s.pkg != nil && s.pkg.logger != nil {
		s.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", s.id, err))
	}
	fields := make([]*SerializedField, len(s.fields))
	for i, f := range s.fields {
		fields[i] = f.Serialize().(*SerializedField)
	}

	methods := make([]*SerializedMethod, len(s.methods))
	for i, m := range s.methods {
		methods[i] = m.Serialize().(*SerializedMethod)
	}

	return &SerializedStruct{
		SerializedType: s.serializeBase(),
		Fields:         fields,
		Methods:        methods,
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
		valueTypeSerialized = v.valueType.Serialize()
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

// Enum represents an enum type (named type with associated constants)
type Enum struct {
	baseType
	underlying Type     // the underlying type (usually int, string, etc.)
	values     []*Value // the constant values that belong to this enum
}

// NewEnum creates a new enum type
func NewEnum(id string, name string, underlying Type) *Enum {
	return &Enum{
		baseType:   newBaseType(id, name, TypeKindEnum),
		underlying: underlying,
		values:     []*Value{},
	}
}

func (e *Enum) Underlying() Type {
	return e.underlying
}

func (e *Enum) Values() []*Value {
	return e.values
}

func (e *Enum) AddValue(value *Value) {
	value.SetParent(e)
	e.values = append(e.values, value)
}

func (e *Enum) Serialize() any {
	if err := e.Load(); err != nil && e.pkg != nil && e.pkg.logger != nil {
		e.pkg.logger.Error(fmt.Sprintf("failed to load type %s: %v", e.id, err))
	}
	var underlyingSerialized any
	if e.underlying != nil {
		underlyingSerialized = e.underlying.Serialize()
	}

	values := make([]*SerializedValue, len(e.values))
	for i, v := range e.values {
		values[i] = v.Serialize().(*SerializedValue)
	}

	return &SerializedEnum{
		SerializedType: e.serializeBase(),
		Underlying:     underlyingSerialized,
		Values:         values,
	}
}

func (e *Enum) Load() error {
	var err error
	e.loadOnce.Do(func() {
		e.loadComments(false)
		if e.loader != nil {
			err = e.loader(e)
		}
		// Load underlying type
		if err == nil && e.underlying != nil {
			err = e.underlying.Load()
		}
		// Load all values
		if err == nil {
			for _, v := range e.values {
				err = v.Load()
				if err != nil {
					return
				}
			}
		}
	})
	return err
}
