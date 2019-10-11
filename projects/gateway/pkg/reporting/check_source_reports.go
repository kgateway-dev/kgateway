package reporting

import (
	"github.com/solo-io/gloo/projects/gateway/pkg/translator"
	"github.com/solo-io/solo-kit/pkg/api/v1/resources/core"
	"github.com/solo-io/solo-kit/pkg/api/v2/reporter"
)

// returns true if all the source objects for the config object were accepted in the resource reports
// errors if parsing the config obj metadata fails (should never happen)
func AllSourcesAccepted(reports reporter.ResourceReports, configObj translator.ObjectWithMetadata) (bool, error) {
	allSourcesAccepted := true

	if err := translator.ForEachSource(configObj, func(src translator.SourceRef) error {
		_, report := reports.Find(src.ResourceKind, core.ResourceRef{src.Name, src.Namespace})

		if report.Errors != nil {
			allSourcesAccepted = false
		}

		return nil
	}); err != nil {
		return false, err
	}

	return allSourcesAccepted, nil
}
