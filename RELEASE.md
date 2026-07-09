# Releasing QORM

A release ships four artifacts from one version tag:

1. **Cross-compiled binaries** — `qorm-{darwin,linux,windows}-{amd64,arm64}` +
   `SHA256SUMS` (ed25519-signed when the release key is configured), attached to
   a GitHub Release.
2. **Container image** — `ghcr.io/qorm/qorm:<tag>` (+ `:latest`, `:sha-<sha>`).
3. **Docs site** — the `docs/` and `api/` trees rendered and published to
   qorm.com under `/docs` and `/api`.
4. **The `go install` version** — the value `qorm version` prints for users who
   `go install ...@<tag>`.

The binary and image builds are automated by GitHub Actions on the tag push.
The site deploy runs locally (its server key is not in the repo). One command
does the whole thing.

## One command (recommended)

```sh
./web_server/release.sh 0.2.1
```

This local orchestrator:
1. runs `scripts/release.sh` (preflight → bump → tag → push),
2. waits for the **Release** and **Docker image** workflows to go green,
3. writes curated notes onto the GitHub Release,
4. deploys the site with `web_server/deploy-site.sh`,
5. verifies the release assets, the image tag, and the live site.

## Repo-side only (no site deploy)

```sh
./scripts/release.sh 0.2.1            # preflight, bump cmd/qorm/main.go, tag, push
./scripts/release.sh 0.2.1 --dry-run  # preview: prints the generated notes, changes nothing
```

Preflight refuses to proceed unless: on `main`, in sync with `origin/main`,
clean working tree, `gofmt`/`go vet`/`go test` all clean, and the tag is new.
Pushing the tag triggers:

- **`.github/workflows/release.yml`** — `vet` + `test`, then
  `scripts/build-all.sh` cross-compiles every platform (stamping the version
  from the tag via `-X main.version`), signs `SHA256SUMS` if the release key is
  set, and publishes the GitHub Release.
- **`.github/workflows/docker.yml`** — builds and pushes the ghcr image.

## Site deploy only

```sh
./web_server/deploy-site.sh
```

Renders `docs/` → `/docs` and `api/` → `/api`, overlays the hand-written
marketing landing pages (`web_server/site/index.html` + `index.zh.html` +
`assets/`) at the root, backs up the current docroot, rsyncs, reloads
OpenResty, and verifies the homepage is the landing page (not the docs index).
Run it any time docs or the landing pages change — it does not need a release.

## One-time setup

- **Release signing** (so `qorm update` can verify downloads): run
  `qorm keygen`, store the private key as the GitHub Actions secret
  `QORM_RELEASE_KEY`, and paste the public key into
  `cmd/qorm/release_pubkey.go`. Until this is done, releases ship an *unsigned*
  `SHA256SUMS` and `qorm update` is fail-closed (needs `--insecure-skip-verify`).
- **Site deploy** needs `web_server/deploy_key` (+ `known_hosts`) — an ed25519
  key trusted by the server. The whole `web_server/` directory is gitignored.

## Versioning

Semantic versioning, tags are `vX.Y.Z`. CI stamps binaries from the tag, so the
only in-repo version to keep in sync is `var version` in `cmd/qorm/main.go`
(what `go install ...@<tag>` reports) — `scripts/release.sh` bumps it for you.
