package scannernew

import "github.com/pablor21/goscanner/typesnew"

// Processor interface for custom type processing during scanning
type Processor interface {
	ScanMode() ScanMode
	SetScanMode(mode ScanMode)
	// ProcessType processes a type and returns whether to continue processing and any error encountered
	// If continueProcessing is false, the scanner will skip further processing of this type
	ProcessType(t typesnew.Type) (continueProcessing bool, err error)
}

// NoOpProcessor is a no-operation processor that always continues processing
type NoOpProcessor struct {
	scanMode ScanMode
}

func NewNoOpProcessor() *NoOpProcessor {
	return &NoOpProcessor{
		scanMode: ScanModeDefault,
	}
}

func (p *NoOpProcessor) ScanMode() ScanMode {
	return p.scanMode
}

func (p *NoOpProcessor) SetScanMode(mode ScanMode) {
	p.scanMode = mode
}

func (p *NoOpProcessor) ProcessType(t typesnew.Type) (bool, error) {
	return true, nil
}
