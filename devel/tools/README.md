# Developer Tools
_**Note**: For tools that help maintain an installation of Gloo Gateway (the product, not the project codebase), build those tools into the [CLI](/projects/gloo/cli) instead._

## Changelog creation tool

Each PR requires a changelog. However, creating the changelog file in the right format and finding the proper directory to place it can be time-consuming. This tool helps alleviate that pain. The following script creates an empty changelog file for you:

```bash
bash devel/tools/changelog.sh
```

_**Note**: The changelog file is automatically placed in a directory based on the previous release. In between minor releases, the directory might be wrong and require you to manually adjust where the changelog is placed.**_

## Kubernetes e2e test resource example generation tool

This tool generates the input resources defined in code as an output yaml file. You can find an example under `test/kubernetes/e2e/features/headless_svc/generate/generate_examples.go`.

These examples are run as part of the codegen, but can also be manually run using the following command:

```bash
go generate <path to the generate.go file>
```