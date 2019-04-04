package surveyutils

import (
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

var PromptInteractiveResourceName = "name of the resource: "

// DEPRECATE - use EnsureMetadataSurvey
func MetadataSurvey(metadata *core.Metadata) error {
	if err := InteractiveNamespace(&metadata.Namespace); err != nil {
		return err
	}
	if err := cliutil.GetStringInput(PromptInteractiveResourceName, &metadata.Name); err != nil {
		return err
	}
	return nil
}

// EnsureMetadataSurvey uses interactive prompts to gather any missing Metadata fields.
// If a field is not empty, it will keep that value and not produce the associated prompt.
// This allows users to set some values with flags and gather any missing values interactively.
func EnsureMetadataSurvey(metadata *core.Metadata) error {
	if err := EnsureInteractiveNamespace(&metadata.Namespace); err != nil {
		return err
	}
	if metadata.Name == "" {
		if err := cliutil.GetStringInput(PromptInteractiveResourceName, &metadata.Name); err != nil {
			return err
		}
	}
	return nil
}
