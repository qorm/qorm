// Package ota fetches a QORM bundle from a remote or local source and returns
// it only if it verifies. It is the transport half of over-the-air UI updates;
// the trust half lives in package bundle. A failed fetch or a failed
// verification returns an error and never yields a bundle — so the caller can
// safely keep running the previous one (rollback by inaction).
package ota

import (
	"crypto/ed25519"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/qorm/qorm/internal/bundle"
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
