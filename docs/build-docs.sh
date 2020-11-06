#!/bin/bash

###################################################################################
# This script generates a versioned docs website for Service Mesh Hub and
# deploys to Firebase.
###################################################################################

set -ex

# Update this array with all versions of SMH to include in the versioned docs website.
declare -a versions=($(cat active_versions.json | jq -rc '."versions" | join(" ")'))
latestVersion=$(cat active_versions.json | jq -r ."latest")

# Firebase configuration
firebaseJson=$(cat <<EOF
{ 
  "hosting": {
    "site": "gloo-docs", 
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
        "destination": "/gloo/latest/index.html"
      },
      {
        "source": "/gloo",
        "destination": "/gloo/latest/index.html"
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

# install go tools to sub-repo
make -C $repoDir install-go-tools

# Generates a data/Edge.yaml file with $1 being the specified version.
function generateHugoVersionsYaml() {
  yamlFile=$repoDir/docs/data/Edge.yaml
  # Truncate file first.
  echo "LatestVersion: $latestVersion" > $yamlFile
  # /gloo prefix is needed because the site is hosted under a domain name with suffix /gloo
  echo "DocsVersion: /gloo/$1" >> $yamlFile
  echo "CodeVersion: $1" >> $yamlFile
  echo "DocsVersions:" >> $yamlFile
  for hugoVersion in "${versions[@]}"
  do
    echo "  - $hugoVersion" >> $yamlFile
  done
}


for version in "${versions[@]}"
do
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
  # go run codegen/docs/docsgen.go

  cd docs
  # Generate data/Edge.yaml file with version info populated.
  generateHugoVersionsYaml $version
  # Use nav bar as defined in main, not the checked out temp repo.
  mkdir -p layouts/partials
  cp -f $workingDir/layouts/partials/versionnavigation.html layouts/partials/versionnavigation.html
  # Generate the versioned static site.
  make site-release

  cat site-latest/index.json | node $workingDir/search/generate-search-index.js > site-latest/search-index.json
  # Copy over versioned static site to firebase content folder.
  mkdir -p $docsSiteDir/public/gloo/$version
  cp -a site-latest/. $docsSiteDir/public/gloo/$version/

  # Discard git changes and vendor_any for subsequent checkouts
  cd $repoDir
  git reset --hard
  rm -fr vendor_any
done