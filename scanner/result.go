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

// EnsureFullyLoaded materializes all lazy-loaded type details
// This must be called before caching to ensure all type data is available
func (s *ScanningResult) EnsureFullyLoaded() error {
	if s == nil {
		return nil
	}

	// Load all types
	for _, id := range s.Types.Keys() {
		if t, exists := s.Types.Get(id); exists {
			if loadable, ok := t.(gstypes.Loadable); ok {
				if err := loadable.Load(); err != nil {
					return err
				}
			}
		}
	}

	// Load all values
	for _, id := range s.Values.Keys() {
		if v, exists := s.Values.Get(id); exists {
			if err := v.Load(); err != nil {
				return err
			}
		}
	}

	// Packages don't have a Load method, but we ensure their files are populated
	// (they're loaded during scanning)

	return nil
}

// ToCache serializes the result to a gzip-compressed JSON cache file
func (s *ScanningResult) ToCache(filename string) error {
	return WriteCache(filename, s)
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
