package typesnew

import "fmt"

// Field represents a struct field
type Field struct {
	baseType
	fieldType    Type // the type of this field
	tag          string
	embedded     bool
	promotedFrom Type // if this field is promoted from an embedded type
	parent       Type // the struct this field belongs to
}

// NewField creates a new field
func NewField(id string, name string, fieldType Type, tag string, embedded bool, parent Type) *Field {
	f := &Field{
		baseType:  newBaseType(id, name, TypeKindField),
		fieldType: fieldType,
		tag:       tag,
		embedded:  embedded,
		parent:    parent,
	}
	// For fields, comment key is "ParentStruct.FieldName"
	if parent != nil {
		f.commentId = parent.Name() + "." + name
	}
	return f
}

func (f *Field) Type() Type {
	return f.fieldType
}

func (f *Field) Tag() string {
	return f.tag
}

func (f *Field) IsEmbedded() bool {
	return f.embedded
}

func (f *Field) PromotedFrom() Type {
	return f.promotedFrom
}

func (f *Field) SetPromotedFrom(t Type) {
	f.promotedFrom = t
}

func (f *Field) Parent() Type {
	return f.parent
}

func (f *Field) Serialize() any {
	if err := f.Load(); err != nil && f.pkg != nil && f.pkg.logger != nil {
		f.pkg.logger.Error(fmt.Sprintf("failed to load field %s: %v", f.id, err))
	}
	promotedFromID := ""
	if f.promotedFrom != nil {
		promotedFromID = f.promotedFrom.Id()
	}

	parentID := ""
	if f.parent != nil {
		parentID = f.parent.Id()
	}

	var fieldTypeSerialized any
	if f.fieldType != nil {
		if f.fieldType.IsNamed() {
			fieldTypeSerialized = serializeTypeRef(f.fieldType)
		} else {
			fieldTypeSerialized = f.fieldType.Serialize()
		}
	}

	return &SerializedField{
		SerializedType: f.serializeBase(),
		Type:           fieldTypeSerialized,
		Tag:            f.tag,
		IsEmbedded:     f.embedded,
		PromotedFrom:   promotedFromID,
		Parent:         parentID,
	}
}

func (f *Field) Load() error {
	var err error
	f.loadOnce.Do(func() {
		// For fields, comment key is "ParentStruct.FieldName"
		if f.parent != nil {
			f.commentId = f.parent.Name() + "." + f.name
		}
		f.loadComments(false)
		if f.loader != nil {
			err = f.loader(f)
		}
		// Don't load field type here - causes deadlock on self-referential types
		// Field types are loaded lazily when accessed
	})
	return err
}

// Method represents a method on a type
type Method struct {
	baseType
	params            []*Parameter
	results           []*Result
	isVariadic        bool
	isPointerReceiver bool
	receiver          Type   // the type this method belongs to
	promotedFrom      Type   // if this method is promoted from an embedded type
	structure         string // full signature string
}

// NewMethod creates a new method
func NewMethod(id string, name string, receiver Type, isPointerReceiver bool) *Method {
	return &Method{
		baseType:          newBaseType(id, name, TypeKindMethod),
		receiver:          receiver,
		isPointerReceiver: isPointerReceiver,
		params:            []*Parameter{},
		results:           []*Result{},
	}
}

func (m *Method) Parameters() []*Parameter {
	return m.params
}

func (m *Method) Results() []*Result {
	return m.results
}

func (m *Method) IsVariadic() bool {
	return m.isVariadic
}

func (m *Method) IsPointerReceiver() bool {
	return m.isPointerReceiver
}

func (m *Method) Receiver() Type {
	return m.receiver
}

func (m *Method) PromotedFrom() Type {
	return m.promotedFrom
}

func (m *Method) SetPromotedFrom(t Type) {
	m.promotedFrom = t
}

func (m *Method) SetStructure(structure string) {
	m.structure = structure
}

func (m *Method) AddParameter(param *Parameter) {
	m.params = append(m.params, param)
	if param.IsVariadic() {
		m.isVariadic = true
	}
}

func (m *Method) AddResult(result *Result) {
	m.results = append(m.results, result)
}

func (m *Method) Serialize() any {
	if err := m.Load(); err != nil && m.pkg != nil && m.pkg.logger != nil {
		m.pkg.logger.Error(fmt.Sprintf("failed to load method %s: %v", m.id, err))
	}
	params := make([]*SerializedParameter, len(m.params))
	for i, p := range m.params {
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

	results := make([]*SerializedResult, len(m.results))
	for i, r := range m.results {
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

	receiverID := ""
	if m.receiver != nil {
		receiverID = m.receiver.Id()
	}

	promotedFromID := ""
	if m.promotedFrom != nil {
		promotedFromID = m.promotedFrom.Id()
	}

	return &SerializedMethod{
		SerializedType:    m.serializeBase(),
		Parameters:        params,
		Results:           results,
		IsVariadic:        m.isVariadic,
		IsPointerReceiver: m.isPointerReceiver,
		Receiver:          receiverID,
		PromotedFrom:      promotedFromID,
		Structure:         m.structure,
	}
}

func (m *Method) Load() error {
	var err error
	m.loadOnce.Do(func() {
		// For methods, comment key is "ReceiverType.MethodName"
		if m.receiver != nil {
			m.commentId = m.receiver.Name() + "." + m.name
		}
		m.loadComments(false)
		if m.loader != nil {
			err = m.loader(m)
		}
		// Don't load parameter/result types - causes deadlock on circular types
	})
	return err
}
