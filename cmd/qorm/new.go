package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cmdNew scaffolds a minimal, runnable QORM app so an agent or a human can
// design from scratch rather than only editing an existing app.
func cmdNew(args []string) int {
	var dir, name string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 < len(args) {
				i++
				name = args[i]
			}
		default:
			dir = args[i]
		}
	}
	if dir == "" {
		fmt.Fprintln(os.Stderr, "usage: qorm new <dir> [--name \"App Name\"]")
		return 2
	}
	// Refuse to scaffold into a non-empty directory.
	if entries, err := os.ReadDir(dir); err == nil && len(entries) > 0 {
		fmt.Fprintf(os.Stderr, "error: %s is not empty\n", dir)
		return 1
	}
	id := sanitizeID(filepath.Base(filepath.Clean(dir)))
	if name == "" {
		name = filepath.Base(filepath.Clean(dir))
	}

	files := map[string]string{
		"qorm.json":        fmt.Sprintf(manifestTmpl, id, jsonString(name)),
		"scenes/main.json": fmt.Sprintf(sceneTmpl, jsonString(name)),
		"actions/inc.json": incActionTmpl,
	}
	for rel, content := range files {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
	}
	fmt.Printf("created QORM app in %s\n", dir)
	fmt.Printf("  run it:   qorm run %s\n", dir)
	fmt.Printf("  design:   qorm mcp %s   (agent surface)\n", dir)
	return 0
}

func sanitizeID(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == ' ' || r == '_':
			b.WriteRune('_')
		}
	}
	if b.Len() == 0 {
		return "qorm_app"
	}
	return b.String()
}

// jsonString escapes a string for embedding in the JSON templates.
func jsonString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

const manifestTmpl = `{
  "type": "app",
  "id": "%s",
  "name": "%s",
  "entry": "main",
  "version": "0.1.0",
  "globalState": {
    "schema": { "count": "number" },
    "initial": { "count": 0 }
  },
  "platforms": {
    "desktop": { "window": { "width": 420, "height": 640, "title": "QORM App" } }
  }
}
`

const sceneTmpl = `{
  "type": "scene",
  "id": "main",
  "root": {
    "type": "column",
    "id": "root",
    "style": { "background": "#0F172A", "padding": 32, "gap": 16 },
    "layout": { "width": "fill", "height": "fill", "align": "center", "justify": "center" },
    "children": [
      { "type": "text", "id": "title", "text": "%s",
        "style": { "color": "#F8FAFC", "fontSize": 28, "fontWeight": 800, "textAlign": "center" } },
      { "type": "text", "id": "count", "text": "Count: {{state.count}}",
        "style": { "color": "#38BDF8", "fontSize": 18, "textAlign": "center" } },
      { "type": "button", "id": "tap", "label": "Tap me",
        "style": { "background": "#38BDF8", "color": "#0F172A", "fontSize": 16, "fontWeight": 700,
                   "width": 160, "height": 48, "borderRadius": 24 },
        "onPress": { "type": "invoke", "name": "inc", "args": { "count": "{{state.count}}" } } }
    ]
  }
}
`

const incActionTmpl = `{
  "type": "action",
  "id": "inc",
  "steps": [
    { "type": "state.set", "path": "count", "value": "{{ count + 1 }}" }
  ]
}
`
