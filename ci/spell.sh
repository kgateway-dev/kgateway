#!/bin/bash

# Update March 2, 2023
# This is a potentially powerful script, and one that has been running in CI but doing nothing:
#   Step #1 - "check-spelling": cat: /workspace/gloo/ci/../ci/spelling_whitelist_words.txt: No such file or directory
#   Step #1 - "check-spelling": cat: /workspace/gloo/ci/../ci/spelling_skip_files.txt: No such file or directory
#   Step #1 - "check-spelling": find: paths must precede expression: `|'
#   Step #1 - "check-spelling": Done
# Leaving this script in the repository in case we want to use it in the future, but removing the execution
# in our CI pipeline because it is a no-op

# this file as adapted from here: https://github.com/envoyproxy/envoy/tree/main/tools/spelling

# Applies requisite code formatters to the source tree
# check_spelling.sh

# Why choose misspell?
# https://github.com/client9/misspell#what-are-other-misspelling-correctors-and-whats-wrong-with-them

set -u
set -e

VERSION="0.3.4"
LINUX_MISSPELL_SHA="34d489dbc5ddb4dfd6d3cfac9fde8660e6c37e6c"
MAC_MISSPELL_SHA="f2607e2297b9e8af562e384c38045033375c7433"
TMP_DIR="/tmp"
OS=""

MISSPELL_ARGS="-error -o stderr"

if [[ "$#" -lt 1 ]]; then
  echo "Usage: $0 check|fix"
  exit -1
fi

if [[ "$1" == "fix" ]]; then
  MISSPELL_ARGS="-w"
fi

if [[ "$(uname)" == "Darwin" ]]; then
  OS="mac"
elif [[ "$(uname)" == "Linux" ]]; then
  OS="linux"
else
  echo "Current only support mac/Linux"
  exit 1
fi

SCRIPTPATH=$( cd "$(dirname "$0")" ; pwd -P )
ROOTDIR="${SCRIPTPATH}/.."
cd "$ROOTDIR"

BIN_FILENAME="misspell_"${VERSION}"_"${OS}"_64bit.tar.gz"
# Install tools we need
if [[ ! -e "${TMP_DIR}/misspell" ]]; then
  if ! wget https://github.com/client9/misspell/releases/download/v"${VERSION}"/"${BIN_FILENAME}" \
  -O "${TMP_DIR}/${BIN_FILENAME}" --no-verbose --tries=3 -o "${TMP_DIR}/wget.log"; then
    cat "${TMP_DIR}/wget.log"
    exit -1
  fi
  tar -xvf "${TMP_DIR}/${BIN_FILENAME}" -C "${TMP_DIR}" &> /dev/null
fi

ACTUAL_SHA=""
EXPECT_SHA=""

if [[ "${OS}" == "linux" ]]; then
  ACTUAL_SHA=$(sha1sum "${TMP_DIR}"/misspell|cut -d' ' -f1)
  EXPECT_SHA="${LINUX_MISSPELL_SHA}"
else
  ACTUAL_SHA=$(shasum -a 1 "${TMP_DIR}"/misspell|cut -d' ' -f1)
  EXPECT_SHA="${MAC_MISSPELL_SHA}"
fi

if [[ ! ${ACTUAL_SHA} == ${EXPECT_SHA} ]]; then
   echo "Expect shasum is ${ACTUAL_SHA}, but actual is shasum ${EXPECT_SHA}"
   exit 1
fi

chmod +x "${TMP_DIR}/misspell"
 
# Spell checking
# All the skipping files are defined in ci/spelling_skip_files.txt
SPELLING_SKIP_FILES="${ROOTDIR}/ci/spelling_skip_files.txt"

# All the ignore words are defined in ci/spelling_whitelist_words.txt
SPELLING_WHITELIST_WORDS_FILE="${ROOTDIR}/ci/spelling_whitelist_words.txt"

WHITELIST_WORDS=$(echo -n $(cat "${SPELLING_WHITELIST_WORDS_FILE}" | \
                  grep -v "^#"|grep -v "^$") | tr ' ' ',')
SKIP_FILES=$(echo $(cat "${SPELLING_SKIP_FILES}") | sed "s| | -e |g")
if [ -z "$SKIP_FILES"]; then
 FILTER_FILES="cat"
else 
 FILTER_FILES="grep -v -e \"${SKIP_FILES}\""
fi

LSCOMMAND=${LSCOMMAND:-"git ls-files"}
$LSCOMMAND | $FILTER_FILES | xargs "${TMP_DIR}/misspell" -i \
  "${WHITELIST_WORDS}" ${MISSPELL_ARGS}

echo Done
