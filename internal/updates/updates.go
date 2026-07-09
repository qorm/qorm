// Package updates is the publish side of OTA: an HTTP server that hands each
// client the bundle it should run, honoring a staged (canary) rollout. Clients
// point their runtime's `POST /update` at `/resolve?app=<id>&client=<id>` and
// verify the signature themselves (see package bundle/ota).
package updates

import (
	"encoding/json"
	"hash/fnv"
	"net/http"
	"os"
	"path/filepath"

	"github.com/qorm/qorm/internal/bundle"
)

// Rollout describes which bundle an app's clients receive. A deterministic
// percentage (by client id) get Canary; the rest get Stable.
type Rollout struct {
	Stable        string `json:"stable"`
	Canary        string `json:"canary,omitempty"`
	CanaryPercent int    `json:"canaryPercent,omitempty"`
}

// Server serves bundles from a directory according to per-app rollouts loaded
// from <dir>/rollout.json.
type Server struct {
	dir      string
	rollouts map[string]Rollout
}

// New loads the rollout config (if present) from dir/rollout.json.
func New(dir string) (*Server, error) {
	s := &Server{dir: dir, rollouts: map[string]Rollout{}}
	if data, err := os.ReadFile(filepath.Join(dir, "rollout.json")); err == nil {
		if err := json.Unmarshal(data, &s.rollouts); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Handler returns the HTTP routes: /resolve (staged selection) and /bundles/.
//
// All routes answer with open CORS (Access-Control-Allow-Origin: * plus an
// OPTIONS preflight): packaged app shells always call us cross-origin
// (qormapp:// on iOS, appassets.androidplatform.net on Android, the PWA's own
// domain on web). This is safe to open wide — bundles are public, unauthenticated
// artifacts whose trust comes from the client-side ed25519 verification
// (ota.FetchVerified), not from who may read them, so there is no confidential
// surface to leak and nothing an attacker gains by fetching them.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/resolve", s.resolve)
	mux.Handle("/bundles/", http.StripPrefix("/bundles/", http.FileServer(http.Dir(s.dir))))
	return allowAnyOrigin(mux)
}

// allowAnyOrigin adds the CORS headers packaged shells need and short-circuits
// OPTIONS preflights. See Handler for why "*" is the right policy here.
func allowAnyOrigin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		if r.Method == http.MethodOptions {
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Bucket maps a client id to a stable 0–99 bucket.
func Bucket(clientID string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(clientID))
	return int(h.Sum32() % 100)
}

// Resolve returns the bundle filename an app's client should run.
func (s *Server) Resolve(app, clientID string) (string, bool) {
	r, ok := s.rollouts[app]
	if !ok {
		return "", false
	}
	if r.Canary != "" && Bucket(clientID) < r.CanaryPercent {
		return r.Canary, true
	}
	return r.Stable, true
}

func (s *Server) resolve(w http.ResponseWriter, req *http.Request) {
	app := req.URL.Query().Get("app")
	client := req.URL.Query().Get("client")
	name, ok := s.Resolve(app, client)
	if !ok || name == "" {
		http.Error(w, "no rollout for app", http.StatusNotFound)
		return
	}
	data, err := os.ReadFile(filepath.Join(s.dir, filepath.Base(name)))
	if err != nil {
		http.Error(w, "bundle not found", http.StatusNotFound)
		return
	}
	// Serve only well-formed bundles; clients still verify the signature.
	if _, err := bundle.Unmarshal(data); err != nil {
		http.Error(w, "invalid bundle", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write(data)
}
