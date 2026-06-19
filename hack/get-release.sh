#!/usr/bin/env bash
# shellcheck disable=SC1000-SC9999

# Finds GitHub releases whose tag commit is an ancestor of a given commit.
set -euo pipefail

usage() {
    echo "Usage: $(basename "$0") [-l|--latest [<commit>]] [-p|--previous [<commit>]]" >&2
    echo "  -l, --latest [<commit>]   Most recent release ancestor of <commit> (default: HEAD)" >&2
    echo "  -p, --previous [<commit>] Most recent release of the previous minor version" >&2
    exit 1
}

mode=""
commit_arg=""
while [[ $# -gt 0 ]]; do
    case "$1" in
        -l|--latest)
            mode="latest"
            if [[ $# -gt 1 && ! "$2" == -* ]]; then
                commit_arg="$2"
                shift
            fi
            ;;
        -p|--previous)
            mode="previous"
            if [[ $# -gt 1 && ! "$2" == -* ]]; then
                commit_arg="$2"
                shift
            fi
            ;;
        *) usage ;;
    esac
    shift
done

[[ -z "$mode" ]] && usage

read -r OWNER REPO < <(gh repo view --json owner,name -q '[.owner.login, .name] | @tsv')
HEAD=$(git rev-parse "${commit_arg:-HEAD}")

QUERY='query($owner: String!, $repo: String!, $cursor: String) {
  repository(owner: $owner, name: $repo) {
    releases(first: 100, after: $cursor, orderBy: {field: CREATED_AT, direction: DESC}) {
      nodes {
        tagName
        isDraft
        tagCommit { oid }
      }
      pageInfo { hasNextPage endCursor }
    }
  }
}'

# Finds the most recent release ancestor of HEAD.
find_release() {
    local cursor=""

    while true; do
        local args=(-f query="$QUERY" -f owner="$OWNER" -f repo="$REPO")
        [[ -n "$cursor" ]] && args+=(-f cursor="$cursor")

        local result
        result=$(gh api graphql "${args[@]}")

        while IFS=$'\t' read -r tag sha; do
            local cmp_status
            cmp_status=$(gh api "repos/$OWNER/$REPO/compare/${sha}...${HEAD}" --jq '.status' 2>/dev/null) || true
            if [[ "$cmp_status" == "ahead" || "$cmp_status" == "identical" ]]; then
                echo "$tag"
                return 0
            fi
        done < <(echo "$result" | jq -r '.data.repository.releases.nodes[] | select(.isDraft == false and .tagName != null and .tagCommit != null) | [.tagName, .tagCommit.oid] | @tsv')

        local has_next
        has_next=$(echo "$result" | jq -r '.data.repository.releases.pageInfo.hasNextPage')
        [[ "$has_next" == "true" ]] || break
        cursor=$(echo "$result" | jq -r '.data.repository.releases.pageInfo.endCursor')
    done

    return 1
}

# Finds the most recent release whose tag starts with a given prefix.
find_release_by_prefix() {
    local prefix="$1"
    local cursor=""

    while true; do
        local args=(-f query="$QUERY" -f owner="$OWNER" -f repo="$REPO")
        [[ -n "$cursor" ]] && args+=(-f cursor="$cursor")

        local result
        result=$(gh api graphql "${args[@]}")

        while IFS= read -r tag; do
            if [[ "$tag" == "$prefix"* ]]; then
                echo "$tag"
                return 0
            fi
        done < <(echo "$result" | jq -r '.data.repository.releases.nodes[] | select(.isDraft == false and .tagName != null) | .tagName')

        local has_next
        has_next=$(echo "$result" | jq -r '.data.repository.releases.pageInfo.hasNextPage')
        [[ "$has_next" == "true" ]] || break
        cursor=$(echo "$result" | jq -r '.data.repository.releases.pageInfo.endCursor')
    done

    return 1
}

if [[ "$mode" == "latest" ]]; then
    find_release || { echo "No matching release found" >&2; exit 1; }
    exit 0
fi

if [[ "$mode" == "previous" ]]; then
    latest=$(find_release) || { echo "No matching release found" >&2; exit 1; }

    if [[ "$latest" =~ ^(v?)([0-9]+)\.([0-9]+)\. ]]; then
        v_prefix="${BASH_REMATCH[1]}"
        major="${BASH_REMATCH[2]}"
        minor="${BASH_REMATCH[3]}"
    else
        echo "Cannot parse version from tag: $latest" >&2
        exit 1
    fi

    prev_prefix="${v_prefix}${major}.$((minor - 1))."
    find_release_by_prefix "$prev_prefix" || { echo "No matching release found for ${prev_prefix}*" >&2; exit 1; }
    exit 0
fi
