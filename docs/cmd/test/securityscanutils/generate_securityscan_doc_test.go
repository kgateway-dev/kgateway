package securityscanutils_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	. "github.com/solo-io/gloo/docs/cmd/securityscanutils"
)

var _ = Describe("generate security scan docgen", func() {
	Context("check markdown", func() {
		It("reports exist", func() {
			existsTag := "1.5.0"
			expectedReport := `
Package|Vulnerability ID|Severity|Installed Version|Fixed Version
---|---|---|---|---
libcrypto1.1|CVE-2020-1967|HIGH|1.1.1d-r3|1.1.1g-r0
libssl1.1|CVE-2020-1967|HIGH|1.1.1d-r3|1.1.1g-r0
`
			url := "https://storage.googleapis.com/solo-gloo-security-scans/" + existsTag + "/gateway_cve_report.docgen"
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
