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
			fmt.Println()
			fmt.Printf("***Latest %d.%d.x Gloo Open Source Release: %s***\n", semver.Major(), semver.Minor(), tag)
			fmt.Println()
			printImageReportGloo(tag)
			prevMinorVersion = semver
		} else {
			fmt.Printf("<details><summary> Release %s </summary>\n", tag)
			fmt.Println()
			printImageReportGloo(tag)
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
			fmt.Println()
			fmt.Printf("***Latest %d.%d.x Gloo Enterprise Release: %s***\n", semver.Major(), semver.Minor(), tag)
			fmt.Println()
			printImageReportGlooE(semver)
			prevMinorVersion = semver
		} else {
			fmt.Printf("<details><summary>Release %s </summary>\n", tag)
			fmt.Println()
			printImageReportGlooE(semver)
			fmt.Println("</details>")
		}
	}

	return nil
}

func printImageReportGloo(tag string) error {
	images := []string{"gateway", "discovery", "gloo", "gloo-envoy-wrapper", "ingress", "access-logger", "sds", "certgen"}
	for _, image := range images {
		fmt.Printf("**Gloo %s image**\n", image)
		fmt.Println()
		// remove `v` from tag
		url := "https://storage.googleapis.com/solo-gloo-security-scans/gloo/" + tag[1:] + "/" + image + "_cve_report.docgen"
		report, err := GetSecurityScanReport(url)
		if err != nil {
			return err
		}
		fmt.Println(report)
		fmt.Println()
	}
	return nil
}

func printImageReportGlooE(semver *version.Version) error {
	tag := semver.String()
	images := []string{"rate-limit-ee", "grpcserver-ee", "grpcserver-envoy", "grpcserver-ui", "gloo-ee", "gloo-envoy-ee-wrapper", "observability-ee", "extauth-ee", "ext-auth-plugins"}
	fedImages := []string{"gloo-fed", "gloo-fed-apiserver", "gloo-fed-apiserver-envoy", "gloo-federation-console", "gloo-fed-rbac-validating-webhook"}
	hasFedVersion, _ := semver.Compare("1.7.0")
	if hasFedVersion >= 0 {
		images = append(images, fedImages...)
	}

	for _, image := range images {
		fmt.Printf("**Gloo Enterprise %s image**\n", image)
		fmt.Println()
		// remove `v` from tag
		url := "https://storage.googleapis.com/solo-gloo-security-scans/glooe/" + tag + "/" + image + "_cve_report.docgen"
		report, err := GetSecurityScanReport(url)
		if err != nil {
			return err
		}
		fmt.Println(report)
		fmt.Println()
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
