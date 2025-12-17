package basic

// import "net/http"

// // type InterfaceA interface {
// // 	MethodA() string
// // 	//MethodB(param int) error
// // 	//MethodC() (int, error)
// // 	//MethodD(param1 string, param2 *float64) *bool
// // 	MethodE(str ...string) *[]string
// // }

// // type unexportedInterfaceB interface {
// // 	UnexportedMethod() int

// // }

// type SelfRef *[]*****SelfRef

// // func (sr SelfRef) Count() int {
// // 	return len(sr)
// // }

// // type StringType string
// // type StringPointer **string
// type IntSlice []int

// func (is *IntSlice) Sum() int {
// 	total := 0
// 	for _, v := range *is {
// 		total += v
// 	}
// 	return total
// }

// // Multiply multiplies each element in the IntSlice by the given factor
// func (is IntSlice) Multiply(factor int) (res IntSlice, err error) {
// 	result := make(IntSlice, len(is))
// 	for i, v := range is {
// 		result[i] = v * factor
// 	}
// 	return result, nil
// }

// func (is IntSlice) VariadicExample(prefix string, factors ...int) IntSlice {
// 	result := make(IntSlice, len(is))
// 	for i, v := range is {
// 		multiplier := 1
// 		for _, f := range factors {
// 			multiplier *= f
// 		}
// 		result[i] = v * multiplier
// 	}
// 	return result
// }

// // This method appends values to the IntSlice
// // and returns a new IntSlice
// // @param values: pointer to IntSlice to append
// // @return: pointer to new IntSlice with appended values
// func (is IntSlice) Append(values *IntSlice) *IntSlice {
// 	if values != nil {
// 		x := append(is, *values...)
// 		return &x
// 	}
// 	return &is
// }

// func (is *IntSlice) UseExternalType(client http.Client) int {
// 	return len(*is)
// }

// // arrays

// type IntArray [5]int

// func (ia IntArray) Sum() int {
// 	total := 0
// 	for _, v := range ia {
// 		total += v
// 	}
// 	return total
// }

// // type PointerSlice [][][]**float64

// // type Str string
// // type StrPtr **Str

// // type IntDeepSlice []IntSlice

// // type StringPointer **string
// // type StringSlice *[]*[][]string
// // type StringSlicePointer *****[]string

// // type NamedSlice []StructA

// // type NamedDeepSlice []NamedSlice

// // type StructA struct {
// // 	// Field1   string
// // 	// Field2   int
// // 	// Field3   string
// // 	// Pointer  *float64
// // 	// SelfRef  *****StructA
// // 	Client http.Client
// // 	// SliceRef *[]************StructA
// // 	SliceRefPtr     *[]string
// // 	SliceRefElemPtr []*NamedSlice
// // }

// // func (sa StructA) MethodA() string {
// // 	return sa.Field1
// // }

// // func (sa *StructA) MethodB(param int) error {
// // 	return nil
// // }

// // func (sa StructA) VariadicMethod(x string, y ...string) (int, error) {
// // 	return 0, nil
// // }

// // type unexportedStructC struct {
// // 	hiddenField string
// // }

// // func (usc unexportedStructC) HiddenMethod() string {
// // 	return usc.hiddenField
// // }

type MyMap map[string]string
type MyDeepMap map[string]MyMap
type MyMapSlice []MyMap
