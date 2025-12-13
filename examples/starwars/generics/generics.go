package generics

type GenericStruct[T any, U interface{ MethodU() string }] struct {
	Field1 T
	Field2 U
}

type AnotherGeneric[K comparable, V *any] struct {
	Key   K
	Value V
}

// type NestedGeneric[X any] struct {
// 	Inner GenericStruct[X, AnotherGeneric[int, string]]
// }

// Generic interface with a type parameter
// @schema("GenericInterface")
// @function("DoSomething")
type GenericInterface[A any] interface {
	DoSomething(param A) A
}

type ComplexGenericInterface[M map[string][]int, S struct{ Name string }] interface {
	ProcessMap(data M) int
	HandleStruct(s S) string
}

type MultiGenericInterface[P any, Q any] interface {
	Combine(p P, q Q) (P, Q)
}

type GenericWithConstraints[T interface {
	ConstraintMethod() bool
}] struct {
	GenericField T
}

type GenericAlias[T any] = map[string]T
type GenericSlice[T any] []T

type GenericMap[K comparable, V any] map[K]V

type GenericChannel[T any] chan T

type GenericFunction[T any] func(input T) T

type GenericStructWithMethods[T any] struct {
	Value T
}

func (g *GenericStructWithMethods[T]) GetValue() T {
	return g.Value
}

func (g *GenericStructWithMethods[T]) SetValue(val T) {
	g.Value = val
}

type ConcreteType struct {
	Data string
}

func (c ConcreteType) ConstraintMethod() bool {
	return len(c.Data) > 0
}

type ConcreteGeneric = GenericWithConstraints[ConcreteType]

type AliasGeneric GenericStructWithMethods[ConcreteType]
