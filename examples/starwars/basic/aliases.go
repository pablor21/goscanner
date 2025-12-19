package basic

import "net/http"

// // Example type aliases
// type StringAlias = string

// // Example alias for pointer to int
// type IntAlias = *int

// type NamedAlias string
// Generic interface with a type parameter
// @schema("GenericInterface")
// @function("DoSomething")
// type GenericInterface[A any, M map[string][]int, S struct{ Name string }] interface {
// 	// This method takes a parameter of type A and returns a value of type A
// 	DoSomething(param A) A // Returns the same type as the parameter
// }

// type ComplexGenericInterface[M map[string][]int, S struct{ Name string }] interface {
// 	ProcessMap(data M) int
// 	HandleStruct(s S) string
// }

// type MultiGenericInterface[P any, Q any] interface {
// 	Combine(p P, q Q) (P, Q)
// }

// type MultipleGenericStruct[X any, Y ~int | float32] struct {
// 	First  X
// 	Second Y
// }

// func (mgs *MultipleGenericStruct[X, Y]) GetFirst() X {
// 	return mgs.First
// }

// func (mgs *MultipleGenericStruct[X, Y]) GetSecond() Y {
// 	return mgs.Second
// }

// type MultipleGenericInstantiation MultipleGenericStruct[string, float32]

// type Struct01 struct {
// 	Field1 string
// }

// func (s *Struct01) Method1(param string) string {
// 	return param + s.Field1
// }

// type Struct02 struct {
// 	Struct01
// }

// type Interface01[T any] interface {
// 	MethodA() (name T, err error)
// }
// type Interface02 interface {
// 	Interface01[string]
// }

// type UseGenericInstantiation struct {
// 	Field MultipleGenericStruct[string, int]
// }

// type EmbeddedGenericStruct struct {
// 	MultipleGenericStruct[string, int]
// }

type GenericWithConstraints[T interface {
	ConstraintMethod() http.ServeMux
}] struct {
	GenericField T
}

func (gwc *GenericWithConstraints[T]) CheckConstraint(x T) http.ServeMux {
	return gwc.GenericField.ConstraintMethod()
}

type ConstraintImpl struct {
}

func (ci ConstraintImpl) ConstraintMethod() http.ServeMux {

	return http.ServeMux{}
}

type AliasForGenericWithConstraints GenericWithConstraints[ConstraintImpl]
