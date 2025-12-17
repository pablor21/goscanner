package typesnew

// SerializedType contains the common serializable fields for all types
type SerializedType struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Kind     TypeKind  `json:"kind"`
	IsNamed  bool      `json:"isNamed"`
	Package  string    `json:"package,omitempty"`
	Comments []Comment `json:"comments,omitempty"`
}

// serializeBase creates a SerializedType from baseType
func (b *baseType) serializeBase() SerializedType {
	pkgPath := ""
	if b.pkg != nil {
		pkgPath = b.pkg.Path()
	}
	return SerializedType{
		ID:       b.id,
		Name:     b.name,
		Kind:     b.kind,
		IsNamed:  b.obj != nil,
		Package:  pkgPath,
		Comments: b.comments,
	}
}

// SerializedBasic represents a serialized basic type
type SerializedBasic struct {
	SerializedType
}

// SerializedPointer represents a serialized pointer type
type SerializedPointer struct {
	SerializedType
	Element any `json:"element"`
	Depth   int `json:"depth"`
}

// SerializedSlice represents a serialized slice/array type
type SerializedSlice struct {
	SerializedType
	Element any   `json:"element"`
	Length  int64 `json:"length,omitempty"` // -1 for slices, >= 0 for arrays
}

// SerializedChan represents a serialized channel type
type SerializedChan struct {
	SerializedType
	Element   any              `json:"element"`
	Direction ChannelDirection `json:"direction"`
}

// SerializedMap represents a serialized map type
type SerializedMap struct {
	SerializedType
	Key   any `json:"key"`
	Value any `json:"value"`
}

// SerializedAlias represents a serialized alias type
type SerializedAlias struct {
	SerializedType
	Underlying any `json:"underlying"`
}

// SerializedParameter represents a serialized parameter
type SerializedParameter struct {
	Name       string `json:"name"`
	Type       any    `json:"type"`
	IsVariadic bool   `json:"is_variadic,omitempty"`
}

// SerializedResult represents a serialized result
type SerializedResult struct {
	Name string `json:"name,omitempty"`
	Type any    `json:"type"`
}

// SerializedFunction represents a serialized function type
type SerializedFunction struct {
	SerializedType
	Parameters []*SerializedParameter `json:"parameters,omitempty"`
	Results    []*SerializedResult    `json:"results,omitempty"`
	IsVariadic bool                   `json:"isVariadic,omitempty"`
}

// SerializedMethod represents a serialized method
type SerializedMethod struct {
	SerializedType
	Parameters        []*SerializedParameter `json:"parameters,omitempty"`
	Results           []*SerializedResult    `json:"results,omitempty"`
	IsVariadic        bool                   `json:"isVariadic,omitempty"`
	IsPointerReceiver bool                   `json:"isPointerReceiver"`
	Receiver          string                 `json:"receiver"` // ID of receiver type
	PromotedFrom      string                 `json:"promotedFrom,omitempty"`
}

// SerializedField represents a serialized field
type SerializedField struct {
	SerializedType
	Type         any    `json:"type"`
	Tag          string `json:"tag,omitempty"`
	IsEmbedded   bool   `json:"isEmbedded,omitempty"`
	PromotedFrom string `json:"promotedFrom,omitempty"`
	Parent       string `json:"parent"` // ID of parent type
}

// SerializedInterface represents a serialized interface type
type SerializedInterface struct {
	SerializedType
	Methods []*SerializedMethod `json:"methods,omitempty"`
}

// SerializedStruct represents a serialized struct type
type SerializedStruct struct {
	SerializedType
	Fields  []*SerializedField  `json:"fields,omitempty"`
	Methods []*SerializedMethod `json:"methods,omitempty"`
}

// SerializedValue represents a serialized constant or variable
type SerializedValue struct {
	SerializedType
	Value     any    `json:"value,omitempty"`
	ValueType any    `json:"valueType"`
	Parent    string `json:"parent,omitempty"` // ID of parent type (for enum values)
}

// SerializedEnum represents a serialized enum type
type SerializedEnum struct {
	SerializedType
	Underlying any                `json:"underlying"`
	Values     []*SerializedValue `json:"values,omitempty"`
}
