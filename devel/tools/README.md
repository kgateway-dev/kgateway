# Developer Tools

## Changelog creation tool

Each PR requires a changelog. However, creating the changelog file in the right format and finding the proper directory to place it can be time-consuming. This tool helps alleviate that pain. The following script creates an empty changelog file for you:

```bash
bash devel/tools/changelog.sh
```

_**Note**: The changelog file is automatically placed in a directory based on the previous release. In between minor releases, the directory might be wrong and require you to manually adjust where the changelog is placed.**_
