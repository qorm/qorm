# Vendored: webview (self-maintained — do not re-add as a dependency)

`internal/webview` is vendored from [webview/webview_go](https://github.com/webview/webview_go)
(the Go binding, commit 6173450) together with the [webview/webview](https://github.com/webview/webview)
C/C++ header it wraps (commit `fb6b17d`, pinned in `libs/webview/version.txt`).
Both are MIT-licensed — see `LICENSE` and `libs/*/LICENSE`. Copyright (c) 2017
Serge Zaitsev and contributors.

## Policy: QORM owns this code

This is **permanently vendored and self-maintained**. It must **never** be turned
back into a `go.mod` dependency:

- The upstream Go binding has been effectively unmaintained for years, so we can't
  rely on it for fixes, new-OS support, or security patches.
- The desktop build links this C/C++ via cgo (`-tags desktop`); a live network
  dependency there would make the one build that needs a toolchain also need the
  network, and would put QORM's native window at the mercy of an abandoned repo.
- Keeping it in-tree means every fix ships in one commit, reviewed like the rest
  of QORM, and the pure-Go default build stays dependency-free.

Treat these files as QORM source: fix bugs here directly, don't wait on upstream.

## Local modifications

Local changes are noted at the top of each changed file (e.g. the build-tag and
package comment in `webview.go`). Keep that convention so a future re-sync can
tell our edits from upstream.

## Re-syncing from upstream (rare)

If you ever pull upstream changes, do it as a manual, reviewed merge — not a
wholesale overwrite:

1. Diff our `webview.go` against the target upstream revision.
2. Re-apply our local modifications (the noted headers) on top.
3. Update the C header under `libs/webview/` and bump `version.txt` to the new
   `webview/webview` commit; update the commit ids in this file.
4. Build and run the desktop app (`-tags desktop`) on every OS you can before
   committing — the binding is cgo, so `go vet` alone won't catch breakage.
