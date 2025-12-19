package scanner

import (
	gstypes "github.com/pablor21/goscanner/types"
)

// ScanningResult holds the results of a scanning operation
type ScanningResult struct {
	Types    *gstypes.TypesCol[gstypes.Type]     `json:"types,omitempty"`
	Values   *gstypes.TypesCol[*gstypes.Value]   `json:"values,omitempty"`
	Packages *gstypes.TypesCol[*gstypes.Package] `json:"packages,omitempty"`
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
// 		Types    *types.TypesCol[types.Type]     `json:"types,omitempty"`
// 		Values   *types.TypesCol[*types.Value]   `json:"values,omitempty"`
// 		Packages *types.TypesCol[*types.Package] `json:"packages,omitempty"`
// 	}{
// 		Types:    s.Types,
// 		Values:   s.Values,
// 		Packages: s.Packages,
// 	})
// }

// NewScanningResult creates a new scanning result
func NewScanningResult() *ScanningResult {
	return &ScanningResult{
		Types:    gstypes.NewTypesCol[gstypes.Type](),
		Values:   gstypes.NewTypesCol[*gstypes.Value](),
		Packages: gstypes.NewTypesCol[*gstypes.Package](),
	}
}
