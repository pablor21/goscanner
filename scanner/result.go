package scanner

type ScanningResult struct {
	Types    TypeCollection    `json:"types,omitempty"`
	Values   ValueCollection   `json:"values,omitempty"`
	Packages PackageCollection `json:"packages,omitempty"`
}
