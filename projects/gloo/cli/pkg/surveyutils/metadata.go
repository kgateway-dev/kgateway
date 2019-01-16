package surveyutils

import (
	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
)

func MetadataSurvey(metadata *core.Metadata) error {
	if err := cliutil.GetStringInput("name of the resource: ", &metadata.Namespace); err != nil {
		return err
	}
	if err := cliutil.GetStringInput("namespace of the resource: ", &metadata.Namespace); err != nil {
		return err
	}
	return nil
}
