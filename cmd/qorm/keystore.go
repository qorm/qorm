package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

// jdkHint tells the user how to get keytool when no JDK is installed.
const jdkHint = `install a JDK to get keytool:
  macOS:         brew install openjdk    (or https://adoptium.net)
  Debian/Ubuntu: sudo apt install default-jdk
  Windows:       winget install EclipseAdoptium.Temurin.21.JDK`

// ensureKeystore resolves the keystore that signs an Android release build.
//
// With --keystore it references the user's own store: alias from --key-alias
// (default "qorm"), passwords from QORM_KEYSTORE_PASS / QORM_KEY_PASS or an
// interactive prompt when stdin is a TTY.
//
// Otherwise it manages <appDir>/.qorm/release.keystore: reused when present
// (Play requires every update to carry the same signature), generated with
// keytool on first release. The random password lands in keystore.properties
// (0600) next to it and the whole .qorm dir is kept out of git.
func ensureKeystore(appDir string, rel releaseOpts) (ksPath, alias, storePass, keyPass string, err error) {
	alias = rel.KeyAlias
	if alias == "" {
		alias = "qorm"
	}
	if rel.Keystore != "" {
		if ksPath, err = filepath.Abs(rel.Keystore); err != nil {
			return
		}
		if _, statErr := os.Stat(ksPath); statErr != nil {
			err = fmt.Errorf("keystore %s: %v", rel.Keystore, statErr)
			return
		}
		storePass, keyPass, err = keystorePasswords()
		return
	}

	dir := filepath.Join(appDir, ".qorm")
	ksPath = filepath.Join(dir, "release.keystore")
	propsPath := filepath.Join(dir, "keystore.properties")
	if _, statErr := os.Stat(ksPath); statErr == nil {
		// reuse the managed keystore so release signatures stay stable
		if props := readProps(propsPath); props["storePassword"] != "" {
			if a := props["keyAlias"]; a != "" && rel.KeyAlias == "" {
				alias = a
			}
			storePass = props["storePassword"]
			if keyPass = props["keyPassword"]; keyPass == "" {
				keyPass = storePass
			}
			fmt.Fprintf(os.Stderr, "signing with existing keystore %s (alias %s)\n", ksPath, alias)
			return
		}
		// keystore present but its properties are gone — fall back to env/TTY
		fmt.Fprintf(os.Stderr, "warn: %s exists but %s is missing its passwords\n", ksPath, propsPath)
		storePass, keyPass, err = keystorePasswords()
		return
	}

	// first release: generate the keystore
	if _, lookErr := exec.LookPath("keytool"); lookErr != nil {
		err = fmt.Errorf("keytool not found — a JDK is required to create the signing keystore\n%s", jdkHint)
		return
	}
	if err = os.MkdirAll(dir, 0o700); err != nil {
		return
	}
	pass, randErr := randPass()
	if randErr != nil {
		err = randErr
		return
	}
	// PKCS12 (keytool's default store type) wants keyPass == storePass
	storePass, keyPass = pass, pass
	cmd := exec.Command("keytool", "-genkeypair",
		"-keystore", ksPath, "-alias", alias,
		"-keyalg", "RSA", "-keysize", "2048", "-validity", "10000",
		"-storepass:env", "QORM_KS_GEN_PASS", "-keypass:env", "QORM_KS_GEN_PASS",
		"-dname", "CN="+dnameEsc(filepath.Base(absOr(appDir))))
	cmd.Env = append(os.Environ(), "QORM_KS_GEN_PASS="+pass)
	if out, runErr := cmd.CombinedOutput(); runErr != nil {
		os.Remove(ksPath)
		err = fmt.Errorf("keytool failed: %v\n%s\n%s", runErr, strings.TrimSpace(string(out)), jdkHint)
		return
	}
	props := map[string]string{
		"storeFile":     filepath.ToSlash(ksPath),
		"storePassword": storePass,
		"keyPassword":   keyPass,
		"keyAlias":      alias,
	}
	if err = writeProps(propsPath, props); err != nil {
		return
	}
	// `*` makes git ignore the whole .qorm dir (keystore, passwords, this file)
	if err = os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("*\n"), 0o644); err != nil {
		return
	}
	fmt.Fprintf(os.Stderr, `
============================= RELEASE KEYSTORE CREATED =============================
  keystore:  %s
  passwords: %s

  * BACK UP both files somewhere safe (password manager / encrypted storage).
  * NEVER commit them — %s/.gitignore already excludes the directory.
  * If you lose this keystore you PERMANENTLY lose the ability to update the
    app on Google Play under the same listing.
  * Strongly recommended: enroll in Play App Signing so Google escrows the
    app signing key and this file only signs uploads.
=====================================================================================
`, ksPath, propsPath, dir)
	return
}

// keystorePasswords resolves store/key passwords from the environment, or an
// interactive prompt when stdin is a terminal.
func keystorePasswords() (storePass, keyPass string, err error) {
	storePass = os.Getenv("QORM_KEYSTORE_PASS")
	keyPass = os.Getenv("QORM_KEY_PASS")
	if storePass == "" {
		if st, _ := os.Stdin.Stat(); st == nil || st.Mode()&os.ModeCharDevice == 0 {
			err = fmt.Errorf("keystore password required: set QORM_KEYSTORE_PASS (and optionally QORM_KEY_PASS)")
			return
		}
		fmt.Fprint(os.Stderr, "keystore password: ")
		fmt.Fscanln(os.Stdin, &storePass)
		if storePass == "" {
			err = fmt.Errorf("empty keystore password")
			return
		}
	}
	if keyPass == "" {
		keyPass = storePass
	}
	return
}

// randPass returns a 32-char hex password from crypto/rand.
func randPass() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate keystore password: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// readProps parses a minimal key=value properties file ("" values on error).
func readProps(path string) map[string]string {
	props := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return props
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if k, v, ok := strings.Cut(line, "="); ok {
			props[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return props
}

// writeProps writes a key=value properties file with 0600 permissions.
func writeProps(path string, props map[string]string) error {
	keys := make([]string, 0, len(props))
	for k := range props {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var b strings.Builder
	for _, k := range keys {
		b.WriteString(k + "=" + props[k] + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o600)
}

// dnameEsc escapes RFC 2253 special characters for a keytool -dname value.
func dnameEsc(s string) string {
	r := strings.NewReplacer(`\`, `\\`, ",", `\,`, "+", `\+`, `"`, `\"`, "<", `\<`, ">", `\>`, ";", `\;`)
	return r.Replace(s)
}

// absOr returns the absolute form of p, or p itself if that fails.
func absOr(p string) string {
	if a, err := filepath.Abs(p); err == nil {
		return a
	}
	return p
}
