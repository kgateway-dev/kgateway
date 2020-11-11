# Gloo Edge docs

## Deploying to a test site

```
make serve-site
```

## Deploying to a versioned test site

NOTE: this process should only be done from master

```
make build-docs
firebase hosting:channel:deploy $(git describe --tags) --project=solo-corp --config=docs/ci/firebase.json
```

## Building the docs

Building the docs is now done directly from the master branch, and occurs each time master is updated.
The docs are built using the `build-docs.sh` script. The script will build all relevant tags/branches of gloo
and then package them in a way which they can be deployed to firebase. The versions used are determined by the 
`active_versions.json` file.

`active_versions.json` contains 3 fields.
  1. "latest" is the name of the tag/branch which should be the default when visiting the docs. "latest" must
  be present in "versions"
  2. "versions" is the list of tags/branches which are considered up-to-date
  3. "oldVersions" is the list of supported tags/branches which are behind latest.

`build-docs.sh` clones gloo into a subdir, checks the repo out at each "version", and builds the docs. Each version
which is built is then moved into `docs/ci/public/edge/<tag>`. Once each version has been build, the whole folder can
be deployed to firebase using the following command:

`firebase deploy --only hosting --project=solo-corp --config=ci/firebase.json`

Building the docs from master allows us to make changes to the way the docs are packaged and published without 
needing to backport the changes each time. Currently, the `build-docs.sh` script copies the `layouts` folder, and the
`Makefile` before building the docs. This allows the build, and styles to remain consistent.


# Shortcode/Hugo tips
- Shortcodes cannot be embedded in other shortcodes
  - This means the "readfile" shortcode does not interpolate shortcodes embedded within the file
  - "Nesting" is different and allowed: you can "nest" short codes in the same manner that you can nest html tags
