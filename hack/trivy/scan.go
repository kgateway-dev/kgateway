package trivy

import (
	"context"
	"fmt"
	"os"
	"path"

	"github.com/hashicorp/go-multierror"
	"github.com/rotisserie/eris"

	"github.com/solo-io/go-utils/contextutils"
	"github.com/solo-io/go-utils/osutils/executils"
	"github.com/solo-io/go-utils/securityscanutils"
)

const (
	outputDir = "_output/scans"
	imageRepo = "quay.io/solo-io"
)

// selected from a recent scan result: https://github.com/solo-io/gloo/issues/7361
var imageNamesToScan = []string{
	"gloo",
	"gloo-envoy-wrapper",
	"discovery",
	"ingress",
	"sds",
	"certgen",
	"access-logger",
	"kubectl",
}

func ScanVersion(version string) error {
	ctx := context.Background()
	contextutils.LoggerFrom(ctx).Infof("Starting ScanVersion with version=%s", version)

	trivyScanner := securityscanutils.NewTrivyScanner(executils.CombinedOutputWithStatus)

	templateFile, err := securityscanutils.GetTemplateFile(securityscanutils.MarkdownTrivyTemplate)
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(templateFile)
	}()

	versionedOutputDir := path.Join(outputDir, version)
	contextutils.LoggerFrom(ctx).Infof("Results will be written to %s", versionedOutputDir)
	err = os.MkdirAll(versionedOutputDir, os.ModePerm)
	if err != nil {
		return err
	}

	var scanResults error
	for _, imageName := range imageNamesToScan {
		image := fmt.Sprintf("%s/%s:%s", imageRepo, imageName, version)
		outputFile := path.Join(versionedOutputDir, fmt.Sprintf("%s.txt", imageName))

		scanCompleted, vulnerabilityFound, scanErr := trivyScanner.ScanImage(ctx, image, templateFile, outputFile)
		contextutils.LoggerFrom(ctx).Infof(
			"Scaned Image: %v, ScanCompleted: %v, VulnerabilityFound: %v, Error: %v",
			image, scanCompleted, vulnerabilityFound, scanErr)

		if vulnerabilityFound {
			scanResults = multierror.Append(scanResults, eris.Errorf("vulnerabilities found for %s", image))
		}
	}
	return scanResults
}
