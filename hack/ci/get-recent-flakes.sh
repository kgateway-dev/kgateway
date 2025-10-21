#!/usr/bin/env bash

# Get merge queue e2e failures from the past `DAYS` days, sorted by frequency.
# Usage: DAYS=<days> SHOW_URLS=<true|false> ./get-recent-flakes.sh
# Environment variables:
#   DAYS: number of days to look back (default: 1)
#   SHOW_URLS: whether to display run URLs in output (default: true)

days=${DAYS:-1}
show_urls=${SHOW_URLS:-true}
start_date=$(date -v-${days}d +"%Y-%m-%dT%H:%M:%S")

repo_info=$(gh repo view --json owner,name -q '.owner.login + "/" + .name')
failing_run_ids=$(gh run list -w=".github/workflows/e2e.yaml" --event=merge_group --created ">$start_date" --limit 100 -s failure --json databaseId | jq '.[].databaseId' -r)

# map of test_name to list of run_urls
# Format: test_map["test_name"]="run_url1,run_url2,run_url3"
declare -A test_map

for run_id in $failing_run_ids
do
    run_url="https://github.com/$repo_info/actions/runs/$run_id"
    failure_lines=$(gh run view $run_id --log-failed | grep FAIL:)
    # extract the test names from the log lines
    failing_tests=$(echo $failure_lines | grep -o 'FAIL: [^ ]*' | sed 's/FAIL: //')

    # associate each failing test with this run_url
    while IFS= read -r test_name; do
        if [ -n "$test_name" ]; then
            # Check if this test already exists in our map
            if [ -n "${test_map[$test_name]}" ]; then
                # Test exists, append run_url to existing list
                test_map[$test_name]="${test_map[$test_name]},$run_url"
            else
                # New test, add to map
                test_map[$test_name]="$run_url"
            fi
        fi
    done <<< "$failing_tests"
    echo -n "." # progress indicator
done

echo -e "\nFailing e2e tests in merge queue over last $days day(s):"
echo "====================================="

# Process each test entry and count occurrences
sorted_tests=""
for test_name in "${!test_map[@]}"; do
    run_urls="${test_map[$test_name]}"
    # Count occurrences by counting commas in run_urls + 1
    count=$(echo "$run_urls" | tr ',' '\n' | grep -c .)
    sorted_tests="$sorted_tests$count|$test_name|$run_urls"$'\n'
done

# Sort by count (descending)
sorted_tests=$(echo "$sorted_tests" | sort -t'|' -k1,1nr)

# Filter out tests that are prefixes of other tests and display results
while IFS= read -r line; do
    if [ -n "$line" ]; then
        # Extract the count, test name, and run_urls
        count=$(echo "$line" | cut -d'|' -f1)
        test_name=$(echo "$line" | cut -d'|' -f2)
        run_urls=$(echo "$line" | cut -d'|' -f3)
        
        # Check if this test name is a prefix of any other test name in the list
        is_prefix=false
        while IFS= read -r other_line; do
            if [ -n "$other_line" ] && [ "$line" != "$other_line" ]; then
                other_test_name=$(echo "$other_line" | cut -d'|' -f2)
                if [[ "$other_test_name" == "$test_name"* ]]; then
                    is_prefix=true
                    break
                fi
            fi
        done <<< "$sorted_tests"
        
        # Only include if it's not a prefix of another test
        if [ "$is_prefix" = false ]; then
            if [ "$count" = "1" ]; then
                echo "$test_name (Failed 1 time)"
            else
                echo "$test_name (Failed $count times)"
            fi
            if [ "$show_urls" = "true" ]; then
                # Split run_urls by comma and display each on a new line
                echo "$run_urls" | tr ',' '\n' | sed 's/^/  /'
            fi
            echo ""
        fi
    fi
done <<< "$sorted_tests"
