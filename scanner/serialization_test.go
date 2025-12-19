package scanner

import (
	"encoding/json"
	"runtime"
	"testing"

	gstypes "github.com/pablor21/goscanner/types"
)

// helper to build a minimal config targeting local example packages
func testConfig() *Config {
	cfg := NewDefaultConfig()
	cfg.Packages = []string{
		"./examples/starwars/basic",
		"./examples/starwars/functions",
	}
	cfg.LogLevel = "error"
	if cfg.MaxConcurrency <= 0 {
		cfg.MaxConcurrency = runtime.NumCPU()
	}
	return cfg
}

func TestDeterministicSerialization(t *testing.T) {
	cfg := testConfig()

	scannerA := NewScanner()
	resA, err := scannerA.ScanWithConfig(cfg)
	if err != nil {
		t.Fatalf("scan A failed: %v", err)
	}
	bA, err := json.MarshalIndent(resA.Serialize(), "", "\t")
	if err != nil {
		t.Fatalf("marshal A failed: %v", err)
	}

	// Run a second scan and compare outputs
	scannerB := NewScanner()
	resB, err := scannerB.ScanWithConfig(cfg)
	if err != nil {
		t.Fatalf("scan B failed: %v", err)
	}
	bB, err := json.MarshalIndent(resB.Serialize(), "", "\t")
	if err != nil {
		t.Fatalf("marshal B failed: %v", err)
	}

	if string(bA) != string(bB) {
		t.Fatalf("serialization is not deterministic between runs\nlenA=%d lenB=%d", len(bA), len(bB))
	}
}

func TestTypesFullyLoadedBeforeSerialize(t *testing.T) {
	cfg := testConfig()

	scanner := NewScanner()
	res, err := scanner.ScanWithConfig(cfg)
	if err != nil {
		t.Fatalf("scan failed: %v", err)
	}

	// For several known types, ensure their internal structures are populated
	typesCol := res.Types

	// local example struct with fields
	if typ, ok := typesCol.Get("github.com/pablor21/goscanner/examples/starwars/basic.MyStruct"); ok {
		if s, ok := typ.(*gstypes.Struct); ok {
			if len(s.Fields()) == 0 {
				t.Fatalf("expected MyStruct to have fields loaded")
			}
		}
	}

	// local example with embed + own field: one embed, one field
	if typ, ok := typesCol.Get("github.com/pablor21/goscanner/examples/starwars/basic.TargetStruct"); ok {
		if s, ok := typ.(*gstypes.Struct); ok {
			if len(s.Embeds()) == 0 || len(s.Fields()) == 0 {
				t.Fatalf("expected TargetStruct to have embeds and fields loaded")
			}
		}
	}

	// interface with methods (from examples)
	if typ, ok := typesCol.Get("github.com/pablor21/goscanner/examples/starwars/basic.MyInterfacePointer"); ok {
		if i, ok := typ.(*gstypes.Interface); ok {
			if len(i.Methods()) == 0 {
				t.Fatalf("expected MyInterfacePointer to have methods loaded")
			}
		}
	}

	// Additionally, ensure Serialize does not mutate structures by comparing counts before/after
	for _, typ := range typesCol.Values() {
		beforeFields, beforeMethods := 0, 0
		beforeEmbeds, beforeParams, beforeResults := 0, 0, 0

		switch v := typ.(type) {
		case *gstypes.Struct:
			beforeFields = len(v.Fields())
			beforeMethods = len(v.Methods())
			beforeEmbeds = len(v.Embeds())
		case *gstypes.Interface:
			beforeMethods = len(v.Methods())
			beforeEmbeds = len(v.Embeds())
		case *gstypes.Function:
			beforeParams = len(v.Parameters())
			beforeResults = len(v.Results())
		}

		_ = typ.Serialize()

		switch v := typ.(type) {
		case *gstypes.Struct:
			if beforeFields != len(v.Fields()) || beforeMethods != len(v.Methods()) || beforeEmbeds != len(v.Embeds()) {
				t.Fatalf("Serialize mutated struct content for %s", v.Id())
			}
		case *gstypes.Interface:
			if beforeMethods != len(v.Methods()) || beforeEmbeds != len(v.Embeds()) {
				t.Fatalf("Serialize mutated interface content for %s", v.Id())
			}
		case *gstypes.Function:
			if beforeParams != len(v.Parameters()) || beforeResults != len(v.Results()) {
				t.Fatalf("Serialize mutated function content for %s", v.Id())
			}
		}
	}
}
