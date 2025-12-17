package scanner

type ScanningResult struct {
	Types    TypeCollection    `json:"types"`
	Values   ValueCollection   `json:"values"`
	Packages PackageCollection `json:"packages"`
}
