package scannernew

import (
	"github.com/pablor21/goscanner/logger"
	"github.com/pablor21/goscanner/typesnew"
)

// ScanningContext holds the configuration and state information for the scanning process
type ScanningContext struct {
	Config       *Config
	Logger       logger.Logger
	ScanMode     ScanMode
	typesCache   map[string]typesnew.Type
	ignoredTypes map[string]struct{}
}

func NewScanningContext(config *Config) *ScanningContext {
	logger.SetupLogger(config.LogLevel)
	return &ScanningContext{
		Config:       config,
		ScanMode:     config.ScanMode,
		Logger:       logger.NewDefaultLogger(),
		typesCache:   make(map[string]typesnew.Type),
		ignoredTypes: make(map[string]struct{}),
	}
}
