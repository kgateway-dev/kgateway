#!/usr/bin/env bash

# Exits 0 only when it is safe to publish the given version: there is no GitHub release for it and
# no git tag for it on origin. Every other outcome — the release exists, or the check could not be
# completed — exits nonzero after explaining why, so callers can use this as a bare guard:
#
#   ./hack/ci/assert-release-absent.sh "$VERSION"
#
# The release workflow uses it both as a fail-fast check in setup and as the final guard in publish,
# so the two cannot diverge.
#
# Requires GH_TOKEN in the environment for the GitHub API call.
set -o errexit
set -o pipefail
set -o nounset

if [[ $# -ne 1 ]]; then
    echo "usage: $(basename "$0") <version>" >&2
    exit 2
fi

version="$1"
repo="${GITHUB_REPOSITORY:-kgateway-dev/kgateway}"

# `gh release view` exits 1 for a missing release *and* for rate limits, 5xx, and auth failures, so
# using it here would treat "cannot reach GitHub" as "safe to publish". Request the release by tag
# with the response status included and require a definite 404 before concluding it is absent.
response="$(gh api "repos/${repo}/releases/tags/${version}" --include 2>/dev/null || true)"
http_code="$(awk 'NR == 1 {print $2}' <<<"$response")"

case "$http_code" in
200)
    echo "release '${version}' already exists as a GitHub release" >&2
    exit 1
    ;;
404) ;; # No *published* release; fall through to the draft and git tag checks.
*)
    echo "unable to determine whether release '${version}' exists (response status: ${http_code:-none})" >&2
    exit 1
    ;;
esac

# The endpoint above returns published releases only, and a draft has no git tag, so neither that
# check nor the tag check below can see a draft. One can be left behind: `gh release create` creates
# the release as a draft, uploads the assets, then publishes, so an interrupted publish leaves a
# draft holding this version's tag_name that would collide with the next attempt. Drafts are only
# reachable by listing releases and matching tag_name.
if ! draft_tags="$(gh api "repos/${repo}/releases" --paginate --jq '.[] | select(.draft) | .tag_name' 2>/dev/null)"; then
    echo "unable to determine whether a draft release '${version}' exists" >&2
    exit 1
fi
if grep -Fxq -e "$version" <<<"$draft_tags"; then
    echo "release '${version}' already exists as a draft release (no git tag yet); delete it with 'gh release delete ${version}' before re-running" >&2
    exit 1
fi

git_status=0
git ls-remote --exit-code --tags origin "refs/tags/${version}" >/dev/null || git_status=$?

# `git ls-remote --exit-code` returns 2 when the ref does not exist. Any other failure means the
# duplicate-release guard could not establish that publishing is safe, so fail closed.
if [[ $git_status -eq 0 ]]; then
    echo "release '${version}' already exists as a git tag on origin" >&2
    exit 1
elif [[ $git_status -ne 2 ]]; then
    echo "unable to determine whether tag '${version}' exists" >&2
    exit 1
fi

echo "release '${version}' does not exist yet"
