package securityscanutils

import (
	"fmt"
	"io/ioutil"
	"net/http"

	"k8s.io/apimachinery/pkg/util/version"
)

func BuildSecurityScanReportGloo(tags []string) error {
	// tags are sorted by minor version
	latestTag := tags[0]
	prevMinorVersion, _ := version.ParseSemantic(latestTag)
	for ix, tag := range tags {
		semver, err := version.ParseSemantic(tag)
		if err != nil {
			return err
		}
		if ix == 0 || semver.Minor() != prevMinorVersion.Minor() {
			fmt.Printf("\n***Latest %d.%d.x Gloo Open Source Release: %s***\n\n", semver.Major(), semver.Minor(), tag)
			err = printImageReportGloo(tag)
			if err != nil {
				return err
			}
			prevMinorVersion = semver
		} else {
			fmt.Printf("<details><summary> Release %s </summary>\n\n", tag)
			err = printImageReportGloo(tag)
			if err != nil {
				return err
			}
			fmt.Println("</details>")
		}
	}

	return nil
}

func BuildSecurityScanReportGlooE(tags []string) error {
	// tags are sorted by minor version
	latestTag := tags[0]
	prevMinorVersion, _ := version.ParseSemantic(latestTag)
	for ix, tag := range tags {
		semver, err := version.ParseSemantic(tag)
		if err != nil {
			return err
		}
		if ix == 0 || semver.Minor() != prevMinorVersion.Minor() {
			fmt.Printf("\n***Latest %d.%d.x Gloo Enterprise Release: %s***\n\n", semver.Major(), semver.Minor(), tag)
			err = printImageReportGlooE(semver)
			if err != nil {
				return err
			}
			prevMinorVersion = semver
		} else {
			fmt.Printf("<details><summary>Release %s </summary>\n\n", tag)
			err = printImageReportGlooE(semver)
			if err != nil {
				return err
			}
			fmt.Println("</details>")
		}
	}

	return nil
}

// List of images included in gloo edge open source
func OpenSourceImages() []string {
	return []string{"access-logger", "certgen", "discovery", "gateway", "gloo", "gloo-envoy-wrapper", "ingress", "sds"}
}

// List of images only included in gloo edge enterprise
// In 1.7, we replaced the grpcserver images with gloo-fed images.
// For images before 1.7, set before17 to true.
func EnterpriseImages(before17 bool) []string {
	extraImages := []string{"gloo-fed", "gloo-fed-apiserver", "gloo-fed-apiserver-envoy", "gloo-federation-console", "gloo-fed-rbac-validating-webhook"}
	if before17 {
		extraImages = []string{"grpcserver-ui", "grpcserver-envoy", "grpcserver-ee"}
	}
	return append([]string{"rate-limit-ee", "gloo-ee", "gloo-ee-envoy-wrapper", "observability-ee", "extauth-ee"}, extraImages...)
}

func printImageReportGloo(tag string) error {
	for _, image := range OpenSourceImages() {
		fmt.Printf("**Gloo %s image**\n\n", image)
		url := "https://storage.googleapis.com/solo-gloo-security-scans/gloo/" + tag + "/" + image + "_cve_report.docgen"
		report, err := GetSecurityScanReport(url)
		if err != nil {
			return err
		}
		fmt.Printf("%s\n\n", report)
	}
	return nil
}

func printImageReportGlooE(semver *version.Version) error {
	tag := semver.String()
	hasFedVersion, _ := semver.Compare("1.7.0")

	for _, image := range EnterpriseImages(hasFedVersion < 0) {
		fmt.Printf("**Gloo Enterprise %s image**\n\n", image)
		url := "https://storage.googleapis.com/solo-gloo-security-scans/solo-projects/" + tag + "/" + image + "_cve_report.docgen"
		report, err := GetSecurityScanReport(url)
		if err != nil {
			return err
		}
		fmt.Printf("%s\n\n", report)
	}
	return nil
}

func GetSecurityScanReport(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}

	var report string
	if resp.StatusCode == http.StatusOK {
		bodyBytes, _ := ioutil.ReadAll(resp.Body)
		report = string(bodyBytes)
	} else if resp.StatusCode == http.StatusNotFound {
		// Older releases may be missing scan results
		report = "No scan found\n"
	}
	resp.Body.Close()

	return report, nil
}
