## Trivy Security Scanning

Trivy is a security scanning tool which we use to scan our images for vulnerabilities.

## Scanning Images Locally
### Scan a single image
You can run a trivy scan identical to CI on your own command line by installing Trivy and running
```shell
trivy image --severity HIGH,CRITICAL quay.io/solo-io/<IMAGE>:<VERSION>
```

### Scan a single version
You can scan all Gloo Edge images for a specific version by running
```shell
VERSION=<VERSION> make scan-version
```

## Generating Scan Result Documentation Locally
### Scan open source images
Using our scanner, we can run scans against groups of images. To filter which version to scan, we use:
```shell
VERSION_CONSTRAINT=">v1.8.0, <v1.9.0" go run generate_docs.go run-security-scan -r gloo
```

### Scanning enterprise images
If you want to run the enterprise security scanning locally, make sure to have your `GITHUB_TOKEN` environment variable set and run the command with `-r` set to the enterprise repository:
```shell
VERSION_CONSTRAINT=">v1.8.0, <v1.9.0" go run generate_docs.go run-security-scan -r glooe
```

### Outputs
The outputs of a trivy scan are the following:
`_output/scans/gloo/markdown_results` - a folder which has scans for each image of each version of gloo that was scanned. The scan results are in markdown format
and are uploaded to a google cloud bucket, which we later pull from during docs generation (which happens on merges to `main`), to generate a human-readable markdown
security scans document, which we [display in our docs](https://docs.solo.io/gloo-edge/main/reference/security-updates/open_source/).