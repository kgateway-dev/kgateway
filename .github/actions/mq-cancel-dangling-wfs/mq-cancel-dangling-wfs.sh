#!/bin/bash

if [[ -z $GITHUB_TOKEN ]]; then echo "❌ env var GITHUB_TOKEN not set"; exit 1; fi
if [[ -z $GH_ORG_REPO ]]; then echo "❌ env var GH_ORG_REPO not set"; exit 1; fi

# Cancel all builds for MQ branches which have already been deleted
# i.e cancel dangling builds

# Start clean
echo '[]' > filtered_runs.json
> all_runs.jsonl

echo "📥 Step 1: Fetching latest workflow runs (up to 300)..."
for page in {1..3}; do
    echo "📄 Fetching page $page of workflow runs..."
    gh api "repos/$GH_ORG_REPO/actions/runs?per_page=100&page=$page" --jq '.workflow_runs[]' |
        jq -c '.' >> all_runs.jsonl
done

echo "🔍 Step 2: Filtering relevant workflow runs (MQ branches + specific workflows)..."
> filtered_runs.jsonl

while IFS= read -r run; do
    path=$(echo "$run" | jq -r '.path')
    branch=$(echo "$run" | jq -r '.head_branch')
    run_id=$(echo "$run" | jq -r '.id')
    status=$(echo "$run" | jq -r '.status')

    echo "➡️  Examining run ID: $run_id on branch: $branch ($path)"

    # Only consider MQ branches
    if [[ "$branch" != gh-readonly-queue/* ]]; then
        echo "    ⏭️ Skipped (not an MQ branch)"
        continue
    fi

    # filter out completed wf runs
    if [[ "$status" == "completed" ]]; then
        echo "    ⏭️ Skipped (workflow is already complete)"
        continue
    fi

    # Check if branch still exists
    echo "    🔎 Checking if branch $branch exists..."
    if gh api "repos/$GH_ORG_REPO/branches/$branch" --silent > /dev/null 2>&1; then
        echo "    ✅ Branch exists — skipping cancel"
    else
        echo "    ❌ Branch missing — will cancel run $run_id"
        jq -n --arg id "$run_id" --arg branch "$branch" '{id: ($id|tonumber), branch: $branch}' >> filtered_runs.jsonl
    fi

done < all_runs.jsonl

echo "🧹 Step 3: Cancelling filtered workflow runs..."
jq -s '.' filtered_runs.jsonl > filtered_runs.json

jq -c '.[]' filtered_runs.json | while read -r run; do
    run_id=$(echo "$run" | jq -r '.id')
    branch=$(echo "$run" | jq -r '.branch')

    echo "🛑 Canceling workflow run ID: $run_id (branch: $branch)"

    if ! gh api -X POST "repos/$GH_ORG_REPO/actions/runs/$run_id/cancel" --silent > /dev/null 2>&1; then
        echo "❌ Failed to cancel run $run_id (might have completed already)"
    else
        echo "✅ Successfully requested cancel for $run_id on branch $branch"
    fi
done

echo "🧼 Step 4: Cleaning up temporary files..."
rm -f filtered_runs.jsonl all_runs.jsonl