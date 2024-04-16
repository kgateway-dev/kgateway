package constants

import "github.com/solo-io/gloo/pkg/utils/helmutils"

const (
	GlooHelmRepoTemplate    = helmutils.RemoteChartUriTemplate
	GlooReleaseName         = "gloo"
	GlooFedReleaseName      = "gloo-fed"
	KnativeServingNamespace = "knative-serving"
)

var (
	// This slice defines the valid prefixes for glooctl extension binaries within the user's PATH (e.g. "glooctl-foo").
	ValidExtensionPrefixes = []string{"glooctl"}
)
