#!/usr/bin/env bash
# Cut a QORM release: preflight → bump the go-install version → tag → push.
#
#   ./scripts/release.sh 0.2.1              # cut v0.2.1
#   ./scripts/release.sh 0.2.1 --dry-run    # show what it would do, change nothing
#
# Pushing the tag triggers the automated half:
#   - .github/workflows/release.yml  → cross-compile 6 platforms, (optionally)
#     ed25519-sign the checksums, publish the GitHub release with binaries.
#   - .github/workflows/docker.yml   → build + push ghcr.io/qorm/qorm:<tag>.
#
# The site (qorm.com docs) is deployed separately — see RELEASE.md. The local
# orchestrator web_server/release.sh runs this script AND the site deploy end
# to end.
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

# --- args -------------------------------------------------------------------
DRY=0
VER=""
for a in "$@"; do
  case "$a" in
    --dry-run) DRY=1 ;;
    -*) echo "unknown flag: $a" >&2; exit 2 ;;
    *) VER="$a" ;;
  esac
done
[ -n "$VER" ] || { echo "usage: $0 <version> [--dry-run]   (e.g. $0 0.2.1)" >&2; exit 2; }
VER="${VER#v}"                                   # accept 0.2.1 or v0.2.1
echo "$VER" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "version must be X.Y.Z (got $VER)" >&2; exit 2; }
TAG="v$VER"

say(){ printf '\033[1m==> %s\033[0m\n' "$*"; }

# --- preflight --------------------------------------------------------------
say "preflight"
[ "$(git rev-parse --abbrev-ref HEAD)" = "main" ] || { echo "not on main" >&2; exit 1; }
git diff --quiet && git diff --cached --quiet || { echo "working tree is dirty — commit or stash first" >&2; exit 1; }
git fetch -q origin
[ "$(git rev-parse @)" = "$(git rev-parse @{u})" ] || { echo "main is not in sync with origin/main" >&2; exit 1; }
git rev-parse "$TAG" >/dev/null 2>&1 && { echo "tag $TAG already exists" >&2; exit 1; }
[ -z "$(gofmt -l cmd internal 2>/dev/null)" ] || { echo "gofmt not clean" >&2; exit 1; }
go vet ./...
go test ./...
say "preflight OK — tests green, tree clean, $TAG is new"

# --- release notes (categorized from the commit log) ------------------------
PREV="$(git describe --tags --abbrev=0 2>/dev/null || echo '')"
RANGE="${PREV:+$PREV..}HEAD"
NOTES="$(mktemp)"
{
  echo "## What's changed"
  echo
  for pat in 'feat' 'fix' 'perf' 'ci' 'build' 'docs'; do
    lines="$(git log --no-merges --pretty='%s' "$RANGE" | grep -E "^$pat(\(|:)" || true)"
    [ -n "$lines" ] || continue
    case "$pat" in
      feat) echo "### Features" ;; fix) echo "### Fixes" ;; perf) echo "### Performance" ;;
      ci) echo "### CI" ;; build) echo "### Build" ;; docs) echo "### Docs" ;;
    esac
    echo "$lines" | sed 's/^/- /'
    echo
  done
  [ -n "$PREV" ] && echo "**Full changelog**: https://github.com/qorm/qorm/compare/$PREV...$TAG"
} > "$NOTES"
say "release notes ($RANGE):"; sed 's/^/    /' "$NOTES"

# --- dry run stops here -----------------------------------------------------
if [ "$DRY" = 1 ]; then
  say "dry-run: would bump version to $VER, commit, tag $TAG, and push main + tag"
  rm -f "$NOTES"; exit 0
fi

# --- bump, commit, tag, push ------------------------------------------------
say "bumping cmd/qorm/main.go version to $VER"
# `go install @<tag>` builds without ldflags, so the hard-coded value must match.
perl -i -pe 's/^var version = ".*"/var version = "'"$VER"'"/' cmd/qorm/main.go
git add cmd/qorm/main.go
git commit -q -m "chore: bump version to $TAG"
git tag -a "$TAG" -F "$NOTES"
say "pushing main + $TAG (this triggers the release + docker workflows)"
git push -q origin main
git push -q origin "$TAG"
rm -f "$NOTES"

say "done. Next:"
echo "  - watch the build:   gh run watch \$(gh run list --workflow Release --limit 1 --json databaseId --jq '.[0].databaseId')"
echo "  - deploy the site:   ./web_server/deploy-site.sh"
echo "  - or run the whole orchestration: ./web_server/release.sh $VER"
