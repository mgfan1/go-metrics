// Package buildinfo печатает версию, дату и коммит сборки.
package buildinfo

import (
	"fmt"
	"io"
)

// Print выводит в w версию, дату и коммит сборки, пустое значение печатает как N/A.
func Print(w io.Writer, version, date, commit string) {
	fmt.Fprintf(w, "Build version: %s\nBuild date: %s\nBuild commit: %s\n", orNA(version), orNA(date), orNA(commit))
}

func orNA(value string) string {
	if value == "" {
		return "N/A"
	}
	return value
}
