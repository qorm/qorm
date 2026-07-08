package mdsite

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// TestSiteLinksResolve builds the real docs/ and api/ trees the way the website
// ships them — as sibling directories under one web root (/docs, /api) — and
// asserts every internal link resolves: no empty hrefs, no dangling relative or
// root-relative targets, no anchors to ids that don't exist. This is what keeps
// the site free of broken/empty links as pages move or get renamed.
func TestSiteLinksResolve(t *testing.T) {
	root := t.TempDir()
	// The docs/api sites are deployed as siblings under a web root that also
	// holds the hand-written landing page and shared assets; seed those so the
	// template's root-relative links (/, /assets/logo.svg) resolve as they do
	// in production.
	os.WriteFile(filepath.Join(root, "index.html"), []byte("<h1>QORM</h1>"), 0o644)
	os.MkdirAll(filepath.Join(root, "assets"), 0o755)
	os.WriteFile(filepath.Join(root, "assets", "logo.svg"), []byte("<svg/>"), 0o644)

	for _, s := range []struct{ src, out, name string }{
		{"../../docs", filepath.Join(root, "docs"), "docs"},
		{"../../api", filepath.Join(root, "api"), "api"},
	} {
		if _, err := os.Stat(s.src); err != nil {
			continue // tree not present in this checkout
		}
		if _, err := BuildSite(s.src, s.out, s.name); err != nil {
			t.Fatalf("BuildSite(%s): %v", s.src, err)
		}
	}

	// index every emitted file, and the ids present on each html page.
	files := map[string]bool{}
	ids := map[string]map[string]bool{}
	idRe := regexp.MustCompile(`id="([^"]+)"`)
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel := filepath.ToSlash(mustRel(t, root, p))
		files[rel] = true
		if strings.HasSuffix(p, ".html") {
			b, _ := os.ReadFile(p)
			set := map[string]bool{}
			for _, m := range idRe.FindAllStringSubmatch(string(b), -1) {
				set[m[1]] = true
			}
			ids[rel] = set
		}
		return nil
	})

	attrRe := regexp.MustCompile(`(?:href|src)="([^"]*)"`)
	unesc := strings.NewReplacer("&amp;", "&", "&#39;", "'", "&quot;", `"`, "&lt;", "<", "&gt;", ">")
	var bad []string
	for rel := range ids {
		b, _ := os.ReadFile(filepath.Join(root, rel))
		dir := filepath.Dir(rel)
		for _, m := range attrRe.FindAllStringSubmatch(string(b), -1) {
			u := strings.TrimSpace(unesc.Replace(m[1]))
			switch {
			case u == "":
				bad = append(bad, rel+" :: empty href/src")
				continue
			case u == "#":
				bad = append(bad, rel+" :: bare '#'")
				continue
			case strings.HasPrefix(u, "http://"), strings.HasPrefix(u, "https://"),
				strings.HasPrefix(u, "mailto:"), strings.HasPrefix(u, "data:"),
				strings.HasPrefix(u, "tel:"), strings.HasPrefix(u, "javascript:"):
				continue
			}
			path, frag := u, ""
			if i := strings.IndexByte(u, '#'); i >= 0 {
				path, frag = u[:i], u[i+1:]
			}
			if path == "" { // same-page anchor
				if frag != "" && !ids[rel][frag] {
					bad = append(bad, rel+" :: missing anchor #"+frag)
				}
				continue
			}
			var tgt string
			if strings.HasPrefix(path, "/") {
				tgt = strings.TrimPrefix(path, "/")
			} else {
				tgt = filepath.ToSlash(filepath.Clean(filepath.Join(dir, path)))
			}
			if tgt == "" {
				tgt = "index.html" // the web root itself
			} else if strings.HasSuffix(tgt, "/") || !strings.Contains(filepath.Base(tgt), ".") {
				tgt = strings.TrimPrefix(strings.TrimSuffix(tgt, "/")+"/index.html", "/")
			}
			if !files[tgt] {
				bad = append(bad, rel+" :: broken -> "+u)
			} else if frag != "" && strings.HasSuffix(tgt, ".html") && !ids[tgt][frag] {
				bad = append(bad, rel+" :: broken anchor -> "+u)
			}
		}
	}
	if len(bad) > 0 {
		t.Errorf("%d broken/empty links in the generated site:\n  %s", len(bad), strings.Join(bad, "\n  "))
	}
}

func mustRel(t *testing.T, base, p string) string {
	r, err := filepath.Rel(base, p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}
