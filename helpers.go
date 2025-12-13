package goscanner

import (
	"runtime"
	"strings"
	"syscall"

	"github.com/pablor21/gonnotation"
)

// isExported reports whether name is an exported Go symbol
// (that is, whether it begins with an upper-case letter).
func isExported(name string) bool {
	// return true
	return name != "" && name[0] >= 'A' && name[0] <= 'Z'
}

func MemUsage() uint64 {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	return m.Alloc // bytes allocated and still in use
}

func RSS() uint64 {
	var stat syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &stat)
	return uint64(stat.Maxrss) * 1024 // RSS in bytes
}

// a comment is any that does not starts with a @
func parseComments(doc string) []string {
	if doc == "" {
		return nil
	}

	// Split the documentation text into lines
	lines := strings.Split(doc, "\n")
	if len(lines) == 0 {
		return nil
	}

	var comments []string
	for _, line := range lines {
		// Trim leading and trailing whitespace
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if !strings.HasPrefix(line, "@") {
			comments = append(comments, line)
		}
	}
	if len(comments) == 0 {
		return nil
	}

	return comments
}

// parseAnnotations extracts annotation lines (those starting with @) from documentation
func parseAnnotations(doc string) []gonnotation.Annotation {
	if doc == "" {
		return nil
	}

	// Split the documentation text into lines
	lines := strings.Split(doc, "\n")
	if len(lines) == 0 {
		return nil
	}

	var annotationLines []string
	for _, line := range lines {
		// Trim leading and trailing whitespace
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "@") {
			annotationLines = append(annotationLines, line)
		}
	}

	if len(annotationLines) == 0 {
		return nil
	}

	// Join annotation lines and parse them
	annotationText := strings.Join(annotationLines, "\n")
	return gonnotation.ParseAnnotationsFromText(annotationText)
}
