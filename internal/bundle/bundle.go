// Package bundle compiles a QORM app into a single, content-addressed,
// optionally signed artifact and verifies it before execution. This is the
// trust primitive behind safe over-the-air UI delivery: the runtime verifies
// the bundle (hash + ed25519 signature) rather than trusting a server.
//
// Everything here is pure Go (crypto/ed25519, crypto/sha256), so signing and
// verification cross-compile to every platform with no C toolchain.
package bundle

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/qorm/qorm/internal/loader"
	"github.com/qorm/qorm/internal/model"
)

// Format is the bundle format identifier.
const Format = "qorm-bundle/1"

// Bundle is a compiled, content-addressed QORM application.
type Bundle struct {
	Format      string     `json:"format"`
	Content     Content    `json:"content"`
	ContentHash string     `json:"contentHash"`
	Signature   *Signature `json:"signature,omitempty"`
}

// Content is the canonical payload that the hash and signature cover.
type Content struct {
	App     map[string]any               `json:"app"`
	Scenes  map[string]map[string]any    `json:"scenes"`
	Actions map[string]map[string]any    `json:"actions"`
	Locales map[string]map[string]string `json:"locales,omitempty"`
	// RequiredCapabilities lists the hardware/native capabilities (by canonical
	// capability name, e.g. "camera") the app needs at runtime. The runtime
	// refuses to start the bundle on a platform missing any of them. omitempty
	// keeps the canonical encoding — and therefore the content hash — of older
	// bundles unchanged.
	RequiredCapabilities []string `json:"requiredCapabilities,omitempty"`
}

// Signature is a detached ed25519 signature over the content hash.
type Signature struct {
	Algorithm string `json:"algorithm"`
	KeyID     string `json:"keyId"`
	Value     string `json:"value"` // base64(signature bytes)
}

// Build compiles the app in dir into an unsigned bundle.
func Build(dir string) (*Bundle, error) {
	docs, err := loader.CollectDocs(dir)
	if err != nil {
		return nil, err
	}
	if len(docs) == 0 {
		return nil, fmt.Errorf("no QORM source documents found under %s", dir)
	}
	return fromDocs(docs, loader.LoadLocales(dir))
}

// FromApp compiles a live (possibly patched) app into an unsigned bundle, so a
// design produced via the agent surface can be exported and shipped.
func FromApp(app *model.App) (*Bundle, error) {
	return fromDocs(loader.AppToDocs(app), app.Locales)
}

// fromDocs splits raw documents into content (with i18n catalogs) and hashes it.
func fromDocs(docs []map[string]any, locales map[string]map[string]string) (*Bundle, error) {
	c := Content{Scenes: map[string]map[string]any{}, Actions: map[string]map[string]any{}, Locales: locales}
	for _, doc := range docs {
		id, _ := doc["id"].(string)
		switch doc["type"] {
		case "app":
			c.App = doc
		case "scene":
			c.Scenes[id] = doc
		case "action":
			c.Actions[id] = doc
		}
	}
	b := &Bundle{Format: Format, Content: c}
	hash, err := b.computeHash()
	if err != nil {
		return nil, err
	}
	b.ContentHash = hash
	return b, nil
}

// computeHash returns the sha256 of the canonical content encoding.
// Go's encoding/json sorts map keys, giving a deterministic serialization.
func (b *Bundle) computeHash() (string, error) {
	data, err := json.Marshal(b.Content)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return "sha256:" + base64.StdEncoding.EncodeToString(sum[:]), nil
}

// Sign attaches an ed25519 signature over the content hash.
func (b *Bundle) Sign(priv ed25519.PrivateKey, keyID string) error {
	hash, err := b.computeHash()
	if err != nil {
		return err
	}
	b.ContentHash = hash
	sig := ed25519.Sign(priv, []byte(hash))
	b.Signature = &Signature{
		Algorithm: "ed25519",
		KeyID:     keyID,
		Value:     base64.StdEncoding.EncodeToString(sig),
	}
	return nil
}

// VerifyError describes why verification failed.
type VerifyError struct{ Reason string }

func (e *VerifyError) Error() string { return "bundle verification failed: " + e.Reason }

// Verify recomputes the hash and, if a trusted public key is supplied, checks
// the signature. A nil trust key verifies integrity only (tamper detection),
// not authenticity. Returns nil when the bundle is safe to run.
func Verify(b *Bundle, trust ed25519.PublicKey) error {
	want, err := b.computeHash()
	if err != nil {
		return err
	}
	if b.ContentHash != want {
		return &VerifyError{Reason: fmt.Sprintf("content hash mismatch (tampered): have %s, want %s", b.ContentHash, want)}
	}
	if trust == nil {
		return nil // integrity verified; authenticity not requested
	}
	if b.Signature == nil {
		return &VerifyError{Reason: "no signature present but a trusted key was required"}
	}
	if b.Signature.Algorithm != "ed25519" {
		return &VerifyError{Reason: "unsupported signature algorithm " + b.Signature.Algorithm}
	}
	sig, err := base64.StdEncoding.DecodeString(b.Signature.Value)
	if err != nil {
		return &VerifyError{Reason: "malformed signature encoding"}
	}
	if !ed25519.Verify(trust, []byte(b.ContentHash), sig) {
		return &VerifyError{Reason: "signature does not match trusted key"}
	}
	return nil
}

// SetVersion stamps a version into the manifest and recomputes the content
// hash. Call before Sign.
func (b *Bundle) SetVersion(version string) error {
	if b.Content.App == nil {
		b.Content.App = map[string]any{"type": "app"}
	}
	b.Content.App["version"] = version
	hash, err := b.computeHash()
	if err != nil {
		return err
	}
	b.ContentHash = hash
	return nil
}

// SetRequiredCapabilities stamps the capability requirements into the content
// and recomputes the content hash, so they are covered by the hash and any
// subsequent signature. Call before Sign. A nil/empty list clears the field.
func (b *Bundle) SetRequiredCapabilities(caps []string) error {
	if len(caps) == 0 {
		b.Content.RequiredCapabilities = nil
	} else {
		b.Content.RequiredCapabilities = caps
	}
	hash, err := b.computeHash()
	if err != nil {
		return err
	}
	b.ContentHash = hash
	return nil
}

// RequiredCapabilities returns the capability requirements declared in the
// content (nil for bundles built before the field existed).
func (b *Bundle) RequiredCapabilities() []string {
	return b.Content.RequiredCapabilities
}

// Version returns the app version carried in the manifest (or "" if unset).
// Because it lives inside the manifest, it is covered by the content hash and
// signature.
func (b *Bundle) Version() string {
	if b.Content.App == nil {
		return ""
	}
	v, _ := b.Content.App["version"].(string)
	return v
}

// RevocationList is the set of revoked signing key ids. A bundle signed by a
// revoked key is refused even if its signature is otherwise valid — the defense
// against a leaked signing key.
type RevocationList map[string]bool

// LoadRevocation reads a revocation list from a JSON file. Accepts either a bare
// array `["keyid", ...]` or an object `{"revoked": ["keyid", ...]}`.
func LoadRevocation(data []byte) (RevocationList, error) {
	var arr []string
	if err := json.Unmarshal(data, &arr); err != nil {
		var obj struct {
			Revoked []string `json:"revoked"`
		}
		if err2 := json.Unmarshal(data, &obj); err2 != nil {
			return nil, fmt.Errorf("revocation list must be a JSON array or {revoked:[...]}: %w", err)
		}
		arr = obj.Revoked
	}
	rl := make(RevocationList, len(arr))
	for _, id := range arr {
		rl[id] = true
	}
	return rl, nil
}

// VerifyWithRevocation verifies integrity + signature, then rejects the bundle
// if it was signed by a revoked key.
func VerifyWithRevocation(b *Bundle, trust ed25519.PublicKey, revoked RevocationList) error {
	if err := Verify(b, trust); err != nil {
		return err
	}
	// Key revocation must be checked against the ACTUAL verifying key, not the
	// self-declared (unsigned) Signature.KeyID field — the signature only covers
	// ContentHash, so a revoked-key holder could otherwise rewrite keyId to any
	// non-revoked string and evade revocation while the signature still verifies.
	if trust != nil && len(revoked) > 0 {
		id := base64.RawStdEncoding.EncodeToString(trust)[:12]
		if revoked[id] {
			return &VerifyError{Reason: "signing key " + id + " is revoked"}
		}
	}
	return nil
}

// ToApp reconstructs a runnable model.App from the bundle content.
func (b *Bundle) ToApp() *model.App {
	docs := make([]map[string]any, 0, len(b.Content.Scenes)+len(b.Content.Actions)+1)
	if b.Content.App != nil {
		docs = append(docs, b.Content.App)
	}
	for _, s := range b.Content.Scenes {
		docs = append(docs, s)
	}
	for _, a := range b.Content.Actions {
		docs = append(docs, a)
	}
	app := loader.FromDocs(docs)
	app.Locales = b.Content.Locales
	return app
}

// Marshal encodes the bundle as indented JSON.
func Marshal(b *Bundle) ([]byte, error) { return json.MarshalIndent(b, "", "  ") }

// Unmarshal decodes a bundle from JSON.
func Unmarshal(data []byte) (*Bundle, error) {
	var b Bundle
	if err := json.Unmarshal(data, &b); err != nil {
		return nil, err
	}
	if b.Format != Format {
		return nil, fmt.Errorf("unsupported bundle format %q", b.Format)
	}
	return &b, nil
}
