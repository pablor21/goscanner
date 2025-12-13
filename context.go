package goscanner

import "github.com/pablor21/goscanner/logger"

// ScanningContext holds the configuration and state information for the scanning process.
// @myAnnotation("ScanningContext")
type ScanningContext struct {
	Config       *Config
	Logger       logger.Logger
	ScanMode     ScanMode
	typesCache   map[string]TypeInfo
	ignoredTypes map[string]struct{}
}

func NewScanningContext(config *Config) *ScanningContext {
	logger.SetupLogger(config.LogLevel)
	return &ScanningContext{
		Config:       config,
		ScanMode:     config.ScanMode,
		Logger:       logger.NewDefaultLogger(),
		typesCache:   make(map[string]TypeInfo),
		ignoredTypes: make(map[string]struct{}),
	}
}
