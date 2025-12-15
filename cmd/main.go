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

	// // load details for each type
	// for _, v := range ret.Types {
	// 	err = v.Load()
	// 	if err != nil {
	// 		panic(err)
	// 	}

	// 	// Trigger comment extraction
	// 	// v.GetComments()
	// }

	// convert the ret to a json
	b, err := json.MarshalIndent(ret, "", "  ")
	if err != nil {
		panic(err)
	}

	// save the output to a file
	_ = os.WriteFile(output, b, 0644)

	// fmt.Println(string(b))
}
