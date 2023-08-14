# Developer Tools

Find tools and information to help you develop the Gloo Edge project.

* `contributing`: Information to help you contribute to the project, such as how to open issues, review pull requests, generate code, and backport fixes.
* `debugging`: Troubleshooting steps for debugging frequent issues. 
* `testing`: Descriptions on how the tests work and how to use them.
* `tools`: A set of scripts and tools that are intended to help you develop the Gloo Edge project's codebase. Learn more about these tools in the short descriptions later in this readme.

_**Note**: For tools that help maintain an installation of Gloo Edge (the product, not the project codebase), build those tools into the [CLI](/projects/gloo/cli) instead._ 

Other resources:
* [Developer guide](https://docs.solo.io/gloo-edge/latest/guides/dev/) to set up your development environment and learn more about extending the functionality of the Gloo Edge project and related plug-ins. While written for external contributors, internal Solo developers might also find this guide helpful when onboarding.
* [Product documentation](https://docs.solo.io/gloo-edge/latest/) with guides for end users to use Gloo Edge as a product
* [Guide to contribute to the documentation](https://docs.solo.io/gloo-edge/latest/contributing/documentation/)

## Changelog creation tool

Each PR requires a changelog. However, creating the changelog file in the right format and finding the proper directory to place it can be time-consuming. This tool helps alleviate that pain. The following script creates an empty changelog file for you:

```bash
bash devel/tools/changelog.sh
```

_**Note**: The changelog file is automatically placed in a directory based on the previous release. In between minor releases, the directory might be wrong and require you to manually adjust where the changelog is placed.**_
