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
	"sort"

	"github.com/qorm/platform/internal/loader"
	"github.com/qorm/platform/internal/model"
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
	// Components are standalone type:"component" definition documents
	// (components/<name>.json), by component name. Components declared inline
	// in qorm.json travel inside App and never appear here; omitempty keeps
	// the canonical encoding — and therefore the content hash — of every
	// bundle without cross-file components unchanged.
	Components map[string]map[string]any `json:"components,omitempty"`
	// Stylesheets are standalone type:"stylesheet" documents (styles/<id>.qss),
	// by sheet id. omitempty keeps the canonical encoding — and therefore the
	// content hash — of every bundle without stylesheets unchanged.
	Stylesheets map[string]map[string]any `json:"stylesheets,omitempty"`
	// ScriptLib is the shared qscript library document (actions/lib.qs,
	// type:"scriptlib") — the fn definitions merged into every script action
	// at dispatch. omitempty keeps the canonical encoding — and therefore the
	// content hash — of every bundle without a library unchanged.
	ScriptLib map[string]any `json:"scriptLib,omitempty"`
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
//
// A duplicate definition is REFUSED rather than resolved. Assigning into these
// maps silently kept the last document seen, while the loader (`qorm run`, the
// playground, CI rendering) keeps the first — so an attacker who added one
// component document with a filename sorting after the original got the benign
// version rendered everywhere a human or a check looked, and the malicious one
// signed into the shipped bundle. Signing was never broken; what was signed
// simply was not what was reviewed. There is no correct guess to make here, so
// the build stops and names the id.
//
// Ids are coerced through loader.DocID, the same coercion the loader applies:
// a document with a non-string id ({"id": 1}) used to key the loader's app
// under "1" and this map under "", quietly dropping it from the package.
func fromDocs(docs []map[string]any, locales map[string]map[string]string) (*Bundle, error) {
	c := Content{Scenes: map[string]map[string]any{}, Actions: map[string]map[string]any{}, Locales: locales}
	for _, doc := range docs {
		id := loader.DocID(doc)
		switch loader.DocType(doc) {
		case "app":
			if c.App != nil {
				return nil, fmt.Errorf(`more than one type:"app" manifest document in this app (the second has id %q): remove the extra manifest — the directory loader keeps the first and packaging would keep the last`, id)
			}
			c.App = doc
		case "scene":
			if _, dup := c.Scenes[id]; dup {
				return nil, duplicateDocError("scene", id)
			}
			c.Scenes[id] = doc
		case "action":
			if _, dup := c.Actions[id]; dup {
				return nil, duplicateDocError("action", id)
			}
			c.Actions[id] = doc
		case "component":
			if _, dup := c.Components[id]; dup {
				return nil, duplicateDocError("component", id)
			}
			if c.Components == nil {
				c.Components = map[string]map[string]any{}
			}
			c.Components[id] = doc
		case "stylesheet":
			if _, dup := c.Stylesheets[id]; dup {
				return nil, duplicateDocError("stylesheet", id)
			}
			if c.Stylesheets == nil {
				c.Stylesheets = map[string]map[string]any{}
			}
			c.Stylesheets[id] = doc
		case "scriptlib":
			// The shared qscript library (actions/lib.qs): one per app. The
			// directory loader diagnoses a second definition and keeps the
			// first; packaging refuses outright, same rule as every other
			// duplicated definition.
			if c.ScriptLib != nil {
				return nil, duplicateDocError("scriptlib", "lib")
			}
			c.ScriptLib = doc
		}
	}
	// A component document colliding with a manifest-INLINE component of the
	// same name is the fourth shape of the same ambiguity. Both compile paths
	// happen to resolve it the same way (the manifest is applied first and
	// wins), but "happens to" is not a property anyone can rely on while
	// reading the sources: two definitions are visible and only one renders.
	// The directory load calls it an error, so packaging does too. Checked
	// after the loop because the manifest may be walked last.
	if inline, ok := c.App["components"].(map[string]any); ok {
		shadowed := make([]string, 0, len(c.Components))
		for name := range c.Components {
			if _, dup := inline[name]; dup {
				shadowed = append(shadowed, name)
			}
		}
		if len(shadowed) > 0 {
			sort.Strings(shadowed) // deterministic message
			return nil, duplicateDocError("component", shadowed[0])
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

// duplicateDocError reports two source documents competing for one id.
func duplicateDocError(kind, id string) error {
	return fmt.Errorf("two %s documents define the id %q: the directory loader would keep the first and packaging the last, so the bundle would not contain the app you rendered and reviewed — remove or rename one of them", kind, id)
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
		reason := fmt.Sprintf("content hash mismatch (tampered): have %s, want %s", b.ContentHash, want)
		// A mismatch on a versioned bundle is usually NOT tampering: an OLD
		// qorm meeting a NEWER bundle's field set re-marshals the same content
		// differently (a field it does not know gets echoed, reordered, or
		// dropped), so the hash derives from different bytes. Point that out
		// instead of crying foul — verification still fails closed either way.
		if v := b.Version(); v != "" {
			reason += fmt.Sprintf(" (bundle declares version %q — this qorm may predate its field set; upgrade qorm or rebuild the bundle)", v)
		}
		return &VerifyError{Reason: reason}
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
// array `["keyid", ...]` or an object `{"revoked": ["keyid", ...]}`. Extra object
// keys alongside "revoked" are ignored.
//
// It fails CLOSED: JSON null, any non-object/non-array document, an object with
// no "revoked" key, and a "revoked" value that is null or not an array of
// strings are all rejected with a *VerifyError rather than parsed as "nobody is
// revoked". A misconfigured or hijacked revocation endpoint serving `null` must
// not be able to silently disable the leaked-key defence. The well-formed empty
// lists `[]` and `{"revoked":[]}` remain valid — a legitimate "nothing revoked".
func LoadRevocation(data []byte) (RevocationList, error) {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, &VerifyError{Reason: fmt.Sprintf("revocation list is not valid JSON: %v", err)}
	}
	var ids []string
	switch tv := v.(type) {
	case []any:
		ids = make([]string, 0, len(tv))
		for i, e := range tv {
			id, ok := e.(string)
			if !ok {
				return nil, &VerifyError{Reason: fmt.Sprintf("revocation list entry %d is not a key id string", i)}
			}
			ids = append(ids, id)
		}
	case map[string]any:
		revoked, present := tv["revoked"]
		if !present {
			return nil, &VerifyError{Reason: `revocation object has no "revoked" array; refusing to treat it as an empty list`}
		}
		arr, ok := revoked.([]any) // JSON null decodes to a nil any, so null is rejected here too
		if !ok {
			return nil, &VerifyError{Reason: `"revoked" must be an array of key id strings`}
		}
		ids = make([]string, 0, len(arr))
		for i, e := range arr {
			id, ok := e.(string)
			if !ok {
				return nil, &VerifyError{Reason: fmt.Sprintf(`"revoked" entry %d is not a key id string`, i)}
			}
			ids = append(ids, id)
		}
	default: // includes JSON null, strings, numbers, booleans
		return nil, &VerifyError{Reason: `revocation list must be a JSON array of key ids or {"revoked":[...]}; null and other JSON types are rejected`}
	}
	rl := make(RevocationList, len(ids))
	for _, id := range ids {
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
	docs := make([]map[string]any, 0, len(b.Content.Scenes)+len(b.Content.Actions)+len(b.Content.Components)+1)
	if b.Content.App != nil {
		docs = append(docs, b.Content.App)
	}
	for _, s := range b.Content.Scenes {
		docs = append(docs, s)
	}
	for _, a := range b.Content.Actions {
		docs = append(docs, a)
	}
	for _, c := range b.Content.Components {
		docs = append(docs, c)
	}
	if b.Content.ScriptLib != nil {
		docs = append(docs, b.Content.ScriptLib)
	}
	// Stylesheet ORDER is semantic (same-class rules cascade in declaration
	// order), and this map iterates nondeterministically — feed the documents
	// in sorted-id order, the same order the collect walk and the serializer
	// produce, so a bundle reconstructs the exact cascade the directory had.
	sheetIDs := make([]string, 0, len(b.Content.Stylesheets))
	for id := range b.Content.Stylesheets {
		sheetIDs = append(sheetIDs, id)
	}
	sort.Strings(sheetIDs)
	for _, id := range sheetIDs {
		docs = append(docs, b.Content.Stylesheets[id])
	}
	app := loader.FromDocs(docs)
	app.Locales = b.Content.Locales
	if len(b.Content.RequiredCapabilities) > 0 {
		app.RequiredCapabilities = append([]string(nil), b.Content.RequiredCapabilities...)
	}
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
