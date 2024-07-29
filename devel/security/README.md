# Security Policies

## Vulnerabilities in Third-Party Libraries

Gloo is scanned on a weekly basis for security vulnerabilities. We use [trivy](https://github.com/aquasecurity/trivy) to scan our products for security issues and we scan OSS Gloo, Gloo EE, and Portal. Scans are initiated from [a single job in OSS Gloo](https://github.com/solo-io/gloo/tree/main/.github/workflows#trivy-vulnerability-scanning). This job checks out the main branch and uses that branch's `.trivyignore` while it scans all requested images and repositories. The scans can also [be run locally](https://github.com/solo-io/gloo/tree/main/docs/cmd/securityscanutils).

Occasionally, the best approach to resolve a reported CVE is to ignore it in Trivy (ie. if we determine that the vulnerability does not affect us, or if it is raised as a false positive). To support this, we use a single `.trivyignore` file [on our main branch](https://github.com/solo-io/gloo/blob/main/.trivyignore). This file is kept up-to-date with all vulnerabilities that we feel can be safely ignored.

All LTS branches are scanned from our main branch. This means that the job definitions to perform the scans are not backported to LTS branches, nor is `.trivyignore`. This is especially relevant if a user is building images for LTS branches/non-`gloo` repositories and attempting to scan them; in these cases, the locally checked out branch will not contain the correct `.trivyignore` file. Users who wish to perform a scan locally should consider downloading the `.trivyignore` file to a location that will not be updated by `git` operations and passing it to Trivy scan using the `--ignorefile` option. 
