package main

import "strings"

// canonicalAssetURL strips query and fragment from a URL/path so the
// preloader cache key matches what the canvas engine looks up.
//
// The games host (web_server/site/games/index.html) cache-busts fetches with
// "?v=<timestamp>", but resolveImageSrc builds BaseDir + "/" + src with no
// query. Without normalization every preload was a cache miss and the engine
// fell back to main-thread sync XHR — broken or empty for image-heavy games
// (raiden, mario). Tetris/2048 were unaffected because they have no PNGs.
func canonicalAssetURL(u string) string {
	if u == "" {
		return u
	}
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}
