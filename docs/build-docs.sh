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
    ],
    "redirects": [      
      {
        "source": "/gloo-edge/master/:path*",
        "destination": "/gloo-edge/latest/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/v1.13.x/:path*",
        "destination": "/gloo-edge/latest/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/changelog/",
        "destination": "/gloo-edge/:version/reference/changelog/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/cli/",
        "destination": "/gloo-edge/:version/reference/cli/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/cli/:path",
        "destination": "/gloo-edge/:version/reference/cli/:path",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/advanced_configuration/:path",
        "destination": "/gloo-edge/:version/installation/advanced_configuration/:path",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/introduction/concepts/",
        "destination": "/gloo-edge/:version/introduction/architecture/concepts/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/introduction/whygloo/",
        "destination": "/gloo-edge/:version/introduction/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/operations/upgrading/1.3.0/",
        "destination": "/gloo-edge/:version/operations/upgrading/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/operations/upgrading/v1.14",
        "destination": "/gloo-edge/:version/operations/upgrading/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/operations/upgrading/v1.3",
        "destination": "/gloo-edge/:version/operations/upgrading/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/routes/route_destinations/single_upstreams/static_upstream/",
        "destination": "/gloo-edge/:version/guides/traffic_management/destination_types/static_upstream/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/contributing/documentation/example_doc/",
        "destination": "/gloo-edge/:version/contributing/documentation/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/dev/writing_auth_plugins",
        "destination": "/gloo-edge/:version/guides/dev/writing_auth_plugins/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/enterprise/",
        "destination": "/gloo-edge/:version/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/enterprise/authentication/",
        "destination": "/gloo-edge/:version/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_integrations/",
        "destination": "/gloo-edge/:version/guides/integrations/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_integrations/:path",
        "destination": "/gloo-edge/:version/guides/integrations/:path",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/tcp_proxy/",
        "destination": "/gloo-edge/:version/guides/traffic_management/listener_configuration/tcp_proxy/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/authentication/",
        "destination": "/gloo-edge/:version/guides/security/auth/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/authentication/:path",
        "destination": "/gloo-edge/:version/guides/security/auth/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/security/",
        "destination": "/gloo-edge/:version/guides/security/auth/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/security/:path",
        "destination": "/gloo-edge/:version/guides/security/auth/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/routes/",
        "destination": "/gloo-edge/:version/guides/traffic_management/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/routes/route_destinations/consul_services/",
        "destination": "/gloo-edge/:versiont/guides/traffic_management/destination_types/consul_services/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/routes/route_destinations/single_upstreams/ec2_upstream/",
        "destination": "/gloo-edge/:version/guides/traffic_management/destination_types/ec2_upstream/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/gloo_routing/virtual_services/routes/route_destinations/single_upstreams/function_routing/",
        "destination": "/gloo-edge/:version/guides/traffic_management/destination_types/aws_lambda/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/user_guides/",
        "destination": "/gloo-edge/:version/guides/traffic_management/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/user_guides/",
        "destination": "/gloo-edge/:version/guides/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/user_guides/:path",
        "destination": "/gloo-edge/:version/guides/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/guides/integrations/service_mesh/gloo_istio_mtls/",
        "destination": "/gloo-edge/:version/guides/integrations/service_mesh/istio/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/guides/security/access_logging/file-based-access-logging",
        "destination": "/gloo-edge/:version/guides/security/access_logging/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/security/auth/",
        "destination": "/gloo-edge/:version/guides/security/auth/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/guides/security/auth/oauth/",
        "destination": "/gloo-edge/:version/guides/security/auth/extauth/oauth/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/guides/security/auth/opa/",
        "destination": "/gloo-edge/:version/guides/security/auth/extauth/opa/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/guides/security/auth/plugin_auth",
        "destination": "/gloo-edge/:version/guides/security/auth/extauth/plugin_auth/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/security/auth/apikey_auth/",
        "destination": "/gloo-edge/:version/guides/security/auth/extauth/apikey_auth/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/guides/traffic_management/destination_types/grpc_to_rest/",
        "destination": "/gloo-edge/:version/guides/traffic_management/destination_types/grpc/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/latest/guides/traffic_management/request_processing/transformations/",
        "destination": "/gloo-edge/:version/guides/traffic_management/request_processing/transformations/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/route_table.proto.sk.md",
        "destination": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/route_table.proto.sk/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/reference/api/github.com/solo-io/solo-apis/api/gloo/enterprise.gloo/v1/auth_config.proto.sk/",
        "destination": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/enterprise/options/extauth/v1/extauth.proto.sk/#authconfig",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/v1/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/",
        "destination": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/gateway.proto.sk/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/v1/github.com/solo-io/gloo/projects/gateway/api/v1/virtual_service.proto.sk/",
        "destination": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gateway/api/v1/virtual_service.proto.sk/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/v1/github.com/solo-io/gloo/projects/gloo/api/v1/plugins/transformation/transformation.proto.sk/",
        "destination": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gloo/api/external/envoy/extensions/transformation/transformation.proto.sk/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/v1/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/",
        "destination": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/v1/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk/",
        "destination": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/upstream.proto.sk/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/reference/cha",
        "destination": "/gloo-edge/:version/reference/changelog/",
        "type":"301"
      },
      {
        "source": "/gloo-edge/:version/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/",
        "destination": "/gloo-edge/:version/reference/api/github.com/solo-io/gloo/projects/gloo/api/v1/proxy.proto.sk/",
        "type":"301"
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
