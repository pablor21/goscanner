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
	return &Field{
		baseType:  newBaseType(id, name, TypeKindField),
		fieldType: fieldType,
		tag:       tag,
		embedded:  embedded,
		parent:    parent,
	}
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
		fieldTypeSerialized = f.fieldType.Serialize()
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
		f.loadComments(false)
		if f.loader != nil {
			err = f.loader(f)
		}
		// Load the field type
		if err == nil && f.fieldType != nil {
			err = f.fieldType.Load()
		}
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
	receiver          Type // the type this method belongs to
	promotedFrom      Type // if this method is promoted from an embedded type
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
			paramTypeSerialized = p.paramType.Serialize()
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
			resultTypeSerialized = r.resultType.Serialize()
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
	}
}

func (m *Method) Load() error {
	var err error
	m.loadOnce.Do(func() {
		m.loadComments(false)
		if m.loader != nil {
			err = m.loader(m)
		}
		// Load parameter types
		if err == nil {
			for _, p := range m.params {
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
			for _, r := range m.results {
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
