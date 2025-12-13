// nolint
package outofscope

type OtherStruct struct {
	Field     string
	Recursion *OtherStruct
}

func (os OtherStruct) Method() string {
	return os.Field
}

func (os *OtherStruct) PointerMethod() string {
	return os.Field
}

func (os OtherStruct) unexportedMethod() string {
	return "unexported"
}

func (os OtherStruct) MixedMethod(param1 string, param2 *int) (string, error) {
	return param1, nil
}
