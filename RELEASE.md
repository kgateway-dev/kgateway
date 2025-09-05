# Releasing kgateway

_This document provides guidance for anyone interested in understanding how releases of kgateway are produced._

## Release Timing

The kgateway project has not yet formalized a release timing process. When that is introduced, it will be documented here.

## Executing a Release

The process for producing a release follows the [GitHub release docs](https://docs.github.com/en/repositories/releasing-projects-on-github/managing-releases-in-a-repository#creating-a-release):

1. Select the branch to release from (e.g. v2.0.x)
2. Enter the release version for the release to kick off (e.g. v2.0.3)
3. Select `Publish Release`
4. The release notes can be manually edited to contain the bug fixes/features etc. within this release. (This will be improved by https://github.com/kgateway-dev/kgateway/issues/11436)

## Future Work

[Release Improvements Milestone](https://github.com/kgateway-dev/kgateway/milestone/62) tracks desired improvements to the release workflow.