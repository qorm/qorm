package main

// releasePubKeys holds the ed25519 public keys trusted to sign QORM release
// checksum manifests (SHA256SUMS.sig). Each entry is the base64 body line of a
// `qorm keygen` .pub file (std base64 of the 32 raw key bytes). `qorm update`
// refuses to install a downloaded binary unless the release's SHA256SUMS
// manifest verifies against one of these keys — see verifyReleaseAsset in
// selfupdate.go; --insecure-skip-verify bypasses the check.
//
// Populating (one-time maintainer step): run `qorm keygen`, store the private
// key file's content as the QORM_RELEASE_KEY GitHub Actions secret, and paste
// the public key's base64 line into this list. While the list is empty this
// build treats every release as unverifiable, and `qorm update` errors unless
// --insecure-skip-verify is given.
//
// Rotation policy: to rotate the release key, first ship at least one release
// whose binary embeds BOTH the old and the new public key while assets are
// still signed with the old key; then switch CI (the QORM_RELEASE_KEY secret)
// to the new private key. Binaries that only embed the old key can update to
// the transition release, and from there to releases signed with the new key.
// Drop the old key only once every release users may update from embeds the
// new one.
var releasePubKeys = []string{
	// "base64-public-key-here", // key <keys.KeyID> — added YYYY-MM-DD
}
