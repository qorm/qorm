package bundle

// Adversarial coverage for the OTA trust primitive: canonicalization
// determinism, the integrity-only vs trusted-key verification modes,
// revocation keyed on the actual verifying key (not the unsigned keyId),
// and rejection of malformed / truncated / wrong-magic artifacts.

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/qorm/qorm/internal/keys"
	"github.com/qorm/qorm/internal/loader"
)

// --- canonicalization determinism ---

func TestCanonicalHashIndependentOfOrder(t *testing.T) {
	// Same logical app, different physical representation: nested maps built in
	// opposite insertion order, documents supplied in a different slice order,
	// locale catalogs built in a different order. The content hash must be
	// identical, because encoding/json sorts map keys at every level. Go also
	// randomizes map iteration per run, so this guards cross-run stability.
	rootFwd := map[string]any{}
	rootFwd["type"], rootFwd["id"], rootFwd["text"] = "column", "root", "hello"
	rootRev := map[string]any{}
	rootRev["text"], rootRev["id"], rootRev["type"] = "hello", "root", "column"
	sceneFwd := map[string]any{"type": "scene", "id": "main", "root": rootFwd}
	sceneRev := map[string]any{"root": rootRev, "id": "main", "type": "scene"}
	app1 := map[string]any{"type": "app", "id": "a1", "name": "One", "entry": "main"}
	app2 := map[string]any{"entry": "main", "name": "One", "id": "a1", "type": "app"}
	mkAction := func() map[string]any {
		return map[string]any{"type": "action", "id": "noop", "steps": []any{
			map[string]any{"type": "state.set", "path": "x", "value": "1"},
		}}
	}

	b1, err := fromDocs([]map[string]any{app1, sceneFwd, mkAction()},
		map[string]map[string]string{"en": {"greet": "hi", "bye": "bye"}, "zh": {"greet": "ni hao"}})
	if err != nil {
		t.Fatalf("fromDocs(1): %v", err)
	}
	b2, err := fromDocs([]map[string]any{mkAction(), sceneRev, app2},
		map[string]map[string]string{"zh": {"greet": "ni hao"}, "en": {"bye": "bye", "greet": "hi"}})
	if err != nil {
		t.Fatalf("fromDocs(2): %v", err)
	}
	if b1.ContentHash != b2.ContentHash {
		t.Errorf("canonical hash must not depend on map insertion order or doc order:\n b1=%s\n b2=%s",
			b1.ContentHash, b2.ContentHash)
	}
	if !strings.HasPrefix(b1.ContentHash, "sha256:") {
		t.Errorf("hash must carry its algorithm prefix, got %q", b1.ContentHash)
	}

	// Different content must NOT collide: flip one nested value.
	b3, err := fromDocs([]map[string]any{app1, sceneFwd, mkAction()},
		map[string]map[string]string{"en": {"greet": "HI", "bye": "bye"}, "zh": {"greet": "ni hao"}})
	if err != nil {
		t.Fatalf("fromDocs(3): %v", err)
	}
	if b3.ContentHash == b1.ContentHash {
		t.Error("changing a locale value must change the content hash")
	}
}

func TestBuildDeterministic(t *testing.T) {
	b1, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build(1): %v", err)
	}
	b2, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build(2): %v", err)
	}
	if b1.ContentHash != b2.ContentHash {
		t.Errorf("rebuilding the same app must yield the same content hash:\n %s\n %s",
			b1.ContentHash, b2.ContentHash)
	}
}

// --- verification modes ---

func TestIntegrityOnlyModeSemantics(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	pub, priv, err := keys.Generate()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}

	// A signed bundle still passes integrity-only verification: the signature
	// is extra metadata, not part of the content hash.
	if err := Verify(b, nil); err != nil {
		t.Errorf("signed bundle must pass integrity-only verify: %v", err)
	}

	// Zeroing the signature bytes is invisible to integrity-only mode but must
	// be rejected once a trust key is supplied.
	orig := b.Signature.Value
	b.Signature.Value = base64.StdEncoding.EncodeToString(make([]byte, ed25519.SignatureSize))
	if err := Verify(b, nil); err != nil {
		t.Errorf("signature bytes are not hashed; integrity-only verify must still pass: %v", err)
	}
	if err := Verify(b, pub); err == nil {
		t.Error("zeroed signature must fail against the trusted key")
	}
	b.Signature.Value = orig
	if err := Verify(b, pub); err != nil {
		t.Errorf("restored signature must verify: %v", err)
	}
}

func TestVerifyErrorString(t *testing.T) {
	e := &VerifyError{Reason: "boom"}
	if got, want := e.Error(), "bundle verification failed: boom"; got != want {
		t.Errorf("VerifyError.Error() = %q, want %q", got, want)
	}
	// A real tamper surfaces the prefix and the reason through Error().
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	b.ContentHash = "sha256:AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA="
	err = Verify(b, nil)
	if err == nil {
		t.Fatal("forged content hash must fail verification")
	}
	if !strings.HasPrefix(err.Error(), "bundle verification failed: ") {
		t.Errorf("verify failure must carry the VerifyError prefix, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "tampered") {
		t.Errorf("hash mismatch reason should mention tampering, got %q", err.Error())
	}
}

func TestVerifyRejectsUnsupportedAlgorithm(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	pub, priv, _ := keys.Generate()
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	for _, alg := range []string{"rsa-pkcs1-sha256", "ed25519 ", "ED25519", ""} {
		b.Signature.Algorithm = alg
		err := Verify(b, pub)
		if err == nil {
			t.Errorf("algorithm %q must be rejected", alg)
			continue
		}
		if ve, ok := err.(*VerifyError); !ok || !strings.Contains(ve.Reason, "unsupported signature algorithm") {
			t.Errorf("algorithm %q: want unsupported-algorithm VerifyError, got %v", alg, err)
		}
	}
}

func TestVerifyRejectsMalformedSignatureValue(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	pub, priv, _ := keys.Generate()
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}

	// Not base64 at all.
	b.Signature.Value = "%%%not-base64%%%"
	err = Verify(b, pub)
	if err == nil {
		t.Fatal("non-base64 signature must be rejected")
	}
	if ve, ok := err.(*VerifyError); !ok || !strings.Contains(ve.Reason, "malformed signature encoding") {
		t.Errorf("want malformed-encoding VerifyError, got %v", err)
	}

	// Valid base64, wrong length: a mismatch, not a panic.
	b.Signature.Value = base64.StdEncoding.EncodeToString([]byte("too-short"))
	err = Verify(b, pub)
	if err == nil {
		t.Fatal("wrong-length signature must be rejected")
	}
	if ve, ok := err.(*VerifyError); !ok || !strings.Contains(ve.Reason, "does not match trusted key") {
		t.Errorf("want signature-mismatch VerifyError, got %v", err)
	}

	// Empty signature decodes to zero bytes: mismatch again.
	b.Signature.Value = ""
	if err := Verify(b, pub); err == nil {
		t.Error("empty signature must be rejected")
	}
}

// --- artifact rejection: malformed / truncated / wrong magic ---

func TestUnmarshalRejectsMalformedArtifacts(t *testing.T) {
	good, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	data, err := Marshal(good)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	cases := []struct {
		name string
		data []byte
	}{
		{"empty", []byte("")},
		{"truncated", data[:len(data)/2]},
		{"binary garbage", []byte{0x00, 0x01, 0xff, 'n', 'o'}},
		{"json array", []byte(`[1,2,3]`)},
		{"json string", []byte(`"qorm-bundle/1"`)},
		{"wrong magic", []byte(strings.Replace(string(data), Format, "qorm-bundle/2", 1))},
		{"missing magic", []byte(`{"content":{"app":null,"scenes":{},"actions":{}},"contentHash":"sha256:e30="}`)},
		{"foreign magic", []byte(`{"format":"electron-asar/1","content":{},"contentHash":"sha256:e30="}`)},
	}
	for _, tc := range cases {
		if _, err := Unmarshal(tc.data); err == nil {
			t.Errorf("%s: Unmarshal must reject this artifact", tc.name)
		}
	}

	// A well-formed document is accepted.
	if _, err := Unmarshal(data); err != nil {
		t.Errorf("valid bundle must unmarshal: %v", err)
	}

	// An artifact that lost its contentHash must fail even integrity-only verify.
	stripped := *good
	stripped.ContentHash = ""
	if err := Verify(&stripped, nil); err == nil {
		t.Error("empty contentHash must fail the integrity check")
	}
}

func TestBuildRejectsUnusableDirs(t *testing.T) {
	// Nonexistent directory.
	if _, err := Build(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("Build on a missing directory must fail")
	}

	// Empty directory: no documents at all.
	if _, err := Build(t.TempDir()); err == nil || !strings.Contains(err.Error(), "no QORM source documents") {
		t.Errorf("Build on an empty directory must report no documents, got %v", err)
	}

	// A directory whose only .json files are malformed or test fixtures (plus a
	// non-json file) yields no usable documents.
	mixed := t.TempDir()
	writeFile(t, filepath.Join(mixed, "broken.json"), `{not json`)
	writeFile(t, filepath.Join(mixed, "fixture.json"), `{"type":"test","id":"t"}`)
	writeFile(t, filepath.Join(mixed, "notes.txt"), `{"type":"app","id":"x"}`)
	if _, err := Build(mixed); err == nil || !strings.Contains(err.Error(), "no QORM source documents") {
		t.Errorf("malformed/fixture-only directory must report no documents, got %v", err)
	}

	// Adding one valid scene document makes it buildable, with a nil manifest.
	writeFile(t, filepath.Join(mixed, "main.json"),
		`{"type":"scene","id":"main","root":{"type":"text","text":"hi"}}`)
	b, err := Build(mixed)
	if err != nil {
		t.Fatalf("Build with one scene: %v", err)
	}
	if b.Content.App != nil {
		t.Errorf("no manifest document -> Content.App must be nil, got %v", b.Content.App)
	}
	if _, ok := b.Content.Scenes["main"]; !ok {
		t.Error("scene 'main' missing from bundle content")
	}
	if err := Verify(b, nil); err != nil {
		t.Errorf("scene-only bundle must pass integrity verify: %v", err)
	}
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

// --- revocation keyed on the actual verifying key ---

func TestRevokedKeyCannotEvadeViaKeyIDRewrite(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	leakedPub, leakedPriv, err := keys.Generate()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	honestPub, _, err := keys.Generate()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	leakedID := keys.KeyID(leakedPub)
	honestID := keys.KeyID(honestPub)
	if err := b.Sign(leakedPriv, leakedID); err != nil {
		t.Fatalf("sign: %v", err)
	}

	// The holder of the revoked key rewrites the UNSIGNED keyId field to a
	// non-revoked identity. The signature covers ContentHash only, so it still
	// verifies — keyId is not authenticated.
	b.Signature.KeyID = honestID
	if err := Verify(b, leakedPub); err != nil {
		t.Fatalf("sanity: signature must remain valid after keyId rewrite: %v", err)
	}

	// Revocation is checked against the actual verifying key, so the bundle is
	// still rejected — the keyId rewrite does not evade the revocation list.
	revoked := RevocationList{leakedID: true}
	err = VerifyWithRevocation(b, leakedPub, revoked)
	if err == nil {
		t.Fatal("bundle signed by a revoked key must be rejected even after keyId is rewritten to a non-revoked identity")
	}
	ve, ok := err.(*VerifyError)
	if !ok {
		t.Fatalf("want *VerifyError, got %T (%v)", err, err)
	}
	if !strings.Contains(ve.Reason, "revoked") || !strings.Contains(ve.Reason, leakedID) {
		t.Errorf("rejection must name the verifying-key-derived id %q, got %q", leakedID, ve.Reason)
	}

	// Conversely, a revocation list that only contains the SPOOFED id (and not
	// the real verifying key) must let the bundle through: the self-declared
	// keyId has no authority in either direction.
	if err := VerifyWithRevocation(b, leakedPub, RevocationList{honestID: true}); err != nil {
		t.Errorf("revocation must be keyed on the verifying key, not keyId: %v", err)
	}

	// With no trust key, authenticity (and therefore revocation) is not
	// checked — documented integrity-only behavior.
	if err := VerifyWithRevocation(b, nil, revoked); err != nil {
		t.Errorf("nil trust key means integrity-only; revocation must not apply: %v", err)
	}
	// An empty revocation list never rejects.
	if err := VerifyWithRevocation(b, leakedPub, nil); err != nil {
		t.Errorf("empty revocation list must verify: %v", err)
	}
	// A genuinely non-revoked signer verifies under its own key.
	if err := VerifyWithRevocation(b, honestPub, revoked); err == nil {
		t.Error("sanity: signature does not verify under the honest key at all")
	}
}

func TestVerifyWithRevocationPropagatesIntegrityFailure(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	pub, priv, _ := keys.Generate()
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	// Tamper AND revoke: the integrity failure must surface (it is checked
	// first), not the revocation verdict.
	b.Content.App["name"] = "Malicious Rename"
	err = VerifyWithRevocation(b, pub, RevocationList{keys.KeyID(pub): true})
	if err == nil {
		t.Fatal("tampered bundle must fail VerifyWithRevocation")
	}
	if !strings.Contains(err.Error(), "tampered") {
		t.Errorf("integrity failure must be reported, got %v", err)
	}
}

// --- hash covers every content section ---

func TestHashCoversEveryContentSection(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	pub, priv, _ := keys.Generate()
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	data, err := Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	firstKey := func(m map[string]map[string]any) string {
		ks := make([]string, 0, len(m))
		for k := range m {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		if len(ks) == 0 {
			t.Fatal("expected a non-empty map")
		}
		return ks[0]
	}
	sceneID := firstKey(b.Content.Scenes)
	actionID := firstKey(b.Content.Actions)

	tamper := func(name string, mutate func(*Bundle)) {
		t.Run(name, func(t *testing.T) {
			c, err := Unmarshal(data) // deep copy of the signed bundle
			if err != nil {
				t.Fatalf("unmarshal copy: %v", err)
			}
			mutate(c)
			if err := Verify(c, pub); err == nil {
				t.Errorf("%s: tamper must fail trusted verify", name)
			}
			if err := Verify(c, nil); err == nil {
				t.Errorf("%s: tamper must fail integrity-only verify", name)
			}
		})
	}
	tamper("manifest field", func(c *Bundle) { c.Content.App["name"] = "Evil" })
	tamper("scene injection", func(c *Bundle) { c.Content.Scenes[sceneID]["injected"] = true })
	tamper("action injection", func(c *Bundle) { c.Content.Actions[actionID]["injected"] = true })
	tamper("added scene", func(c *Bundle) {
		c.Content.Scenes["backdoor"] = map[string]any{"type": "scene", "id": "backdoor"}
	})
	tamper("added locale", func(c *Bundle) {
		c.Content.Locales = map[string]map[string]string{"en": {"title": "spoof"}}
	})
	tamper("added capability", func(c *Bundle) { c.Content.RequiredCapabilities = []string{"sms"} })
	tamper("dropped action", func(c *Bundle) { delete(c.Content.Actions, actionID) })
}

// --- versioning and capability stamping ---

func TestSetVersionIsHashCovered(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if v := b.Version(); v != "" {
		t.Errorf("fresh bundle must have no version, got %q", v)
	}
	before := b.ContentHash
	if err := b.SetVersion("1.2.3"); err != nil {
		t.Fatalf("SetVersion: %v", err)
	}
	if b.Version() != "1.2.3" {
		t.Errorf("Version() = %q, want 1.2.3", b.Version())
	}
	if b.ContentHash == before {
		t.Error("version must be covered by the content hash")
	}

	pub, priv, _ := keys.Generate()
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := Verify(b, pub); err != nil {
		t.Fatalf("verify after sign: %v", err)
	}

	// Bumping the version after signing invalidates the signature until re-sign.
	if err := b.SetVersion("1.2.4"); err != nil {
		t.Fatalf("SetVersion(2): %v", err)
	}
	if err := Verify(b, pub); err == nil {
		t.Error("version bump after signing must break the signature")
	}
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("re-sign: %v", err)
	}
	if err := Verify(b, pub); err != nil {
		t.Fatalf("verify after re-sign: %v", err)
	}

	// The version lives in the hashed manifest, so it survives JSON round-trip.
	data, err := Marshal(b)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.Version() != "1.2.4" {
		t.Errorf("version lost in round-trip, got %q", got.Version())
	}

	// SetVersion on a bundle with no manifest creates one.
	bare := &Bundle{Format: Format, Content: Content{
		Scenes: map[string]map[string]any{}, Actions: map[string]map[string]any{},
	}}
	if bare.Version() != "" {
		t.Errorf("manifest-less bundle must report empty version, got %q", bare.Version())
	}
	if err := bare.SetVersion("0.0.1"); err != nil {
		t.Fatalf("SetVersion on bare bundle: %v", err)
	}
	if bare.Content.App == nil || bare.Version() != "0.0.1" {
		t.Errorf("SetVersion must create the manifest: app=%v version=%q", bare.Content.App, bare.Version())
	}
}

func TestSetRequiredCapabilitiesClearRestoresLegacyHash(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	legacy := b.ContentHash
	if err := b.SetRequiredCapabilities([]string{"camera"}); err != nil {
		t.Fatalf("SetRequiredCapabilities: %v", err)
	}
	if b.ContentHash == legacy {
		t.Error("adding capabilities must change the content hash")
	}
	// Clearing (nil or empty) restores the exact pre-field canonical encoding,
	// so the hash matches older bundles byte-for-byte.
	if err := b.SetRequiredCapabilities(nil); err != nil {
		t.Fatalf("clear(nil): %v", err)
	}
	if b.ContentHash != legacy {
		t.Errorf("clearing caps must restore the legacy hash:\n got  %s\n want %s", b.ContentHash, legacy)
	}
	if b.RequiredCapabilities() != nil {
		t.Errorf("cleared caps must be nil, got %v", b.RequiredCapabilities())
	}
	if err := b.SetRequiredCapabilities([]string{}); err != nil {
		t.Fatalf("clear(empty): %v", err)
	}
	if b.ContentHash != legacy || b.RequiredCapabilities() != nil {
		t.Error("empty caps slice must behave like nil (omitempty canonicalization)")
	}
}

// Documenting current behavior: RequiredCapabilities is conceptually a set but
// is encoded as an ordered array, so the same requirement set declared in a
// different order produces a different content hash (and therefore a different
// signature). Any future canonicalization fix changes every hash in flight, so
// such a change must be deliberate — this test flags it.
func TestRequiredCapabilitiesOrderSensitivity(t *testing.T) {
	b1, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	b2, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := b1.SetRequiredCapabilities([]string{"camera", "location"}); err != nil {
		t.Fatalf("caps(1): %v", err)
	}
	if err := b2.SetRequiredCapabilities([]string{"location", "camera"}); err != nil {
		t.Fatalf("caps(2): %v", err)
	}
	if b1.ContentHash == b2.ContentHash {
		t.Error("canonicalization of capabilities changed to order-independent: " +
			"this silently invalidates every previously signed bundle; update this test and the format docs deliberately")
	}
}

// --- revocation list parsing ---

func TestLoadRevocationRejectsMalformed(t *testing.T) {
	for _, bad := range []string{
		`not json`,
		`"a bare string"`,
		`{"revoked":[1,2]}`, // non-string entries
		`{"revoked":{}}`,    // wrong type for revoked
	} {
		if _, err := LoadRevocation([]byte(bad)); err == nil {
			t.Errorf("LoadRevocation(%s) must fail", bad)
		}
	}

	// Valid forms: array (deduplicated), object, and empty array.
	rl, err := LoadRevocation([]byte(`["a","b","a"]`))
	if err != nil || len(rl) != 2 || !rl["a"] || !rl["b"] {
		t.Errorf("array form: rl=%v err=%v", rl, err)
	}
	rl, err = LoadRevocation([]byte(`{"revoked":["c"]}`))
	if err != nil || len(rl) != 1 || !rl["c"] {
		t.Errorf("object form: rl=%v err=%v", rl, err)
	}
	rl, err = LoadRevocation([]byte(`[]`))
	if err != nil || len(rl) != 0 {
		t.Errorf("empty array: rl=%v err=%v", rl, err)
	}
}

// Regression test for the round-5 security defect (fixed): JSON null, an empty
// object, a null "revoked" field, and a foreign object used to parse silently
// as "nobody is revoked" — a fail-open trap that let a misconfigured or
// hijacked revocation endpoint disable the leaked-key defence. LoadRevocation
// now fails closed: each such payload is rejected with a *VerifyError so
// callers surface the misconfiguration instead of un-revoking keys. The
// well-formed empty lists {"revoked":[]} and [] stay valid (a legitimate
// "nothing revoked").
func TestLoadRevocationFailsClosedOnNullOrForeignJSON(t *testing.T) {
	for _, soft := range []string{`null`, `{}`, `{"revoked":null}`, `{"unrelated":1}`} {
		rl, err := LoadRevocation([]byte(soft))
		if err == nil {
			t.Fatalf("%s must be rejected (fail closed), got empty list %v", soft, rl)
		}
		if _, ok := err.(*VerifyError); !ok {
			t.Errorf("%s: want *VerifyError so callers can surface it, got %T (%v)", soft, err, err)
		}
		if rl != nil {
			t.Errorf("%s: rejected input must yield a nil list, got %v", soft, rl)
		}
	}

	// Well-formed empty lists remain valid: an operator revoking nothing
	// serves {"revoked":[]} (or []), and that must load without error.
	for _, empty := range []string{`{"revoked":[]}`, `[]`} {
		rl, err := LoadRevocation([]byte(empty))
		if err != nil {
			t.Fatalf("well-formed empty list %s must remain valid: %v", empty, err)
		}
		if len(rl) != 0 {
			t.Errorf("empty list %s must parse to zero revocations, got %v", empty, rl)
		}
	}

	// Concrete impact: a misconfigured or hijacked revocation endpoint
	// serving `null` can no longer silently un-revoke a leaked key.
	pub, priv, err := keys.Generate()
	if err != nil {
		t.Fatalf("keygen: %v", err)
	}
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if err := b.Sign(priv, keys.KeyID(pub)); err != nil {
		t.Fatalf("sign: %v", err)
	}
	if err := VerifyWithRevocation(b, pub, RevocationList{keys.KeyID(pub): true}); err == nil {
		t.Fatal("sanity: the key IS revoked when the list is well-formed")
	}

	// The caller contract used by cmd/qorm (loadRuntime / buildServer / the
	// verify command) and the OTA path: a revocation payload that fails to
	// parse aborts verification entirely — VerifyWithRevocation is never
	// reached with a phantom empty list, so the bundle signed by the leaked
	// key is refused rather than silently accepted.
	rl, loadErr := LoadRevocation([]byte(`null`))
	verdict := loadErr
	if verdict == nil {
		verdict = VerifyWithRevocation(b, pub, rl)
	}
	if verdict == nil {
		t.Fatal("a null revocation payload must refuse the bundle, not silently un-revoke the signing key")
	}

	// And a genuinely empty (but well-formed) list revokes nobody: a bundle
	// signed by a non-revoked key still verifies under it.
	empty, err := LoadRevocation([]byte(`{"revoked":[]}`))
	if err != nil {
		t.Fatalf("well-formed empty list must load: %v", err)
	}
	if err := VerifyWithRevocation(b, pub, empty); err != nil {
		t.Errorf("well-formed empty revocation list must verify a non-revoked bundle: %v", err)
	}
}

// --- ToApp rehydration via loader.FromDocs ---

func TestToAppRehydratesViaFromDocs(t *testing.T) {
	b, err := Build(counterDir())
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	app := b.ToApp()
	if app.Entry != "main" {
		t.Errorf("entry = %q, want main", app.Entry)
	}
	if app.EntryRoot() == nil {
		t.Fatal("rehydrated app must have an entry root")
	}
	if len(app.Scenes) != len(b.Content.Scenes) || len(app.Actions) != len(b.Content.Actions) {
		t.Errorf("scene/action counts lost: %d/%d scenes, %d/%d actions",
			len(app.Scenes), len(b.Content.Scenes), len(app.Actions), len(b.Content.Actions))
	}
	if _, ok := app.Actions["increment"]; !ok {
		t.Error("action 'increment' lost in ToApp")
	}

	// A bundle with scenes but no manifest rehydrates with a working entry.
	sceneOnly := &Bundle{Format: Format, Content: Content{
		Scenes: map[string]map[string]any{
			"main": {"type": "scene", "id": "main", "root": map[string]any{"type": "text", "text": "bare"}},
		},
		Actions: map[string]map[string]any{},
	}}
	app2 := sceneOnly.ToApp()
	if app2.EntryRoot() == nil {
		t.Fatal("scene-only bundle must rehydrate an entry root")
	}
	if app2.EntryRoot().Text != "bare" {
		t.Errorf("entry root text = %q, want bare", app2.EntryRoot().Text)
	}

	// A manifest-only bundle yields the manifest fields but no entry root.
	manifestOnly := &Bundle{Format: Format, Content: Content{
		App:     map[string]any{"type": "app", "id": "x", "entry": "main"},
		Scenes:  map[string]map[string]any{},
		Actions: map[string]map[string]any{},
	}}
	app3 := manifestOnly.ToApp()
	if app3.EntryRoot() != nil {
		t.Error("bundle without scenes must have no entry root")
	}
	if app3.ID != "x" {
		t.Errorf("manifest id lost, got %q", app3.ID)
	}
}

func TestFromAppReflectsRuntimePatches(t *testing.T) {
	app, err := loader.LoadDir(counterDir())
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	b1, err := FromApp(app)
	if err != nil {
		t.Fatalf("FromApp(1): %v", err)
	}
	app.Name = "Patched Name"
	b2, err := FromApp(app)
	if err != nil {
		t.Fatalf("FromApp(2): %v", err)
	}
	if b1.ContentHash == b2.ContentHash {
		t.Error("patching the manifest must change the content hash")
	}
	if got := b2.ToApp().Name; got != "Patched Name" {
		t.Errorf("patched name lost through FromApp -> ToApp, got %q", got)
	}
}

// --- unencodable content surfaces errors instead of panicking ---

func TestUnencodableContentSurfacesErrors(t *testing.T) {
	// json.Marshal cannot encode funcs/chans; every hash-dependent entry point
	// must return that error rather than crash.
	poisoned := []map[string]any{{"type": "app", "id": "x", "poison": func() {}}}
	if _, err := fromDocs(poisoned, nil); err == nil {
		t.Error("fromDocs must fail on non-JSON-encodable content")
	}

	b := &Bundle{Format: Format, Content: Content{
		App: map[string]any{"type": "app", "poison": make(chan int)},
	}}
	if _, err := b.computeHash(); err == nil {
		t.Error("computeHash must fail on non-encodable content")
	}
	if err := b.Sign(ed25519.PrivateKey(make([]byte, ed25519.PrivateKeySize)), "k"); err == nil {
		t.Error("Sign must fail when the content cannot be hashed")
	}
	if err := Verify(b, nil); err == nil {
		t.Error("Verify must fail when the content cannot be hashed")
	}
	if err := b.SetVersion("1.0"); err == nil {
		t.Error("SetVersion must fail when the content cannot be hashed")
	}
	if err := b.SetRequiredCapabilities([]string{"camera"}); err == nil {
		t.Error("SetRequiredCapabilities must fail when the content cannot be hashed")
	}
}

// TestLoadRevocationRejectsNonStringEntries covers the remaining fail-closed
// branches in LoadRevocation's decode-into-any logic: a bare top-level array
// or a {"revoked":[...]} object whose ENTRIES are not key-id strings (number,
// null, boolean, nested value) must be rejected with a *VerifyError naming
// the offending index, and top-level scalar JSON (123, true, a bare string)
// must hit the default rejection. In every case the returned list must be
// nil, never a partial or empty list that would silently un-revoke keys.
func TestLoadRevocationRejectsNonStringEntries(t *testing.T) {
	cases := []struct {
		in   string
		want string // substring of the expected VerifyError.Reason
	}{
		// bare top-level array with a non-string element
		{`[123]`, "revocation list entry 0 is not a key id string"},
		{`[null]`, "revocation list entry 0 is not a key id string"},
		{`["ok", true]`, "revocation list entry 1 is not a key id string"},
		{`[["nested"]]`, "revocation list entry 0 is not a key id string"},
		// object form with a non-string element in "revoked"
		{`{"revoked":[123]}`, `"revoked" entry 0 is not a key id string`},
		{`{"revoked":[null]}`, `"revoked" entry 0 is not a key id string`},
		{`{"revoked":["a", {}]}`, `"revoked" entry 1 is not a key id string`},
		// "revoked" present but not an array at all
		{`{"revoked":"a"}`, `"revoked" must be an array of key id strings`},
		// top-level scalar JSON: parseable, but neither array nor object
		{`123`, "revocation list must be a JSON array of key ids"},
		{`true`, "revocation list must be a JSON array of key ids"},
		{`"a bare string"`, "revocation list must be a JSON array of key ids"},
	}
	for _, c := range cases {
		rl, err := LoadRevocation([]byte(c.in))
		if err == nil {
			t.Errorf("LoadRevocation(%s) must fail closed, got %v", c.in, rl)
			continue
		}
		ve, ok := err.(*VerifyError)
		if !ok {
			t.Errorf("%s: want *VerifyError so callers can surface it, got %T (%v)", c.in, err, err)
			continue
		}
		if !strings.Contains(ve.Reason, c.want) {
			t.Errorf("%s: Reason %q must contain %q", c.in, ve.Reason, c.want)
		}
		if rl != nil {
			t.Errorf("%s: rejected input must yield a nil list, got %v", c.in, rl)
		}
	}
}
