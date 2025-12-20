package main

import (
	"encoding/json"
	"flag"
	"os"
	"strings"

	"github.com/pablor21/goscanner/logger"
	"github.com/pablor21/goscanner/scanner"
)

var pkg string
var output string
var cacheOut string
var useCache bool

func main() {
	// get the package scanning to (flag)
	flag.StringVar(&pkg, "pkg", "../examples/starwars/basic,../examples/starwars/functions", "Package to scan")
	flag.StringVar(&output, "out", "output.json", "Output file")
	flag.StringVar(&cacheOut, "cache-out", ".scan.cache", "Output binary cache file (gzip-compressed JSON)")
	flag.BoolVar(&useCache, "use-cache", false, "Load from cache if available (default: false)")
	flag.Parse()

	cfg := scanner.NewDefaultConfig()
	cfg.Packages = strings.Split(pkg, ",")
	cfg.LogLevel = "info"

	// Create a logger for the main function
	logger.SetupLogger(cfg.LogLevel)
	log := logger.NewDefaultLogger()

	var ret *scanner.ScanningResult
	var err error

	// Try to load from cache if requested
	if useCache && cacheOut != "" && scanner.IsCacheValid(cacheOut) {
		ret, err = scanner.ReadCache(cacheOut)
		if err == nil {
			log.Infof("Loaded scanning results from cache: %s", cacheOut)
			goto writeOutput
		}
		log.Warnf("Failed to load cache, falling back to full scan: %v", err)
	}

	// Perform full scan
	ret, err = scanner.NewScanner().ScanWithConfig(cfg)
	if err != nil {
		panic(err)
	}

	// Ensure all types are fully loaded before caching
	if cacheOut != "" {
		if err := ret.EnsureFullyLoaded(); err != nil {
			log.Warnf("Failed to fully load types before caching: %v", err)
		}
	}

writeOutput:
	// Save cache if specified
	if cacheOut != "" {
		if err := ret.ToCache(cacheOut); err != nil {
			log.Warnf("Failed to write cache file %s: %v", cacheOut, err)
		} else {
			log.Infof("Cache written to: %s", cacheOut)
		}
	}

	// Save JSON output if specified
	if output != "" {
		serializedret := ret.Serialize()

		// convert the ret to a json
		b, err := json.MarshalIndent(serializedret, "", "\t")
		if err != nil {
			panic(err)
		}

		// save the output to a file
		_ = os.WriteFile(output, b, 0644)
		log.Infof("JSON output written to: %s", output)
	}
}
