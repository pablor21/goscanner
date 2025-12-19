package scanner

import (
	"go/types"

	"github.com/pablor21/goscanner/logger"
)

// ScanningContext holds the configuration and state information for the scanning process
type ScanningContext struct {
	Config       *Config
	Logger       logger.Logger
	ScanMode     ScanMode
	typesCache   map[string]types.Type
	ignoredTypes map[string]struct{}
}

func NewScanningContext(config *Config) *ScanningContext {
	logger.SetupLogger(config.LogLevel)
	return &ScanningContext{
		Config:       config,
		ScanMode:     config.ScanMode,
		Logger:       logger.NewDefaultLogger(),
		typesCache:   make(map[string]types.Type),
		ignoredTypes: make(map[string]struct{}),
	}
}
