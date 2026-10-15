#!/usr/bin/env bash
# shellcheck disable=SC1000-SC9999

# Prints the sha256 digest of an image manifest's raw bytes for the given ref.
#
# Shared by the release workflow's stage job (capturing the gated digest) and publish job
# (drift check before promotion), so both compute the digest identically.
set -o errexit
set -o pipefail
set -o nounset

if [[ $# -ne 1 ]]; then
    echo "usage: $(basename "$0") <image-ref>" >&2
    exit 2
fi

# The digest of a manifest is the sha256 of its raw bytes; --raw returns exactly those. The bytes go
# to a file rather than straight down a pipe so that a failed inspect (a missing tag, a registry
# error) prints nothing: piping would hash the empty input and hand the caller a plausible-looking
# digest alongside the nonzero exit. Redirecting preserves the bytes exactly as `--raw` emitted
# them, trailing newline included, which is what makes this digest usable as a `ref@digest`.
raw="$(mktemp)"
trap 'rm -f "$raw"' EXIT
docker buildx imagetools inspect "$1" --raw >"$raw"
sha256sum <"$raw" | awk '{print "sha256:"$1}'
