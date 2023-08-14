# Developer Tools
A set of scripts and tools that are intended to aid in the development of Gloo Edge. 

_Tools that aid in the maintenance of a Gloo Edge installation should be built into the [CLI](/projects/gloo/cli)_

### Changelog Creation

Each PR requires a changelog, but creating the format for the changelog and identifying the proper directory to place it can be time-consuming. This tools aims to alleviate that pain. With the following script, an empty changelog will be created:

```bash
bash devel/tools/changelog.sh
```

**The directory where this is placed is based on the previous release. As a result, if you are crossing minor release boundaries, this may be inaccurate and need to be manually adjusted.**
