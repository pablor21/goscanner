package main

import (
	"encoding/json"
	"flag"
	"os"

	"github.com/pablor21/goscanner/scanner"
)

var pkg string
var output string

func main() {
	// get the package scanning to (flag)
	flag.StringVar(&pkg, "pkg", "../examples/starwars/basic", "Package to scan")
	flag.StringVar(&output, "out", "output.json", "Output file")
	flag.Parse()

	cfg := scanner.NewDefaultConfig()
	cfg.Packages = []string{pkg}
	cfg.LogLevel = "debug"

	ret, err := scanner.NewScanner().ScanWithConfig(cfg)
	if err != nil {
		panic(err)
	}

	serializedret := ret.Serialize()

	// convert the ret to a json
	b, err := json.MarshalIndent(serializedret, "", "\t")
	if err != nil {
		panic(err)
	}

	// save the output to a file
	_ = os.WriteFile(output, b, 0644)

	// fmt.Println(string(b))
}
