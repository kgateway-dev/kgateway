package surveyutils

import (
	"fmt"

	"github.com/solo-io/gloo/pkg/cliutil"
	"github.com/solo-io/gloo/projects/gloo/cli/pkg/cmd/options"
)

func AddVirtualServiceFlagsInteractive(vs *options.InputVirtualService) error {

	var msgProvider = func() string {
		return fmt.Sprintf("Add a domain for this virtual service (empty defaults to all domains)? Current domains %v", vs.Domains)
	}

	if err := cliutil.GetStringSliceInputLazyPrompt(msgProvider, &vs.Domains); err != nil {
		return err
	}

	return nil
}
