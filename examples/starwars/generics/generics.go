package generics

// type GenericStruct[T any, U interface{ MethodU() string }] struct {
// 	Field1 T
// 	Field2 U
// }

// type AnotherGeneric[K comparable, V *any] struct {
// 	Key   K
// 	Value V
// }

// // type NestedGeneric[X any] struct {
// // 	Inner GenericStruct[X, AnotherGeneric[int, string]]
// // }

// // Generic interface with a type parameter
// // @schema("GenericInterface")
// // @function("DoSomething")
// type GenericInterface[A any] interface {
// 	DoSomething(param A) A
// }

// type ComplexGenericInterface[M map[string][]int, S struct{ Name string }] interface {
// 	ProcessMap(data M) int
// 	HandleStruct(s S) string
// }

// type MultiGenericInterface[P any, Q any] interface {
// 	Combine(p P, q Q) (P, Q)
// }

// type GenericWithConstraints[T interface {
// 	ConstraintMethod() bool
// }] struct {
// 	GenericField T
// }

type X = string

// // type Y string

//type GenericAlias[T any, K comparable] = map[*K]*T

// // type GenericSlice[T any] []T

// type GenericUnion[T int | string] []T
// type GenericApproximation[T ~string | ~int] []T

// type InterfaceConstraint interface {
// 	Data() (result string)
// }

// type GenericWithStruct[T InterfaceConstraint] []T

// type GenericWithAnonymousConstraint[T interface {
// 	Data() string
// }] []T

// type GenericMap[K comparable, V any] map[K]V

// type GenericChannel[T any] chan T

// type GenericFunction[T any] func(input T) T

// type GenericStructWithMethods[T any] struct {
// 	Value T
// }

// func (g *GenericStructWithMethods[T]) GetValue() T {
// 	return g.Value
// }

// func (g *GenericStructWithMethods[T]) SetValue(val T) {
// 	g.Value = val
// }

// type ConcreteType struct {
// 	Data string
// }

// func (c ConcreteType) ConstraintMethod() bool {
// 	return len(c.Data) > 0
// }

// This is a simple description of a concrete generic type
// @schema("ConcreteGeneric")
// type ConcreteGeneric = GenericWithConstraints[ConcreteType]

// // Alias for a generic struct with methods instantiated with ConcreteType
// // @schema("AliasGeneric")
// type AliasGeneric GenericStructWithMethods[ConcreteType]

type Struct01 struct {
	Field1 string
	Field2 int
}

// func (s Struct01) GetField1() (x *GenericAlias[string, int]) {
// 	return &s.Field1
// }

// func (s *Struct01) SetField1(val GenericAlias[string, int]) {
// 	s.Field1 = val
// }

// type StructAlias = Struct01
