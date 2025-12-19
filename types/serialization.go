package types

// SerializedType contains the common serializable fields for all types
type SerializedType struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Kind     TypeKind  `json:"kind"`
	IsNamed  bool      `json:"named,omitempty"`
	Exported bool      `json:"exported"`
	Distance int       `json:"distance"`
	Package  string    `json:"package,omitempty"`
	Files    []string  `json:"files,omitempty"`
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
		Exported: b.exported,
		Distance: b.distance,
		Package:  pkgPath,
		Files:    b.files,
		Comments: b.comments,
	}
}

// SerializedBasic represents a serialized basic type
type SerializedBasic struct {
	SerializedType
	Underlying interface{} `json:"underlying,omitempty"` // For named basic types
}

// SerializedPointer represents a serialized pointer type
type SerializedPointer struct {
	SerializedType
	Element   any    `json:"element"`
	Depth     int    `json:"depth"`
	Structure string `json:"structure,omitempty"` // Full pointer notation (e.g., "*****string")
}

// SerializedSlice represents a serialized slice/array type
type SerializedSlice struct {
	SerializedType
	Element   any    `json:"element"`
	Length    int64  `json:"length,omitempty"` // -1 for slices, >= 0 for arrays
	Structure string `json:"structure,omitempty"`
}

// SerializedChan represents a serialized channel type
type SerializedChan struct {
	SerializedType
	Element   any              `json:"element"`
	Direction ChannelDirection `json:"direction"`
	Structure string           `json:"structure,omitempty"`
}

// SerializedMap represents a serialized map type
type SerializedMap struct {
	SerializedType
	Key       any    `json:"key"`
	Value     any    `json:"value"`
	Structure string `json:"structure,omitempty"`
}

// SerializedAlias represents a serialized alias type
type SerializedAlias struct {
	SerializedType
	Underlying any `json:"underlying"`
}

// SerializedParameter represents a serialized parameter
type SerializedParameter struct {
	Name       string `json:"name"`
	Type       any    `json:"type"` // Type ID+kind or full type object for complex types
	IsVariadic bool   `json:"is_variadic,omitempty"`
}

// SerializedResult represents a serialized result
type SerializedResult struct {
	Name string `json:"name,omitempty"`
	Type any    `json:"type"` // Type ID+kind or full type object for complex types
}

// SerializedFunction represents a serialized function type
type SerializedFunction struct {
	SerializedType
	Parameters []*SerializedParameter     `json:"parameters,omitempty"`
	Results    []*SerializedResult        `json:"results,omitempty"`
	IsVariadic bool                       `json:"isVariadic,omitempty"`
	Structure  string                     `json:"structure,omitempty"`
	TypeParams []*SerializedTypeParameter `json:"typeParams,omitempty"`
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
	Structure         string                 `json:"structure,omitempty"`
}

// SerializedField represents a serialized field
type SerializedField struct {
	SerializedType
	Type         any    `json:"type"` // Type ID+kind or full type object for complex types
	Tag          string `json:"tag,omitempty"`
	IsEmbedded   bool   `json:"isEmbedded,omitempty"`
	PromotedFrom string `json:"promotedFrom,omitempty"`
	Parent       string `json:"parent"` // ID of parent type
}

// SerializedInterface represents a serialized interface type
type SerializedInterface struct {
	SerializedType
	Embeds     []any                      `json:"embeds,omitempty"`
	Methods    []*SerializedMethod        `json:"methods,omitempty"`
	TypeParams []*SerializedTypeParameter `json:"typeParams,omitempty"`
}

// SerializedStruct represents a serialized struct type
type SerializedStruct struct {
	SerializedType
	Embeds     []any                      `json:"embeds,omitempty"`
	Fields     []*SerializedField         `json:"fields,omitempty"`
	Methods    []*SerializedMethod        `json:"methods,omitempty"`
	TypeParams []*SerializedTypeParameter `json:"typeParams,omitempty"`
}

// SerializedValue represents a serialized constant or variable
type SerializedValue struct {
	SerializedType
	Value     any    `json:"value,omitempty"`
	ValueType any    `json:"valueType"`
	Parent    string `json:"parent,omitempty"` // ID of parent type (for enum values)
}

// SerializedTypeParameter represents a serialized type parameter
type SerializedTypeParameter struct {
	SerializedType
	Index      int `json:"index"`
	Constraint any `json:"constraint,omitempty"` // Reference to constraint type
}

// SerializedUnionTerm represents a single term in a union
type SerializedUnionTerm struct {
	Type          any  `json:"type"`          // Type ID+kind or full type object
	Approximation bool `json:"approximation"` // true for ~T, false for T
}

// SerializedUnion represents a serialized union constraint
type SerializedUnion struct {
	SerializedType
	Terms []SerializedUnionTerm `json:"terms"`
}

// SerializedInstantiatedGeneric represents a serialized instantiated generic
type SerializedInstantiatedGeneric struct {
	SerializedType
	Origin   string `json:"origin"`   // ID of the base generic type
	TypeArgs []any  `json:"typeArgs"` // Type arguments with param names
}

// SerializedEnum represents a serialized enum type
type SerializedEnum struct {
	SerializedType
	Underlying any                `json:"underlying"`
	Values     []*SerializedValue `json:"values,omitempty"`
}
