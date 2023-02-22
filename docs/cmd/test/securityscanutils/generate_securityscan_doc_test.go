package securityscanutils_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/docs/cmd/securityscanutils"
)

var _ = Describe("generate security scan docgen", func() {
	Context("check markdown", func() {
		It("reports exist for gloo", func() {
			existsTag := "1.5.0"
			expectedReport := `
Vulnerability ID|Package|Severity|Installed Version|Fixed Version|Reference
---|---|---|---|---|---
CVE-2020-1967|libcrypto1.1|HIGH|1.1.1d-r3|1.1.1g-r0|https://avd.aquasec.com/nvd/cve-2020-1967
CVE-2021-23840|libcrypto1.1|HIGH|1.1.1d-r3|1.1.1j-r0|https://avd.aquasec.com/nvd/cve-2021-23840
CVE-2020-1967|libssl1.1|HIGH|1.1.1d-r3|1.1.1g-r0|https://avd.aquasec.com/nvd/cve-2020-1967
CVE-2021-23840|libssl1.1|HIGH|1.1.1d-r3|1.1.1j-r0|https://avd.aquasec.com/nvd/cve-2021-23840
`
			url := "https://storage.googleapis.com/solo-gloo-security-scans/gloo/" + existsTag + "/gateway_cve_report.docgen"
			report, err := GetSecurityScanReport(url)
			Expect(err).To(Not(HaveOccurred()))
			Expect(report).To(Equal(expectedReport))
		})

		It("reports exist for glooe", func() {
			existsTag := "1.5.0"
			expectedReport := `
Vulnerability ID|Package|Severity|Installed Version|Fixed Version|Reference
---|---|---|---|---|---
CVE-2021-23840|libcrypto1.1|HIGH|1.1.1g-r0|1.1.1j-r0|https://avd.aquasec.com/nvd/cve-2021-23840
CVE-2021-23840|libssl1.1|HIGH|1.1.1g-r0|1.1.1j-r0|https://avd.aquasec.com/nvd/cve-2021-23840
`
			url := "https://storage.googleapis.com/solo-gloo-security-scans/glooe/" + existsTag + "/gloo-ee_cve_report.docgen"
			report, err := GetSecurityScanReport(url)
			Expect(err).To(Not(HaveOccurred()))
			Expect(report).To(Equal(expectedReport))
		})

		It("report does not exist", func() {
			missingTag := "1.1.1"
			expectedReport := "No scan found\n"
			url := "https://storage.googleapis.com/solo-gloo-security-scans/" + missingTag + "/gateway_cve_report.docgen"
			report, err := GetSecurityScanReport(url)
			Expect(err).To(Not(HaveOccurred()))
			Expect(report).To(Equal(expectedReport))
		})
	})

})
