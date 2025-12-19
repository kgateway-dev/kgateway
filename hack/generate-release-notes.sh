#!/bin/bash

set -o errexit
set -o nounset
set -o pipefail

# Track execution time
SECONDS=0

# Default values
PREVIOUS_TAG=""
CURRENT_TAG="HEAD"
OUTPUT_DIR="${OUTPUT_DIR:-_output}"
OUTPUT_FILE="${OUTPUT_DIR}/RELEASE_NOTES.md"
REPO_OVERRIDE=""
DEBUG=${DEBUG:-""}
TEMP_DIR=""

# Help message
function show_help() {
    echo "Usage: $0 [options]"
    echo "Options:"
    echo "  -p, --previous-tag <tag>     Previous release tag (required)"
    echo "  -c, --current-tag <tag>      Current release tag (default: HEAD)"
    echo "  -r, --repo <owner/repo>      GitHub repository (default: kgateway-dev/kgateway)"
    echo "  -o, --output <file>          Output file (default: _output/RELEASE_NOTES.md)"
    echo "  -h, --help                   Show this help message"
    echo ""
    echo "Environment variables:"
    echo "  GITHUB_TOKEN               GitHub API token (required)"
    echo "  DEBUG                      Set to any value to enable debug output"
    echo ""
    echo "Examples:"
    echo "  $0 -p v2.0.0 -c v2.1.0"
    echo "  $0 -p v2.0.0-rc.1 -c v2.0.0-rc.2 -o changelog.md"
}

function debug() {
    if [ -n "$DEBUG" ]; then
        echo "DEBUG: $*" >&2
    fi
}

function cleanup() {
    if [ -n "$TEMP_DIR" ] && [ -d "$TEMP_DIR" ]; then
        rm -rf "$TEMP_DIR"
    fi
}

# Get section title for a given kind
function get_section_title() {
    local kind="$1"
    case "$kind" in
        "breaking_change") echo "Breaking Changes" ;;
        "feature") echo "New Features" ;;
        "fix") echo "Bug Fixes" ;;
        "deprecation") echo "Deprecations" ;;
        "documentation") echo "Documentation" ;;
        "cleanup") echo "Cleanup" ;;
        "install") echo "Installation Changes" ;;
        "bump") echo "Dependency Updates" ;;
        *) echo "" ;;
    esac
}

# Check if a release note is effectively "NONE"
function is_none_release_note() {
    local note="$1"
    # Convert to lowercase and trim whitespace
    local normalized=$(echo "$note" | tr '[:upper:]' '[:lower:]' | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')

    # Check if empty or "none"
    if [ -z "$normalized" ] || [ "$normalized" = "none" ]; then
        return 0  # true in bash
    fi
    return 1  # false in bash
}

# Extract release note from PR body
function extract_release_note() {
    local body="$1"
    local in_block=0
    local note=""

    # Strip Windows carriage returns
    body=$(echo "$body" | tr -d '\r')

    while IFS= read -r line; do
        if [[ "$line" =~ ^[[:space:]]*\`\`\`release-note[[:space:]]*$ ]]; then
            in_block=1
        elif [[ "$line" =~ ^[[:space:]]*\`\`\`[[:space:]]*$ ]] && [ "$in_block" -eq 1 ]; then
            in_block=0
            break
        elif [ "$in_block" -eq 1 ]; then
            if [ -n "$note" ]; then
                note+=$'\n'
            fi
            note+="$line"
        fi
    done <<< "$body"

    # Trim leading/trailing whitespace while preserving internal formatting
    note=$(echo "$note" | sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//')
    echo "$note"
}

# Format a release note for markdown output
function format_release_note() {
    local note="$1"
    local pr_link="$2"

    # Handle single line vs multiline notes differently
    if [[ "$note" =~ $'\n' ]]; then
        # Multiline note: format as bullet point with proper indentation
        # First line gets the bullet point
        local first_line=$(echo "$note" | head -n1)
        local rest_lines=$(echo "$note" | tail -n +2)

        echo "* ${first_line}"
        # Indent continuation lines with 2 spaces
        if [ -n "$rest_lines" ]; then
            echo "$rest_lines" | sed 's/^/  /'
        fi
        # Add PR link on a separate indented line
        echo "  ${pr_link}"
    else
        # Single line note: add note and PR link on same line
        echo "* ${note} ${pr_link}"
    fi
}

# Process PRs for the repository
function process_prs() {
    local owner="$1"
    local repo="$2"
    local start_ref="$3"
    local end_ref="$4"
    local pr_numbers_file="$5"

    # Get PR numbers from commit messages
    git log "$start_ref..$end_ref" --pretty=format:"%s%n%b" | \
        grep -o -E "(#|\\()[0-9]+\\)?" | \
        tr -d '#()' | \
        sort -un > "$pr_numbers_file"

    if [ ! -s "$pr_numbers_file" ]; then
        debug "No PR numbers found in commit messages between $start_ref and $end_ref"
        return
    fi

    debug "Processing PRs from $owner/$repo..."

    # Process each PR
    while read -r PR_NUMBER; do
        # Fetch PR data using GitHub API
        PR_DATA=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
            -H "Accept: application/vnd.github.v3+json" \
            "https://api.github.com/repos/$owner/$repo/pulls/$PR_NUMBER")

        # Check if PR exists
        if [[ $(echo "$PR_DATA" | jq -r '.message // empty') == "Not Found" ]]; then
            debug "PR #$PR_NUMBER not found, trying issue endpoint..."
            # Try to fetch as an issue instead
            PR_DATA=$(curl -s -H "Authorization: token $GITHUB_TOKEN" \
                -H "Accept: application/vnd.github.v3+json" \
                "https://api.github.com/repos/$owner/$repo/issues/$PR_NUMBER")
            if [[ $(echo "$PR_DATA" | jq -r '.message // empty') == "Not Found" ]]; then
                debug "Issue #$PR_NUMBER not found either, skipping"
                continue
            fi
        fi

        # Extract PR title and body
        PR_TITLE=$(echo "$PR_DATA" | jq -r .title)
        PR_BODY=$(echo "$PR_DATA" | jq -r '.body // empty')

        # Extract release note
        RELEASE_NOTE=$(extract_release_note "$PR_BODY")

        debug "Extracted release note for PR #$PR_NUMBER: '$RELEASE_NOTE'"

        # Skip if release note is "NONE" or empty
        if is_none_release_note "$RELEASE_NOTE"; then
            debug "Skipping PR #$PR_NUMBER - release note is none/empty"
            continue
        fi

        # Get PR labels
        LABELS=$(echo "$PR_DATA" | jq -r '.labels[].name')

        # Find the kind label
        KIND=""
        while read -r LABEL; do
            if [[ "$LABEL" =~ ^kind/ ]]; then
                KIND=${LABEL#kind/}
                break
            fi
        done <<< "$LABELS"

        # Skip if no kind label
        if [[ -z "$KIND" ]]; then
            debug "PR #$PR_NUMBER missing kind label, skipping"
            continue
        fi

        debug "Found PR #$PR_NUMBER ($KIND): $PR_TITLE"

        # Get section title
        SECTION_TITLE=$(get_section_title "$KIND")
        if [ -n "$SECTION_TITLE" ]; then
            local pr_link="([#$PR_NUMBER](https://github.com/$owner/$repo/pull/$PR_NUMBER))"
            local formatted_note=$(format_release_note "$RELEASE_NOTE" "$pr_link")
            debug "Formatted note for PR #$PR_NUMBER: '$formatted_note'"
            echo "$formatted_note" >> "$TEMP_DIR/$KIND.txt"
        fi
    done < "$pr_numbers_file"
}

# Parse arguments
while [[ $# -gt 0 ]]; do
    case $1 in
        -p|--previous-tag)
            PREVIOUS_TAG="$2"
            shift 2
            ;;
        -c|--current-tag)
            CURRENT_TAG="$2"
            shift 2
            ;;
        -o|--output)
            OUTPUT_FILE="$2"
            shift 2
            ;;
        -r|--repo)
            REPO_OVERRIDE="$2"
            shift 2
            ;;
        -h|--help)
            show_help
            exit 0
            ;;
        *)
            echo "Unknown option: $1"
            show_help
            exit 1
            ;;
    esac
done

# Validate required parameters
if [ -z "$PREVIOUS_TAG" ]; then
    echo "Error: Previous tag is required"
    show_help
    exit 1
fi

if [ -z "${GITHUB_TOKEN:-}" ]; then
    echo "Error: GITHUB_TOKEN environment variable is required"
    echo "Export it with: export GITHUB_TOKEN=<your_token>"
    exit 1
fi

# Create temporary directory for PR data
TEMP_DIR=$(mktemp -d)
trap cleanup EXIT

# Get repository information (defaults to upstream, can be overridden for forks)
if [ -n "$REPO_OVERRIDE" ]; then
    OWNER=$(echo "$REPO_OVERRIDE" | cut -d'/' -f1)
    REPO=$(echo "$REPO_OVERRIDE" | cut -d'/' -f2)
else
    OWNER="kgateway-dev"
    REPO="kgateway"
fi

debug "Repository: $OWNER/$REPO"
debug "Previous tag: $PREVIOUS_TAG"
debug "Current tag/ref: $CURRENT_TAG"

# Process PRs
echo "Fetching PRs between $PREVIOUS_TAG and $CURRENT_TAG..."
process_prs "$OWNER" "$REPO" "$PREVIOUS_TAG" "$CURRENT_TAG" "$TEMP_DIR/prs.txt"

# Check if we found any release notes
FOUND_NOTES=0
for KIND in breaking_change feature fix deprecation documentation cleanup install bump; do
    if [[ -f "$TEMP_DIR/$KIND.txt" ]]; then
        FOUND_NOTES=1
        break
    fi
done

if [[ $FOUND_NOTES -eq 0 ]]; then
    echo "No release notes found in any PRs"
    exit 1
fi

# Ensure output directory exists
mkdir -p "$(dirname "$OUTPUT_FILE")"

# Initialize the output file
cat > "$OUTPUT_FILE" << EOF
## Release Notes

### Changes since $PREVIOUS_TAG
EOF

# Generate the final release notes
for KIND in breaking_change feature fix deprecation documentation cleanup install bump; do
    if [[ -f "$TEMP_DIR/$KIND.txt" ]]; then
        SECTION_TITLE=$(get_section_title "$KIND")
        echo -e "\n#### $SECTION_TITLE\n" >> "$OUTPUT_FILE"
        cat "$TEMP_DIR/$KIND.txt" >> "$OUTPUT_FILE"
    fi
done

echo "Release notes have been generated in $OUTPUT_FILE (${SECONDS}s)"
