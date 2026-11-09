#!/usr/bin/env bash

# Wait at the point images are needed, allowing cluster setup to overlap their build.
set -euo pipefail

: "${GITHUB_REPOSITORY:?}"
: "${GITHUB_RUN_ID:?}"
# The image build job's name, and the name of its step that uploads the image archive.
: "${BUILD_JOB_NAME:?}"
: "${UPLOAD_STEP_NAME:?}"

# GITHUB_TOKEN allows 1,000 API requests per hour per repository, shared by
# every shard of every concurrent run, so poll gently.
poll_interval=30
# Ride out transient API failures (5xx, rate limiting) for about ten minutes.
max_api_errors=20
# The build job exists from the start of the run, so a missing job means
# BUILD_JOB_NAME no longer matches the workflow.
max_missing=3

api_errors=0
missing=0
# The workflow step has a slightly longer timeout as a second bound on API calls.
deadline=$((SECONDS + 2700))
while (( SECONDS < deadline )); do
  # Inspect the latest execution of the build job in this run. This also supports
  # rerunning only failed shards, which reuse the successful build and its artifact.
  # The artifact can be downloaded as soon as its upload step succeeds, about a
  # minute before the job's post-job cache saves finish, so that is enough.
  if ! state=$(gh api --paginate \
    "repos/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID/jobs?filter=latest&per_page=100" \
    --jq '.jobs[] | select(.name == env.BUILD_JOB_NAME)
      | if any(.steps[]?; .name == env.UPLOAD_STEP_NAME and .conclusion == "success") then "success"
        elif .status == "completed" then .conclusion
        else .status end'); then
    api_errors=$((api_errors + 1))
    if (( api_errors >= max_api_errors )); then
      echo "Giving up after $api_errors consecutive GitHub API failures." >&2
      exit 1
    fi
    sleep "$poll_interval"
    continue
  fi
  api_errors=0
  state=${state%%$'\n'*}
  case "$state" in
    success)
      echo "Shared e2e images uploaded."
      exit 0
      ;;
    "")
      missing=$((missing + 1))
      if (( missing >= max_missing )); then
        echo "No job named '$BUILD_JOB_NAME' in run $GITHUB_RUN_ID." >&2
        exit 1
      fi
      ;;
    queued|in_progress|waiting|pending|requested)
      missing=0
      ;;
    *)
      echo "Shared e2e image build did not succeed: $state" >&2
      exit 1
      ;;
  esac
  sleep "$poll_interval"
done

echo "Timed out waiting for shared e2e image build." >&2
exit 1
