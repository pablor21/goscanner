package scanner

import (
	gcr "github.com/pablor21/goscanner/types"
)

type Processor interface {
	ScanMode() ScanMode
	SetScanMode(mode ScanMode)
	// ProcessType processes a type and returns whether to continue processing and any error encountered
	// This is used for custom processing of types during scanning
	// If continueProcessing is false, the scanner will skip further processing of this type
	// and move to the next type in the queue, if there is no processors left to process the type
	// it will be stop the processing of the type entirely
	ProcessType(t gcr.Type) (continueProcessing bool, err error)
}

type NoOpProcessor struct{}

func (p *NoOpProcessor) ScanMode() ScanMode {
	return ScanModeDefault
}

func (p *NoOpProcessor) SetScanMode(mode ScanMode) {
	// No-op
}

func (p *NoOpProcessor) ProcessType(t gcr.Type) (bool, error) {
	return true, nil
}
