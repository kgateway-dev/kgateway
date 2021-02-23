package securityscanutils

import (
	"fmt"
	"io/ioutil"
	"net/http"
)

func BuildSecurityScanMarkdownReport(tags []string) error {
	images := []string{"gateway", "discovery", "gloo", "gloo_envoy_wrapper", "ingress", "access_logger", "sds", "certgen"}
	for _, tag := range tags {
		fmt.Printf("<details><summary> Gloo Release %s </summary>\n", tag)
		fmt.Println()
		for _, image := range images {
			fmt.Printf("**Gloo %s image**\n", image)
			fmt.Println()
			// remove `v` from tag
			url := "https://storage.googleapis.com/solo-gloo-security-scans/" + tag[1:] + "/" + image + "_cve_report.docgen"
			report, err := GetSecurityScanReport(url)
			if err != nil {
				return err
			}
			fmt.Println(report)
		}
		fmt.Println("</details>")
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
