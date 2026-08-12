package playcore

import "strings"

// CanonicalAssetURL strips query and fragment from a URL/path so a preloader
// cache key matches what the canvas engine looks up (BaseDir + "/" + src).
//
// The games host cache-busts fetches with "?v=<timestamp>"; without
// normalization every preload would miss and fall back to main-thread sync XHR.
func CanonicalAssetURL(u string) string {
	if u == "" {
		return u
	}
	if i := strings.IndexAny(u, "?#"); i >= 0 {
		return u[:i]
	}
	return u
}
