package basic

// // struct type info
// type Struct01 struct {
// 	// these are field comments
// 	FieldA *string
// 	FieldB *int // this is an inline comment
// 	// Server         http.Server
// 	MyComplesField net.Conn
// }

// // Describe provides a description of Struct01
// func (s Struct01) Describe() string {
// 	return "Struct01 with FieldA: "
// }

// type Struct02 struct {
// 	StructField Struct01
// 	FieldC      float64
// // }

// type MyInterface interface {
// 	// This method does something important
// 	DoSomething(input string) *<-chan *[]*[]int
// 	// Connect() http.ConnState
// } // interface with a method

// // these comments are for MyInterfaceAlis
// type MyInterfaceAlis = **MyInterface

// type MyInterfacePointer interface {
// 	MyInterface
// 	DoSomethingElse(param int) error
// }

// type SourceStruct struct {
// 	EmbeddedFieldA int
// 	EmbeddedFieldB string
// }

// func (s *SourceStruct) IsEmbedded() *bool {
// 	return nil
// }

// type TargetStruct struct {
// 	*SourceStruct
// 	OwnFieldX bool
// }
