---
title: Changelog Entry Types
weight: 90
description: Explanation of the entry types used in our changelogs
---

You will find several different kinds of changelog entries:

- **Dependency Bumps**

A notice about a dependency in the project that had its version bumped in this release. Be sure to check for any
"**Breaking Change**" entries accompanying a dependency bump. For example, a `gloo`
version bump in Gloo Edge Enterprise may mean a change to a proto API.

- **Breaking Changes**

A notice of a non-backwards-compatible change to some API. This can include things like a changed
proto format, a change to the Helm chart, and other breakages. Occasionally a breaking change
may mean that the process to upgrade the product is slightly different; in that case, we will be sure
to specify in the changelog how the break must be handled.

- **Helm Changes**

A notice of a change to our Helm chart. One of these entries does not by itself signify a breaking
change to the Helm chart; you will find an accompanying "**Breaking Change**" entry in the release
notes if that is the case. 

- **New Features**

A description of a new feature that has been implemented in this release.

- **Fixes**

A description of a bug that was resolved in this release.
