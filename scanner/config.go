package scanner

import (
	"embed"
	"encoding/json"
	"errors"
	"strings"

	"github.com/pablor21/goscanner/logger"
)

type ScanMode uint8

const (
	ScanModeNone      ScanMode = 0
	ScanModeTypes     ScanMode = 1 << iota // Basic type information
	ScanModeMethods                        // Include methods
	ScanModeFields                         // Include struct fields
	ScanModeFunctions                      // Include standalone functions
	ScanModeDocs                           // Include documentation
	ScanModeComments                       // Parse and extract comments

	// Predefined combinations
	ScanModeBasic   = ScanModeTypes | ScanModeDocs
	ScanModeDefault = ScanModeTypes | ScanModeMethods | ScanModeDocs | ScanModeComments
	ScanModeFull    = ScanModeTypes | ScanModeMethods | ScanModeFields | ScanModeFunctions | ScanModeDocs | ScanModeComments
)

func (m ScanMode) String() string {
	return string(m)
}

func (m ScanMode) Has(mode ScanMode) bool {
	return m&mode == mode
}

// parse a string separated by commas into a ScanMode
// e.g. "types,methods,fields" -> ScanModeTypes | ScanModeMethods | ScanModeFields
func (m ScanMode) FromString(str string) ScanMode {
	s := strings.Split(strings.ToLower(str), ",")
	if len(s) == 0 {
		return ScanModeDefault
	}
	for _, v := range s {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch v {
		case "types":
			m |= ScanModeTypes
		case "methods":
			m |= ScanModeMethods
		case "fields":
			m |= ScanModeFields
		case "functions":
			m |= ScanModeFunctions
		case "docs", "annotations", "comments":
			m |= ScanModeDocs
		case "full":
			m = ScanModeFull
		case "default":
			m = ScanModeDefault
		default:
			panic("unknown scan mode " + v)
		}
	}
	return m
}

func (m *ScanMode) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	*m = m.FromString(str)
	return nil
}

func (m ScanMode) MarshalJSON() ([]byte, error) {
	var parts []string
	if m.Has(ScanModeTypes) {
		parts = append(parts, "types")
	}
	if m.Has(ScanModeMethods) {
		parts = append(parts, "methods")
	}
	if m.Has(ScanModeFields) {
		parts = append(parts, "fields")
	}
	if m.Has(ScanModeFunctions) {
		parts = append(parts, "functions")
	}
	if m.Has(ScanModeDocs) {
		parts = append(parts, "docs")
	}
	str := strings.Join(parts, ",")
	return []byte(`"` + str + `"`), nil
}

type VisibilityLevel uint8

const (
	VisibilityLevelExported VisibilityLevel = 1 << iota
	VisibilityLevelUnexported
	VisibilityLevelAll = VisibilityLevelExported | VisibilityLevelUnexported
)

func (v VisibilityLevel) String() string {
	return string(v)
}

func (v VisibilityLevel) Has(level VisibilityLevel) bool {
	return v&level == level
}

func (v VisibilityLevel) FromString(str string) VisibilityLevel {
	s := strings.Split(strings.ToLower(str), ",")
	if len(s) == 0 {
		return VisibilityLevelExported
	}
	var level VisibilityLevel
	for _, v := range s {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		switch v {
		case "exported":
			level |= VisibilityLevelExported
		case "unexported":
			level |= VisibilityLevelUnexported
		case "all":
			level = VisibilityLevelAll
		default:
			panic("unknown visibility level " + v)
		}
	}
	return level
}

func (v *VisibilityLevel) UnmarshalJSON(data []byte) error {
	str := strings.Trim(string(data), `"`)
	*v = v.FromString(str)
	return nil
}

func (v VisibilityLevel) MarshalJSON() ([]byte, error) {
	var parts []string
	if v.Has(VisibilityLevelExported) {
		parts = append(parts, "exported")
	}
	if v.Has(VisibilityLevelUnexported) {
		parts = append(parts, "unexported")
	}
	str := strings.Join(parts, ",")
	return []byte(`"` + str + `"`), nil
}

//go:embed config.json
var defaultConfigFs embed.FS

type Config struct {
	Packages           []string        `json:"packages" yaml:"packages"`
	ScanMode           ScanMode        `json:"scan_mode" yaml:"scan_mode"`
	FieldVisibility    VisibilityLevel `json:"field_visibility" yaml:"field_visibility"`
	MethodVisibility   VisibilityLevel `json:"method_visibility" yaml:"method_visibility"`
	FunctionVisibility VisibilityLevel `json:"function_visibility" yaml:"function_visibility"`
	TypeVisibility     VisibilityLevel `json:"type_visibility" yaml:"type_visibility"`
	EnumVisibility     VisibilityLevel `json:"enum_visibility" yaml:"enum_visibility"`
	LogLevel           logger.LogLevel `json:"log_level" yaml:"log_level"`
}

func NewDefaultConfig() *Config {
	data, err := defaultConfigFs.ReadFile("config.json")
	if err != nil {
		panic("failed to read default config: " + err.Error())
	}
	cfg := &Config{}
	err = cfg.fromJSON(data)
	if err != nil {
		panic("failed to parse default config: " + err.Error())
	}
	return cfg
}

func NewConfigFromBytes(data []byte, format string) (*Config, error) {
	cfg := &Config{}
	var err error
	switch format {
	case "json":
		err = cfg.fromJSON(data)
	default:
		err = errors.New("unsupported config format: " + format)
	}
	if err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) fromJSON(data []byte) error {
	// delete all comments from data
	str := string(data)
	lines := strings.Split(str, "\n")
	var uncommentedLines []string
	for _, line := range lines {
		if strings.Contains(line, "//") {
			line = strings.Split(line, "//")[0]
		}
		uncommentedLines = append(uncommentedLines, line)
	}
	str = strings.Join(uncommentedLines, "\n")
	data = []byte(str)

	return json.Unmarshal(data, c)
}
