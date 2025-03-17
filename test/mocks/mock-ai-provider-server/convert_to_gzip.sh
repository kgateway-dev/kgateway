#!/bin/bash

# Get the directory where the script is located
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"

# Input and output paths
INPUT_FILE="$SCRIPT_DIR/data/routing/openai_non_streaming.json"
OUTPUT_FILE="$SCRIPT_DIR/data/routing/openai_non_streaming.txt.gz"

# Read the input file and compress it to gzip format
gzip -c "$INPUT_FILE" > "$OUTPUT_FILE"