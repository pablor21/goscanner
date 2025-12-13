package goscanner

import (
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
		default:
			panic("unknown scan mode " + v)
		}
	}
	return m
}

type Config struct {
	Packages []string        `json:"packages" yaml:"packages"`
	ScanMode ScanMode        `json:"scan_mode" yaml:"scan_mode"`
	LogLevel logger.LogLevel `json:"log_level" yaml:"log_level"`
}

func NewDefaultConfig() *Config {
	return &Config{
		// Packages: []string{"golang.org/x/sync/**", "net/**"},
		Packages: []string{"../examples/starwars/functions/**", "!../main"},
		ScanMode: ScanModeFull,
		LogLevel: logger.LogLevelInfo,
	}
}
