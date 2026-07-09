// Package sourcemap maps a live node's id back to where it is declared in an
// app's source .json files. QORM writes ids literally in the scene/action JSON,
// so a node the agent found (or a human clicked in the devtool) can be traced to
// an exact file + line to edit — the reverse-lookup a UI inspector needs, with no
// change to the loader or the node model.
package sourcemap

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Location is where an id is declared: a repo-relative-ish file path (relative to
// the app's base dir), the 1-based line, and that line trimmed for display.
type Location struct {
	File    string `json:"file"`
	Line    int    `json:"line"`
	Snippet string `json:"snippet"`
}

// Locate finds the first `"id": "<id>"` declaration under baseDir and returns its
// location. ok is false when baseDir is empty (e.g. a bundle, which has no source
// tree) or the id is not declared literally (e.g. a templated id).
func Locate(baseDir, id string) (Location, bool) {
	if baseDir == "" || id == "" {
		return Location{}, false
	}
	re := idDeclRE(id)
	var found Location
	ok := false
	_ = walkJSON(baseDir, func(rel, path string) error {
		if ok {
			return fs.SkipAll
		}
		if loc, hit := scanFile(path, rel, re); hit {
			found, ok = loc, true
			return fs.SkipAll
		}
		return nil
	})
	return found, ok
}

// LocateAll returns id -> Location for every id declared under baseDir (first
// declaration wins on a duplicate). Useful for a devtool that resolves many
// clicked elements without re-scanning per lookup.
func LocateAll(baseDir string) map[string]Location {
	out := map[string]Location{}
	if baseDir == "" {
		return out
	}
	re := anyIDDeclRE()
	_ = walkJSON(baseDir, func(rel, path string) error {
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
		for line := 1; sc.Scan(); line++ {
			text := sc.Text()
			if m := re.FindStringSubmatch(text); m != nil {
				if _, dup := out[m[1]]; !dup {
					out[m[1]] = Location{File: rel, Line: line, Snippet: strings.TrimSpace(text)}
				}
			}
		}
		return nil
	})
	return out
}

// walkJSON calls fn(relPath, absPath) for every .json file under baseDir, skipping
// hidden directories.
func walkJSON(baseDir string, fn func(rel, path string) error) error {
	return filepath.WalkDir(baseDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() != "." && strings.HasPrefix(d.Name(), ".") {
				return fs.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".json" {
			return nil
		}
		rel, rerr := filepath.Rel(baseDir, path)
		if rerr != nil {
			rel = path
		}
		return fn(rel, path)
	})
}

func scanFile(path, rel string, re *regexp.Regexp) (Location, bool) {
	f, err := os.Open(path)
	if err != nil {
		return Location{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for line := 1; sc.Scan(); line++ {
		text := sc.Text()
		if re.MatchString(text) {
			return Location{File: rel, Line: line, Snippet: strings.TrimSpace(text)}, true
		}
	}
	return Location{}, false
}

func idDeclRE(id string) *regexp.Regexp {
	return regexp.MustCompile(`"id"\s*:\s*"` + regexp.QuoteMeta(id) + `"`)
}

func anyIDDeclRE() *regexp.Regexp {
	return regexp.MustCompile(`"id"\s*:\s*"([^"]+)"`)
}
