#!/usr/bin/env bash

# Wait at the point images are needed, allowing cluster setup to overlap their build.
set -euo pipefail

: "${GITHUB_REPOSITORY:?}"
: "${GITHUB_RUN_ID:?}"
# The image build job's name, the name of its step that uploads the image archive,
# and the name of the uploaded artifact.
: "${BUILD_JOB_NAME:?}"
: "${UPLOAD_STEP_NAME:?}"
: "${ARTIFACT_NAME:?}"

# GITHUB_TOKEN requests per hour per repository are shared by every shard of
# every concurrent run. kgateway-dev is on GitHub Enterprise (15,000 per hour),
# which can afford a short interval: about 240 requests per run, and about 5s
# of average lag after the upload. Forks may be on plans limited to 1,000 per
# hour, so they poll gently. IS_FORK is unset outside the workflow; treat that
# like a fork.
if [[ "${IS_FORK:-}" == "false" ]]; then
  poll_interval=10
else
  poll_interval=30
fi
# Ride out transient API failures (5xx, rate limiting) for about ten minutes.
max_api_errors=$((600 / poll_interval))
# The build job exists from the start of the run, so a job missing for about a
# minute means BUILD_JOB_NAME no longer matches the workflow.
max_missing=$((60 / poll_interval))

api_errors=0
missing=0
uploaded=false
# The workflow step has a slightly longer timeout as a second bound on API calls.
deadline=$((SECONDS + 2700))
while (( SECONDS < deadline )); do
  # Inspect the latest execution of the build job in this run. This also supports
  # rerunning only failed shards, which reuse the successful build and its artifact.
  # The upload step succeeds about a minute before the job's post-job cache
  # saves finish, so it is the earliest signal.
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
      uploaded=true
      break
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

if [[ "$uploaded" != true ]]; then
  echo "Timed out waiting for shared e2e image build." >&2
  exit 1
fi

# The artifact can be missing for a second or two after its upload step
# succeeds, and downloading it then fails. Wait until the run lists it.
for _ in $(seq 1 60); do
  if count=$(gh api \
    "repos/$GITHUB_REPOSITORY/actions/runs/$GITHUB_RUN_ID/artifacts?name=$ARTIFACT_NAME" \
    --jq '[.artifacts[] | select(.expired | not)] | length') && (( count > 0 )); then
    echo "Shared e2e images uploaded."
    exit 0
  fi
  sleep 2
done

echo "Upload step succeeded, but run $GITHUB_RUN_ID never listed artifact '$ARTIFACT_NAME'." >&2
exit 1
