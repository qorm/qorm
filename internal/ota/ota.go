// Package ota fetches a QORM bundle from a remote or local source and returns
// it only if it verifies. It is the transport half of over-the-air UI updates;
// the trust half lives in package bundle. A failed fetch or a failed
// verification returns an error and never yields a bundle — so the caller can
// safely keep running the previous one (rollback by inaction).
package ota

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/qorm/platform/internal/bundle"
)

// maxBundle is the largest accepted bundle payload: 32 MiB. Anything bigger
// is a hard error (never a silent truncation) — the caller treats a failed
// fetch as "no update" and keeps running the current bundle.
const maxBundle = 32 << 20

// maxRedirects caps the redirect chain a BlockPrivate fetch will follow
// (the default Go client would follow 10 with no per-hop scrutiny).
const maxRedirects = 5

// Option adjusts how Fetch reaches the network. With no options the historical
// behavior is kept: any reachable destination, default redirect policy.
type Option func(*fetchOpts)

type fetchOpts struct {
	blockPrivate bool
}

// BlockPrivate hardens http(s) fetches against SSRF: dials to private
// (RFC 1918 / RFC 4193), link-local (which covers the 169.254.169.254 and
// fe80:: cloud metadata addresses), multicast and unspecified destinations are
// refused, and redirects are capped at maxRedirects hops with every hop
// re-vetted. The decisive check runs inside the dialer, AFTER DNS resolution,
// so a hostname that resolves (or re-resolves — DNS rebinding) to a private
// address is refused too; the redirect hook additionally rejects literal-IP
// hops early for a clearer error.
//
// Threat model: POST /update lets any same-origin caller hand the server a
// URL to fetch. Unvetted, that turns the server into a proxy into networks
// only IT can reach — the cloud metadata service, LAN-internal admin
// endpoints. Loopback destinations stay ALLOWED on purpose: local bundle
// servers (httptest, a `qorm` updates server on localhost) are the normal
// development workflow, and anything listening on this host's loopback is
// already reachable by every local process directly, so refusing it would
// break dev without shrinking the surface this option is about (hosts only
// the server can see). Local file paths are likewise untouched — they never
// leave the machine.
//
// The dial-time check is enforced by the OS dialer and therefore does not
// apply on js/wasm, where the browser's fetch (and its same-origin policy)
// performs the transport instead.
func BlockPrivate() Option {
	return func(o *fetchOpts) { o.blockPrivate = true }
}

// Fetch retrieves raw bundle bytes from an http(s) URL or a local file path.
// Both paths enforce the maxBundle size cap: an oversize payload is an error.
func Fetch(source string, opts ...Option) ([]byte, error) {
	var cfg fetchOpts
	for _, o := range opts {
		o(&cfg)
	}
	if strings.HasPrefix(source, "http://") || strings.HasPrefix(source, "https://") {
		resp, err := newClient(cfg).Get(source)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("fetch %s: status %d", source, resp.StatusCode)
		}
		return readCapped(resp.Body)
	}
	f, err := os.Open(source)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return readCapped(f)
}

// newClient builds the HTTP client for one fetch. Without BlockPrivate it is
// the plain 30s-timeout client; with it, both the dialer and the redirect
// policy enforce the SSRF guard described on BlockPrivate.
func newClient(cfg fetchOpts) *http.Client {
	client := &http.Client{Timeout: 30 * time.Second}
	if !cfg.blockPrivate {
		return client
	}
	dialer := &net.Dialer{
		// Control runs for every connection attempt with the concrete
		// post-DNS-resolution address, so it is the check redirects and DNS
		// rebinding cannot route around.
		Control: func(network, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return fmt.Errorf("ota: unparseable dial target %q: %w", address, err)
			}
			ip := net.ParseIP(host)
			if ip == nil {
				return fmt.Errorf("ota: refusing non-IP dial target %q", host)
			}
			return checkPublicIP(ip)
		},
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DialContext = dialer.DialContext
	client.Transport = transport
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= maxRedirects {
			return fmt.Errorf("ota: stopped after %d redirects", maxRedirects)
		}
		return checkRedirectTarget(req.URL)
	}
	return client
}

// checkRedirectTarget vets one redirect hop: only http(s) schemes, and a
// literal-IP host must pass checkPublicIP right away (a clearer error than
// failing inside the dialer). Hostname targets are not resolved here — the
// dialer's Control hook re-checks them after DNS resolution.
func checkRedirectTarget(u *url.URL) error {
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("ota: refusing redirect to non-http scheme %q", u.Scheme)
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		return checkPublicIP(ip)
	}
	return nil
}

// checkPublicIP refuses private (RFC 1918 / RFC 4193), link-local (including
// 169.254.169.254 and fe80:: metadata addresses), multicast and unspecified
// destinations, IPv4 and IPv6 alike (IPv4-mapped IPv6 included — the net.IP
// predicates unmap it). Loopback is allowed; see BlockPrivate for why.
func checkPublicIP(ip net.IP) error {
	switch {
	case ip.IsPrivate(), ip.IsLinkLocalUnicast(), ip.IsLinkLocalMulticast(),
		ip.IsInterfaceLocalMulticast(), ip.IsMulticast(), ip.IsUnspecified():
		return fmt.Errorf("ota: refusing to fetch from private or link-local address %s", ip)
	}
	return nil
}

// readCapped reads up to maxBundle bytes, erroring when the source holds more
// — so a truncated payload can never slip through to the verifier as if it
// were the whole bundle.
func readCapped(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxBundle+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBundle {
		return nil, fmt.Errorf("bundle exceeds the %d MiB size cap", maxBundle>>20)
	}
	return data, nil
}

// ClientConfig is a packaged app's client-side OTA configuration: the
// window.__QORM_UPDATE__ document that server.OfflineHTML injects into the
// offline HTML. URL + App locate the update server; Trust is the ed25519
// public key every OTA bundle must be signed with; Revoked is the OPTIONAL
// key-id revocation snapshot the package ships with.
//
// A shipped package should carry a revocation snapshot (the freshly-built
// revocation list from `qorm sign`/release tooling): OTA can only refuse
// bundles signed by a key the CLIENT knows about — once a key is revoked
// AFTER the package shipped, the next release (or store re-signing) is the
// only channel that reaches a stopped-updating device.
type ClientConfig struct {
	URL     string
	App     string
	Trust   ed25519.PublicKey
	Revoked bundle.RevocationList
}

// ParseClientConfig parses the JSON form of window.__QORM_UPDATE__ into a
// ClientConfig. The trust key is REQUIRED — without it OTA stays off, the
// same fail-closed model as the live server's /update. `revoked` is optional
// (absent or null means "nothing revoked"); when present it must be a valid
// revocation list (see bundle.LoadRevocation) — a malformed one is an error,
// never silently empty.
func ParseClientConfig(data []byte) (*ClientConfig, error) {
	var doc struct {
		URL     string          `json:"url"`
		App     string          `json:"app"`
		Trust   string          `json:"trust"`
		Revoked json.RawMessage `json:"revoked"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("update config is not valid JSON: %v", err)
	}
	cfg := &ClientConfig{URL: strings.TrimRight(doc.URL, "/"), App: doc.App}
	if cfg.URL == "" || cfg.App == "" {
		return nil, fmt.Errorf("update config needs url and app")
	}
	raw, err := base64.StdEncoding.DecodeString(doc.Trust)
	if err != nil || len(raw) != ed25519.PublicKeySize {
		return nil, fmt.Errorf("update config has no valid trust key (base64 ed25519 public key) — OTA stays off")
	}
	cfg.Trust = ed25519.PublicKey(raw)
	if len(doc.Revoked) > 0 && string(doc.Revoked) != "null" {
		if cfg.Revoked, err = bundle.LoadRevocation(doc.Revoked); err != nil {
			return nil, err
		}
	}
	return cfg, nil
}

// VerifyBundle verifies an OTA-origin bundle the way the live server's
// /update does: integrity + signature against Trust, then rejection of any
// revoked signing key. A nil config (no update server declared) or a missing
// trust key fails closed — an unverifiable bundle is never activated.
func (c *ClientConfig) VerifyBundle(b *bundle.Bundle) error {
	if c == nil || len(c.Trust) == 0 {
		return fmt.Errorf("update config has no valid trust key — OTA bundles cannot be verified")
	}
	return bundle.VerifyWithRevocation(b, c.Trust, c.Revoked)
}

// FetchVerified fetches, decodes and verifies a bundle in one step, rejecting
// revoked signing keys. The returned bundle is safe to activate. Any error
// means the update should be rejected and the current bundle kept.
func FetchVerified(source string, trust ed25519.PublicKey, revoked bundle.RevocationList, opts ...Option) (*bundle.Bundle, error) {
	data, err := Fetch(source, opts...)
	if err != nil {
		return nil, fmt.Errorf("fetch: %w", err)
	}
	b, err := bundle.Unmarshal(data)
	if err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	if err := bundle.VerifyWithRevocation(b, trust, revoked); err != nil {
		return nil, err
	}
	return b, nil
}
