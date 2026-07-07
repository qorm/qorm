package main

import (
	"fmt"
	"os"

	"github.com/qorm/qorm/internal/mdsite"
)

// cmdDocs renders the markdown docs tree into a static HTML site (pure Go).
func cmdDocs(args []string) int {
	docsDir, outDir := "docs", "site"
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--docs":
			if i+1 < len(args) {
				i++
				docsDir = args[i]
			}
		case "-o", "--out":
			if i+1 < len(args) {
				i++
				outDir = args[i]
			}
		}
	}
	n, err := mdsite.BuildSite(docsDir, outDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("rendered %d pages: %s -> %s/index.html\n", n, docsDir, outDir)
	return 0
}
