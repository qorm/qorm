#!/usr/bin/env bash
# Cut a QORM release: preflight → archive CHANGELOG.md → bump the go-install
# version → tag → push (retried) → assert the remote refs align with the bump
# commit. At tag time the CHANGELOG `## [Unreleased]` section is archived into
# `## [vX.Y.Z] - <date>` (plus the compare footer link) in its own
# `docs: changelog vX.Y.Z` commit ahead of the version bump; preflight fails
# on an empty or missing section, and on an already-archived version.
#
# Pushes are retried (5 attempts, increasing backoff) and — before the script
# says "done" — it reads origin back and asserts that both `refs/heads/main`
# and the tag dereference equal the bump commit. A flaky push that lands one
# ref but not the other (the v0.3.6 incident: main pushed, tag diverged) fails
# loudly with the mismatching refs and exact recovery commands instead of
# silently leaving a broken release behind.
#
#   ./scripts/release.sh 0.2.1              # cut v0.2.1
#   ./scripts/release.sh 0.2.1 --dry-run    # show what it would do, change nothing
#   ./scripts/release.sh --selftest-changelog  # hidden: unit-test the archiver
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
SELFTEST=0
VER=""
for a in "$@"; do
  case "$a" in
    --dry-run) DRY=1 ;;
    --selftest-changelog) SELFTEST=1 ;;
    -*) echo "unknown flag: $a" >&2; exit 2 ;;
    *) VER="$a" ;;
  esac
done
if [ "$SELFTEST" = 0 ]; then                     # self-test needs no version
  [ -n "$VER" ] || { echo "usage: $0 <version> [--dry-run]   (e.g. $0 0.2.1)" >&2; exit 2; }
  VER="${VER#v}"                                 # accept 0.2.1 or v0.2.1
  echo "$VER" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+$' || { echo "version must be X.Y.Z (got $VER)" >&2; exit 2; }
  TAG="v$VER"
fi

say(){ printf '\033[1m==> %s\033[0m\n' "$*"; }

# --- changelog archive helpers ----------------------------------------------
# CHANGELOG.md keeps a `## [Unreleased]` scratch section at the top: entries
# accumulate there as changes land. At tag time this script archives them: a
# fresh empty `## [Unreleased]` stays on top, the accumulated content moves
# under a new `## [vX.Y.Z] - <date>` heading, and a compare footer link is
# inserted above the newest existing footer link. The helpers below are pure
# file operations (no git) so the hidden --selftest-changelog flag can drive
# them on temp files. The repo standardizes on `perl -i` (BSD sed differs).

# changelog_preflight VER FILE — return non-zero with a message on stderr unless
#   (a) a `## [Unreleased]` section exists;
#   (b) its body has at least one non-blank line (prose-only sections are OK);
#   (c) a `## [vVER]` heading does not already exist (double-archive guard);
#   (d) a `[vVER]:` footer link does not already exist.
changelog_preflight() {
  local ver="$1" file="$2" ver_re line next body
  ver_re="${ver//./\\.}"
  [ -f "$file" ] || { echo "changelog preflight: $file not found" >&2; return 1; }
  line="$(grep -n '^## \[Unreleased\]$' "$file" | head -n1 | cut -d: -f1 || true)"
  if [ -z "$line" ]; then
    echo "changelog preflight: '## [Unreleased]' section is missing from $file — restore it before releasing" >&2
    return 1
  fi
  # body = the lines between the Unreleased heading and the next '## ' heading
  next="$(tail -n +"$((line + 1))" "$file" | grep -n '^## ' | head -n1 | cut -d: -f1 || true)"
  if [ -n "$next" ]; then
    body="$(tail -n +"$((line + 1))" "$file" | head -n "$((next - 1))")"
  else
    body="$(tail -n +"$((line + 1))" "$file")"
  fi
  if ! printf '%s\n' "$body" | grep -q '[^[:space:]]'; then
    echo "changelog preflight: '## [Unreleased]' in $file is empty — add changelog entries before releasing (never rename the heading by hand)" >&2
    return 1
  fi
  if grep -qE "^## \[v${ver_re}\]( .*)?$" "$file"; then
    echo "changelog preflight: '## [v$ver]' already exists in $file — v$ver was already archived" >&2
    return 1
  fi
  if grep -qE "^\[v${ver_re}\]:" "$file"; then
    echo "changelog preflight: footer link '[v$ver]:' already exists in $file — v$ver was already archived" >&2
    return 1
  fi
  return 0
}

# changelog_archive VER DATE FILE [GIT_PREV] — rewrite FILE in place:
#   (a) split the first `## [Unreleased]` heading line into a fresh empty
#       `## [Unreleased]` on top and `## [vVER] - DATE` above the old content;
#   (b) insert `[vVER]: https://github.com/qorm/qorm/compare/vPREV...vVER`
#       immediately above the current top footer link (`^\[vX.Y.Z\]:`), with
#       vPREV read from that link. If GIT_PREV is given and disagrees with
#       vPREV, warn on stderr (the footer has drifted before) but proceed.
changelog_archive() {
  local ver="$1" date="$2" file="$3" git_prev="${4-}"
  local top lineno top_text vprev link
  QORM_NEWVER="$ver" QORM_NEWDATE="$date" perl -i -pe '
    if (!$done && s/^## \[Unreleased\]$/## [Unreleased]\n\n## [v$ENV{QORM_NEWVER}] - $ENV{QORM_NEWDATE}/) { $done = 1 }
  ' "$file"
  top="$(grep -nE '^\[v[0-9]+\.[0-9]+\.[0-9]+\]:' "$file" | head -n1 || true)"
  if [ -z "$top" ]; then
    echo "WARN: no footer compare links in $file — skipped the [v$ver] link" >&2
    return 0
  fi
  lineno="${top%%:*}"
  top_text="${top#*:}"
  vprev="$(printf '%s\n' "$top_text" | sed -nE 's/^\[(v[0-9]+\.[0-9]+\.[0-9]+)\]:.*/\1/p')"
  if [ -z "$vprev" ]; then
    echo "WARN: could not read the previous version from the top footer link — skipped the [v$ver] link" >&2
    return 0
  fi
  if [ -n "$git_prev" ] && [ "$vprev" != "$git_prev" ]; then
    echo "WARN: changelog footer names $vprev as the previous release but the latest git tag is $git_prev (footer drift); inserting above the top link anyway" >&2
  fi
  link="[v$ver]: https://github.com/qorm/qorm/compare/$vprev...v$ver"
  QORM_NEWLINK="$link" perl -i -pe 'print "$ENV{QORM_NEWLINK}\n" if $. == '"$lineno" "$file"
  return 0
}

# --- push / alignment helpers ------------------------------------------------
# A flaky network mid-push can update one remote ref and not the other,
# leaving the tag diverged from main — exactly the v0.3.6 incident, where the
# push half-failed yet was reported as done. push_with_retry retries the push
# with an increasing backoff; remote_tag_sha reads the tag's dereferenced
# commit back from the remote; refs_aligned is a pure string comparison (no
# git, no network) so --selftest-changelog can exercise the check itself. No
# `timeout` binary anywhere — it does not exist on macOS.

# push_with_retry [git-push-args...] — run `git push <args>` up to 5 times,
# sleeping 5/10/20/40/60s after each failure; return non-zero only after all
# five attempts have failed.
push_with_retry() {
  local n=0 wait
  for wait in 5 10 20 40 60; do
    n=$((n + 1))
    if git push "$@"; then
      return 0
    fi
    echo "push failed (attempt $n/5); waiting ${wait}s before the next attempt: git push $*" >&2
    sleep "$wait"
  done
  echo "push failed after 5 attempts: git push $*" >&2
  return 1
}

# refs_aligned EXPECT MAIN_REF TAG_REF — return 0 iff MAIN_REF and TAG_REF are
# both non-empty and both equal EXPECT. Pure comparison, safe under set -e in
# an `if` condition; the selftest drives it directly.
refs_aligned() {
  local expect="$1" main_ref="$2" tag_ref="$3"
  [ -n "$main_ref" ] && [ -n "$tag_ref" ] \
    && [ "$main_ref" = "$expect" ] && [ "$tag_ref" = "$expect" ]
}

# remote_tag_sha TAG — print the remote tag's dereferenced commit SHA (empty
# on failure): `git ls-remote origin "refs/tags/TAG^{}"`, falling back to
# reading origin/TAG after a fetch when the peeled line is unavailable.
remote_tag_sha() {
  local tag="$1" sha
  sha="$(git ls-remote origin "refs/tags/$tag^{}" | cut -f1 || true)"
  if [ -z "$sha" ]; then
    git fetch -q origin || true
    sha="$(git rev-parse --verify -q "origin/$tag^{commit}" || true)"
    [ -n "$sha" ] || sha="$(git rev-parse --verify -q "$tag^{commit}" || true)"
  fi
  printf '%s\n' "$sha"
}

# changelog_selftest — hidden `--selftest-changelog`: builds minimal changelogs
# in a mktemp dir and drives changelog_preflight / changelog_archive over the
# empty, populated, and already-archived cases, plus pure refs_aligned checks
# of the post-push comparison logic. No git and no writes outside the temp
# dir, so it is safe to run anywhere. Prints PASS/FAIL per assertion, returns
# non-zero on any miss, always removes the temp dir.
changelog_selftest() {
  local tmp fails=0 f fw err u nxt nxt_heading first_h body n old_top above
  tmp="$(mktemp -d)" || { echo "selftest: mktemp failed" >&2; return 1; }
  st_pass() { printf 'PASS: %s\n' "$*"; }
  st_fail() { printf 'FAIL: %s\n' "$*"; fails=$((fails + 1)); }

  # case 1: an empty [Unreleased] section is rejected by preflight
  f="$tmp/empty.md"
  cat > "$f" <<'EOF'
# Changelog

## [Unreleased]

## [v0.1.0] - 2026-01-01

### Added
- initial release

[v0.1.0]: https://github.com/qorm/qorm/releases/tag/v0.1.0
EOF
  if err="$(changelog_preflight 0.2.0 "$f" 2>&1)"; then
    st_fail "empty Unreleased: preflight accepted an empty section"
  else
    st_pass "empty Unreleased: preflight rejects it ($err)"
  fi

  # case 2: a populated [Unreleased] section is accepted and archived correctly
  f="$tmp/populated.md"
  cat > "$f" <<'EOF'
# Changelog

## [Unreleased]

### Added
- new thing

## [v0.1.0] - 2026-01-01

### Added
- initial release

[v0.1.0]: https://github.com/qorm/qorm/compare/v0.0.9...v0.1.0
EOF
  cp "$f" "$tmp/populated.orig"
  if changelog_preflight 0.2.0 "$f" 2>"$tmp/preflight.err"; then
    st_pass "populated Unreleased: preflight accepts it"
  else
    st_fail "populated Unreleased: preflight rejected it ($(cat "$tmp/preflight.err"))"
  fi
  changelog_archive 0.2.0 2026-01-02 "$f" v0.1.0 2>"$tmp/archive.err" \
    && st_pass "populated Unreleased: archive transform succeeds" \
    || st_fail "populated Unreleased: archive transform failed ($(cat "$tmp/archive.err"))"
  n="$(grep -c '^## \[v0\.2\.0\] - 2026-01-02$' "$f" || true)"
  [ "$n" = 1 ] && st_pass "archive: exactly one '## [v0.2.0] - 2026-01-02' heading" \
               || st_fail "archive: expected exactly one new version heading, got $n"
  first_h="$(grep '^## ' "$f" | head -n1)"
  [ "$first_h" = "## [Unreleased]" ] && st_pass "archive: fresh '## [Unreleased]' stays on top" \
                                     || st_fail "archive: first heading is '$first_h', not '## [Unreleased]'"
  u="$(grep -n '^## \[Unreleased\]$' "$f" | head -n1 | cut -d: -f1 || true)"
  nxt="$(tail -n +"$((u + 1))" "$f" | grep -n '^## ' | head -n1 | cut -d: -f1 || true)"
  nxt_heading="$(tail -n +"$((u + 1))" "$f" | grep '^## ' | head -n1 || true)"
  [ "$nxt_heading" = "## [v0.2.0] - 2026-01-02" ] \
    && st_pass "archive: new version heading sits directly under [Unreleased]" \
    || st_fail "archive: heading under [Unreleased] is '$nxt_heading'"
  body="$(tail -n +"$((u + 1))" "$f" | head -n "$((nxt - 1))")"
  if printf '%s\n' "$body" | grep -q '[^[:space:]]'; then
    st_fail "archive: the fresh [Unreleased] section is not empty"
  else
    st_pass "archive: the fresh [Unreleased] section is empty"
  fi
  if awk '/^## \[v0\.2\.0\] - 2026-01-02$/{f=1;next} /^## \[v0\.1\.0\]/{f=0} f' "$f" | grep -q '^- new thing$'; then
    st_pass "archive: old content preserved under the new version heading"
  else
    st_fail "archive: '- new thing' lost from the archived section"
  fi
  if old_top="$(grep -nE '^\[v0\.1\.0\]:' "$f" | head -n1 | cut -d: -f1)" && [ -n "$old_top" ]; then
    above="$(head -n "$((old_top - 1))" "$f" | tail -n 1)"
    [ "$above" = "[v0.2.0]: https://github.com/qorm/qorm/compare/v0.1.0...v0.2.0" ] \
      && st_pass "archive: compare link inserted above the old top link with correct vPREV" \
      || st_fail "archive: line above old top link is '$above'"
  else
    st_fail "archive: old top footer link '[v0.1.0]:' disappeared"
  fi

  # case 2b: footer drift warns (but does not fail) when the git tag disagrees
  fw="$tmp/drift.md"
  cp "$tmp/populated.orig" "$fw"
  changelog_archive 0.2.0 2026-01-02 "$fw" v0.1.1 2>"$tmp/drift.err" \
    && st_pass "footer drift: archive transform still succeeds" \
    || st_fail "footer drift: archive transform failed ($(cat "$tmp/drift.err"))"
  if grep -q '^WARN' "$tmp/drift.err"; then
    st_pass "footer drift: warns when the git tag disagrees with the footer"
  else
    st_fail "footer drift: expected a WARN on stderr, got: $(cat "$tmp/drift.err")"
  fi

  # case 3: archiving the same version twice is rejected by the guard
  f="$tmp/archived.md"
  cat > "$f" <<'EOF'
# Changelog

## [Unreleased]

### Added
- yet another thing

## [v0.2.0] - 2026-01-02

### Added
- new thing

## [v0.1.0] - 2026-01-01

### Added
- initial release

[v0.2.0]: https://github.com/qorm/qorm/compare/v0.1.0...v0.2.0
[v0.1.0]: https://github.com/qorm/qorm/compare/v0.0.9...v0.1.0
EOF
  if err="$(changelog_preflight 0.2.0 "$f" 2>&1)"; then
    st_fail "double-archive: preflight accepted a second run for v0.2.0"
  else
    case "$err" in
      *already*) st_pass "double-archive: second run rejected by the already-archived guard" ;;
      *) st_fail "double-archive: rejected, but not by the already-archived guard: $err" ;;
    esac
  fi

  # case 4: refs_aligned — the pure post-push comparison (no git, no network)
  if refs_aligned abc123abc123 abc123abc123 abc123abc123; then
    st_pass "refs_aligned: matching main + tag refs pass"
  else
    st_fail "refs_aligned: matching main + tag refs should pass"
  fi
  if refs_aligned abc123abc123 abc123abc123 deadbeefdead; then
    st_fail "refs_aligned: a mismatched tag ref must fail"
  else
    st_pass "refs_aligned: rejects a mismatched tag ref"
  fi
  if refs_aligned abc123abc123 wrongshamain abc123abc123; then
    st_fail "refs_aligned: a mismatched main ref must fail"
  else
    st_pass "refs_aligned: rejects a mismatched main ref"
  fi
  if refs_aligned abc123abc123 abc123abc123 ""; then
    st_fail "refs_aligned: an empty tag ref must fail"
  else
    st_pass "refs_aligned: rejects an empty tag ref"
  fi
  if refs_aligned abc123abc123 "" abc123abc123; then
    st_fail "refs_aligned: an empty main ref must fail"
  else
    st_pass "refs_aligned: rejects an empty main ref"
  fi

  rm -rf "$tmp"
  if [ "$fails" -eq 0 ]; then
    echo "selftest: all changelog assertions passed"
    return 0
  fi
  echo "selftest: $fails changelog assertion(s) FAILED" >&2
  return 1
}

# Hidden self-test for the changelog archive helpers: temp files only, no git.
if [ "$SELFTEST" = 1 ]; then
  changelog_selftest
  exit 0
fi

# --- preflight --------------------------------------------------------------
say "preflight"
[ "$(git rev-parse --abbrev-ref HEAD)" = "main" ] || { echo "not on main" >&2; exit 1; }
git diff --quiet && git diff --cached --quiet || { echo "working tree is dirty — commit or stash first" >&2; exit 1; }
git fetch -q origin
[ "$(git rev-parse @)" = "$(git rev-parse @{u})" ] || { echo "main is not in sync with origin/main" >&2; exit 1; }
git rev-parse "$TAG" >/dev/null 2>&1 && { echo "tag $TAG already exists" >&2; exit 1; }
[ -z "$(gofmt -l cmd internal pkg 2>/dev/null)" ] || { echo "gofmt not clean" >&2; exit 1; }
go vet ./...
go test ./...
changelog_preflight "$VER" CHANGELOG.md
say "preflight OK — tests green, tree clean, changelog ready, $TAG is new"

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
  say "dry-run: would archive the changelog as [$TAG] - $(date +%Y-%m-%d), bump version to $VER, commit both, tag $TAG, and push main + tag"
  rm -f "$NOTES"; exit 0
fi

# --- archive the changelog (own commit, before the version bump) ------------
RELDATE="$(date +%Y-%m-%d)"
say "archiving CHANGELOG.md: [Unreleased] → [$TAG] - $RELDATE"
changelog_archive "$VER" "$RELDATE" CHANGELOG.md "$PREV"
git add CHANGELOG.md
git commit -q -m "docs: changelog $TAG"

# --- bump, commit, tag, push ------------------------------------------------
say "bumping cmd/qorm/main.go version to $VER"
# `go install @<tag>` builds without ldflags, so the hard-coded value must match.
perl -i -pe 's/^var version = ".*"/var version = "'"$VER"'"/' cmd/qorm/main.go
git add cmd/qorm/main.go
git commit -q -m "chore: bump version to $TAG"
git tag -a "$TAG" -F "$NOTES"
say "pushing main + $TAG (this triggers the release + docker workflows)"
# main: a plain fast-forward push with retry — preflight enforced
# main == origin/main, so no --force (branch protection blocks force-pushes,
# and a fast-forward is what we want).
push_with_retry -q origin main
# tag: --force makes the push idempotent across retries — if an earlier
# attempt half-created the tag remotely, the retry corrects it instead of
# leaving the tag diverged from main (the v0.3.6 incident).
push_with_retry -q --force origin "$TAG"

# --- post-push alignment assertion -------------------------------------------
# Both pushes reported success, but read origin back and PROVE the remote is
# aligned before declaring the release done: remote main must equal the bump
# commit AND the tag must dereference to it. Remote reads can lag, so retry
# the comparison on a backoff budget; if it still mismatches, fail loudly with
# the offending refs and exact recovery steps — never print "done".
BUMP="$(git rev-parse HEAD)"
say "verifying remote alignment: origin main and $TAG must both dereference to $BUMP"
ALIGNED=0
MAIN_SHA=""
TAG_SHA=""
for ALIGN_WAIT in 5 10 20 40 60; do
  MAIN_SHA="$(git ls-remote origin refs/heads/main | cut -f1 || true)"
  TAG_SHA="$(remote_tag_sha "$TAG")"
  if refs_aligned "$BUMP" "$MAIN_SHA" "$TAG_SHA"; then
    ALIGNED=1
    break
  fi
  echo "remote not aligned yet (main=${MAIN_SHA:-<empty>}, $TAG=${TAG_SHA:-<empty>}, want $BUMP); re-checking in ${ALIGN_WAIT}s" >&2
  sleep "$ALIGN_WAIT"
done
if [ "$ALIGNED" != 1 ]; then
  {
    echo "RELEASE PUSH FAILED THE ALIGNMENT CHECK — this release is NOT done."
    echo "  expected commit:      $BUMP"
    [ "$MAIN_SHA" = "$BUMP" ] || echo "  remote main is WRONG: ${MAIN_SHA:-<empty>} (expected $BUMP)"
    [ "$TAG_SHA" = "$BUMP" ] || echo "  remote $TAG is WRONG: ${TAG_SHA:-<empty>} (expected $BUMP)"
    echo "Recover by re-running the push step, then verify both refs equal $BUMP:"
    echo "  git push origin main                  # fast-forward main to $BUMP"
    echo "  git push --force origin $TAG         # force-move the tag onto $BUMP"
    echo "  git ls-remote origin refs/heads/main 'refs/tags/$TAG^{}'"
  } >&2
  rm -f "$NOTES"
  exit 1
fi
say "remote aligned: origin main and $TAG both dereference to $BUMP"
rm -f "$NOTES"

say "done. Next:"
echo "  - watch the build:   gh run watch \$(gh run list --workflow Release --limit 1 --json databaseId --jq '.[0].databaseId')"
echo "  - deploy the site:   ./web_server/deploy-site.sh"
echo "  - or run the whole orchestration: ./web_server/release.sh $VER"
