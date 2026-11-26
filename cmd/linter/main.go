// Линтер проекта: запускает анализатор exitcheck сам по себе или через go vet -vettool.
package main

import "golang.org/x/tools/go/analysis/singlechecker"

func main() {
	singlechecker.Main(Analyzer)
}
