package types

type EnumInfo struct {
	BasicTypeInfo
	Values []*EnumValueInfo `json:"values,omitempty"`
}

type EnumValueInfo struct {
	Name  string `json:"name"`
	Value int64  `json:"value"`
}
