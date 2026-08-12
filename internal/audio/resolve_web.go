package audio

import (
	"fmt"
	"path"
	"strings"
)

// ResolveWebSrc joins an author-relative audio src onto a browser BaseDir
// (URL prefix such as "/games/mario/" or "https://host/app/"). Same jail
// rules as resolveImageSrc for App.Web: no absolute/remote src, no ".."
// escape. Pure helper — unit-tested without a browser.
func ResolveWebSrc(baseDir, src string) (string, error) {
	src = strings.TrimSpace(src)
	if src == "" {
		return "", fmt.Errorf("empty src")
	}
	if strings.Contains(src, "://") || strings.HasPrefix(src, "data:") || strings.HasPrefix(src, "//") {
		return "", fmt.Errorf("remote src not supported")
	}
	if strings.HasPrefix(src, "/") {
		return "", fmt.Errorf("absolute src not allowed")
	}
	// path.Clean on a relative path: "audio/../x" → "x"; "../etc" → "../etc".
	cleaned := path.Clean(src)
	if cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("src escapes app dir")
	}
	// path.Clean(".") for empty-ish inputs; refuse those.
	if cleaned == "." || cleaned == "" {
		return "", fmt.Errorf("empty src")
	}
	if baseDir == "" {
		return "", fmt.Errorf("no app BaseDir")
	}
	return strings.TrimRight(baseDir, "/") + "/" + cleaned, nil
}
