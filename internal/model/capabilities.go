package model

// CapabilitiesPolicy controls which qormToNative ops may run at call time.
// Declared in qorm.json under "capabilities". When empty and the app carries
// no bundle RequiredCapabilities, the runtime keeps legacy open access.
type CapabilitiesPolicy struct {
	// Mode selects how the allowlist is built:
	//   "" / "used-only" — widgets in scenes + RequiredCapabilities + Allow
	//   "manifest"       — only Allow (+ CustomOps)
	//   "open"           — disable call-time gate (startup checks may still apply)
	Mode      string
	Allow     []string // canonical capability stems
	Deny      []string // stems or op names always rejected
	CustomOps []string // middle-layer op names permitted when enforcing
}

// HasPolicy reports whether the manifest declared a capabilities block.
func (p CapabilitiesPolicy) HasPolicy() bool {
	return p.Mode != "" || len(p.Allow) > 0 || len(p.Deny) > 0 || len(p.CustomOps) > 0
}
