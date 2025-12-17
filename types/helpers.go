package types

import (
	"strings"
)

func ExtractComments(doc string) string {
	if doc == "" {
		return ""
	}
	var lines []string

	for _, c := range strings.Split(doc, "\n") {
		trimmed := strings.TrimSpace(c)
		if trimmed != "" {
			lines = append(lines, trimmed)
		}
	}

	return strings.Join(lines, "\n")
}
