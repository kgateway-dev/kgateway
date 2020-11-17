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
repoDir=$workingDir/gloo-temp

mkdir -p $docsSiteDir
echo $firebaseJson > $docsSiteDir/firebase.json

git clone https://github.com/solo-io/gloo.git $repoDir

export PATH=$workingDir/_output/.bin:$PATH

# Generates a data/Solo.yaml file with $1 being the specified version.
# Should end up looking like the follwing:

# LatestVersion: 1.5.8
# DocsVersion: /gloo-edge/1.3.32
# CodeVersion: 1.3.32
# DocsVersions:
#   - master
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
  echo "Generating site for version $version"
  cd $repoDir
  if [[ "$version" == "master" ]]
  then
    git checkout master
  else
    git checkout tags/v"$version"
  fi
  # Replace version with "latest" if it's the latest version. This enables URLs with "/latest/..."
  if [[ "$version" ==  "$latestVersion" ]]
  then
    version="latest"
  fi

  cd docs
  # Generate data/Solo.yaml file with version info populated.
  generateHugoVersionsYaml $version
  # Use styles as defined on master, not the checked out temp repo.
  mkdir -p layouts/partials
  cp -a $workingDir/layouts/partials/. layouts/partials/
  cp -f $workingDir/Makefile Makefile
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

# Generate docs for all versions
for version in "${versions[@]}"
do
  generateSiteForVersion $version
done

# Generate docs for all previous versions
for version in "${oldVersions[@]}"
do
  generateSiteForVersion $version
done