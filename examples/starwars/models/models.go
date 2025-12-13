package models

import "github.com/pablor21/goscanner/examples/starwars/outofscope"

type InterfaceExample interface {
	MyMethod01() int
}

type EmbeddedInterface interface {
	InterfaceExample
	MyMethod02() string
}

// EmbeddedStruct is an example of a struct with embedded fields
// that reference out-of-scope types
// @schema("EmbeddedStruct")
type EmbeddedStruct struct {
	// @field("id")
	ID int `json:"id" schema:"id"`
}

func (e EmbeddedStruct) GetID() int {
	return e.ID
}

// Human represents a human character with recursive family relationships
// @schema("Human")
type Human struct {
	EmbeddedStruct
	Name *string `json:"name" schema:"name"`
	// @field("family")
	Family         []Human                                   `json:"family" schema:"family"`
	Cannel         chan *[]outofscope.OtherStruct            `json:"cannel" schema:"cannel"`
	DeppArray      map[string][][][][]outofscope.OtherStruct `json:"depp_array" schema:"depp_array"`
	ComplexType    chan *map[string]*[][]*outofscope.OtherStruct
	AnonymousField struct {
		Field1 string
		Field2 int
	} `json:"anonymous" schema:"anonymous"`

	PointerToAnonymousField *struct {
		FieldA float64
		FieldB bool
		FieldC []string
		FieldD map[int]outofscope.OtherStruct
	} `json:"pointer_to_anonymous" schema:"pointer_to_anonymous"`
}
