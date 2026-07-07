package render

import "fmt"

// Built-in SVG icon set — the framework's alternative to emoji. Icons are clean
// stroke glyphs (24×24, stroke=currentColor) so they inherit text color/size
// and look native. The `icon` widget resolves a name here; unknown names fall
// back to rendering the raw text. Keep glyphs simple + geometric so they render
// correctly and read at small sizes.
var iconPaths = map[string]string{
	// hardware capabilities
	"camera":      `<path d="M3 8a2 2 0 0 1 2-2h2l1.5-2h7L17 4h2a2 2 0 0 1 2 2v11a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/><circle cx="12" cy="12.5" r="3.5"/>`,
	"image":       `<rect x="3" y="4" width="18" height="16" rx="2"/><circle cx="8.5" cy="9" r="1.8"/><path d="M4 18l5-5 4 4 3-3 4 4"/>`,
	"video":       `<rect x="2" y="6" width="13" height="12" rx="2"/><path d="M15 10l6-3v10l-6-3z"/>`,
	"mic":         `<rect x="9" y="3" width="6" height="11" rx="3"/><path d="M6 11a6 6 0 0 0 12 0M12 17v4M8 21h8"/>`,
	"location":    `<path d="M12 21s-6-5.2-6-10a6 6 0 0 1 12 0c0 4.8-6 10-6 10z"/><circle cx="12" cy="11" r="2.2"/>`,
	"compass":     `<circle cx="12" cy="12" r="9"/><path d="M15.5 8.5l-2 5-5 2 2-5z"/>`,
	"bluetooth":   `<path d="M7 7l10 10-5 4V3l5 4L7 17"/>`,
	"wifi":        `<path d="M2 8.5a15 15 0 0 1 20 0M5 12a10 10 0 0 1 14 0M8 15.5a5 5 0 0 1 8 0"/><circle cx="12" cy="19" r="1"/>`,
	"nfc":         `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 15V9l5 6V9M17 9v6"/>`,
	"battery":     `<rect x="2" y="7" width="18" height="10" rx="2"/><path d="M22 11v2"/><rect x="4" y="9" width="10" height="6" rx="1" fill="currentColor" stroke="none"/>`,
	"bell":        `<path d="M6 9a6 6 0 0 1 12 0c0 5 2 6 2 6H4s2-1 2-6z"/><path d="M10 19a2 2 0 0 0 4 0"/>`,
	"flashlight":  `<path d="M8 3h8v3l-1.5 3v9a1 1 0 0 1-1 1h-3a1 1 0 0 1-1-1V9L8 6z"/><path d="M11 12h2"/>`,
	"volume":      `<path d="M4 9v6h4l5 4V5L8 9z"/><path d="M16 9a4 4 0 0 1 0 6"/>`,
	"brightness":  `<circle cx="12" cy="12" r="4"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M19 5l-2 2M7 17l-2 2"/>`,
	"share":       `<circle cx="18" cy="5" r="3"/><circle cx="6" cy="12" r="3"/><circle cx="18" cy="19" r="3"/><path d="M8.6 10.5l6.8-4M8.6 13.5l6.8 4"/>`,
	"clipboard":   `<rect x="5" y="4" width="14" height="17" rx="2"/><rect x="9" y="2.5" width="6" height="4" rx="1"/><path d="M8 11h8M8 15h6"/>`,
	"copy":        `<rect x="8" y="8" width="12" height="12" rx="2"/><path d="M16 8V6a2 2 0 0 0-2-2H6a2 2 0 0 0-2 2v8a2 2 0 0 0 2 2h2"/>`,
	"device":      `<rect x="6" y="2" width="12" height="20" rx="3"/><path d="M11 18h2"/>`,
	"globe":       `<circle cx="12" cy="12" r="9"/><path d="M3 12h18M12 3c3 3.5 3 14 0 18M12 3c-3 3.5-3 14 0 18"/>`,
	"sun":         `<circle cx="12" cy="12" r="4"/><path d="M12 2v3M12 19v3M2 12h3M19 12h3M5 5l2 2M17 17l2 2M19 5l-2 2M7 17l-2 2"/>`,
	"zap":         `<path d="M13 2L4 14h7l-1 8 9-12h-7z"/>`,
	"database":    `<ellipse cx="12" cy="6" rx="8" ry="3"/><path d="M4 6v12c0 1.7 3.6 3 8 3s8-1.3 8-3V6M4 12c0 1.7 3.6 3 8 3s8-1.3 8-3"/>`,
	"lock":        `<rect x="5" y="11" width="14" height="10" rx="2"/><path d="M8 11V7a4 4 0 0 1 8 0v4"/>`,
	"fingerprint": `<path d="M12 5a7 7 0 0 0-7 7v3M19 12a7 7 0 0 0-3.5-6M8.5 20a10 10 0 0 1-1-4v-4a4.5 4.5 0 0 1 9 0v4M12 12v4a4 4 0 0 0 .5 2"/>`,
	"screenshot":  `<rect x="3" y="5" width="18" height="14" rx="2"/><path d="M7 5V3M17 5V3M7 19v2M17 19v2M3 9h2M19 9h2M3 15h2M19 15h2"/>`,
	// common UI
	"check":         `<path d="M4 12l5 5L20 6"/>`,
	"x":             `<path d="M6 6l12 12M18 6L6 18"/>`,
	"plus":          `<path d="M12 5v14M5 12h14"/>`,
	"minus":         `<path d="M5 12h14"/>`,
	"search":        `<circle cx="11" cy="11" r="7"/><path d="M21 21l-4.3-4.3"/>`,
	"settings":      `<circle cx="12" cy="12" r="3"/><path d="M12 2v3M12 19v3M4.2 4.2l2.1 2.1M17.7 17.7l2.1 2.1M2 12h3M19 12h3M4.2 19.8l2.1-2.1M17.7 6.3l2.1-2.1"/>`,
	"home":          `<path d="M4 11l8-7 8 7M6 10v10h12V10"/>`,
	"user":          `<circle cx="12" cy="8" r="4"/><path d="M4 21a8 8 0 0 1 16 0"/>`,
	"heart":         `<path d="M12 21C7 17 3 13.5 3 9a4.5 4.5 0 0 1 9-1 4.5 4.5 0 0 1 9 1c0 4.5-4 8-9 12z"/>`,
	"star":          `<path d="M12 3l2.6 5.3 5.9.9-4.3 4.1 1 5.8-5.2-2.7-5.2 2.7 1-5.8L3.5 9.2l5.9-.9z"/>`,
	"chevron-right": `<path d="M9 5l7 7-7 7"/>`,
	"chevron-down":  `<path d="M5 9l7 7 7-7"/>`,
	"download":      `<path d="M12 3v12M7 11l5 5 5-5M4 21h16"/>`,
	"upload":        `<path d="M12 21V9M7 13l5-5 5 5M4 4h16"/>`,
}

// iconSVG returns an inline stroke SVG for a named icon, or "" if the name is
// unknown. size is the pixel box; color comes from currentColor (inherits text).
func iconSVG(name string, size float64) string {
	body, ok := iconPaths[name]
	if !ok {
		return ""
	}
	if size <= 0 {
		size = 24
	}
	return fmt.Sprintf(`<svg width="%g" height="%g" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" style="display:inline-block;vertical-align:middle;flex:none;">%s</svg>`, size, size, body)
}
