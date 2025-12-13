package functions

import "fmt"

// RegularFunction is a simple function that takes two parameters and returns a boolean
// @info("This is a regular function", returns="boolean indicating if b equals the rune of a")
func RegularFunction(a int, b string) bool {
	return b == string(rune(a))
}

func AnotherFunction(x float64) error {
	if x < 0 {
		return nil
	}
	return nil
}

func VariadicFunction(prefix string, values ...int) string {
	result := prefix
	for _, v := range values {
		result += string(rune(v))
	}
	return result
}

func FunctionWithMultipleReturns(a int, b int) (int, int, error) {
	if b == 0 {
		return 0, 0, nil
	}
	return a / b, a % b, nil
}

func FunctionWithNoParams() string {
	return "No parameters here!"
}

func FunctionWithNoReturns(a string) {
	println("Received:", a)
}

func GenericFunction[T any](input T) T {
	return input
}

func GenericFunctionWithConstraints[T interface {
	~int | ~float64
}](input T) T {
	return input
}

func GenericFunctionWithMultipleTypeParams[T any, U any](input1 T, input2 U) (T, U) {
	return input1, input2
}

func GenericVariadicFunction[T any](prefix string, values ...T) string {
	result := prefix
	for _, v := range values {
		result += fmt.Sprintf("%v", v)
	}
	return result
}

func GenericFunctionReturningGeneric[T any](input T) *[]*T {
	return &[]*T{&input}
}
func GenericFunctionWithNoParams[T any]() T {
	var zero T
	return zero
}

func GenericFunctionWithNoReturns[T any](input T) {
	println("Received generic input")
}

func GenericFunctionWithConstraintsAndMultipleReturns[T interface {
	~int | ~float64
}](input T) (T, T, error) {
	if input == 0 {
		return input, input, nil
	}
	return input, input, nil
}
