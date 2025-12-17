// Package basic provides basic examples for testing
package basic

// This is a very long
// comment for the basic package
// spanning multiple lines
// to test comment extraction.
// It should be associated with the package itself.

// X is a basic named type

// // This is the comment for X
// type X string

// // String returns the string representation of X
// func (x X) String() string {
// 	return string(x)
// }

// // PointerReceiverMethod is a method with a pointer receiver
// func (x *X) PointerReceiverMethod() string {
// 	return string(*x)
// }

// // func Server() http.Server {
// // 	return http.Server{
// // 		Addr: net.JoinHostPort("localhost", "8080"),
// // 	}
// // }

// func SimpleFunction(a int, b string) (string, error) {
// 	return b, nil
// }

// // IntSliceAppend appends an integer to a slice of integers and returns the new slice.
// func IntSliceAppend(values *string, variadic ...int) (ret *X, err error, str string) {
// 	x := X(*values + " appended")
// 	return &x, nil, "done"
// }

// // Other struct type
// type MyStruct struct {
// 	// Basic fields
// 	FieldA int
// 	// Another field
// 	FieldB string // comment for FieldB
// } // end of MyStruct

// // FnType is a function type that takes an int and a string
// // and returns a string and an error
// type FnType func(a int, b string) (string, error)

// func (f *FnType) Invoke(a int, b string) (string, error) {
// 	return (*f)(a, b)
// }

// // // FnPointer is a pointer to a function type
// // // e.g., type FnPointer = *FnType
// // // these are really cool comments!!!!!
// // type FnPointer *FnType

// // func ExampleFunction(a int, b string) (string, error) {
// // 	return b, nil
// // }

// // func ExampleFunctionPointer(a *int, b *string) (*string, error) {
// // 	return b, nil
// // }
