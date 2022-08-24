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
			err = printImageReportGloo(semver)
			if err != nil {
				return err
			}
			prevMinorVersion = semver
		} else {
			fmt.Printf("<details><summary> Release %s </summary>\n\n", tag)
			err = printImageReportGloo(semver)
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

// List of images included in gloo edge open source version 1.<version>.x
func OpenSourceImages(semver *version.Version) []string {
	if semver.AtLeast(version.MustParseSemantic("1.12.0")) {
		//Removed gateway
		return []string{"access-logger", "certgen", "discovery", "gloo", "gloo-envoy-wrapper", "ingress", "sds", "kubectl"}
	} else if semver.LessThan(version.MustParseSemantic("1.12.0")) && semver.AtLeast(version.MustParseSemantic("1.11.0")) {
		//Added kubectl
		return []string{"access-logger", "certgen", "discovery", "gateway", "gloo", "gloo-envoy-wrapper", "ingress", "sds", "kubectl"}
	} else {
		return []string{"access-logger", "certgen", "discovery", "gateway", "gloo", "gloo-envoy-wrapper", "ingress", "sds"}
	}
}

// List of images only included in gloo edge enterprise
// In 1.7, we replaced the grpcserver images with gloo-fed images.
func EnterpriseImages(semver *version.Version) []string {
	extraImages := []string{"gloo-fed", "gloo-fed-apiserver", "gloo-fed-apiserver-envoy", "gloo-federation-console", "gloo-fed-rbac-validating-webhook"}
	if semver.LessThan(version.MustParseSemantic("1.7.0")) {
		extraImages = []string{"grpcserver-ui", "grpcserver-envoy", "grpcserver-ee"}
	}
	return append([]string{"rate-limit-ee", "gloo-ee", "gloo-ee-envoy-wrapper", "observability-ee", "extauth-ee", "discovery-ee", "caching-ee"}, extraImages...)
}

func printImageReportGloo(semver *version.Version) error {
	for _, image := range OpenSourceImages(semver) {
		fmt.Printf("**Gloo %s image**\n\n", image)
		url := "https://storage.googleapis.com/solo-gloo-security-scans/gloo/" + semver.String() + "/" + image + "_cve_report.docgen"
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
	for _, image := range EnterpriseImages(semver) {
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
