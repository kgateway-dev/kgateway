## Trivy Security Scanning

Trivy is a security scanning tool which we use to scan our images for vulnerabilities.
You can run a trivy scan identical to CI on your own command line by installing trivy and running
```shell
trivy image --severity HIGH,CRITICAL quay.io/solo-io/<IMAGE>:<VERSION>
```

In CI, we do this for all versions above a certain version, specified by the `MIN_SCANNED_VERSION` environment variable.
To run our trivy scan utils locally, make sure the `_output` dir exists and run
```shell
IMAGE_REPO=quay.io/solo-io SCAN_DIR=_output/scans MIN_SCANNED_VERSION="v1.6.0" go run generate_docs.go run-security-scan -r gloo 
```

If you want to run the enterprise security scanning locally, make sure to have your `GITHUB_TOKEN` environment variable set and run
```shell
IMAGE_REPO=quay.io/solo-io SCAN_DIR=_output/scans MIN_SCANNED_VERSION="v1.6.0" go run generate_docs.go run-security-scan -r glooe 
```

### Outputs

The outputs of a trivy scan are the following:
`_output/scans/gloo/markdown_results` - a folder which has scans for each image of each version of gloo that was scanned. The scan results are in markdown format
and are uploaded to a google cloud bucket, which we later pull from during docs generation (which happens on merges to `master`), to generate a human-readable markdown
security scans document, which we [display in our docs](https://docs.solo.io/gloo-edge/master/reference/security-updates/open_source/).

`_output/scans/gloo/sarif_results` - a folder which has .sarif files containing scan results for each image of each version of gloo that was scanned.
These .sarif files are then uploaded to github, and the scan results can be seen on the security tab of the gloo repo.