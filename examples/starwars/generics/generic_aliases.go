package generics

// Test file for generic type aliases - all possible cases

// ============================================================================
// Base generic types for testing
// ============================================================================

// GenericStruct is a generic struct with methods
type GenericStruct[T any] struct {
	Value T
}

func (g *GenericStruct[T]) GetValue() T {
	return g.Value
}

func (g *GenericStruct[T]) SetValue(v T) {
	g.Value = v
}

// GenericInterface is a generic interface
type GenericInterface[T any] interface {
	Process(T) T
	GetResult() T
}

// GenericSliceType is a generic type wrapping a slice
type GenericSliceType[T any] []T

func (g GenericSliceType[T]) Len() int {
	return len(g)
}

// GenericMapType is a generic type wrapping a map
type GenericMapType[K comparable, V any] map[K]V

func (g GenericMapType[K, V]) Get(k K) V {
	return g[k]
}

// GenericChanType is a generic type wrapping a channel
type GenericChanType[T any] chan T

// ============================================================================
// CASE 1: Direct alias to instantiated generic struct
// ============================================================================

// DirectStructAlias is a direct alias to an instantiated generic struct
type DirectStructAlias = GenericStruct[string]

// ============================================================================
// CASE 2: Direct alias to instantiated generic interface
// ============================================================================

// DirectInterfaceAlias is a direct alias to an instantiated generic interface
type DirectInterfaceAlias = GenericInterface[int]

// ============================================================================
// CASE 3: Alias to pointer to instantiated generic
// ============================================================================

// PointerToGenericAlias is an alias to a pointer to an instantiated generic
type PointerToGenericAlias = *GenericStruct[int]

// ============================================================================
// CASE 4: Alias to slice of instantiated generic
// ============================================================================

// SliceOfGenericAlias is an alias to a slice of instantiated generics
type SliceOfGenericAlias = []GenericStruct[bool]

// ============================================================================
// CASE 5: Alias to array of instantiated generic
// ============================================================================

// ArrayOfGenericAlias is an alias to an array of instantiated generics
type ArrayOfGenericAlias = [10]GenericStruct[float64]

// ============================================================================
// CASE 6: Alias to map with instantiated generic as value
// ============================================================================

// MapWithGenericValueAlias is an alias to a map with generic value
type MapWithGenericValueAlias = map[string]GenericStruct[int]

// ============================================================================
// CASE 7: Alias to map with instantiated generic as key
// ============================================================================

// MapWithGenericKeyAlias is an alias to a map with generic key (if comparable)
// Note: GenericStruct[string] is not comparable, so we use a simple type
type MapWithGenericKeyAlias = map[string]int

// ============================================================================
// CASE 8: Alias to channel of instantiated generic
// ============================================================================

// ChanOfGenericAlias is an alias to a channel of instantiated generics
type ChanOfGenericAlias = chan GenericStruct[string]

// ============================================================================
// CASE 9: Alias to send-only channel of instantiated generic
// ============================================================================

// SendChanOfGenericAlias is an alias to a send-only channel
type SendChanOfGenericAlias = chan<- GenericStruct[int]

// ============================================================================
// CASE 10: Alias to receive-only channel of instantiated generic
// ============================================================================

// RecvChanOfGenericAlias is an alias to a receive-only channel
type RecvChanOfGenericAlias = <-chan GenericStruct[bool]

// ============================================================================
// CASE 11: Alias to instantiated generic slice type
// ============================================================================

// InstantiatedGenericSliceAlias is an alias to an instantiated generic slice wrapper
type InstantiatedGenericSliceAlias = GenericSliceType[string]

// ============================================================================
// CASE 12: Alias to instantiated generic map type
// ============================================================================

// InstantiatedGenericMapAlias is an alias to an instantiated generic map wrapper
type InstantiatedGenericMapAlias = GenericMapType[string, int]

// ============================================================================
// CASE 13: Alias to instantiated generic channel type
// ============================================================================

// InstantiatedGenericChanAlias is an alias to an instantiated generic channel wrapper
type InstantiatedGenericChanAlias = GenericChanType[float64]

// ============================================================================
// CASE 14: Multiple type parameters
// ============================================================================

// MultiParamGeneric is a generic type with multiple parameters
type MultiParamGeneric[T, U, V any] struct {
	First  T
	Second U
	Third  V
}

func (m *MultiParamGeneric[T, U, V]) GetFirst() T {
	return m.First
}

// MultiParamAlias is an alias to a multi-parameter instantiated generic
type MultiParamAlias = MultiParamGeneric[string, int, bool]

// ============================================================================
// CASE 15: Nested generics
// ============================================================================

// NestedGenericAlias is an alias to nested instantiated generics
type NestedGenericAlias = GenericStruct[GenericStruct[string]]

// ============================================================================
// CASE 16: Pointer to slice of generic
// ============================================================================

// PointerToSliceOfGenericAlias combines pointer and slice
type PointerToSliceOfGenericAlias = *[]GenericStruct[string]

// ============================================================================
// CASE 17: Complex nested structure
// ============================================================================

// ComplexNestedAlias tests deeply nested generic structures
type ComplexNestedAlias = map[string]*[]GenericStruct[GenericInterface[int]]

// ============================================================================
// CASE 18: Generic with constraints
// ============================================================================

// Numeric is a constraint for numeric types
type Numeric interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 |
		~uint | ~uint8 | ~uint16 | ~uint32 | ~uint64 |
		~float32 | ~float64
}

// ConstrainedGeneric is a generic type with a constraint
type ConstrainedGeneric[T Numeric] struct {
	Value T
}

func (c *ConstrainedGeneric[T]) Add(other T) T {
	return c.Value + other
}

// ConstrainedGenericAlias is an alias to a constrained generic
type ConstrainedGenericAlias = ConstrainedGeneric[int]

// ============================================================================
// CASE 19: Generic function type (wrapped in struct)
// ============================================================================

// GenericFuncType wraps a generic function type
type GenericFuncType[T, U any] struct {
	Fn func(T) U
}

// GenericFuncAlias is an alias to an instantiated generic function wrapper
type GenericFuncAlias = GenericFuncType[string, int]

// ============================================================================
// CASE 20: Interface with embedded generic interface
// ============================================================================

// ExtendedInterface embeds a generic interface
type ExtendedInterface[T any] interface {
	GenericInterface[T]
	Extra() string
}

// ExtendedInterfaceAlias is an alias to an instantiated extended interface
type ExtendedInterfaceAlias = ExtendedInterface[float64]
