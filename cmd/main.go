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
		_, err = v.GetDetails()
		if err != nil {
			panic(err)
		}
	}

	// convert the ret to a json
	b, err := json.MarshalIndent(ret, "", "  ")
	if err != nil {
		panic(err)
	}

	// save the output to a file
	os.WriteFile("output.json", b, 0644)

	// fmt.Println(string(b))
}
