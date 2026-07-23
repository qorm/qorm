package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/qorm/qorm/internal/keys"
)

type githubRelease struct {
	TagName string `json:"tag_name"`
	Assets  []struct {
		Name        string `json:"name"`
		DownloadURL string `json:"browser_download_url"`
	} `json:"assets"`
}

// releaseAPIURL serves the latest-release metadata (GitHub API JSON shape).
// It is a package variable so loopback tests can point it at a local server.
var releaseAPIURL = "https://api.github.com/repos/qorm/qorm/releases/latest"

// errGoInstallUnavailable marks a go-install phase that was skipped because no
// Go toolchain is on PATH: a silent fallback to the signed-binary download,
// distinct from a phase that ran and failed (which warns before falling back).
var errGoInstallUnavailable = errors.New("go toolchain not available")

// goInstallPhase runs the 'go install ...@latest' fast path that precedes the
// signed-binary download and reports its outcome: nil means the install
// command succeeded (the caller STILL verifies the installed version before
// reporting success), errGoInstallUnavailable means the phase was skipped, and
// any other error means it ran and failed. Loopback tests override it so the
// update flow stays deterministic and offline: returning nil exercises the
// post-install version check without a network or toolchain, and returning
// errGoInstallUnavailable keeps the signed-binary flow on its own path.
var goInstallPhase = defaultGoInstallPhase

func defaultGoInstallPhase() error {
	if _, err := exec.LookPath("go"); err != nil {
		return errGoInstallUnavailable
	}
	fmt.Println("Updating via 'go install'...")
	cmd := exec.Command("go", "install", "github.com/qorm/qorm/cmd/qorm@latest")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// selfExe resolves the executable a self-update replaces. Loopback tests
// override it so they never touch the running test binary.
var selfExe = os.Executable

// goInstallBinDir resolves the directory `go install` writes executables to:
// `go env GOBIN` when set, otherwise the bin directory of the first `go env
// GOPATH` entry. It is a package variable so loopback tests can point it at a
// temp directory without invoking the real Go toolchain.
var goInstallBinDir = defaultGoInstallBinDir

func defaultGoInstallBinDir() (string, error) {
	if gobin := strings.TrimSpace(runGoEnv("GOBIN")); gobin != "" {
		return gobin, nil
	}
	gopath := strings.TrimSpace(runGoEnv("GOPATH"))
	first, _, _ := strings.Cut(gopath, string(os.PathListSeparator)) // GOPATH may be a list; `go install` uses the first entry
	if first == "" {
		return "", errors.New("neither GOBIN nor GOPATH resolves to a directory (is the Go toolchain on PATH?)")
	}
	return filepath.Join(first, "bin"), nil
}

// runGoEnv returns `go env <key>`'s output, or "" when go is unavailable or
// the command fails.
func runGoEnv(key string) string {
	out, err := exec.Command("go", "env", key).Output()
	if err != nil {
		return ""
	}
	return string(out)
}

// qormBinaryName is the platform file name `go install` writes for this
// command (qorm.exe on Windows).
func qormBinaryName() string {
	if runtime.GOOS == "windows" {
		return "qorm.exe"
	}
	return "qorm"
}

// cmdUpdate checks for QORM CLI updates on GitHub and performs a self-update.
// Downloaded binaries are verified against the release's ed25519-signed
// SHA256SUMS manifest before they replace the current executable, unless
// --insecure-skip-verify is given. Both install paths are additionally
// version-checked against the release the version gate approved: the 'go
// install' fast path performs no signature verification of its own, so after
// it runs the freshly installed binary (at the GOBIN/GOPATH-bin destination or
// the running binary's location) must report a version at least as new as the
// approved target; the signed-binary path execs the downloaded binary BEFORE
// swapping it in and refuses the swap unless it reports the approved version
// (the signature proves the file was signed, not that it is the release its
// tag claims). Either check failing refuses the update, fail-closed.
func cmdUpdate(args []string) int {
	skipVerify := false
	for _, a := range args {
		switch a {
		case "--insecure-skip-verify":
			skipVerify = true
		default:
			fmt.Fprintf(os.Stderr, "error: unknown flag %q\nusage: qorm update [--insecure-skip-verify]\n", a)
			return 2
		}
	}

	fmt.Println("Checking for updates...")

	req, err := http.NewRequest("GET", releaseAPIURL, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create request: %v\n", err)
		return 1
	}
	req.Header.Set("User-Agent", "qorm-cli-updater")

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to check updates: %v\n", err)
		return 1
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: failed to check updates, GitHub returned HTTP %d\n", resp.StatusCode)
		return 1
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to parse update response: %v\n", err)
		return 1
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(version, "v")

	// Version gate: install only a STRICTLY NEWER release. The remote tag is
	// the same field that has always fed latestVersion (the release's
	// tag_name; the signed SHA256SUMS sidecar carries no version of its own).
	// Comparing by equality only would let a compromised or misconfigured
	// release endpoint serve an OLDER signed release and have it installed as
	// a "self-update" — a rollback to a potentially vulnerable build. Equal,
	// older, prerelease-of-current, and unparseable tags are all refused
	// without replacing the binary.
	cmp, err := compareSemver(release.TagName, version)
	if err != nil {
		fmt.Fprintf(os.Stderr, "warn: cannot compare remote version %q with the running version %q (%v); refusing to self-update.\n", release.TagName, version, err)
		return 0
	}
	if cmp == 0 {
		fmt.Printf("QORM is already up to date (version %s)\n", version)
		return 0
	}
	if cmp < 0 {
		fmt.Printf("QORM is up to date: the latest release (v%s) is older than the running v%s; refusing to downgrade.\n", latestVersion, currentVersion)
		return 0
	}

	fmt.Printf("A new version of QORM is available: v%s (current: v%s)\n", latestVersion, currentVersion)

	// Phase 1: Try using 'go install' if Go toolchain is locally available
	if err := goInstallPhase(); err == nil {
		// The 'go install @latest' phase trusts the Go module proxy, which is
		// never asked to prove what it served: a compromised (or stale) proxy
		// could resolve @latest to a module OLDER than the strictly-newer
		// signed release the version gate above approved. Defense in depth:
		// run the freshly installed binary's `version` command and refuse to
		// report success unless it reports >= the approved target version.
		// verifyInstalledVersion checks the resolved GOBIN/GOPATH-bin install
		// location AND the running binary's location (selfExe), since `go
		// install` may write somewhere other than selfExe.
		if err := verifyInstalledVersion(latestVersion); err != nil {
			fmt.Fprintln(os.Stderr, "WARNING: 'go install' exited successfully, but the installed binary fails the post-install version check — NOT reporting a successful update.")
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			return 1
		}
		fmt.Println("QORM updated successfully via go install!")
		return 0
	} else if !errors.Is(err, errGoInstallUnavailable) {
		fmt.Println("warn: go install failed, falling back to pre-compiled binary update...")
	}

	// Phase 2: Self-update by downloading the pre-compiled binary asset
	expectedAsset := fmt.Sprintf("qorm-%s-%s", runtime.GOOS, runtime.GOARCH)
	if runtime.GOOS == "windows" {
		expectedAsset += ".exe"
	}

	targetAsset := ""
	targetURL := ""
	for _, asset := range release.Assets {
		if strings.EqualFold(asset.Name, expectedAsset) {
			targetAsset = asset.Name
			targetURL = asset.DownloadURL
			break
		}
	}

	if targetURL == "" {
		fmt.Fprintf(os.Stderr, "error: no pre-compiled binary asset named %q found for %s/%s. Please update manually.\n", expectedAsset, runtime.GOOS, runtime.GOARCH)
		return 1
	}

	fmt.Printf("Downloading pre-compiled binary: %s...\n", targetAsset)

	exePath, err := selfExe()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to resolve executable path: %v\n", err)
		return 1
	}

	respDL, err := client.Get(targetURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: download failed: %v\n", err)
		return 1
	}
	defer respDL.Body.Close()

	if respDL.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "error: download failed, GitHub returned HTTP %d\n", respDL.StatusCode)
		return 1
	}

	tmpDir := filepath.Dir(exePath)
	tmpFile, err := os.CreateTemp(tmpDir, "qorm_download_*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to create temporary file: %v\n", err)
		return 1
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)
	defer tmpFile.Close()

	if _, err := io.Copy(tmpFile, respDL.Body); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to save download contents: %v\n", err)
		return 1
	}
	tmpFile.Close()

	if skipVerify {
		fmt.Fprintln(os.Stderr, "WARNING: --insecure-skip-verify given — installing the downloaded binary WITHOUT sha256/ed25519 verification. Only do this if you trust the network path to GitHub.")
	} else if err := verifyDownloadedBinary(client, release, tmpPath, targetAsset); err != nil {
		fmt.Fprintf(os.Stderr, "error: release verification failed: %v\n", err)
		fmt.Fprintln(os.Stderr, "hint: re-run with --insecure-skip-verify to update without verification (NOT recommended).")
		return 1
	}

	if err := os.Chmod(tmpPath, 0755); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to set executable permissions: %v\n", err)
		return 1
	}

	// Defense in depth for the release pipeline itself: a valid ed25519
	// signature proves the publisher signed THIS file, not that the file is
	// the release its tag claims — a misbuilt or stale binary published under
	// a newer tag would otherwise be swapped over the running binary. Exec the
	// downloaded binary's `version` command BEFORE the swap and refuse unless
	// it reports >= the approved target. Refusing here is cheap precisely
	// because it happens while the current binary is still in place: the swap
	// below cannot be undone, but this check can still veto it.
	if err := verifyPreSwapVersion(tmpPath, latestVersion); err != nil {
		fmt.Fprintln(os.Stderr, "error: refusing to install the downloaded binary — the current binary has NOT been replaced.")
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 1 // the deferred os.Remove(tmpPath) cleans up the refused download
	}

	// Rename current executable to backup
	oldPath := exePath + ".old"
	_ = os.Remove(oldPath)

	if err := os.Rename(exePath, oldPath); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to back up current binary: %v\n", err)
		return 1
	}

	if err := os.Rename(tmpPath, exePath); err != nil {
		fmt.Fprintf(os.Stderr, "error: failed to replace binary: %v (restoring backup...)\n", err)
		_ = os.Rename(oldPath, exePath)
		return 1
	}

	_ = os.Remove(oldPath)

	fmt.Printf("QORM updated successfully to version v%s!\n", latestVersion)
	return 0
}

// installedVersionTimeout bounds the post-install / pre-swap `version` command
// so a wedged replacement binary cannot hang `qorm update` forever.
const installedVersionTimeout = 10 * time.Second

// verifyPreSwapVersion runs the downloaded binary's `version` command at
// tmpPath (already verified and executable on disk) and fails closed unless it
// reports a version >= want — the strictly-newer remote version the version
// gate approved. It guards the signed-binary download path: the ed25519
// signature proves the release publisher signed the file, not that the file is
// the release its tag claims, and once the caller swaps it in the replacement
// cannot be undone. A failure here refuses the swap while the current binary
// is still untouched.
func verifyPreSwapVersion(tmpPath, want string) error {
	got, err := installedBinaryVersion(tmpPath)
	if err != nil {
		return fmt.Errorf("cannot verify the downloaded binary: %v", err)
	}
	cmp, err := compareSemver(got, want)
	if err != nil {
		return fmt.Errorf("downloaded binary reports an unparseable version %q (expected v%s or newer): %v", got, want, err)
	}
	if cmp < 0 {
		return fmt.Errorf("pre-swap version check failed: the downloaded binary reports v%s, which is OLDER than the approved target v%s — the release pipeline published a stale binary under a newer tag; refusing to replace the current binary", got, want)
	}
	return nil
}

// verifyInstalledVersion checks that the freshly installed binary reports a
// version >= want — the strictly-newer remote version the version gate already
// approved — and fails closed otherwise. This is defense in depth for the
// 'go install @latest' fast path: that phase trusts the Go module proxy, which
// (if compromised) could resolve @latest to a module OLDER than the signed
// release the gate checked, and nothing about that phase is signed.
//
// GOBIN reconciliation: `go install` writes to GOBIN (or GOPATH/bin), which
// may differ from the running binary's directory (selfExe) — e.g. the user
// installed via a package manager but has a Go toolchain. Reading only selfExe
// would then see the STALE binary and fail closed on a legitimate install. So
// the check runs against the resolved install location (goInstallBinDir) when
// it exists and differs from selfExe, and succeeds when EITHER location
// reports the expected version; it fails closed only when NEITHER does. When
// the resolved location cannot be checked (go env unavailable, no binary
// there), the check warns and falls back to selfExe instead of hard-failing.
//
// Honest limitation: 'go install' may already have replaced the binary on disk
// by the time this check runs, and that cannot be un-done from here. A failing
// check therefore does NOT roll anything back; it refuses to report success,
// exits non-zero, and warns loudly so the user can reinstall a known-good
// release manually.
func verifyInstalledVersion(want string) error {
	selfPath, selfErr := selfExe()

	// Resolve the `go install` destination and check it first when it holds a
	// binary that is not selfExe itself.
	resolvedPath := ""
	var resolvedFail error
	if dir, err := goInstallBinDir(); err != nil {
		fmt.Fprintf(os.Stderr, "warn: cannot resolve the 'go install' destination (%v); checking the running binary's location instead.\n", err)
	} else {
		candidate := filepath.Join(dir, qormBinaryName())
		switch fi, statErr := os.Stat(candidate); {
		case statErr != nil || fi.IsDir():
			fmt.Fprintf(os.Stderr, "warn: no qorm binary at the resolved 'go install' destination %s; checking the running binary's location instead.\n", candidate)
		case selfErr == nil && samePath(candidate, selfPath):
			// The install landed exactly where the running binary lives: the
			// selfExe check below covers it, no need to run the same file twice.
		default:
			resolvedPath = candidate
		}
	}
	if resolvedPath != "" {
		if resolvedFail = installedVersionAtLeast(resolvedPath, want); resolvedFail == nil {
			return nil
		}
	}

	var selfFail error
	if selfErr != nil {
		selfFail = fmt.Errorf("cannot verify the installed version: resolving the executable path failed: %v", selfErr)
	} else {
		selfFail = installedVersionAtLeast(selfPath, want)
	}
	if selfFail == nil {
		return nil
	}
	if resolvedFail != nil {
		// Neither location reports the expected version: fail closed.
		return fmt.Errorf("post-install version check failed: neither the 'go install' destination (%s) nor the running binary reports v%s or newer (%v; %v); the Go module proxy may have served a stale or compromised module; refusing to report a successful update", resolvedPath, want, resolvedFail, selfFail)
	}
	return selfFail
}

// installedVersionAtLeast runs exe's `version` command and fails closed unless
// it reports a version >= want.
func installedVersionAtLeast(exe, want string) error {
	got, err := installedBinaryVersion(exe)
	if err != nil {
		return fmt.Errorf("cannot verify the installed version: %v", err)
	}
	cmp, err := compareSemver(got, want)
	if err != nil {
		return fmt.Errorf("installed binary reports an unparseable version %q (expected v%s or newer): %v", got, want, err)
	}
	if cmp < 0 {
		return fmt.Errorf("post-install version check failed: the installed binary at %s reports v%s, which is OLDER than the approved target v%s; the Go module proxy may have served a stale or compromised module; refusing to report a successful update", exe, got, want)
	}
	return nil
}

// samePath reports whether a and b refer to the same file, comparing cleaned
// symlink-resolved paths (selfExe and the resolved install location may reach
// the same binary through different links).
func samePath(a, b string) bool {
	if ra, err := filepath.EvalSymlinks(a); err == nil {
		a = ra
	}
	if rb, err := filepath.EvalSymlinks(b); err == nil {
		b = rb
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// installedBinaryVersion runs `exe version` with a bounded timeout and extracts
// the version the binary reports from its first output line.
func installedBinaryVersion(exe string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), installedVersionTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, "version")
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running %q version failed: %v", exe, err)
	}
	return parseVersionLine(stdout.String())
}

// parseVersionLine extracts X.Y.Z from a `qorm version` output line of the form
// `qorm X.Y.Z (go1.2.3 os/arch)` — the exact format main.go prints. The
// extracted field is validated by compareSemver at the call site; here only
// the line shape is checked.
func parseVersionLine(out string) (string, error) {
	line, _, _ := strings.Cut(strings.TrimSpace(out), "\n")
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "qorm" {
		return "", fmt.Errorf("unexpected version output %q", line)
	}
	return fields[1], nil
}

// semver is a parsed release version: a numeric X.Y.Z core plus optional
// prerelease identifiers. Build metadata is dropped at parse time because it
// carries no precedence.
type semver struct {
	major, minor, patch uint64
	pre                 []string // nil for a final release
}

// compareSemver compares two release tags of the form [v]X.Y.Z[-pre][+build]
// (the repo's tag shape) and returns -1, 0, or +1 when a is older than, equal
// to, or newer than b. A leading "v" and surrounding whitespace are tolerated
// on both sides; a prerelease of the same X.Y.Z sorts older than the release
// (semver.org precedence); build metadata is ignored. It errors when either
// tag is not a well-formed version — the self-update gate treats that as
// "not newer" and refuses to install, fail-closed.
func compareSemver(a, b string) (int, error) {
	av, err := parseSemver(a)
	if err != nil {
		return 0, fmt.Errorf("version %q: %w", a, err)
	}
	bv, err := parseSemver(b)
	if err != nil {
		return 0, fmt.Errorf("version %q: %w", b, err)
	}
	return av.compare(bv), nil
}

// parseSemver parses a release tag into a semver, rejecting anything that is
// not a clean [v]X.Y.Z[-pre][+build] so a tag from a hostile release endpoint
// cannot slip past the version gate.
func parseSemver(s string) (semver, error) {
	orig := s
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexByte(s, '+'); i >= 0 {
		s = s[:i] // build metadata has no precedence
	}
	var pre []string
	if i := strings.IndexByte(s, '-'); i >= 0 {
		ids := strings.Split(s[i+1:], ".")
		pre = make([]string, 0, len(ids))
		for _, id := range ids {
			if err := semverIdentifier(id); err != nil {
				return semver{}, fmt.Errorf("version %q: %w", orig, err)
			}
			pre = append(pre, id)
		}
		s = s[:i]
	}
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return semver{}, fmt.Errorf("version %q is not in X.Y.Z form", orig)
	}
	var nums [3]uint64
	for i, p := range parts {
		n, err := semverNumber(p)
		if err != nil {
			return semver{}, fmt.Errorf("version %q: %w", orig, err)
		}
		nums[i] = n
	}
	return semver{major: nums[0], minor: nums[1], patch: nums[2], pre: pre}, nil
}

// semverNumber parses one numeric version component: digits only, no sign, no
// leading zeros, and within uint64 range.
func semverNumber(s string) (uint64, error) {
	if s == "" {
		return 0, errors.New("empty version component")
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0, fmt.Errorf("component %q is not a number", s)
		}
	}
	if len(s) > 1 && s[0] == '0' {
		return 0, fmt.Errorf("component %q has a leading zero", s)
	}
	n, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("component %q is out of range", s)
	}
	return n, nil
}

// semverIdentifier validates one dot-separated prerelease identifier:
// [0-9A-Za-z-], non-empty, and no leading zeros on purely numeric identifiers
// (semver.org section 9).
func semverIdentifier(id string) error {
	if id == "" {
		return errors.New("empty prerelease identifier")
	}
	allDigits := true
	for _, r := range id {
		switch {
		case r >= '0' && r <= '9':
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '-':
			allDigits = false
		default:
			return fmt.Errorf("prerelease identifier %q has an invalid character", id)
		}
	}
	if allDigits && len(id) > 1 && id[0] == '0' {
		return fmt.Errorf("numeric prerelease identifier %q has a leading zero", id)
	}
	return nil
}

// compare returns -1, 0, or +1 when v is older than, equal to, or newer than
// o: the numeric core decides first; then a final release outranks any
// prerelease of the same X.Y.Z; then prerelease identifiers decide.
func (v semver) compare(o semver) int {
	for _, d := range []struct{ a, b uint64 }{
		{v.major, o.major}, {v.minor, o.minor}, {v.patch, o.patch},
	} {
		if d.a != d.b {
			if d.a < d.b {
				return -1
			}
			return 1
		}
	}
	switch {
	case v.pre == nil && o.pre == nil:
		return 0
	case v.pre == nil:
		return 1 // a release is newer than a prerelease of the same X.Y.Z
	case o.pre == nil:
		return -1
	}
	for i := 0; i < len(v.pre) && i < len(o.pre); i++ {
		if c := compareSemverIdentifier(v.pre[i], o.pre[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(v.pre) < len(o.pre):
		return -1 // all shared identifiers equal: the shorter list is older
	case len(v.pre) > len(o.pre):
		return 1
	}
	return 0
}

// compareSemverIdentifier orders two prerelease identifiers: numeric
// identifiers compare as numbers and sort before alphanumeric ones, which
// compare ASCII-lexicographically (semver.org section 11).
func compareSemverIdentifier(a, b string) int {
	aNum, aIsNum := semverIdentifierNumber(a)
	bNum, bIsNum := semverIdentifierNumber(b)
	switch {
	case aIsNum && bIsNum:
		switch {
		case aNum < bNum:
			return -1
		case aNum > bNum:
			return 1
		}
		return 0
	case aIsNum:
		return -1
	case bIsNum:
		return 1
	}
	return strings.Compare(a, b)
}

// semverIdentifierNumber reports whether id is a purely numeric identifier
// and, if so, its value. Leading zeros were already rejected at parse time.
func semverIdentifierNumber(id string) (uint64, bool) {
	for _, r := range id {
		if r < '0' || r > '9' {
			return 0, false
		}
	}
	n, err := strconv.ParseUint(id, 10, 64)
	return n, err == nil
}

// maxManifestSize caps SHA256SUMS / SHA256SUMS.sig downloads (1 MiB).
const maxManifestSize = 1 << 20

// verifyDownloadedBinary fetches the release's SHA256SUMS + SHA256SUMS.sig and
// verifies the binary saved at tmpPath against them using the public keys
// embedded in this build (releasePubKeys). It errors when this build embeds no
// keys or the release carries no signed manifest.
func verifyDownloadedBinary(client *http.Client, release githubRelease, tmpPath, assetName string) error {
	pubs, err := parseReleasePubKeys(releasePubKeys)
	if err != nil {
		return err
	}
	if len(pubs) == 0 {
		return errors.New("this qorm build embeds no release public keys, so downloads cannot be verified")
	}

	sumsURL, sigURL := "", ""
	for _, asset := range release.Assets {
		switch asset.Name {
		case sumsAssetName:
			sumsURL = asset.DownloadURL
		case sigAssetName:
			sigURL = asset.DownloadURL
		}
	}
	if sumsURL == "" || sigURL == "" {
		return fmt.Errorf("release %s does not provide signed checksums (%s + %s)", release.TagName, sumsAssetName, sigAssetName)
	}

	sums, err := fetchSmallAsset(client, sumsURL, maxManifestSize)
	if err != nil {
		return fmt.Errorf("download %s: %w", sumsAssetName, err)
	}
	sig, err := fetchSmallAsset(client, sigURL, maxManifestSize)
	if err != nil {
		return fmt.Errorf("download %s: %w", sigAssetName, err)
	}
	bin, err := os.ReadFile(tmpPath)
	if err != nil {
		return err
	}
	if err := verifyReleaseAsset(bin, sums, sig, assetName, pubs); err != nil {
		return err
	}
	pub, err := matchReleaseKey(sums, sig, pubs)
	if err != nil {
		return err // unreachable after verifyReleaseAsset, kept for safety
	}
	fmt.Printf("verified: sha256 ok, ed25519 ok (key %s)\n", keys.KeyID(pub))
	return nil
}

// fetchSmallAsset downloads url, refusing bodies larger than limit bytes.
func fetchSmallAsset(client *http.Client, url string, limit int64) ([]byte, error) {
	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("asset exceeds %d byte limit", limit)
	}
	return data, nil
}

// parseReleasePubKeys decodes base64-encoded embedded release public keys.
func parseReleasePubKeys(encoded []string) ([]ed25519.PublicKey, error) {
	pubs := make([]ed25519.PublicKey, 0, len(encoded))
	for _, s := range encoded {
		raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(s))
		if err != nil || len(raw) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("embedded release public key %q is invalid", s)
		}
		pubs = append(pubs, ed25519.PublicKey(raw))
	}
	return pubs, nil
}

// matchReleaseKey returns the first key in pubs whose ed25519 signature over
// sums verifies. sig is the base64 signature (surrounding whitespace ignored).
func matchReleaseKey(sums, sig []byte, pubs []ed25519.PublicKey) (ed25519.PublicKey, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil {
		return nil, fmt.Errorf("%s is not valid base64: %v", sigAssetName, err)
	}
	for _, pub := range pubs {
		if ed25519.Verify(pub, sums, raw) {
			return pub, nil
		}
	}
	return nil, fmt.Errorf("%s signature does not verify against any embedded release public key", sumsAssetName)
}

// verifyReleaseAsset checks a downloaded binary against a signed SHA256SUMS
// manifest: the manifest's ed25519 signature must verify against one of pubs,
// the manifest must list assetName, and bin's sha256 must match the listed
// digest. Pure function — unit-tested in selfupdate_test.go.
func verifyReleaseAsset(bin, sums, sig []byte, assetName string, pubs []ed25519.PublicKey) error {
	if len(pubs) == 0 {
		return errors.New("no release public keys embedded in this build")
	}
	if _, err := matchReleaseKey(sums, sig, pubs); err != nil {
		return err
	}
	want, err := manifestDigest(sums, assetName)
	if err != nil {
		return err
	}
	got := sha256.Sum256(bin)
	if hex.EncodeToString(got[:]) != want {
		return fmt.Errorf("sha256 mismatch for %s: manifest has %s, downloaded binary is %s", assetName, want, hex.EncodeToString(got[:]))
	}
	return nil
}

// manifestDigest extracts the lowercase hex sha256 recorded for assetName in a
// "sha256hex  filename" manifest.
func manifestDigest(sums []byte, assetName string) (string, error) {
	for _, line := range strings.Split(string(sums), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		if strings.TrimPrefix(fields[1], "*") != assetName {
			continue
		}
		digest := strings.ToLower(fields[0])
		if len(digest) != sha256.Size*2 {
			return "", fmt.Errorf("malformed sha256 digest for %s in %s", assetName, sumsAssetName)
		}
		if _, err := hex.DecodeString(digest); err != nil {
			return "", fmt.Errorf("malformed sha256 digest for %s in %s", assetName, sumsAssetName)
		}
		return digest, nil
	}
	return "", fmt.Errorf("asset %q is not listed in %s", assetName, sumsAssetName)
}
