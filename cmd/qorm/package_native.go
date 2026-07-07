package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qorm/qorm/internal/bundle"
	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
)

// usedWidgets returns the set of widget types an app actually uses across all its
// scenes — the basis for deriving the permissions the package must declare.
func usedWidgets(appDir string) map[string]bool {
	used := map[string]bool{}
	app, err := loader.LoadDir(appDir)
	if err != nil {
		return used
	}
	var walk func(n *model.Node)
	walk = func(n *model.Node) {
		if n == nil {
			return
		}
		used[n.Type] = true
		for _, c := range n.Children {
			walk(c)
		}
		walk(n.Template)
	}
	if r := app.EntryRoot(); r != nil {
		walk(r)
	}
	for _, sc := range app.Scenes {
		walk(sc)
	}
	return used
}

// pkgID turns an app name into a safe reverse-DNS-ish identifier segment.
func pkgID(name string) string {
	id := regexp.MustCompile(`[^a-z0-9]`).ReplaceAllString(strings.ToLower(name), "")
	if id == "" {
		id = "app"
	}
	if id[0] >= '0' && id[0] <= '9' {
		id = "a" + id
	}
	return id
}

// appWidgets loads an app's home-screen widgets.
func appWidgets(appDir string) []model.Widget {
	app, err := loader.LoadDir(appDir)
	if err != nil {
		return nil
	}
	return app.Widgets
}

// appShortcuts loads an app's icon quick actions.
func appShortcuts(appDir string) []model.Shortcut {
	app, err := loader.LoadDir(appDir)
	if err != nil {
		return nil
	}
	return app.Shortcuts
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func xmlEsc(s string) string {
	r := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;")
	return r.Replace(s)
}

// firstDevice returns the identifier of a connected, available physical device.
func firstDevice() string {
	out, err := exec.Command("xcrun", "devicectl", "list", "devices").Output()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.Contains(line, "available") && !strings.Contains(line, "unavailable") &&
			(strings.Contains(strings.ToLower(line), "iphone") || strings.Contains(strings.ToLower(line), "ipad")) {
			f := strings.Fields(line)
			for _, tok := range f {
				if len(tok) == 36 && strings.Count(tok, "-") == 4 {
					return tok
				}
			}
		}
	}
	return ""
}

// awProvider returns the first declared widget (or a name-only placeholder).
func awProvider(ws []model.Widget, name string) model.Widget {
	if len(ws) > 0 {
		return ws[0]
	}
	return model.Widget{Name: name}
}

// spliceUser injects the app's native/<file> snippet at a marker in a generated
// bridge, so an app can register its OWN native ops (the same qormToNative /
// qormOn<X> contract) without forking the framework. Empty file → fallback.
func spliceUser(src, marker, appDir, file, fallback string) string {
	code := fallback
	if b, err := os.ReadFile(filepath.Join(appDir, "native", file)); err == nil {
		code = string(b) // the app's snippet replaces the fallback
	}
	return strings.Replace(src, marker, code, 1)
}

// injectUserGo copies the app's native/desktop.go into the cmd/qorm package as
// userops_gen.go so `go build` compiles the user's Go middle-layer INTO the one
// binary; the returned func removes it afterward. No file → no-op.
func injectUserGo(appDir, pkg string) func() {
	src := filepath.Join(appDir, "native", "desktop.go")
	data, err := os.ReadFile(src)
	if err != nil {
		return func() {}
	}
	out, err := exec.Command("go", "list", "-e", "-f", "{{.Dir}}", pkg).Output()
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: can't locate cmd/qorm to compile native/desktop.go: %v\n", err)
		return func() {}
	}
	// strip any //go:build / // +build lines (the app keeps them so go build ./...
	// skips the file; injected into cmd/qorm it must compile unconditionally).
	var kept []string
	for _, ln := range strings.Split(string(data), "\n") {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "//go:build") || strings.HasPrefix(t, "// +build") {
			continue
		}
		kept = append(kept, ln)
	}
	dst := filepath.Join(strings.TrimSpace(string(out)), "userops_gen.go")
	if err := os.WriteFile(dst, []byte(strings.Join(kept, "\n")), 0o644); err != nil {
		// Don't fail silently: a go-installed qorm points at the read-only module
		// cache, so the write fails and the app's native ops would be dropped
		// with no warning. Tell the user how to get them included.
		fmt.Fprintf(os.Stderr, "warn: could NOT compile native/desktop.go into this package (%v)\n"+
			"      your custom native ops will be MISSING. Build qorm from a writable\n"+
			"      source checkout (git clone + go build ./cmd/qorm) instead of `go install`.\n", err)
		return func() {}
	}
	fmt.Fprintf(os.Stderr, "compiling your Go middle-layer (native/desktop.go) into the binary…\n")
	return func() { os.Remove(dst) }
}

// writeAppBundle compiles the app source into one signed bundle.json in destDir
// (the same artifact web/mobile ships) and copies native/ alongside so the
// desktop server can still inject native/web.js. Keeps all platforms consistent:
// one bundle drives the app everywhere.
func writeAppBundle(srcDir, destDir string) error {
	b, err := bundle.Build(srcDir)
	if err != nil {
		return err
	}
	bj, err := bundle.Marshal(b)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(destDir, "bundle.json"), bj, 0o644); err != nil {
		return err
	}
	if nd := filepath.Join(srcDir, "native"); dirExists(nd) {
		return copyTree(nd, filepath.Join(destDir, "native"))
	}
	return nil
}

func dirExists(p string) bool { fi, err := os.Stat(p); return err == nil && fi.IsDir() }
