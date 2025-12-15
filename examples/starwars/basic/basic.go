package basic

import "net/http"

// type InterfaceA interface {
// 	MethodA() string
// 	//MethodB(param int) error
// 	//MethodC() (int, error)
// 	//MethodD(param1 string, param2 *float64) *bool
// 	MethodE(str ...string) *[]string
// }

// type unexportedInterfaceB interface {
// 	UnexportedMethod() int
// }

type NamedSlice []StructA // doc

type NamedDeepSlice []NamedSlice

type StructA struct {
	// Field1   string
	// Field2   int
	// Field3   string
	// Pointer  *float64
	// SelfRef  *****StructA
	Client http.Client
	// SliceRef *[]************StructA
	SliceRefPtr     *[]string
	SliceRefElemPtr []*NamedSlice
}

// func (sa StructA) MethodA() string {
// 	return sa.Field1
// }

// func (sa *StructA) MethodB(param int) error {
// 	return nil
// }

// func (sa StructA) VariadicMethod(x string, y ...string) (int, error) {
// 	return 0, nil
// }

// type unexportedStructC struct {
// 	hiddenField string
// }

// func (usc unexportedStructC) HiddenMethod() string {
// 	return usc.hiddenField
// }
