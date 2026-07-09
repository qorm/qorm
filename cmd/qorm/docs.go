package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/qorm/qorm/internal/mdsite"
)

// cmdDocs renders the markdown docs tree into a static HTML site (pure Go).
func cmdDocs(args []string) int {
	docsDir, outDir, siteName := "docs", "site", ""
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
		case "--name":
			if i+1 < len(args) {
				i++
				siteName = args[i]
			}
		}
	}
	if siteName == "" {
		// default the header label to the source folder's base name (docs, api, …)
		siteName = filepath.Base(docsDir)
	}
	// Stamp the header with the release this binary reports, so /docs and /api
	// always show the version they were rendered from.
	mdsite.Version = version
	n, err := mdsite.BuildSite(docsDir, outDir, siteName)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1
	}
	fmt.Printf("rendered %d pages: %s -> %s/index.html\n", n, docsDir, outDir)
	return 0
}
