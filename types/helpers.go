package types

import (
	"strings"
)

func ExtractComments(doc string) (lines []string) {
	if doc == "" {
		return nil
	}

	for _, c := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(c)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	return
}
