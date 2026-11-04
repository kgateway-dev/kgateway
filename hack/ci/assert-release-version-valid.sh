#!/usr/bin/env bash

# Exits 0 only when the given version is a valid one to publish. Every other outcome exits nonzero after explaining why.
#
#   ./hack/ci/assert-release-version-valid.sh "$VERSION"
#
# Two independent checks run by default:
#
#   1. The version is not already released: no GitHub release for it (published or draft) and no git
#      tag for it on origin. Never overridable.
#
#   2. The version follows the version that should precede it. For a GA release the expected
#      predecessor is:
#        - v{major}.{minor}.{patch-1} for a patch release;
#        - any v{major}.{minor-1}.x release for a new minor — an RC-only line does not count, since a
#          line that never shipped is exactly the gap worth reporting;
#        - nothing at all for a new major, so vN.0.0 always needs the override.
#      Non-GA releases are not validated.
#      Set ALLOW_MISSING_PREVIOUS_VERSION=true to release over a gap anyway.
#
# --skip-predecessor omits check 2. The release workflow's publish job passes the flag to check that
# the version has not been released while the workflow was running.

set -o errexit
set -o pipefail
set -o nounset

usage() {
    echo "usage: $(basename "$0") [--skip-predecessor] <version>" >&2
}

version=""
check_predecessor=true

while [[ $# -gt 0 ]]; do
    case "$1" in
    --skip-predecessor)
        check_predecessor=false
        ;;
    -*)
        echo "unknown option '$1'" >&2
        usage
        exit 2
        ;;
    *)
        if [[ -n "$version" ]]; then
            usage
            exit 2
        fi
        version="$1"
        ;;
    esac
    shift
done

if [[ -z "$version" ]]; then
    usage
    exit 2
fi

# Check 1: the version has not been released already.
assert_not_released() {
    local repo="${GITHUB_REPOSITORY:-kgateway-dev/kgateway}"

    # `gh release view` exits 1 for a missing release *and* for rate limits, 5xx, and auth failures, so
    # using it here would treat "cannot reach GitHub" as "safe to publish". Request the release by tag
    # with the response status included and require a definite 404 before concluding it is absent.
    local response http_code
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
    local draft_tags
    if ! draft_tags="$(gh api "repos/${repo}/releases" --paginate --jq '.[] | select(.draft) | .tag_name' 2>/dev/null)"; then
        echo "unable to determine whether a draft release '${version}' exists" >&2
        exit 1
    fi
    if grep -Fxq -e "$version" <<<"$draft_tags"; then
        echo "release '${version}' already exists as a draft release (no git tag yet); delete it with 'gh release delete ${version}' before re-running" >&2
        exit 1
    fi

    local git_status=0
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
}

# Check 2: the version follows the version that should precede it.
assert_predecessor_tagged() {
    if [[ ! "$version" =~ ^v([0-9]+)\.([0-9]+)\.([0-9]+)$ ]]; then
        echo "'${version}' is not a GA release; no predecessor is expected"
        return 0
    fi

    local major="${BASH_REMATCH[1]}"
    local minor="${BASH_REMATCH[2]}"
    local patch="${BASH_REMATCH[3]}"

    local tags
    tags="$(git tag --list)"

    # Check 1 owns this case authoritatively, from origin. Repeating it locally costs one grep and
    # catches what check 1 cannot see: a tag that exists only in this clone, never pushed.
    if grep -Fxq -e "$version" <<<"$tags"; then
        echo "cannot release '${version}' because it is already tagged" >&2
        exit 1
    fi

    # On success, `followed` names the release this one was checked against, so the output states what
    # was verified instead of implying the version passed every check.
    local followed="" problem="" expected previous_minor
    if [[ -z "$tags" ]]; then
        problem="this repository has no tags, so the predecessor cannot be verified (was it checked out with all tags fetched?)"
    elif [[ "$patch" -gt 0 ]]; then
        expected="v${major}.${minor}.$((patch - 1))"
        if grep -Fxq -e "$expected" <<<"$tags"; then
            followed="$expected"
        else
            problem="its expected predecessor '${expected}' is not tagged"
        fi
    elif [[ "$minor" -gt 0 ]]; then
        previous_minor=$((minor - 1))
        # The newest patch of the previous minor. `tail` rather than `head` so no consumer exits early
        # and SIGPIPEs a producer under pipefail.
        followed="$(grep -E "^v${major}\.${previous_minor}\.(0|[1-9][0-9]*)$" <<<"$tags" | sort -V | tail -1 || true)"
        if [[ -z "$followed" ]]; then
            problem="no v${major}.${previous_minor}.x release is tagged"
        fi
    else
        problem="v${major}.0.0 starts a new major line, which has no expected predecessor"
    fi

    if [[ -z "$problem" ]]; then
        echo "release '${version}' follows '${followed}'"
        return 0
    fi

    if [[ "${ALLOW_MISSING_PREVIOUS_VERSION:-false}" == "true" ]]; then
        echo "Warning: releasing '${version}' although ${problem}; continuing because ALLOW_MISSING_PREVIOUS_VERSION is set" >&2
        return 0
    fi

    echo "cannot release '${version}' because ${problem}; to release it anyway, set ALLOW_MISSING_PREVIOUS_VERSION=true (the release workflow's 'allow_missing_previous_version' input)" >&2
    exit 1
}

assert_not_released

if [[ "$check_predecessor" == true ]]; then
    assert_predecessor_tagged
fi
