package scannernew

import (
	"github.com/pablor21/goscanner/typesnew"
)

// ScanningResult holds the results of a scanning operation
type ScanningResult struct {
	Types    *typesnew.TypesCol[typesnew.Type]     `json:"types,omitempty"`
	Values   *typesnew.TypesCol[*typesnew.Value]   `json:"values,omitempty"`
	Packages *typesnew.TypesCol[*typesnew.Package] `json:"packages,omitempty"`
}

func (s *ScanningResult) Serialize() any {

	return map[string]any{
		"types":    s.Types.Serialize(),
		"values":   s.Values.Serialize(),
		"packages": s.Packages.Serialize(),
	}
}

// func (s *ScanningResult) MarshalJSON() ([]byte, error) {
// 	s.Serialize()
// 	return json.Marshal(struct {
// 		Types    *typesnew.TypesCol[typesnew.Type]     `json:"types,omitempty"`
// 		Values   *typesnew.TypesCol[*typesnew.Value]   `json:"values,omitempty"`
// 		Packages *typesnew.TypesCol[*typesnew.Package] `json:"packages,omitempty"`
// 	}{
// 		Types:    s.Types,
// 		Values:   s.Values,
// 		Packages: s.Packages,
// 	})
// }

// NewScanningResult creates a new scanning result
func NewScanningResult() *ScanningResult {
	return &ScanningResult{
		Types:    typesnew.NewTypesCol[typesnew.Type](),
		Values:   typesnew.NewTypesCol[*typesnew.Value](),
		Packages: typesnew.NewTypesCol[*typesnew.Package](),
	}
}
