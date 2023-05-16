#!/bin/bash

###################################################################################
# This script generates a versioned docs website for Gloo Edge which can 
# then be deployed to firebase
###################################################################################

set -ex

# Update this array with all versions of Gloo Edge to include in the versioned docs website.
declare -a versions=($(cat active_versions.json | jq -rc '."versions" | join(" ")'))
declare -a oldVersions=($(cat active_versions.json | jq -rc '."oldVersions" | join(" ")'))
latestVersion=$(cat active_versions.json | jq -r ."latest")

# verify that latestVersion is in versions
latestVersionInVersions=false
for version in "${versions[@]}"
do
    if [ "$version" == "$latestVersion" ]; then
      latestVersionInVersions=true
    fi
done
if ! $latestVersionInVersions ; then
  echo "latest version not in versions, update the versions in active_versions.json"
  exit 1
fi

# Firebase configuration
firebaseJson=$(cat <<EOF
{ 
  "hosting": {
    "site": "gloo-edge", 
    "public": "public", 
    "ignore": [
      "firebase.json",
      "themes/**/*",
      "content/**/*",
      "**/.*",
      "resources/**/*",
      "examples/**/*"
    ],
    "rewrites": [      
      {
        "source": "/",
        "destination": "/gloo-edge/latest/index.html"
      },
      {
        "source": "/gloo-edge",
        "destination": "/gloo-edge/latest/index.html"
      }
    ] 
  } 
}
EOF
)

# This script assumes that the working directory is in the docs folder
workingDir=$(pwd)
docsSiteDir=$workingDir/ci
tempContentDir=$docsSiteDir/temp
tempChangelogDir=$docsSiteDir/temp_changelogs
repoDir=$workingDir/gloo-temp

mkdir -p $docsSiteDir
mkdir -p $tempContentDir
echo $firebaseJson > $docsSiteDir/firebase.json

git clone https://github.com/solo-io/gloo.git $repoDir

export PATH=$workingDir/_output/.bin:$PATH

# Generates a data/Solo.yaml file with $1 being the specified version.
# Should end up looking like the follwing:

# LatestVersion: 1.5.8
# DocsVersion: /gloo-edge/1.3.32
# CodeVersion: 1.3.32
# DocsVersions:
#   - main
#   - 1.6.0-beta8
#   - 1.5.8
# OldVersions:
#   - 1.4.15
#   - 1.3.32

function generateHugoVersionsYaml() {
  yamlFile=$repoDir/docs/data/Solo.yaml
  # Truncate file first.
  echo "LatestVersion: $latestVersion" > $yamlFile
  # /gloo-edge prefix is needed because the site is hosted under a domain name with suffix /gloo-edge
  echo "DocsVersion: /gloo-edge/$1" >> $yamlFile
  echo "CodeVersion: $1" >> $yamlFile
  echo "DocsVersions:" >> $yamlFile
  for hugoVersion in "${versions[@]}"
  do
    echo "  - $hugoVersion" >> $yamlFile
  done
  echo "OldVersions:" >> $yamlFile
  for hugoVersion in "${oldVersions[@]}"
  do
    echo "  - $hugoVersion" >> $yamlFile
  done
}


function generateSiteForVersion() {
  version=$1
  latestMainTag=$2
  echo "Generating site for version $version"
  cd $repoDir
  # Replace version with "latest" if it's the latest version. This enables URLs with "/latest/..."
  if [[ "$version" ==  "$latestVersion" ]]
  then
    version="latest"
  fi
  git checkout "$latestMainTag"

  cd docs
  # Generate data/Solo.yaml file with version info populated.
  generateHugoVersionsYaml $version

  # Replace the main's content directory with the version we're building
  rm -r $repoDir/docs/content
  mkdir $repoDir/docs/content
  cp -a $tempContentDir/$version/. $repoDir/docs/content/

  # replace the main's changelog directory with the changelogs for the version we're building
  rm -r $repoDir/changelog
  mkdir $repoDir/changelog
  cp -a $tempChangelogDir/$version/. $repoDir/changelog/

  # Remove the file responsible for the "security scan too large" bug if necessary
  guilty_path="./content/reference/security-updates"
  if cat $guilty_path/enterprise/_index.md | grep -q "glooe-security-scan-0"; then
    echo "$version contains the updated security scan template"
  else
    echo "$version does not contain the updated security scan template"
    rm -rf $guilty_path
  fi

  # Generate the versioned static site.
  make site-release

  # If we are on the latest version, then copy over `404.html` so firebase uses that.
  # https://firebase.google.com/docs/hosting/full-config#404
  if [[ "$version" ==  "latest" ]]
  then
    cp site-latest/404.html $docsSiteDir/public/404.html
  fi

  cat site-latest/index.json | node $workingDir/search/generate-search-index.js > site-latest/search-index.json
  # Copy over versioned static site to firebase content folder.
  mkdir -p $docsSiteDir/public/gloo-edge/$version
  cp -a site-latest/. $docsSiteDir/public/gloo-edge/$version/

  # Discard git changes and vendor_any for subsequent checkouts
  cd $repoDir
  git reset --hard
  rm -fr vendor_any
}

# Copies the /docs/content directory from the specified version ($1) and stores it in a temp location
function getContentForVersion() {
  version=$1
  latestMainTag=$2
  echo "Getting site content for version $version"
  cd $repoDir
  if [[ "$version" == "main" ]]
  then
    git checkout "$latestMainTag"
  else
    git checkout "$version"
  fi
  # Replace version with "latest" if it's the latest version. This enables URLs with "/latest/..."
  if [[ "$version" ==  "$latestVersion" ]]
  then
    version="latest"
  fi

  cp -a $repoDir/docs/content/. $tempContentDir/$version/

  mkdir -p $tempChangelogDir/$version
  cp -a $repoDir/changelog/. $tempChangelogDir/$version/
}

# We build docs for all active and old version of Gloo, on pull requests (and merges) to main.
# On pull requests to main by Solo developers, we want to run doc generation
# against the commit that will become the latest main commit.
# This will allow us to verify if the change we are introducing is valid.
# Therefore, we use the head SHA on pull requests by Solo developers
latestMainTag="main"
if [[ "$USE_PR_SHA_AS_MAIN" == "true" ]]
then
  latestMainTag=$PULL_REQUEST_SHA
  echo using $PULL_REQUEST_SHA, as this will be the next commit to main
fi

# Obtain /docs/content dir from all versions
for version in "${versions[@]}"
do
  getContentForVersion $version $latestMainTag
done


# Obtain /docs/content dir from all previous versions
for version in "${oldVersions[@]}"
do
  getContentForVersion $version $latestMainTag
done


# Generate docs for all versions
for version in "${versions[@]}"
do
  generateSiteForVersion $version $latestMainTag
done

# Generate docs for all previous versions
for version in "${oldVersions[@]}"
do
  generateSiteForVersion $version $latestMainTag
done