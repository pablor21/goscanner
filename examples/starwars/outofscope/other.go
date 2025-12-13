package outofscope

type OtherStruct struct {
	Field     string
	Recursion *OtherStruct
}
