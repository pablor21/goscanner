package main

import (
	"encoding/json"
	"os"

	"github.com/pablor21/goscanner"
)

func main() {
	ret, err := goscanner.NewScanner().Scan()
	if err != nil {
		panic(err)
	}

	// load details for each type
	for _, v := range ret.Types {
		_, err = v.Load()
		if err != nil {
			panic(err)
		}

		// Trigger comment and annotation extraction
		_ = v.GetComments()
		_ = v.GetAnnotations()
	}

	// convert the ret to a json
	b, err := json.MarshalIndent(ret, "", "  ")
	if err != nil {
		panic(err)
	}

	// save the output to a file
	_ = os.WriteFile("output.json", b, 0644)

	// fmt.Println(string(b))
}
