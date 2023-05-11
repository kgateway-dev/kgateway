# Running the Helm Test Suite
- You will need to set the VERSION environment variable - this will need to be a valid semver ex: 1.0.0
- When running locally, you can set the `renderers`' [`manifestOutputDir`](helm_suite_test.go) field to be the `debugOutputDir` const. This will store the test's rendered chart in the `_output/helm/charts` directory.
