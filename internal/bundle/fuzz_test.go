package bundle

import "testing"

// FuzzBundle ensures bundle decoding, integrity verification, and app
// reconstruction never panic on arbitrary input (bundles arrive over the
// wire, so robustness matters). Verification may fail, but must not crash.
func FuzzBundle(f *testing.F) {
	for _, s := range []string{
		"",
		"{}",
		"null",
		// Malformed signature block (bad algorithm, non-base64 value).
		`{"format":"qorm-bundle/1","content":{"app":{"type":"app"},"scenes":{},"actions":{}},"contentHash":"sha256:AAAA","signature":{"algorithm":"md5","keyId":"k","value":"!!not-base64!!"}}`,
		// Plausible but wrong: valid shape, tampered content hash.
		`{"format":"qorm-bundle/1","content":{"app":{"type":"app","entry":"main"},"scenes":{"main":{"type":"scene","id":"main","root":{"type":"text","id":"t","text":"x"}}},"actions":{}},"contentHash":"sha256:wrong"}`,
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, data string) {
		b, err := Unmarshal([]byte(data))
		if err != nil {
			return
		}
		_ = Verify(b, nil) // must not panic; nil trust = integrity-only
		_ = b.ToApp()      // must not panic
	})
}
